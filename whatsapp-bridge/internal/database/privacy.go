package database

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// MediaRef identifies a stored media file so the caller can delete it from disk.
type MediaRef struct {
	ChatJID   string `json:"chat_jid"`
	MessageID string `json:"message_id"`
}

// AnonymizeResult summarizes an anonymization request.
type AnonymizeResult struct {
	Messages  int64      `json:"messages"`
	ChatJIDs  []string   `json:"chat_jids"`
	Pseudonym string     `json:"pseudonym"`
	Media     []MediaRef `json:"-"`
}

const anonymizedText = "[conteúdo removido a pedido do titular]"

// AnonymizeSubject removes the personal data of one person (LGPD art. 18) from the captured history.
//
//   - Every message in the direct chat with the person, and every message they wrote in groups,
//     loses its text, sender name and media references.
//   - The chat identifier that embeds their phone number is replaced by a pseudonym, and their
//     nickname, earlier versions of the messages and webhook delivery logs are deleted.
//   - Group chat identifiers stay: they identify the group, not the person.
//
// The action is recorded in privacy_log. The rows move to new rowids and the log entry lists the
// old chat identifiers, which is how the search indexer knows to drop what it indexed.
func (store *MessageStore) AnonymizeSubject(subject, actor string) (*AnonymizeResult, error) {
	user := strings.TrimSpace(strings.SplitN(subject, "@", 2)[0])
	if user == "" {
		return nil, fmt.Errorf("subject is required")
	}
	sum := sha256.Sum256([]byte(user))
	pseudonym := "anon-" + hex.EncodeToString(sum[:8])
	pseudoChat := pseudonym + "@anonymized"

	res := &AnonymizeResult{Pseudonym: pseudonym}
	err := store.enqueueWrite(func(tx *sql.Tx) error {
		if _, err := tx.Exec(`PRAGMA defer_foreign_keys = ON`); err != nil {
			return err
		}

		// Direct chats with the person (regular JID or hidden-user LID form).
		var directChats []string
		rows, err := tx.Query(`SELECT jid FROM chats WHERE (jid LIKE ? OR jid LIKE ?) AND jid NOT LIKE '%@g.us'`, user+"@%", user+":%@%")
		if err != nil {
			return err
		}
		for rows.Next() {
			var jid string
			if err := rows.Scan(&jid); err != nil {
				rows.Close()
				return err
			}
			directChats = append(directChats, jid)
		}
		rows.Close()

		// Media of everything about to be scrubbed, so the files can be deleted.
		mediaRows, err := tx.Query(
			`SELECT DISTINCT chat_jid, id FROM messages
			 WHERE COALESCE(media_type, '') != '' AND (sender = ? OR chat_jid IN (SELECT jid FROM chats WHERE jid LIKE ? OR jid LIKE ?))`,
			user, user+"@%", user+":%@%")
		if err != nil {
			return err
		}
		for mediaRows.Next() {
			var ref MediaRef
			if err := mediaRows.Scan(&ref.ChatJID, &ref.MessageID); err != nil {
				mediaRows.Close()
				return err
			}
			res.Media = append(res.Media, ref)
		}
		mediaRows.Close()

		var maxRowID int64
		if err := tx.QueryRow(`SELECT COALESCE(MAX(rowid), 0) FROM messages`).Scan(&maxRowID); err != nil {
			return err
		}

		r, err := tx.Exec(
			`UPDATE messages SET
				sender = ?, sender_name = '[anonimizado]', content = ?, media_type = '', filename = '', url = '', direct_path = '',
				media_key = NULL, file_sha256 = NULL, file_enc_sha256 = NULL, file_length = 0,
				quoted_sender_name = NULL, quoted_text_preview = NULL, forwarded_from = NULL,
				rowid = rowid + ?
			 WHERE sender = ? OR chat_jid IN (SELECT jid FROM chats WHERE (jid LIKE ? OR jid LIKE ?) AND jid NOT LIKE '%@g.us')`,
			pseudonym, anonymizedText, maxRowID, user, user+"@%", user+":%@%")
		if err != nil {
			return err
		}
		res.Messages, _ = r.RowsAffected()

		// Earlier versions and webhook payloads carry the same text.
		if _, err := tx.Exec(
			`DELETE FROM message_versions WHERE chat_jid IN (SELECT jid FROM chats WHERE (jid LIKE ? OR jid LIKE ?) AND jid NOT LIKE '%@g.us')`,
			user+"@%", user+":%@%"); err != nil {
			return err
		}
		if _, err := tx.Exec(`DELETE FROM contact_nicknames WHERE jid LIKE ? OR jid LIKE ?`, user+"@%", user+":%@%"); err != nil {
			return err
		}

		for _, jid := range directChats {
			res.ChatJIDs = append(res.ChatJIDs, jid)
			if _, err := tx.Exec(`DELETE FROM webhook_logs WHERE chat_jid = ?`, jid); err != nil {
				return err
			}
			if _, err := tx.Exec(`UPDATE OR IGNORE chat_instances SET chat_jid = ? WHERE chat_jid = ?`, pseudoChat, jid); err != nil {
				return err
			}
			if _, err := tx.Exec(`UPDATE OR IGNORE messages SET chat_jid = ? WHERE chat_jid = ?`, pseudoChat, jid); err != nil {
				return err
			}
			if _, err := tx.Exec(`UPDATE OR IGNORE chats SET jid = ?, name = '[anonimizado]' WHERE jid = ?`, pseudoChat, jid); err != nil {
				return err
			}
		}

		details, _ := json.Marshal(map[string]any{"chat_jids": res.ChatJIDs, "messages": res.Messages, "pseudonym": pseudonym})
		_, err = tx.Exec(`INSERT INTO privacy_log (actor, action, subject, details) VALUES (?, 'anonymize', ?, ?)`, actor, pseudonym, string(details))
		return err
	}, false, true)
	if err != nil {
		return nil, fmt.Errorf("failed to anonymize subject: %w", err)
	}
	return res, nil
}

// PurgeOlderThan deletes captured messages (and their versions and webhook logs) older than the
// cutoff: the retention policy. It records the cutoff in privacy_log so the search indexer drops
// what it indexed, and returns how many messages were removed.
func (store *MessageStore) PurgeOlderThan(cutoff time.Time, actor string) (int64, error) {
	var removed int64
	err := store.enqueueWrite(func(tx *sql.Tx) error {
		if _, err := tx.Exec(
			`DELETE FROM message_versions WHERE recorded_at < ? OR EXISTS (
				SELECT 1 FROM messages m WHERE m.instance_jid = message_versions.instance_jid
				  AND m.chat_jid = message_versions.chat_jid AND m.id = message_versions.message_id
				  AND datetime(m.timestamp) < datetime(?))`, cutoff, cutoff); err != nil {
			return err
		}
		r, err := tx.Exec(`DELETE FROM messages WHERE datetime(timestamp) < datetime(?)`, cutoff)
		if err != nil {
			return err
		}
		removed, _ = r.RowsAffected()
		if _, err := tx.Exec(`DELETE FROM webhook_logs WHERE datetime(created_at) < datetime(?)`, cutoff); err != nil {
			return err
		}
		if removed == 0 {
			return nil
		}
		details, _ := json.Marshal(map[string]any{"cutoff": cutoff.UTC().Format(time.RFC3339), "messages": removed})
		_, err = tx.Exec(`INSERT INTO privacy_log (actor, action, subject, details) VALUES (?, 'purge', ?, ?)`, actor, "retention", string(details))
		return err
	}, false, true)
	if err != nil {
		return 0, fmt.Errorf("failed to purge messages: %w", err)
	}
	return removed, nil
}

// PrivacyLogEntry is one anonymization or retention action.
type PrivacyLogEntry struct {
	ID      int       `json:"id"`
	TS      time.Time `json:"ts"`
	Actor   string    `json:"actor,omitempty"`
	Action  string    `json:"action"`
	Subject string    `json:"subject,omitempty"`
	Details string    `json:"details,omitempty"`
}

// ListPrivacyLog returns the most recent privacy actions.
func (store *MessageStore) ListPrivacyLog(limit int) ([]PrivacyLogEntry, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	rows, err := store.db.Query(
		`SELECT id, ts, COALESCE(actor, ''), action, COALESCE(subject, ''), COALESCE(details, '') FROM privacy_log ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []PrivacyLogEntry{}
	for rows.Next() {
		var e PrivacyLogEntry
		if err := rows.Scan(&e.ID, &e.TS, &e.Actor, &e.Action, &e.Subject, &e.Details); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
