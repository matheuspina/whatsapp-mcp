# Deploying on Coolify (single Dockerfile)

The `Dockerfile` in the repository root builds one image that runs the three services together: the Go bridge, the
Python MCP server and the web panel. It exists for platforms that accept exactly one file called `Dockerfile`, such as
Coolify's **Dockerfile** build pack. `docker-compose.yaml` (with `Dockerfile.bridge`, `Dockerfile.mcp` and
`Dockerfile.web-ui`) remains the way to run them as separate containers.

```
public port 8080 (nginx)   /          web panel
                           /api/      bridge REST API      -> 127.0.0.1:8180  (loopback only)
                           /mcp ...   MCP server + OAuth   -> 127.0.0.1:8081  (only when MCP_PUBLIC_URL is set)
```

Everything is served from one origin, so the panel needs no CORS setup and the browser talks to `/api`.

## Coolify settings

| Setting | Value |
|---------|-------|
| Build pack | Dockerfile (path `/Dockerfile`, base directory `/`) |
| Ports Exposes | `8080` |
| Domain | `https://whatsapp.example.com` |
| Persistent storage | a volume mounted at `/app/store` |
| Instances | one (a WhatsApp session cannot be shared by two processes) |

Environment variables:

| Variable | Required | Description |
|----------|----------|-------------|
| `API_KEY` | yes | Bridge API key. Generate with `openssl rand -hex 32`. |
| `WEB_UI_USERNAME`, `WEB_UI_PASSWORD` | yes | Panel login. They are also the MCP sign-in. Not the example values. |
| `MCP_PUBLIC_URL` | for MCP | The origin of the domain above, for example `https://whatsapp.example.com`. Without it the MCP server is not started and `/mcp` answers 404. |
| `CORS_ORIGINS` | no | Defaults to `MCP_PUBLIC_URL`. Set it yourself if the panel is served from a different origin. |

Any other variable in [configuration.md](configuration.md) works too. The container fixes the internal ports and
addresses itself (`API_PORT`, `API_BIND_HOST`, `HOST`, `PORT`, `BRIDGE_HOST`, `MCP_TRANSPORT`), so setting those has no effect.

## After the first deploy

1. Open the domain, sign in and pair WhatsApp from the Pairing page.
2. Open the **MCP** page, put `https://whatsapp.example.com/mcp` in Server URL and follow the card for your client.

## Things to know

- **Persistent storage is not optional.** `/app/store` holds the WhatsApp session and every message. Without a volume
  the container logs a warning and the device has to be paired again after each redeploy.
- **MCP is off without `MCP_PUBLIC_URL`.** The endpoint has no login of its own; with OAuth on, clients sign in with the panel
  credentials (see [mcp-oauth.md](mcp-oauth.md)). It is never exposed publicly without it.
- **Health check.** The image checks `/login/` (nginx up), not `/api/health`, which answers 503 while WhatsApp is not connected,
  for example before pairing. With that endpoint the proxy would drop the container and the pairing page would be unreachable.
  If you set a health check in Coolify, use `/login/` too.
- **Redeploys.** If Coolify starts the new container before stopping the old one, both use the same WhatsApp session and
  volume and WhatsApp disconnects one of them. If you see that, turn the health check off in Coolify so it stops the old container first.
- **The search indexer is not included.** It is the fourth service in `docker-compose.yaml` and stays a separate container.
- **Any process that exits stops the container**, so Coolify restarts the whole stack rather than leaving it half working.
- Everything runs as an unprivileged user; the entrypoint only starts as root to fix the ownership of a fresh volume.
