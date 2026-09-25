"""Local text embedding behind a small provider interface.

Nothing here talks to a hosted API: ``fastembed`` and ``sentence_transformers``
run in-process, and ``ollama`` talks to a server the user runs themselves. The
model name and vector size are stored next to the index (see IndexStore), so
switching backend or model is detected and re-embeds everything.
"""

import math
from typing import Any, Protocol

from lib.utils import logger

from . import config


class EmbeddingError(RuntimeError):
    """The embedding backend could not be loaded or failed to embed."""


class Embedder(Protocol):
    """Turns text into L2-normalised vectors so cosine distance is meaningful."""

    name: str
    dims: int

    def embed_passages(self, texts: list[str]) -> list[list[float]]: ...

    def embed_query(self, text: str) -> list[float]: ...


def _is_e5(model: str) -> bool:
    return "e5" in model.lower().split("/")[-1]


def _normalize(vector: list[float]) -> list[float]:
    norm = math.sqrt(sum(v * v for v in vector))
    if norm == 0:
        return vector
    return [v / norm for v in vector]


class _PrefixMixin:
    """e5 models are trained with ``query: `` / ``passage: `` prefixes."""

    model: str

    def _passage(self, text: str) -> str:
        return f"passage: {text}" if _is_e5(self.model) else text

    def _query(self, text: str) -> str:
        return f"query: {text}" if _is_e5(self.model) else text


# Models fastembed does not list but that ship an ONNX export on Hugging Face.
_FASTEMBED_CUSTOM_MODELS: dict[str, dict[str, Any]] = {
    "intfloat/multilingual-e5-small": {"dim": 384, "model_file": "onnx/model.onnx"},
    "intfloat/multilingual-e5-base": {"dim": 768, "model_file": "onnx/model.onnx"},
}


class FastEmbedEmbedder(_PrefixMixin):
    """ONNX inference through fastembed: no torch, small image."""

    def __init__(self, model: str, cache_dir: str | None = None):
        try:
            from fastembed import TextEmbedding
            from fastembed.common.model_description import ModelSource, PoolingType
        except ImportError as exc:
            raise EmbeddingError("EMBEDDING_BACKEND=fastembed needs the 'fastembed' package installed") from exc

        self.model = model
        self.name = model
        try:
            supported = {m["model"] for m in TextEmbedding.list_supported_models()}
            if model not in supported:
                custom = _FASTEMBED_CUSTOM_MODELS.get(model)
                if custom is None:
                    raise EmbeddingError(
                        f"fastembed does not support model '{model}'. Use one of the models it lists, "
                        f"{', '.join(sorted(_FASTEMBED_CUSTOM_MODELS))}, or another EMBEDDING_BACKEND"
                    )
                TextEmbedding.add_custom_model(
                    model=model,
                    pooling=PoolingType.MEAN,
                    normalization=True,
                    sources=ModelSource(hf=model),
                    dim=custom["dim"],
                    model_file=custom["model_file"],
                )
            self._model = TextEmbedding(model, cache_dir=cache_dir)
            self.dims = len(next(iter(self._model.embed(["dimension probe"]))))
        except EmbeddingError:
            raise
        except Exception as exc:
            raise EmbeddingError(f"could not load embedding model '{model}': {exc}") from exc

    def _embed(self, texts: list[str]) -> list[list[float]]:
        return [_normalize([float(x) for x in vec]) for vec in self._model.embed(texts)]

    def embed_passages(self, texts: list[str]) -> list[list[float]]:
        return self._embed([self._passage(t) for t in texts])

    def embed_query(self, text: str) -> list[float]:
        return self._embed([self._query(text)])[0]


class SentenceTransformersEmbedder(_PrefixMixin):
    """PyTorch inference. Optional: install torch (CPU) and sentence-transformers yourself."""

    def __init__(self, model: str, cache_dir: str | None = None):
        try:
            from sentence_transformers import SentenceTransformer
        except ImportError as exc:
            raise EmbeddingError(
                "EMBEDDING_BACKEND=sentence_transformers needs the 'sentence-transformers' package installed"
            ) from exc

        self.model = model
        self.name = model
        try:
            self._model = SentenceTransformer(model, cache_folder=cache_dir)
            self.dims = int(self._model.get_sentence_embedding_dimension())
        except Exception as exc:
            raise EmbeddingError(f"could not load embedding model '{model}': {exc}") from exc

    def _embed(self, texts: list[str]) -> list[list[float]]:
        vectors = self._model.encode(texts, normalize_embeddings=True, show_progress_bar=False)
        return [[float(x) for x in vec] for vec in vectors]

    def embed_passages(self, texts: list[str]) -> list[list[float]]:
        return self._embed([self._passage(t) for t in texts])

    def embed_query(self, text: str) -> list[float]:
        return self._embed([self._query(text)])[0]


class OllamaEmbedder(_PrefixMixin):
    """Embeddings from an Ollama server the user runs (for example ``bge-m3``)."""

    def __init__(self, model: str, url: str):
        self.model = model
        self.name = model
        self._url = url.rstrip("/")
        try:
            self.dims = len(self._request(["dimension probe"])[0])
        except Exception as exc:
            raise EmbeddingError(f"could not reach Ollama at {self._url} for model '{model}': {exc}") from exc

    def _request(self, texts: list[str]) -> list[list[float]]:
        import httpx

        response = httpx.post(f"{self._url}/api/embed", json={"model": self.model, "input": texts}, timeout=120)
        response.raise_for_status()
        return [_normalize(vec) for vec in response.json()["embeddings"]]

    def embed_passages(self, texts: list[str]) -> list[list[float]]:
        return self._request([self._passage(t) for t in texts])

    def embed_query(self, text: str) -> list[float]:
        return self._request([self._query(text)])[0]


def create_embedder(
    backend: str | None = None,
    model: str | None = None,
    cache_dir: str | None = None,
    ollama_url: str | None = None,
) -> Embedder:
    """Build the configured embedder; raises EmbeddingError if it cannot be used."""
    backend = (backend or config.EMBEDDING_BACKEND).lower()
    model = model or config.EMBEDDING_MODEL
    cache_dir = cache_dir or config.EMBEDDING_CACHE_DIR
    logger.info("search embeddings: loading backend=%s model=%s", backend, model)

    if backend == "fastembed":
        return FastEmbedEmbedder(model, cache_dir)
    if backend == "sentence_transformers":
        return SentenceTransformersEmbedder(model, cache_dir)
    if backend == "ollama":
        return OllamaEmbedder(model, ollama_url or config.OLLAMA_URL)
    raise EmbeddingError(f"unknown EMBEDDING_BACKEND '{backend}': use fastembed, sentence_transformers, ollama or none")


def embeddings_enabled(backend: str | None = None) -> bool:
    """Whether the semantic side is on. ``EMBEDDING_BACKEND=none`` keeps keyword search only."""
    return (backend or config.EMBEDDING_BACKEND).lower() not in ("none", "off", "")
