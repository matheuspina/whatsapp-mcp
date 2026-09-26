package database

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Lifecycle of a media file, as stored in message_media.status.
const (
	// MediaLocal: downloaded, but object storage is off or the file is not queued yet.
	MediaLocal = "local"
	// MediaPendingUpload: waiting for an upload worker (also the state of a retry).
	MediaPendingUpload = "pending_upload"
	// MediaUploading: claimed by a worker.
	MediaUploading = "uploading"
	// MediaUploaded: stored in the bucket at ObjectKey.
	MediaUploaded = "uploaded"
	// MediaFailed: gave up; LastError says why.
	MediaFailed = "failed"
)

// MediaRecord is a row of message_media: where the file of one message lives.
type MediaRecord struct {
	InstanceJID string     `json:"instance_jid"`
	ChatJID     string     `json:"chat_jid"`
	MessageID   string     `json:"message_id"`
	MediaType   string     `json:"media_type"`
	Filename    string     `json:"filename,omitempty"`
	ContentType string     `json:"content_type,omitempty"`
	MessageTime time.Time  `json:"message_time"`
	Size        int64      `json:"size"`
	Status      string     `json:"status"`
	LocalPath   string     `json:"local_path,omitempty"`
	Bucket      string     `json:"bucket,omitempty"`
	ObjectKey   string     `json:"object_key,omitempty"`
	Attempts    int        `json:"attempts"`
	LastError   string     `json:"last_error,omitempty"`
	UploadedAt  *time.Time `json:"uploaded_at,omitempty"`
}

// mediaColumns is the column list every MediaRecord query selects, in scanMedia order.
const mediaColumns = `instance_jid, chat_jid, message_id, media_type, COALESCE(filename, ''), COALESCE(content_type, ''),
	message_time, COALESCE(size, 0), status, COALESCE(local_path, ''), COALESCE(bucket, ''), COALESCE(object_key, ''),
	attempts, COALESCE(last_error, ''), uploaded_at`

func scanMedia(row rowScanner) (*MediaRecord, error) {
	var (
		rec         MediaRecord
		messageTime sql.NullTime
	)
	if err := row.Scan(&rec.InstanceJID, &rec.ChatJID, &rec.MessageID, &rec.MediaType, &rec.Filename, &rec.ContentType,
		&messageTime, &rec.Size, &rec.Status, &rec.LocalPath, &rec.Bucket, &rec.ObjectKey,
		&rec.Attempts, &rec.LastError, &rec.UploadedAt); err != nil {
		return nil, err
	}
	if messageTime.Valid {
		rec.MessageTime = messageTime.Time
	}
	return &rec, nil
}

// RecordMedia registers a downloaded file. A file already stored in the bucket, or being
// uploaded right now, is left alone, so a message that history sync delivers a second time
// cannot send it back through the queue.
// It goes through the low-priority write queue: media bookkeeping never delays a new message.
func (store *MessageStore) RecordMedia(rec MediaRecord) error {
	return store.enqueueWrite(func(tx *sql.Tx) error {
		_, err := tx.Exec(
			`INSERT INTO message_media
			 (instance_jid, chat_jid, message_id, media_type, filename, content_type, message_time, size, status, local_path, next_attempt_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL)
			 ON CONFLICT(instance_jid, chat_jid, message_id) DO UPDATE SET
			     media_type = excluded.media_type,
			     filename = excluded.filename,
			     content_type = excluded.content_type,
			     message_time = excluded.message_time,
			     size = excluded.size,
			     status = excluded.status,
			     local_path = excluded.local_path,
			     attempts = 0,
			     last_error = NULL,
			     next_attempt_at = NULL,
			     updated_at = CURRENT_TIMESTAMP
			 WHERE message_media.status NOT IN ('uploaded', 'uploading')`,
			rec.InstanceJID, rec.ChatJID, rec.MessageID, rec.MediaType, rec.Filename, rec.ContentType,
			rec.MessageTime, rec.Size, rec.Status, rec.LocalPath,
		)
		return err
	}, true, true)
}

// RecordMediaFailure notes that a message's media could not be downloaded, so the panel and
// the retry tooling can see it instead of it vanishing into a log line.
func (store *MessageStore) RecordMediaFailure(rec MediaRecord, reason string) error {
	return store.enqueueWrite(func(tx *sql.Tx) error {
		_, err := tx.Exec(
			`INSERT INTO message_media
			 (instance_jid, chat_jid, message_id, media_type, filename, content_type, message_time, status, last_error)
			 VALUES (?, ?, ?, ?, ?, ?, ?, 'failed', ?)
			 ON CONFLICT(instance_jid, chat_jid, message_id) DO UPDATE SET
			     last_error = excluded.last_error,
			     updated_at = CURRENT_TIMESTAMP
			 WHERE message_media.status = 'failed'`,
			rec.InstanceJID, rec.ChatJID, rec.MessageID, rec.MediaType, rec.Filename, rec.ContentType, rec.MessageTime, reason,
		)
		return err
	}, true, true)
}

// MediaRegistered reports whether a message's media is already tracked and worth keeping: a
// failed download does not count, so it is tried again the next time the message arrives.
func (store *MessageStore) MediaRegistered(instanceJID, chatJID, messageID string) bool {
	var n int
	err := store.db.QueryRow(
		`SELECT COUNT(*) FROM message_media WHERE instance_jid = ? AND chat_jid = ? AND message_id = ? AND status != 'failed'`,
		instanceJID, chatJID, messageID).Scan(&n)
	return err == nil && n > 0
}

// GetMediaRecord returns the media registered for one message of one number.
func (store *MessageStore) GetMediaRecord(instanceJID, chatJID, messageID string) (*MediaRecord, error) {
	rec, err := scanMedia(store.db.QueryRow(
		`SELECT `+mediaColumns+` FROM message_media WHERE instance_jid = ? AND chat_jid = ? AND message_id = ?`,
		instanceJID, chatJID, messageID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return rec, err
}

// FindMediaRecord returns the media registered for a message when the caller does not know which
// number captured it. If several numbers hold the same message, the one already uploaded wins.
func (store *MessageStore) FindMediaRecord(chatJID, messageID string) (*MediaRecord, error) {
	rec, err := scanMedia(store.db.QueryRow(
		`SELECT `+mediaColumns+` FROM message_media WHERE chat_jid = ? AND message_id = ?
		 ORDER BY (status = 'uploaded') DESC, updated_at DESC LIMIT 1`,
		chatJID, messageID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return rec, err
}

// ClaimPendingUploads takes up to limit files that are due for upload and marks them
// 'uploading'. It has a single caller (the upload dispatcher), so the read-then-mark pair
// cannot hand the same file to two workers.
func (store *MessageStore) ClaimPendingUploads(limit int) ([]*MediaRecord, error) {
	rows, err := store.db.Query(
		`SELECT `+mediaColumns+` FROM message_media
		 WHERE status = 'pending_upload' AND (next_attempt_at IS NULL OR datetime(next_attempt_at) <= datetime(?))
		 ORDER BY created_at LIMIT ?`,
		time.Now().UTC(), limit)
	if err != nil {
		return nil, fmt.Errorf("list pending uploads: %w", err)
	}
	var claimed []*MediaRecord
	for rows.Next() {
		rec, err := scanMedia(rows)
		if err != nil {
			_ = rows.Close()
			return nil, err
		}
		claimed = append(claimed, rec)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	_ = rows.Close()
	if len(claimed) == 0 {
		return nil, nil
	}

	err = store.enqueueWrite(func(tx *sql.Tx) error {
		for _, rec := range claimed {
			if _, err := tx.Exec(
				`UPDATE message_media SET status = 'uploading', updated_at = CURRENT_TIMESTAMP
				 WHERE instance_jid = ? AND chat_jid = ? AND message_id = ? AND status = 'pending_upload'`,
				rec.InstanceJID, rec.ChatJID, rec.MessageID); err != nil {
				return err
			}
		}
		return nil
	}, true, true)
	if err != nil {
		return nil, fmt.Errorf("claim uploads: %w", err)
	}
	return claimed, nil
}

// MarkMediaUploaded records that the file is in the bucket. With keepLocal false the local path
// is cleared, so readers resolve the file through the bucket from now on.
func (store *MessageStore) MarkMediaUploaded(rec *MediaRecord, bucket, objectKey string, keepLocal bool) error {
	return store.enqueueWrite(func(tx *sql.Tx) error {
		localPath := rec.LocalPath
		if !keepLocal {
			localPath = ""
		}
		_, err := tx.Exec(
			`UPDATE message_media
			 SET status = 'uploaded', bucket = ?, object_key = ?, local_path = NULLIF(?, ''), last_error = NULL,
			     uploaded_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
			 WHERE instance_jid = ? AND chat_jid = ? AND message_id = ?`,
			bucket, objectKey, localPath, rec.InstanceJID, rec.ChatJID, rec.MessageID)
		return err
	}, true, true)
}

// MarkMediaUploadFailed records a failed attempt. With retryAt set the file goes back to the
// queue and is retried then; without it the file is marked failed for good.
func (store *MessageStore) MarkMediaUploadFailed(rec *MediaRecord, reason string, attempts int, retryAt *time.Time) error {
	return store.enqueueWrite(func(tx *sql.Tx) error {
		status := MediaFailed
		var next any
		if retryAt != nil {
			status = MediaPendingUpload
			next = retryAt.UTC()
		}
		_, err := tx.Exec(
			`UPDATE message_media
			 SET status = ?, attempts = ?, last_error = ?, next_attempt_at = ?, updated_at = CURRENT_TIMESTAMP
			 WHERE instance_jid = ? AND chat_jid = ? AND message_id = ?`,
			status, attempts, reason, next, rec.InstanceJID, rec.ChatJID, rec.MessageID)
		return err
	}, true, true)
}

// ResetStuckUploads puts files a crashed process left 'uploading' back in the queue.
func (store *MessageStore) ResetStuckUploads() (int64, error) {
	var n int64
	err := store.enqueueWrite(func(tx *sql.Tx) error {
		res, err := tx.Exec(`UPDATE message_media SET status = 'pending_upload', updated_at = CURRENT_TIMESTAMP WHERE status = 'uploading'`)
		if err != nil {
			return err
		}
		n, err = res.RowsAffected()
		return err
	}, true, true)
	return n, err
}

// QueueLocalMedia moves up to limit locally stored files to the upload queue. It is how files
// downloaded while object storage was off reach the bucket once it is configured, and how the
// operator retries the ones that failed (includeFailed).
func (store *MessageStore) QueueLocalMedia(limit int, includeFailed bool) (int64, error) {
	statuses := `'local'`
	if includeFailed {
		statuses = `'local', 'failed'`
	}
	var n int64
	err := store.enqueueWrite(func(tx *sql.Tx) error {
		res, err := tx.Exec(
			`UPDATE message_media
			 SET status = 'pending_upload', attempts = 0, last_error = NULL, next_attempt_at = NULL, updated_at = CURRENT_TIMESTAMP
			 WHERE rowid IN (
			     SELECT rowid FROM message_media
			     WHERE status IN (`+statuses+`) AND COALESCE(local_path, '') != ''
			     ORDER BY created_at LIMIT ?)`, limit)
		if err != nil {
			return err
		}
		n, err = res.RowsAffected()
		return err
	}, true, true)
	return n, err
}

// MediaQueueStats counts media rows by status.
func (store *MessageStore) MediaQueueStats() (map[string]int, error) {
	rows, err := store.db.Query(`SELECT status, COUNT(*) FROM message_media GROUP BY status`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	stats := map[string]int{}
	for rows.Next() {
		var status string
		var n int
		if err := rows.Scan(&status, &n); err != nil {
			return nil, err
		}
		stats[status] = n
	}
	return stats, rows.Err()
}

// GetSetting returns a value saved with SetSetting, or ErrNotFound.
func (store *MessageStore) GetSetting(key string) (string, error) {
	var value string
	err := store.db.QueryRow(`SELECT value FROM app_settings WHERE key = ?`, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return value, err
}

// SetSetting saves a setting edited from the panel.
func (store *MessageStore) SetSetting(key, value string) error {
	return store.enqueueWrite(func(tx *sql.Tx) error {
		_, err := tx.Exec(
			`INSERT INTO app_settings (key, value) VALUES (?, ?)
			 ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = CURRENT_TIMESTAMP`, key, value)
		return err
	}, false, true)
}
