"""OAuth 2.1 authorization server rules, independent of the HTTP layer."""

from __future__ import annotations

import base64
import hashlib
import hmac
import re
import secrets
import threading
import time
from collections import deque
from dataclasses import dataclass

from mcp.server.auth.provider import AccessToken

from lib.utils import logger

from .clients import AUTH_METHODS, Client, is_valid_redirect_uri, redirect_uri_allowed, well_known_client
from .config import (
    AUTHORIZATION_CODE_TTL,
    AUTHORIZATION_REQUEST_TTL,
    SCOPE_OFFLINE,
    SCOPE_TOOLS,
    SUPPORTED_SCOPES,
    OAuthConfig,
)
from .store import AuthCode, AuthRequest, OAuthStore, TokenRecord, digest

_PKCE_CHALLENGE_RE = re.compile(r"^[A-Za-z0-9_\-]{43}$")  # base64url(SHA-256) without padding
_PKCE_VERIFIER_RE = re.compile(r"^[A-Za-z0-9\-._~]{43,128}$")  # RFC 7636 section 4.1

MAX_REDIRECT_URIS = 10
MAX_CLIENT_NAME_LEN = 100

LOGIN_MAX_FAILURES = 5
LOGIN_GLOBAL_MAX_FAILURES = 30
LOGIN_WINDOW = 300


class OAuthError(Exception):
    """An OAuth protocol error, carrying the RFC error code and an HTTP status."""

    def __init__(self, error: str, description: str = "", status: int = 400) -> None:
        super().__init__(description or error)
        self.error = error
        self.description = description
        self.status = status


@dataclass
class TokenGrant:
    """Result of a successful token request."""

    access_token: str
    refresh_token: str
    expires_in: int
    scope: str


def _token() -> str:
    return secrets.token_urlsafe(32)


def _eq(a: str, b: str) -> bool:
    return hmac.compare_digest(a.encode("utf-8"), b.encode("utf-8"))


def pkce_s256(verifier: str) -> str:
    """The S256 code_challenge for verifier."""
    return base64.urlsafe_b64encode(hashlib.sha256(verifier.encode("ascii")).digest()).rstrip(b"=").decode("ascii")


def normalize_scope(requested: str | None) -> str:
    """Scopes actually granted. Unknown scopes (openid, profile, ...) are ignored rather than rejected,
    because several clients send them by default; mcp:tools is always granted."""
    asked = set((requested or "").split())
    granted = [SCOPE_TOOLS]
    if SCOPE_OFFLINE in asked:
        granted.append(SCOPE_OFFLINE)
    return " ".join(granted)


class LoginThrottle:
    """Failed-login limiter keyed on the peer address, plus a global ceiling.

    The peer address is the TCP peer, never X-Forwarded-For, which a client can forge. Behind a reverse
    proxy every caller shares the proxy's address, so the per-address limit then acts as a global one.
    """

    def __init__(self, now=time.monotonic) -> None:
        self._now = now
        self._lock = threading.Lock()
        self._by_key: dict[str, deque[float]] = {}
        self._all: deque[float] = deque()

    def _prune(self, q: deque[float]) -> None:
        cutoff = self._now() - LOGIN_WINDOW
        while q and q[0] <= cutoff:
            q.popleft()

    def retry_after(self, key: str) -> int:
        """Seconds to wait before another attempt, or 0 when the caller may try."""
        with self._lock:
            mine = self._by_key.get(key, deque())
            self._prune(mine)
            self._prune(self._all)
            waits = []
            if len(mine) >= LOGIN_MAX_FAILURES:
                waits.append(mine[0] + LOGIN_WINDOW - self._now())
            if len(self._all) >= LOGIN_GLOBAL_MAX_FAILURES:
                waits.append(self._all[0] + LOGIN_WINDOW - self._now())
            return max(1, int(max(waits))) if waits else 0

    def fail(self, key: str) -> None:
        with self._lock:
            now = self._now()
            self._by_key.setdefault(key, deque()).append(now)
            self._all.append(now)

    def reset(self, key: str) -> None:
        with self._lock:
            self._by_key.pop(key, None)


class OAuthService:
    """Authorization server logic: registration, authorization, token issuance, verification."""

    def __init__(self, config: OAuthConfig, store: OAuthStore) -> None:
        self.config = config
        self.store = store
        self.throttle = LoginThrottle()

    # --- clients -----------------------------------------------------------------------------

    def get_client(self, client_id: str | None) -> Client | None:
        if not client_id:
            return None
        return self.store.get_client(client_id) or well_known_client(client_id)

    def register_client(self, metadata: dict) -> tuple[Client, str | None]:
        """RFC 7591 dynamic registration. Returns the client and its plaintext secret, if it has one.

        Raises:
            OAuthError: ``invalid_redirect_uri`` or ``invalid_client_metadata``.
        """
        uris = metadata.get("redirect_uris")
        if not isinstance(uris, list) or not uris or len(uris) > MAX_REDIRECT_URIS:
            raise OAuthError("invalid_redirect_uri", f"redirect_uris must be a list of 1 to {MAX_REDIRECT_URIS} URIs")
        for uri in uris:
            if not is_valid_redirect_uri(uri):
                raise OAuthError("invalid_redirect_uri", f"redirect URI not allowed: {str(uri)[:200]}")

        method = metadata.get("token_endpoint_auth_method") or "client_secret_basic"
        if method not in AUTH_METHODS:
            raise OAuthError("invalid_client_metadata", f"unsupported token_endpoint_auth_method: {method}")

        grants = metadata.get("grant_types") or ["authorization_code"]
        if not isinstance(grants, list) or "authorization_code" not in grants:
            raise OAuthError("invalid_client_metadata", "grant_types must include authorization_code")
        grants = [g for g in grants if g in {"authorization_code", "refresh_token"}]

        name = metadata.get("client_name")
        name = name.strip()[:MAX_CLIENT_NAME_LEN] if isinstance(name, str) else ""

        secret = None if method == "none" else secrets.token_hex(32)
        client = Client(
            client_id=secrets.token_hex(16),
            client_name=name,
            redirect_uris=list(dict.fromkeys(uris)),
            token_endpoint_auth_method=method,
            client_secret_hash=digest(secret) if secret else None,
            grant_types=grants,
            scope=normalize_scope(metadata.get("scope")),
        )
        self.store.save_client(client)
        return client, secret

    def authenticate_client(self, client_id: str | None, secret: str | None) -> Client:
        """Authenticate the client at /token or /revoke.

        Raises:
            OAuthError: ``invalid_client`` (401) for an unknown client or a wrong or missing secret.
        """
        client = self.get_client(client_id)
        if client is None:
            raise OAuthError("invalid_client", "unknown client", 401)
        if client.client_secret_hash is not None:
            if not secret or not _eq(digest(secret), client.client_secret_hash):
                raise OAuthError("invalid_client", "client authentication failed", 401)
        return client

    # --- authorization -----------------------------------------------------------------------

    def accepts_resource(self, resource: str) -> bool:
        """RFC 8707: this server issues tokens for its MCP endpoint (or its origin) only."""
        return resource.rstrip("/") in {self.config.public_url, self.config.resource_url}

    def begin_authorization(
        self,
        client: Client,
        *,
        redirect_uri: str | None,
        code_challenge: str,
        code_challenge_method: str | None,
        scope: str | None,
        state: str | None,
        resource: str | None,
    ) -> tuple[str, AuthRequest]:
        """Validate an /authorize request (client and redirect_uri already checked) and park it.

        Returns the request id that ties the login form to this request, and the parked request.

        Raises:
            OAuthError: ``invalid_request`` or ``invalid_target``.
        """
        if code_challenge_method != "S256" or not _PKCE_CHALLENGE_RE.match(code_challenge or ""):
            raise OAuthError("invalid_request", "PKCE with code_challenge_method=S256 is required")
        if resource and not self.accepts_resource(resource):
            raise OAuthError("invalid_target", "unknown resource")

        explicit = redirect_uri is not None
        request = AuthRequest(
            client_id=client.client_id,
            redirect_uri=redirect_uri or client.redirect_uris[0],
            redirect_uri_explicit=explicit,
            code_challenge=code_challenge,
            scope=normalize_scope(scope),
            state=state,
            resource=resource,
        )
        request_id = _token()
        self.store.save_auth_request(request_id, request, AUTHORIZATION_REQUEST_TTL)
        return request_id, request

    def resolve_redirect_uri(self, client: Client, redirect_uri: str | None) -> str | None:
        """The redirect URI to use, or None when it is not acceptable for this client."""
        if redirect_uri is None:
            return client.redirect_uris[0] if len(client.redirect_uris) == 1 else None
        return redirect_uri if redirect_uri_allowed(client.redirect_uris, redirect_uri) else None

    def check_credentials(self, username: str, password: str) -> bool:
        """Constant-time comparison of both fields, evaluated before branching."""
        user_ok = _eq(username, self.config.username)
        pass_ok = _eq(password, self.config.password)
        return user_ok and pass_ok

    def issue_code(self, request: AuthRequest) -> str:
        code = _token()
        self.store.save_code(
            code,
            AuthCode(
                client_id=request.client_id,
                redirect_uri=request.redirect_uri,
                redirect_uri_explicit=request.redirect_uri_explicit,
                code_challenge=request.code_challenge,
                scope=request.scope,
                resource=request.resource,
            ),
            AUTHORIZATION_CODE_TTL,
        )
        return code

    # --- tokens ------------------------------------------------------------------------------

    def _mint(self, client_id: str, scope: str, resource: str | None, family_id: str | None = None) -> TokenGrant:
        now = int(time.time())
        access, refresh = _token(), _token()
        self.store.save_tokens(
            TokenRecord(
                access_hash=digest(access),
                refresh_hash=digest(refresh),
                family_id=family_id or _token(),
                client_id=client_id,
                scope=scope,
                resource=resource,
                access_expires_at=now + self.config.access_token_ttl,
                refresh_expires_at=now + self.config.refresh_token_ttl,
            )
        )
        return TokenGrant(access, refresh, self.config.access_token_ttl, scope)

    def exchange_code(
        self, client: Client, *, code: str | None, redirect_uri: str | None, code_verifier: str | None
    ) -> TokenGrant:
        """authorization_code grant, with PKCE and single-use codes.

        Raises:
            OAuthError: ``invalid_request`` or ``invalid_grant``.
        """
        if not code or not code_verifier:
            raise OAuthError("invalid_request", "code and code_verifier are required")
        stored = self.store.pop_code(code)
        if stored is None or stored.client_id != client.client_id:
            raise OAuthError("invalid_grant", "authorization code is invalid, expired or already used")
        if stored.redirect_uri_explicit and redirect_uri != stored.redirect_uri:
            raise OAuthError("invalid_grant", "redirect_uri does not match the authorization request")
        if not _PKCE_VERIFIER_RE.match(code_verifier) or not _eq(pkce_s256(code_verifier), stored.code_challenge):
            raise OAuthError("invalid_grant", "code_verifier does not match the code_challenge")
        return self._mint(client.client_id, stored.scope, stored.resource)

    def refresh(self, client: Client, *, refresh_token: str | None, scope: str | None) -> TokenGrant:
        """refresh_token grant. The refresh token is rotated; reusing an old one revokes the whole family.

        Raises:
            OAuthError: ``invalid_request``, ``invalid_grant`` or ``invalid_scope``.
        """
        if not refresh_token:
            raise OAuthError("invalid_request", "refresh_token is required")
        current = self.store.peek_refresh(refresh_token)
        if current is not None and current.client_id != client.client_id:
            raise OAuthError("invalid_grant", "refresh token was issued to another client")

        record, reused = self.store.consume_refresh(refresh_token)
        if reused:
            logger.warning("OAuth refresh token reuse detected for client %s; token family revoked", client.client_id)
        if record is None:
            raise OAuthError("invalid_grant", "refresh token is invalid or expired")

        granted = record.scope
        if scope:
            asked = set(scope.split())
            if not asked <= set(record.scope.split()):
                raise OAuthError("invalid_scope", "requested scope exceeds the original grant")
            granted = " ".join(s for s in record.scope.split() if s in asked) or SCOPE_TOOLS
        return self._mint(client.client_id, granted, record.resource, family_id=record.family_id)

    def revoke(self, client: Client, token: str) -> None:
        self.store.revoke_token(token, client.client_id)

    # --- resource server ---------------------------------------------------------------------

    async def verify_token(self, token: str) -> AccessToken | None:
        """TokenVerifier for the MCP endpoint. Accepts OAuth access tokens and, when set, API_KEY."""
        static = self.config.api_key
        if static and _eq(token, static):
            return AccessToken(token=token, client_id="api-key", scopes=[SCOPE_TOOLS], expires_at=None)
        record = self.store.get_access(token)
        if record is None:
            return None
        return AccessToken(
            token=token,
            client_id=record.client_id,
            scopes=record.scope.split(),
            expires_at=record.access_expires_at,
            resource=record.resource,
        )


__all__ = ["SUPPORTED_SCOPES", "LoginThrottle", "OAuthError", "OAuthService", "TokenGrant", "pkce_s256"]
