"""Fixtures for the search package tests: synthetic messages.db / whatsapp.db.

The schemas mirror the real bridge tables (see docs/database.md), but every
name, number and message here is fictional. The scenario is a small "Trabalho
2026" work group planning a trip to Sao Paulo, plus one individual chat, so
tests can exercise name resolution, media markers and chunking together.
"""

import os
import sqlite3
from dataclasses import dataclass
from datetime import UTC, datetime, timedelta

import pytest

os.environ.setdefault("WA_SKIP_DB_CHECK", "1")

GROUP_JID = "120363000000000000@g.us"
GROUP_NAME = "Trabalho 2026"

# Individual chat whose stored name is a bare phone number (the ~46% case),
# so name resolution has to fall back to a contact lookup by chat jid.
ANA_JID = "5511900000001@s.whatsapp.net"
ANA_NAME = "Ana Souza"

# Resolved directly: a whatsmeow_contacts row keyed by the @lid JID itself.
CARLA_LID_JID = "999100000000003@lid"
CARLA_NAME = "Carla Nunes"

# Resolved indirectly: whatsmeow_lid_map(lid -> pn) then whatsmeow_contacts by pn.
BRUNO_LID_JID = "999100000000002@lid"
BRUNO_PN = "5511900000002"
BRUNO_NAME = "Bruno Lima"

# The rare bare-JID sender form (no "@"), left unresolved by design.
BARE_SENDER = "5511900000004"

EPOCH = datetime(2026, 3, 2, 9, 0, 0, tzinfo=UTC)


def ts(offset_minutes: float = 0, base: datetime = EPOCH) -> str:
    """A bridge-format timestamp string, offset from a fixed base instant."""
    moment = base + timedelta(minutes=offset_minutes)
    return moment.strftime("%Y-%m-%d %H:%M:%S+00:00")


def init_messages_db(path: str) -> None:
    conn = sqlite3.connect(path)
    try:
        conn.executescript(
            """
            CREATE TABLE chats (
                jid TEXT PRIMARY KEY,
                name TEXT,
                last_message_time TIMESTAMP
            );
            CREATE TABLE messages (
                id TEXT,
                chat_jid TEXT,
                sender TEXT,
                sender_name TEXT,
                content TEXT,
                timestamp TEXT,
                is_from_me INTEGER,
                media_type TEXT,
                filename TEXT,
                file_length INTEGER,
                url TEXT,
                media_key BLOB,
                file_sha256 BLOB,
                file_enc_sha256 BLOB,
                PRIMARY KEY (id, chat_jid)
            );
            """
        )
        conn.commit()
    finally:
        conn.close()


def init_whatsapp_db(path: str) -> None:
    conn = sqlite3.connect(path)
    try:
        conn.executescript(
            """
            CREATE TABLE whatsmeow_contacts (
                our_jid TEXT,
                their_jid TEXT,
                first_name TEXT,
                full_name TEXT,
                push_name TEXT,
                business_name TEXT
            );
            CREATE TABLE whatsmeow_lid_map (
                lid TEXT PRIMARY KEY,
                pn TEXT
            );
            """
        )
        conn.commit()
    finally:
        conn.close()


def insert_chat(messages_db_path: str, jid: str, name: str | None) -> None:
    conn = sqlite3.connect(messages_db_path)
    try:
        conn.execute("INSERT OR REPLACE INTO chats (jid, name) VALUES (?, ?)", (jid, name))
        conn.commit()
    finally:
        conn.close()


def insert_message(
    messages_db_path: str,
    *,
    message_id: str,
    chat_jid: str,
    sender: str,
    content: str,
    timestamp: str,
    is_from_me: bool = False,
    media_type: str | None = None,
    filename: str | None = None,
) -> None:
    """Insert (or, if the id already exists, replace) one message.

    A replace mimics the bridge's INSERT OR REPLACE: SQLite gives the row a
    fresh, higher rowid, since the old one isn't reused.
    """
    conn = sqlite3.connect(messages_db_path)
    try:
        conn.execute("DELETE FROM messages WHERE id = ? AND chat_jid = ?", (message_id, chat_jid))
        conn.execute(
            """
            INSERT INTO messages (id, chat_jid, sender, sender_name, content, timestamp, is_from_me, media_type, filename)
            VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
            """,
            (message_id, chat_jid, sender, sender, content, timestamp, int(is_from_me), media_type, filename),
        )
        conn.commit()
    finally:
        conn.close()


def insert_contact(
    whatsapp_db_path: str,
    *,
    their_jid: str,
    first_name: str | None = None,
    full_name: str | None = None,
    push_name: str | None = None,
    business_name: str | None = None,
) -> None:
    conn = sqlite3.connect(whatsapp_db_path)
    try:
        conn.execute(
            "INSERT INTO whatsmeow_contacts (their_jid, first_name, full_name, push_name, business_name) VALUES (?, ?, ?, ?, ?)",
            (their_jid, first_name, full_name, push_name, business_name),
        )
        conn.commit()
    finally:
        conn.close()


def insert_lid_map(whatsapp_db_path: str, *, lid: str, pn: str) -> None:
    conn = sqlite3.connect(whatsapp_db_path)
    try:
        conn.execute("INSERT INTO whatsmeow_lid_map (lid, pn) VALUES (?, ?)", (lid, pn))
        conn.commit()
    finally:
        conn.close()


@dataclass
class Store:
    dir: str
    messages_db_path: str
    whatsapp_db_path: str


@pytest.fixture
def empty_store(tmp_path) -> Store:
    messages_db_path = str(tmp_path / "messages.db")
    whatsapp_db_path = str(tmp_path / "whatsapp.db")
    init_messages_db(messages_db_path)
    init_whatsapp_db(whatsapp_db_path)
    return Store(dir=str(tmp_path), messages_db_path=messages_db_path, whatsapp_db_path=whatsapp_db_path)


@pytest.fixture
def synthetic_store(empty_store: Store) -> Store:
    """The "Trabalho 2026" scenario: one group, one individual chat, mixed
    sender JID forms, and messages covering media markers, work chatter and
    noise -- used by name resolution and end-to-end indexer tests."""
    insert_chat(empty_store.messages_db_path, GROUP_JID, GROUP_NAME)
    insert_chat(empty_store.messages_db_path, ANA_JID, ANA_JID.split("@")[0])  # bare-number stored name

    insert_contact(empty_store.whatsapp_db_path, their_jid=ANA_JID, full_name=ANA_NAME)
    insert_contact(empty_store.whatsapp_db_path, their_jid=CARLA_LID_JID, push_name=CARLA_NAME)
    insert_contact(empty_store.whatsapp_db_path, their_jid=f"{BRUNO_PN}@s.whatsapp.net", full_name=BRUNO_NAME)
    insert_lid_map(empty_store.whatsapp_db_path, lid=BRUNO_LID_JID.split("@")[0], pn=BRUNO_PN)

    messages = [
        dict(message_id="MSG0001", chat_jid=GROUP_JID, sender=ANA_JID, content="bom dia pessoal", timestamp=ts(0)),
        dict(message_id="MSG0002", chat_jid=GROUP_JID, sender=BRUNO_LID_JID, content="bom dia", timestamp=ts(1)),
        dict(
            message_id="MSG0003",
            chat_jid=GROUP_JID,
            sender=ANA_JID,
            content="alguem confirma a viagem pra SP mes que vem?",
            timestamp=ts(2),
        ),
        dict(
            message_id="MSG0004",
            chat_jid=GROUP_JID,
            sender=CARLA_LID_JID,
            content="eu vou, bora marcar sampa",
            timestamp=ts(3),
        ),
        dict(message_id="MSG0005", chat_jid=GROUP_JID, sender=BARE_SENDER, content="ok", timestamp=ts(4)),
        dict(
            message_id="MSG0006",
            chat_jid=GROUP_JID,
            sender=ANA_JID,
            content="",
            timestamp=ts(5),
            media_type="image",
        ),
        dict(
            message_id="MSG0007",
            chat_jid=GROUP_JID,
            sender="owner-placeholder",
            content="fechado, reservo o hotel em Sao Paulo",
            timestamp=ts(6),
            is_from_me=True,
        ),
        dict(message_id="MSG0008", chat_jid=GROUP_JID, sender=CARLA_LID_JID, content="kkk bora", timestamp=ts(7)),
        dict(
            message_id="MSG0009",
            chat_jid=ANA_JID,
            sender=ANA_JID,
            content="tô na loja de material desde março, bem corrido",
            timestamp=ts(8),
        ),
        dict(
            message_id="MSG0010",
            chat_jid=ANA_JID,
            sender="owner-placeholder",
            content="",
            timestamp=ts(9),
            is_from_me=True,
            media_type="audio",
        ),
    ]
    for msg in messages:
        if msg["sender"] == "owner-placeholder":
            # is_from_me rows still carry the owner's own JID as sender.
            msg["sender"] = "5511900000099@s.whatsapp.net"
        insert_message(empty_store.messages_db_path, **msg)

    return empty_store
