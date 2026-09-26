package database

import (
	"database/sql"
	"os"
	"strings"
	"testing"
)

// buildLegacyDatabase recreates the state left by the previous release: baseline tables plus the
// first organization migration, with messages keyed by (id, chat_jid), and some data.
func buildLegacyDatabase(t *testing.T) {
	t.Helper()
	if err := os.MkdirAll("store", 0o755); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite3", "file:store/messages.db?_foreign_keys=on&_journal_mode=WAL")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if err := createTables(db); err != nil {
		t.Fatal(err)
	}
	if err := runMigrations(db); err != nil {
		t.Fatal(err)
	}
	if err := runSQLMigrationFile(db, "002_add_organization_and_instances.sql"); err != nil {
		t.Fatal(err)
	}

	for _, q := range []string{
		`INSERT INTO chats (jid, name, last_message_time) VALUES ('a@s.whatsapp.net', 'A', '2026-05-01 10:00:00+00:00')`,
		`INSERT INTO messages (id, chat_jid, sender, content, timestamp, is_from_me) VALUES ('M1', 'a@s.whatsapp.net', 'a', 'um', '2026-05-01 10:00:00+00:00', 0)`,
		`INSERT INTO messages (id, chat_jid, sender, content, timestamp, is_from_me) VALUES ('M2', 'a@s.whatsapp.net', 'a', 'dois', '2026-05-01 10:01:00+00:00', 0)`,
		`INSERT INTO messages (id, chat_jid, sender, content, timestamp, is_from_me, instance_jid) VALUES ('M3', 'a@s.whatsapp.net', 'a', 'tres', '2026-05-01 10:02:00+00:00', 0, '5511@s.whatsapp.net')`,
		`DELETE FROM messages WHERE id = 'M2'`, // leaves a gap in the rowids
		`INSERT INTO departments (name) VALUES ('Comercial')`,
		`INSERT INTO employees (department_id, name) VALUES (1, 'Ana')`,
		`INSERT INTO instances (phone_jid, employee_id, alias, status) VALUES ('5511@s.whatsapp.net', 1, 'Vendas', 'connected')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
}

func TestLegacyDatabaseMigratesWithoutLosingData(t *testing.T) {
	t.Chdir(t.TempDir())
	buildLegacyDatabase(t)

	var rowIDsBefore []int64
	func() {
		db, _ := sql.Open("sqlite3", "file:store/messages.db")
		defer db.Close()
		rows, _ := db.Query("SELECT rowid FROM messages ORDER BY rowid")
		defer rows.Close()
		for rows.Next() {
			var id int64
			_ = rows.Scan(&id)
			rowIDsBefore = append(rowIDsBefore, id)
		}
	}()

	store, err := NewMessageStore()
	if err != nil {
		t.Fatalf("legacy database failed to migrate: %v", err)
	}
	defer store.Close()

	// The primary key now includes the instance.
	cols, _, err := tableColumns(t.Context(), store.db, "messages")
	if err != nil {
		t.Fatal(err)
	}
	if cols["instance_jid"].pk == 0 {
		t.Error("instance_jid should be part of the messages primary key")
	}

	// Data and rowids (the indexer's resume cursor) survived.
	var rowIDsAfter []int64
	rows, _ := store.db.Query("SELECT rowid FROM messages ORDER BY rowid")
	for rows.Next() {
		var id int64
		_ = rows.Scan(&id)
		rowIDsAfter = append(rowIDsAfter, id)
	}
	rows.Close()
	if len(rowIDsAfter) != len(rowIDsBefore) || rowIDsAfter[len(rowIDsAfter)-1] != rowIDsBefore[len(rowIDsBefore)-1] {
		t.Errorf("rowids changed: %v -> %v", rowIDsBefore, rowIDsAfter)
	}
	var inst string
	_ = store.db.QueryRow("SELECT instance_jid FROM messages WHERE id = 'M3'").Scan(&inst)
	if inst != "5511@s.whatsapp.net" {
		t.Errorf("instance attribution lost: %q", inst)
	}
	if got := countRows(t, store, "SELECT COUNT(*) FROM messages WHERE id = 'M1' AND instance_jid = ''"); got != 1 {
		t.Error("unattributed legacy message should get an empty instance")
	}

	// Organization data and the instance row survived, and instances can now be pending.
	got, err := store.GetInstanceByPhone("5511@s.whatsapp.net")
	if err != nil || got == nil || got.EmployeeName != "Ana" || got.DepartmentName != "Comercial" || !got.AllowSend {
		t.Fatalf("instance lost or wrong: %+v err=%v", got, err)
	}
	if _, err := store.CreatePendingInstance("novo", nil, false, nil); err != nil {
		t.Errorf("phone_jid should be nullable now: %v", err)
	}
	// The existing owner became the first assignment.
	if n := countRows(t, store, "SELECT COUNT(*) FROM instance_assignments WHERE employee_id = 1 AND valid_to IS NULL"); n != 1 {
		t.Errorf("existing owner should have an open assignment, got %d", n)
	}
	// Two instances can now hold the same message id.
	putMessage(t, store, "5522@s.whatsapp.net", "a@s.whatsapp.net", "M3", "a", "tres", timeNow())

	// A backup was written before the rebuild.
	backups, _ := os.ReadDir("store")
	found := false
	for _, e := range backups {
		if strings.HasPrefix(e.Name(), "messages.db.pre-003") {
			found = true
		}
	}
	if !found {
		t.Error("expected a pre-migration backup next to the database")
	}

	// Every step is recorded, and reopening applies nothing again.
	if n := countRows(t, store, "SELECT COUNT(*) FROM schema_migrations"); n != 3 {
		t.Errorf("expected 3 recorded migrations, got %d", n)
	}
	store.Close()
	again, err := NewMessageStore()
	if err != nil {
		t.Fatalf("second start failed: %v", err)
	}
	defer again.Close()
	if n := countRows(t, again, "SELECT COUNT(*) FROM messages"); n != 3 {
		t.Errorf("second start changed the data: %d messages", n)
	}
}

func TestFreshDatabaseHasFinalSchema(t *testing.T) {
	store := newTestStore(t)
	for _, table := range []string{"messages", "instances", "chat_instances", "instance_assignments", "message_versions", "access_log", "privacy_log", "schema_migrations"} {
		if countRows(t, store, "SELECT COUNT(*) FROM sqlite_master WHERE name = ?", table) != 1 {
			t.Errorf("table %s missing", table)
		}
	}
	if countRows(t, store, "SELECT COUNT(*) FROM sqlite_master WHERE type = 'view' AND name = 'messages_unique'") != 1 {
		t.Error("view messages_unique missing")
	}
}

func TestSplitSQLStatements(t *testing.T) {
	got := splitSQLStatements("-- comment; with semicolon\nCREATE TABLE a (x);\n\n-- another\nINSERT INTO a VALUES (1);\n")
	if len(got) != 2 || !strings.HasPrefix(got[0], "CREATE TABLE") || !strings.HasPrefix(got[1], "INSERT") {
		t.Errorf("unexpected statements: %q", got)
	}
}
