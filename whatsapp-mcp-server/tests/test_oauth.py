"""End-to-end tests of the MCP OAuth 2.1 layer against the real FastMCP HTTP app."""

import base64
import re
from urllib.parse import parse_qs, urlparse

import pytest
from mcp.server.fastmcp import FastMCP
from starlette.testclient import TestClient

from lib.oauth import OAuthConfigError, OAuthService, OAuthSetup
from lib.oauth.clients import is_valid_redirect_uri, redirect_uri_allowed
from lib.oauth.config import load_config
from lib.oauth.service import LoginThrottle, pkce_s256
from lib.oauth.store import OAuthStore

PUBLIC = "https://mcp.example.com"
VERIFIER = "v" * 43
CHALLENGE = pkce_s256(VERIFIER)
INIT = {
    "jsonrpc": "2.0",
    "id": 1,
    "method": "initialize",
    "params": {"protocolVersion": "2025-03-26", "capabilities": {}, "clientInfo": {"name": "t", "version": "1"}},
}
MCP_HEADERS = {"Accept": "application/json, text/event-stream", "Content-Type": "application/json"}


def _env(**over):
    env = {
        "MCP_PUBLIC_URL": PUBLIC,
        "WEB_UI_USERNAME": "admin",
        "WEB_UI_PASSWORD": "s3cret-pass",
        "API_KEY": "static-api-key",
    }
    env.update(over)
    return env


def _service(tmp_path=None, **over) -> OAuthService:
    config = load_config(resource_path="/mcp", store_path=str(tmp_path or "/tmp"), env=_env(**over))
    assert config is not None
    store = OAuthStore(":memory:" if tmp_path is None else config.db_path)
    return OAuthService(config, store)


def _app(service: OAuthService):
    mcp = FastMCP("test", **OAuthSetup(service).fastmcp_kwargs())
    OAuthSetup(service).register(mcp)
    return mcp.streamable_http_app()


@pytest.fixture
def service():
    return _service()


@pytest.fixture
def client(service):
    with TestClient(_app(service), base_url=PUBLIC, follow_redirects=False) as c:
        yield c


def _register(client, **over):
    body = {
        "client_name": "Test Client",
        "redirect_uris": ["https://client.example/cb"],
        "token_endpoint_auth_method": "none",
        "grant_types": ["authorization_code", "refresh_token"],
        **over,
    }
    res = client.post("/register", json=body)
    assert res.status_code == 201, res.text
    return res.json()


def _authorize(client, client_id, redirect_uri="https://client.example/cb", **over):
    params = {
        "response_type": "code",
        "client_id": client_id,
        "redirect_uri": redirect_uri,
        "code_challenge": CHALLENGE,
        "code_challenge_method": "S256",
        "state": "st-1",
        **over,
    }
    return client.get("/authorize", params={k: v for k, v in params.items() if v is not None})


def _request_id(res) -> str:
    match = re.search(r'name="request_id" value="([^"]+)"', res.text)
    assert match, res.text
    return match.group(1)


def _sign_in(client, request_id, password="s3cret-pass", action="approve"):
    return client.post(
        "/oauth/login",
        data={"request_id": request_id, "username": "admin", "password": password, "action": action},
    )


def _get_code(client, client_id, redirect_uri="https://client.example/cb"):
    res = _sign_in(client, _request_id(_authorize(client, client_id, redirect_uri)))
    assert res.status_code == 302, res.text
    return parse_qs(urlparse(res.headers["location"]).query)["code"][0]


def _token(client, client_id, code, redirect_uri="https://client.example/cb", verifier=VERIFIER, **headers):
    return client.post(
        "/token",
        data={
            "grant_type": "authorization_code",
            "client_id": client_id,
            "code": code,
            "redirect_uri": redirect_uri,
            "code_verifier": verifier,
        },
        **headers,
    )


# --- configuration -----------------------------------------------------------------------------


def test_oauth_is_off_without_public_url():
    assert load_config(resource_path="/mcp", store_path="/tmp", env={}) is None


@pytest.mark.parametrize(
    "over",
    [
        {"WEB_UI_PASSWORD": ""},
        {"WEB_UI_PASSWORD": "CHANGEME_USE_A_STRONG_PASSWORD"},
        {"MCP_PUBLIC_URL": "http://mcp.example.com"},
        {"MCP_PUBLIC_URL": "https://mcp.example.com/mcp"},
        {"MCP_PUBLIC_URL": "ftp://mcp.example.com"},
        {"MCP_OAUTH_ACCESS_TOKEN_TTL": "0"},
    ],
)
def test_invalid_configuration_fails_fast(over):
    with pytest.raises(OAuthConfigError):
        load_config(resource_path="/mcp", store_path="/tmp", env=_env(**over))


def test_http_is_allowed_on_loopback_and_placeholder_api_key_is_dropped():
    cfg = load_config(
        resource_path="/mcp",
        store_path="/tmp",
        env=_env(MCP_PUBLIC_URL="http://127.0.0.1:8081/", API_KEY="CHANGEME_USE_openssl_rand_hex_32"),
    )
    assert cfg and cfg.public_url == "http://127.0.0.1:8081" and cfg.api_key is None


# --- discovery and protection ------------------------------------------------------------------


def test_mcp_endpoint_requires_a_token_and_points_to_the_metadata(client):
    res = client.post("/mcp", json=INIT, headers=MCP_HEADERS)
    assert res.status_code == 401
    challenge = res.headers["www-authenticate"]
    assert 'resource_metadata="https://mcp.example.com/.well-known/oauth-protected-resource/mcp"' in challenge


def test_discovery_documents_agree_on_the_issuer(client):
    prm = client.get("/.well-known/oauth-protected-resource/mcp").json()
    assert prm["resource"] == f"{PUBLIC}/mcp"
    assert client.get("/.well-known/oauth-protected-resource").json()["resource"] == f"{PUBLIC}/mcp"

    paths = [
        "/.well-known/oauth-authorization-server",
        "/.well-known/oauth-authorization-server/mcp",
        "/.well-known/openid-configuration",
        "/.well-known/openid-configuration/mcp",
        "/mcp/.well-known/openid-configuration",
    ]
    for path in paths:
        meta = client.get(path).json()
        assert meta["issuer"] == prm["authorization_servers"][0], path
        assert meta["code_challenge_methods_supported"] == ["S256"]
        assert meta["registration_endpoint"] == f"{PUBLIC}/register"
        assert {"none", "client_secret_basic", "client_secret_post"} <= set(
            meta["token_endpoint_auth_methods_supported"]
        )


def test_metadata_answers_cors_preflight(client):
    res = client.options("/.well-known/oauth-authorization-server", headers={"Origin": "https://claude.ai"})
    assert res.status_code == 204 and res.headers["access-control-allow-origin"] == "*"


# --- the full flow -----------------------------------------------------------------------------


def test_full_authorization_code_flow_with_refresh_rotation(client):
    reg = _register(client)
    assert "client_secret" not in reg

    page = _authorize(client, reg["client_id"])
    assert page.status_code == 200 and "Test Client" in page.text
    assert (
        page.headers["x-frame-options"] == "DENY"
        and "frame-ancestors 'none'" in page.headers["content-security-policy"]
    )

    bad = _sign_in(client, _request_id(page), password="wrong")
    assert bad.status_code == 401 and "Invalid username or password" in bad.text

    ok = _sign_in(client, _request_id(bad))
    assert ok.status_code == 302
    target = urlparse(ok.headers["location"])
    query = parse_qs(target.query)
    assert f"{target.scheme}://{target.netloc}{target.path}" == "https://client.example/cb"
    assert query["state"] == ["st-1"] and query["iss"] == [f"{PUBLIC}/"]

    tokens = _token(client, reg["client_id"], query["code"][0])
    assert tokens.status_code == 200 and tokens.headers["cache-control"] == "no-store"
    body = tokens.json()
    assert body["token_type"] == "Bearer" and body["expires_in"] == 3600 and "mcp:tools" in body["scope"]

    assert (
        client.post(
            "/mcp", json=INIT, headers={**MCP_HEADERS, "Authorization": f"Bearer {body['access_token']}"}
        ).status_code
        == 200
    )

    refreshed = client.post(
        "/token",
        data={"grant_type": "refresh_token", "client_id": reg["client_id"], "refresh_token": body["refresh_token"]},
    )
    assert refreshed.status_code == 200
    new = refreshed.json()
    assert new["access_token"] != body["access_token"] and new["refresh_token"] != body["refresh_token"]

    # Replaying the rotated-out refresh token is a leak signal: it is refused and the family is revoked.
    replay = client.post(
        "/token",
        data={"grant_type": "refresh_token", "client_id": reg["client_id"], "refresh_token": body["refresh_token"]},
    )
    assert replay.status_code == 400 and replay.json()["error"] == "invalid_grant"
    assert (
        client.post(
            "/mcp", json=INIT, headers={**MCP_HEADERS, "Authorization": f"Bearer {new['access_token']}"}
        ).status_code
        == 401
    )


def test_authorization_code_is_single_use_and_bound_to_pkce_and_redirect(client):
    cid = _register(client)["client_id"]

    code = _get_code(client, cid)
    assert _token(client, cid, code, verifier="w" * 43).json()["error"] == "invalid_grant"
    assert _token(client, cid, code).json()["error"] == "invalid_grant"  # already consumed by the failed try

    code = _get_code(client, cid)
    assert _token(client, cid, code, redirect_uri="https://client.example/other").json()["error"] == "invalid_grant"

    code = _get_code(client, cid)
    assert _token(client, cid, code).status_code == 200
    assert _token(client, cid, code).status_code == 400


def test_code_cannot_be_redeemed_by_another_client(client):
    a, b = _register(client)["client_id"], _register(client)["client_id"]
    assert _token(client, b, _get_code(client, a)).json()["error"] == "invalid_grant"


def test_deny_redirects_with_access_denied(client):
    cid = _register(client)["client_id"]
    res = _sign_in(client, _request_id(_authorize(client, cid)), action="deny")
    query = parse_qs(urlparse(res.headers["location"]).query)
    assert res.status_code == 302 and query["error"] == ["access_denied"] and query["state"] == ["st-1"]


def test_sign_in_request_is_single_use(client):
    cid = _register(client)["client_id"]
    rid = _request_id(_authorize(client, cid))
    assert _sign_in(client, rid).status_code == 302
    assert _sign_in(client, rid).status_code == 400


def test_failed_logins_are_throttled(client):
    cid = _register(client)["client_id"]
    rid = _request_id(_authorize(client, cid))
    for _ in range(5):
        assert _sign_in(client, rid, password="nope").status_code == 401
    locked = _sign_in(client, rid)  # correct password, still refused
    assert locked.status_code == 429 and int(locked.headers["retry-after"]) > 0


def test_login_throttle_window_and_reset():
    now = [0.0]
    throttle = LoginThrottle(now=lambda: now[0])
    for _ in range(5):
        throttle.fail("ip")
    assert throttle.retry_after("ip") > 0 and throttle.retry_after("other") == 0
    now[0] = 301
    assert throttle.retry_after("ip") == 0
    throttle.fail("ip")
    throttle.reset("ip")
    assert throttle.retry_after("ip") == 0


# --- request validation ------------------------------------------------------------------------


def test_unknown_client_and_unregistered_redirect_never_redirect(client):
    unknown = _authorize(client, "does-not-exist")
    assert unknown.status_code == 400 and "location" not in unknown.headers

    cid = _register(client)["client_id"]
    evil = _authorize(client, cid, redirect_uri="https://evil.example/steal")
    assert evil.status_code == 400 and "location" not in evil.headers and "request_id" not in evil.text


def test_pkce_is_mandatory_and_only_s256(client):
    cid = _register(client)["client_id"]
    for over in ({"code_challenge": None}, {"code_challenge_method": "plain"}, {"code_challenge_method": None}):
        res = _authorize(client, cid, **over)
        assert res.status_code == 302
        assert parse_qs(urlparse(res.headers["location"]).query)["error"] == ["invalid_request"]


def test_unsupported_response_type_and_wrong_resource(client):
    cid = _register(client)["client_id"]
    res = _authorize(client, cid, response_type="token")
    assert parse_qs(urlparse(res.headers["location"]).query)["error"] == ["unsupported_response_type"]
    res = _authorize(client, cid, resource="https://other.example/mcp")
    assert parse_qs(urlparse(res.headers["location"]).query)["error"] == ["invalid_target"]
    assert _authorize(client, cid, resource=f"{PUBLIC}/mcp").status_code == 200
    assert _authorize(client, cid, resource=f"{PUBLIC}/").status_code == 200


@pytest.mark.parametrize(
    "uri,valid",
    [
        ("https://claude.ai/api/mcp/auth_callback", True),
        ("http://localhost:8123/callback", True),
        ("http://127.0.0.1/callback", True),
        ("cursor://anysphere.cursor-mcp/oauth/callback", True),
        ("urn:ietf:wg:oauth:2.0:oob", True),
        ("http://evil.example/cb", False),
        ("javascript:alert(1)", False),
        ("data:text/html,x", False),
        ("https://client.example/cb#frag", False),
        ("https://client.example/ cb", False),
        ("notaurl", False),
        ("", False),
    ],
)
def test_redirect_uri_policy(uri, valid):
    assert is_valid_redirect_uri(uri) is valid


def test_loopback_redirect_ignores_the_port_but_not_the_path():
    registered = ["http://127.0.0.1/callback"]
    assert redirect_uri_allowed(registered, "http://127.0.0.1:54321/callback")
    assert not redirect_uri_allowed(registered, "http://127.0.0.1:54321/other")
    assert not redirect_uri_allowed(registered, "http://localhost:54321/callback")
    assert not redirect_uri_allowed(["https://a.example/cb"], "https://a.example:444/cb")


def test_registration_rejects_bad_metadata(client):
    bad = [
        {"redirect_uris": []},
        {"redirect_uris": ["http://evil.example/cb"]},
        {"redirect_uris": ["https://a.example/cb"], "token_endpoint_auth_method": "private_key_jwt"},
        {"redirect_uris": ["https://a.example/cb"], "grant_types": ["implicit"]},
    ]
    for body in bad:
        res = client.post("/register", json=body)
        assert res.status_code == 400 and res.json()["error"] in {"invalid_redirect_uri", "invalid_client_metadata"}
    assert client.post("/register", content=b"not json").status_code == 400


def test_registration_accepts_a_client_without_refresh_token_grant(client):
    reg = _register(client, grant_types=["authorization_code"])
    assert reg["grant_types"] == ["authorization_code"]


# --- client authentication ---------------------------------------------------------------------


def test_confidential_client_authenticates_with_basic_or_post_and_never_without_secret(client):
    reg = _register(client, token_endpoint_auth_method="client_secret_basic")
    cid, secret = reg["client_id"], reg["client_secret"]

    assert _token(client, cid, _get_code(client, cid)).status_code == 401  # no secret

    basic = base64.b64encode(f"{cid}:{secret}".encode()).decode()
    res = client.post(
        "/token",
        headers={"Authorization": f"Basic {basic}"},
        data={
            "grant_type": "authorization_code",
            "code": _get_code(client, cid),
            "redirect_uri": "https://client.example/cb",
            "code_verifier": VERIFIER,
        },
    )
    assert res.status_code == 200

    wrong = base64.b64encode(f"{cid}:nope".encode()).decode()
    res = client.post(
        "/token",
        headers={"Authorization": f"Basic {wrong}"},
        data={"grant_type": "refresh_token", "refresh_token": "x"},
    )
    assert res.status_code == 401 and res.headers["www-authenticate"].startswith("Basic")

    res = client.post(
        "/token",
        data={
            "grant_type": "authorization_code",
            "client_id": cid,
            "client_secret": secret,
            "code": _get_code(client, cid),
            "redirect_uri": "https://client.example/cb",
            "code_verifier": VERIFIER,
        },
    )
    assert res.status_code == 200


def test_token_endpoint_accepts_json_bodies_and_rejects_other_grants(client):
    cid = _register(client)["client_id"]
    res = client.post(
        "/token",
        json={
            "grant_type": "authorization_code",
            "client_id": cid,
            "code": _get_code(client, cid),
            "redirect_uri": "https://client.example/cb",
            "code_verifier": VERIFIER,
        },
    )
    assert res.status_code == 200
    res = client.post("/token", data={"grant_type": "password", "client_id": cid})
    assert res.status_code == 400 and res.json()["error"] == "unsupported_grant_type"
    assert client.post("/token", data={"grant_type": "refresh_token", "client_id": "ghost"}).status_code == 401


def test_refresh_cannot_widen_scope_or_cross_clients(client):
    a, b = _register(client)["client_id"], _register(client)["client_id"]
    tokens = _token(client, a, _get_code(client, a)).json()

    res = client.post(
        "/token", data={"grant_type": "refresh_token", "client_id": b, "refresh_token": tokens["refresh_token"]}
    )
    assert res.json()["error"] == "invalid_grant"
    res = client.post(
        "/token",
        data={
            "grant_type": "refresh_token",
            "client_id": a,
            "refresh_token": tokens["refresh_token"],
            "scope": "mcp:tools admin",
        },
    )
    assert res.json()["error"] == "invalid_scope"


def test_revocation_kills_the_access_token(client):
    cid = _register(client)["client_id"]
    tokens = _token(client, cid, _get_code(client, cid)).json()
    auth = {**MCP_HEADERS, "Authorization": f"Bearer {tokens['access_token']}"}
    assert client.post("/mcp", json=INIT, headers=auth).status_code == 200

    assert client.post("/revoke", data={"client_id": cid, "token": tokens["refresh_token"]}).status_code == 200
    assert client.post("/mcp", json=INIT, headers=auth).status_code == 401
    assert client.post("/revoke", data={"client_id": cid, "token": "unknown"}).status_code == 200


def test_static_api_key_is_accepted_as_bearer(client):
    ok = client.post("/mcp", json=INIT, headers={**MCP_HEADERS, "Authorization": "Bearer static-api-key"})
    assert ok.status_code == 200
    bad = client.post("/mcp", json=INIT, headers={**MCP_HEADERS, "Authorization": "Bearer nope"})
    assert bad.status_code == 401


def test_expired_access_token_is_rejected(tmp_path):
    svc = _service(MCP_OAUTH_ACCESS_TOKEN_TTL="1")
    with TestClient(_app(svc), base_url=PUBLIC, follow_redirects=False) as c:
        cid = _register(c)["client_id"]
        tokens = _token(c, cid, _get_code(c, cid)).json()
        assert tokens["expires_in"] == 1
        svc.store._conn.execute("UPDATE oauth_tokens SET access_expires_at = access_expires_at - 10")
        res = c.post("/mcp", json=INIT, headers={**MCP_HEADERS, "Authorization": f"Bearer {tokens['access_token']}"})
        assert res.status_code == 401


# --- well-known clients and non-HTTP redirects -------------------------------------------------


def test_out_of_band_flow_shows_the_code_on_the_page(client):
    res = _authorize(client, "codex", redirect_uri="urn:ietf:wg:oauth:2.0:oob")
    done = _sign_in(client, _request_id(res))
    assert done.status_code == 200
    code = re.search(r"<textarea[^>]*>([^<]+)</textarea>", done.text).group(1)
    tokens = _token(client, "codex", code, redirect_uri="urn:ietf:wg:oauth:2.0:oob")
    assert tokens.status_code == 200


def test_private_scheme_redirect_gets_a_handoff_page(client):
    uri = "cursor://anysphere.cursor-mcp/oauth/callback"
    done = _sign_in(client, _request_id(_authorize(client, "cursor", redirect_uri=uri)))
    assert done.status_code == 200
    link = re.search(r'href="(cursor://[^"]+)"', done.text).group(1).replace("&amp;", "&")
    assert parse_qs(urlparse(link).query)["state"] == ["st-1"]


def test_well_known_client_rejects_foreign_redirects_but_allows_loopback_ports(client):
    assert _authorize(client, "claude", redirect_uri="https://evil.example/cb").status_code == 400
    assert _authorize(client, "claude-code", redirect_uri="http://localhost:39999/callback").status_code == 200


# --- persistence -------------------------------------------------------------------------------


def test_tokens_survive_a_restart_and_are_not_stored_in_clear(tmp_path):
    first = _service(tmp_path)
    with TestClient(_app(first), base_url=PUBLIC, follow_redirects=False) as c:
        cid = _register(c)["client_id"]
        tokens = _token(c, cid, _get_code(c, cid)).json()
    first.store.close()

    second = _service(tmp_path)
    with TestClient(_app(second), base_url=PUBLIC, follow_redirects=False) as c:
        res = c.post("/mcp", json=INIT, headers={**MCP_HEADERS, "Authorization": f"Bearer {tokens['access_token']}"})
        assert res.status_code == 200
    dump = "".join(second.store._conn.iterdump())
    assert tokens["access_token"] not in dump and tokens["refresh_token"] not in dump
    second.store.close()


def test_client_registry_is_capped(monkeypatch):
    import lib.oauth.store as store_module

    monkeypatch.setattr(store_module, "MAX_CLIENTS", 3)
    svc = _service()
    for _ in range(6):
        svc.register_client({"redirect_uris": ["https://a.example/cb"], "token_endpoint_auth_method": "none"})
    assert svc.store._conn.execute("SELECT COUNT(*) FROM oauth_clients").fetchone()[0] == 3
