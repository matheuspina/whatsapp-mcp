-- Migration 004: Governance (per-instance chat index, assignment history, audit trail, privacy)
-- Target: store/messages.db
-- Requires: 002 and 003 (instance-scoped message keys)
-- Backward compatible: YES (only adds tables, columns, indexes and views)

-- ============================================================================
-- PART 1: WHICH INSTANCE SAW WHICH CHAT
-- ============================================================================
-- chats stays one row per chat JID (a shared directory of names). chat_instances records
-- which instances participate in each chat, so listings can be scoped per instance.

CREATE TABLE IF NOT EXISTS chat_instances (
    instance_jid TEXT NOT NULL,
    chat_jid TEXT NOT NULL,
    last_message_time TIMESTAMP,
    PRIMARY KEY (instance_jid, chat_jid)
);

CREATE INDEX IF NOT EXISTS idx_chat_instances_chat ON chat_instances(chat_jid);

INSERT OR IGNORE INTO chat_instances (instance_jid, chat_jid, last_message_time)
    SELECT instance_jid, chat_jid, MAX(timestamp) FROM messages GROUP BY instance_jid, chat_jid;

-- ============================================================================
-- PART 2: WHO OPERATED EACH NUMBER, AND WHEN
-- ============================================================================
-- instances.employee_id is the current owner. This table keeps the history so messages
-- stay attributed to whoever held the number when they were exchanged.

CREATE TABLE IF NOT EXISTS instance_assignments (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    instance_id INTEGER NOT NULL,
    employee_id INTEGER,
    valid_from TIMESTAMP NOT NULL,
    valid_to TIMESTAMP,
    FOREIGN KEY (instance_id) REFERENCES instances(id),
    FOREIGN KEY (employee_id) REFERENCES employees(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_assignments_instance ON instance_assignments(instance_id, valid_from);
CREATE INDEX IF NOT EXISTS idx_assignments_employee ON instance_assignments(employee_id);

INSERT INTO instance_assignments (instance_id, employee_id, valid_from)
    SELECT id, employee_id, '1970-01-01 00:00:00+00:00' FROM instances
    WHERE employee_id IS NOT NULL
      AND NOT EXISTS (SELECT 1 FROM instance_assignments a WHERE a.instance_id = instances.id);

-- ============================================================================
-- PART 3: INSTANCE GOVERNANCE FLAGS
-- ============================================================================

ALTER TABLE instances ADD COLUMN allow_send INTEGER NOT NULL DEFAULT 0;
ALTER TABLE instances ADD COLUMN corporate_asset_confirmed INTEGER NOT NULL DEFAULT 0;
ALTER TABLE instances ADD COLUMN corporate_terms_version TEXT;
ALTER TABLE instances ADD COLUMN corporate_confirmed_by TEXT;
ALTER TABLE instances ADD COLUMN corporate_confirmed_at TIMESTAMP;

-- Instances that already exist keep the sending behaviour they had before this migration.
UPDATE instances SET allow_send = 1;

-- ============================================================================
-- PART 4: AUDIT TRAIL
-- ============================================================================
-- Previous text of edited or revoked messages. The messages row always carries the latest
-- text, so nothing that was said is lost.

CREATE TABLE IF NOT EXISTS message_versions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    instance_jid TEXT NOT NULL,
    chat_jid TEXT NOT NULL,
    message_id TEXT NOT NULL,
    content TEXT,
    reason TEXT NOT NULL,
    recorded_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_message_versions_msg ON message_versions(instance_jid, chat_jid, message_id);

-- Who queried message data through the MCP server or the panel.
CREATE TABLE IF NOT EXISTS access_log (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    ts TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    actor TEXT,
    client_id TEXT,
    action TEXT NOT NULL,
    resource TEXT,
    params TEXT,
    result_count INTEGER
);

CREATE INDEX IF NOT EXISTS idx_access_log_ts ON access_log(ts DESC);

-- ============================================================================
-- PART 5: PRIVACY (LGPD) LOG
-- ============================================================================

CREATE TABLE IF NOT EXISTS privacy_log (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    ts TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    actor TEXT,
    action TEXT NOT NULL,
    subject TEXT,
    details TEXT
);

-- ============================================================================
-- PART 6: DE-DUPLICATED VIEW
-- ============================================================================
-- The same message can be stored once per instance that saw it (a group with two
-- monitored numbers). Readers that want a single copy use this view.

CREATE VIEW IF NOT EXISTS messages_unique AS
    SELECT m.* FROM messages m
    WHERE NOT EXISTS (
        SELECT 1 FROM messages o
        WHERE o.chat_jid = m.chat_jid AND o.id = m.id AND o.rowid < m.rowid
    );
