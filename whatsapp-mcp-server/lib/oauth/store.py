"""SQLite persistence for OAuth clients, authorization requests, codes and tokens.

Tokens, codes and client secrets are stored as SHA-256 digests, so a copy of the database file does
not reveal a usable credential. State survives restarts, which is what keeps connected AI clients
signed in across `docker compose up -d --build`.

The database is private to the MCP server (`mcp_oauth.db` next to `messages.db`) and creates its own
tables idempotently; it is not part of the bridge schema and needs no migration script.
"""

from __future__ import annotations

import hashlib
import json
import os
import sqlite3
import threading
import time
from dataclasses import dataclass

from .clients import Client

MAX_CLIENTS = 1000

_SCHEMA = """
CREATE TABLE IF NOT EXISTS oauth_clients (
    client_id TEXT PRIMARY KEY,
    data TEXT NOT NULL,
    created_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS oauth_auth_requests (
    id_hash TEXT PRIMARY KEY,
    data TEXT NOT NULL,
    expires_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS oauth_codes (
    code_hash TEXT PRIMARY KEY,
    data TEXT NOT NULL,
    expires_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS oauth_tokens (
    access_hash TEXT PRIMARY KEY,
    refresh_hash TEXT NOT NULL UNIQUE,
    family_id TEXT NOT NULL,
    client_id TEXT NOT NULL,
    scope TEXT NOT NULL,
    resource TEXT,
    access_expires_at INTEGER NOT NULL,
    refresh_expires_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_oauth_tokens_family ON oauth_tokens(family_id);
CREATE TABLE IF NOT EXISTS oauth_used_refresh (
    refresh_hash TEXT PRIMARY KEY,
    family_id TEXT NOT NULL,
    expires_at INTEGER NOT NULL
);
"""


def digest(value: str) -> str:
    """SHA-256 hex digest used to store secrets at rest."""
    return hashlib.sha256(value.encode("utf-8")).hexdigest()


@dataclass
class AuthRequest:
    """A validated /authorize request waiting for the user to sign in."""

    client_id: str
    redirect_uri: str
    redirect_uri_explicit: bool
    code_challenge: str
    scope: str
    state: str | None = None
    resource: str | None = None


@dataclass
class AuthCode:
    """A one-time authorization code awaiting exchange at /token."""

    client_id: str
    redirect_uri: str
    redirect_uri_explicit: bool
    code_challenge: str
    scope: str
    resource: str | None = None


@dataclass
class TokenRecord:
    """An issued access/refresh token pair (only digests are persisted)."""

    access_hash: str
    refresh_hash: str
    family_id: str
    client_id: str
    scope: str
    resource: str | None
    access_expires_at: int
    refresh_expires_at: int


class OAuthStore:
    """Thread-safe SQLite store. Use ``":memory:"`` for tests."""

    def __init__(self, path: str) -> None:
        if path != ":memory:":
            os.makedirs(os.path.dirname(path) or ".", exist_ok=True)
        self._lock = threading.RLock()
        self._conn = sqlite3.connect(path, check_same_thread=False, isolation_level=None)
        self._conn.row_factory = sqlite3.Row
        with self._lock:
            self._conn.executescript(_SCHEMA)
        if path != ":memory:":
            try:
                os.chmod(path, 0o600)
            except OSError:
                pass

    def close(self) -> None:
        with self._lock:
            self._conn.close()

    # --- clients -----------------------------------------------------------------------------

    def save_client(self, client: Client) -> None:
        payload = json.dumps(client.__dict__)
        with self._lock:
            self._conn.execute(
                "INSERT OR REPLACE INTO oauth_clients (client_id, data, created_at) VALUES (?, ?, ?)",
                (client.client_id, payload, client.created_at),
            )
            self._prune_clients()

    def get_client(self, client_id: str) -> Client | None:
        with self._lock:
            row = self._conn.execute("SELECT data FROM oauth_clients WHERE client_id = ?", (client_id,)).fetchone()
        if row is None:
            return None
        return Client(**json.loads(row["data"]))

    def _prune_clients(self) -> None:
        """Drop the oldest clients that hold no token once the registry is over MAX_CLIENTS."""
        count = self._conn.execute("SELECT COUNT(*) FROM oauth_clients").fetchone()[0]
        if count <= MAX_CLIENTS:
            return
        self._conn.execute(
            """DELETE FROM oauth_clients WHERE client_id IN (
                   SELECT client_id FROM oauth_clients
                   WHERE client_id NOT IN (SELECT DISTINCT client_id FROM oauth_tokens)
                   ORDER BY created_at ASC LIMIT ?)""",
            (count - MAX_CLIENTS,),
        )

    # --- authorization requests --------------------------------------------------------------

    def save_auth_request(self, request_id: str, req: AuthRequest, ttl: int) -> None:
        with self._lock:
            self._purge_expired()
            self._conn.execute(
                "INSERT INTO oauth_auth_requests (id_hash, data, expires_at) VALUES (?, ?, ?)",
                (digest(request_id), json.dumps(req.__dict__), int(time.time()) + ttl),
            )

    def get_auth_request(self, request_id: str) -> AuthRequest | None:
        """Read a pending request without consuming it (used to redisplay the form after a bad password)."""
        with self._lock:
            row = self._conn.execute(
                "SELECT data, expires_at FROM oauth_auth_requests WHERE id_hash = ?", (digest(request_id),)
            ).fetchone()
        if row is None or row["expires_at"] < time.time():
            return None
        return AuthRequest(**json.loads(row["data"]))

    def pop_auth_request(self, request_id: str) -> AuthRequest | None:
        with self._lock:
            row = self._conn.execute(
                "SELECT data, expires_at FROM oauth_auth_requests WHERE id_hash = ?", (digest(request_id),)
            ).fetchone()
            self._conn.execute("DELETE FROM oauth_auth_requests WHERE id_hash = ?", (digest(request_id),))
        if row is None or row["expires_at"] < time.time():
            return None
        return AuthRequest(**json.loads(row["data"]))

    # --- authorization codes -----------------------------------------------------------------

    def save_code(self, code: str, auth_code: AuthCode, ttl: int) -> None:
        with self._lock:
            self._conn.execute(
                "INSERT INTO oauth_codes (code_hash, data, expires_at) VALUES (?, ?, ?)",
                (digest(code), json.dumps(auth_code.__dict__), int(time.time()) + ttl),
            )

    def pop_code(self, code: str) -> AuthCode | None:
        """Consume a code. It is deleted even when expired, so a code can never be used twice."""
        with self._lock:
            row = self._conn.execute(
                "SELECT data, expires_at FROM oauth_codes WHERE code_hash = ?", (digest(code),)
            ).fetchone()
            self._conn.execute("DELETE FROM oauth_codes WHERE code_hash = ?", (digest(code),))
        if row is None or row["expires_at"] < time.time():
            return None
        return AuthCode(**json.loads(row["data"]))

    # --- tokens ------------------------------------------------------------------------------

    def save_tokens(self, rec: TokenRecord) -> None:
        with self._lock:
            self._conn.execute(
                """INSERT INTO oauth_tokens (access_hash, refresh_hash, family_id, client_id, scope, resource,
                                             access_expires_at, refresh_expires_at)
                   VALUES (?, ?, ?, ?, ?, ?, ?, ?)""",
                (
                    rec.access_hash,
                    rec.refresh_hash,
                    rec.family_id,
                    rec.client_id,
                    rec.scope,
                    rec.resource,
                    rec.access_expires_at,
                    rec.refresh_expires_at,
                ),
            )

    def get_access(self, access_token: str) -> TokenRecord | None:
        with self._lock:
            row = self._conn.execute(
                "SELECT * FROM oauth_tokens WHERE access_hash = ?", (digest(access_token),)
            ).fetchone()
        if row is None or row["access_expires_at"] < time.time():
            return None
        return TokenRecord(**dict(row))

    def peek_refresh(self, refresh_token: str) -> TokenRecord | None:
        with self._lock:
            row = self._conn.execute(
                "SELECT * FROM oauth_tokens WHERE refresh_hash = ?", (digest(refresh_token),)
            ).fetchone()
        return TokenRecord(**dict(row)) if row else None

    def consume_refresh(self, refresh_token: str) -> tuple[TokenRecord | None, bool]:
        """Rotate-out a refresh token.

        Returns ``(record, reused)``. ``record`` is the row that was removed (None when unknown or
        expired). ``reused`` is True when the token was already rotated out before: the whole token
        family is revoked then, because a second use means the token leaked (OAuth 2.1 section 4.3.1).
        """
        h = digest(refresh_token)
        now = int(time.time())
        with self._lock:
            row = self._conn.execute("SELECT * FROM oauth_tokens WHERE refresh_hash = ?", (h,)).fetchone()
            if row is None:
                used = self._conn.execute(
                    "SELECT family_id FROM oauth_used_refresh WHERE refresh_hash = ?", (h,)
                ).fetchone()
                if used is not None:
                    self.revoke_family(used["family_id"])
                    return None, True
                return None, False
            rec = TokenRecord(**dict(row))
            self._conn.execute("DELETE FROM oauth_tokens WHERE refresh_hash = ?", (h,))
            self._conn.execute(
                "INSERT OR REPLACE INTO oauth_used_refresh (refresh_hash, family_id, expires_at) VALUES (?, ?, ?)",
                (h, rec.family_id, rec.refresh_expires_at),
            )
        if rec.refresh_expires_at < now:
            return None, False
        return rec, False

    def revoke_family(self, family_id: str) -> None:
        with self._lock:
            self._conn.execute("DELETE FROM oauth_tokens WHERE family_id = ?", (family_id,))

    def revoke_token(self, token: str, client_id: str) -> None:
        """Revoke the pair that token (access or refresh) belongs to, when it belongs to client_id."""
        h = digest(token)
        with self._lock:
            row = self._conn.execute(
                "SELECT family_id, client_id FROM oauth_tokens WHERE access_hash = ? OR refresh_hash = ?", (h, h)
            ).fetchone()
            if row is not None and row["client_id"] == client_id:
                self.revoke_family(row["family_id"])

    def _purge_expired(self) -> None:
        now = int(time.time())
        self._conn.execute("DELETE FROM oauth_auth_requests WHERE expires_at < ?", (now,))
        self._conn.execute("DELETE FROM oauth_codes WHERE expires_at < ?", (now,))
        self._conn.execute("DELETE FROM oauth_tokens WHERE refresh_expires_at < ?", (now,))
        self._conn.execute("DELETE FROM oauth_used_refresh WHERE expires_at < ?", (now,))
