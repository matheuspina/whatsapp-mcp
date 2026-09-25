# Single-image build: bridge (Go), MCP server (Python) and web panel (Next.js + nginx) in one container.
# Made for platforms that take exactly one file called Dockerfile (Coolify's "Dockerfile" build pack).
# docker-compose.yaml with Dockerfile.bridge / Dockerfile.mcp / Dockerfile.web-ui is still the way to run
# the three services separately. See docs/coolify.md.
#
#   public port 8080 (nginx)  /         web panel (static)
#                             /api/     bridge REST API      -> 127.0.0.1:8180
#                             /mcp ...  MCP server + OAuth   -> 127.0.0.1:8081 (only when MCP_PUBLIC_URL is set)

# --- Go bridge. Built on bookworm so the binary runs on the bookworm runtime below (same glibc). ---
FROM golang:1.25-bookworm AS bridge-builder
ENV CGO_ENABLED=1
WORKDIR /src
COPY whatsapp-bridge/go.mod whatsapp-bridge/go.sum ./
RUN go mod download
COPY whatsapp-bridge/ ./
RUN go build -trimpath -o /out/whatsapp-bridge .

# --- Web panel: static export. The API base is /api because nginx serves the panel and the bridge on one origin. ---
# Runs on the build machine's own architecture: the output is static files, identical for every platform, and
# next build (Turbopack) crashes under QEMU emulation.
FROM --platform=$BUILDPLATFORM node:20-alpine AS web-builder
WORKDIR /app
COPY whatsapp-web-ui/package*.json ./
RUN npm ci
COPY whatsapp-web-ui/ ./
ENV NEXT_PUBLIC_API_BASE_URL=/api
RUN npm run build

# --- MCP server dependencies in an isolated venv (build tools stay out of the final image) ---
FROM python:3.13-slim-bookworm AS mcp-builder
WORKDIR /app
COPY whatsapp-mcp-server/requirements.txt .
# gradio/gradio_client are only used by gradio-main.py (local dev UI), not by main.py.
# pip rather than uv: uv crashes under QEMU, which multi-arch builds on an arm64 machine need for amd64.
RUN grep -vE "^gradio" requirements.txt > requirements-docker.txt \
    && python -m venv /opt/venv \
    && /opt/venv/bin/pip install --no-cache-dir -r requirements-docker.txt

# --- Runtime ---
FROM python:3.13-slim-bookworm AS runtime

# gosu drops root after fixing volume ownership, tini reaps children and forwards signals, wget is for HEALTHCHECK
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    ffmpeg \
    gosu \
    nginx \
    tini \
    wget \
    && rm -rf /var/lib/apt/lists/* /usr/share/nginx/html/*

ARG APP_UID=1000
ARG APP_GID=1000
RUN groupadd -g ${APP_GID} appuser && useradd -l -u ${APP_UID} -g appuser appuser

COPY --from=mcp-builder /opt/venv /opt/venv
ENV PATH="/opt/venv/bin:$PATH" \
    TZ=UTC

WORKDIR /app
COPY --from=bridge-builder /out/whatsapp-bridge /app/whatsapp-bridge/whatsapp-bridge
COPY whatsapp-mcp-server /app/whatsapp-mcp-server
COPY --from=web-builder /app/out /usr/share/nginx/html
COPY docker/nginx.conf /etc/nginx/nginx.conf
COPY docker/nginx-mcp-routes.conf /etc/nginx/mcp-routes.conf
COPY docker/entrypoint.sh /app/entrypoint.sh

# /app/store holds the WhatsApp session and the message database: mount a persistent volume there.
# The bridge reads it as ./store from its own directory, the MCP server as /app/store.
RUN mkdir -p /app/store /app/media \
    && ln -s /app/store /app/whatsapp-bridge/store \
    && chmod +x /app/entrypoint.sh /app/whatsapp-bridge/whatsapp-bridge \
    && chown -R appuser:appuser /app

EXPOSE 8080

# Checks that nginx serves the panel, not /api/health: that endpoint answers 503 whenever WhatsApp is not connected
# (for example before pairing), which would mark the container unhealthy and make the proxy drop it, so the panel
# needed to pair would be unreachable. A bridge or MCP crash needs no probe: entrypoint.sh exits when any process does.
HEALTHCHECK --interval=30s --timeout=5s --start-period=60s --retries=3 \
    CMD wget -qO /dev/null http://127.0.0.1:8080/login/ || exit 1

ENTRYPOINT ["/usr/bin/tini", "--", "/app/entrypoint.sh"]
