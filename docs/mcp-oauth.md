# MCP OAuth 2.1

By default the MCP endpoint has no login and is reachable only from your machine. Setting `MCP_PUBLIC_URL` turns on
**OAuth 2.1**: the endpoint then requires a bearer token, and AI clients obtain one by signing you in through a page served
by the MCP server itself. This is the standard flow MCP clients (Claude, Cursor, ChatGPT, Codex, VS Code, Antigravity and
others) implement, so you can expose the server on a domain and connect a client by pasting one URL.

It applies to the `streamable-http` and `sse` transports. `stdio` has no network endpoint and is unaffected.

## Turn it on

1. Pick the public origin your AI client will use. It must be `https`, except `http` on `localhost` / `127.0.0.1`.
   Put a reverse proxy that terminates TLS in front of `127.0.0.1:8081` (the MCP port stays bound to localhost).
2. In `.env`:

   ```bash
   MCP_PUBLIC_URL=https://mcp.example.com   # origin only: no path
   WEB_UI_USERNAME=admin
   WEB_UI_PASSWORD=<a strong password>
   ```

3. `docker compose up -d --build whatsapp-mcp`
4. Point the client at `https://mcp.example.com/mcp`. It discovers everything else by itself and opens the sign-in page.

The sign-in page checks the **same username and password as the web panel**. There is one account.
The MCP server refuses to start when `MCP_PUBLIC_URL` is set and those credentials are missing or still `CHANGEME...`.

| Variable | Default | Description |
|----------|---------|-------------|
| `MCP_PUBLIC_URL` | *(unset, OAuth off)* | Public origin of the MCP server. Also the OAuth issuer. |
| `MCP_OAUTH_ACCESS_TOKEN_TTL` | `3600` | Access token lifetime, in seconds. |
| `MCP_OAUTH_REFRESH_TOKEN_TTL` | `2592000` | Refresh token lifetime (30 days), in seconds. |

## What is implemented

| Standard | Where |
|----------|-------|
| Protected resource metadata, RFC 9728 | `/.well-known/oauth-protected-resource` and `.../mcp`; `401` carries `WWW-Authenticate: Bearer resource_metadata=...` |
| Authorization server metadata, RFC 8414 | `/.well-known/oauth-authorization-server`, `/.well-known/openid-configuration`, and their `/mcp` suffixed forms |
| Dynamic client registration, RFC 7591 | `POST /register` |
| Authorization code + PKCE (`S256` only), OAuth 2.1 | `GET /authorize`, `POST /token` |
| Refresh tokens with rotation and reuse detection | `POST /token`, `grant_type=refresh_token` |
| Token revocation, RFC 7009 | `POST /revoke` |
| Resource indicators, RFC 8707 | `resource` is accepted when it names this server, rejected otherwise |
| Issuer identification, RFC 9207 | `iss` is sent on every authorization response |
| Client authentication | `none` (public), `client_secret_post`, `client_secret_basic` |

Flow, from the client's point of view:

```
client ─ POST /mcp ───────────────▶ 401 + resource_metadata
client ─ GET  /.well-known/... ───▶ where the authorization server is
client ─ POST /register ──────────▶ client_id
client ─ browser ─ GET /authorize ▶ sign-in and consent page ─ POST /oauth/login ─▶ redirect with ?code=
client ─ POST /token ─────────────▶ access_token + refresh_token
client ─ POST /mcp + Bearer ──────▶ tools
```

Tokens issued this way carry one scope, `mcp:tools`. `offline_access` and any other scope a client asks for are accepted
and ignored, because several clients send them by default.

### Scripts and clients without OAuth

`API_KEY` is also accepted as a bearer token, which suits scripts and `claude mcp add --header`:

```bash
claude mcp add --transport http whatsapp https://mcp.example.com/mcp --header "Authorization: Bearer $API_KEY"
```

The key is broader than an OAuth token (it also opens the bridge REST API), so keep it out of shared configs.

## Connecting clients

Every client below only needs the URL `https://<your MCP_PUBLIC_URL>/mcp`. They register themselves through
`POST /register`, so no client id or secret is configured by hand.

| Client | Redirect it uses | Notes |
|--------|------------------|-------|
| Claude (web, desktop, mobile) | `https://claude.ai/api/mcp/auth_callback` | Add it as a custom connector. |
| Claude Code | `http://localhost:<port>/callback` | `claude mcp add --transport http ...`, then `/mcp` to sign in. |
| Cursor | `cursor://...` | A hand-off page with an "Open the application" link is shown after sign-in. |
| ChatGPT, Codex | `https://chatgpt.com/...`, `http://127.0.0.1:<port>/...` | Codex may use the out-of-band page (see below). |
| VS Code | `https://vscode.dev/redirect`, `http://127.0.0.1:<port>/` | |
| Google Antigravity | `https://antigravity.google/...`, `antigravity://...` | |
| Grok and others | whatever they register | Works for any client that follows the MCP authorization spec. |

Verified end to end here: the official MCP Python SDK client (discovery, registration, PKCE, tool listing) and the automated
tests. The rows above come from each client's published redirect URIs and the MCP authorization spec, and were not run against
the real products; if one fails, the server log shows the rejected step.

**Out-of-band clients.** A client that registers `urn:ietf:wg:oauth:2.0:oob` gets a page that shows the authorization code to
copy into the terminal instead of a redirect.

**Fixed client ids.** Clients configured by hand with a `client_id` and no registration step can use one of the built-in
public clients: `claude`, `claude-ai`, `claude-code`, `chatgpt`, `openai`, `codex`, `openai-codex`, `cursor`, `cursor-ide`,
`vscode`, `antigravity`, `google-antigravity`, `antigravity-ide`, `antigravity-cli`, `gemini`. They use PKCE and no secret, and can
only redirect to the URIs built into the server.

## Security model

- **Redirect URIs are validated, never trusted.** A registration is refused unless each URI is `https`, `http` on loopback,
  a private-use scheme (`cursor://`, `vscode://`...) or the out-of-band URN. At `/authorize` the URI must match a registered one
  exactly; loopback URIs may differ in port only (RFC 8252). An unknown client or unregistered URI produces an error page, never a
  redirect, so the server cannot be used as an open redirect to collect codes.
- **You approve every connection.** Registration is open (the spec requires it for clients to connect), so the sign-in page is
  the gate: it names the client and where it returns to, and needs your password every time. Only continue for connections you started.
- **PKCE is mandatory**, `S256` only. Codes are single use and last 60 seconds.
- **Refresh tokens rotate.** Presenting an already-used refresh token is treated as theft and revokes that whole chain.
- **Secrets at rest.** Tokens, codes and client secrets are stored as SHA-256 digests in `store/mcp_oauth.db` (mode `0600`).
  Tokens survive a restart, so connected clients stay signed in across rebuilds. Delete the file to sign every client out.
- **Login throttling.** 5 failed sign-ins per address in 5 minutes, and 30 in total, then `429` with `Retry-After`.
  The address is the TCP peer (`X-Forwarded-For` is ignored because it can be forged), so behind a reverse proxy the limit is effectively global.
- **Sign-in page hardening.** No JavaScript, strict CSP, `X-Frame-Options: DENY`, `no-referrer`, `no-store`.
- **DNS rebinding.** With OAuth on, the SDK's localhost-only `Host` allow-list is switched off, since it would reject your public
  domain. The bearer token is what protects the endpoint.

## Limits worth knowing

- One account, the panel's. There are no per-user tokens or roles, and a token grants every enabled toolset.
  Combine it with [`WHATSAPP_MCP_TOOLSETS`](configuration.md#mcp-server) to expose less.
- Nothing in this stack terminates TLS. Use a reverse proxy, and publish only the MCP port through it, not the bridge or panel.
- Discovery and `/register` are public by design. Registered clients are capped at 1000 (oldest unused ones are dropped first).
- The sign-in page has no session: each authorization asks for the password again.

## Revoking access

- One client: it calls `POST /revoke`, or you remove the connector in the client.
- Everything: stop the MCP container, delete `store/mcp_oauth.db`, start it again. Changing `WEB_UI_PASSWORD` stops new
  sign-ins but does not end tokens already issued.
