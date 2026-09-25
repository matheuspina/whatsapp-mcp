# Roadmap

This is a direction, not a promise. Priorities follow what is useful and safe for personal use.
Ideas and pull requests are welcome; see [CONTRIBUTING.md](CONTRIBUTING.md).

## Shipped

- WhatsApp Web multi-device bridge (Go, whatsmeow) with a REST API, media download, webhooks and history sync
- Curated MCP tool surface (29 tools) grouped into toolsets, over stdio and streamable HTTP
- Messaging, replies and quotes, reactions, polls, message edit and delete, read receipts
- Groups, contacts and nicknames, presence, blocklist, newsletters
- Optional send allowlist and anti-ban send throttling
- Web panel: device pairing with a phone code, sync status, webhook management
- Web panel login with server-side sessions and a list of active sessions ([docs](docs/authentication.md))
- Configurable history window at pairing time and on-demand history per chat
- Local hybrid search: a background indexer, local embeddings and the `search_messages` / `index_status` tools ([docs](docs/search.md))

## Next

- **Tuning local search on a real history.** Measure indexing time and memory on a multi-year history, and tune the chunk size, the candidate counts and `SEARCH_MIN_SIMILARITY` against real questions.
- **Persistent panel sessions** (optional), so a restart does not sign you out.
- **A ready-made TLS profile** for the Compose file, for people who want to reach the panel from another device.
- **Cleanup of unused optional modules** left over from earlier forks (`lib/recall.py`, `lib/transcribe.py`).

## Later

- A read-only message browser in the panel
- Backup and restore helpers for `store/`
- Prebuilt container images

## Not planned

- Multi-tenant or hosted deployments. The design assumes one person on a trusted machine.
- Bulk or unsolicited messaging features.
