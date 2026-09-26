package database

import (
	"database/sql"
	"fmt"
	"time"

	"whatsapp-bridge/internal/auth"
)

// SaveSession persists an authenticated web UI session into messages.db
func (s *MessageStore) SaveSession(sess *auth.Session) error {
	query := `
		INSERT INTO web_sessions (token, id, username, created_at, last_seen_at, expires_at, ip, user_agent)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(token) DO UPDATE SET
			last_seen_at = excluded.last_seen_at,
			expires_at = excluded.expires_at,
			ip = excluded.ip,
			user_agent = excluded.user_agent
	`
	_, err := s.db.Exec(query,
		sess.Token, sess.ID, sess.Username,
		sess.CreatedAt, sess.LastSeenAt, sess.ExpiresAt,
		sess.IP, sess.UserAgent,
	)
	if err != nil {
		return fmt.Errorf("failed to save session: %w", err)
	}
	return nil
}

// UpdateSession updates last seen and expiry timestamp for a session
func (s *MessageStore) UpdateSession(token string, lastSeen, expiresAt time.Time) error {
	query := `UPDATE web_sessions SET last_seen_at = ?, expires_at = ? WHERE token = ?`
	_, err := s.db.Exec(query, lastSeen, expiresAt, token)
	if err != nil {
		return fmt.Errorf("failed to update session: %w", err)
	}
	return nil
}

// DeleteSession removes a session by its token
func (s *MessageStore) DeleteSession(token string) error {
	_, err := s.db.Exec(`DELETE FROM web_sessions WHERE token = ?`, token)
	if err != nil {
		return fmt.Errorf("failed to delete session: %w", err)
	}
	return nil
}

// DeleteSessionByID removes a session by its public ID
func (s *MessageStore) DeleteSessionByID(id string) error {
	_, err := s.db.Exec(`DELETE FROM web_sessions WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("failed to delete session by id: %w", err)
	}
	return nil
}

// LoadSessions loads all non-expired sessions from the database
func (s *MessageStore) LoadSessions(now time.Time) ([]*auth.Session, error) {
	rows, err := s.db.Query(`
		SELECT token, id, username, created_at, last_seen_at, expires_at, ip, user_agent
		FROM web_sessions
		WHERE expires_at > ?
	`, now)
	if err != nil {
		return nil, fmt.Errorf("failed to load sessions: %w", err)
	}
	defer rows.Close()

	var sessions []*auth.Session
	for rows.Next() {
		var sess auth.Session
		var ip, userAgent sql.NullString
		if err := rows.Scan(
			&sess.Token, &sess.ID, &sess.Username,
			&sess.CreatedAt, &sess.LastSeenAt, &sess.ExpiresAt,
			&ip, &userAgent,
		); err != nil {
			return nil, fmt.Errorf("failed to scan session: %w", err)
		}
		if ip.Valid {
			sess.IP = ip.String
		}
		if userAgent.Valid {
			sess.UserAgent = userAgent.String
		}
		sessions = append(sessions, &sess)
	}
	return sessions, rows.Err()
}

// PurgeExpired drops expired sessions from the database
func (s *MessageStore) PurgeExpired(now time.Time) error {
	_, err := s.db.Exec(`DELETE FROM web_sessions WHERE expires_at <= ?`, now)
	if err != nil {
		return fmt.Errorf("failed to purge expired sessions: %w", err)
	}
	return nil
}
