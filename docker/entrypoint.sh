#!/bin/bash
# Runs the bridge, the MCP server and nginx in one container and stops them together.
# Starts as root only to fix volume ownership, then re-runs itself as appuser.
set -euo pipefail

STORE=/app/store
BRIDGE_PORT=8180
MCP_PORT=8081

if [ "$(id -u)" = "0" ]; then
  mkdir -p "$STORE" /app/media /tmp/nginx

  if ! mountpoint -q "$STORE"; then
    echo "WARNING: $STORE is not a mounted volume. The WhatsApp session and every message are lost when the container is recreated. Add persistent storage at $STORE." >&2
  fi

  # A fresh volume is owned by root. Only walk the tree when the top level is wrong, the store can be large.
  app_uid="$(id -u appuser)"
  if [ "$(stat -c %u "$STORE")" != "$app_uid" ]; then
    chown -R appuser:appuser "$STORE"
  fi
  chown -R appuser:appuser /app/media /tmp/nginx

  # The MCP routes are only exposed through nginx when OAuth is on. Without it the endpoint has no login,
  # so those paths answer 404 instead of falling through to the panel's index.html.
  if [ -n "${MCP_PUBLIC_URL:-}" ]; then
    cp /etc/nginx/mcp-routes.conf /etc/nginx/mcp-active.conf
  else
    printf '%s\n' \
      'location ~ ^/(mcp|register|authorize|token|revoke|oauth/login)$ { return 404; }' \
      'location ^~ /.well-known/ { return 404; }' > /etc/nginx/mcp-active.conf
  fi

  exec gosu appuser "$0" "$@"
fi

# The panel and the API share one origin (the public URL), which is what the bridge's Origin check must allow.
export CORS_ORIGINS="${CORS_ORIGINS:-${MCP_PUBLIC_URL:-}}"

pids=()
stop_all() {
  trap - TERM INT
  kill "${pids[@]}" 2>/dev/null || true
  wait 2>/dev/null || true
}
trap 'stop_all; exit 143' TERM INT

# The bridge only listens on loopback: nginx is the single public entry point.
(cd /app/whatsapp-bridge && exec env API_PORT="$BRIDGE_PORT" API_BIND_HOST=127.0.0.1 ./whatsapp-bridge) &
bridge_pid=$!
pids+=("$bridge_pid")

if [ -n "${MCP_PUBLIC_URL:-}" ]; then
  # The MCP server refuses to import until the bridge has created messages.db.
  for _ in $(seq 1 120); do
    [ -f "$STORE/messages.db" ] && break
    if ! kill -0 "$bridge_pid" 2>/dev/null; then
      echo "The bridge exited before creating messages.db" >&2
      exit 1
    fi
    sleep 1
  done
  if [ ! -f "$STORE/messages.db" ]; then
    echo "Timed out waiting for $STORE/messages.db" >&2
    stop_all
    exit 1
  fi

  (cd /app/whatsapp-mcp-server && exec env MCP_TRANSPORT=streamable-http HOST=127.0.0.1 PORT="$MCP_PORT" \
    BRIDGE_HOST="127.0.0.1:$BRIDGE_PORT" python main.py) &
  pids+=("$!")
else
  echo "MCP_PUBLIC_URL is not set: the MCP server is not started. Set it to this deployment's public origin (for example https://whatsapp.example.com) to enable it." >&2
fi

nginx -e stderr -c /etc/nginx/nginx.conf -g 'daemon off;' &
pids+=("$!")

# Any process ending takes the container down, so the platform restarts it instead of leaving a half-working stack.
code=0
wait -n || code=$?
echo "A process exited with code $code, stopping the container" >&2
stop_all
exit "$code"
