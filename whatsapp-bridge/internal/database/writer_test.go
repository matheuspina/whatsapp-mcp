package database

import (
	"database/sql"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

func TestWriterQueueConcurrentWrites(t *testing.T) {
	store := newTestStore(t)

	const numGoroutines = 30
	const messagesPerGoroutine = 10
	totalMessages := numGoroutines * messagesPerGoroutine

	var wg sync.WaitGroup
	errCh := make(chan error, totalMessages*2)

	// Concurrently write chats and messages across many goroutines
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(routineID int) {
			defer wg.Done()
			chatJID := fmt.Sprintf("chat_%d@s.whatsapp.net", routineID)

			if err := store.StoreChat(chatJID, fmt.Sprintf("Chat %d", routineID), time.Now()); err != nil {
				errCh <- fmt.Errorf("routine %d store chat: %w", routineID, err)
				return
			}

			for j := 0; j < messagesPerGoroutine; j++ {
				msgID := fmt.Sprintf("msg_%d_%d", routineID, j)
				sender := fmt.Sprintf("sender_%d", routineID)
				content := fmt.Sprintf("Hello world from routine %d msg %d", routineID, j)
				ts := time.Now()

				// Alternate between priority (regular) and bulk writes
				var err error
				if j%2 == 0 {
					err = store.StoreMessage(msgID, chatJID, sender, sender, content, ts, false, "", "", "", "", nil, nil, nil, 0)
				} else {
					err = store.StoreMessageBulk(msgID, chatJID, sender, sender, content, ts, false, "", "", "", "", nil, nil, nil, 0)
				}

				if err != nil {
					errCh <- fmt.Errorf("routine %d msg %d: %w", routineID, j, err)
				}
			}
		}(i)
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Errorf("Concurrent write error: %v", err)
	}

	// Bulk writes return once queued, so give the worker a moment to drain them.
	var count int
	var err error
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		if count, err = store.GetMessageCount(); err != nil || count == totalMessages {
			break
		}
	}
	if err != nil {
		t.Fatalf("GetMessageCount error: %v", err)
	}

	if count != totalMessages {
		t.Errorf("Expected %d messages, got %d", totalMessages, count)
	}
}

func TestWriterQueueBatchResilience(t *testing.T) {
	store := newTestStore(t)

	// Enqueue valid write
	err1 := store.StoreChat("valid_1@s.whatsapp.net", "Valid 1", time.Now())
	if err1 != nil {
		t.Fatalf("store valid chat 1: %v", err1)
	}

	// Enqueue an invalid write statement
	errBad := store.enqueueWrite(func(tx *sql.Tx) error {
		_, err := tx.Exec("INSERT INTO non_existent_table VALUES (1, 2, 3)")
		return err
	}, false, true)

	if errBad == nil {
		t.Errorf("expected error for bad table, got nil")
	}

	// Enqueue another valid write right after
	err2 := store.StoreChat("valid_2@s.whatsapp.net", "Valid 2", time.Now())
	if err2 != nil {
		t.Fatalf("store valid chat 2: %v", err2)
	}

	// Verify that valid writes survived and were committed despite the bad query
	chats, err := store.GetChats()
	if err != nil {
		t.Fatalf("get chats: %v", err)
	}

	if len(chats) != 2 {
		t.Errorf("expected 2 valid chats, got %d", len(chats))
	}
}

func TestWriterQueueCleanShutdown(t *testing.T) {
	t.Chdir(t.TempDir())
	store, err := NewMessageStore()
	if err != nil {
		t.Fatalf("NewMessageStore: %v", err)
	}

	// Store some items
	for i := 0; i < 20; i++ {
		_ = store.StoreChat(fmt.Sprintf("chat_%d@s.whatsapp.net", i), fmt.Sprintf("Chat %d", i), time.Now())
	}

	// Close store
	if err := store.Close(); err != nil {
		t.Fatalf("store.Close failed: %v", err)
	}

	// Writes after close should return ErrStoreClosed
	err = store.StoreChat("after_close@s.whatsapp.net", "After", time.Now())
	if err != ErrStoreClosed {
		t.Errorf("expected ErrStoreClosed, got %v", err)
	}
}

func TestWriterBulkWritesAreNotThrottledPerMessage(t *testing.T) {
	store := newTestStore(t)
	if err := store.StoreChat("bulk@s.whatsapp.net", "Bulk", time.Now()); err != nil {
		t.Fatal(err)
	}

	const n = 1000
	start := time.Now()
	for i := 0; i < n; i++ {
		if err := store.StoreMessageBulk(fmt.Sprintf("id%d", i), "bulk@s.whatsapp.net", "s", "s", "hello", time.Now(), false, "", "", "", "", nil, nil, nil, 0); err != nil {
			t.Fatal(err)
		}
	}

	// Bulk writes return once queued; wait for the worker to drain them.
	deadline := time.Now().Add(10 * time.Second)
	for countRows(t, store, "SELECT COUNT(*) FROM messages") < n {
		if time.Now().After(deadline) {
			t.Fatalf("only %d of %d bulk messages committed after 10s", countRows(t, store, "SELECT COUNT(*) FROM messages"), n)
		}
		time.Sleep(10 * time.Millisecond)
	}

	// The old design committed one message per 25ms tick: 1000 messages took 25s.
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("bulk of %d messages took %v", n, elapsed)
	}
}

func TestWriterFailedTasksDoNotAffectTheirBatch(t *testing.T) {
	store := newTestStore(t)

	const total = 60
	var wg sync.WaitGroup
	results := make([]error, total)
	for i := 0; i < total; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i] = store.enqueueWrite(func(tx *sql.Tx) error {
				if i%6 == 0 {
					_, err := tx.Exec("INSERT INTO no_such_table VALUES (1)")
					return err
				}
				_, err := tx.Exec("INSERT INTO chats (jid, name) VALUES (?, ?)", fmt.Sprintf("c%d@s.whatsapp.net", i), "c")
				return err
			}, false, true)
		}(i)
	}
	wg.Wait()

	failed := 0
	for i, err := range results {
		if (i%6 == 0) != (err != nil) {
			t.Errorf("task %d: unexpected result %v", i, err)
		}
		if err != nil {
			failed++
		}
	}
	if got := countRows(t, store, "SELECT COUNT(*) FROM chats"); got != total-failed {
		t.Errorf("expected %d committed chats, got %d", total-failed, got)
	}
}

func TestWriterCloseNeverLosesAcceptedWrites(t *testing.T) {
	t.Chdir(t.TempDir())
	store, err := NewMessageStore()
	if err != nil {
		t.Fatal(err)
	}

	var accepted int64
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				err := store.StoreChat(fmt.Sprintf("g%d_%d@s.whatsapp.net", g, i), "x", time.Now())
				switch err {
				case nil:
					atomic.AddInt64(&accepted, 1)
				case ErrStoreClosed:
					return
				default:
					t.Errorf("unexpected error: %v", err)
					return
				}
			}
		}(g)
	}

	time.Sleep(15 * time.Millisecond)
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	wg.Wait()

	reopened, err := NewMessageStore()
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if got := countRows(t, reopened, "SELECT COUNT(*) FROM chats"); int64(got) != atomic.LoadInt64(&accepted) {
		t.Errorf("%d writes were acknowledged but %d chats are stored", accepted, got)
	}
}
