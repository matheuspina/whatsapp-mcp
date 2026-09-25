# Local Search (Phase 1: keyword index)

A background process turns your message history into a local, searchable index: exact-term (keyword)
search today, with local semantic search and MCP tools to follow in later phases. No message content
ever leaves your machine -- the indexer only reads the bridge's SQLite databases and writes to a new,
separate SQLite database (`store-index/index.db` in Docker).

This document covers what is built (Phase 1). See [ROADMAP.md](../ROADMAP.md) for what's next.

## What it does

```
store/messages.db  ──read-only──▶  indexer  ──writes──▶  store-index/index.db
store/whatsapp.db  ──read-only──▶     │
                                       ▼
                              store-index/index.db:
                              - messages_idx  (resolved text, FTS5-searchable)
                              - chunks        (conversation windows, for Phase 2 embeddings)
```

A single background loop (`whatsapp-mcp-server/search/indexer.py`) polls `messages.db` for new or
changed rows, resolves display names, and keeps two things up to date in `index.db`:

- **`messages_idx`**, a keyword-searchable copy of each message's text (SQLite FTS5, accent-insensitive),
  with the sender and chat resolved to a display name.
- **`chunks`**, overlapping windows of consecutive messages per chat, formatted as short transcripts.
  Chunking exists now so Phase 2 can embed each chunk without re-deriving conversation boundaries; chunk
  text is not yet searchable on its own.

Indexing is **incremental and resumable**: progress is a single cursor (the source database's `rowid`,
not a timestamp, since WhatsApp's history sync can insert old messages long after newer ones) stored in
`index.db`. A restart picks up exactly where it left off, and reprocessing the same messages twice never
duplicates data.

## Why rowid, not timestamp

History sync can deliver messages from years ago well after today's messages have already been indexed,
and an edited or corrected message is re-inserted by the bridge (`INSERT OR REPLACE`), which gives it a
new, higher `rowid` while its `id` stays the same. Scanning by `rowid` means every row is seen exactly
once, in the order the bridge wrote it, regardless of what timestamp it carries -- and reprocessing a
replaced row just updates the existing index entry in place.

## Chunking

Each chat's messages are split into windows so a later embedding step (Phase 2) has coherent,
right-sized text to embed. A new window starts when:

- the silence since the last message exceeds `CHUNK_GAP_MINUTES`, or
- the window would exceed `CHUNK_MAX_MESSAGES` messages, or
- the window would exceed `CHUNK_MAX_CHARS` characters.

Each new window (after the first) is seeded with the last `CHUNK_OVERLAP` messages of the window it
follows, so consecutive chunks share context instead of cutting a conversation at an arbitrary line.

When new messages arrive at the end of a chat, only the chat's currently open window is extended or
closed. When old messages arrive out of order (history sync), only the chunks whose time span comes
within `CHUNK_GAP_MINUTES` of the new messages are recomputed -- the rest of the chat's chunks are left
untouched. A chunk touched by a rebuild is marked unembedded, so Phase 2 knows to re-embed it.

## Name resolution

- `messages.sender_name` in the bridge database is **not used**: it duplicates the sender JID in the
  overwhelming majority of rows and carries no information.
- A sender's display name is looked up in `whatsapp.db`'s `whatsmeow_contacts` table
  (`full_name` > `push_name` > `first_name` > `business_name`), which also holds entries for `@lid`
  (linked-device) JIDs directly. When a `@lid` JID has no direct entry, `whatsmeow_lid_map` resolves it
  to a phone-number JID for a second lookup.
- A chat's stored name (`chats.name`) is often a bare phone number; when it looks like one, the same
  contact lookup is used instead.
- Media without a caption is indexed with a marker (`[imagem]`, `[áudio]`, `[vídeo]`, or
  `[documento: <filename>]`) instead of being skipped, so "that PDF Ana sent" is still findable. A
  caption, when present, is kept as-is.
- Newsletters and broadcast lists (`@newsletter`, `@broadcast`) are not indexed.

## Privacy

The indexer only opens `messages.db` and `whatsapp.db` read-only (`mode=ro`) and never writes to them.
Its own log output never includes message content, chat names or sender names -- only counts and timing
(for backfill progress). `index.db` never leaves the machine; there is no network call anywhere in
this package.

## Configuration

All of these are optional; see [configuration.md](configuration.md) for the full variable reference.

| Variable | Default | Description |
|----------|---------|-------------|
| `INDEX_DB_PATH` | `store/index.db` (`/app/index/index.db` in Docker, which is `./store-index/index.db` on the host) | Where the search index database is written. Outside Docker the default is relative to the directory you run from. |
| `INDEX_POLL_SECONDS` | `20` | How often the indexer checks for new messages once it is caught up. |
| `CHUNK_GAP_MINUTES` | `30` | Silence, in minutes, that starts a new chunk. |
| `CHUNK_MAX_MESSAGES` | `15` | Maximum messages per chunk before it splits. |
| `CHUNK_MAX_CHARS` | `1500` | Maximum characters per chunk before it splits. |
| `CHUNK_OVERLAP` | `2` | Messages repeated at the start of the next chunk, for context. |

## Running it

Docker Compose runs the indexer as its own service, alongside the bridge and MCP server:

```bash
mkdir -p store-index                 # on Linux, so the container user can write to it
docker compose up -d --build indexer
docker compose logs -f indexer
```

It mounts `store/` read-only (it must never be able to write to the bridge's databases) and keeps
`index.db` in a separate, writable `store-index/` directory instead.

Without Docker:

```bash
cd whatsapp-mcp-server
uv run python -m search.indexer
```

## Sensitive data

`index.db` holds a **plain-text copy of your message text** (plus resolved names), so it is as sensitive as
`store/messages.db`. It is git-ignored (`store-index/`); keep it under the same disk encryption and never share it.
Deleting it is safe: the indexer rebuilds it from `messages.db` on the next start.

## What's not here yet

- **Local embeddings and semantic ranking** (Phase 2): the `chunks` table and its `embedded` flag exist
  so this can slot in without re-chunking history.
- **MCP search tools** (Phase 3): `messages_idx` and `chunks` are not yet exposed to the AI assistant.
- **Validation against a real, large message history.** Tests run against synthetic fixtures; indexing
  performance and memory use on a multi-year, multi-thousand-chat history has not been measured yet.
