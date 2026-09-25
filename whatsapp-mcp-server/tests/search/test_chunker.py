"""Tests for search/chunker.py: the pure chunking function and region rebuilds."""

from search.chunker import ChunkConfig, build_chunks, rebuild_chat_region
from search.index_store import ChunkMessage, IndexedMessage, IndexStore

CHAT = "trabalho2026@g.us"
CHAT_NAME = "Trabalho 2026"


def msg(message_id: str, ts: int, sender: str = "Ana", text: str = "oi") -> ChunkMessage:
    return ChunkMessage(message_id=message_id, ts=ts, sender_name=sender, text=text)


class TestBuildChunksEmpty:
    def test_empty_input_returns_no_chunks(self):
        assert build_chunks([], CHAT, CHAT_NAME, is_group=True) == []


class TestBuildChunksGap:
    def test_messages_within_gap_stay_in_one_chunk(self):
        messages = [msg("m1", 0), msg("m2", 60), msg("m3", 120)]
        cfg = ChunkConfig(gap_minutes=30, max_messages=15, max_chars=1500, overlap=2)

        chunks = build_chunks(messages, CHAT, CHAT_NAME, True, cfg)

        assert len(chunks) == 1
        assert chunks[0].message_ids == ["m1", "m2", "m3"]

    def test_silence_beyond_gap_starts_a_new_chunk(self):
        gap = 30 * 60
        messages = [msg("m1", 0), msg("m2", 60), msg("m3", 60 + gap + 1)]
        cfg = ChunkConfig(gap_minutes=30, overlap=0)

        chunks = build_chunks(messages, CHAT, CHAT_NAME, True, cfg)

        assert len(chunks) == 2
        assert chunks[0].message_ids == ["m1", "m2"]
        assert chunks[1].message_ids == ["m3"]

    def test_gap_boundary_is_exclusive(self):
        """A gap of exactly gap_minutes stays in the same window; only exceeding it splits."""
        gap = 30 * 60
        messages = [msg("m1", 0), msg("m2", gap)]
        cfg = ChunkConfig(gap_minutes=30, overlap=0)

        chunks = build_chunks(messages, CHAT, CHAT_NAME, True, cfg)

        assert len(chunks) == 1


class TestBuildChunksLimits:
    def test_max_messages_splits_the_window(self):
        messages = [msg(f"m{i}", i * 10) for i in range(5)]
        cfg = ChunkConfig(gap_minutes=30, max_messages=3, max_chars=100_000, overlap=0)

        chunks = build_chunks(messages, CHAT, CHAT_NAME, True, cfg)

        assert [len(c.message_ids) for c in chunks] == [3, 2]

    def test_max_chars_splits_the_window(self):
        long_text = "x" * 100
        messages = [msg(f"m{i}", i * 10, text=long_text) for i in range(5)]
        # Each formatted line is roughly "HH:MM Ana: " + 100 chars ~= 112 chars.
        cfg = ChunkConfig(gap_minutes=30, max_messages=100, max_chars=250, overlap=0)

        chunks = build_chunks(messages, CHAT, CHAT_NAME, True, cfg)

        assert len(chunks) > 1
        assert sum(len(c.message_ids) for c in chunks) == 5

    def test_a_single_oversized_message_is_not_split(self):
        long_text = "x" * 5000
        messages = [msg("m1", 0, text=long_text)]
        cfg = ChunkConfig(max_chars=100)

        chunks = build_chunks(messages, CHAT, CHAT_NAME, True, cfg)

        assert len(chunks) == 1
        assert chunks[0].message_ids == ["m1"]


class TestBuildChunksOverlap:
    def test_new_window_is_seeded_with_the_previous_tail(self):
        gap = 30 * 60
        messages = [msg("m1", 0), msg("m2", 60), msg("m3", 60 + gap + 1), msg("m4", 60 + gap + 61)]
        cfg = ChunkConfig(gap_minutes=30, overlap=2)

        chunks = build_chunks(messages, CHAT, CHAT_NAME, True, cfg)

        assert len(chunks) == 2
        assert chunks[0].message_ids == ["m1", "m2"]
        # m1, m2 carried over as overlap context, m3/m4 are the new content.
        assert chunks[1].message_ids == ["m1", "m2", "m3", "m4"]
        assert chunks[1].primary_message_ids == ["m3", "m4"]

    def test_overlap_is_capped_to_the_previous_window_size(self):
        """A single-message window can't lend 2 messages of overlap; it lends what it has."""
        gap = 30 * 60
        messages = [msg("m1", 0), msg("m2", gap + 1), msg("m3", gap + 1 + gap + 1)]
        cfg = ChunkConfig(gap_minutes=30, overlap=2)

        chunks = build_chunks(messages, CHAT, CHAT_NAME, True, cfg)

        assert len(chunks) == 3
        assert chunks[0].message_ids == ["m1"]
        # chunk0 only has 1 message to lend, so chunk1's overlap is capped to it.
        assert chunks[1].message_ids == ["m1", "m2"]
        assert chunks[1].primary_message_ids == ["m2"]
        # chunk1 has 2 messages, so chunk2 gets the full overlap of 2.
        assert chunks[2].message_ids == ["m1", "m2", "m3"]
        assert chunks[2].primary_message_ids == ["m3"]

    def test_overlap_zero_means_no_shared_messages(self):
        gap = 30 * 60
        messages = [msg("m1", 0), msg("m2", gap + 1)]
        cfg = ChunkConfig(gap_minutes=30, overlap=0)

        chunks = build_chunks(messages, CHAT, CHAT_NAME, True, cfg)

        assert chunks[0].message_ids == ["m1"]
        assert chunks[1].message_ids == ["m2"]


class TestBuildChunksDeterminism:
    def test_same_input_produces_the_same_chunk_ids(self):
        messages = [msg("m1", 0), msg("m2", 60), msg("m3", 120)]
        cfg = ChunkConfig()

        first = build_chunks(messages, CHAT, CHAT_NAME, True, cfg)
        second = build_chunks(messages, CHAT, CHAT_NAME, True, cfg)

        assert [c.chunk_id for c in first] == [c.chunk_id for c in second]
        assert first[0].text == second[0].text

    def test_out_of_order_input_is_sorted_before_chunking(self):
        messages = [msg("m3", 120), msg("m1", 0), msg("m2", 60)]

        chunks = build_chunks(messages, CHAT, CHAT_NAME, True, ChunkConfig())

        assert chunks[0].message_ids == ["m1", "m2", "m3"]

    def test_chunk_text_contains_group_header_and_lines(self):
        messages = [msg("m1", 0, sender="Ana", text="oi pessoal")]

        chunks = build_chunks(messages, CHAT, CHAT_NAME, True, ChunkConfig())

        assert chunks[0].text.startswith(f"[{CHAT_NAME} | grupo]")
        assert "Ana: oi pessoal" in chunks[0].text

    def test_individual_chat_header_says_privado(self):
        messages = [msg("m1", 0)]

        chunks = build_chunks(messages, "5511900000001@s.whatsapp.net", "Ana Souza", False, ChunkConfig())

        assert "| privado]" in chunks[0].text


class TestRebuildChatRegion:
    def _store(self, tmp_path) -> IndexStore:
        return IndexStore(str(tmp_path / "index.db"))

    def test_appending_a_tail_message_extends_the_open_window(self, tmp_path):
        store = self._store(tmp_path)
        try:
            store.upsert_messages(
                [
                    IndexedMessage("m1", CHAT, 0, "ana@s.whatsapp.net", "Ana", False, "oi"),
                    IndexedMessage("m2", CHAT, 60, "ana@s.whatsapp.net", "Ana", False, "tudo bem"),
                ]
            )
            rebuild_chat_region(store, CHAT, CHAT_NAME, True, 0, 60)

            store.upsert_messages([IndexedMessage("m3", CHAT, 120, "ana@s.whatsapp.net", "Ana", False, "beleza")])
            chunks = rebuild_chat_region(store, CHAT, CHAT_NAME, True, 120, 120)

            assert len(chunks) == 1
            assert chunks[0].message_ids == ["m1", "m2", "m3"]
        finally:
            store.close()

    def test_message_beyond_the_gap_creates_a_new_window(self, tmp_path):
        store = self._store(tmp_path)
        try:
            gap = 30 * 60
            store.upsert_messages([IndexedMessage("m1", CHAT, 0, "ana", "Ana", False, "oi")])
            rebuild_chat_region(store, CHAT, CHAT_NAME, True, 0, 0)

            far_ts = gap + 3600
            store.upsert_messages([IndexedMessage("m2", CHAT, far_ts, "ana", "Ana", False, "de novo")])
            rebuild_chat_region(store, CHAT, CHAT_NAME, True, far_ts, far_ts)

            all_chunks = store.get_chunks_touching(CHAT, 0, far_ts)
            assert len(all_chunks) == 2
        finally:
            store.close()

    def test_late_old_message_only_rebuilds_touching_chunks(self, tmp_path):
        store = self._store(tmp_path)
        try:
            gap = 30 * 60
            # Two far-apart windows.
            store.upsert_messages(
                [
                    IndexedMessage("m1", CHAT, 0, "ana", "Ana", False, "oi"),
                    IndexedMessage("m2", CHAT, 100_000, "ana", "Ana", False, "muito depois"),
                ]
            )
            rebuild_chat_region(store, CHAT, CHAT_NAME, True, 0, 0)
            rebuild_chat_region(store, CHAT, CHAT_NAME, True, 100_000, 100_000)
            untouched = store.get_chunks_touching(CHAT, 100_000, 100_000)
            untouched_id = untouched[0].chunk_id

            # A message landing between the two, but close to the first, arrives late.
            late_ts = 60
            store.upsert_messages([IndexedMessage("m3", CHAT, late_ts, "ana", "Ana", False, "esqueci de mandar")])
            rebuild_chat_region(store, CHAT, CHAT_NAME, True, late_ts, late_ts)

            first_window = store.get_chunks_touching(CHAT, 0, 60)
            assert len(first_window) == 1
            region_messages = store.get_messages_in_range(CHAT, first_window[0].start_ts, first_window[0].end_ts)
            assert {m.message_id for m in region_messages} == {"m1", "m3"}

            far_window = store.get_chunks_touching(CHAT, 100_000, 100_000)
            assert far_window[0].chunk_id == untouched_id  # left untouched
            assert 100_000 - late_ts > gap  # sanity: far window really is out of gap range
        finally:
            store.close()

    def test_rebuild_marks_chunks_unembedded(self, tmp_path):
        store = self._store(tmp_path)
        try:
            store.upsert_messages([IndexedMessage("m1", CHAT, 0, "ana", "Ana", False, "oi")])
            rebuild_chat_region(store, CHAT, CHAT_NAME, True, 0, 0)

            row = store._conn.execute("SELECT embedded FROM chunks WHERE chat_jid = ?", (CHAT,)).fetchone()
            assert row[0] == 0
        finally:
            store.close()
