"""End-to-end tests for search/indexer.py against the synthetic store."""

from search import source
from search.index_store import IndexStore
from search.indexer import run_once
from tests.search.conftest import ANA_JID, GROUP_JID, insert_message, ts


def make_index_store(tmp_path) -> IndexStore:
    return IndexStore(str(tmp_path / "index.db"))


class TestRunOnce:
    def test_indexes_all_messages_on_first_run(self, synthetic_store, tmp_path):
        store = make_index_store(tmp_path)
        try:
            with source.ContactResolver(synthetic_store.whatsapp_db_path) as resolver:
                processed = run_once(store, synthetic_store.messages_db_path, resolver)

            assert processed == 10
            rows = store.get_messages_in_range(GROUP_JID, 0, 10_000_000_000)
            assert len(rows) == 8  # 8 of the 10 fixture messages are in the group chat
        finally:
            store.close()

    def test_resolves_names_and_media_markers(self, synthetic_store, tmp_path):
        store = make_index_store(tmp_path)
        try:
            with source.ContactResolver(synthetic_store.whatsapp_db_path) as resolver:
                run_once(store, synthetic_store.messages_db_path, resolver)

            rows = store.get_messages_in_range(GROUP_JID, 0, 10_000_000_000)
            by_id = {}
            for r in rows:
                by_id[r.message_id] = r
            assert by_id["MSG0001"].sender_name == "Ana Souza"
            assert by_id["MSG0006"].text == "[imagem]"  # captionless image marker
        finally:
            store.close()

    def test_builds_chunks_for_touched_chats(self, synthetic_store, tmp_path):
        store = make_index_store(tmp_path)
        try:
            with source.ContactResolver(synthetic_store.whatsapp_db_path) as resolver:
                run_once(store, synthetic_store.messages_db_path, resolver)

            chunks = store.get_chunks_touching(GROUP_JID, 0, 10_000_000_000)
            assert len(chunks) >= 1
        finally:
            store.close()

    def test_cursor_advances_and_resumes(self, synthetic_store, tmp_path):
        store = make_index_store(tmp_path)
        try:
            with source.ContactResolver(synthetic_store.whatsapp_db_path) as resolver:
                first = run_once(store, synthetic_store.messages_db_path, resolver, limit=4)
                cursor_after_first = store.get_cursor()
                assert first == 4
                assert cursor_after_first > 0

                second = run_once(store, synthetic_store.messages_db_path, resolver, limit=4)
                assert second == 4
                assert store.get_cursor() > cursor_after_first
        finally:
            store.close()

    def test_no_new_messages_returns_zero_and_updates_last_run(self, synthetic_store, tmp_path):
        store = make_index_store(tmp_path)
        try:
            with source.ContactResolver(synthetic_store.whatsapp_db_path) as resolver:
                run_once(store, synthetic_store.messages_db_path, resolver)
                processed = run_once(store, synthetic_store.messages_db_path, resolver)

            assert processed == 0
            assert store.get_meta("last_run_at") is not None
        finally:
            store.close()

    def test_rerunning_the_same_state_is_idempotent(self, synthetic_store, tmp_path):
        store = make_index_store(tmp_path)
        try:
            with source.ContactResolver(synthetic_store.whatsapp_db_path) as resolver:
                run_once(store, synthetic_store.messages_db_path, resolver)
                rows_after_first = store.get_messages_in_range(GROUP_JID, 0, 10_000_000_000)

                # Simulate a restart mid-way: rewind the cursor and reprocess.
                store.set_cursor(0)
                run_once(store, synthetic_store.messages_db_path, resolver)
                rows_after_replay = store.get_messages_in_range(GROUP_JID, 0, 10_000_000_000)

            assert len(rows_after_first) == len(rows_after_replay)
        finally:
            store.close()

    def test_replaced_row_updates_content_and_rebuilds_chunk(self, synthetic_store, tmp_path):
        store = make_index_store(tmp_path)
        try:
            with source.ContactResolver(synthetic_store.whatsapp_db_path) as resolver:
                run_once(store, synthetic_store.messages_db_path, resolver)

                insert_message(
                    synthetic_store.messages_db_path,
                    message_id="MSG0001",
                    chat_jid=GROUP_JID,
                    sender=ANA_JID,
                    content="bom dia pessoal (editado)",
                    timestamp=ts(0),
                )
                processed = run_once(store, synthetic_store.messages_db_path, resolver)

            assert processed == 1  # only the replaced row is new by rowid
            rows = store.get_messages_in_range(GROUP_JID, 0, 10_000_000_000)
            matching = [r for r in rows if r.message_id == "MSG0001"]
            assert len(matching) == 1
            assert matching[0].text == "bom dia pessoal (editado)"

            chunk_rows = store.get_chunks_touching(GROUP_JID, 0, 10_000_000_000)
            chunk_texts = [
                store._conn.execute("SELECT text FROM chunks WHERE chunk_id = ?", (c.chunk_id,)).fetchone()[0]
                for c in chunk_rows
            ]
            assert any("bom dia pessoal (editado)" in t for t in chunk_texts)
        finally:
            store.close()

    def test_out_of_order_late_message_is_indexed_into_the_past(self, synthetic_store, tmp_path):
        store = make_index_store(tmp_path)
        try:
            with source.ContactResolver(synthetic_store.whatsapp_db_path) as resolver:
                run_once(store, synthetic_store.messages_db_path, resolver)

                # A history-sync message with an old timestamp, inserted after live traffic.
                insert_message(
                    synthetic_store.messages_db_path,
                    message_id="MSG0000",
                    chat_jid=GROUP_JID,
                    sender=ANA_JID,
                    content="mensagem antiga chegando atrasada",
                    timestamp=ts(0.5),  # between MSG0001 (ts 0) and MSG0002 (ts 1)
                )
                run_once(store, synthetic_store.messages_db_path, resolver)

            rows = store.get_messages_in_range(GROUP_JID, 0, 10_000_000_000)
            assert any(r.message_id == "MSG0000" for r in rows)
        finally:
            store.close()
