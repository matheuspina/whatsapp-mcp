"""Pytest fixtures for WhatsApp MCP Server tests."""

import os

# Must be set before any test module imports lib.utils — importing lib.utils raises
# FileNotFoundError when messages.db doesn't exist (by design, to prevent silent empty
# results in production). Tests use temp DBs and don't need the real store.
os.environ.setdefault("WA_SKIP_DB_CHECK", "1")
os.environ.setdefault("MCP_ACCESS_LOG", "false")

import sqlite3
import tempfile
from datetime import datetime

import pytest

from tests.bridge_schema import create_bridge_schema


@pytest.fixture
def temp_messages_db():
    """Create a temporary messages database with test data."""
    with tempfile.NamedTemporaryFile(suffix=".db", delete=False) as f:
        db_path = f.name

    conn = sqlite3.connect(db_path)
    cursor = conn.cursor()

    # Create tables: the bridge's real schema, so tests fail when the Python code reads a column
    # the bridge does not have.
    create_bridge_schema(conn)
    cursor = conn.cursor()

    # Insert test data
    cursor.execute("INSERT INTO chats (jid, name) VALUES (?, ?)", ("123456789@s.whatsapp.net", "Test User"))
    cursor.execute("INSERT INTO chats (jid, name) VALUES (?, ?)", ("987654321@g.us", "Test Group"))

    # Insert test messages
    now = datetime.now().isoformat()
    cursor.execute(
        """INSERT INTO messages (id, chat_jid, sender, content, timestamp, is_from_me, media_type, filename, file_length, sender_name)
           VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)""",
        (
            "msg1",
            "123456789@s.whatsapp.net",
            "123456789@s.whatsapp.net",
            "Hello world",
            now,
            0,
            None,
            None,
            None,
            "Test User",
        ),
    )
    cursor.execute(
        """INSERT INTO messages (id, chat_jid, sender, content, timestamp, is_from_me, media_type, filename, file_length, sender_name)
           VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)""",
        ("msg2", "123456789@s.whatsapp.net", "me", "Hi there", now, 1, None, None, None, "Me"),
    )

    conn.commit()
    conn.close()

    yield db_path

    # Cleanup
    os.unlink(db_path)


@pytest.fixture
def temp_whatsapp_db():
    """Create a temporary WhatsApp database with test contacts."""
    with tempfile.NamedTemporaryFile(suffix=".db", delete=False) as f:
        db_path = f.name

    conn = sqlite3.connect(db_path)
    cursor = conn.cursor()

    cursor.execute("""
        CREATE TABLE whatsmeow_contacts (
            their_jid TEXT PRIMARY KEY,
            first_name TEXT,
            full_name TEXT,
            push_name TEXT,
            business_name TEXT
        )
    """)

    cursor.execute(
        """INSERT INTO whatsmeow_contacts (their_jid, first_name, full_name, push_name, business_name)
           VALUES (?, ?, ?, ?, ?)""",
        ("123456789@s.whatsapp.net", "John", "John Doe", "Johnny", None),
    )

    conn.commit()
    conn.close()

    yield db_path

    os.unlink(db_path)


@pytest.fixture
def mock_bridge_url(monkeypatch):
    """Mock the bridge URL to prevent real API calls."""
    monkeypatch.setenv("BRIDGE_HOST", "localhost:9999")
