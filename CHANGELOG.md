# Changelog

All notable changes to this project are recorded here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions follow [Semantic Versioning](https://semver.org/).

## \[Unreleased]

Based on `whatsapp-mcp-extended` 0.3.0 (see [NOTICE.md](NOTICE.md)).

### Changed

- Media downloads stream to disk (`store/media/<chat>/<message id>.<ext>`) through a bounded pool
  (`MEDIA_DOWNLOAD_WORKERS`) instead of one unbounded goroutine per message and the whole file in memory. The file is
  named after the message id, not the arrival time, so two media received in the same second no longer overwrite each
  other. Failed downloads are recorded instead of only logged.
- **Several WhatsApp numbers.** The bridge now runs one client per paired number; every event, connection state,
  reconnection and presence loop belongs to its own number. One number logging out, timing out or being replaced no
  longer disconnects the others or restarts the container; the watchdog exits only when every paired number has been
  offline for over three minutes. Each number has its own send warm-up state.
- `messages` is keyed by `(instance_jid, chat_jid, id)`: the same group message seen by two numbers is stored once per
  number instead of one overwriting the other. Chunks and messages in the search index are per number and chat, so a
  customer talking to two numbers is two conversations. Migration `003` rebuilds the tables (with a backup) and the
  search index is rebuilt from `messages.db` on the next indexer start. See [docs/migrations.md](docs/migrations.md).
- Migrations `002` onward are applied by the bridge at startup and recorded in `schema_migrations`; the table definitions
  are no longer repeated in Go code.
- Writes to `messages.db` go through one writer that commits batches under per-task savepoints. Live messages are served
  before history sync, and history sync no longer waits for each commit (it was limited to about 40 messages per second,
  now thousands). Webhook configuration writes use the same writer. The upsert that stores a message no longer resets its
  audit state when history sync redelivers it.
- Routes that act on the WhatsApp account require `?instance=` when several numbers are paired and answer `403` for a
  number without send permission. New `INSTANCE_ALLOW_SEND_DEFAULT`. MCP tools that act on WhatsApp accept `instance_jid`.
- `GET /api/health` is healthy while any paired number is connected. `employee` updates are partial.
- Web panel: *Números WhatsApp* (rewritten around instance ids: link people, send permission, attestation, a QR code that
  renews itself), *Mensagens* is now a unified feed filtered by department, employee, number, text and dates.

### Added

- **Media storage in an S3-compatible bucket** (Cloudflare R2, Amazon S3, MinIO, ...). Downloaded media is queued and
  uploaded in the background by a bounded worker pool, then registered in the new `message_media` table (migration
  `005`). Files are organized by department, employee, number and conversation; the location of every file is in the
  database, so the agent can always reach it. Configured in *Settings > Media storage* (credentials stored encrypted)
  or with `S3_*` variables. New routes `GET/PUT /api/settings/media-storage`, `POST .../test`, `POST .../retry`,
  `GET /api/media/url`; `POST /api/download` reads from the local copy or the bucket before asking the WhatsApp CDN.
  See [docs/media-storage.md](docs/media-storage.md).

- Organization model: departments, employees, numbers, and the history of who held each number (`instance_assignments`),
  so messages stay with whoever held the number at the time. REST endpoints and panel pages for all of it.
- Audit: revoked messages are flagged with when and by whom, edits and revocations keep the earlier text
  (`message_versions`), a per-call access log (`access_log`), and a panel page *Auditoria*.
- MCP: `list_departments`, `list_employees`, `resolve_employee`, `list_instances`, `get_audit_trail`,
  `get_audit_deleted_messages`, `get_employee_activity_summary`, `search_department_conversations`, `list_access_log`, and
  `department_id` / `employee_id` / `instance_jid` filters on `search_messages`.
- Access policy for MCP clients (`MCP_ACCESS_POLICY`, `MCP_READ_ONLY`): which numbers and periods a client may read and
  whether it may act on WhatsApp, enforced where data is read. Fails closed. See [docs/governance.md](docs/governance.md).
- Corporate-asset attestation when pairing a number, optionally required to record (`REQUIRE_CORPORATE_CONFIRMATION`).
- Privacy tools: anonymize a person, retention by age (`RETENTION_DAYS`), both logged and applied to the search index.
- Webhook trigger type `instance_jid`, and `instance_jid` in webhook payloads.
- Tests read the bridge's real migrations, so a Python query for a column the bridge does not create now fails a test.

### Fixed

- `list_employees`, `resolve_employee` and `list_instances` returned nothing against a real database because they
  queried columns the bridge never created; they now raise a clear error instead of returning an empty list.
- The web panel called `/api/instances/undefined/...` and sent fields the bridge does not have; the contract now matches.
- The "message deleted" badge never appeared because the read paths did not return the flag.
- `Message.is_group` was dropped from the API response.
- ESLint errors in the pairing dashboard and webhook components, and hard-coded palette colors on the numbers page.
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

