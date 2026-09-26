# Configuration

All settings are environment variables. With Docker Compose, put them in a `.env` file next to `docker-compose.yaml`
(start from [`.env.example`](../.env.example)). The whole `.env` is passed to the bridge container; the MCP server
receives the variables listed in the compose file.

> `.env` holds secrets. It is git-ignored; keep it that way and restrict it with `chmod 600 .env`.

## Required

| Variable | Description |
|----------|-------------|
| `API_KEY` | Shared secret for the bridge REST API (`X-API-Key` header). Used by the MCP server and scripts. Generate with `openssl rand -hex 32`. |
| `WEB_UI_USERNAME` | Username for the web panel login. |
| `WEB_UI_PASSWORD` | Password for the web panel login. Use a strong one (`openssl rand -base64 18`). |

The bridge will not start without `API_KEY` unless `DISABLE_AUTH_CHECK=true` (development only).
It also refuses to start while `API_KEY` or `WEB_UI_PASSWORD` still hold the `CHANGEME...` example value from `.env.example`, so the public placeholder can never become your real credential.

## Web panel

| Variable | Default | Description |
|----------|---------|-------------|
| `WEB_UI_SESSION_TTL` | `24h` | Inactivity timeout of a panel session. Every request extends it. Go duration (`30m`, `8h`). |
| `CORS_ORIGINS` | *(none)* | Extra allowed browser origins, comma separated. `localhost` and `127.0.0.1` on ports `8089` and `8090` are always allowed. |

See [authentication.md](authentication.md).

## History sync

These only take effect **when you pair a device**. To change them afterwards, unlink the device in WhatsApp
(*Settings → Linked devices*), delete `store/whatsapp.db`, and pair again. WhatsApp decides what it actually sends: the values are a request.
See [history-sync.md](history-sync.md).

| Variable | Default | Description |
|----------|---------|-------------|
| `HISTORY_SYNC_DAYS_LIMIT` | `365` | Days of history to ask for. |
| `HISTORY_SYNC_SIZE_MB` | `5000` | Maximum size of the initial history sync. |
| `STORAGE_QUOTA_MB` | `10240` | Storage quota announced to WhatsApp. |

## MCP server

| Variable | Default | Description |
|----------|---------|-------------|
| `WHATSAPP_MCP_TOOLSETS` | `all` | Toolsets to expose, comma separated: `core`, `send`, `media`, `history`, `contacts_write`, `message_admin`, `groups`, `presence`, `account_admin`, `newsletter`, `search`, or `all`. |
| `WHATSAPP_MCP_TOOLS` | *(none)* | Individual tools to expose in addition, by name (for example `manage_group,delete_message`). |
| `MCP_TRANSPORT` | `stdio` (`streamable-http` in Docker) | `stdio`, `sse` or `streamable-http`. |
| `HOST` / `PORT` | `0.0.0.0` / `8081` | Bind address for the HTTP transports. |
| `BRIDGE_HOST` | `localhost:8080` (`whatsapp-bridge` in Docker) | Where the MCP server finds the bridge. |
| `MCP_PUBLIC_URL` | *(unset)* | Public origin of the MCP server. Setting it turns on OAuth 2.1 for the MCP endpoint and requires `WEB_UI_USERNAME` / `WEB_UI_PASSWORD`. See [mcp-oauth.md](mcp-oauth.md). |
| `MCP_OAUTH_ACCESS_TOKEN_TTL` / `MCP_OAUTH_REFRESH_TOKEN_TTL` | `3600` / `2592000` | OAuth token lifetimes, in seconds. |
| `WA_STORE_PATH` | auto | Directory containing `messages.db` and `whatsapp.db`. |
| `WA_SKIP_DB_CHECK` | `0` | Set to `1` to skip the startup check that `messages.db` exists (tests and linting). |
| `DEBUG` | `false` | Verbose logging. |

## Local search indexer

The background indexer that keeps the search index (`index.db`, in the `index-data` volume) up to date. See [search.md](search.md).

| Variable | Default | Description |
|----------|---------|-------------|
| `INDEX_DB_PATH` | `store/index.db` (`/app/index/index.db` in Docker) | Where the search index database is written. Compose sets it; you rarely need to. |
| `INDEX_POLL_SECONDS` | `20` | How often the indexer checks for new messages once caught up. |
| `CHUNK_GAP_MINUTES` | `30` | Silence, in minutes, that starts a new conversation chunk. |
| `CHUNK_MAX_MESSAGES` | `15` | Maximum messages per chunk before it splits. |
| `CHUNK_MAX_CHARS` | `1500` | Maximum characters per chunk before it splits. |
| `CHUNK_OVERLAP` | `2` | Messages repeated at the start of the next chunk, for context. |
| `EMBEDDING_BACKEND` | `fastembed` | `fastembed` (ONNX, no PyTorch), `sentence_transformers` (needs `torch` and `sentence-transformers` installed), `ollama`, or `none` to keep keyword search only. Set the same value for the `indexer` and `whatsapp-mcp` services. |
| `EMBEDDING_MODEL` | `intfloat/multilingual-e5-small` | Embedding model (384 dimensions). Changing it makes the indexer rebuild every vector. With `ollama`, an Ollama model name such as `bge-m3`. |
| `EMBEDDING_CACHE_DIR` | *(library default)* | Where the model is stored. Compose points it at the `model-cache` volume. |
| `EMBED_BATCH_SIZE` | `32` | Chunks embedded per step. Lower it if the indexer runs out of memory. |
| `OLLAMA_URL` | `http://host.docker.internal:11434` | Ollama server, only for `EMBEDDING_BACKEND=ollama`. |

## Search tools

Read by the MCP server's `search` toolset (`search_messages`, `index_status`). See [search.md](search.md).

| Variable | Default | Description |
|----------|---------|-------------|
| `SEARCH_K_FTS` | `50` | Keyword candidates taken before fusing with the semantic ones. |
| `SEARCH_K_VEC` | `50` | Semantic candidates taken before fusing with the keyword ones. |
| `SEARCH_MIN_SIMILARITY` | `0` | Drop semantic matches below this cosine similarity (0 keeps everything). Scores of the default model cluster between roughly 0.8 and 0.9, so tune it against your own history before setting it. |
| `DISPLAY_TZ` | `America/Bahia` | Timezone for the `date_from` / `date_to` filters and for the dates in results. |

## Bridge

| Variable | Default | Description |
|----------|---------|-------------|
| `API_PORT` | `8080` | Bridge HTTP port inside the container. |
| `API_BIND_HOST` | `127.0.0.1` (`0.0.0.0` in Docker) | Bind address. Compose still publishes the port on `127.0.0.1` only. |
| `WHATSAPP_ALLOWLIST_JIDS` | *(none)* | Comma-separated phone numbers or JIDs. When set, sending to anyone else is rejected. |
| `PRESENCE_PING_ENABLED` | `true` | Set `false` to stop broadcasting presence to contacts. |
| `PRESENCE_PING_INTERVAL` | `20m` | How often to ping presence. Keep it at 20 minutes or more. |
| `WA_PRESENCE_MODE` | `human` | `human` stays offline except briefly around outgoing activity; `always_online` stays online while connected. |
| `PRESENCE_LINGER_MIN` / `PRESENCE_LINGER_MAX` | `8s` / `15s` | How long `human` mode stays online after sending. |

### Send throttling (anti-ban)

Off by default. Adds human-like delays and a warm-up ramp to outgoing messages.

| Variable | Default | Description |
|----------|---------|-------------|
| `ANTIBAN_ENABLED` | `false` | Turn the send interceptor on. |
| `ANTIBAN_TEXT_DELAY_MIN` / `ANTIBAN_TEXT_DELAY_MAX` | `1.5s` / `4s` | Delay range before a text message. |
| `ANTIBAN_FEEDBACK_DELAY_MIN` / `ANTIBAN_FEEDBACK_DELAY_MAX` | `500ms` / `1.5s` | Delay range before reactions and receipts. |
| `ANTIBAN_TYPING_MS_PER_CHAR` | `30` | Simulated typing speed. |
| `ANTIBAN_WARMUP_DAYS` | `7` | Length of the warm-up ramp. |
| `ANTIBAN_WARMUP_START_LIMIT` | `20` | Messages per day on the first warm-up day. |
| `ANTIBAN_WARMUP_STATE_PATH` | `store/antiban_warmup.json` | Where the warm-up state is kept. |
| `ANTIBAN_RISK_PAUSE_THRESHOLD` | `70` | Risk score at which sending pauses. |

## Development-only switches

**Never enable these in a real deployment.**

| Variable | Effect |
|----------|--------|
| `DISABLE_AUTH_CHECK=true` | Lets the bridge start without `API_KEY`. |
| `DISABLE_SSRF_CHECK=true` | Allows webhook URLs that point at private networks. |
| `DISABLE_PATH_CHECK=true` | Disables the allowed-directory check for media paths. |

## Ports

| Service | Host | Container | Purpose |
|---------|------|-----------|---------|
| Bridge API | `127.0.0.1:8180` | `8080` | REST API |
| MCP server | `127.0.0.1:8081` | `8081` | MCP over streamable HTTP, at `/mcp` |
| Web panel | `127.0.0.1:8090` | `8080` | Login, pairing, sessions, webhooks |
