# Changelog

All notable changes to this project are recorded here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions follow [Semantic Versioning](https://semver.org/).

## \[Unreleased]

Based on `whatsapp-mcp-extended` 0.3.0 (see [NOTICE.md](NOTICE.md)).

### Fixed

- The `indexer` service crash-looped with `sqlite3.OperationalError: unable to open database file` when `./store-index`
  did not exist: Docker created it as root and the non-root container user could not write to it. The index now lives
  in the `index-data` named volume.

### Added

- Local hybrid search (Phases 2 and 3): the indexer now embeds every chunk with a local model
  (`intfloat/multilingual-e5-small` through fastembed, stored with sqlite-vec) and the MCP server has a new
  `search` toolset with `search_messages` (keyword, semantic or hybrid, fused with reciprocal rank fusion, filtered by
  chat, sender and dates in `America/Bahia`) and `index_status`. Switching `EMBEDDING_MODEL` or the backend rebuilds
  the vectors. `EMBEDDING_BACKEND=none` keeps keyword search only. New variables: `EMBEDDING_BACKEND`,
  `EMBEDDING_MODEL`, `EMBEDDING_CACHE_DIR`, `EMBED_BATCH_SIZE`, `OLLAMA_URL`, `SEARCH_K_FTS`, `SEARCH_K_VEC`,
  `SEARCH_MIN_SIMILARITY`, `DISPLAY_TZ`. See [docs/search.md](docs/search.md).
- Root `Dockerfile` that builds the bridge, the MCP server and the web panel into one image behind nginx, for
  platforms that accept a single Dockerfile (Coolify). See [docs/coolify.md](docs/coolify.md).
- MCP tab in the web panel: lists the common MCP clients (Claude, Claude Code, Cursor, VS Code, ChatGPT, Codex,
  Antigravity, Gemini CLI, Windsurf, Grok) with a connect link or a copyable command and config for each.
- Local search indexer (Phase 1 of hybrid search): a background service that keeps a keyword-searchable
  SQLite FTS5 index (`store-index/index.db`) in sync with `messages.db`, plus overlapping conversation chunks
  ready for local embeddings in a later phase. Runs as its own `indexer` Compose service, read-only
  against the bridge store. No message content leaves the machine or is logged. See
  [docs/search.md](docs/search.md).
- Web panel login with a username and password from `.env` (`WEB_UI_USERNAME`, `WEB_UI_PASSWORD`, `WEB_UI_SESSION_TTL`).
- Server-side sessions delivered as an `HttpOnly`, `SameSite=Strict` cookie, with CSRF protection through an `Origin` check and login throttling.
- Active sessions list in Settings, with the ability to end other sessions. New endpoints: `POST /api/auth/login`, `POST /api/auth/logout`, `GET /api/auth/me`, `GET /api/auth/sessions`, `DELETE /api/auth/sessions/{id}`.
- Documentation: configuration reference, authentication, architecture, and a rewritten README and security policy.
- `NOTICE.md`, `COMMERCIAL.md`, `ACKNOWLEDGEMENTS.md` and a code of conduct.
- OAuth 2.1 for the MCP endpoint, opt-in through `MCP_PUBLIC_URL`: discovery (RFC 9728, RFC 8414), dynamic client registration, authorization code with PKCE, rotating refresh tokens, revocation, and a sign-in page that reuses the panel credentials. `API_KEY` is also accepted as a bearer token. See [docs/mcp-oauth.md](docs/mcp-oauth.md).

### Changed

- Project name and branding: **WhatsApp MCP** by Matheus Pina. The MCP server now reports itself as `whatsapp-mcp`.
- License: modifications and additions are under the PolyForm Noncommercial License 1.0.0. Inherited MIT-licensed code keeps its notice (see [NOTICE.md](NOTICE.md)).
- The web panel no longer asks for, stores or sends the API key. Any key saved by the earlier panel in the browser is discarded.
- Unauthorized bridge responses are JSON instead of plain text.
- `docker-compose.yaml` passes `.env` to the bridge (`env_file`) and the history sync variables through, so they actually take effect.
- Documentation reorganized under `docs/`.

### Security

- The bridge no longer prints `API_KEY` in its startup banner.
- The bridge refuses to start when `API_KEY` or `WEB_UI_PASSWORD` still have the `CHANGEME...` example value from `.env.example`.

