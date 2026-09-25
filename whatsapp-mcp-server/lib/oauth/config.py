"""Environment-driven configuration for the MCP OAuth 2.1 authorization server."""

from __future__ import annotations

import os
from collections.abc import Mapping
from dataclasses import dataclass
from urllib.parse import urlparse

SCOPE_TOOLS = "mcp:tools"
SCOPE_OFFLINE = "offline_access"
SUPPORTED_SCOPES = (SCOPE_TOOLS, SCOPE_OFFLINE)

LOOPBACK_HOSTS = frozenset({"localhost", "127.0.0.1", "::1"})

DEFAULT_ACCESS_TOKEN_TTL = 3600
DEFAULT_REFRESH_TOKEN_TTL = 30 * 24 * 3600
AUTHORIZATION_REQUEST_TTL = 600
AUTHORIZATION_CODE_TTL = 60


class OAuthConfigError(ValueError):
    """Raised when OAuth is enabled but its configuration is unusable."""


@dataclass(frozen=True)
class OAuthConfig:
    """Resolved settings for the OAuth authorization server and the protected MCP resource."""

    public_url: str  # issuer, scheme://host[:port], no trailing slash
    resource_path: str  # path of the MCP endpoint, for example /mcp
    username: str
    password: str
    api_key: str | None  # optional static bearer token for scripts and clients without OAuth
    db_path: str
    access_token_ttl: int = DEFAULT_ACCESS_TOKEN_TTL
    refresh_token_ttl: int = DEFAULT_REFRESH_TOKEN_TTL

    @property
    def issuer(self) -> str:
        """Issuer identifier. The trailing slash matches what the MCP SDK publishes in the protected
        resource metadata (authorization_servers), so both documents name the same issuer."""
        return f"{self.public_url}/"

    @property
    def resource_url(self) -> str:
        return f"{self.public_url}{self.resource_path}"

    def endpoint(self, path: str) -> str:
        return f"{self.public_url}{path}"


def is_placeholder(value: str) -> bool:
    """True for the CHANGEME... values shipped in .env.example."""
    return value.strip().upper().startswith("CHANGEME")


def _parse_public_url(raw: str) -> str:
    parsed = urlparse(raw)
    if parsed.scheme not in {"http", "https"} or not parsed.hostname:
        raise OAuthConfigError("MCP_PUBLIC_URL must be an absolute http(s) URL, for example https://mcp.example.com")
    if parsed.path not in {"", "/"} or parsed.query or parsed.fragment:
        raise OAuthConfigError("MCP_PUBLIC_URL must be the origin only (no path, query or fragment)")
    if parsed.scheme == "http" and parsed.hostname not in LOOPBACK_HOSTS:
        raise OAuthConfigError("MCP_PUBLIC_URL must use https unless it points at localhost or 127.0.0.1")
    return f"{parsed.scheme}://{parsed.netloc}"


def _positive_int(env: Mapping[str, str], name: str, default: int) -> int:
    raw = env.get(name, "").strip()
    if not raw:
        return default
    try:
        value = int(raw)
    except ValueError as exc:
        raise OAuthConfigError(f"{name} must be a whole number of seconds") from exc
    if value <= 0:
        raise OAuthConfigError(f"{name} must be greater than zero")
    return value


def load_config(
    *,
    resource_path: str,
    store_path: str,
    env: Mapping[str, str] | None = None,
) -> OAuthConfig | None:
    """Build the OAuth configuration, or return None when MCP_PUBLIC_URL is not set.

    OAuth is opt-in: without MCP_PUBLIC_URL the MCP endpoint keeps its historical open behaviour.
    Once it is set, the panel credentials are mandatory, because they are what the login page checks.

    Raises:
        OAuthConfigError: when OAuth is enabled but the configuration is invalid.
    """
    env = os.environ if env is None else env
    raw_url = env.get("MCP_PUBLIC_URL", "").strip()
    if not raw_url:
        return None

    username = env.get("WEB_UI_USERNAME", "").strip()
    password = env.get("WEB_UI_PASSWORD", "")
    if not username or not password:
        raise OAuthConfigError(
            "MCP_PUBLIC_URL is set, so WEB_UI_USERNAME and WEB_UI_PASSWORD are required: "
            "they are the credentials the OAuth login page checks"
        )
    if is_placeholder(password) or is_placeholder(username):
        raise OAuthConfigError("WEB_UI_USERNAME / WEB_UI_PASSWORD still have the CHANGEME example value")

    api_key = (env.get("API_KEY") or env.get("WHATSAPP_API_KEY") or "").strip() or None
    if api_key and is_placeholder(api_key):
        api_key = None

    return OAuthConfig(
        public_url=_parse_public_url(raw_url),
        resource_path=resource_path,
        username=username,
        password=password,
        api_key=api_key,
        db_path=os.path.join(store_path, "mcp_oauth.db"),
        access_token_ttl=_positive_int(env, "MCP_OAUTH_ACCESS_TOKEN_TTL", DEFAULT_ACCESS_TOKEN_TTL),
        refresh_token_ttl=_positive_int(env, "MCP_OAUTH_REFRESH_TOKEN_TTL", DEFAULT_REFRESH_TOKEN_TTL),
    )
