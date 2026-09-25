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


def resolve_messages_db_path() -> str:
    """Path to the bridge's messages.db, from the shared store resolution."""
    from lib.utils import MESSAGES_DB_PATH

    return MESSAGES_DB_PATH


def resolve_whatsapp_db_path() -> str:
    """Path to the bridge's whatsapp.db, from the shared store resolution."""
    from lib.utils import WHATSAPP_DB_PATH

    return WHATSAPP_DB_PATH
