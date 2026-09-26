"""Owns store/index.db: the local search index (messages + FTS5 + chunks).

This is the only module that writes to index.db. It never touches the
bridge's messages.db / whatsapp.db (see source.py for that).
"""

import json
import os
import sqlite3
from dataclasses import dataclass, field

from lib.utils import logger

SCHEMA_VERSION = "1"
VEC_TABLE = "vec_chunks"

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
    instance_jid TEXT,
    is_deleted_remote INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (chat_jid, message_id)
);

CREATE INDEX IF NOT EXISTS idx_messages_idx_chat_ts ON messages_idx(chat_jid, ts);
CREATE INDEX IF NOT EXISTS idx_messages_idx_chunk ON messages_idx(chunk_id);
CREATE INDEX IF NOT EXISTS idx_messages_idx_sender ON messages_idx(sender_jid, sender_name);

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


def load_vec_extension(conn: sqlite3.Connection) -> bool:
    """Load sqlite-vec into ``conn``. Returns False (and logs why) when it cannot be loaded."""
    try:
        import sqlite_vec

        conn.enable_load_extension(True)
        try:
            sqlite_vec.load(conn)
        finally:
            conn.enable_load_extension(False)
        return True
    except (ImportError, AttributeError, sqlite3.Error) as exc:
        logger.warning("search: sqlite-vec unavailable, semantic search is off (%s)", exc)
        return False


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
    instance_jid: str | None = None
    is_deleted_remote: bool = False


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
class PendingChunk:
    """A chunk waiting for its embedding. ``rowid`` is the key used in vec_chunks."""

    rowid: int
    chunk_id: str
    chat_jid: str
    start_ts: int
    text: str


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
        self.vec_available = load_vec_extension(self._conn)
        self._init_schema()

    def close(self) -> None:
        self._conn.close()

    def __enter__(self) -> "IndexStore":
        return self

    def __exit__(self, *_exc: object) -> None:
        self.close()

    def _init_schema(self) -> None:
        self._conn.executescript(_SCHEMA_SQL)
        for col in ("instance_jid TEXT", "is_deleted_remote INTEGER DEFAULT 0"):
            try:
                self._conn.execute(f"ALTER TABLE messages_idx ADD COLUMN {col}")
            except sqlite3.OperationalError:
                pass
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
            INSERT INTO messages_idx (message_id, chat_jid, ts, sender_jid, sender_name, from_me, text, instance_jid, is_deleted_remote)
            VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
            ON CONFLICT(chat_jid, message_id) DO UPDATE SET
                ts = excluded.ts,
                sender_jid = excluded.sender_jid,
                sender_name = excluded.sender_name,
                from_me = excluded.from_me,
                text = excluded.text,
                instance_jid = excluded.instance_jid,
                is_deleted_remote = excluded.is_deleted_remote
            """,
            [
                (
                    r.message_id,
                    r.chat_jid,
                    r.ts,
                    r.sender_jid,
                    r.sender_name,
                    int(r.from_me),
                    r.text,
                    r.instance_jid,
                    int(r.is_deleted_remote),
                )
                for r in rows
            ],
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
        # Vectors are keyed by the chunk's rowid, so they must go before the row does
        # (a freed rowid can be handed to a different chunk) and before a rebuilt chunk
        # is re-embedded (its old vector describes text that no longer exists).
        self._delete_vectors_for(set(old_chunk_ids) | {c.chunk_id for c in new_chunks})
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

    # -- vectors ------------------------------------------------------------

    def has_vec_table(self) -> bool:
        row = self._conn.execute("SELECT 1 FROM sqlite_master WHERE name = ?", (VEC_TABLE,)).fetchone()
        return row is not None

    def _delete_vectors_for(self, chunk_ids: set[str]) -> None:
        if not chunk_ids or not self.vec_available or not self.has_vec_table():
            return
        ids = list(chunk_ids)
        for start in range(0, len(ids), 500):
            batch = ids[start : start + 500]
            placeholders = ",".join("?" * len(batch))
            rowids = [
                r[0] for r in self._conn.execute(f"SELECT rowid FROM chunks WHERE chunk_id IN ({placeholders})", batch)
            ]
            if rowids:
                marks = ",".join("?" * len(rowids))
                self._conn.execute(f"DELETE FROM {VEC_TABLE} WHERE chunk_id IN ({marks})", rowids)

    def ensure_vec_table(self, model: str, dims: int) -> bool:
        """Make sure vec_chunks exists for this model, rebuilding it when the model or size changed.

        Returns True when the vector table was (re)created and every chunk was marked for
        re-embedding, which is the case on first use and whenever the model changes.
        """
        if not self.vec_available:
            raise RuntimeError("sqlite-vec is not available: cannot store embeddings")

        stored_model = self.get_meta("embedding_model")
        stored_dims = self.get_meta("embedding_dims")
        exists = self.has_vec_table()
        if exists and stored_model == model and stored_dims == str(dims):
            return False

        if exists:
            logger.info(
                "search embeddings: model changed (%s/%s -> %s/%d), rebuilding vectors",
                stored_model,
                stored_dims,
                model,
                dims,
            )
            self._conn.execute(f"DROP TABLE {VEC_TABLE}")
        self._conn.execute(
            f"""
            CREATE VIRTUAL TABLE {VEC_TABLE} USING vec0(
                chunk_id INTEGER PRIMARY KEY,
                embedding float[{int(dims)}] distance_metric=cosine,
                chat_jid TEXT,
                start_ts INTEGER
            )
            """
        )
        self._conn.execute("UPDATE chunks SET embedded = 0")
        self._conn.commit()
        self.set_meta("embedding_model", model)
        self.set_meta("embedding_dims", str(dims))
        return True

    def fetch_unembedded_chunks(self, limit: int) -> list["PendingChunk"]:
        rows = self._conn.execute(
            "SELECT rowid, chunk_id, chat_jid, start_ts, text FROM chunks WHERE embedded = 0 ORDER BY rowid LIMIT ?",
            (limit,),
        ).fetchall()
        return [PendingChunk(rowid=r[0], chunk_id=r[1], chat_jid=r[2], start_ts=r[3], text=r[4]) for r in rows]

    def store_vectors(self, items: list[tuple["PendingChunk", list[float]]]) -> None:
        """Write one vector per chunk (replacing any old one) and mark those chunks embedded."""
        if not items:
            return
        from sqlite_vec import serialize_float32

        for chunk, vector in items:
            self._conn.execute(f"DELETE FROM {VEC_TABLE} WHERE chunk_id = ?", (chunk.rowid,))
            self._conn.execute(
                f"INSERT INTO {VEC_TABLE} (chunk_id, embedding, chat_jid, start_ts) VALUES (?, ?, ?, ?)",
                (chunk.rowid, serialize_float32(vector), chunk.chat_jid, chunk.start_ts),
            )
        marks = ",".join("?" * len(items))
        self._conn.execute(f"UPDATE chunks SET embedded = 1 WHERE rowid IN ({marks})", [c.rowid for c, _ in items])
        self._conn.commit()

    def count_chunks(self, embedded: bool | None = None) -> int:
        if embedded is None:
            return self._conn.execute("SELECT COUNT(*) FROM chunks").fetchone()[0]
        return self._conn.execute("SELECT COUNT(*) FROM chunks WHERE embedded = ?", (int(embedded),)).fetchone()[0]

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
