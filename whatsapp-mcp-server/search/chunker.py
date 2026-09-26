"""Splits a chat's messages into overlapping conversation windows ("chunks").

``build_chunks`` is a pure function: same input, same output, which is what
lets incremental rebuilds and reruns stay idempotent. ``rebuild_chat_region``
is the orchestration layer that decides *which* messages to feed it and
writes the result back to the IndexStore.
"""

import hashlib
from dataclasses import dataclass

from .index_store import Chunk, ChunkMessage, IndexStore
from .source import utc_date_str, utc_time_str


@dataclass(frozen=True)
class ChunkConfig:
    gap_minutes: int = 30
    max_messages: int = 15
    max_chars: int = 1500
    overlap: int = 2


def _format_line(message: ChunkMessage) -> str:
    return f"{utc_time_str(message.ts)} {message.sender_name}: {message.text}"


def _chunk_id(chat_jid: str, anchor_message_id: str, instance_jid: str = "") -> str:
    # The number is part of the identity: two numbers talking to the same customer have two
    # different conversations, even though the chat JID is the same.
    key = f"{instance_jid}|{chat_jid}:{anchor_message_id}" if instance_jid else f"{chat_jid}:{anchor_message_id}"
    return hashlib.sha1(key.encode()).hexdigest()[:16]


def _make_chunk(
    full: list[ChunkMessage],
    primary: list[ChunkMessage],
    anchor: str,
    chat_jid: str,
    chat_name: str,
    is_group: bool,
    instance_jid: str = "",
) -> Chunk:
    start_ts = full[0].ts
    end_ts = full[-1].ts
    kind = "grupo" if is_group else "privado"
    header = f"[{chat_name} | {kind}] {utc_date_str(start_ts)}"
    body = "\n".join(_format_line(m) for m in full)
    return Chunk(
        chunk_id=_chunk_id(chat_jid, anchor, instance_jid),
        instance_jid=instance_jid,
        chat_jid=chat_jid,
        chat_name=chat_name,
        is_group=is_group,
        start_ts=start_ts,
        end_ts=end_ts,
        message_ids=[m.message_id for m in full],
        senders=sorted({m.sender_name for m in full}),
        text=f"{header}\n{body}",
        primary_message_ids=[m.message_id for m in primary],
    )


def build_chunks(
    messages: list[ChunkMessage],
    chat_jid: str,
    chat_name: str,
    is_group: bool,
    cfg: ChunkConfig = ChunkConfig(),
    instance_jid: str = "",
) -> list[Chunk]:
    """Chunk an ordered set of messages for one chat, as seen by one number.

    A new window starts when the silence since the last message exceeds
    ``gap_minutes``, or the window would exceed ``max_messages`` or
    ``max_chars``. Each new window (after the first) is seeded with the last
    ``overlap`` messages of the window it follows, so consecutive chunks
    share context.
    """
    if not messages:
        return []

    ordered = sorted(messages, key=lambda m: (m.ts, m.message_id))

    gap_seconds = cfg.gap_minutes * 60
    chunks: list[Chunk] = []
    overlap: list[ChunkMessage] = []
    window: list[ChunkMessage] = []
    anchor = ordered[0].message_id

    def close_window() -> None:
        nonlocal overlap
        if not window:
            return
        full = overlap + window
        chunks.append(_make_chunk(full, window, anchor, chat_jid, chat_name, is_group, instance_jid))
        overlap = full[-cfg.overlap :] if cfg.overlap > 0 else []

    for msg in ordered:
        if not window:
            window = [msg]
            anchor = msg.message_id
            continue

        candidate = overlap + window + [msg]
        candidate_chars = sum(len(_format_line(m)) for m in candidate)
        gap = msg.ts - window[-1].ts

        if gap > gap_seconds or len(window) + 1 > cfg.max_messages or candidate_chars > cfg.max_chars:
            close_window()
            window = [msg]
            anchor = msg.message_id
        else:
            window.append(msg)

    close_window()
    return chunks


def rebuild_chat_region(
    store: IndexStore,
    chat_jid: str,
    chat_name: str,
    is_group: bool,
    min_ts: int,
    max_ts: int,
    cfg: ChunkConfig = ChunkConfig(),
    instance_jid: str = "",
) -> list[Chunk]:
    """Recompute chunks for the region of a chat touched by new/changed messages.

    Handles both cases from a single code path:
    - New tail messages within ``gap_minutes`` of the chat's last chunk
      extend it (or start a fresh one once a limit is hit).
    - Late-arriving old messages (from history sync landing out of ts order)
      only rebuild the chunks whose span comes within ``gap_minutes`` of
      [min_ts, max_ts]; chunks further away are left untouched.
    """
    gap_seconds = cfg.gap_minutes * 60
    touching = store.get_chunks_touching(chat_jid, min_ts - gap_seconds, max_ts + gap_seconds, instance_jid)

    region_start = min_ts
    region_end = max_ts
    for chunk in touching:
        region_start = min(region_start, chunk.start_ts)
        region_end = max(region_end, chunk.end_ts)

    region_messages = store.get_messages_in_range(chat_jid, region_start, region_end, instance_jid)
    new_chunks = build_chunks(region_messages, chat_jid, chat_name, is_group, cfg, instance_jid)
    store.replace_chunks(chat_jid, [c.chunk_id for c in touching], new_chunks, instance_jid)
    return new_chunks
