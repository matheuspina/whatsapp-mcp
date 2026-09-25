"""Hybrid search over store/index.db: keyword (FTS5) + semantic (sqlite-vec), fused with RRF.

This is the read side of the index. It opens index.db read-only, so it can never
change what the indexer wrote, and it works even while the indexer is still
backfilling: whatever is indexed is searchable, and the result says how far the
index has got (``coverage``).

Errors a caller can act on (ambiguous chat name, unknown sender, bad date) are
raised as ``SearchError`` with a message written for the agent that will read it.
"""

import json
import re
import sqlite3
import time
import unicodedata
from collections.abc import Callable
from dataclasses import dataclass
from datetime import datetime, timedelta
from typing import Any, Literal
from zoneinfo import ZoneInfo

from lib.utils import logger

from . import config, source
from .embedder import Embedder, EmbeddingError
from .index_store import VEC_TABLE, load_vec_extension

SearchMode = Literal["hybrid", "keyword", "semantic"]

RRF_K = 60
DEFAULT_LIMIT = 10
MAX_LIMIT = 30
MAX_MESSAGE_CHARS = 500
MAX_PAYLOAD_CHARS = 60_000
CACHE_TTL_SECONDS = 60
# A chunk that starts before date_from can still overlap it; this is how far back the vector filter looks.
DATE_SLACK_SECONDS = 24 * 3600

# Words that carry no search signal in a Portuguese question. Dropped from the keyword query only;
# the semantic side sees the full sentence.
_STOPWORDS = {
    "a", "o", "as", "os", "um", "uma", "uns", "umas", "de", "do", "da", "dos", "das", "em", "no", "na", "nos",
    "nas", "por", "para", "pra", "com", "e", "ou", "que", "se", "foi", "ser", "sobre", "quando", "quem", "qual",
    "quais", "como", "onde", "algo", "alguem", "ao", "aos", "eu", "ele", "ela", "me", "te", "lhe", "mais", "muito",
    "ja", "so", "tem", "tinha", "esta", "esse", "essa", "isso", "este", "esta", "dito", "disse", "falou", "falado",
}  # fmt: skip


class SearchError(ValueError):
    """The search request cannot be answered as asked; the message says what to change."""


def normalize(text: str) -> str:
    """Lowercase and strip accents, for name matching."""
    decomposed = unicodedata.normalize("NFKD", text)
    return "".join(c for c in decomposed if not unicodedata.combining(c)).casefold()


def _tokens(text: str) -> list[str]:
    return re.findall(r"[^\W_]+", text, flags=re.UNICODE)


def build_fts_query(query: str) -> str | None:
    """Turn free text into a safe FTS5 expression, or None when nothing searchable is left.

    Every term is quoted (so user text can never be read as FTS operators), terms of three or
    more letters match as prefixes ("viag" finds "viagem"), and short ones such as "SP" match
    exactly. The unicode61 tokenizer already ignores accents and case on both sides.
    """
    terms = _tokens(query)
    if not terms:
        return None
    meaningful = [t for t in terms if normalize(t) not in _STOPWORDS] or terms
    parts = [f'"{t}"*' if len(t) >= 3 else f'"{t}"' for t in dict.fromkeys(meaningful)]
    return "text : (" + " OR ".join(parts) + ")"


@dataclass
class _Filters:
    chat_jids: list[str] | None = None
    sender_jids: list[str] | None = None
    ts_from: int | None = None
    ts_to: int | None = None

    @property
    def any(self) -> bool:
        return bool(self.chat_jids or self.sender_jids or self.ts_from is not None or self.ts_to is not None)


def _marks(values: list[Any]) -> str:
    return ",".join("?" * len(values))


class SearchService:
    """Answers search and status questions from index.db.

    One instance is meant to live as long as the MCP server: it caches the chat and sender lists
    for a minute and keeps the embedding model loaded once it has been used.
    """

    def __init__(
        self,
        index_db_path: str = config.INDEX_DB_PATH,
        embedder_factory: Callable[[], Embedder] | None = None,
        messages_db_path: str | None = None,
        display_tz: str = config.DISPLAY_TZ,
        k_fts: int = config.SEARCH_K_FTS,
        k_vec: int = config.SEARCH_K_VEC,
        min_similarity: float = config.SEARCH_MIN_SIMILARITY,
    ):
        self._index_db_path = index_db_path
        self._embedder_factory = embedder_factory
        self._messages_db_path = messages_db_path
        self._tz = ZoneInfo(display_tz)
        self._k_fts = k_fts
        self._k_vec = k_vec
        self._min_similarity = min_similarity

        self._embedder: Embedder | None = None
        self._embedder_error: str | None = None
        self._embedder_failed_at = 0.0
        self._chats_cache: tuple[float, list[dict[str, Any]]] | None = None
        self._senders_cache: tuple[float, list[tuple[str, str]]] | None = None

    # -- plumbing ---------------------------------------------------------

    def _connect(self) -> sqlite3.Connection:
        try:
            conn = sqlite3.connect(f"file:{self._index_db_path}?mode=ro", uri=True)
        except sqlite3.OperationalError as exc:
            raise SearchError(
                f"The search index is not available at {self._index_db_path}. Start the 'indexer' service "
                f"and let it finish its first pass, then try again ({exc})."
            ) from exc
        conn.execute("PRAGMA busy_timeout = 5000")
        load_vec_extension(conn)
        return conn

    def _iso(self, ts: int) -> str:
        return datetime.fromtimestamp(ts, self._tz).isoformat()

    def _date(self, ts: int) -> str:
        return datetime.fromtimestamp(ts, self._tz).date().isoformat()

    def _parse_bound(self, value: str, *, end: bool, name: str) -> int:
        try:
            parsed = datetime.fromisoformat(value.strip())
        except ValueError as exc:
            raise SearchError(f"{name} must be an ISO date like 2026-03-12, got '{value}'") from exc
        date_only = len(value.strip()) == 10
        if parsed.tzinfo is None:
            parsed = parsed.replace(tzinfo=self._tz)
        if date_only and end:
            parsed = parsed + timedelta(days=1) - timedelta(seconds=1)
        return int(parsed.timestamp())

    def _get_embedder(self) -> Embedder:
        if self._embedder is not None:
            return self._embedder
        if self._embedder_factory is None:
            raise EmbeddingError("no embedding backend is configured (EMBEDDING_BACKEND=none)")
        if self._embedder_error and time.monotonic() - self._embedder_failed_at < CACHE_TTL_SECONDS:
            raise EmbeddingError(self._embedder_error)
        try:
            self._embedder = self._embedder_factory()
        except EmbeddingError as exc:
            self._embedder_error = str(exc)
            self._embedder_failed_at = time.monotonic()
            raise
        self._embedder_error = None
        return self._embedder

    # -- name resolution ----------------------------------------------------

    def _chats(self, conn: sqlite3.Connection) -> list[dict[str, Any]]:
        now = time.monotonic()
        if self._chats_cache and now - self._chats_cache[0] < CACHE_TTL_SECONDS:
            return self._chats_cache[1]
        rows = conn.execute(
            "SELECT chat_jid, chat_name, is_group, MAX(start_ts) FROM chunks GROUP BY chat_jid"
        ).fetchall()
        chats = [
            {"jid": r[0], "name": r[1] or r[0].split("@")[0], "is_group": bool(r[2]), "last_ts": r[3]} for r in rows
        ]
        self._chats_cache = (now, chats)
        return chats

    def resolve_chat(self, conn: sqlite3.Connection, chat: str) -> dict[str, Any]:
        """Find one indexed chat by JID, phone number or (accent-insensitive) name."""
        chats = self._chats(conn)
        wanted = chat.strip()
        by_jid = [c for c in chats if c["jid"] == wanted]
        if by_jid:
            return by_jid[0]

        if wanted.isdigit():
            by_phone = [c for c in chats if c["jid"].split("@")[0] == wanted]
            if by_phone:
                return by_phone[0]

        needle = normalize(wanted)
        exact = [c for c in chats if normalize(c["name"]) == needle]
        if len(exact) == 1:
            return exact[0]
        candidates = exact
        if not candidates:
            words = _tokens(needle)
            candidates = [c for c in chats if words and all(w in normalize(c["name"]) for w in words)]
        if len(candidates) == 1:
            return candidates[0]
        if not candidates:
            raise SearchError(f"No indexed chat matches '{chat}'. Use list_chats to find the exact name or JID.")

        candidates.sort(key=lambda c: c["last_ts"] or 0, reverse=True)
        listing = "; ".join(f"{c['name']} ({c['jid']})" for c in candidates[:10])
        raise SearchError(f"'{chat}' matches {len(candidates)} chats. Pass the JID of one of them: {listing}")

    def _senders(self, conn: sqlite3.Connection) -> list[tuple[str, str]]:
        now = time.monotonic()
        if self._senders_cache and now - self._senders_cache[0] < CACHE_TTL_SECONDS:
            return self._senders_cache[1]
        rows = conn.execute("SELECT DISTINCT sender_jid, sender_name FROM messages_idx").fetchall()
        senders = [(r[0] or "", r[1] or "") for r in rows]
        self._senders_cache = (now, senders)
        return senders

    def resolve_sender(self, conn: sqlite3.Connection, sender: str) -> tuple[list[str], list[str]]:
        """Sender JIDs and display names matching a name or phone number. Raises if none match."""
        wanted = sender.strip()
        senders = self._senders(conn)
        digits = re.sub(r"\D", "", wanted)
        if digits and len(digits) >= 6 and digits == re.sub(r"[\s+()-]", "", wanted):
            matched = [(j, n) for j, n in senders if digits in j.split("@")[0]]
        else:
            words = _tokens(normalize(wanted))
            matched = [(j, n) for j, n in senders if words and all(w in normalize(n) for w in words)]
        if not matched:
            raise SearchError(f"No indexed sender matches '{sender}'. Try a different spelling or a phone number.")
        jids = sorted({j for j, _ in matched if j})
        names = sorted({n for _, n in matched if n})
        return jids, names

    # -- retrieval paths ------------------------------------------------------

    def _keyword(self, conn: sqlite3.Connection, fts_query: str, filters: _Filters) -> list[tuple[str, str]]:
        """(chunk_id, message_id) for the best keyword matches, best first."""
        sql = [
            "SELECT m.chunk_id, m.message_id FROM messages_fts",
            "JOIN messages_idx m ON m.rowid = messages_fts.rowid",
            "WHERE messages_fts MATCH ? AND m.chunk_id IS NOT NULL",
        ]
        params: list[Any] = [fts_query]
        if filters.chat_jids:
            sql.append(f"AND m.chat_jid IN ({_marks(filters.chat_jids)})")
            params += filters.chat_jids
        if filters.sender_jids:
            sql.append(f"AND m.sender_jid IN ({_marks(filters.sender_jids)})")
            params += filters.sender_jids
        if filters.ts_from is not None:
            sql.append("AND m.ts >= ?")
            params.append(filters.ts_from)
        if filters.ts_to is not None:
            sql.append("AND m.ts <= ?")
            params.append(filters.ts_to)
        sql.append("ORDER BY bm25(messages_fts) LIMIT ?")
        params.append(self._k_fts)
        return [(r[0], r[1]) for r in conn.execute(" ".join(sql), params)]

    def _semantic(
        self, conn: sqlite3.Connection, query: str, filters: _Filters, sender_names: list[str]
    ) -> list[tuple[str, float]]:
        """(chunk_id, similarity) for the nearest chunks, best first."""
        if not conn.execute("SELECT 1 FROM sqlite_master WHERE name = ?", (VEC_TABLE,)).fetchone():
            raise EmbeddingError("the index has no embeddings yet (the indexer is still computing them)")
        embedder = self._get_embedder()
        stored_model = conn.execute("SELECT value FROM meta WHERE key = 'embedding_model'").fetchone()
        if stored_model and stored_model[0] != embedder.name:
            raise EmbeddingError(
                f"the index was embedded with '{stored_model[0]}' but this server uses '{embedder.name}'; "
                "set the same EMBEDDING_MODEL on both"
            )

        from sqlite_vec import serialize_float32

        k = self._k_vec * 3 if sender_names else self._k_vec
        sql = [f"SELECT chunk_id, distance FROM {VEC_TABLE} WHERE embedding MATCH ? AND k = ?"]
        params: list[Any] = [serialize_float32(embedder.embed_query(query)), k]
        if filters.chat_jids:
            sql.append(f"AND chat_jid IN ({_marks(filters.chat_jids)})")
            params += filters.chat_jids
        if filters.ts_from is not None:
            sql.append("AND start_ts >= ?")
            params.append(filters.ts_from - DATE_SLACK_SECONDS)
        if filters.ts_to is not None:
            sql.append("AND start_ts <= ?")
            params.append(filters.ts_to)
        hits = conn.execute(" ".join(sql), params).fetchall()
        if not hits:
            return []

        rowids = [h[0] for h in hits]
        rows = conn.execute(
            f"SELECT rowid, chunk_id, end_ts, senders FROM chunks WHERE rowid IN ({_marks(rowids)})", rowids
        ).fetchall()
        info = {r[0]: r for r in rows}

        wanted_names = set(sender_names)
        out: list[tuple[str, float]] = []
        for rowid, distance in hits:
            row = info.get(rowid)
            if row is None:
                continue
            similarity = 1.0 - float(distance)
            if similarity < self._min_similarity:
                continue
            if filters.ts_from is not None and row[2] < filters.ts_from:
                continue
            if wanted_names and not wanted_names.intersection(json.loads(row[3])):
                continue
            out.append((row[1], similarity))
        return out[: self._k_vec]

    # -- search -----------------------------------------------------------------

    def search(
        self,
        query: str,
        chat: str | None = None,
        sender: str | None = None,
        date_from: str | None = None,
        date_to: str | None = None,
        mode: SearchMode = "hybrid",
        limit: int = DEFAULT_LIMIT,
    ) -> dict[str, Any]:
        """Search the index. See the ``search_messages`` tool for the meaning of each argument."""
        if not query or not query.strip():
            raise SearchError("query must not be empty")
        if mode not in ("hybrid", "keyword", "semantic"):
            raise SearchError("mode must be one of: hybrid, keyword, semantic")
        limit = max(1, min(int(limit), MAX_LIMIT))

        conn = self._connect()
        try:
            filters = _Filters()
            scope_chat: dict[str, Any] | None = None
            sender_names: list[str] = []
            if chat:
                scope_chat = self.resolve_chat(conn, chat)
                filters.chat_jids = [scope_chat["jid"]]
            if sender:
                filters.sender_jids, sender_names = self.resolve_sender(conn, sender)
            if date_from:
                filters.ts_from = self._parse_bound(date_from, end=False, name="date_from")
            if date_to:
                filters.ts_to = self._parse_bound(date_to, end=True, name="date_to")
            if filters.ts_from is not None and filters.ts_to is not None and filters.ts_from > filters.ts_to:
                raise SearchError("date_from is after date_to")

            notes: list[str] = []
            keyword_hits: list[tuple[str, str]] = []
            semantic_hits: list[tuple[str, float]] = []

            if mode in ("hybrid", "keyword"):
                fts_query = build_fts_query(query)
                if fts_query:
                    keyword_hits = self._keyword(conn, fts_query, filters)
                elif mode == "keyword":
                    raise SearchError("query has no searchable words")

            if mode in ("hybrid", "semantic"):
                try:
                    semantic_hits = self._semantic(conn, query, filters, sender_names)
                except (EmbeddingError, sqlite3.Error) as exc:
                    if mode == "semantic":
                        raise SearchError(f"Semantic search is not available: {exc}") from exc
                    logger.warning("search: semantic path skipped: %s", exc)
                    notes.append(f"Semantic search unavailable ({exc}); showing keyword matches only.")

            results = self._fuse_and_build(conn, keyword_hits, semantic_hits, limit, notes)
            coverage = self._coverage(conn, filters, scope_chat, notes)
            response: dict[str, Any] = {"results": results, "coverage": coverage}
            if filters.any:
                response["filters"] = {
                    "chat": scope_chat and {"jid": scope_chat["jid"], "name": scope_chat["name"]},
                    "sender_matched": sender_names or None,
                    "date_from": date_from,
                    "date_to": date_to,
                }
            return response
        finally:
            conn.close()

    def _fuse_and_build(
        self,
        conn: sqlite3.Connection,
        keyword_hits: list[tuple[str, str]],
        semantic_hits: list[tuple[str, float]],
        limit: int,
        notes: list[str],
    ) -> list[dict[str, Any]]:
        scores: dict[str, float] = {}
        matched_by: dict[str, list[str]] = {}
        keyword_messages: dict[str, set[str]] = {}
        similarity: dict[str, float] = {}

        keyword_rank = 0
        for chunk_id, message_id in keyword_hits:
            if chunk_id not in keyword_messages:
                keyword_rank += 1
                scores[chunk_id] = scores.get(chunk_id, 0.0) + 1.0 / (RRF_K + keyword_rank)
                matched_by.setdefault(chunk_id, []).append("keyword")
            keyword_messages.setdefault(chunk_id, set()).add(message_id)
        for rank, (chunk_id, sim) in enumerate(semantic_hits, start=1):
            scores[chunk_id] = scores.get(chunk_id, 0.0) + 1.0 / (RRF_K + rank)
            matched_by.setdefault(chunk_id, []).append("semantic")
            similarity[chunk_id] = sim

        ranked = sorted(scores, key=lambda cid: scores[cid], reverse=True)[:limit]
        if not ranked:
            return []

        chunk_rows = {
            r[0]: r
            for r in conn.execute(
                "SELECT chunk_id, chat_jid, chat_name, is_group, start_ts, end_ts, message_ids "
                f"FROM chunks WHERE chunk_id IN ({_marks(ranked)})",
                ranked,
            )
        }

        results: list[dict[str, Any]] = []
        budget = MAX_PAYLOAD_CHARS
        for chunk_id in ranked:
            row = chunk_rows.get(chunk_id)
            if row is None:
                continue
            messages = self._chunk_messages(conn, row[1], json.loads(row[6]), keyword_messages.get(chunk_id, set()))
            size = sum(len(m["text"]) + 80 for m in messages)
            if results and size > budget:
                notes.append("Output was cut to fit the size limit; narrow the search or lower the limit.")
                break
            budget -= size
            result: dict[str, Any] = {
                "chat": {"jid": row[1], "name": row[2] or row[1].split("@")[0], "is_group": bool(row[3])},
                "period": {"start": self._iso(row[4]), "end": self._iso(row[5])},
                "score": round(scores[chunk_id], 5),
                "matched_by": matched_by[chunk_id],
                "messages": messages,
            }
            if chunk_id in similarity:
                result["similarity"] = round(similarity[chunk_id], 3)
            results.append(result)
        return results

    def _chunk_messages(
        self, conn: sqlite3.Connection, chat_jid: str, message_ids: list[str], keyword_ids: set[str]
    ) -> list[dict[str, Any]]:
        rows = conn.execute(
            "SELECT message_id, ts, sender_jid, sender_name, from_me, text FROM messages_idx "
            "WHERE chat_jid = ? AND message_id IN (SELECT value FROM json_each(?)) ORDER BY ts, message_id",
            (chat_jid, json.dumps(message_ids)),
        ).fetchall()
        out = []
        for message_id, ts, sender_jid, sender_name, from_me, text in rows:
            text = text or ""
            if len(text) > MAX_MESSAGE_CHARS:
                text = text[: MAX_MESSAGE_CHARS - 1] + "…"
            out.append(
                {
                    "id": message_id,
                    "ts": self._iso(ts),
                    "sender": sender_name or (sender_jid or "").split("@")[0],
                    "from_me": bool(from_me),
                    "text": text,
                    "keyword_hit": message_id in keyword_ids,
                }
            )
        return out

    # -- coverage and status -----------------------------------------------------

    def _span(self, conn: sqlite3.Connection, chat_jid: str | None) -> tuple[int | None, int | None]:
        if chat_jid:
            row = conn.execute(
                "SELECT MIN(start_ts), MAX(end_ts) FROM chunks WHERE chat_jid = ?", (chat_jid,)
            ).fetchone()
        else:
            row = conn.execute("SELECT MIN(start_ts), MAX(end_ts) FROM chunks").fetchone()
        return row[0], row[1]

    def _coverage(
        self, conn: sqlite3.Connection, filters: _Filters, scope_chat: dict[str, Any] | None, notes: list[str]
    ) -> dict[str, Any]:
        oldest, newest = self._span(conn, scope_chat["jid"] if scope_chat else None)
        text = ["Results are limited to the history synced to this index."]
        pending = self._pending_embeddings(conn)
        if pending:
            text.append(f"{pending} chunks are still waiting for embeddings, so semantic matches may be incomplete.")
        text.extend(notes)
        return {
            "oldest_indexed": self._date(oldest) if oldest is not None else None,
            "newest_indexed": self._date(newest) if newest is not None else None,
            "note": " ".join(text),
        }

    def _pending_embeddings(self, conn: sqlite3.Connection) -> int:
        if not conn.execute("SELECT 1 FROM sqlite_master WHERE name = ?", (VEC_TABLE,)).fetchone():
            return 0
        return conn.execute("SELECT COUNT(*) FROM chunks WHERE embedded = 0").fetchone()[0]

    def status(self, chat: str | None = None) -> dict[str, Any]:
        """Health of the index: what is indexed, what is still pending, and which model embeds it."""
        conn = self._connect()
        try:
            meta = dict(conn.execute("SELECT key, value FROM meta").fetchall())
            oldest, newest = self._span(conn, None)
            chunks_total = conn.execute("SELECT COUNT(*) FROM chunks").fetchone()[0]
            has_vectors = bool(conn.execute("SELECT 1 FROM sqlite_master WHERE name = ?", (VEC_TABLE,)).fetchone())
            pending_embeddings = conn.execute("SELECT COUNT(*) FROM chunks WHERE embedded = 0").fetchone()[0]
            status: dict[str, Any] = {
                "index": {
                    "messages_indexed": conn.execute("SELECT COUNT(*) FROM messages_idx").fetchone()[0],
                    "chunks": chunks_total,
                    "chunks_awaiting_embedding": pending_embeddings if has_vectors else chunks_total,
                    "oldest": self._date(oldest) if oldest is not None else None,
                    "newest": self._date(newest) if newest is not None else None,
                    "last_run_at": meta.get("last_run_at"),
                },
                "embedding": {
                    "model": meta.get("embedding_model"),
                    "dims": int(meta["embedding_dims"]) if meta.get("embedding_dims") else None,
                    "semantic_search_ready": has_vectors and pending_embeddings < chunks_total,
                },
            }
            if self._messages_db_path:
                try:
                    cursor = int(meta.get("source_cursor") or 0)
                    status["source"] = {
                        "messages_not_yet_indexed": source.count_messages_after(self._messages_db_path, cursor)
                    }
                except sqlite3.Error as exc:
                    logger.warning("search status: could not read messages.db: %s", exc)
            if chat:
                found = self.resolve_chat(conn, chat)
                first, last = self._span(conn, found["jid"])
                status["chat"] = {
                    "jid": found["jid"],
                    "name": found["name"],
                    "is_group": found["is_group"],
                    "messages_indexed": conn.execute(
                        "SELECT COUNT(*) FROM messages_idx WHERE chat_jid = ?", (found["jid"],)
                    ).fetchone()[0],
                    "chunks": conn.execute(
                        "SELECT COUNT(*) FROM chunks WHERE chat_jid = ?", (found["jid"],)
                    ).fetchone()[0],
                    "oldest": self._date(first) if first is not None else None,
                    "newest": self._date(last) if last is not None else None,
                }
            return status
        finally:
            conn.close()
