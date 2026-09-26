package database

import (
	"testing"
	"time"
)

// newTestStore opens a real store in a temporary working directory, running the full startup
// path (baseline schema, versioned migrations, indexes) exactly as the bridge does.
func newTestStore(t *testing.T) *MessageStore {
	t.Helper()
	t.Chdir(t.TempDir())

	store, err := NewMessageStore()
	if err != nil {
		t.Fatalf("NewMessageStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

// putMessage stores a text message for an instance, creating its chat first.
func putMessage(t *testing.T, store *MessageStore, instance, chat, id, sender, content string, ts time.Time) {
	t.Helper()
	if err := store.StoreChatWithInstance(chat, "Chat "+chat, ts, instance); err != nil {
		t.Fatalf("StoreChatWithInstance: %v", err)
	}
	if err := store.StoreMessageWithInstance(id, chat, sender, sender, content, ts, false, "", "", "", "", nil, nil, nil, 0, instance, false); err != nil {
		t.Fatalf("StoreMessageWithInstance: %v", err)
	}
}

func countRows(t *testing.T, store *MessageStore, query string, args ...any) int {
	t.Helper()
	var n int
	if err := store.db.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return n
}

func timeNow() time.Time { return time.Now() }
