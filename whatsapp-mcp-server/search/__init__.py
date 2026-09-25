"""Local hybrid message search: keyword (FTS5) indexing and chunking.

This package builds and maintains ``store/index.db``, a local SQLite index
derived from the bridge's read-only ``messages.db`` / ``whatsapp.db``. No
message content ever leaves the machine. See docs/search.md.
"""
