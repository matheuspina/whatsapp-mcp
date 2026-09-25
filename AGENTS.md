# AGENTS.md: guidelines for AI agents and contributors

Read this before changing code in **WhatsApp MCP**. Human contributors should also read [CONTRIBUTING.md](CONTRIBUTING.md).

## Architecture

1. **`whatsapp-bridge/` (Go).** Built on `whatsmeow`. Owns the WhatsApp connection, `store/messages.db` and `store/whatsapp.db`, media downloads, webhooks, the REST API, and the web panel's login sessions (`internal/auth`).
2. **`whatsapp-mcp-server/` (Python).** FastMCP server exposing 27 MCP tools grouped in toolsets. Reads messages straight from SQLite and calls the bridge REST API for actions. Some modules under `lib/` (`recall.py`, `transcribe.py`) are unused leftovers and are not wired into any tool.
3. **`whatsapp-web-ui/` (Next.js, static export).** Web panel: login, pairing, sync status, active sessions, webhooks. Calls the bridge API from the browser.

See [docs/architecture.md](docs/architecture.md).

## Critical rules

1. **Safety of the user's account.**
   - Never change send rates or payload behaviour in ways that could trigger WhatsApp anti-spam detection.
   - Respect the `WHATSAPP_ALLOWLIST_JIDS` gate when it is set.
   - Never call tools that send, edit or delete messages while developing or testing, not even "to check that it works".

2. **Privacy.**
   - Never log message content. Log counts, ids and timings.
   - Never commit `.env`, `store/`, session files, real phone numbers or message data. Use obviously fake data in tests and docs.

3. **Credentials stay out of the browser.**
   - The panel authenticates with an `HttpOnly` session cookie. Do not add code that keeps tokens, passwords or the API key in `localStorage`, `sessionStorage` or JavaScript-readable cookies.
   - Do not print secrets in logs or banners.

4. **Database and state.**
   - SQLite access to `messages.db` and `whatsapp.db` uses WAL mode and proper transactions. Do not modify `whatsapp.db`, which belongs to whatsmeow.
   - Never break LID to phone JID resolution (`<id>@lid` to phone JID).
   - Schema changes ship as idempotent, append-only migrations (see [docs/migrations.md](docs/migrations.md)).

5. **Optional dependencies** must be imported lazily and fail with a structured error (`{success: false, message: ...}`), so the default install keeps working.

## Quick commands

```bash
# Bridge
cd whatsapp-bridge && go test -race ./...

# MCP server
cd whatsapp-mcp-server && uv run pytest && uv run ruff check .

# Web panel
cd whatsapp-web-ui && npx tsc --noEmit && npx eslint src && npm run build
```

There is no hot reload in Docker. After a change: `docker compose up -d --build <service>`.
