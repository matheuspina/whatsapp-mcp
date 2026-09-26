# Documentation

| Document | What it covers |
|----------|----------------|
| [Architecture](architecture.md) | Components, data flow, storage, security boundaries, where to extend |
| [Configuration](configuration.md) | Every environment variable, defaults and ports |
| [Authentication](authentication.md) | Web panel login, sessions, API key, protections and limits |
| [MCP OAuth](mcp-oauth.md) | OAuth 2.1 for the MCP endpoint: setup, supported clients, security model |
| [Coolify](coolify.md) | Deploying the whole stack from the single root `Dockerfile` |
| [History sync](history-sync.md) | How much history WhatsApp sends, on-demand requests, limits |
| [Webhooks](webhooks.md) | Real-time message webhooks: triggers, matching, signatures, retries |
| [Database](database.md) | SQLite schema, what is and is not captured |
| [Migrations](migrations.md) | Running schema migrations on an existing installation |
| [Governance](governance.md) | Several numbers, people and departments, audit trail, access policy for AI clients, corporate attestation, LGPD |
| [Media storage](media-storage.md) | Keeping media in Cloudflare R2, S3 or MinIO: the upload queue, folder layout, settings, reading files back |
| [Search](search.md) | Local hybrid search: the indexer, chunking, local embeddings, and the `search_messages` / `index_status` tools |
| [Response design](response-design.md) | Why tool responses carry raw data instead of interpretation |

Also in the repository root: [README](../README.md), [SECURITY](../SECURITY.md), [CONTRIBUTING](../CONTRIBUTING.md),
[ROADMAP](../ROADMAP.md), [CHANGELOG](../CHANGELOG.md), [COMMERCIAL](../COMMERCIAL.md), [NOTICE](../NOTICE.md).
