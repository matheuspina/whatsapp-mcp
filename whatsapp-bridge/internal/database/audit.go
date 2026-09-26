package database

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"whatsapp-bridge/internal/types"
)

// MarkMessageRemoteDeleted flags a message as revoked by its sender (or a group admin) without
// removing it, and keeps the text that was revoked as a version. revokedBy is the user that sent
// the revoke. The row moves to a new rowid so the search indexer re-reads it. It reports whether
// the message was known.
func (store *MessageStore) MarkMessageRemoteDeleted(instanceJID, messageID, chatJID, revokedBy string, at time.Time) (bool, error) {
	found := false
	err := store.enqueueWrite(func(tx *sql.Tx) error {
		var content sql.NullString
		var alreadyDeleted bool
		err := tx.QueryRow(
			`SELECT content, COALESCE(is_deleted_remote, 0) FROM messages WHERE instance_jid = ? AND chat_jid = ? AND id = ?`,
			instanceJID, chatJID, messageID,
		).Scan(&content, &alreadyDeleted)
		if err == sql.ErrNoRows {
			return nil
		}
		if err != nil {
			return err
		}
		found = true
		if alreadyDeleted {
			return nil
		}
		if _, err := tx.Exec(
			`INSERT INTO message_versions (instance_jid, chat_jid, message_id, content, reason, recorded_at) VALUES (?, ?, ?, ?, 'delete', ?)`,
			instanceJID, chatJID, messageID, content.String, at,
		); err != nil {
			return err
		}
		_, err = tx.Exec(
			`UPDATE messages SET is_deleted_remote = 1, deleted_at = ?, deleted_by = ?, rowid = (SELECT MAX(rowid) + 1 FROM messages)
			 WHERE instance_jid = ? AND chat_jid = ? AND id = ?`,
			at, revokedBy, instanceJID, chatJID, messageID,
		)
		return err
	}, false, true)
	return found, err
}

// RecordMessageEdit stores the previous text of an edited message as a version and replaces the
// message text with the edited one. It reports whether the message was known.
func (store *MessageStore) RecordMessageEdit(instanceJID, chatJID, messageID, newContent string, at time.Time) (bool, error) {
	found := false
	err := store.enqueueWrite(func(tx *sql.Tx) error {
		var old sql.NullString
		err := tx.QueryRow(
			`SELECT content FROM messages WHERE instance_jid = ? AND chat_jid = ? AND id = ?`,
			instanceJID, chatJID, messageID,
		).Scan(&old)
		if err == sql.ErrNoRows {
			return nil
		}
		if err != nil {
			return err
		}
		found = true
		if _, err := tx.Exec(
			`INSERT INTO message_versions (instance_jid, chat_jid, message_id, content, reason, recorded_at) VALUES (?, ?, ?, ?, 'edit', ?)`,
			instanceJID, chatJID, messageID, old.String, at,
		); err != nil {
			return err
		}
		_, err = tx.Exec(
			`UPDATE messages SET content = ?, is_edited = 1, edit_count = COALESCE(edit_count, 0) + 1, rowid = (SELECT MAX(rowid) + 1 FROM messages)
			 WHERE instance_jid = ? AND chat_jid = ? AND id = ?`,
			newContent, instanceJID, chatJID, messageID,
		)
		return err
	}, false, true)
	return found, err
}

// GetMessageVersions returns the earlier texts of a message, oldest first.
func (store *MessageStore) GetMessageVersions(instanceJID, chatJID, messageID string) ([]types.MessageVersion, error) {
	rows, err := store.db.Query(
		`SELECT id, COALESCE(content, ''), reason, recorded_at FROM message_versions
		 WHERE instance_jid = ? AND chat_jid = ? AND message_id = ? ORDER BY recorded_at ASC, id ASC`,
		instanceJID, chatJID, messageID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []types.MessageVersion
	for rows.Next() {
		var v types.MessageVersion
		if err := rows.Scan(&v.ID, &v.Content, &v.Reason, &v.RecordedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// FeedFilter narrows the unified message feed. Employee and department are resolved through the
// assignment history, so a message counts for whoever held the number when it was exchanged.
type FeedFilter struct {
	InstanceJID  string
	EmployeeID   *int
	DepartmentID *int
	ChatJID      string
	Query        string
	DeletedOnly  bool
	Since        *time.Time
	Until        *time.Time
	Before       *time.Time // pagination cursor: only messages older than this
	Limit        int
}

const maxFeedLimit = 500

// ListMessageFeed returns captured messages across instances, newest first, with the
// organizational context (number, employee, department) attached.
func (store *MessageStore) ListMessageFeed(f FeedFilter) ([]types.FeedMessage, error) {
	limit := f.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > maxFeedLimit {
		limit = maxFeedLimit
	}

	var where []string
	var args []any
	if f.InstanceJID != "" {
		where = append(where, "m.instance_jid = ?")
		args = append(args, f.InstanceJID)
	}
	if f.ChatJID != "" {
		where = append(where, "m.chat_jid = ?")
		args = append(args, f.ChatJID)
	}
	if f.EmployeeID != nil {
		where = append(where, "a.employee_id = ?")
		args = append(args, *f.EmployeeID)
	}
	if f.DepartmentID != nil {
		where = append(where, "e.department_id = ?")
		args = append(args, *f.DepartmentID)
	}
	if f.DeletedOnly {
		where = append(where, "m.is_deleted_remote = 1")
	}
	if q := strings.TrimSpace(f.Query); q != "" {
		where = append(where, `(m.content LIKE ? ESCAPE '\' OR m.sender_name LIKE ? ESCAPE '\')`)
		like := "%" + escapeLike(q) + "%"
		args = append(args, like, like)
	}
	if f.Since != nil {
		where = append(where, "datetime(m.timestamp) >= datetime(?)")
		args = append(args, *f.Since)
	}
	if f.Until != nil {
		where = append(where, "datetime(m.timestamp) < datetime(?)")
		args = append(args, *f.Until)
	}
	if f.Before != nil {
		where = append(where, "datetime(m.timestamp) < datetime(?)")
		args = append(args, *f.Before)
	}

	query := `
		SELECT m.id, m.chat_jid, COALESCE(c.name, ''), COALESCE(m.sender, ''), COALESCE(m.sender_name, ''), COALESCE(m.content, ''),
		       m.timestamp, COALESCE(m.is_from_me, 0), COALESCE(m.media_type, ''), m.instance_jid, COALESCE(i.alias, ''),
		       a.employee_id, COALESCE(e.name, ''), e.department_id, COALESCE(d.name, ''),
		       COALESCE(m.is_edited, 0), COALESCE(m.is_deleted_remote, 0), m.deleted_at, COALESCE(m.deleted_by, '')
		FROM messages m
		LEFT JOIN chats c ON c.jid = m.chat_jid
		LEFT JOIN instances i ON i.phone_jid = m.instance_jid
		LEFT JOIN instance_assignments a ON a.instance_id = i.id
			AND datetime(m.timestamp) >= datetime(a.valid_from)
			AND (a.valid_to IS NULL OR datetime(m.timestamp) < datetime(a.valid_to))
		LEFT JOIN employees e ON e.id = a.employee_id
		LEFT JOIN departments d ON d.id = e.department_id`
	if len(where) > 0 {
		query += " WHERE " + strings.Join(where, " AND ")
	}
	query += " ORDER BY m.timestamp DESC, m.rowid DESC LIMIT ?"
	args = append(args, limit)

	rows, err := store.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list message feed: %w", err)
	}
	defer rows.Close()

	out := []types.FeedMessage{}
	for rows.Next() {
		var m types.FeedMessage
		if err := rows.Scan(&m.ID, &m.ChatJID, &m.ChatName, &m.Sender, &m.SenderName, &m.Content,
			&m.Timestamp, &m.IsFromMe, &m.MediaType, &m.InstanceJID, &m.InstanceAlias,
			&m.EmployeeID, &m.EmployeeName, &m.DepartmentID, &m.DepartmentName,
			&m.IsEdited, &m.IsDeletedRemote, &m.DeletedAt, &m.DeletedBy); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// ListMessageConversations groups captured messages by (instance_jid, chat_jid) for the
// read-only chat view. Filters are applied to messages before grouping so a search can narrow
// the conversation list without changing the identity of a conversation.
func (store *MessageStore) ListMessageConversations(f FeedFilter) ([]types.MessageConversation, error) {
	limit := f.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > maxFeedLimit {
		limit = maxFeedLimit
	}

	var where []string
	var args []any
	if f.InstanceJID != "" {
		where = append(where, "m.instance_jid = ?")
		args = append(args, f.InstanceJID)
	}
	if f.EmployeeID != nil {
		where = append(where, "a.employee_id = ?")
		args = append(args, *f.EmployeeID)
	}
	if f.DepartmentID != nil {
		where = append(where, "e.department_id = ?")
		args = append(args, *f.DepartmentID)
	}
	if f.DeletedOnly {
		where = append(where, "m.is_deleted_remote = 1")
	}
	if q := strings.TrimSpace(f.Query); q != "" {
		where = append(where, `(m.content LIKE ? ESCAPE '\\' OR m.sender_name LIKE ? ESCAPE '\\')`)
		like := "%" + escapeLike(q) + "%"
		args = append(args, like, like)
	}
	if f.Since != nil {
		where = append(where, "datetime(m.timestamp) >= datetime(?)")
		args = append(args, *f.Since)
	}
	if f.Until != nil {
		where = append(where, "datetime(m.timestamp) < datetime(?)")
		args = append(args, *f.Until)
	}
	if f.Before != nil {
		where = append(where, "datetime(m.timestamp) < datetime(?)")
		args = append(args, *f.Before)
	}
	if f.ChatJID != "" {
		where = append(where, "m.chat_jid = ?")
		args = append(args, f.ChatJID)
	}

	query := `
		WITH ranked AS (
			SELECT m.instance_jid AS instance_jid, COALESCE(i.alias, '') AS instance_alias,
			       m.chat_jid AS chat_jid, COALESCE(c.name, '') AS chat_name,
			       COALESCE(m.content, '') AS last_message, COALESCE(m.sender_name, '') AS last_sender_name,
			       m.timestamp AS last_message_time, COALESCE(m.is_from_me, 0) AS last_is_from_me,
			       a.employee_id AS employee_id, COALESCE(e.name, '') AS employee_name,
			       e.department_id AS department_id, COALESCE(d.name, '') AS department_name,
			       ROW_NUMBER() OVER (
				       PARTITION BY m.instance_jid, m.chat_jid
				       ORDER BY m.timestamp DESC, m.rowid DESC
			       ) AS message_rank,
			       COUNT(*) OVER (PARTITION BY m.instance_jid, m.chat_jid) AS message_count
			FROM messages m
			LEFT JOIN chats c ON c.jid = m.chat_jid
			LEFT JOIN instances i ON i.phone_jid = m.instance_jid
			LEFT JOIN instance_assignments a ON a.instance_id = i.id
				AND datetime(m.timestamp) >= datetime(a.valid_from)
				AND (a.valid_to IS NULL OR datetime(m.timestamp) < datetime(a.valid_to))
			LEFT JOIN employees e ON e.id = a.employee_id
			LEFT JOIN departments d ON d.id = e.department_id`
	if len(where) > 0 {
		query += " WHERE " + strings.Join(where, " AND ")
	}
	query += `
		)
		SELECT instance_jid, instance_alias, chat_jid, chat_name, last_message, last_sender_name,
		       last_message_time, last_is_from_me, employee_id, employee_name, department_id,
		       department_name, message_count
		FROM ranked
		WHERE message_rank = 1
		ORDER BY last_message_time DESC
		LIMIT ?`
	args = append(args, limit)

	rows, err := store.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list message conversations: %w", err)
	}
	defer rows.Close()

	out := []types.MessageConversation{}
	for rows.Next() {
		var c types.MessageConversation
		var employeeID, departmentID sql.NullInt64
		if err := rows.Scan(&c.InstanceJID, &c.InstanceAlias, &c.ChatJID, &c.ChatName,
			&c.LastMessage, &c.LastSenderName, &c.LastMessageTime, &c.LastIsFromMe,
			&employeeID, &c.EmployeeName, &departmentID, &c.DepartmentName, &c.MessageCount); err != nil {
			return nil, err
		}
		c.IsGroup = strings.HasSuffix(c.ChatJID, "@g.us")
		if employeeID.Valid {
			id := int(employeeID.Int64)
			c.EmployeeID = &id
		}
		if departmentID.Valid {
			id := int(departmentID.Int64)
			c.DepartmentID = &id
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func escapeLike(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s)
}

// ListChatsForInstance returns the chats an instance takes part in, newest first.
func (store *MessageStore) ListChatsForInstance(instanceJID string) (map[string]time.Time, error) {
	rows, err := store.db.Query(
		`SELECT chat_jid, last_message_time FROM chat_instances WHERE instance_jid = ? ORDER BY last_message_time DESC`,
		instanceJID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string]time.Time{}
	for rows.Next() {
		var jid string
		var ts sql.NullTime
		if err := rows.Scan(&jid, &ts); err != nil {
			return nil, err
		}
		out[jid] = ts.Time
	}
	return out, rows.Err()
}

// InsertAccessLog records that message data was read.
func (store *MessageStore) InsertAccessLog(e types.AccessLogEntry) error {
	return store.enqueueWrite(func(tx *sql.Tx) error {
		_, err := tx.Exec(
			`INSERT INTO access_log (actor, client_id, action, resource, params, result_count) VALUES (?, ?, ?, ?, ?, ?)`,
			e.Actor, e.ClientID, e.Action, e.Resource, e.Params, e.ResultCount,
		)
		return err
	}, false, true)
}

// ListAccessLog returns the most recent access log entries.
func (store *MessageStore) ListAccessLog(limit int) ([]types.AccessLogEntry, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	rows, err := store.db.Query(
		`SELECT id, ts, COALESCE(actor, ''), COALESCE(client_id, ''), action, COALESCE(resource, ''), COALESCE(params, ''), result_count
		 FROM access_log ORDER BY id DESC LIMIT ?`, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []types.AccessLogEntry{}
	for rows.Next() {
		var e types.AccessLogEntry
		var n sql.NullInt64
		if err := rows.Scan(&e.ID, &e.Timestamp, &e.Actor, &e.ClientID, &e.Action, &e.Resource, &e.Params, &n); err != nil {
			return nil, err
		}
		if n.Valid {
			v := int(n.Int64)
			e.ResultCount = &v
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
