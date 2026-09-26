# Local Search

Your message history becomes searchable on your machine, by exact words and by meaning. A background
indexer builds the index, and two MCP tools, `search_messages` and `index_status`, let your assistant use
it. No message content ever leaves your machine: the indexer only reads the bridge's SQLite databases,
the embedding model runs locally, and the index is a separate SQLite database
(the `index-data` volume in Docker).

## What it does

```
store/messages.db  ──read-only──▶  indexer  ──writes──▶  index-data volume: index.db
store/whatsapp.db  ──read-only──▶     │                   - messages_idx  (resolved text, FTS5)
                                      │                   - chunks        (conversation windows)
                       local embedding model ────────────▶ - vec_chunks    (one vector per chunk)
                                                                  │ read-only
                                                                  ▼
                                          MCP server: search_messages, index_status
```

A single background loop (`whatsapp-mcp-server/search/indexer.py`) polls `messages.db` for new or
changed rows, resolves display names, and keeps three things up to date in `index.db`:

- **`messages_idx`**, a keyword-searchable copy of each message's text (SQLite FTS5, accent-insensitive),
  with the sender and chat resolved to a display name.
- **`chunks`**, overlapping windows of consecutive messages per chat, formatted as short transcripts.
- **`vec_chunks`**, one embedding per chunk ([sqlite-vec](https://github.com/asg017/sqlite-vec)), so a
  question can find a conversation that shares no words with it ("viagem de SP" finds "bora marcar sampa").

Indexing is **incremental and resumable**: progress is a single cursor (the source database's `rowid`,
not a timestamp, since WhatsApp's history sync can insert old messages long after newer ones) stored in
`index.db`. A restart picks up exactly where it left off, and reprocessing the same messages twice never
duplicates data. The indexer reads all messages first, so keyword search works quickly, and then embeds
the chunks in the background while it is idle.

## The search tools

Both tools are in the `search` toolset, which is part of `all` and read-only. They open `index.db`
read-only and work while the indexer is still catching up; `coverage` in every result says how far it has got.

### `search_messages`

| Argument | Default | Meaning |
|----------|---------|---------|
| `query` | required | Free text, for example `viagem de SP` or `quem trabalha na loja XXX` |
| `chat` | *(all)* | Chat name (accent-insensitive, a part of the name is enough) or JID. An ambiguous name is an error that lists the candidates with their JIDs |
| `sender` | *(all)* | Name or phone number of who wrote it |
| `date_from`, `date_to` | *(none)* | `YYYY-MM-DD`, in `DISPLAY_TZ` (`America/Bahia`); `date_to` includes the whole day |
| `mode` | `hybrid` | `keyword`, `semantic` or `hybrid` |
| `limit` | `10` | Excerpts to return, at most 30 |

It returns excerpts, best first. Each one is a few consecutive messages with chat, period, and per message
the id, time, sender and text (cut at 500 characters); `keyword_hit` marks the messages that contain the
search words, and the others are context. `matched_by` says whether the excerpt came from `keyword`,
`semantic` or both, and semantic matches carry their `similarity`. `coverage` gives the oldest and newest
indexed date and a note, for example when embeddings are still being computed.

The tool description tells the assistant how to use it: pass `chat` when the user names one, read the
surroundings with `get_message_context` before concluding, check `index_status` and call `request_history`
when the answer may predate the synced history, and for open questions run two or three differently worded
searches and combine them.

### `index_status`

Messages and chunks indexed, the oldest and newest dates, how many chunks still wait for an embedding,
the embedding model, when the indexer last ran, and how many source messages are not indexed yet. Pass
`chat` for that chat's own range.

## How results are ranked

1. **Keyword.** The query is split into words, quoted so nothing in it can act as an FTS operator, and
   common Portuguese filler ("de", "quando", "sobre", ...) is dropped. Words of three or more letters match
   as prefixes ("viag" finds "viagem"); short ones such as "SP" match exactly. Messages are ranked with BM25.
2. **Semantic.** The query is embedded and the nearest chunks are taken by cosine distance, with the chat and
   date filters applied inside the vector search.
3. **Fusion.** Both lists are merged per chunk with reciprocal rank fusion (`score = Σ 1/(60 + rank)`), so an
   excerpt found by both paths ranks above one found by only one.

If the semantic side is not available (embeddings off, model not downloaded yet, no vectors yet) hybrid
search falls back to keyword results and says so in `coverage.note`. `mode=semantic` returns an error instead.

## Embeddings

Embeddings are computed on your machine by one of three backends, chosen with `EMBEDDING_BACKEND`:

| Backend | Notes |
|---------|-------|
| `fastembed` (default) | ONNX runtime, no PyTorch. Runs `intfloat/multilingual-e5-small` (384 dimensions, good Portuguese) |
| `sentence_transformers` | Needs `torch` (CPU is enough) and `sentence-transformers` installed in the image |
| `ollama` | Uses a model on an Ollama server you run, for example `bge-m3` (1024 dimensions) |
| `none` | No embeddings: keyword search only |

The model (about 0.5 GB) downloads the first time the indexer starts and is kept in the `model-cache` Docker
volume, shared with the MCP server, which loads it on the first semantic search. The model name and vector
size are stored in `index.db`. **If you change `EMBEDDING_MODEL` or the backend, the indexer detects it and
rebuilds every vector**, which takes as long as the first pass. The MCP server refuses semantic search when
its model differs from the one the index was built with, so set the same values on both services.

If the model cannot be loaded (for example no network on the first start), the indexer keeps indexing
keywords and retries every five minutes.

## Chunking

Each chat's messages are split into windows so each vector describes a coherent piece of conversation. A
new window starts when:

- the silence since the last message exceeds `CHUNK_GAP_MINUTES`, or
- the window would exceed `CHUNK_MAX_MESSAGES` messages, or
- the window would exceed `CHUNK_MAX_CHARS` characters.

Each new window (after the first) is seeded with the last `CHUNK_OVERLAP` messages of the window it
follows, so consecutive chunks share context instead of cutting a conversation at an arbitrary line.

When new messages arrive at the end of a chat, only the chat's currently open window is extended or
closed. When old messages arrive out of order (history sync), only the chunks whose time span comes
within `CHUNK_GAP_MINUTES` of the new messages are recomputed. A chunk touched by a rebuild loses its old
vector and is embedded again.

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
Its own log output never includes message content, chat names or sender names, only counts and timing.
The embedding model runs in the container; nothing is sent to a hosted API. The one download is the
model itself, from Hugging Face, the first time. `index.db` never leaves the machine.

What the search tools return is **message text sent to your AI provider**, like any other read tool. Turn
the toolset off with `WHATSAPP_MCP_TOOLSETS` if you do not want that.

## Configuration

All of these are optional; see [configuration.md](configuration.md) for the full variable reference.

| Variable | Default | Description |
|----------|---------|-------------|
| `INDEX_DB_PATH` | `store/index.db` (`/app/index/index.db` in Docker, stored in the `index-data` Docker volume) | Where the search index database is written. |
| `INDEX_POLL_SECONDS` | `20` | How often the indexer checks for new messages once it is caught up. |
| `CHUNK_GAP_MINUTES` | `30` | Silence, in minutes, that starts a new chunk. |
| `CHUNK_MAX_MESSAGES` | `15` | Maximum messages per chunk before it splits. |
| `CHUNK_MAX_CHARS` | `1500` | Maximum characters per chunk before it splits. |
| `CHUNK_OVERLAP` | `2` | Messages repeated at the start of the next chunk, for context. |
| `EMBEDDING_BACKEND` | `fastembed` | `fastembed`, `sentence_transformers`, `ollama` or `none`. |
| `EMBEDDING_MODEL` | `intfloat/multilingual-e5-small` | The embedding model. |
| `EMBED_BATCH_SIZE` | `32` | Chunks embedded per step. |
| `OLLAMA_URL` | `http://host.docker.internal:11434` | Only for the `ollama` backend. |
| `SEARCH_K_FTS`, `SEARCH_K_VEC` | `50`, `50` | Candidates per path before fusion. |
| `SEARCH_MIN_SIMILARITY` | `0` | Drop semantic matches below this similarity. |
| `DISPLAY_TZ` | `America/Bahia` | Timezone of the date filters and of the dates in results. |

## Running it

Docker Compose runs the indexer as its own service, alongside the bridge and MCP server:

```bash
docker compose up -d --build
docker compose logs -f indexer
```

The indexer mounts `store/` read-only (it must never be able to write to the bridge's databases) and keeps
`index.db` in a separate, writable Docker volume (`index-data`). The MCP server mounts that volume too and
opens `index.db` read-only. The first start downloads the model and then embeds the whole history, so
expect the semantic side to fill in gradually; `index_status` shows the progress.

Without Docker:

```bash
cd whatsapp-mcp-server
uv run python -m search.indexer
```

## Sensitive data

`index.db` holds a **plain-text copy of your message text** (plus resolved names) and vectors derived from
it, so it is as sensitive as `store/messages.db`. It lives in a Docker volume, outside the repository; keep it under the same
disk encryption and never share it. Deleting it is safe: the indexer rebuilds it from `messages.db` on the
next start.

## Testing

```bash
cd whatsapp-mcp-server
uv run pytest tests/search                       # fast, uses a stand-in embedder
RUN_MODEL_TESTS=1 uv run pytest tests/search/test_real_model.py   # downloads the real model once
```

## Known limits

- **Measured once, tuned never.** On one real history (about 30,000 messages in 5,700 chunks over three years and
  600 chats) on an Apple Silicon Mac, embedding every chunk took about 14 minutes (roughly 7 chunks a second),
  the indexer peaked near 1.5 GB of memory, and each search took 10 to 90 ms. Result quality was not
  evaluated, and the chunk size, candidate counts and `SEARCH_MIN_SIMILARITY` have not been tuned on real
  questions. Inside Docker the speed depends on the CPUs you give the container.
- The default model gives similarity scores that sit close together (roughly 0.8 to 0.9), so semantic
  search always returns its nearest chunks, relevant or not. Rely on `matched_by` and read the excerpt.
- The date filters of the semantic path compare against the chunk's start time, so an excerpt that starts
  a few hours before `date_from` can still be returned when it overlaps it.
