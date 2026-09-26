"""Background loop that keeps store/index.db up to date with messages.db.

Resumable: progress is a single rowid cursor in index.db's meta table, so a
restart picks up exactly where it left off and reprocessing the same batch
twice is a no-op (see IndexStore.upsert_messages and rebuild_chat_region).

Logging never includes message content, chat names, or sender names --
only counts, ids-as-counts and timings.
"""

import time
from datetime import UTC, datetime

from lib.utils import logger

from . import source
from .chunker import ChunkConfig, rebuild_chat_region
from .config import (
    CHUNK_GAP_MINUTES,
    CHUNK_MAX_CHARS,
    CHUNK_MAX_MESSAGES,
    CHUNK_OVERLAP,
    EMBED_BATCH_SIZE,
    INDEX_DB_PATH,
    INDEX_POLL_SECONDS,
    resolve_messages_db_path,
    resolve_whatsapp_db_path,
)
from .embedder import Embedder, EmbeddingError, create_embedder, embeddings_enabled
from .index_store import IndexedMessage, IndexStore

BATCH_SIZE = 500
EMBEDDER_RETRY_SECONDS = 300


def chunk_config() -> ChunkConfig:
    return ChunkConfig(
        gap_minutes=CHUNK_GAP_MINUTES,
        max_messages=CHUNK_MAX_MESSAGES,
        max_chars=CHUNK_MAX_CHARS,
        overlap=CHUNK_OVERLAP,
    )


def run_once(
    store: IndexStore, messages_db_path: str, resolver: source.ContactResolver, limit: int = BATCH_SIZE
) -> int:
    """Process one batch of new/changed messages starting from the stored cursor.

    Returns the number of raw rows read (0 means caught up).
    """
    apply_privacy_events(store, messages_db_path)
    cursor = store.get_cursor()
    batch = source.fetch_messages_after(messages_db_path, cursor, limit)
    if not batch:
        store.set_meta("last_run_at", datetime.now(UTC).isoformat())
        return 0

    # One conversation per (number, chat): the same customer talking to two numbers is two conversations.
    by_chat: dict[tuple[str, str], list[IndexedMessage]] = {}
    max_rowid = cursor
    for raw in batch:
        max_rowid = max(max_rowid, raw.rowid)
        if not source.should_index_chat(raw.chat_jid):
            continue
        ts = source.parse_timestamp(raw.timestamp)
        text = source.display_text(raw.content, raw.media_type, raw.filename)
        sender_name = resolver.resolve_sender_name(raw.sender, raw.is_from_me)
        indexed = IndexedMessage(
            message_id=raw.message_id,
            chat_jid=raw.chat_jid,
            ts=ts,
            sender_jid=raw.sender,
            sender_name=sender_name,
            from_me=raw.is_from_me,
            text=text,
            instance_jid=raw.instance_jid or "",
            is_deleted_remote=raw.is_deleted_remote,
        )
        by_chat.setdefault((raw.instance_jid or "", raw.chat_jid), []).append(indexed)

    all_rows = [row for rows in by_chat.values() for row in rows]
    store.upsert_messages(all_rows)

    if by_chat:
        chat_names = source.get_chat_names(messages_db_path, {chat_jid for _, chat_jid in by_chat})
        cfg = chunk_config()
        for (instance_jid, chat_jid), rows in by_chat.items():
            chat_name = resolver.resolve_chat_name(chat_jid, chat_names.get(chat_jid))
            is_group = chat_jid.endswith("@g.us")
            min_ts = min(r.ts for r in rows)
            max_ts = max(r.ts for r in rows)
            rebuild_chat_region(store, chat_jid, chat_name, is_group, min_ts, max_ts, cfg, instance_jid)

    store.set_cursor(max_rowid)
    store.set_meta("last_run_at", datetime.now(UTC).isoformat())
    return len(batch)


def apply_privacy_events(store: IndexStore, messages_db_path: str, cfg: ChunkConfig | None = None) -> int:
    """Forget what the bridge's privacy log says was anonymized or purged. Returns events applied.

    The bridge rewrites the affected messages, which the indexer picks up like any change, but it
    cannot reach index.db: the old chat identifier (a phone number) and text would otherwise stay
    searchable. The log entries carry what to drop.
    """
    cursor = int(store.get_meta("privacy_cursor") or 0)
    events = source.fetch_privacy_events(messages_db_path, cursor)
    if not events:
        return 0

    cfg = cfg or chunk_config()
    for event in events:
        if event.action == "anonymize":
            for chat_jid in event.details.get("chat_jids", []):
                store.purge_chat(chat_jid)
        elif event.action == "purge" and event.details.get("cutoff"):
            cutoff = int(datetime.fromisoformat(event.details["cutoff"]).timestamp())
            store.purge_before(cutoff)
        store.set_meta("privacy_cursor", str(event.id))

    # Messages that outlived their chunk get a new one.
    for instance_jid, chat_jid, min_ts, max_ts in store.unchunked_ranges():
        rebuild_chat_region(
            store, chat_jid, chat_jid.split("@")[0], chat_jid.endswith("@g.us"), min_ts, max_ts, cfg, instance_jid
        )
    logger.info("search indexer: applied %d privacy events", len(events))
    return len(events)


def embed_pending(store: IndexStore, embedder: Embedder, batch_size: int = EMBED_BATCH_SIZE) -> int:
    """Embed one batch of chunks that have no vector yet. Returns how many were embedded (0 = none left)."""
    pending = store.fetch_unembedded_chunks(batch_size)
    if not pending:
        return 0
    vectors = embedder.embed_passages([chunk.text for chunk in pending])
    store.store_vectors(list(zip(pending, vectors, strict=True)))
    return len(pending)


def _load_embedder(store: IndexStore) -> Embedder | None:
    """Create the configured embedder and prepare vec_chunks for it; None if it cannot be used right now."""
    try:
        embedder = create_embedder()
        if store.ensure_vec_table(embedder.name, embedder.dims):
            logger.info("search embeddings: %d chunks queued for embedding", store.count_chunks(embedded=False))
        return embedder
    except (EmbeddingError, RuntimeError) as exc:
        logger.warning(
            "search embeddings: unavailable, keeping keyword indexing only (retry in %ds): %s",
            EMBEDDER_RETRY_SECONDS,
            exc,
        )
        return None


def run_forever(
    messages_db_path: str,
    whatsapp_db_path: str,
    index_db_path: str = INDEX_DB_PATH,
    poll_seconds: int = INDEX_POLL_SECONDS,
) -> None:
    store = IndexStore(index_db_path)
    resolver = source.ContactResolver(whatsapp_db_path)
    try:
        starting_cursor = store.get_cursor()
        total_pending = source.count_messages_after(messages_db_path, starting_cursor)
        is_backfill = starting_cursor == 0 and total_pending > 0
        if is_backfill:
            logger.info("search indexer: starting backfill of %d messages", total_pending)

        use_embeddings = embeddings_enabled()
        embedder: Embedder | None = None
        next_embedder_attempt = 0.0
        embedded_total = 0

        processed_total = 0
        start_time = time.monotonic()
        while True:
            processed = run_once(store, messages_db_path, resolver)
            if not processed:
                # Caught up on messages: spend the idle time embedding, and only sleep when that is done too.
                if use_embeddings and embedder is None and time.monotonic() >= next_embedder_attempt:
                    embedder = _load_embedder(store)
                    if embedder is None:
                        next_embedder_attempt = time.monotonic() + EMBEDDER_RETRY_SECONDS
                if embedder is not None:
                    try:
                        embedded = embed_pending(store, embedder)
                    except Exception:
                        logger.exception("search embeddings: batch failed, retrying later")
                        embedded = 0
                    if embedded:
                        embedded_total += embedded
                        logger.info(
                            "search embeddings: embedded %d chunks this run, %d pending",
                            embedded_total,
                            store.count_chunks(embedded=False),
                        )
                        continue
                time.sleep(poll_seconds)
                continue

            processed_total += processed
            if is_backfill:
                elapsed = time.monotonic() - start_time
                rate = processed_total / elapsed if elapsed > 0 else 0
                remaining = max(total_pending - processed_total, 0)
                eta_seconds = int(remaining / rate) if rate > 0 else 0
                logger.info(
                    "search indexer: backfilled %d/%d messages (eta %ds)",
                    min(processed_total, total_pending),
                    total_pending,
                    eta_seconds,
                )
                if processed_total >= total_pending:
                    is_backfill = False
            # Keep draining while there is a backlog instead of sleeping.
    finally:
        resolver.close()
        store.close()


def main() -> None:
    run_forever(resolve_messages_db_path(), resolve_whatsapp_db_path())


if __name__ == "__main__":
    main()
