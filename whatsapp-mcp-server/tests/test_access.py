"""Access policy and scoped database connections."""

import json
import sqlite3

import pytest

from lib import access, database
from lib.access import AccessDenied, PolicyError, Scope, Window

JOAO = "5511900000001@s.whatsapp.net"
MARIA = "5511900000002@s.whatsapp.net"
CLIENTE = "5511977770000@s.whatsapp.net"
GROUP = "120363000000000000@g.us"


@pytest.fixture
def org(temp_messages_db, monkeypatch):
    monkeypatch.setattr("lib.utils.MESSAGES_DB_PATH", temp_messages_db)
    monkeypatch.setattr("lib.database.MESSAGES_DB_PATH", temp_messages_db)
    conn = sqlite3.connect(temp_messages_db)
    for stmt in [
        "INSERT INTO departments (id, name) VALUES (1, 'Comercial'), (2, 'Financeiro')",
        "INSERT INTO employees (id, department_id, name) VALUES (1, 1, 'Ana'), (2, 2, 'Bia'), (3, 2, 'Caio')",
        f"INSERT INTO instances (id, phone_jid, employee_id, alias) VALUES (1, '{JOAO}', 3, 'Vendas'), (2, '{MARIA}', 2, 'Cobrança')",
        "INSERT INTO instance_assignments (instance_id, employee_id, valid_from, valid_to) VALUES (1, 1, '1970-01-01 00:00:00+00:00', '2026-03-05 00:00:00+00:00')",
        "INSERT INTO instance_assignments (instance_id, employee_id, valid_from) VALUES (1, 3, '2026-03-05 00:00:00+00:00')",
        "INSERT INTO instance_assignments (instance_id, employee_id, valid_from) VALUES (2, 2, '1970-01-01 00:00:00+00:00')",
        f"INSERT INTO chats (jid, name) VALUES ('{CLIENTE}', 'Cliente'), ('{GROUP}', 'Grupo')",
        f"INSERT INTO chat_instances (instance_jid, chat_jid) VALUES ('{JOAO}', '{CLIENTE}'), ('{MARIA}', '{GROUP}'), ('{JOAO}', '{GROUP}')",
    ]:
        conn.execute(stmt)
    for instance, chat, msg_id, ts in [
        (JOAO, CLIENTE, "J-OLD", "2026-03-02 10:00:00+00:00"),
        (JOAO, CLIENTE, "J-NEW", "2026-03-10 10:00:00+00:00"),
        (MARIA, GROUP, "M1", "2026-03-10 10:00:00+00:00"),
        (JOAO, GROUP, "M1", "2026-03-10 10:00:00+00:00"),
    ]:
        conn.execute(
            "INSERT INTO messages (id, chat_jid, sender, content, timestamp, is_from_me, instance_jid) VALUES (?, ?, ?, 'x', ?, 0, ?)",
            (msg_id, chat, chat, ts, instance),
        )
    conn.commit()
    conn.close()
    return temp_messages_db


def ids(conn, table="messages"):
    return sorted(r[0] for r in conn.execute(f"SELECT id FROM {table}"))


class TestScopedConnections:
    def test_unrestricted_callers_see_each_message_once(self, org):
        conn = access.connect_messages(org)
        assert ids(conn).count("M1") == 1
        assert ids(conn, "messages_all").count("M1") == 2

    def test_only_the_scope_numbers_are_visible(self, org):
        conn = access.connect_messages(org, scope=Scope(windows=(Window(MARIA),)))
        assert ids(conn) == ["M1"]
        assert [r[0] for r in conn.execute("SELECT jid FROM chats")] == [GROUP]

    def test_a_period_limits_what_is_visible(self, org):
        after = int(__import__("datetime").datetime(2026, 3, 5, tzinfo=__import__("datetime").UTC).timestamp())
        conn = access.connect_messages(org, scope=Scope(windows=(Window(JOAO, after, None),)))
        assert "J-NEW" in ids(conn) and "J-OLD" not in ids(conn)
        conn = access.connect_messages(org, scope=Scope(windows=(Window(JOAO, None, after),)))
        assert "J-OLD" in ids(conn) and "J-NEW" not in ids(conn)

    def test_an_empty_scope_sees_nothing(self, org):
        conn = access.connect_messages(org, scope=Scope(windows=()))
        assert ids(conn) == [] and ids(conn, "messages_all") == []

    def test_connections_are_read_only(self, org):
        conn = access.connect_messages(org)
        with pytest.raises(sqlite3.OperationalError):
            conn.execute("DELETE FROM messages")
        with pytest.raises(sqlite3.OperationalError):
            access.connect_messages(org, scope=Scope(windows=(Window(JOAO),))).execute(
                "UPDATE messages SET content = 'y'"
            )

    def test_versions_follow_the_scope(self, org):
        conn = sqlite3.connect(org)
        conn.execute(
            "INSERT INTO message_versions (instance_jid, chat_jid, message_id, content, reason) VALUES (?, ?, 'M1', 'a', 'edit')",
            (MARIA, GROUP),
        )
        conn.execute(
            "INSERT INTO message_versions (instance_jid, chat_jid, message_id, content, reason) VALUES (?, ?, 'M1', 'b', 'edit')",
            (JOAO, GROUP),
        )
        conn.commit()
        conn.close()
        scoped = access.connect_messages(org, scope=Scope(windows=(Window(JOAO),)))
        assert [r[0] for r in scoped.execute("SELECT content FROM message_versions")] == ["b"]

    def test_a_database_from_before_instances_is_left_alone(self, tmp_path):
        path = str(tmp_path / "old.db")
        conn = sqlite3.connect(path)
        conn.execute("CREATE TABLE messages (id TEXT, chat_jid TEXT)")
        conn.execute("INSERT INTO messages VALUES ('a', 'c')")
        conn.commit()
        conn.close()
        assert ids(access.connect_messages(path, scope=Scope(windows=(Window(JOAO),)))) == ["a"]

    def test_existing_readers_inherit_the_scope(self, org):
        token = access.set_scope(Scope(windows=(Window(MARIA),)))
        try:
            chats = database.list_chats()
            assert {c["jid"] for c in chats} == {GROUP}
        finally:
            access.reset_scope(token)
        assert {CLIENTE, GROUP} <= {c["jid"] for c in database.list_chats()}


class TestContactBooks:
    def test_only_the_address_books_of_the_scope_numbers(self, tmp_path):
        path = str(tmp_path / "whatsapp.db")
        conn = sqlite3.connect(path)
        conn.execute("CREATE TABLE whatsmeow_contacts (our_jid TEXT, their_jid TEXT, full_name TEXT)")
        conn.executemany(
            "INSERT INTO whatsmeow_contacts VALUES (?, ?, ?)",
            [
                ("5511900000001:7@s.whatsapp.net", "c1", "Do Joao"),
                ("5511900000002.0:3@s.whatsapp.net", "c2", "Da Maria"),
                ("5511900000002@s.whatsapp.net", "c3", "Da Maria 2"),
            ],
        )
        conn.commit()
        conn.close()

        joao = access.connect_whatsapp(path, scope=Scope(windows=(Window(JOAO),)))
        assert [r[0] for r in joao.execute("SELECT full_name FROM whatsmeow_contacts")] == ["Do Joao"]
        maria = access.connect_whatsapp(path, scope=Scope(windows=(Window(MARIA),)))
        assert sorted(r[0] for r in maria.execute("SELECT full_name FROM whatsmeow_contacts")) == [
            "Da Maria",
            "Da Maria 2",
        ]
        everyone = access.connect_whatsapp(path)
        assert everyone.execute("SELECT COUNT(*) FROM whatsmeow_contacts").fetchone()[0] == 3


class TestPolicyFile:
    def write(self, tmp_path, monkeypatch, doc):
        path = tmp_path / "policy.json"
        path.write_text(json.dumps(doc) if not isinstance(doc, str) else doc)
        monkeypatch.setenv("MCP_ACCESS_POLICY", str(path))
        access.reload_policy()
        return path

    def test_no_policy_means_no_restriction(self, monkeypatch):
        monkeypatch.delenv("MCP_ACCESS_POLICY", raising=False)
        access.reload_policy()
        scope = access.scope_for("anyone")
        assert not scope.restricted and not scope.read_only

    def test_read_only_environment_switch(self, monkeypatch):
        monkeypatch.delenv("MCP_ACCESS_POLICY", raising=False)
        monkeypatch.setenv("MCP_READ_ONLY", "true")
        access.reload_policy()
        assert access.scope_for("anyone").read_only

    def test_a_client_rule_grants_departments_for_the_periods_they_held_the_numbers(self, org, tmp_path, monkeypatch):
        self.write(tmp_path, monkeypatch, {"clients": {"comercial-bot": {"departments": [1], "read_only": True}}})
        scope = access.scope_for("comercial-bot")
        assert scope.read_only and scope.restricted
        # Comercial only held JOAO's number, until March 5th.
        assert [w.instance_jid for w in scope.windows or ()] == [JOAO]
        assert (scope.windows or ())[0].end is not None
        conn = access.connect_messages(org, scope=scope)
        assert ids(conn) == ["J-OLD", "M1"] or ids(conn) == ["J-OLD"]
        assert "J-NEW" not in ids(conn)

    def test_employee_and_instance_grants_combine(self, org, tmp_path, monkeypatch):
        self.write(tmp_path, monkeypatch, {"clients": {"c": {"employees": [2], "instances": [JOAO]}}})
        scope = access.scope_for("c")
        assert {w.instance_jid for w in scope.windows or ()} == {JOAO, MARIA}

    def test_unlisted_clients_get_the_default_rule(self, org, tmp_path, monkeypatch):
        self.write(tmp_path, monkeypatch, {"default": {"read_only": True}, "clients": {"admin": {"read_only": False}}})
        assert access.scope_for("stranger").read_only
        assert not access.scope_for("admin").read_only
        assert not access.scope_for("admin").restricted

    def test_tool_lists(self, org, tmp_path, monkeypatch):
        self.write(
            tmp_path,
            monkeypatch,
            {
                "clients": {
                    "c": {"deny_tools": ["get_audit_trail"], "allow_tools": None},
                    "d": {"allow_tools": ["search_messages"]},
                }
            },
        )
        with pytest.raises(AccessDenied):
            access.scope_for("c").check_tool("get_audit_trail", read_only_tool=True)
        access.scope_for("c").check_tool("search_messages", read_only_tool=True)
        access.scope_for("d").check_tool("search_messages", read_only_tool=True)
        with pytest.raises(AccessDenied):
            access.scope_for("d").check_tool("list_messages", read_only_tool=True)

    @pytest.mark.parametrize(
        "doc",
        [
            "not json",
            "[]",
            {"default": []},
            {"clients": []},
            {"clients": {"c": {"departments": ["x"]}}},
            {"clients": {"c": {"allow_tools": "x"}}},
        ],
    )
    def test_a_broken_policy_denies_everything_instead_of_opening_up(self, tmp_path, monkeypatch, doc):
        self.write(tmp_path, monkeypatch, doc)
        scope = access.scope_for("c")
        assert scope.restricted and scope.windows == () and scope.read_only
        with pytest.raises(AccessDenied):
            scope.check_tool("list_messages", read_only_tool=True)

    def test_a_missing_policy_file_denies_everything(self, tmp_path, monkeypatch):
        monkeypatch.setenv("MCP_ACCESS_POLICY", str(tmp_path / "nope.json"))
        access.reload_policy()
        assert access.scope_for("c").windows == ()

    def test_organization_data_that_cannot_be_read_denies_instead_of_granting_everything(self, tmp_path, monkeypatch):
        empty = str(tmp_path / "empty.db")
        sqlite3.connect(empty).close()
        monkeypatch.setattr("lib.utils.MESSAGES_DB_PATH", empty)
        self.write(tmp_path, monkeypatch, {"clients": {"c": {"departments": [1]}}})
        assert access.scope_for("c").windows == ()

    def test_edits_to_the_file_are_picked_up(self, org, tmp_path, monkeypatch):
        path = self.write(tmp_path, monkeypatch, {"clients": {"c": {"instances": [JOAO]}}})
        assert {w.instance_jid for w in access.scope_for("c").windows or ()} == {JOAO}
        import os

        path.write_text(json.dumps({"clients": {"c": {"instances": [MARIA]}}}))
        os.utime(path, (path.stat().st_atime, path.stat().st_mtime + 5))
        assert {w.instance_jid for w in access.scope_for("c").windows or ()} == {MARIA}


def test_parse_policy_rejects_booleans_as_ids():
    with pytest.raises(PolicyError):
        access.parse_policy('{"clients": {"c": {"employees": [true]}}}')


class TestAccessLog:
    def test_a_tool_call_is_reported_to_the_bridge(self, monkeypatch):
        monkeypatch.setenv("MCP_ACCESS_LOG", "true")
        sent: dict = {}

        def fake_post(url, json, headers, timeout):
            sent.update(url=url, body=json, timeout=timeout)

        monkeypatch.setattr("requests.post", fake_post)
        access.record_call(
            "search_messages", "client-1", {"query": "bolo", "limit": None}, {"results": [1, 2]}, "ok", True
        )
        assert sent["url"].endswith("/access-log")
        assert sent["body"]["actor"] == "mcp:client-1" and sent["body"]["action"] == "tool:search_messages"
        assert sent["body"]["result_count"] == 2
        assert json.loads(sent["body"]["params"]) == {"query": "bolo"}
        assert sent["timeout"] <= 5

    def test_message_bodies_are_not_logged_for_tools_that_send(self, monkeypatch):
        monkeypatch.setenv("MCP_ACCESS_LOG", "true")
        sent: dict = {}
        monkeypatch.setattr("requests.post", lambda url, json, headers, timeout: sent.update(body=json))
        access.record_call(
            "send_message", "c", {"recipient": "551199", "message": "segredo"}, {"success": True}, "ok", False
        )
        assert "segredo" not in sent["body"]["params"] and "551199" not in sent["body"]["params"]
        assert json.loads(sent["body"]["params"]) == ["message", "recipient"]

    def test_denied_attempts_are_recorded_as_such(self, monkeypatch):
        monkeypatch.setenv("MCP_ACCESS_LOG", "true")
        sent: dict = {}
        monkeypatch.setattr("requests.post", lambda url, json, headers, timeout: sent.update(body=json))
        access.record_call("send_message", "c", {}, None, "denied", False)
        assert sent["body"]["action"] == "tool:send_message:denied"

    def test_a_bridge_that_is_down_does_not_break_the_call(self, monkeypatch):
        monkeypatch.setenv("MCP_ACCESS_LOG", "true")

        def boom(*_a, **_k):
            raise ConnectionError("bridge down")

        monkeypatch.setattr("requests.post", boom)
        access.record_call("list_chats", "c", {}, [], "ok", True)  # must not raise

    def test_can_be_switched_off(self, monkeypatch):
        monkeypatch.setenv("MCP_ACCESS_LOG", "false")
        monkeypatch.setattr("requests.post", lambda *a, **k: pytest.fail("should not post"))
        access.record_call("list_chats", "c", {}, [], "ok", True)
