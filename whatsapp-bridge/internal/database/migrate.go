package database

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"time"

	"whatsapp-bridge/migrations"
)

// schemaMigration is one versioned step. Exactly one of file or fn is set: SQL scripts live
// in whatsapp-bridge/migrations, while steps that need conditional logic (SQLite cannot
// change a primary key in place) are written in Go.
type schemaMigration struct {
	version string
	file    string
	fn      func(db *sql.DB) error
}

var schemaMigrationSteps = []schemaMigration{
	{version: "002_add_organization_and_instances", file: "002_add_organization_and_instances.sql"},
	{version: "003_instance_scoped_keys", fn: migrateInstanceScopedKeys},
	{version: "004_governance", file: "004_governance.sql"},
}

// applySchemaMigrations runs every step not yet recorded in schema_migrations, in order.
// Each step is safe to run against a database that already contains its changes (databases
// migrated by hand or by an earlier bridge version): duplicate-column errors are tolerated
// and every CREATE uses IF NOT EXISTS.
func applySchemaMigrations(db *sql.DB) error {
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version TEXT PRIMARY KEY,
		applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	)`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	for _, step := range schemaMigrationSteps {
		var applied int
		if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version = ?`, step.version).Scan(&applied); err != nil {
			return fmt.Errorf("check migration %s: %w", step.version, err)
		}
		if applied > 0 {
			continue
		}

		var err error
		if step.fn != nil {
			err = step.fn(db)
		} else {
			err = runSQLMigrationFile(db, step.file)
		}
		if err != nil {
			return fmt.Errorf("migration %s: %w", step.version, err)
		}

		if _, err := db.Exec(`INSERT OR IGNORE INTO schema_migrations (version) VALUES (?)`, step.version); err != nil {
			return fmt.Errorf("record migration %s: %w", step.version, err)
		}
	}
	return nil
}

// runSQLMigrationFile executes an embedded script inside one transaction.
func runSQLMigrationFile(db *sql.DB, file string) error {
	raw, err := migrations.FS.ReadFile(file)
	if err != nil {
		return fmt.Errorf("read %s: %w", file, err)
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	for _, stmt := range splitSQLStatements(string(raw)) {
		if _, err := tx.Exec(stmt); err != nil {
			if strings.Contains(err.Error(), "duplicate column name") {
				continue
			}
			return fmt.Errorf("%s: %w (statement: %.80s)", file, err, stmt)
		}
	}
	return tx.Commit()
}

// splitSQLStatements drops whole-line comments and splits on semicolons. The migration
// scripts contain no triggers and no semicolons inside string literals.
func splitSQLStatements(script string) []string {
	var kept []string
	for _, line := range strings.Split(script, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "--") {
			continue
		}
		kept = append(kept, line)
	}

	var stmts []string
	for _, part := range strings.Split(strings.Join(kept, "\n"), ";") {
		if s := strings.TrimSpace(part); s != "" {
			stmts = append(stmts, s)
		}
	}
	return stmts
}

// tableColumns returns column name -> (pk position, not-null) for a table.
type columnInfo struct {
	pk      int
	notNull bool
}

func tableColumns(ctx context.Context, q interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}, table string) (map[string]columnInfo, []string, error) {
	rows, err := q.QueryContext(ctx, fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	cols := map[string]columnInfo{}
	var order []string
	for rows.Next() {
		var cid, notNull, pk int
		var name, ctype string
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notNull, &dflt, &pk); err != nil {
			return nil, nil, err
		}
		cols[name] = columnInfo{pk: pk, notNull: notNull == 1}
		order = append(order, name)
	}
	return cols, order, rows.Err()
}

const messagesInstanceScopedDDL = `CREATE TABLE messages_new (
	id TEXT NOT NULL,
	chat_jid TEXT NOT NULL,
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
	instance_jid TEXT NOT NULL DEFAULT '',
	is_deleted_remote BOOLEAN DEFAULT 0,
	deleted_at TIMESTAMP,
	deleted_by TEXT,
	PRIMARY KEY (instance_jid, chat_jid, id),
	FOREIGN KEY (chat_jid) REFERENCES chats(jid)
)`

const instancesNullableJIDDDL = `CREATE TABLE instances_new (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	phone_jid TEXT UNIQUE,
	employee_id INTEGER,
	alias TEXT,
	status TEXT DEFAULT 'disconnected',
	paired_at TIMESTAMP,
	last_seen_at TIMESTAMP,
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (employee_id) REFERENCES employees(id) ON DELETE SET NULL
)`

// migrateInstanceScopedKeys rebuilds two tables whose keys cannot be altered in place:
//
//   - messages: the primary key becomes (instance_jid, chat_jid, id) so the same message
//     seen by two monitored numbers (a shared group) is stored once per instance instead of
//     one row overwriting the other. Rows keep their rowid, which the search indexer uses
//     as its resume cursor.
//   - instances: phone_jid becomes nullable so a number that is still pairing (no JID yet)
//     can have a row.
//
// This is the one migration that rebuilds tables. It only runs when the old shape is
// detected, inside a transaction, after writing a consistent backup next to the database.
func migrateInstanceScopedKeys(db *sql.DB) error {
	ctx := context.Background()
	conn, err := db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()

	msgCols, msgOrder, err := tableColumns(ctx, conn, "messages")
	if err != nil {
		return fmt.Errorf("inspect messages: %w", err)
	}
	instCols, instOrder, err := tableColumns(ctx, conn, "instances")
	if err != nil {
		return fmt.Errorf("inspect instances: %w", err)
	}

	needMessages := msgCols["instance_jid"].pk == 0
	needInstances := len(instCols) > 0 && instCols["phone_jid"].notNull
	if !needMessages && !needInstances {
		return nil
	}

	var messageRows int
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM messages`).Scan(&messageRows); err != nil {
		return err
	}
	if messageRows > 0 {
		if err := backupBeforeRebuild(ctx, conn); err != nil {
			return fmt.Errorf("backup before rebuild: %w", err)
		}
	}

	if _, err := conn.ExecContext(ctx, `PRAGMA foreign_keys = OFF`); err != nil {
		return err
	}
	defer func() { _, _ = conn.ExecContext(ctx, `PRAGMA foreign_keys = ON`) }()

	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if needMessages {
		if err := rebuildMessages(ctx, tx, msgOrder); err != nil {
			return fmt.Errorf("rebuild messages: %w", err)
		}
	}
	if needInstances {
		if err := rebuildInstances(ctx, tx, instOrder); err != nil {
			return fmt.Errorf("rebuild instances: %w", err)
		}
	}
	return tx.Commit()
}

func backupBeforeRebuild(ctx context.Context, conn *sql.Conn) error {
	path := "store/messages.db.pre-003.bak"
	if _, err := os.Stat(path); err == nil {
		path = fmt.Sprintf("store/messages.db.pre-003.%s.bak", time.Now().UTC().Format("20060102T150405Z"))
	}
	_, err := conn.ExecContext(ctx, fmt.Sprintf("VACUUM INTO '%s'", strings.ReplaceAll(path, "'", "''")))
	return err
}

func rebuildMessages(ctx context.Context, tx *sql.Tx, oldOrder []string) error {
	if _, err := tx.ExecContext(ctx, `DROP TABLE IF EXISTS messages_new`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, messagesInstanceScopedDDL); err != nil {
		return err
	}

	newCols, newOrder, err := tableColumns(ctx, tx, "messages_new")
	if err != nil {
		return err
	}
	old := map[string]bool{}
	for _, c := range oldOrder {
		old[c] = true
	}

	var dst, src []string
	for _, c := range newOrder {
		if !old[c] {
			continue
		}
		dst = append(dst, c)
		switch c {
		case "id", "chat_jid", "instance_jid":
			src = append(src, fmt.Sprintf("COALESCE(%s, '')", c))
		default:
			src = append(src, c)
		}
	}
	_ = newCols

	stmt := fmt.Sprintf("INSERT OR IGNORE INTO messages_new (rowid, %s) SELECT rowid, %s FROM messages ORDER BY rowid",
		strings.Join(dst, ", "), strings.Join(src, ", "))
	if _, err := tx.ExecContext(ctx, stmt); err != nil {
		return err
	}
	for _, s := range []string{
		`DROP TABLE messages`,
		`ALTER TABLE messages_new RENAME TO messages`,
		`CREATE INDEX IF NOT EXISTS idx_messages_instance_chat ON messages(instance_jid, chat_jid, timestamp DESC)`,
	} {
		if _, err := tx.ExecContext(ctx, s); err != nil {
			return err
		}
	}
	return nil
}

func rebuildInstances(ctx context.Context, tx *sql.Tx, oldOrder []string) error {
	if _, err := tx.ExecContext(ctx, `DROP TABLE IF EXISTS instances_new`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, instancesNullableJIDDDL); err != nil {
		return err
	}
	_, newOrder, err := tableColumns(ctx, tx, "instances_new")
	if err != nil {
		return err
	}
	old := map[string]bool{}
	for _, c := range oldOrder {
		old[c] = true
	}
	var cols []string
	for _, c := range newOrder {
		if old[c] {
			cols = append(cols, c)
		}
	}
	list := strings.Join(cols, ", ")
	if _, err := tx.ExecContext(ctx, fmt.Sprintf("INSERT INTO instances_new (%s) SELECT %s FROM instances", list, list)); err != nil {
		return err
	}
	for _, s := range []string{
		`DROP TABLE instances`,
		`ALTER TABLE instances_new RENAME TO instances`,
		`CREATE INDEX IF NOT EXISTS idx_instances_employee ON instances(employee_id)`,
	} {
		if _, err := tx.ExecContext(ctx, s); err != nil {
			return err
		}
	}
	return nil
}
