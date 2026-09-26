"""Tests for search/index_store.py: schema, FTS sync, upsert idempotency."""

from search.index_store import SCHEMA_VERSION, Chunk, IndexedMessage, IndexStore

CHAT = "trabalho2026@g.us"


def store(tmp_path) -> IndexStore:
    return IndexStore(str(tmp_path / "index.db"))


class TestMeta:
    def test_unset_key_returns_none(self, tmp_path):
        s = store(tmp_path)
        try:
            assert s.get_meta("nope") is None
        finally:
            s.close()

    def test_set_and_get_roundtrip(self, tmp_path):
        s = store(tmp_path)
        try:
            s.set_meta("source_cursor", "42")
            assert s.get_meta("source_cursor") == "42"
        finally:
            s.close()

    def test_schema_version_is_set_on_init(self, tmp_path):
        s = store(tmp_path)
        try:
            assert s.get_meta("schema_version") == SCHEMA_VERSION
        finally:
            s.close()

    def test_cursor_defaults_to_zero(self, tmp_path):
        s = store(tmp_path)
        try:
            assert s.get_cursor() == 0
            s.set_cursor(7)
            assert s.get_cursor() == 7
        finally:
            s.close()


class TestSchemaIsIdempotent:
    def test_reopening_an_existing_db_does_not_lose_data(self, tmp_path):
        db_path = str(tmp_path / "index.db")
        s1 = IndexStore(db_path)
        s1.set_cursor(5)
        s1.close()

        s2 = IndexStore(db_path)
        try:
            assert s2.get_cursor() == 5
        finally:
            s2.close()


class TestUpsertMessages:
    def test_insert_then_read_back(self, tmp_path):
        s = store(tmp_path)
        try:
            s.upsert_messages([IndexedMessage("m1", CHAT, 100, "ana@s.whatsapp.net", "Ana", False, "oi pessoal")])
            rows = s.get_messages_in_range(CHAT, 0, 200)
            assert len(rows) == 1
            assert rows[0].text == "oi pessoal"
        finally:
            s.close()

    def test_upsert_of_same_key_replaces_content_without_duplicating(self, tmp_path):
        s = store(tmp_path)
        try:
            s.upsert_messages([IndexedMessage("m1", CHAT, 100, "ana", "Ana", False, "original")])
            s.upsert_messages([IndexedMessage("m1", CHAT, 100, "ana", "Ana", False, "editado")])

            rows = s.get_messages_in_range(CHAT, 0, 200)
            assert len(rows) == 1
            assert rows[0].text == "editado"
        finally:
            s.close()

    def test_empty_batch_is_a_no_op(self, tmp_path):
        s = store(tmp_path)
        try:
            s.upsert_messages([])
            assert s.get_messages_in_range(CHAT, 0, 1_000_000) == []
        finally:
            s.close()

    def test_rerunning_the_same_batch_is_idempotent(self, tmp_path):
        s = store(tmp_path)
        try:
            rows = [
                IndexedMessage("m1", CHAT, 100, "ana", "Ana", False, "oi"),
                IndexedMessage("m2", CHAT, 200, "ana", "Ana", False, "tudo bem"),
            ]
            s.upsert_messages(rows)
            s.upsert_messages(rows)

            assert len(s.get_messages_in_range(CHAT, 0, 1000)) == 2
        finally:
            s.close()


class TestFtsSync:
    def test_search_finds_inserted_text(self, tmp_path):
        s = store(tmp_path)
        try:
            s.upsert_messages([IndexedMessage("m1", CHAT, 100, "ana", "Ana", False, "vamos pra sampa mes que vem")])
            results = s.search_fts("sampa")
            assert any(r[1] == "m1" for r in results)
        finally:
            s.close()

    def test_accent_insensitive_search(self, tmp_path):
        s = store(tmp_path)
        try:
            s.upsert_messages([IndexedMessage("m1", CHAT, 100, "ana", "Ana", False, "vamos pra São Paulo")])
            results = s.search_fts("sao paulo")
            assert any(r[1] == "m1" for r in results)

            results_accented = s.search_fts("São Paulo")
            assert any(r[1] == "m1" for r in results_accented)
        finally:
            s.close()

    def test_update_is_reflected_in_search(self, tmp_path):
        s = store(tmp_path)
        try:
            s.upsert_messages([IndexedMessage("m1", CHAT, 100, "ana", "Ana", False, "conteudo original")])
            s.upsert_messages([IndexedMessage("m1", CHAT, 100, "ana", "Ana", False, "outra coisa")])

            assert s.search_fts("original") == []
            assert any(r[1] == "m1" for r in s.search_fts("outra"))
        finally:
            s.close()

    def test_search_matches_sender_name_too(self, tmp_path):
        s = store(tmp_path)
        try:
            s.upsert_messages([IndexedMessage("m1", CHAT, 100, "ana", "Bruno Lima", False, "oi")])
            results = s.search_fts("Bruno")
            assert any(r[1] == "m1" for r in results)
        finally:
            s.close()


class TestChunks:
    def test_replace_chunks_inserts_and_deletes(self, tmp_path):
        s = store(tmp_path)
        try:
            chunk = Chunk(
                chunk_id="c1",
                chat_jid=CHAT,
                chat_name="Trabalho 2026",
                is_group=True,
                start_ts=0,
                end_ts=100,
                message_ids=["m1"],
                senders=["Ana"],
                text="[Trabalho 2026 | grupo] 02/03/2026\n09:00 Ana: oi",
                primary_message_ids=["m1"],
            )
            s.upsert_messages([IndexedMessage("m1", CHAT, 0, "ana", "Ana", False, "oi")])
            s.replace_chunks(CHAT, [], [chunk])

            touching = s.get_chunks_touching(CHAT, 0, 100)
            assert [c.chunk_id for c in touching] == ["c1"]

            s.replace_chunks(CHAT, ["c1"], [])
            assert s.get_chunks_touching(CHAT, 0, 100) == []
        finally:
            s.close()

    def test_replace_chunks_updates_message_chunk_id(self, tmp_path):
        s = store(tmp_path)
        try:
            s.upsert_messages([IndexedMessage("m1", CHAT, 0, "ana", "Ana", False, "oi")])
            chunk = Chunk(
                chunk_id="c1",
                chat_jid=CHAT,
                chat_name="Trabalho 2026",
                is_group=True,
                start_ts=0,
                end_ts=0,
                message_ids=["m1"],
                senders=["Ana"],
                text="text",
                primary_message_ids=["m1"],
            )
            s.replace_chunks(CHAT, [], [chunk])

            row = s._conn.execute(
                "SELECT chunk_id FROM messages_idx WHERE chat_jid = ? AND message_id = ?", (CHAT, "m1")
            ).fetchone()
            assert row[0] == "c1"
        finally:
            s.close()
