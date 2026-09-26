package database

import (
	"database/sql"
	"fmt"
	"os"
	"strings"
	"sync/atomic"

	_ "github.com/mattn/go-sqlite3"
)

// MessageStore handles database operations for storing message history and webhook configurations
type MessageStore struct {
	db     *sql.DB
	writer *writerQueue

	// requireCorporateConfirmation gates message capture on the corporate-asset attestation.
	requireCorporateConfirmation atomic.Bool
}

// NewMessageStore initializes a new message store with SQLite database
func NewMessageStore() (*MessageStore, error) {
	// Create directory for database if it doesn't exist
	if err := os.MkdirAll("store", 0755); err != nil {
		return nil, fmt.Errorf("failed to create store directory: %v", err)
	}

	// WAL mode: allows concurrent readers + one writer, preventing SQLITE_BUSY when
	// the MCP server and bridge access the same file simultaneously.
	db, err := sql.Open("sqlite3", "file:store/messages.db?_foreign_keys=on&_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("failed to open message database: %v", err)
	}

	// Create tables if they don't exist
	err = createTables(db)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to create tables: %v", err)
	}

	// Run migrations for existing databases
	if err = runMigrations(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to run migrations: %v", err)
	}

	// Versioned schema migrations (organization, instance-scoped keys, governance)
	if err = applySchemaMigrations(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to apply schema migrations: %w", err)
	}

	// Indexes come last: they reference columns added by the migrations above.
	if err = createIndexes(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to create indexes: %w", err)
	}

	store := &MessageStore{db: db}
	store.ensureWriter()

	return store, nil
}

// runMigrations applies database migrations for schema updates
func runMigrations(db *sql.DB) error {
	migrations := []struct {
		name string
		sql  string
	}{
		{
			name: "sender_name",
			sql:  `ALTER TABLE messages ADD COLUMN sender_name TEXT`,
		},
		{
			name: "quoted_message_id",
			sql:  `ALTER TABLE messages ADD COLUMN quoted_message_id TEXT`,
		},
		{
			name: "quoted_sender_name",
			sql:  `ALTER TABLE messages ADD COLUMN quoted_sender_name TEXT`,
		},
		{
			name: "quoted_text_preview",
			sql:  `ALTER TABLE messages ADD COLUMN quoted_text_preview TEXT`,
		},
		{
			name: "reply_to_message_id",
			sql:  `ALTER TABLE messages ADD COLUMN reply_to_message_id TEXT`,
		},
		{
			name: "edit_count",
			sql:  `ALTER TABLE messages ADD COLUMN edit_count INTEGER DEFAULT 0`,
		},
		{
			name: "is_edited",
			sql:  `ALTER TABLE messages ADD COLUMN is_edited BOOLEAN DEFAULT 0`,
		},
		{
			name: "is_forwarded",
			sql:  `ALTER TABLE messages ADD COLUMN is_forwarded BOOLEAN DEFAULT 0`,
		},
		{
			name: "forwarded_from",
			sql:  `ALTER TABLE messages ADD COLUMN forwarded_from TEXT`,
		},
		{
			name: "is_system_message",
			sql:  `ALTER TABLE messages ADD COLUMN is_system_message BOOLEAN DEFAULT 0`,
		},
		{
			name: "system_message_type",
			sql:  `ALTER TABLE messages ADD COLUMN system_message_type TEXT`,
		},
		{
			name: "direct_path",
			sql:  `ALTER TABLE messages ADD COLUMN direct_path TEXT`,
		},
	}

	for _, m := range migrations {
		_, err := db.Exec(m.sql)
		if err != nil && !strings.Contains(err.Error(), "duplicate column name") {
			// Unexpected migration error - log but don't fail
			fmt.Printf("Warning: migration error (%s column): %v\n", m.name, err)
		}
	}

	// Ensure web_sessions table exists for migrated databases
	_, _ = db.Exec(`
		CREATE TABLE IF NOT EXISTS web_sessions (
			token TEXT PRIMARY KEY,
			id TEXT UNIQUE NOT NULL,
			username TEXT NOT NULL,
			created_at TIMESTAMP NOT NULL,
			last_seen_at TIMESTAMP NOT NULL,
			expires_at TIMESTAMP NOT NULL,
			ip TEXT,
			user_agent TEXT
		);
	`)

	return nil
}

// createTables creates all necessary database tables
func createTables(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS chats (
			jid TEXT PRIMARY KEY,
			name TEXT,
			last_message_time TIMESTAMP
		);

		CREATE TABLE IF NOT EXISTS messages (
			id TEXT,
			chat_jid TEXT,
			sender TEXT,
			sender_name TEXT,
			content TEXT,
			timestamp TIMESTAMP,
			is_from_me BOOLEAN,
			media_type TEXT,
			filename TEXT,
			url TEXT,
			media_key BLOB,
			file_sha256 BLOB,
			file_enc_sha256 BLOB,
			file_length INTEGER,
			direct_path TEXT,
			quoted_message_id TEXT,
			quoted_sender_name TEXT,
			quoted_text_preview TEXT,
			reply_to_message_id TEXT,
			edit_count INTEGER DEFAULT 0,
			is_edited BOOLEAN DEFAULT 0,
			is_forwarded BOOLEAN DEFAULT 0,
			forwarded_from TEXT,
			is_system_message BOOLEAN DEFAULT 0,
			system_message_type TEXT,
			PRIMARY KEY (id, chat_jid),
			FOREIGN KEY (chat_jid) REFERENCES chats(jid)
		);

		CREATE TABLE IF NOT EXISTS contact_nicknames (
			jid TEXT PRIMARY KEY,
			nickname TEXT NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);

		CREATE TABLE IF NOT EXISTS webhook_configs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			webhook_url TEXT NOT NULL,
			secret_token TEXT,
			enabled BOOLEAN DEFAULT 1,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);

		CREATE TABLE IF NOT EXISTS webhook_triggers (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			webhook_config_id INTEGER REFERENCES webhook_configs(id),
			trigger_type TEXT NOT NULL,
			trigger_value TEXT,
			match_type TEXT DEFAULT 'exact',
			enabled BOOLEAN DEFAULT 1
		);

		CREATE TABLE IF NOT EXISTS webhook_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			webhook_config_id INTEGER REFERENCES webhook_configs(id),
			message_id TEXT,
			chat_jid TEXT,
			trigger_type TEXT,
			trigger_value TEXT,
			payload TEXT,
			response_status INTEGER,
			response_body TEXT,
			attempt_count INTEGER DEFAULT 1,
			delivered_at TIMESTAMP,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);

		CREATE TABLE IF NOT EXISTS web_sessions (
			token TEXT PRIMARY KEY,
			id TEXT UNIQUE NOT NULL,
			username TEXT NOT NULL,
			created_at TIMESTAMP NOT NULL,
			last_seen_at TIMESTAMP NOT NULL,
			expires_at TIMESTAMP NOT NULL,
			ip TEXT,
			user_agent TEXT
		);
	`)
	return err
}

// createIndexes creates the baseline indexes. Indexes that depend on migration-added
// columns are created by the migrations themselves.
func createIndexes(db *sql.DB) error {
	indexes := []string{
		`CREATE INDEX IF NOT EXISTS idx_messages_chat_timestamp ON messages(chat_jid, timestamp DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_messages_sender ON messages(sender)`,
		`CREATE INDEX IF NOT EXISTS idx_messages_media_type ON messages(media_type)`,
		`CREATE INDEX IF NOT EXISTS idx_chats_last_message_time ON chats(last_message_time DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_messages_chat_id ON messages(chat_jid, id)`,
		`CREATE INDEX IF NOT EXISTS idx_messages_instance_ts ON messages(instance_jid, timestamp DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_messages_ts ON messages(timestamp DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_messages_deleted ON messages(is_deleted_remote) WHERE is_deleted_remote = 1`,
		`CREATE INDEX IF NOT EXISTS idx_web_sessions_expires ON web_sessions(expires_at)`,
	}
	for _, idx := range indexes {
		if _, err := db.Exec(idx); err != nil {
			return fmt.Errorf("index (%s): %w", idx, err)
		}
	}
	return nil
}

// Close flushes the writer queue and closes the database connection.
func (store *MessageStore) Close() error {
	if store.writer != nil {
		store.writer.close()
	}
	return store.db.Close()
}

// GetDB returns the underlying database connection for direct access
func (store *MessageStore) GetDB() *sql.DB {
	return store.db
}
