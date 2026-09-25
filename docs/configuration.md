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
| `WHATSAPP_MCP_TOOLSETS` | `all` | Toolsets to expose, comma separated: `core`, `send`, `media`, `history`, `contacts_write`, `message_admin`, `groups`, `presence`, `account_admin`, `newsletter`, or `all`. |
| `WHATSAPP_MCP_TOOLS` | *(none)* | Individual tools to expose in addition, by name (for example `manage_group,delete_message`). |
| `MCP_TRANSPORT` | `stdio` (`streamable-http` in Docker) | `stdio`, `sse` or `streamable-http`. |
| `HOST` / `PORT` | `0.0.0.0` / `8081` | Bind address for the HTTP transports. |
| `BRIDGE_HOST` | `localhost:8080` (`whatsapp-bridge` in Docker) | Where the MCP server finds the bridge. |
| `WA_STORE_PATH` | auto | Directory containing `messages.db` and `whatsapp.db`. |
| `WA_SKIP_DB_CHECK` | `0` | Set to `1` to skip the startup check that `messages.db` exists (tests and linting). |
| `DEBUG` | `false` | Verbose logging. |

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
