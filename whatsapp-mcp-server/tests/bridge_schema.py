"""The bridge's real database schema, for tests that read what the bridge writes.

The migrations under whatsapp-bridge/migrations are applied as they are, so a column the Python code
reads that the bridge does not create fails here instead of returning an empty list in production.
"""

import sqlite3
from pathlib import Path

MIGRATIONS = Path(__file__).resolve().parents[2] / "whatsapp-bridge" / "migrations"

# messages after the bridge's instance-scoped-keys rebuild (migration 003, written in Go).
MESSAGES_DDL = """
CREATE TABLE messages (
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
)
"""


def split_statements(script: str) -> list[str]:
    """Same splitting the bridge does: drop comment lines, split on semicolons."""
    kept = [line for line in script.splitlines() if not line.strip().startswith("--")]
    return [s.strip() for s in "\n".join(kept).split(";") if s.strip()]


def apply_migration(conn: sqlite3.Connection, name: str) -> None:
    for statement in split_statements((MIGRATIONS / name).read_text()):
        try:
            conn.execute(statement)
        except sqlite3.OperationalError as exc:
            if "duplicate column name" not in str(exc):
                raise


def create_bridge_schema(conn: sqlite3.Connection) -> None:
    """Chats, messages, nicknames and every organization/governance table, as the bridge creates them."""
    conn.executescript(
        """
        CREATE TABLE chats (jid TEXT PRIMARY KEY, name TEXT, last_message_time TIMESTAMP);
        CREATE TABLE contact_nicknames (
            jid TEXT PRIMARY KEY, nickname TEXT NOT NULL,
            created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP, updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
        );
        """
    )
    conn.execute(MESSAGES_DDL)
    apply_migration(conn, "002_add_organization_and_instances.sql")
    apply_migration(conn, "004_governance.sql")
    conn.commit()
