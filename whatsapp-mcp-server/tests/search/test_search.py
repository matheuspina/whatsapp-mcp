"""Tests for search/search.py against a synthetic Portuguese index (no model download)."""

import pytest

from search.index_store import IndexStore
from search.search import SearchError, SearchService, build_fts_query
from tests.search.conftest import (
    ANA_JID,
    GROUP_JID,
    LOJA_JID,
    FakeEmbedder,
    build_index,
)


@pytest.fixture
def index_path(tmp_path) -> str:
    return str(tmp_path / "index.db")


@pytest.fixture
def built(search_store, index_path):
    embedder = FakeEmbedder()
    build_index(search_store, index_path, embedder)
    return search_store, index_path, embedder


def make_service(store, index_path, embedder=None) -> SearchService:
    factory = (lambda: embedder) if embedder is not None else None
    return SearchService(index_path, embedder_factory=factory, messages_db_path=store.messages_db_path)


class TestBuildFtsQuery:
    def test_quotes_terms_and_prefixes_long_ones(self):
        assert build_fts_query("viagem SP") == 'text : ("viagem"* OR "SP")'

    def test_operators_and_quotes_are_neutralised(self):
        query = build_fts_query('foo" OR NOT (bar) NEAR/2 *')
        assert query is not None
        assert '"OR"' in query  # a quoted word, not an operator
        assert "(bar)" not in query

    def test_drops_stopwords_but_falls_back_when_only_stopwords(self):
        assert build_fts_query("quando foi dito sobre a viagem de SP") == 'text : ("viagem"* OR "SP")'
        assert build_fts_query("de a o") == 'text : ("de" OR "a" OR "o")'

    def test_nothing_searchable(self):
        assert build_fts_query("  ?! ") is None


class TestKeywordSearch:
    def test_finds_message_with_author_and_bahia_time(self, built):
        store, index_path, _ = built
        result = make_service(store, index_path).search("viagem SP", mode="keyword")

        top = result["results"][0]
        assert top["chat"]["name"] == "Trabalho 2026"
        assert top["chat"]["is_group"] is True
        assert top["matched_by"] == ["keyword"]
        hit = next(m for m in top["messages"] if m["keyword_hit"])
        assert hit["sender"] == "Ana Souza"
        assert hit["ts"] == "2026-03-02T07:00:00-03:00"  # 10:00 UTC shown in America/Bahia
        assert "viagem" in hit["text"]

    def test_accent_and_case_insensitive(self, built):
        store, index_path, _ = built
        result = make_service(store, index_path).search("CAFE cenoura", mode="keyword")
        assert result["results"][0]["chat"]["jid"] == LOJA_JID

    def test_no_match_returns_empty_results_with_coverage(self, built):
        store, index_path, _ = built
        result = make_service(store, index_path).search("zzzinexistente", mode="keyword")
        assert result["results"] == []
        assert result["coverage"]["oldest_indexed"] == "2026-03-02"
        assert "limited to the history" in result["coverage"]["note"]

    def test_empty_query_is_an_error(self, built):
        store, index_path, _ = built
        with pytest.raises(SearchError):
            make_service(store, index_path).search("   ")


class TestFilters:
    def test_chat_by_partial_accent_insensitive_name(self, built):
        store, index_path, _ = built
        result = make_service(store, index_path).search("loja", chat="loja CENTRO", mode="keyword")
        assert {r["chat"]["jid"] for r in result["results"]} == {LOJA_JID}
        assert result["filters"]["chat"]["name"] == "Loja Centro"

    def test_chat_by_jid(self, built):
        store, index_path, _ = built
        result = make_service(store, index_path).search("bom dia", chat=GROUP_JID, mode="keyword")
        assert result["results"] and all(r["chat"]["jid"] == GROUP_JID for r in result["results"])

    def test_ambiguous_chat_lists_candidates(self, built):
        store, index_path, _ = built
        # "o" appears in both group names: ambiguous, and the error names both with their JIDs.
        with pytest.raises(SearchError) as exc:
            make_service(store, index_path).search("bom", chat="o", mode="keyword")
        message = str(exc.value)
        assert "Trabalho 2026" in message and "Loja Centro" in message and LOJA_JID in message

    def test_unknown_chat(self, built):
        store, index_path, _ = built
        with pytest.raises(SearchError, match="No indexed chat"):
            make_service(store, index_path).search("x", chat="Familia", mode="keyword")

    def test_sender_by_name(self, built):
        store, index_path, _ = built
        result = make_service(store, index_path).search("loja", sender="carla", mode="keyword")
        hits = [m for r in result["results"] for m in r["messages"] if m["keyword_hit"]]
        assert hits and all(m["sender"] == "Carla Nunes" for m in hits)
        assert result["filters"]["sender_matched"] == ["Carla Nunes"]

    def test_sender_by_phone(self, built):
        store, index_path, _ = built
        result = make_service(store, index_path).search("ok", sender="5511900000001", mode="keyword")
        hits = [m for r in result["results"] for m in r["messages"] if m["keyword_hit"]]
        assert hits and all(m["sender"] == "Ana Souza" for m in hits)

    def test_unknown_sender(self, built):
        store, index_path, _ = built
        with pytest.raises(SearchError, match="No indexed sender"):
            make_service(store, index_path).search("ok", sender="Zelda", mode="keyword")

    def test_date_bounds_use_bahia_time_and_are_inclusive(self, built):
        store, index_path, _ = built
        service = make_service(store, index_path)
        # T05 is 2026-03-05 14:00 UTC = 11:00 in Bahia.
        assert service.search("hotel", date_from="2026-03-05", date_to="2026-03-05", mode="keyword")["results"]
        assert not service.search("hotel", date_from="2026-03-06", mode="keyword")["results"]
        assert not service.search("hotel", date_to="2026-03-04", mode="keyword")["results"]

    def test_bad_date_and_reversed_range(self, built):
        store, index_path, _ = built
        service = make_service(store, index_path)
        with pytest.raises(SearchError, match="date_from"):
            service.search("x", date_from="ontem")
        with pytest.raises(SearchError, match="after"):
            service.search("x", date_from="2026-03-05", date_to="2026-03-01")


class TestSemanticAndHybrid:
    def test_semantic_finds_text_without_shared_words(self, built):
        store, index_path, embedder = built
        service = make_service(store, index_path, embedder)
        result = service.search("viagem de SP", chat="Trabalho 2026", mode="semantic")

        assert result["results"], "semantic search returned nothing"
        assert all("semantic" in r["matched_by"] for r in result["results"])
        texts = " ".join(m["text"] for r in result["results"][:3] for m in r["messages"])
        assert "sampa" in texts and "Sao Paulo" in texts
        assert result["results"][0]["similarity"] > 0.5

    def test_hybrid_ranks_chunk_matched_by_both_first(self, built):
        store, index_path, embedder = built
        result = make_service(store, index_path, embedder).search("viagem de SP")

        top = result["results"][0]
        assert top["matched_by"] == ["keyword", "semantic"]
        assert any(m["keyword_hit"] and "viagem" in m["text"] for m in top["messages"])
        # The follow-up ("hotel em Sao Paulo") has no keyword overlap but is found by meaning.
        assert any(
            "Sao Paulo" in m["text"]
            for r in result["results"]
            if r["matched_by"] == ["semantic"]
            for m in r["messages"]
        )

    def test_open_question_finds_store_staff(self, built):
        store, index_path, embedder = built
        result = make_service(store, index_path, embedder).search("quem trabalha na loja XXX", limit=5)
        assert result["results"][0]["chat"]["jid"] == LOJA_JID

    def test_hybrid_falls_back_to_keyword_without_embedder(self, built):
        store, index_path, _ = built
        result = make_service(store, index_path, None).search("viagem")
        assert result["results"]
        assert "Semantic search unavailable" in result["coverage"]["note"]

    def test_semantic_mode_without_embedder_is_an_error(self, built):
        store, index_path, _ = built
        with pytest.raises(SearchError, match="Semantic search is not available"):
            make_service(store, index_path, None).search("viagem", mode="semantic")

    def test_index_without_vectors_falls_back_and_semantic_mode_errors(self, search_store, index_path):
        build_index(search_store, index_path, embedder=None)
        service = make_service(search_store, index_path, FakeEmbedder())
        assert service.search("viagem")["results"]
        assert "no embeddings yet" in service.search("viagem")["coverage"]["note"]
        with pytest.raises(SearchError, match="no embeddings yet"):
            service.search("viagem", mode="semantic")

    def test_model_mismatch_is_reported(self, built):
        store, index_path, _ = built
        other = FakeEmbedder(name="another-model")
        with pytest.raises(SearchError, match="another-model"):
            make_service(store, index_path, other).search("viagem", mode="semantic")

    def test_semantic_respects_chat_sender_and_date_filters(self, built):
        store, index_path, embedder = built
        service = make_service(store, index_path, embedder)

        only_loja = service.search("viagem", chat=LOJA_JID, mode="semantic")
        assert all(r["chat"]["jid"] == LOJA_JID for r in only_loja["results"])

        only_bruno = service.search("loja", sender="Bruno", mode="semantic")
        assert only_bruno["results"]
        assert all("Bruno Lima" in {m["sender"] for m in r["messages"]} for r in only_bruno["results"])

        late = service.search("viagem", date_from="2026-03-05", mode="semantic")
        assert all(r["period"]["end"] >= "2026-03-05" for r in late["results"])

    def test_pending_embeddings_are_reported(self, search_store, index_path):
        from search.index_store import IndexStore

        embedder = FakeEmbedder()
        build_index(search_store, index_path, embedder)
        index = IndexStore(index_path)
        index._conn.execute("UPDATE chunks SET embedded = 0 WHERE rowid = 1")
        index._conn.commit()
        index.close()

        note = make_service(search_store, index_path, embedder).search("viagem")["coverage"]["note"]
        assert "waiting for embeddings" in note


class TestResultShape:
    def test_long_messages_are_truncated(self, search_store, index_path):
        from tests.search.conftest import insert_message, ts

        insert_message(
            search_store.messages_db_path,
            message_id="LONG1",
            chat_jid=GROUP_JID,
            sender=ANA_JID,
            content="pizza " + "x" * 2000,
            timestamp=ts(10 * 24 * 60),
        )
        build_index(search_store, index_path)
        result = make_service(search_store, index_path).search("pizza", mode="keyword")
        text = next(m["text"] for m in result["results"][0]["messages"] if m["keyword_hit"])
        assert len(text) == 500 and text.endswith("…")

    def test_limit_is_clamped(self, built):
        store, index_path, _ = built
        service = make_service(store, index_path)
        assert len(service.search("bom dia OR loja OR viagem OR bolo", mode="keyword", limit=1)["results"]) == 1
        assert len(service.search("bom", mode="keyword", limit=999)["results"]) <= 30

    def test_missing_index_explains_what_to_do(self, tmp_path):
        service = SearchService(str(tmp_path / "nope.db"))
        with pytest.raises(SearchError, match="indexer"):
            service.search("x")


class TestStatus:
    def test_reports_counts_model_and_chat(self, built):
        store, index_path, embedder = built
        status = make_service(store, index_path, embedder).status()

        assert status["index"]["messages_indexed"] == 11
        assert status["index"]["chunks_awaiting_embedding"] == 0
        assert status["index"]["oldest"] == "2026-03-02"
        assert status["embedding"] == {"model": "fake-concepts", "dims": embedder.dims, "semantic_search_ready": True}
        assert status["source"]["messages_not_yet_indexed"] == 0
        assert status["index"]["last_run_at"]

        chat_status = make_service(store, index_path, embedder).status(chat="Loja Centro")["chat"]
        assert chat_status["jid"] == LOJA_JID and chat_status["messages_indexed"] == 3

    def test_status_of_index_without_embeddings(self, search_store, index_path):
        build_index(search_store, index_path, embedder=None)
        status = make_service(search_store, index_path).status()
        assert status["embedding"]["semantic_search_ready"] is False
        assert status["index"]["chunks_awaiting_embedding"] == status["index"]["chunks"]
        assert IndexStore(index_path).count_chunks() == status["index"]["chunks"]
