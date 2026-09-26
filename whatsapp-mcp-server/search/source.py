"""Read-only access to the WhatsApp bridge's SQLite databases.

Every connection here is opened with ``mode=ro`` and never writes. Callers
pass explicit database paths so this module has no import-time dependency
on the bridge store being present (keeps it testable against fixtures).
"""

import json
import sqlite3
from dataclasses import dataclass
from datetime import UTC, datetime

DEFAULT_BATCH_SIZE = 500
DEFAULT_OWNER_LABEL = "Eu"

_SKIPPED_CHAT_SUFFIXES = ("@newsletter", "@broadcast")

_MEDIA_MARKERS = {
    "image": "[imagem]",
    "audio": "[áudio]",
    "video": "[vídeo]",
}


@dataclass(frozen=True)
class RawMessage:
    """A row from messages.db, unresolved (no name lookups applied yet)."""

    rowid: int
    message_id: str
    chat_jid: str
    sender: str
    content: str
    timestamp: str
    is_from_me: bool
    media_type: str | None
    filename: str | None
    instance_jid: str | None = None
    is_deleted_remote: bool = False


def _connect_readonly(db_path: str) -> sqlite3.Connection:
    conn = sqlite3.connect(f"file:{db_path}?mode=ro", uri=True)
    conn.execute("PRAGMA busy_timeout = 5000")
    return conn


@dataclass(frozen=True)
class PrivacyEvent:
    """An anonymization or retention action recorded by the bridge."""

    id: int
    action: str
    details: dict


def fetch_privacy_events(messages_db_path: str, after_id: int) -> list[PrivacyEvent]:
    """Privacy log entries newer than after_id. Empty when the bridge has not created the log yet."""
    conn = _connect_readonly(messages_db_path)
    try:
        try:
            rows = conn.execute(
                "SELECT id, action, details FROM privacy_log WHERE id > ? ORDER BY id ASC", (after_id,)
            ).fetchall()
        except sqlite3.OperationalError:
            return []  # an older bridge: no privacy log
    finally:
        conn.close()
    events = []
    for event_id, action, details in rows:
        try:
            parsed = json.loads(details) if details else {}
        except ValueError:
            parsed = {}
        events.append(PrivacyEvent(id=event_id, action=action, details=parsed if isinstance(parsed, dict) else {}))
    return events


def should_index_chat(chat_jid: str) -> bool:
    """Whether a chat is eligible for indexing (skips newsletters/broadcasts)."""
    return not chat_jid.endswith(_SKIPPED_CHAT_SUFFIXES)


def user_part(jid: str) -> str:
    """The identifier portion of a JID, before the ``@``."""
    return jid.split("@")[0]


def parse_timestamp(value: str) -> int:
    """Parse the bridge's ``YYYY-MM-DD HH:MM:SS+00:00`` timestamp to a UTC epoch int."""
    return int(datetime.fromisoformat(value).timestamp())


def display_text(content: str, media_type: str | None, filename: str | None) -> str:
    """Text to index for a message: the caption/content, or a marker for captionless media."""
    if content:
        return content
    if not media_type:
        return ""
    if media_type == "document" and filename:
        return f"[documento: {filename}]"
    return _MEDIA_MARKERS.get(media_type, f"[{media_type}]")


def fetch_messages_after(messages_db_path: str, cursor: int, limit: int = DEFAULT_BATCH_SIZE) -> list[RawMessage]:
    """Fetch up to ``limit`` messages with rowid > cursor, ordered by rowid.

    rowid (not timestamp) is the resumable cursor: history sync inserts old
    messages later, and INSERT OR REPLACE gives a replaced row a new, higher
    rowid, so scanning by rowid reprocesses replaced rows idempotently.
    """
    conn = _connect_readonly(messages_db_path)
    try:
        col_names = {r[1] for r in conn.execute("PRAGMA table_info(messages)").fetchall()}
        cols = ["rowid", "id", "chat_jid", "sender", "content", "timestamp", "is_from_me", "media_type", "filename"]
        cols.append("instance_jid" if "instance_jid" in col_names else "NULL AS instance_jid")
        cols.append("is_deleted_remote" if "is_deleted_remote" in col_names else "0 AS is_deleted_remote")
        col_str = ", ".join(cols)

        rows = conn.execute(
            f"""
            SELECT {col_str}
            FROM messages
            WHERE rowid > ?
            ORDER BY rowid ASC
            LIMIT ?
            """,
            (cursor, limit),
        ).fetchall()
    finally:
        conn.close()
    return [
        RawMessage(
            rowid=row[0],
            message_id=row[1],
            chat_jid=row[2],
            sender=row[3],
            content=row[4] or "",
            timestamp=row[5],
            is_from_me=bool(row[6]),
            media_type=row[7],
            filename=row[8],
            instance_jid=row[9],
            is_deleted_remote=bool(row[10]),
        )
        for row in rows
    ]


def count_messages_after(messages_db_path: str, cursor: int) -> int:
    """Count messages with rowid > cursor, used for backfill progress logging."""
    conn = _connect_readonly(messages_db_path)
    try:
        (count,) = conn.execute("SELECT COUNT(*) FROM messages WHERE rowid > ?", (cursor,)).fetchone()
    finally:
        conn.close()
    return count


def get_chat_names(messages_db_path: str, chat_jids: set[str]) -> dict[str, str | None]:
    """Look up the stored ``chats.name`` for a set of chat JIDs."""
    if not chat_jids:
        return {}
    conn = _connect_readonly(messages_db_path)
    try:
        placeholders = ",".join("?" * len(chat_jids))
        rows = conn.execute(f"SELECT jid, name FROM chats WHERE jid IN ({placeholders})", tuple(chat_jids)).fetchall()
    finally:
        conn.close()
    return dict(rows)


class ContactResolver:
    """Resolves display names for senders and chats from whatsapp.db.

    Holds one read-only connection open for its lifetime and caches lookups,
    since the same JIDs repeat heavily across a backfill.
    """

    def __init__(self, whatsapp_db_path: str, owner_label: str = DEFAULT_OWNER_LABEL):
        self._conn = _connect_readonly(whatsapp_db_path)
        self._owner_label = owner_label
        self._contact_cache: dict[str, str | None] = {}
        self._lid_cache: dict[str, str | None] = {}

    def close(self) -> None:
        self._conn.close()

    def __enter__(self) -> "ContactResolver":
        return self

    def __exit__(self, *_exc: object) -> None:
        self.close()

    def _lookup_contact(self, jid: str) -> str | None:
        if jid in self._contact_cache:
            return self._contact_cache[jid]
        row = self._conn.execute(
            "SELECT first_name, full_name, push_name, business_name FROM whatsmeow_contacts WHERE their_jid = ? LIMIT 1",
            (jid,),
        ).fetchone()
        name = None
        if row:
            first_name, full_name, push_name, business_name = row
            name = full_name or push_name or first_name or business_name
        self._contact_cache[jid] = name
        return name

    def _lookup_lid_pn(self, lid_jid: str) -> str | None:
        if lid_jid in self._lid_cache:
            return self._lid_cache[lid_jid]
        lid = user_part(lid_jid)
        row = self._conn.execute("SELECT pn FROM whatsmeow_lid_map WHERE lid = ? LIMIT 1", (lid,)).fetchone()
        pn = row[0] if row else None
        self._lid_cache[lid_jid] = pn
        return pn

    def _lookup_via_lid_map(self, lid_jid: str) -> str | None:
        pn = self._lookup_lid_pn(lid_jid)
        if not pn:
            return None
        pn_jid = pn if "@" in pn else f"{pn}@s.whatsapp.net"
        return self._lookup_contact(pn_jid)

    def resolve_sender_name(self, sender_jid: str, is_from_me: bool) -> str:
        """Display name for a message sender. ``sender_name`` in messages.db is ignored:
        it equals the sender JID in ~99.9% of rows, so it carries no information."""
        if is_from_me:
            return self._owner_label
        name = self._lookup_contact(sender_jid)
        if name:
            return name
        if sender_jid.endswith("@lid"):
            name = self._lookup_via_lid_map(sender_jid)
            if name:
                return name
        return user_part(sender_jid)

    def resolve_chat_name(self, chat_jid: str, stored_name: str | None) -> str:
        """Display name for a chat, falling back to contact lookup for bare-number names."""
        if chat_jid.endswith("@g.us"):
            return stored_name or user_part(chat_jid)
        if stored_name and not stored_name.strip().lstrip("+").isdigit():
            return stored_name
        name = self._lookup_contact(chat_jid)
        if name:
            return name
        if chat_jid.endswith("@lid"):
            name = self._lookup_via_lid_map(chat_jid)
            if name:
                return name
        return stored_name or user_part(chat_jid)


def utc_date_str(epoch_seconds: int) -> str:
    """Format an epoch timestamp as a UTC ``DD/MM/YYYY`` date string."""
    return datetime.fromtimestamp(epoch_seconds, tz=UTC).strftime("%d/%m/%Y")


def utc_time_str(epoch_seconds: int) -> str:
    """Format an epoch timestamp as a UTC ``HH:MM`` time string."""
    return datetime.fromtimestamp(epoch_seconds, tz=UTC).strftime("%H:%M")
