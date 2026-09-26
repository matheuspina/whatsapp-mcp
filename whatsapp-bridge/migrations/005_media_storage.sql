-- Migration 005: media storage (object storage upload queue and registry)
-- Date: 2026-09-26
-- Description: registers every downloaded media file, tracks its upload to an S3-compatible
--              bucket (Cloudflare R2, MinIO, AWS S3, ...), and adds the small key/value table
--              the panel uses for settings edited at runtime.
-- Safety: append-only. Only new tables and indexes; nothing existing is touched.

-- ============================================================================
-- PART 1: MEDIA REGISTRY AND UPLOAD QUEUE
-- ============================================================================
-- One row per message that carries media. The row is also the upload queue: a worker picks the
-- rows whose status is 'pending_upload' and whose next_attempt_at is due, so the queue survives a
-- restart and never holds media in memory.
--
--   local           downloaded, object storage is not configured (or the file is not queued yet)
--   pending_upload  downloaded to local_path, waiting for a worker
--   uploading       a worker has claimed it
--   uploaded        stored at (bucket, object_key); local_path is cleared unless copies are kept
--   failed          gave up after too many attempts (or the download itself failed)

CREATE TABLE IF NOT EXISTS message_media (
    instance_jid TEXT NOT NULL,
    chat_jid TEXT NOT NULL,
    message_id TEXT NOT NULL,
    media_type TEXT NOT NULL,
    filename TEXT,
    content_type TEXT,
    message_time TIMESTAMP,
    size INTEGER DEFAULT 0,
    status TEXT NOT NULL,
    local_path TEXT,
    bucket TEXT,
    object_key TEXT,
    attempts INTEGER NOT NULL DEFAULT 0,
    next_attempt_at TIMESTAMP,
    last_error TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    uploaded_at TIMESTAMP,
    PRIMARY KEY (instance_jid, chat_jid, message_id)
);

CREATE INDEX IF NOT EXISTS idx_message_media_queue ON message_media(status, next_attempt_at);
CREATE INDEX IF NOT EXISTS idx_message_media_message ON message_media(chat_jid, message_id);

-- ============================================================================
-- PART 2: FROZEN FOLDER PER CHAT
-- ============================================================================
-- The folder a chat's media goes to (department/employee/number/chat) is decided once and
-- reused. Renaming a person or a contact later does not split a conversation across folders.

CREATE TABLE IF NOT EXISTS media_folders (
    instance_jid TEXT NOT NULL,
    chat_jid TEXT NOT NULL,
    prefix TEXT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (instance_jid, chat_jid)
);

-- ============================================================================
-- PART 3: SETTINGS EDITED FROM THE PANEL
-- ============================================================================

CREATE TABLE IF NOT EXISTS app_settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
