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
    INDEX_DB_PATH,
    INDEX_POLL_SECONDS,
    resolve_messages_db_path,
    resolve_whatsapp_db_path,
)
from .index_store import IndexedMessage, IndexStore

BATCH_SIZE = 500


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
    cursor = store.get_cursor()
    batch = source.fetch_messages_after(messages_db_path, cursor, limit)
    if not batch:
        store.set_meta("last_run_at", datetime.now(UTC).isoformat())
        return 0

    by_chat: dict[str, list[IndexedMessage]] = {}
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
        )
        by_chat.setdefault(raw.chat_jid, []).append(indexed)

    all_rows = [row for rows in by_chat.values() for row in rows]
    store.upsert_messages(all_rows)

    if by_chat:
        chat_names = source.get_chat_names(messages_db_path, set(by_chat))
        cfg = chunk_config()
        for chat_jid, rows in by_chat.items():
            chat_name = resolver.resolve_chat_name(chat_jid, chat_names.get(chat_jid))
            is_group = chat_jid.endswith("@g.us")
            min_ts = min(r.ts for r in rows)
            max_ts = max(r.ts for r in rows)
            rebuild_chat_region(store, chat_jid, chat_name, is_group, min_ts, max_ts, cfg)

    store.set_cursor(max_rowid)
    store.set_meta("last_run_at", datetime.now(UTC).isoformat())
    return len(batch)


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

        processed_total = 0
        start_time = time.monotonic()
        while True:
            processed = run_once(store, messages_db_path, resolver)
            if not processed:
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
