"""HTML pages of the OAuth flow: sign in and consent, out-of-band code, hand-off and errors.

The pages use no JavaScript, which lets them ship with a strict Content-Security-Policy.
"""

from __future__ import annotations

from html import escape
from urllib.parse import urlparse

# form-action is deliberately absent: Chrome also applies it to the redirect that follows the form
# post, and that redirect goes to the AI client's own callback.
CSP = "default-src 'none'; style-src 'unsafe-inline'; base-uri 'none'; frame-ancestors 'none'"

_STYLE = """
:root { color-scheme: light dark; --bg:#f5f6f8; --card:#fff; --fg:#14181f; --muted:#5b6473; --line:#dde1e8;
        --accent:#128c4a; --accent-fg:#fff; --danger-bg:#fdecec; --danger-fg:#a31d1d; --code:#eef0f4; }
@media (prefers-color-scheme: dark) {
  :root { --bg:#0e1116; --card:#171b22; --fg:#e8ebf0; --muted:#98a1b0; --line:#2a303a;
          --accent:#2bb673; --accent-fg:#06180e; --danger-bg:#3a1618; --danger-fg:#ffb4b4; --code:#0e1116; } }
* { box-sizing: border-box; }
body { margin:0; min-height:100vh; display:flex; align-items:center; justify-content:center; padding:16px;
       background:var(--bg); color:var(--fg); font:16px/1.5 system-ui,-apple-system,"Segoe UI",Roboto,sans-serif; }
main { width:100%; max-width:420px; background:var(--card); border:1px solid var(--line); border-radius:14px; padding:28px; }
h1 { font-size:1.25rem; margin:0 0 4px; }
p { margin:0 0 14px; color:var(--muted); font-size:.92rem; }
.client { color:var(--fg); font-weight:600; }
.box { background:var(--code); border:1px solid var(--line); border-radius:8px; padding:10px 12px; margin:0 0 16px;
       font-size:.85rem; word-break:break-all; }
.box b { display:block; font-size:.72rem; text-transform:uppercase; letter-spacing:.04em; color:var(--muted); }
label { display:block; font-size:.85rem; margin:0 0 4px; color:var(--muted); }
input[type=text], input[type=password], textarea { width:100%; padding:10px 12px; margin:0 0 14px; border-radius:8px;
       border:1px solid var(--line); background:var(--bg); color:var(--fg); font:inherit; }
textarea { font-family:ui-monospace,SFMono-Regular,Menlo,monospace; font-size:.85rem; resize:none; }
.row { display:flex; gap:10px; }
button, .btn { flex:1; padding:10px 14px; border-radius:8px; border:1px solid var(--line); background:transparent;
       color:var(--fg); font:inherit; font-weight:600; cursor:pointer; text-align:center; text-decoration:none; display:block; }
button.primary, .btn.primary { background:var(--accent); border-color:var(--accent); color:var(--accent-fg); }
.error { background:var(--danger-bg); color:var(--danger-fg); border-radius:8px; padding:10px 12px; margin:0 0 14px; font-size:.88rem; }
"""


def _page(title: str, body: str, head: str = "") -> str:
    return (
        f'<!doctype html><html lang="en"><head><meta charset="utf-8">'
        f'<meta name="viewport" content="width=device-width, initial-scale=1">'
        f'<meta name="referrer" content="no-referrer"><title>{escape(title)}</title>{head}'
        f"<style>{_STYLE}</style></head><body><main>{body}</main></body></html>"
    )


def _origin_label(redirect_uri: str) -> str:
    """What to show the user about where the client will be sent: host for web URLs, scheme otherwise."""
    if redirect_uri.startswith("urn:"):
        return "code shown on this page"
    parsed = urlparse(redirect_uri)
    if parsed.scheme in {"http", "https"}:
        return parsed.netloc
    return f"{parsed.scheme}:// (application on this device)"


def render_login(
    *,
    request_id: str,
    client_name: str,
    redirect_uri: str,
    scope: str,
    username: str = "",
    error: str | None = None,
) -> str:
    err = f'<div class="error" role="alert">{escape(error)}</div>' if error else ""
    body = f"""
<h1>Connect to WhatsApp MCP</h1>
<p><span class="client">{escape(client_name)}</span> wants to use the tools of this server: read your
WhatsApp chats and, depending on the enabled toolsets, send and delete messages on your behalf.</p>
<div class="box"><b>Returns to</b>{escape(_origin_label(redirect_uri))}</div>
<div class="box"><b>Permission</b>{escape(scope)}</div>
{err}
<form method="post" action="/oauth/login" autocomplete="on">
  <input type="hidden" name="request_id" value="{escape(request_id)}">
  <label for="username">Username</label>
  <input id="username" name="username" type="text" value="{escape(username)}" autocomplete="username" required>
  <label for="password">Password</label>
  <input id="password" name="password" type="password" autocomplete="current-password" required autofocus>
  <div class="row">
    <button type="submit" name="action" value="deny" formnovalidate>Deny</button>
    <button type="submit" name="action" value="approve" class="primary">Sign in and allow</button>
  </div>
</form>
<p style="margin:14px 0 0">Only continue if you started this connection yourself.</p>"""
    return _page("Connect to WhatsApp MCP", body)


def render_out_of_band(*, code: str, state: str | None, client_name: str) -> str:
    state_line = f"<p>State: <code>{escape(state)}</code></p>" if state else ""
    body = f"""
<h1>Authorization complete</h1>
<p><span class="client">{escape(client_name)}</span> is authorized. Copy this code and paste it into the
application that asked for it.</p>
<label for="code">Authorization code</label>
<textarea id="code" readonly rows="3">{escape(code)}</textarea>
{state_line}
<p>The code works once and expires in a minute.</p>"""
    return _page("Authorization complete", body)


def render_handoff(*, target_url: str, client_name: str, denied: bool = False) -> str:
    """Page for redirect targets a plain 302 cannot reliably reach (private-use app schemes)."""
    title = "Request denied" if denied else "Authorization complete"
    text = (
        "You denied the request. Return to the application."
        if denied
        else f"{escape(client_name)} is authorized. Return to the application to finish."
    )
    head = f'<meta http-equiv="refresh" content="0;url={escape(target_url, quote=True)}">'
    body = f"""
<h1>{escape(title)}</h1>
<p>{text}</p>
<a class="btn primary" href="{escape(target_url, quote=True)}">Open the application</a>"""
    return _page(title, body, head)


def render_error(message: str) -> str:
    body = f"""
<h1>Cannot continue</h1>
<div class="error" role="alert">{escape(message)}</div>
<p>Close this window and start the connection again from your AI client.</p>"""
    return _page("Cannot continue", body)
