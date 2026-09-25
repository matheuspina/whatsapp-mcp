"""OAuth 2.1 for the MCP endpoint: authorization server, dynamic client registration and token verification.

Opt-in through ``MCP_PUBLIC_URL``. See docs/mcp-oauth.md.
"""

from __future__ import annotations

from dataclasses import dataclass
from typing import Any

from mcp.server.auth.settings import AuthSettings
from mcp.server.fastmcp import FastMCP
from mcp.server.transport_security import TransportSecuritySettings

from .config import SCOPE_TOOLS, OAuthConfig, OAuthConfigError, load_config
from .handlers import register_routes
from .service import OAuthService
from .store import OAuthStore

__all__ = ["OAuthConfig", "OAuthConfigError", "OAuthService", "OAuthSetup", "setup_oauth"]

_RESOURCE_PATHS = {"streamable-http": "/mcp", "sse": "/sse"}


@dataclass
class OAuthSetup:
    """Everything main.py needs to protect the MCP endpoint."""

    service: OAuthService

    def fastmcp_kwargs(self) -> dict[str, Any]:
        """Keyword arguments for FastMCP(...) that turn on bearer authentication and the discovery metadata."""
        cfg = self.service.config
        return {
            "auth": AuthSettings(
                issuer_url=cfg.issuer,  # type: ignore[arg-type]
                resource_server_url=cfg.resource_url,  # type: ignore[arg-type]
                required_scopes=[SCOPE_TOOLS],
            ),
            "token_verifier": self.service,
            # FastMCP's default allow-list only admits localhost Host headers, which would turn every
            # request to the public domain into a 421. A bearer token already stops DNS rebinding.
            "transport_security": TransportSecuritySettings(enable_dns_rebinding_protection=False),
        }

    def register(self, mcp: FastMCP) -> None:
        """Add the /authorize, /token, /register, /revoke and discovery routes to mcp."""
        register_routes(mcp, self.service)


def setup_oauth(transport: str, store_path: str) -> OAuthSetup | None:
    """Build the OAuth setup for an HTTP transport, or None when OAuth is off.

    Args:
        transport: the MCP transport in use. Only the HTTP transports can carry OAuth.
        store_path: directory that holds the SQLite files; the OAuth database lives there too.

    Raises:
        OAuthConfigError: when MCP_PUBLIC_URL is set but the rest of the configuration is unusable.
    """
    resource_path = _RESOURCE_PATHS.get(transport)
    if resource_path is None:
        return None
    config = load_config(resource_path=resource_path, store_path=store_path)
    if config is None:
        return None
    return OAuthSetup(service=OAuthService(config, OAuthStore(config.db_path)))
