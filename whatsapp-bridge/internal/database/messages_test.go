package database

import (
	"testing"
	"time"
)

func newTestMessageStore(t *testing.T) *MessageStore {
	t.Helper()
	return newTestStore(t)
}

func TestStoreChatKeepsNewestTimestamp(t *testing.T) {
	store := newTestMessageStore(t)
	older := time.Date(2026, 5, 15, 10, 0, 0, 0, time.UTC)
	newer := older.Add(2 * time.Hour)

	if err := store.StoreChat("123@s.whatsapp.net", "Alice", newer); err != nil {
		t.Fatalf("store newer chat: %v", err)
	}
	if err := store.StoreChat("123@s.whatsapp.net", "Alice Old", older); err != nil {
		t.Fatalf("store older chat: %v", err)
	}

	var name string
	var got time.Time
	if err := store.db.QueryRow("SELECT name, last_message_time FROM chats WHERE jid = ?", "123@s.whatsapp.net").Scan(&name, &got); err != nil {
		t.Fatalf("read chat: %v", err)
	}
	if !got.Equal(newer) {
		t.Fatalf("last_message_time = %v, want %v", got, newer)
	}
	if name != "Alice Old" {
		t.Fatalf("name = %q, want latest non-empty name", name)
	}
}

func TestStoreChatEmptyNameDoesNotEraseExistingName(t *testing.T) {
	store := newTestMessageStore(t)
	now := time.Date(2026, 5, 15, 10, 0, 0, 0, time.UTC)

	if err := store.StoreChat("123@s.whatsapp.net", "Alice", now); err != nil {
		t.Fatalf("store named chat: %v", err)
	}
	if err := store.StoreChat("123@s.whatsapp.net", "", now.Add(time.Hour)); err != nil {
		t.Fatalf("store empty-name chat: %v", err)
	}

	var name string
	if err := store.db.QueryRow("SELECT name FROM chats WHERE jid = ?", "123@s.whatsapp.net").Scan(&name); err != nil {
		t.Fatalf("read chat: %v", err)
	}
	if name != "Alice" {
		t.Fatalf("name = %q, want existing name", name)
	}
}
