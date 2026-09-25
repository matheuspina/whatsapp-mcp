"""Tests for the embedding side of the index: embedder wiring, vec_chunks upkeep and the indexer loop."""

import pytest

from search import embedder as embedder_module
from search import indexer
from search.embedder import (
    EmbeddingError,
    FastEmbedEmbedder,
    OllamaEmbedder,
    create_embedder,
    embeddings_enabled,
)
from search.index_store import IndexStore
from search.indexer import embed_pending
from tests.search.conftest import GROUP_JID, FakeEmbedder, build_index, insert_message, ts


def vector_count(store: IndexStore) -> int:
    return store._conn.execute("SELECT COUNT(*) FROM vec_chunks").fetchone()[0]


def vector_rowids(store: IndexStore) -> set[int]:
    return {r[0] for r in store._conn.execute("SELECT chunk_id FROM vec_chunks")}


def chunk_rowids(store: IndexStore) -> set[int]:
    return {r[0] for r in store._conn.execute("SELECT rowid FROM chunks")}


@pytest.fixture
def index_path(tmp_path) -> str:
    return str(tmp_path / "index.db")


class TestVectorStore:
    def test_every_chunk_gets_one_vector(self, search_store, index_path):
        build_index(search_store, index_path, FakeEmbedder())
        store = IndexStore(index_path)
        try:
            assert store.count_chunks(embedded=False) == 0
            assert vector_rowids(store) == chunk_rowids(store)
            assert store.get_meta("embedding_model") == "fake-concepts"
        finally:
            store.close()

    def test_model_change_rebuilds_vectors_and_requeues_chunks(self, search_store, index_path):
        build_index(search_store, index_path, FakeEmbedder())
        store = IndexStore(index_path)
        try:
            bigger = FakeEmbedder(name="other", concepts={"a": {"x"}, "b": {"y"}, "c": {"z"}, "d": {"w"}})
            assert store.ensure_vec_table(bigger.name, bigger.dims) is True
            assert vector_count(store) == 0
            assert store.count_chunks(embedded=False) == store.count_chunks()
            assert store.get_meta("embedding_model") == "other"
            assert store.get_meta("embedding_dims") == str(bigger.dims)

            while embed_pending(store, bigger, batch_size=4):
                pass
            assert vector_rowids(store) == chunk_rowids(store)
            # Same model again: nothing to do.
            assert store.ensure_vec_table(bigger.name, bigger.dims) is False
        finally:
            store.close()

    def test_rebuilt_chunk_loses_its_stale_vector_until_re_embedded(self, search_store, index_path):
        embedder = FakeEmbedder()
        build_index(search_store, index_path, embedder)
        insert_message(
            search_store.messages_db_path,
            message_id="T99",
            chat_jid=GROUP_JID,
            sender="5511900000001@s.whatsapp.net",
            content="mais uma sobre a viagem",
            timestamp=ts(70),  # lands inside the "viagem" window, so that chunk is rebuilt
        )
        from search import source
        from search.indexer import run_once

        store = IndexStore(index_path)
        try:
            before = vector_count(store)
            with source.ContactResolver(search_store.whatsapp_db_path) as resolver:
                run_once(store, search_store.messages_db_path, resolver)
            pending = store.count_chunks(embedded=False)
            assert pending >= 1
            assert vector_count(store) == before - pending  # stale vectors dropped with the rebuild

            while embed_pending(store, embedder, batch_size=4):
                pass
            assert vector_rowids(store) == chunk_rowids(store)
        finally:
            store.close()

    def test_deleted_chunks_do_not_leave_orphan_vectors(self, search_store, index_path):
        build_index(search_store, index_path, FakeEmbedder())
        store = IndexStore(index_path)
        try:
            chunk_id = store._conn.execute("SELECT chunk_id FROM chunks LIMIT 1").fetchone()[0]
            store.replace_chunks(GROUP_JID, [chunk_id], [])
            assert vector_rowids(store) <= chunk_rowids(store)
        finally:
            store.close()

    def test_without_sqlite_vec_keyword_indexing_still_works(self, search_store, index_path, monkeypatch):
        import search.index_store as index_store

        monkeypatch.setattr(index_store, "load_vec_extension", lambda conn: False)
        build_index(search_store, index_path, embedder=None)
        store = IndexStore(index_path)
        try:
            assert store.vec_available is False
            assert store.count_chunks() > 0
            with pytest.raises(RuntimeError, match="sqlite-vec"):
                store.ensure_vec_table("m", 4)
        finally:
            store.close()


class TestEmbedPending:
    def test_embeds_in_batches_until_done(self, search_store, index_path):
        embedder = FakeEmbedder()
        build_index(search_store, index_path)
        store = IndexStore(index_path)
        try:
            store.ensure_vec_table(embedder.name, embedder.dims)
            total = store.count_chunks()
            assert embed_pending(store, embedder, batch_size=2) == 2
            assert store.count_chunks(embedded=False) == total - 2
            while embed_pending(store, embedder, batch_size=2):
                pass
            assert store.count_chunks(embedded=False) == 0
            assert embed_pending(store, embedder) == 0
        finally:
            store.close()


class TestLoadEmbedder:
    def test_unavailable_embedder_is_not_fatal(self, index_path, monkeypatch):
        def boom():
            raise EmbeddingError("no model")

        monkeypatch.setattr(indexer, "create_embedder", boom)
        store = IndexStore(index_path)
        try:
            assert indexer._load_embedder(store) is None
        finally:
            store.close()

    def test_loaded_embedder_prepares_the_vector_table(self, index_path, monkeypatch):
        monkeypatch.setattr(indexer, "create_embedder", lambda: FakeEmbedder())
        store = IndexStore(index_path)
        try:
            assert indexer._load_embedder(store) is not None
            assert store.has_vec_table()
        finally:
            store.close()


class TestEmbedders:
    def test_backend_switch(self):
        assert embeddings_enabled("fastembed")
        assert not embeddings_enabled("none")
        assert not embeddings_enabled("OFF")
        with pytest.raises(EmbeddingError, match="unknown EMBEDDING_BACKEND"):
            create_embedder(backend="openai")

    def test_fastembed_rejects_unsupported_model_before_downloading(self):
        with pytest.raises(EmbeddingError, match="does not support model"):
            FastEmbedEmbedder("acme/not-a-real-model")

    def test_e5_models_get_query_and_passage_prefixes(self):
        prefixed = embedder_module._PrefixMixin()
        prefixed.model = "intfloat/multilingual-e5-small"
        assert prefixed._passage("oi") == "passage: oi"
        assert prefixed._query("oi") == "query: oi"
        plain = embedder_module._PrefixMixin()
        plain.model = "BAAI/bge-m3"
        assert plain._passage("oi") == "oi" and plain._query("oi") == "oi"

    def test_ollama_embedder_normalises_and_reports_dims(self, monkeypatch):
        import httpx

        calls = []

        class Response:
            def raise_for_status(self):
                pass

            def json(self):
                return {"embeddings": [[3.0, 4.0]] * len(calls[-1]["input"])}

        def fake_post(url, json, timeout):
            calls.append({"url": url, **json})
            return Response()

        monkeypatch.setattr(httpx, "post", fake_post)
        embedder = OllamaEmbedder("bge-m3", "http://ollama.local:11434/")
        assert embedder.dims == 2
        assert embedder.embed_query("oi") == pytest.approx([0.6, 0.8])
        assert calls[-1]["url"] == "http://ollama.local:11434/api/embed"

    def test_ollama_unreachable_raises_embedding_error(self, monkeypatch):
        import httpx

        def fail(*_a, **_k):
            raise httpx.ConnectError("refused")

        monkeypatch.setattr(httpx, "post", fail)
        with pytest.raises(EmbeddingError, match="Ollama"):
            OllamaEmbedder("bge-m3", "http://nowhere:1")


class _StopLoop(Exception):
    pass


class TestRunForever:
    def _run(self, store, index_path, monkeypatch, sleeps_allowed=1):
        sleeps = []

        def fake_sleep(seconds):
            sleeps.append(seconds)
            if len(sleeps) > sleeps_allowed:
                raise _StopLoop

        monkeypatch.setattr(indexer.time, "sleep", fake_sleep)
        with pytest.raises(_StopLoop):
            indexer.run_forever(store.messages_db_path, store.whatsapp_db_path, index_path, poll_seconds=1)
        return sleeps

    def test_indexes_messages_then_embeds_all_chunks_before_sleeping(self, search_store, index_path, monkeypatch):
        embedder = FakeEmbedder()
        monkeypatch.setattr(indexer, "create_embedder", lambda: embedder)
        monkeypatch.setattr(indexer, "embed_pending", lambda s, e, batch_size=2: embed_pending(s, e, 2))

        self._run(search_store, index_path, monkeypatch)

        store = IndexStore(index_path)
        try:
            assert store.count_chunks() > 0
            assert store.count_chunks(embedded=False) == 0
            assert vector_rowids(store) == chunk_rowids(store)
            assert embedder.passage_calls >= 2  # several batches, not one giant call
        finally:
            store.close()

    def test_keyword_indexing_survives_an_unavailable_embedder(self, search_store, index_path, monkeypatch):
        def boom():
            raise EmbeddingError("offline")

        monkeypatch.setattr(indexer, "create_embedder", boom)
        self._run(search_store, index_path, monkeypatch)

        store = IndexStore(index_path)
        try:
            assert store.count_chunks() > 0
            assert not store.has_vec_table()
        finally:
            store.close()

    def test_backend_none_never_loads_a_model(self, search_store, index_path, monkeypatch):
        monkeypatch.setattr(indexer, "embeddings_enabled", lambda: False)
        monkeypatch.setattr(indexer, "create_embedder", lambda: pytest.fail("embedder must not be created"))
        self._run(search_store, index_path, monkeypatch)

    def test_a_failing_batch_is_retried_not_fatal(self, search_store, index_path, monkeypatch):
        class Flaky(FakeEmbedder):
            def embed_passages(self, texts):
                self.passage_calls += 1
                if self.passage_calls == 1:
                    raise RuntimeError("transient")
                return super().embed_passages(texts)

        embedder = Flaky()
        monkeypatch.setattr(indexer, "create_embedder", lambda: embedder)
        self._run(search_store, index_path, monkeypatch, sleeps_allowed=3)

        store = IndexStore(index_path)
        try:
            assert store.count_chunks(embedded=False) == 0
        finally:
            store.close()
