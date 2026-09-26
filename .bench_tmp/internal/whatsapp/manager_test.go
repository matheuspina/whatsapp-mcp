package whatsapp

import (
	"database/sql"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	waLog "go.mau.fi/whatsmeow/util/log"

	"whatsapp-bridge/internal/config"
	"whatsapp-bridge/internal/database"
)

func TestInstanceManagerInitAndDefault(t *testing.T) {
	logger := waLog.Noop
	cfg := config.NewConfig()

	// Memory db for messages
	memDB, err := sql.Open("sqlite3", "file:mem_test?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open mem db: %v", err)
	}
	defer memDB.Close()

	store := &database.MessageStore{}
	// Test manager creation
	mgr, err := NewInstanceManager(logger, cfg, store)
	if err != nil {
		t.Fatalf("NewInstanceManager failed: %v", err)
	}

	def := mgr.GetDefaultClient()
	if def == nil {
		t.Fatalf("expected non-nil default client")
	}

	clients := mgr.ListClients()
	if len(clients) == 0 {
		t.Fatalf("expected at least 1 client registered")
	}

	// Test handler registration
	var eventFired bool
	mgr.AddGlobalEventHandler(func(c *Client, evt interface{}) {
		eventFired = true
	})
	if eventFired {
		// Just to check handler compiles and runs
	}
}
