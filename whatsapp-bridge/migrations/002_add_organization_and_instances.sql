-- Migration 002: Add Organization Entities (Departments, Employees) and Instances
-- Target: store/messages.db
-- Date: 2026-09-26
-- Backward compatible: YES (append-only)

-- ============================================================================
-- PART 1: ORGANIZATIONAL TABLES
-- ============================================================================

CREATE TABLE IF NOT EXISTS departments (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    description TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS employees (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    department_id INTEGER,
    name TEXT NOT NULL,
    role TEXT,
    email TEXT,
    active BOOLEAN DEFAULT 1,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (department_id) REFERENCES departments(id) ON DELETE SET NULL
);

CREATE TABLE IF NOT EXISTS instances (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    phone_jid TEXT UNIQUE NOT NULL,
    employee_id INTEGER,
    alias TEXT,
    status TEXT DEFAULT 'disconnected',
    paired_at TIMESTAMP,
    last_seen_at TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (employee_id) REFERENCES employees(id) ON DELETE SET NULL
);

-- ============================================================================
-- PART 2: NEW COLUMNS FOR MESSAGES AND CHATS
-- ============================================================================

ALTER TABLE messages ADD COLUMN instance_jid TEXT;
ALTER TABLE messages ADD COLUMN is_deleted_remote BOOLEAN DEFAULT 0;
ALTER TABLE chats ADD COLUMN instance_jid TEXT;

-- ============================================================================
-- PART 3: PERFORMANCE INDEXES
-- ============================================================================

CREATE INDEX IF NOT EXISTS idx_messages_instance_chat
    ON messages(instance_jid, chat_jid, timestamp DESC);

CREATE INDEX IF NOT EXISTS idx_messages_is_deleted
    ON messages(is_deleted_remote);

CREATE INDEX IF NOT EXISTS idx_employees_department
    ON employees(department_id);

CREATE INDEX IF NOT EXISTS idx_instances_employee
    ON instances(employee_id);
