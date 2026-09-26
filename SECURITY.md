# Security Policy

## Reporting a vulnerability

Please report security problems **privately**, not in a public issue.

- Use GitHub's [private vulnerability reporting](https://github.com/matheuspina/whatsapp-mcp/security/advisories/new), or
- email **mathpinab@gmail.com** with a description, steps to reproduce and the impact you see.

You can expect an acknowledgement within a few days. Once a fix is released, the issue can be disclosed publicly.
This is a personal project maintained by one person, so there is no formal SLA.

## Supported versions

Only the latest release receives security fixes. Please update before reporting.

## Threat model

WhatsApp MCP is built for **personal use on a machine you trust**, by one person, for their own account.
It is **not** designed for multi-tenant use or exposure to the public internet without extra hardening.

### What is protected

- **Local-only network exposure.** The Docker Compose file publishes every port on `127.0.0.1` only (bridge API `8180`, MCP server `8081`, web panel `8090`), and the services talk over an isolated internal network.
- **Bridge API authentication.** Every bridge endpoint needs either the `X-API-Key` header (constant-time comparison; used by the MCP server and scripts) or a valid web panel session. The bridge refuses to start without `API_KEY` (unless you explicitly disable this for development), and refuses the `CHANGEME...` example values for `API_KEY` and `WEB_UI_PASSWORD`.
- **Web panel login.** Username and password come from `.env`. Sessions live on the server, and the browser only receives an `HttpOnly`, `SameSite=Strict` cookie that page scripts cannot read. State-changing requests authenticated by cookie must come from an allowlisted `Origin` (CSRF defence). Failed logins are throttled (5 failures per 5 minutes per address). Active sessions can be listed and ended from the panel. Details in [docs/authentication.md](docs/authentication.md).
- **Secrets stay out of logs.** The bridge does not print the API key.
- **Hardening inherited from upstream.** Media paths are validated against allowed directories (symlinks resolved), webhook URLs are checked to block private-network targets (SSRF), webhook payloads are signed with HMAC-SHA256, the API is rate limited, and sensitive events go to an audit log. An optional allowlist (`WHATSAPP_ALLOWLIST_JIDS`) limits who the bridge can send to.

### Known limitations

These are real. Read them before you connect an AI agent to your account.

- **The MCP endpoint has no authentication unless you turn on OAuth.** Set `MCP_PUBLIC_URL` to require an OAuth 2.1 token, see [docs/mcp-oauth.md](docs/mcp-oauth.md); without it the MCP server (`http://127.0.0.1:8081/mcp`) accepts connections from any process on the same machine, and its tools include sending and deleting messages. The `127.0.0.1` binding keeps it off the network, but any local program can use it. Mitigate with a smaller toolset (for example `WHATSAPP_MCP_TOOLSETS=core`), the send allowlist, and by keeping human approval switched on for write tools in your AI client.
- **Prompt injection.** Anyone who can message you can put text in front of your AI agent. Do not let an agent send messages, delete anything or call other connected tools without your approval. Avoid using this MCP in the same session as other connectors that can send email or messages unattended.
- **Data at rest is not encrypted.** Messages, contacts and the WhatsApp session live in plain SQLite files under `store/`, and received media is downloaded to disk. Use full-disk encryption (FileVault, BitLocker, LUKS) and treat `store/` like a password vault: never commit it or share it. The same goes for the `index-data` Docker volume: the search indexer keeps a plain-text copy of your message text there.
- **Secrets in `.env`.** `API_KEY` and `WEB_UI_PASSWORD` are stored in plain text in `.env`, which is git-ignored. Keep it readable only by you (`chmod 600 .env`), and use strong values.
- **Sessions are in memory.** Restarting the bridge signs everyone out.
- **No TLS out of the box.** Traffic stays on your machine. If you expose the panel beyond `127.0.0.1`, put a TLS-terminating reverse proxy in front (the bridge honours `X-Forwarded-Proto: https` for the cookie `Secure` flag) and restrict who can reach it.
- **Inside Docker, client addresses are the gateway's.** Session addresses and login throttling use the Docker network gateway rather than the browser's real address.
- **Malicious media.** Downloaded files are written to the local filesystem. Scan untrusted files before opening them.

### Account risk

This project uses the unofficial WhatsApp Web protocol. That is against WhatsApp's terms of service, and WhatsApp can restrict or ban accounts that use it. Read-only use lowers the risk but does not remove it. Do not use it for bulk or unsolicited messaging, and prefer a number you can afford to lose. You use it at your own risk. See the disclaimer in the [README](README.md#disclaimer).

## Hardening checklist

- [ ] Set a long random `API_KEY` and a strong `WEB_UI_PASSWORD` (`openssl rand -hex 32`, `openssl rand -base64 18`).
- [ ] `chmod 600 .env`, and never commit `.env`, `store/`.
- [ ] Enable full-disk encryption.
- [ ] Expose only the toolsets you need with `WHATSAPP_MCP_TOOLSETS`.
- [ ] Set `WHATSAPP_ALLOWLIST_JIDS` if the agent should only message specific people or groups.
- [ ] Require approval for write tools in your AI client.
- [ ] Keep the ports on `127.0.0.1`.
- [ ] Rebuild regularly to pick up dependency updates (`docker compose build --pull`).
