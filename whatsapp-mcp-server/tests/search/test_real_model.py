"""End-to-end check with the real multilingual-e5-small model.

Skipped by default: it downloads ~470 MB the first time. Run with
``RUN_MODEL_TESTS=1 uv run pytest tests/search/test_real_model.py -v``
(``EMBEDDING_CACHE_DIR`` reuses an existing model cache).
"""

import os

import pytest

from search.embedder import create_embedder
from search.search import SearchService
from tests.search.conftest import GROUP_JID, LOJA_JID, build_index

pytestmark = pytest.mark.skipif(
    not os.getenv("RUN_MODEL_TESTS"), reason="set RUN_MODEL_TESTS=1 to download and run the real model"
)


@pytest.fixture(scope="module")
def embedder():
    return create_embedder(backend="fastembed", model="intfloat/multilingual-e5-small")


def test_trip_question_finds_the_sao_paulo_excerpts_first(search_store, tmp_path, embedder):
    index_path = str(tmp_path / "index.db")
    build_index(search_store, index_path, embedder)
    service = SearchService(index_path, embedder_factory=lambda: embedder)

    result = service.search("quando foi dito algo sobre a viagem de SP e quem falou", chat="Trabalho 2026")

    top_chats = {r["chat"]["jid"] for r in result["results"]}
    assert top_chats == {GROUP_JID}
    texts = [" ".join(m["text"] for m in r["messages"]) for r in result["results"][:3]]
    assert any("viagem pra SP" in t for t in texts[:1])
    assert any("Sao Paulo" in t for t in texts[:3])


def test_who_works_at_the_store_is_in_the_top_results(search_store, tmp_path, embedder):
    index_path = str(tmp_path / "index.db")
    build_index(search_store, index_path, embedder)
    service = SearchService(index_path, embedder_factory=lambda: embedder)

    result = service.search("quem trabalha na loja XXX", limit=5)

    assert result["results"][0]["chat"]["jid"] == LOJA_JID
