"""Environment configuration for the local search indexer.

Only the variables documented in docs/search.md are read here. The source
databases reuse the bridge store resolution in ``lib.utils`` (``WA_STORE_PATH``);
the index itself is written to ``INDEX_DB_PATH``.
"""

import os


def _int_env(name: str, default: int) -> int:
    value = os.getenv(name)
    if not value:
        return default
    return int(value)


INDEX_DB_PATH = os.getenv("INDEX_DB_PATH", "store/index.db")
INDEX_POLL_SECONDS = _int_env("INDEX_POLL_SECONDS", 20)

CHUNK_GAP_MINUTES = _int_env("CHUNK_GAP_MINUTES", 30)
CHUNK_MAX_MESSAGES = _int_env("CHUNK_MAX_MESSAGES", 15)
CHUNK_MAX_CHARS = _int_env("CHUNK_MAX_CHARS", 1500)
CHUNK_OVERLAP = _int_env("CHUNK_OVERLAP", 2)

# Embeddings are computed locally. "none" turns the semantic side off and keeps keyword search only.
EMBEDDING_BACKEND = os.getenv("EMBEDDING_BACKEND", "fastembed").strip().lower()
EMBEDDING_MODEL = os.getenv("EMBEDDING_MODEL", "intfloat/multilingual-e5-small")
EMBED_BATCH_SIZE = _int_env("EMBED_BATCH_SIZE", 32)
OLLAMA_URL = os.getenv("OLLAMA_URL", "http://host.docker.internal:11434")
EMBEDDING_CACHE_DIR = os.getenv("EMBEDDING_CACHE_DIR") or None

# Candidates taken from each retrieval path before rank fusion.
SEARCH_K_FTS = _int_env("SEARCH_K_FTS", 50)
SEARCH_K_VEC = _int_env("SEARCH_K_VEC", 50)
# Semantic matches below this cosine similarity are dropped (0 keeps everything the model returns).
SEARCH_MIN_SIMILARITY = float(os.getenv("SEARCH_MIN_SIMILARITY") or 0.0)

# Timezone used to read date filters and to print dates in search results.
DISPLAY_TZ = os.getenv("DISPLAY_TZ", "America/Bahia")


def resolve_messages_db_path() -> str:
    """Path to the bridge's messages.db, from the shared store resolution."""
    from lib.utils import MESSAGES_DB_PATH

    return MESSAGES_DB_PATH


def resolve_whatsapp_db_path() -> str:
    """Path to the bridge's whatsapp.db, from the shared store resolution."""
    from lib.utils import WHATSAPP_DB_PATH

    return WHATSAPP_DB_PATH
