package database

import (
	"database/sql"
	"fmt"
	"sync"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

func TestWriterQueueConcurrentWrites(t *testing.T) {
	// Create in-memory test database
	db, err := sql.Open("sqlite3", "file:"+t.Name()+"?mode=memory&cache=shared&_foreign_keys=on")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	if err := createTables(db); err != nil {
		t.Fatalf("create tables: %v", err)
	}
	if err := runMigrations(db); err != nil {
		t.Fatalf("run migrations: %v", err)
	}

	store := &MessageStore{db: db}
	store.ensureWriter()
	defer store.Close()

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

	// Verify total count in database
	count, err := store.GetMessageCount()
	if err != nil {
		t.Fatalf("GetMessageCount error: %v", err)
	}

	if count != totalMessages {
		t.Errorf("Expected %d messages, got %d", totalMessages, count)
	}
}

func TestWriterQueueBatchResilience(t *testing.T) {
	db, err := sql.Open("sqlite3", "file:"+t.Name()+"?mode=memory&cache=shared&_foreign_keys=on")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	if err := createTables(db); err != nil {
		t.Fatalf("create tables: %v", err)
	}

	store := &MessageStore{db: db}
	store.ensureWriter()
	defer store.Close()

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
	db, err := sql.Open("sqlite3", "file:"+t.Name()+"?mode=memory&cache=shared&_foreign_keys=on")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	if err := createTables(db); err != nil {
		t.Fatalf("create tables: %v", err)
	}

	store := &MessageStore{db: db}
	store.ensureWriter()

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
