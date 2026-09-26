# CLAUDE.md

Guidance for Claude Code (claude.ai/code) when working in this repository. See also [AGENTS.md](AGENTS.md).

## Project overview

WhatsApp MCP is a Model Context Protocol server that gives an AI assistant access to a personal WhatsApp account. It is a Docker Compose stack of three services: a Go bridge, a Python MCP server and a Next.js web panel.

## Architecture

```
AI client ──MCP──▶ whatsapp-mcp (Python, :8081/mcp) ──REST + X-API-Key──▶ whatsapp-bridge (Go, :8080 → host 8180) ◀──▶ WhatsApp
                          │                                                        │
                          └──────── read-only SQLite ──▶ store/ (messages.db, whatsapp.db) ◀┘
Browser ──▶ web-ui (nginx, host :8090) ──REST + session cookie──▶ whatsapp-bridge
```

- **`whatsapp-bridge/` (Go).** `internal/api` (HTTP handlers, middleware, auth handlers), `internal/auth` (panel sessions), `internal/whatsapp` (client wrapper, messages, media), `internal/webhook`, `internal/database` (SQLite), `internal/config`, `internal/antiban`, `internal/security` (audit log), `internal/types`.
- **`whatsapp-mcp-server/` (Python).** `main.py` registers tools and toolsets (stdio, SSE or streamable HTTP via `MCP_TRANSPORT`); `whatsapp.py` holds the client functions; `lib/` has `models`, `database`, `bridge`, `utils`. `gradio-main.py` is an optional Gradio variant.
- **`whatsapp-web-ui/` (Next.js, static export served by nginx).** `src/app` pages (`login`, `pairing`, `settings`, `webhooks`, `mcp-clients`, `instances`, `messages`, `audit`, `departments`, `employees`), `src/components`, `src/lib/api.ts` (bridge client), `src/lib/store.ts` (zustand).

## Commands

```bash
cp .env.example .env            # set API_KEY, WEB_UI_USERNAME, WEB_UI_PASSWORD
docker compose up -d --build    # start everything
docker compose logs -f whatsapp-bridge
docker compose up -d --build <service>   # rebuild one service after a change (no hot reload)
```

Development without Docker:

```bash
cd whatsapp-bridge && go run main.go && go test -race ./...
cd whatsapp-mcp-server && uv sync --all-extras && uv run python check.py && uv run pytest --cov=lib -v
cd whatsapp-web-ui && npm ci && npm run dev
```

Run `uv run python check.py` (quick) before a Docker build; it catches import and syntax errors in seconds.

When `whatsmeow` reports `Client outdated (405)`:

```bash
cd whatsapp-bridge && go get -u go.mau.fi/whatsmeow@latest && go mod tidy
```

## Database migrations

Schema changes must not require rebuilding from scratch. Add an idempotent, append-only script in `whatsapp-bridge/migrations/` (`NNN_feature.sql`, `IF NOT EXISTS`, no `DROP`), register it in `schemaMigrationSteps` (`internal/database/migrate.go`), which the bridge applies at startup and records in `schema_migrations`, keep it backward compatible, and document it in [docs/migrations.md](docs/migrations.md). The one exception is a change SQLite cannot make in place (a primary key): write it in Go, run it only when the old shape is detected, inside a transaction, after a `VACUUM INTO` backup (see `003_instance_scoped_keys`). Tests must exercise a legacy database, not only a fresh one.

## Key patterns

- **Toolsets.** Tools register through the `@tool(<toolset>, <title>, ...)` decorator in `main.py`. `WHATSAPP_MCP_TOOLSETS` selects toolsets (`all` by default); `WHATSAPP_MCP_TOOLS` adds single tools by name.
- **Reads vs writes.** Reads come straight from SQLite; actions go through the bridge REST API. In Go, every write to `messages.db` goes through `enqueueWrite` (one writer goroutine, one transaction per batch, one savepoint per task); never call `db.Exec` for a write from a handler. In Python, open the databases with `connect_messages` / `connect_whatsapp` from `lib/access.py`, never `sqlite3.connect`, so the caller's access scope applies.
- **Numbers.** The bridge runs one client per paired number (`InstanceManager`). Handlers receive the client the event belongs to; never use the default client inside an event handler. Messages are keyed by `(instance_jid, chat_jid, id)` and attributed to the person who held the number at that time through `instance_assignments`. Routes that act on the account go through `s.outbound(...)`, read-only ones through `s.scoped(...)`.
- **Access scope.** `lib/access.py` applies the MCP client's policy (`MCP_ACCESS_POLICY`) to database reads and search; a broken policy denies everything. Tests that read bridge tables build them from the real migrations (`tests/bridge_schema.py`).
- **Webhooks.** Triggers: `all`, `chat_jid`, `instance_jid`, `sender`, `keyword`, `media_type`. Matching: `exact`, `contains`, `regex`. Delivery is async with exponential backoff and HMAC-SHA256 signatures.
- **JIDs.** Individual `{phone}@s.whatsapp.net`, group `{id}@g.us`, linked device `{id}@lid`, broadcast `status@broadcast`.
- **Message ids** are hex strings (for example `3EB028A580CF7CC9AAF3A2`), used by edit, delete, react and mark-read.

## Ports

| Service | Host | Container |
|---------|------|-----------|
| Bridge REST API | `127.0.0.1:8180` | `8080` |
| MCP server (`/mcp`) | `127.0.0.1:8081` | `8081` |
| Web panel | `127.0.0.1:8090` | `8080` |

## Security

- **Bridge API** requires `X-API-Key` (`API_KEY`, constant-time comparison) or a panel session. The bridge exits at startup without `API_KEY` unless `DISABLE_AUTH_CHECK=true` (development only).
- **Panel login** uses `WEB_UI_USERNAME` / `WEB_UI_PASSWORD`, server-side sessions, an `HttpOnly` `SameSite=Strict` cookie, an `Origin` check on cookie-authenticated state changes, and login throttling. See [docs/authentication.md](docs/authentication.md).
- **Webhook URLs** that point at private networks are blocked (`DISABLE_SSRF_CHECK=true` to test locally).
- **Media paths** must be inside the allowed directories (`DISABLE_PATH_CHECK=true` for development only).
- **Rate limit:** 100 requests per minute per address on the bridge API.
- **CORS:** `localhost` and `127.0.0.1` on ports 8089 and 8090, plus `CORS_ORIGINS`.
- **Containers** run as a non-root user in production. Never print secrets in logs.

See [SECURITY.md](SECURITY.md) and [docs/configuration.md](docs/configuration.md).

## Code standards

**Go:** use the logger, not `fmt.Println` (except QR and startup status); godoc on exported functions; table-driven tests; wrap errors with `fmt.Errorf("context: %w", err)`; exit with `os.Exit(1)` on critical startup errors.

**Python:** use `logger` from `lib.utils`, not `print()`; type hints and docstrings on public functions; raise exceptions instead of returning empty on error.

**TypeScript:** strict types; no credentials in browser storage; state from the server, not from `localStorage`. Styling: no hardcoded palette colors (`green-500`, hex values) or per-page typography in pages. Use the theme tokens in `globals.css` (`primary`, `success`, `warning`, `destructive`, `muted`), `PageContainer` and `PageHeader` from `components/layout/page.tsx`, and the shared components in `components/common/`.

## Testing

```bash
cd whatsapp-mcp-server && uv run pytest --cov=lib -v
cd whatsapp-bridge && go test -v -race ./...
cd whatsapp-web-ui && npx tsc --noEmit && npx eslint src
```

Minimum 50% coverage. CI runs on push and pull request to `main`: `go-test.yml`, `python-test.yml`, `lint.yml` and `security.yml`.
