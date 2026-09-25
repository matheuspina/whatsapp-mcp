"""Starlette handlers for the OAuth 2.1 endpoints served next to the MCP endpoint.

Endpoints: discovery (RFC 9728 and RFC 8414, plus the OpenID Connect discovery paths some clients
probe), dynamic client registration (RFC 7591), /authorize with a sign-in and consent page, /token
(authorization_code and refresh_token, PKCE S256) and /revoke (RFC 7009).
"""

from __future__ import annotations

import base64
import binascii
import json
from collections.abc import Awaitable, Callable
from typing import Any
from urllib.parse import parse_qsl, unquote, urlencode, urlparse, urlunparse

from mcp.server.fastmcp import FastMCP
from starlette.requests import Request
from starlette.responses import HTMLResponse, JSONResponse, RedirectResponse, Response

from lib.utils import logger

from .clients import AUTH_METHODS, OOB_URIS
from .config import SCOPE_OFFLINE, SCOPE_TOOLS
from .service import OAuthError, OAuthService
from .ui import CSP, render_error, render_handoff, render_login, render_out_of_band

MAX_BODY_BYTES = 64 * 1024

Handler = Callable[[Request], Awaitable[Response]]

_NO_STORE = {"Cache-Control": "no-store", "Pragma": "no-cache"}
_CORS = {
    "Access-Control-Allow-Origin": "*",
    "Access-Control-Allow-Methods": "GET, POST, OPTIONS",
    "Access-Control-Allow-Headers": "Content-Type, Authorization, MCP-Protocol-Version",
    "Access-Control-Max-Age": "86400",
}


def _cors(handler: Handler) -> Handler:
    """Allow browser-based clients (for example MCP Inspector) to call an endpoint that carries no cookies."""

    async def wrapper(request: Request) -> Response:
        if request.method == "OPTIONS":
            return Response(status_code=204, headers=_CORS)
        response = await handler(request)
        response.headers.update(_CORS)
        return response

    return wrapper


def _html(content: str, status: int = 200, headers: dict[str, str] | None = None) -> HTMLResponse:
    return HTMLResponse(
        content,
        status_code=status,
        headers={
            **_NO_STORE,
            "Content-Security-Policy": CSP,
            "X-Frame-Options": "DENY",
            "X-Content-Type-Options": "nosniff",
            "Referrer-Policy": "no-referrer",
            **(headers or {}),
        },
    )


def _oauth_error(err: OAuthError, *, basic_challenge: bool = False) -> JSONResponse:
    headers = dict(_NO_STORE)
    if err.status == 401 and basic_challenge:
        headers["WWW-Authenticate"] = 'Basic realm="oauth"'
    body = {"error": err.error}
    if err.description:
        body["error_description"] = err.description
    return JSONResponse(body, status_code=err.status, headers=headers)


def _peer(request: Request) -> str:
    return request.client.host if request.client else "unknown"


# --- discovery ---------------------------------------------------------------------------------


def _authorization_server_metadata(service: OAuthService) -> dict[str, Any]:
    cfg = service.config
    return {
        "issuer": cfg.issuer,
        "authorization_endpoint": cfg.endpoint("/authorize"),
        "token_endpoint": cfg.endpoint("/token"),
        "registration_endpoint": cfg.endpoint("/register"),
        "revocation_endpoint": cfg.endpoint("/revoke"),
        "response_types_supported": ["code"],
        "response_modes_supported": ["query"],
        "grant_types_supported": ["authorization_code", "refresh_token"],
        "code_challenge_methods_supported": ["S256"],
        "token_endpoint_auth_methods_supported": list(AUTH_METHODS),
        "revocation_endpoint_auth_methods_supported": list(AUTH_METHODS),
        "scopes_supported": [SCOPE_TOOLS, SCOPE_OFFLINE],
        "authorization_response_iss_parameter_supported": True,
        "service_documentation": "https://github.com/matheuspina/whatsapp-mcp/blob/main/docs/mcp-oauth.md",
    }


def _resource_metadata(service: OAuthService) -> dict[str, Any]:
    cfg = service.config
    return {
        "resource": cfg.resource_url,
        "authorization_servers": [cfg.issuer],
        "scopes_supported": [SCOPE_TOOLS, SCOPE_OFFLINE],
        "bearer_methods_supported": ["header"],
        "resource_name": "WhatsApp MCP",
    }


# --- registration ------------------------------------------------------------------------------


async def _read_json_object(request: Request) -> dict[str, Any] | None:
    raw = await request.body()
    if len(raw) > MAX_BODY_BYTES:
        return None
    try:
        data = json.loads(raw)
    except ValueError:
        return None
    return data if isinstance(data, dict) else None


def _register(service: OAuthService) -> Handler:
    async def handler(request: Request) -> Response:
        metadata = await _read_json_object(request)
        if metadata is None:
            return _oauth_error(OAuthError("invalid_client_metadata", "body must be a JSON object under 64 KiB"))
        try:
            client, secret = service.register_client(metadata)
        except OAuthError as err:
            return _oauth_error(err)

        body: dict[str, Any] = {
            "client_id": client.client_id,
            "client_id_issued_at": client.created_at,
            "client_name": client.client_name,
            "redirect_uris": client.redirect_uris,
            "grant_types": client.grant_types,
            "response_types": ["code"],
            "token_endpoint_auth_method": client.token_endpoint_auth_method,
            "scope": client.scope,
        }
        if secret:
            body["client_secret"] = secret
            body["client_secret_expires_at"] = 0
        logger.info("OAuth client registered: %s (%s)", client.client_id, client.display_name)
        return JSONResponse(body, status_code=201, headers=_NO_STORE)

    return handler


# --- authorization -----------------------------------------------------------------------------


def _with_params(uri: str, params: dict[str, str | None]) -> str:
    parsed = urlparse(uri)
    query = parse_qsl(parsed.query, keep_blank_values=True)
    query.extend((k, v) for k, v in params.items() if v is not None)
    return urlunparse(parsed._replace(query=urlencode(query)))


def _deliver(service: OAuthService, redirect_uri: str, params: dict[str, str | None], client_name: str) -> Response:
    """Send an authorization response (code or error) to the client's redirect URI."""
    params = {**params, "iss": service.config.issuer}
    is_error = "error" in params
    if redirect_uri in OOB_URIS:
        if is_error:
            return _html(render_error(f"{client_name}: {params.get('error_description') or params['error']}"))
        return _html(render_out_of_band(code=params["code"] or "", state=params.get("state"), client_name=client_name))

    target = _with_params(redirect_uri, params)
    if urlparse(redirect_uri).scheme in {"http", "https"}:
        return RedirectResponse(target, status_code=302, headers={**_NO_STORE, "Referrer-Policy": "no-referrer"})
    # Private-use schemes (cursor://, vscode://, ...): a page with a manual link works where a bare 302 shows a blank tab.
    return _html(render_handoff(target_url=target, client_name=client_name, denied=is_error))


def _authorize(service: OAuthService) -> Handler:
    async def handler(request: Request) -> Response:
        q = request.query_params
        client = service.get_client(q.get("client_id"))
        if client is None:
            return _html(render_error("Unknown client. The application is not registered with this server."), 400)

        requested_uri = q.get("redirect_uri")
        redirect_uri = service.resolve_redirect_uri(client, requested_uri)
        if redirect_uri is None:
            # Never redirect to an unvalidated URI: answer the user directly.
            return _html(render_error("The redirect address is not registered for this application."), 400)

        state = q.get("state")

        def fail(error: str, description: str) -> Response:
            return _deliver(
                service,
                redirect_uri,
                {"error": error, "error_description": description, "state": state},
                client.display_name,
            )

        if q.get("response_type") != "code":
            return fail("unsupported_response_type", "only response_type=code is supported")
        try:
            request_id, pending = service.begin_authorization(
                client,
                redirect_uri=requested_uri,
                code_challenge=q.get("code_challenge", ""),
                code_challenge_method=q.get("code_challenge_method"),
                scope=q.get("scope"),
                state=state,
                resource=q.get("resource"),
            )
        except OAuthError as err:
            return fail(err.error, err.description)

        return _html(
            render_login(
                request_id=request_id,
                client_name=client.display_name,
                redirect_uri=pending.redirect_uri,
                scope=pending.scope,
            )
        )

    return handler


def _login(service: OAuthService) -> Handler:
    async def handler(request: Request) -> Response:
        form = await request.form()
        request_id = str(form.get("request_id", ""))
        pending = service.store.get_auth_request(request_id) if request_id else None
        client = service.get_client(pending.client_id) if pending else None
        if pending is None or client is None:
            return _html(render_error("This sign-in request expired or was already used."), 400)

        if form.get("action") == "deny":
            service.store.pop_auth_request(request_id)
            return _deliver(
                service,
                pending.redirect_uri,
                {"error": "access_denied", "error_description": "the user denied the request", "state": pending.state},
                client.display_name,
            )

        peer = _peer(request)
        username = str(form.get("username", ""))

        def again(message: str, status: int, headers: dict[str, str] | None = None) -> Response:
            page = render_login(
                request_id=request_id,
                client_name=client.display_name,
                redirect_uri=pending.redirect_uri,
                scope=pending.scope,
                username=username,
                error=message,
            )
            return _html(page, status, headers)

        wait = service.throttle.retry_after(peer)
        if wait:
            return again(f"Too many failed attempts. Try again in {wait} seconds.", 429, {"Retry-After": str(wait)})

        if not service.check_credentials(username, str(form.get("password", ""))):
            service.throttle.fail(peer)
            logger.warning("OAuth sign-in failed for client %s from %s", client.client_id, peer)
            return again("Invalid username or password.", 401)

        service.throttle.reset(peer)
        request_data = service.store.pop_auth_request(request_id)
        if request_data is None:
            return _html(render_error("This sign-in request expired or was already used."), 400)
        code = service.issue_code(request_data)
        logger.info("OAuth authorization granted to client %s (%s)", client.client_id, client.display_name)
        return _deliver(
            service, request_data.redirect_uri, {"code": code, "state": request_data.state}, client.display_name
        )

    return handler


# --- token and revocation ----------------------------------------------------------------------


async def _read_params(request: Request) -> dict[str, str]:
    if len(await request.body()) > MAX_BODY_BYTES:
        return {}
    if "json" in request.headers.get("content-type", ""):
        data = await _read_json_object(request)
        return {k: v for k, v in (data or {}).items() if isinstance(v, str)}
    form = await request.form()
    return {k: v for k, v in form.items() if isinstance(v, str)}


def _client_credentials(request: Request, params: dict[str, str]) -> tuple[str | None, str | None, bool]:
    """client_id and secret from HTTP Basic (client_secret_basic) or the body (client_secret_post)."""
    header = request.headers.get("authorization", "")
    if header[:6].lower() == "basic ":
        try:
            decoded = base64.b64decode(header[6:].strip(), validate=True).decode("utf-8")
        except (binascii.Error, UnicodeDecodeError):
            return None, None, True
        client_id, sep, secret = decoded.partition(":")
        if sep:
            return unquote(client_id), unquote(secret), True
    return params.get("client_id"), params.get("client_secret"), False


def _token(service: OAuthService) -> Handler:
    async def handler(request: Request) -> Response:
        params = await _read_params(request)
        client_id, secret, used_basic = _client_credentials(request, params)
        try:
            client = service.authenticate_client(client_id, secret)
            grant_type = params.get("grant_type")
            if grant_type == "authorization_code":
                grant = service.exchange_code(
                    client,
                    code=params.get("code"),
                    redirect_uri=params.get("redirect_uri"),
                    code_verifier=params.get("code_verifier"),
                )
            elif grant_type == "refresh_token":
                grant = service.refresh(client, refresh_token=params.get("refresh_token"), scope=params.get("scope"))
            else:
                raise OAuthError("unsupported_grant_type", "grant_type must be authorization_code or refresh_token")
        except OAuthError as err:
            return _oauth_error(err, basic_challenge=used_basic)

        return JSONResponse(
            {
                "access_token": grant.access_token,
                "token_type": "Bearer",
                "expires_in": grant.expires_in,
                "refresh_token": grant.refresh_token,
                "scope": grant.scope,
            },
            headers=_NO_STORE,
        )

    return handler


def _revoke(service: OAuthService) -> Handler:
    async def handler(request: Request) -> Response:
        params = await _read_params(request)
        client_id, secret, used_basic = _client_credentials(request, params)
        try:
            client = service.authenticate_client(client_id, secret)
        except OAuthError as err:
            return _oauth_error(err, basic_challenge=used_basic)
        token = params.get("token")
        if token:
            service.revoke(client, token)
        # RFC 7009 section 2.2: the answer is 200 whether or not the token existed.
        return Response(status_code=200, headers=_NO_STORE)

    return handler


# --- registration of routes --------------------------------------------------------------------


def register_routes(mcp: FastMCP, service: OAuthService) -> None:
    """Attach the OAuth endpoints to the FastMCP HTTP app. These routes are public by design."""
    resource_path = service.config.resource_path
    as_metadata = _cors(_json_handler(lambda: _authorization_server_metadata(service)))
    rs_metadata = _cors(_json_handler(lambda: _resource_metadata(service)))

    # The SDK already serves /.well-known/oauth-protected-resource<resource_path>; add the bare form.
    mcp.custom_route("/.well-known/oauth-protected-resource", ["GET", "OPTIONS"])(rs_metadata)
    for path in (
        "/.well-known/oauth-authorization-server",
        f"/.well-known/oauth-authorization-server{resource_path}",
        "/.well-known/openid-configuration",
        f"/.well-known/openid-configuration{resource_path}",
        f"{resource_path}/.well-known/openid-configuration",
    ):
        mcp.custom_route(path, ["GET", "OPTIONS"])(as_metadata)

    mcp.custom_route("/register", ["POST", "OPTIONS"])(_cors(_register(service)))
    mcp.custom_route("/authorize", ["GET"])(_authorize(service))
    mcp.custom_route("/oauth/login", ["POST"])(_login(service))
    mcp.custom_route("/token", ["POST", "OPTIONS"])(_cors(_token(service)))
    mcp.custom_route("/revoke", ["POST", "OPTIONS"])(_cors(_revoke(service)))


def _json_handler(build: Callable[[], dict[str, Any]]) -> Handler:
    async def handler(_request: Request) -> Response:
        return JSONResponse(build(), headers={"Cache-Control": "public, max-age=300"})

    return handler
