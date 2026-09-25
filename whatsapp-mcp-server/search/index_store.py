"""Owns store/index.db: the local search index (messages + FTS5 + chunks).

This is the only module that writes to index.db. It never touches the
bridge's messages.db / whatsapp.db (see source.py for that).
"""

import json
import os
import sqlite3
from dataclasses import dataclass, field

SCHEMA_VERSION = "1"

_SCHEMA_SQL = """
CREATE TABLE IF NOT EXISTS meta (
    key TEXT PRIMARY KEY,
    value TEXT
);

CREATE TABLE IF NOT EXISTS messages_idx (
    message_id TEXT NOT NULL,
    chat_jid TEXT NOT NULL,
    ts INTEGER NOT NULL,
    sender_jid TEXT,
    sender_name TEXT,
    from_me INTEGER NOT NULL DEFAULT 0,
    text TEXT,
    chunk_id TEXT,
    PRIMARY KEY (chat_jid, message_id)
);

CREATE INDEX IF NOT EXISTS idx_messages_idx_chat_ts ON messages_idx(chat_jid, ts);
CREATE INDEX IF NOT EXISTS idx_messages_idx_chunk ON messages_idx(chunk_id);

CREATE VIRTUAL TABLE IF NOT EXISTS messages_fts USING fts5(
    text,
    sender_name,
    content='messages_idx',
    content_rowid='rowid',
    tokenize='unicode61 remove_diacritics 2'
);

CREATE TRIGGER IF NOT EXISTS messages_idx_ai AFTER INSERT ON messages_idx BEGIN
    INSERT INTO messages_fts(rowid, text, sender_name) VALUES (new.rowid, new.text, new.sender_name);
END;

CREATE TRIGGER IF NOT EXISTS messages_idx_ad AFTER DELETE ON messages_idx BEGIN
    INSERT INTO messages_fts(messages_fts, rowid, text, sender_name) VALUES ('delete', old.rowid, old.text, old.sender_name);
END;

CREATE TRIGGER IF NOT EXISTS messages_idx_au AFTER UPDATE ON messages_idx BEGIN
    INSERT INTO messages_fts(messages_fts, rowid, text, sender_name) VALUES ('delete', old.rowid, old.text, old.sender_name);
    INSERT INTO messages_fts(rowid, text, sender_name) VALUES (new.rowid, new.text, new.sender_name);
END;

CREATE TABLE IF NOT EXISTS chunks (
    chunk_id TEXT PRIMARY KEY,
    chat_jid TEXT NOT NULL,
    chat_name TEXT,
    is_group INTEGER NOT NULL DEFAULT 0,
    start_ts INTEGER NOT NULL,
    end_ts INTEGER NOT NULL,
    message_ids TEXT NOT NULL,
    senders TEXT NOT NULL,
    text TEXT NOT NULL,
    embedded INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_chunks_chat_ts ON chunks(chat_jid, start_ts, end_ts);
"""


@dataclass(frozen=True)
class IndexedMessage:
    """A resolved message row ready to be stored in messages_idx."""

    message_id: str
    chat_jid: str
    ts: int
    sender_jid: str
    sender_name: str
    from_me: bool
    text: str


@dataclass(frozen=True)
class ChunkMessage:
    """A message as read back from messages_idx, for feeding the chunker."""

    message_id: str
    ts: int
    sender_name: str
    text: str


@dataclass(frozen=True)
class ChunkRow:
    """A chunk's identity and time span, for region lookups."""

    chunk_id: str
    start_ts: int
    end_ts: int


@dataclass(frozen=True)
class Chunk:
    """A chunked conversation window, ready to be stored in the chunks table."""

    chunk_id: str
    chat_jid: str
    chat_name: str
    is_group: bool
    start_ts: int
    end_ts: int
    message_ids: list[str]
    senders: list[str]
    text: str
    # Messages this chunk "owns" for reverse lookup (excludes overlap borrowed
    # from the previous window, since those already belong to another chunk).
    primary_message_ids: list[str] = field(default_factory=list)


class IndexStore:
    """Wraps the index.db SQLite connection (WAL mode)."""

    def __init__(self, db_path: str):
        self._db_path = db_path
        parent = os.path.dirname(db_path)
        if parent:
            os.makedirs(parent, exist_ok=True)
        self._conn = sqlite3.connect(db_path)
        self._conn.execute("PRAGMA journal_mode=WAL")
        self._conn.execute("PRAGMA busy_timeout = 5000")
        self._init_schema()

    def close(self) -> None:
        self._conn.close()

    def __enter__(self) -> "IndexStore":
        return self

    def __exit__(self, *_exc: object) -> None:
        self.close()

    def _init_schema(self) -> None:
        self._conn.executescript(_SCHEMA_SQL)
        if self.get_meta("schema_version") is None:
            self.set_meta("schema_version", SCHEMA_VERSION)
        self._conn.commit()

    # -- meta -----------------------------------------------------------

    def get_meta(self, key: str) -> str | None:
        row = self._conn.execute("SELECT value FROM meta WHERE key = ?", (key,)).fetchone()
        return row[0] if row else None

    def set_meta(self, key: str, value: str) -> None:
        self._conn.execute(
            "INSERT INTO meta (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value",
            (key, value),
        )
        self._conn.commit()

    def get_cursor(self) -> int:
        value = self.get_meta("source_cursor")
        return int(value) if value is not None else 0

    def set_cursor(self, rowid: int) -> None:
        self.set_meta("source_cursor", str(rowid))

    # -- messages ---------------------------------------------------------

    def upsert_messages(self, rows: list[IndexedMessage]) -> None:
        """Insert or update messages_idx rows, keyed on (chat_jid, message_id).

        Uses UPSERT (not INSERT OR REPLACE) so a replayed/replaced message
        keeps its rowid and updates the FTS index via the AFTER UPDATE
        trigger, rather than churning through a delete+insert.
        """
        if not rows:
            return
        self._conn.executemany(
            """
            INSERT INTO messages_idx (message_id, chat_jid, ts, sender_jid, sender_name, from_me, text)
            VALUES (?, ?, ?, ?, ?, ?, ?)
            ON CONFLICT(chat_jid, message_id) DO UPDATE SET
                ts = excluded.ts,
                sender_jid = excluded.sender_jid,
                sender_name = excluded.sender_name,
                from_me = excluded.from_me,
                text = excluded.text
            """,
            [(r.message_id, r.chat_jid, r.ts, r.sender_jid, r.sender_name, int(r.from_me), r.text) for r in rows],
        )
        self._conn.commit()

    def get_messages_in_range(self, chat_jid: str, start_ts: int, end_ts: int) -> list[ChunkMessage]:
        rows = self._conn.execute(
            """
            SELECT message_id, ts, sender_name, text
            FROM messages_idx
            WHERE chat_jid = ? AND ts BETWEEN ? AND ?
            ORDER BY ts ASC, message_id ASC
            """,
            (chat_jid, start_ts, end_ts),
        ).fetchall()
        return [ChunkMessage(message_id=r[0], ts=r[1], sender_name=r[2] or "", text=r[3] or "") for r in rows]

    # -- chunks -----------------------------------------------------------

    def get_chunks_touching(self, chat_jid: str, start_ts: int, end_ts: int) -> list[ChunkRow]:
        rows = self._conn.execute(
            "SELECT chunk_id, start_ts, end_ts FROM chunks WHERE chat_jid = ? AND start_ts <= ? AND end_ts >= ?",
            (chat_jid, end_ts, start_ts),
        ).fetchall()
        return [ChunkRow(chunk_id=r[0], start_ts=r[1], end_ts=r[2]) for r in rows]

    def replace_chunks(self, chat_jid: str, old_chunk_ids: list[str], new_chunks: list[Chunk]) -> None:
        """Delete stale chunks and (re)insert the freshly computed ones.

        New chunks always land with embedded=0: any chunk touched by a
        rebuild needs re-embedding (Phase 2), even one that keeps its
        chunk_id and only grew.
        """
        stale = set(old_chunk_ids) - {c.chunk_id for c in new_chunks}
        if stale:
            placeholders = ",".join("?" * len(stale))
            self._conn.execute(f"DELETE FROM chunks WHERE chunk_id IN ({placeholders})", tuple(stale))

        for chunk in new_chunks:
            self._conn.execute(
                """
                INSERT INTO chunks (chunk_id, chat_jid, chat_name, is_group, start_ts, end_ts, message_ids, senders, text, embedded)
                VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 0)
                ON CONFLICT(chunk_id) DO UPDATE SET
                    chat_name = excluded.chat_name,
                    is_group = excluded.is_group,
                    start_ts = excluded.start_ts,
                    end_ts = excluded.end_ts,
                    message_ids = excluded.message_ids,
                    senders = excluded.senders,
                    text = excluded.text,
                    embedded = 0
                """,
                (
                    chunk.chunk_id,
                    chat_jid,
                    chunk.chat_name,
                    int(chunk.is_group),
                    chunk.start_ts,
                    chunk.end_ts,
                    json.dumps(chunk.message_ids, ensure_ascii=False),
                    json.dumps(chunk.senders, ensure_ascii=False),
                    chunk.text,
                ),
            )
            if chunk.primary_message_ids:
                placeholders = ",".join("?" * len(chunk.primary_message_ids))
                self._conn.execute(
                    f"UPDATE messages_idx SET chunk_id = ? WHERE chat_jid = ? AND message_id IN ({placeholders})",
                    (chunk.chunk_id, chat_jid, *chunk.primary_message_ids),
                )
        self._conn.commit()

    # -- search -------------------------------------------------------------

    def search_fts(self, query: str, limit: int = 20) -> list[tuple[str, str, str]]:
        """Keyword search over indexed message text. Returns (chat_jid, message_id, text)."""
        rows = self._conn.execute(
            """
            SELECT m.chat_jid, m.message_id, m.text
            FROM messages_fts f
            JOIN messages_idx m ON m.rowid = f.rowid
            WHERE messages_fts MATCH ?
            ORDER BY f.rank
            LIMIT ?
            """,
            (query, limit),
        ).fetchall()
        return [(r[0], r[1], r[2]) for r in rows]
