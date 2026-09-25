"""OAuth client model, redirect URI policy and the well-known pre-registered clients."""

from __future__ import annotations

import re
import time
from dataclasses import dataclass, field
from urllib.parse import urlparse

from .config import LOOPBACK_HOSTS

OOB_URIS = frozenset({"urn:ietf:wg:oauth:2.0:oob", "urn:ietf:wg:oauth:2.0:oob:auto"})

# Schemes that must never receive an authorization code: they execute script or expose local files.
_FORBIDDEN_SCHEMES = frozenset({"javascript", "data", "file", "vbscript", "about", "blob", "ftp", "ws", "wss"})
_SCHEME_RE = re.compile(r"^[a-z][a-z0-9+.\-]*$")
_MAX_REDIRECT_URI_LEN = 2048

AUTH_METHODS = ("none", "client_secret_post", "client_secret_basic")


@dataclass
class Client:
    """A registered OAuth client (dynamic or pre-registered)."""

    client_id: str
    redirect_uris: list[str]
    client_name: str = ""
    token_endpoint_auth_method: str = "none"
    client_secret_hash: str | None = None
    grant_types: list[str] = field(default_factory=lambda: ["authorization_code", "refresh_token"])
    scope: str = ""
    created_at: int = field(default_factory=lambda: int(time.time()))

    @property
    def is_public(self) -> bool:
        return self.client_secret_hash is None

    @property
    def display_name(self) -> str:
        return self.client_name or self.client_id


def _parse(uri: str):
    try:
        return urlparse(uri)
    except ValueError:
        return None


def is_valid_redirect_uri(uri: str) -> bool:
    """Whether uri may be registered as a redirect URI.

    Accepted: https URLs, http URLs on loopback (RFC 8252 section 7.3), private-use custom schemes
    such as cursor:// or vscode:// (section 7.1) and the out-of-band URNs used by CLI clients.
    """
    if not isinstance(uri, str) or not uri or len(uri) > _MAX_REDIRECT_URI_LEN:
        return False
    if any(ch.isspace() or ord(ch) < 0x20 for ch in uri):
        return False
    if uri in OOB_URIS:
        return True
    parsed = _parse(uri)
    if parsed is None or not parsed.scheme or parsed.fragment:
        return False
    scheme = parsed.scheme.lower()
    if scheme == "https":
        return bool(parsed.hostname)
    if scheme == "http":
        return parsed.hostname in LOOPBACK_HOSTS
    if scheme in _FORBIDDEN_SCHEMES or not _SCHEME_RE.match(scheme):
        return False
    return bool(parsed.netloc or parsed.path.strip("/"))


def is_loopback_uri(uri: str) -> bool:
    parsed = _parse(uri)
    return bool(parsed and parsed.scheme == "http" and parsed.hostname in LOOPBACK_HOSTS)


def redirect_uri_allowed(registered: list[str], requested: str) -> bool:
    """Exact match, or for loopback URIs a match that ignores the port (RFC 8252 section 7.3)."""
    if requested in registered:
        return True
    if not is_loopback_uri(requested):
        return False
    wanted = _parse(requested)
    for candidate in registered:
        if not is_loopback_uri(candidate):
            continue
        known = _parse(candidate)
        if (
            wanted is not None
            and known is not None
            and known.hostname == wanted.hostname
            and known.path == wanted.path
            and known.query == wanted.query
        ):
            return True
    return False


# --- Well-known clients -------------------------------------------------------------------
# Most current clients register themselves through POST /register. These fixed client ids exist
# for clients configured by hand with a client_id and no registration step. They are public
# clients (PKCE only) and can only ever redirect to the URIs listed here.

_LOOPBACK = ["http://localhost/callback", "http://127.0.0.1/callback"]

_CLAUDE = [
    "https://claude.ai/api/mcp/auth_callback",
    "https://claude.com/api/mcp/auth_callback",
    "https://claude.ai/api/mcp/oauth_callback",
    *_LOOPBACK,
]
_CHATGPT = [
    "https://chatgpt.com/connector_platform_oauth_redirect",
    "https://chatgpt.com/connector/oauth/callback",
    "https://chatgpt.com/oauth/callback",
    "https://platform.openai.com/oauth/callback",
    *_LOOPBACK,
]
_CODEX = [*_CHATGPT, "urn:ietf:wg:oauth:2.0:oob", "urn:ietf:wg:oauth:2.0:oob:auto"]
_CURSOR = [
    "cursor://anysphere.cursor-mcp/oauth/callback",
    "cursor://anysphere.cursor-mcp/oauth",
    "https://www.cursor.com/agents/mcp/oauth/callback",
    *_LOOPBACK,
]
_VSCODE = [
    "https://vscode.dev/redirect",
    "vscode://vscode.github-authentication/did-authenticate",
    "http://127.0.0.1:33418/",
    *_LOOPBACK,
]
_ANTIGRAVITY = [
    "https://antigravity.google/oauth-callback",
    "antigravity://oauth-callback",
    "urn:ietf:wg:oauth:2.0:oob",
    "urn:ietf:wg:oauth:2.0:oob:auto",
    "http://localhost/oauth-callback",
    "http://127.0.0.1/oauth-callback",
    *_LOOPBACK,
]

_WELL_KNOWN: dict[str, tuple[str, list[str]]] = {
    "claude": ("Claude", _CLAUDE),
    "claude-ai": ("Claude", _CLAUDE),
    "claude-code": ("Claude Code", _CLAUDE),
    "chatgpt": ("ChatGPT", _CHATGPT),
    "openai": ("ChatGPT", _CHATGPT),
    "codex": ("OpenAI Codex", _CODEX),
    "openai-codex": ("OpenAI Codex", _CODEX),
    "cursor": ("Cursor", _CURSOR),
    "cursor-ide": ("Cursor", _CURSOR),
    "vscode": ("Visual Studio Code", _VSCODE),
    "antigravity": ("Google Antigravity", _ANTIGRAVITY),
    "google-antigravity": ("Google Antigravity", _ANTIGRAVITY),
    "antigravity-ide": ("Google Antigravity", _ANTIGRAVITY),
    "antigravity-cli": ("Google Antigravity", _ANTIGRAVITY),
    "gemini": ("Gemini / Antigravity", _ANTIGRAVITY),
}


def well_known_client(client_id: str) -> Client | None:
    """Return the built-in public client registered under client_id, if any."""
    entry = _WELL_KNOWN.get(client_id)
    if entry is None:
        return None
    name, uris = entry
    return Client(client_id=client_id, client_name=name, redirect_uris=list(uris))
