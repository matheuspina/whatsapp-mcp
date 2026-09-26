"""Who may see what through the MCP server.

Every tool call runs under a *scope*: the set of WhatsApp numbers (and, for numbers that changed
hands, the periods) whose conversations the caller may read, plus whether it may act on WhatsApp at
all. The scope comes from an access policy and is enforced where the data is read, not by each tool:

* SQLite connections opened through :func:`connect_messages` and :func:`connect_whatsapp` see
  temporary views named like the real tables (``messages``, ``chats``, ``whatsmeow_contacts``) that
  only contain what the scope allows. Every query in the server therefore inherits the restriction,
  including tools added later.
* The search index filters on the same windows (see ``search.search``).

The policy is a JSON file named by ``MCP_ACCESS_POLICY``::

    {
      "default": {"read_only": true},
      "clients": {
        "<oauth client id>": {
          "departments": [2],          # numbers held by people of these departments, while they held them
          "employees": [5],            # ... or by these people
          "instances": ["5511...@s.whatsapp.net"],   # ... or these numbers, always
          "read_only": true,           # no tool that acts on WhatsApp
          "deny_tools": ["get_audit_deleted_messages"],
          "allow_tools": null          # when a list, only these tools
        }
      }
    }

A client with no entry gets ``default``; without ``default`` it is unrestricted, which is what the
server did before policies existed. ``MCP_READ_ONLY=true`` makes read-only the default for everyone.
"""

from __future__ import annotations

import contextvars
import json
import os
import sqlite3
import threading
import time
from collections.abc import Iterable
from dataclasses import dataclass, field
from datetime import UTC, datetime
from typing import Any

from .utils import logger

POLICY_TTL_SECONDS = 30


class AccessDenied(PermissionError):
    """The caller's scope does not allow the requested operation."""


@dataclass(frozen=True)
class Window:
    """One number, optionally limited to a period (epoch seconds, end exclusive)."""

    instance_jid: str
    start: int | None = None
    end: int | None = None


@dataclass(frozen=True)
class Scope:
    """What a caller may read and do."""

    client_id: str = "local"
    # None means unrestricted; otherwise the windows the caller may read (an empty tuple sees nothing).
    windows: tuple[Window, ...] | None = None
    read_only: bool = False
    deny_tools: frozenset[str] = field(default_factory=frozenset)
    allow_tools: frozenset[str] | None = None

    @property
    def restricted(self) -> bool:
        return self.windows is not None

    def instance_jids(self) -> set[str]:
        return {w.instance_jid for w in self.windows or ()}

    def check_tool(self, name: str, *, read_only_tool: bool) -> None:
        if name in self.deny_tools or (self.allow_tools is not None and name not in self.allow_tools):
            raise AccessDenied(f"The tool '{name}' is not available to this client.")
        if self.read_only and not read_only_tool:
            raise AccessDenied(f"The tool '{name}' acts on WhatsApp, and this client has read-only access.")


UNRESTRICTED = Scope()

_current_scope: contextvars.ContextVar[Scope | None] = contextvars.ContextVar("wa_scope", default=None)
_current_instance: contextvars.ContextVar[str | None] = contextvars.ContextVar("wa_instance", default=None)


def current_scope() -> Scope:
    """The scope of the tool call being served, or the default scope outside one (tests, scripts)."""
    return _current_scope.get() or _default_scope()


def set_scope(scope: Scope) -> contextvars.Token:
    return _current_scope.set(scope)


def reset_scope(token: contextvars.Token) -> None:
    _current_scope.reset(token)


def current_instance() -> str | None:
    """The number an outbound tool call asked to act as."""
    return _current_instance.get()


def set_instance(instance_jid: str | None) -> contextvars.Token:
    return _current_instance.set(instance_jid or None)


def reset_instance(token: contextvars.Token) -> None:
    _current_instance.reset(token)


def caller_id() -> str:
    """The OAuth client making the current request, or "local" when the server is not behind OAuth."""
    try:
        from mcp.server.auth.middleware.auth_context import get_access_token

        token = get_access_token()
    except Exception:  # not in a request, or the SDK has no auth context
        return "local"
    return token.client_id if token and token.client_id else "local"


# --- policy -----------------------------------------------------------------------------------


def _read_only_env() -> bool:
    return os.getenv("MCP_READ_ONLY", "").strip().lower() in {"1", "true", "yes"}


def _default_scope() -> Scope:
    return Scope(read_only=_read_only_env())


@dataclass
class _Rule:
    instances: list[str]
    employees: list[int]
    departments: list[int]
    read_only: bool
    deny_tools: frozenset[str]
    allow_tools: frozenset[str] | None

    @property
    def restricts_data(self) -> bool:
        return bool(self.instances or self.employees or self.departments)


class PolicyError(ValueError):
    """The access policy file is unreadable or malformed."""


def _parse_rule(raw: Any, where: str) -> _Rule:
    if not isinstance(raw, dict):
        raise PolicyError(f"{where} must be an object")

    def ints(key: str) -> list[int]:
        values = raw.get(key) or []
        if not isinstance(values, list) or not all(isinstance(v, int) and not isinstance(v, bool) for v in values):
            raise PolicyError(f"{where}.{key} must be a list of numbers")
        return list(values)

    def strs(key: str) -> list[str]:
        values = raw.get(key) or []
        if not isinstance(values, list) or not all(isinstance(v, str) for v in values):
            raise PolicyError(f"{where}.{key} must be a list of strings")
        return list(values)

    allow = raw.get("allow_tools")
    if allow is not None and (not isinstance(allow, list) or not all(isinstance(v, str) for v in allow)):
        raise PolicyError(f"{where}.allow_tools must be null or a list of strings")

    return _Rule(
        instances=strs("instances"),
        employees=ints("employees"),
        departments=ints("departments"),
        read_only=bool(raw.get("read_only", _read_only_env())),
        deny_tools=frozenset(strs("deny_tools")),
        allow_tools=frozenset(allow) if allow is not None else None,
    )


def parse_policy(text: str) -> tuple[_Rule | None, dict[str, _Rule]]:
    """Parse a policy document into (default rule, per-client rules). Raises PolicyError."""
    try:
        doc = json.loads(text)
    except ValueError as exc:
        raise PolicyError(f"not valid JSON: {exc}") from exc
    if not isinstance(doc, dict):
        raise PolicyError("the policy must be a JSON object")
    default = _parse_rule(doc["default"], "default") if doc.get("default") is not None else None
    clients_raw = doc.get("clients", {})
    if not isinstance(clients_raw, dict):
        raise PolicyError("clients must be an object")
    return default, {cid: _parse_rule(rule, f"clients.{cid}") for cid, rule in clients_raw.items()}


def _to_epoch(value: str | None) -> int | None:
    """Epoch seconds of a timestamp written by the bridge (None passes through)."""
    if not value:
        return None
    parsed = datetime.fromisoformat(value.strip())
    if parsed.tzinfo is None:
        parsed = parsed.replace(tzinfo=UTC)
    return int(parsed.timestamp())


def resolve_windows(messages_db_path: str, rule: _Rule) -> tuple[Window, ...]:
    """The numbers and periods a rule grants, read from the bridge's organization tables."""
    windows = [Window(jid) for jid in rule.instances]
    if rule.employees or rule.departments:
        conn = sqlite3.connect(f"file:{messages_db_path}?mode=ro", uri=True)
        try:
            clauses, params = [], []
            if rule.employees:
                clauses.append(f"a.employee_id IN ({','.join('?' * len(rule.employees))})")
                params += rule.employees
            if rule.departments:
                clauses.append(f"e.department_id IN ({','.join('?' * len(rule.departments))})")
                params += rule.departments
            rows = conn.execute(
                f"""
                SELECT i.phone_jid, a.valid_from, a.valid_to
                FROM instance_assignments a
                JOIN instances i ON i.id = a.instance_id
                JOIN employees e ON e.id = a.employee_id
                WHERE i.phone_jid IS NOT NULL AND ({" OR ".join(clauses)})
                """,
                params,
            ).fetchall()
        except sqlite3.OperationalError as exc:
            # Failing closed: a policy that names employees must not silently grant nothing or everything.
            raise PolicyError(f"the organization tables are not available to resolve the policy: {exc}") from exc
        finally:
            conn.close()
        windows += [Window(jid, _to_epoch(start), _to_epoch(end)) for jid, start, end in rows]
    return tuple(windows)


def windows_for(
    messages_db_path: str,
    *,
    instances: Iterable[str] = (),
    employees: Iterable[int] = (),
    departments: Iterable[int] = (),
) -> tuple[Window, ...]:
    """Windows for the numbers held by these people or departments (while they held them) and these numbers."""
    rule = _Rule(
        instances=list(instances),
        employees=list(employees),
        departments=list(departments),
        read_only=False,
        deny_tools=frozenset(),
        allow_tools=None,
    )
    return resolve_windows(messages_db_path, rule)


def window_allows(windows: tuple[Window, ...], instance_jid: str, start: int, end: int | None = None) -> bool:
    """Whether a message at ``start`` (or a span start..end) of a number falls inside any window.

    For a span the test is overlap, which is what deciding whether a chunk may be considered needs;
    exact per-message filtering happens when messages are read.
    """
    end = start if end is None else end
    for w in windows:
        if w.instance_jid != instance_jid:
            continue
        if (w.start is None or end >= w.start) and (w.end is None or start < w.end):
            return True
    return False


class _PolicyCache:
    """Reads the policy file when it changes and resolves scopes for a short time."""

    def __init__(self) -> None:
        self._lock = threading.Lock()
        self._loaded_mtime: float | None = None
        self._loaded_path: str | None = None
        self._default: _Rule | None = None
        self._clients: dict[str, _Rule] = {}
        self._scopes: dict[str, tuple[float, Scope]] = {}

    def _load(self, path: str) -> None:
        mtime = os.path.getmtime(path)
        if path == self._loaded_path and mtime == self._loaded_mtime:
            return
        with open(path, encoding="utf-8") as fh:
            self._default, self._clients = parse_policy(fh.read())
        self._loaded_path, self._loaded_mtime = path, mtime
        self._scopes.clear()
        logger.info("access policy loaded from %s (%d client rules)", path, len(self._clients))

    def scope_for(self, client_id: str, messages_db_path: str) -> Scope:
        path = os.getenv("MCP_ACCESS_POLICY")
        if not path:
            return _default_scope() if client_id == "local" else Scope(client_id=client_id, read_only=_read_only_env())
        with self._lock:
            try:
                self._load(path)
            except (OSError, PolicyError) as exc:
                # An unreadable policy denies everything rather than opening everything up.
                logger.error("access policy %s is unusable, denying access: %s", path, exc)
                return Scope(client_id=client_id, windows=(), read_only=True, allow_tools=frozenset())
            cached = self._scopes.get(client_id)
            if cached and time.monotonic() - cached[0] < POLICY_TTL_SECONDS:
                return cached[1]
            rule = self._clients.get(client_id, self._default)
            if rule is None:
                scope = Scope(client_id=client_id, read_only=_read_only_env())
            else:
                try:
                    windows = resolve_windows(messages_db_path, rule) if rule.restricts_data else None
                except PolicyError as exc:
                    logger.error("cannot resolve the access scope of client %s, denying access: %s", client_id, exc)
                    windows = ()
                scope = Scope(
                    client_id=client_id,
                    windows=windows,
                    read_only=rule.read_only,
                    deny_tools=rule.deny_tools,
                    allow_tools=rule.allow_tools,
                )
            self._scopes[client_id] = (time.monotonic(), scope)
            return scope


_policy = _PolicyCache()


def scope_for(client_id: str) -> Scope:
    """The scope a client is served under."""
    from .utils import MESSAGES_DB_PATH

    return _policy.scope_for(client_id, MESSAGES_DB_PATH)


def reload_policy() -> None:
    """Forget the cached policy (tests, or after editing the file when mtime granularity is too coarse)."""
    global _policy
    _policy = _PolicyCache()


# --- scoped connections ------------------------------------------------------------------------


def _iso(epoch: int | None) -> str | None:
    """A window bound in the form SQLite's datetime() returns, so both sides compare as text."""
    return datetime.fromtimestamp(epoch, UTC).strftime("%Y-%m-%d %H:%M:%S") if epoch is not None else None


def _table_columns(conn: sqlite3.Connection, table: str) -> list[str]:
    return [row[1] for row in conn.execute(f"PRAGMA main.table_info({table})")]


def _has_table(conn: sqlite3.Connection, name: str) -> bool:
    return conn.execute("SELECT 1 FROM main.sqlite_master WHERE name = ?", (name,)).fetchone() is not None


def _install_message_views(conn: sqlite3.Connection, scope: Scope) -> None:
    cols = _table_columns(conn, "messages")
    if "instance_jid" not in cols:
        return  # a database from before instances existed: nothing to scope or de-duplicate
    select = ", ".join(f"s.{c}" for c in cols)

    if scope.windows is None:
        conn.execute(
            f"""
            CREATE TEMP VIEW messages AS
            SELECT {select.replace("s.", "m.")} FROM main.messages m
            WHERE NOT EXISTS (
                SELECT 1 FROM main.messages o
                WHERE o.chat_jid = m.chat_jid AND o.id = m.id AND o.rowid < m.rowid
            )
            """
        )
        # Every copy, one per number that captured the message: what auditing needs.
        conn.execute(f"CREATE TEMP VIEW messages_all AS SELECT {select.replace('s.', 'm.')} FROM main.messages m")
        return

    conn.execute("CREATE TEMP TABLE _scope_windows (instance_jid TEXT, valid_from TEXT, valid_to TEXT)")
    conn.executemany(
        "INSERT INTO temp._scope_windows VALUES (?, ?, ?)",
        [(w.instance_jid, _iso(w.start), _iso(w.end)) for w in scope.windows],
    )
    conn.execute(
        """
        CREATE TEMP VIEW _scoped_messages AS
        SELECT m.rowid AS _rid, m.* FROM main.messages m
        WHERE EXISTS (
            SELECT 1 FROM temp._scope_windows w
            WHERE w.instance_jid = m.instance_jid
              AND (w.valid_from IS NULL OR datetime(m.timestamp) >= w.valid_from)
              AND (w.valid_to IS NULL OR datetime(m.timestamp) < w.valid_to)
        )
        """
    )
    conn.execute(
        f"""
        CREATE TEMP VIEW messages AS
        SELECT {select} FROM temp._scoped_messages s
        WHERE NOT EXISTS (
            SELECT 1 FROM temp._scoped_messages o
            WHERE o.chat_jid = s.chat_jid AND o.id = s.id AND o._rid < s._rid
        )
        """
    )
    conn.execute(f"CREATE TEMP VIEW messages_all AS SELECT {select} FROM temp._scoped_messages s")
    if _has_table(conn, "message_versions"):
        version_cols = ", ".join(f"v.{c}" for c in _table_columns(conn, "message_versions"))
        conn.execute(
            f"""
            CREATE TEMP VIEW message_versions AS
            SELECT {version_cols} FROM main.message_versions v
            WHERE v.instance_jid IN (SELECT instance_jid FROM temp._scope_windows)
            """
        )
    if _has_table(conn, "chat_instances"):
        chat_cols = ", ".join(f"c.{c}" for c in _table_columns(conn, "chats"))
        conn.execute(
            f"""
            CREATE TEMP VIEW chats AS
            SELECT {chat_cols} FROM main.chats c
            WHERE EXISTS (
                SELECT 1 FROM main.chat_instances ci
                WHERE ci.chat_jid = c.jid AND ci.instance_jid IN (SELECT instance_jid FROM temp._scope_windows)
            )
            """
        )


def connect_messages(path: str, *, scope: Scope | None = None) -> sqlite3.Connection:
    """A read-only connection to messages.db that only shows what the scope allows.

    Without a scope it uses the one of the current tool call. Even unrestricted callers see each
    message once: the same message captured by two numbers is stored twice and de-duplicated here.
    """
    scope = scope or current_scope()
    conn = sqlite3.connect(f"file:{path}?mode=ro", uri=True)
    try:
        conn.execute("PRAGMA busy_timeout = 5000")
        _install_message_views(conn, scope)
    except sqlite3.Error:
        conn.close()
        raise
    return conn


def connect_messages_rw(path: str) -> sqlite3.Connection:
    """A writable connection for the few tables the server owns (contact nicknames). Not scoped."""
    conn = sqlite3.connect(path, timeout=10.0)
    conn.execute("PRAGMA busy_timeout = 10000")
    return conn


def _users(jids: Iterable[str]) -> list[str]:
    return sorted({j.split("@")[0].split(":")[0].split(".")[0] for j in jids})


def connect_whatsapp(path: str, *, scope: Scope | None = None) -> sqlite3.Connection:
    """A read-only connection to whatsapp.db showing only the address books of the scope's numbers."""
    scope = scope or current_scope()
    conn = sqlite3.connect(f"file:{path}?mode=ro", uri=True)
    try:
        conn.execute("PRAGMA busy_timeout = 5000")
        if scope.windows is not None and _has_table(conn, "whatsmeow_contacts"):
            conn.execute("CREATE TEMP TABLE _scope_users (user TEXT)")
            conn.executemany("INSERT INTO temp._scope_users VALUES (?)", [(u,) for u in _users(scope.instance_jids())])
            cols = ", ".join(f"c.{c}" for c in _table_columns(conn, "whatsmeow_contacts"))
            # our_jid is the device JID: user[.agent][:device]@server.
            conn.execute(
                f"""
                CREATE TEMP VIEW whatsmeow_contacts AS
                SELECT {cols} FROM main.whatsmeow_contacts c
                WHERE EXISTS (
                    SELECT 1 FROM temp._scope_users u
                    WHERE c.our_jid LIKE u.user || '@%' OR c.our_jid LIKE u.user || ':%' OR c.our_jid LIKE u.user || '.%'
                )
                """
            )
    except sqlite3.Error:
        conn.close()
        raise
    return conn


# --- access log -----------------------------------------------------------------------------------


def _result_count(result: Any) -> int | None:
    if isinstance(result, list):
        return len(result)
    if isinstance(result, dict):
        for key in ("results", "messages", "employees", "instances"):
            if isinstance(result.get(key), list):
                return len(result[key])
    return None


def _describe_params(params: dict[str, Any], read_only_tool: bool) -> str:
    """What was asked. For tools that act on WhatsApp only the parameter names are kept: the values
    are message bodies and recipients, which belong to the conversation, not to the access log."""
    if not read_only_tool:
        return json.dumps(sorted(params))
    return json.dumps({k: str(v)[:200] for k, v in params.items() if v is not None}, ensure_ascii=False)[:1000]


def record_call(
    tool: str, caller: str, params: dict[str, Any], result: Any, outcome: str, read_only_tool: bool
) -> None:
    """Tell the bridge which tool a client called, so the panel can show who looked at what.

    Best effort: a bridge that is down must not break the tool call, so failures are only logged.
    """
    if os.getenv("MCP_ACCESS_LOG", "true").strip().lower() in {"0", "false", "no"}:
        return
    try:
        import requests

        from .bridge import _get_headers
        from .utils import WHATSAPP_API_BASE_URL

        count = _result_count(result)
        requests.post(
            f"{WHATSAPP_API_BASE_URL}/access-log",
            json={
                "actor": f"mcp:{caller}",
                "client_id": caller,
                "action": f"tool:{tool}" if outcome == "ok" else f"tool:{tool}:{outcome}",
                "resource": tool,
                "params": _describe_params(params, read_only_tool),
                "result_count": count,
            },
            headers=_get_headers(),
            timeout=3,
        )
    except Exception as exc:  # noqa: BLE001 - never let auditing break a tool
        logger.warning("could not record the access log entry for %s: %s", tool, exc)
