# Authentication

WhatsApp MCP has two ways to authenticate, for two kinds of caller.

| Caller | Credential | Where it comes from |
|--------|-----------|---------------------|
| A person using the web panel | Username and password, exchanged for a session cookie | `WEB_UI_USERNAME`, `WEB_UI_PASSWORD` in `.env` |
| The MCP server and scripts | `X-API-Key` header | `API_KEY` in `.env` |

Both are accepted by every protected bridge endpoint. The panel never needs, sees or stores the API key.

## Web panel login

1. Set `WEB_UI_USERNAME` and `WEB_UI_PASSWORD` in `.env`, then start the stack. The bridge logs `Web UI login enabled`.
2. Open `http://127.0.0.1:8090`. Without a session you are sent to the sign-in page.
3. Signing in makes the bridge create a **session on the server** and send back one cookie:

   ```
   Set-Cookie: wa_session=<random token>; Path=/; HttpOnly; SameSite=Strict
   ```

   The cookie is `Secure` when the request came over HTTPS. There is no `Max-Age`, so it is a browser-session cookie.
   The token is 256 random bits and is **not** part of the response body, so scripts running in the page can never read it.
4. Every request the panel makes carries the cookie automatically (`credentials: "include"`). The panel keeps no credential
   in `localStorage` or `sessionStorage`; it asks the bridge who it is (`GET /api/auth/me`) whenever it loads.

Sessions expire after `WEB_UI_SESSION_TTL` without use (default 24 hours). Any request pushes the expiry forward.

### Active sessions

*Settings → Active sessions* lists everyone currently signed in, as the bridge sees it, so the list is the same from any
browser. Each row shows the browser and operating system, the address, when it signed in and when it was last active.
The current session is marked. Any session can end any other one.

### Endpoints

| Method and path | Auth | Purpose |
|-----------------|------|---------|
| `POST /api/auth/login` | none | Body `{"username","password"}`. Sets the cookie. Answers `401` on bad credentials, `429` when throttled, `501` when login is not configured. |
| `POST /api/auth/logout` | none | Ends the caller's session and clears the cookie. Safe to call when already signed out. |
| `GET /api/auth/me` | cookie | `{"username","expires_at"}`, or `401`. |
| `GET /api/auth/sessions` | cookie or API key | Active sessions, with `current` marked. Never includes tokens. |
| `DELETE /api/auth/sessions/{id}` | cookie or API key | Ends one session by its public id. |

Errors are JSON: `{"success": false, "error": "..."}`.

## Protections

- **Constant-time comparison** for the username, password and API key.
- **Login throttling.** After 5 failed attempts within 5 minutes from one address, further attempts get `429` with
  `Retry-After`, even with the right password. A successful login resets the count. The address is the TCP peer;
  `X-Forwarded-For` is ignored because a client can forge it.
- **CSRF.** A cookie is sent by the browser automatically, so a request that changes state (`POST`, `PUT`, `DELETE`) and is
  authenticated by cookie must also carry an `Origin` header on the CORS allowlist, otherwise it gets `403`.
  `SameSite=Strict` alone is not enough, because another local port such as `localhost:3000` counts as the same site.
  Requests authenticated by `X-API-Key` are not subject to this check.
- **Public session ids.** Sessions have a short public id used for listing and ending them, separate from the secret token.
- **Nothing sensitive in logs.** The bridge does not print `API_KEY` or session tokens. Failed logins are written to the
  audit log with the address and user agent.

## Limits worth knowing

- Sessions are held **in memory**. Restarting the bridge signs everyone out.
- The password and API key are plain text in `.env`. Keep the file private (`chmod 600 .env`).
- Behind Docker, the address recorded for a session, and used for throttling, is the network gateway's, not the browser's.
  For one user this makes the throttle effectively global for the panel.
- There is no TLS by default. If you expose the panel beyond `127.0.0.1`, use a reverse proxy that terminates TLS and
  sets `X-Forwarded-Proto: https`.
- There is one account. Multiple users or roles are not supported.
- The MCP endpoint is **not** covered by any of this. See [SECURITY.md](../SECURITY.md#known-limitations).

## Rotating credentials

```bash
openssl rand -hex 32        # new API_KEY
openssl rand -base64 18     # new WEB_UI_PASSWORD
```

Edit `.env`, then `docker compose up -d`. Changing `API_KEY` restarts the bridge and the MCP server so they pick it up;
changing the password or restarting the bridge ends all panel sessions. Your WhatsApp pairing is unaffected (it lives in `store/`).
