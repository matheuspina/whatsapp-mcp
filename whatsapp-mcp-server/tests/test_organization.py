"""Organization and audit queries, run against the bridge's real schema."""

import sqlite3

import pytest

from lib import access, database
from lib.database import DatabaseError

JOAO = "5511900000001@s.whatsapp.net"
MARIA = "5511900000002@s.whatsapp.net"
CLIENTE = "5511977770000@s.whatsapp.net"


@pytest.fixture
def org_db(temp_messages_db, monkeypatch):
    """The fixture database plus two departments, three people and two numbers."""
    monkeypatch.setattr("lib.database.MESSAGES_DB_PATH", temp_messages_db)
    monkeypatch.setattr("lib.utils.MESSAGES_DB_PATH", temp_messages_db)
    conn = sqlite3.connect(temp_messages_db)
    for stmt in [
        "INSERT INTO departments (id, name, description) VALUES (1, 'Comercial', 'Vendas')",
        "INSERT INTO departments (id, name, description) VALUES (2, 'Financeiro', 'Cobrança')",
        "INSERT INTO employees (id, department_id, name, role, email) VALUES (1, 1, 'João Silva', 'Vendedor', 'joao@x.com')",
        "INSERT INTO employees (id, department_id, name, role) VALUES (2, 2, 'João Carlos', 'Analista Financeiro')",
        "INSERT INTO employees (id, department_id, name, role, active) VALUES (3, 1, 'Maria', 'Vendedora', 0)",
        f"INSERT INTO instances (id, phone_jid, employee_id, alias, status, allow_send) VALUES (1, '{JOAO}', 1, 'Vendas 1', 'connected', 1)",
        f"INSERT INTO instances (id, phone_jid, employee_id, alias, status) VALUES (2, '{MARIA}', 2, 'Cobrança', 'disconnected')",
        "INSERT INTO instance_assignments (instance_id, employee_id, valid_from) VALUES (1, 1, '2026-01-01 00:00:00+00:00')",
        "INSERT INTO instance_assignments (instance_id, employee_id, valid_from) VALUES (2, 2, '2026-01-01 00:00:00+00:00')",
        f"INSERT INTO chats (jid, name) VALUES ('{CLIENTE}', 'Cliente Fulano')",
    ]:
        conn.execute(stmt)
    conn.commit()
    conn.close()
    return temp_messages_db


def add_message(db, instance, msg_id, content, ts, *, deleted=False, edited=False):
    conn = sqlite3.connect(db)
    conn.execute(
        """INSERT INTO messages (id, chat_jid, sender, sender_name, content, timestamp, is_from_me, instance_jid,
                                 is_deleted_remote, is_edited)
           VALUES (?, ?, ?, ?, ?, ?, 0, ?, ?, ?)""",
        (msg_id, CLIENTE, CLIENTE, "Fulano", content, ts, instance, int(deleted), int(edited)),
    )
    conn.commit()
    conn.close()


def test_departments_and_employees_read_the_real_schema(org_db):
    departments = database.list_departments()
    assert [d["name"] for d in departments] == ["Comercial", "Financeiro"]

    employees = database.list_employees()
    assert [e["name"] for e in employees] == ["João Carlos", "João Silva"]  # the inactive one is hidden
    joao = next(e for e in employees if e["name"] == "João Silva")
    assert joao["department_name"] == "Comercial"
    assert joao["email"] == "joao@x.com"
    assert joao["instance_jids"] == [JOAO]

    assert len(database.list_employees(include_inactive=True)) == 3
    assert [e["name"] for e in database.list_employees(department_id=2)] == ["João Carlos"]
    assert [e["name"] for e in database.list_employees(query="analista")] == ["João Carlos"]


def test_resolve_employee_never_guesses_between_two_joaos(org_db):
    result = database.resolve_employee("joão")
    assert result["found"] and result["ambiguous"]
    assert {e["department_name"] for e in result["employees"]} == {"Comercial", "Financeiro"}

    exact = database.resolve_employee("João Silva")
    assert exact["found"] and not exact["ambiguous"]
    assert [e["id"] for e in exact["employees"]] == [1]

    assert database.resolve_employee("Zé") == {"query": "Zé", "found": False, "ambiguous": False, "employees": []}
    with pytest.raises(DatabaseError):
        database.resolve_employee("  ")


def test_list_instances_reads_the_real_schema(org_db):
    instances = database.list_instances()
    assert [i["alias"] for i in instances] == ["Vendas 1", "Cobrança"]
    first = instances[0]
    assert first["phone_jid"] == JOAO
    assert first["phone_number"] == "5511900000001"
    assert first["employee_name"] == "João Silva"
    assert first["department_name"] == "Comercial"
    assert first["allow_send"] is True and instances[1]["allow_send"] is False
    assert first["status"] == "connected"


def test_removed_instances_are_hidden_unless_asked(org_db):
    conn = sqlite3.connect(org_db)
    conn.execute("UPDATE instances SET status = 'removed' WHERE id = 2")
    conn.commit()
    conn.close()
    assert len(database.list_instances()) == 1
    assert len(database.list_instances(include_removed=True)) == 2


def test_deleted_messages_carry_attribution_and_the_revoked_text(org_db):
    add_message(org_db, JOAO, "M1", "desconto de 30%", "2026-03-10 10:00:00+00:00", deleted=True)
    add_message(org_db, JOAO, "M2", "bom dia", "2026-03-10 10:01:00+00:00")
    conn = sqlite3.connect(org_db)
    conn.execute(
        "INSERT INTO message_versions (instance_jid, chat_jid, message_id, content, reason) VALUES (?, ?, 'M1', 'desconto de 30%', 'delete')",
        (JOAO, CLIENTE),
    )
    conn.commit()
    conn.close()

    deleted = database.get_audit_deleted_messages()
    assert [m["id"] for m in deleted] == ["M1"]
    row = deleted[0]
    assert row["instance_alias"] == "Vendas 1"
    assert row["employee_name"] == "João Silva"
    assert row["department_name"] == "Comercial"
    assert row["chat_name"] == "Cliente Fulano"
    assert row["versions"] == [
        {"content": "desconto de 30%", "reason": "delete", "recorded_at": row["versions"][0]["recorded_at"]}
    ]

    assert database.get_audit_deleted_messages(employee_id=2) == []
    assert len(database.get_audit_deleted_messages(department_id=1)) == 1


def test_audit_trail_shows_every_copy_and_who_held_the_number(org_db):
    # The same message captured by two numbers, and a number that changed hands.
    add_message(org_db, JOAO, "M1", "olá", "2026-03-10 10:00:00+00:00")
    add_message(org_db, MARIA, "M1", "olá", "2026-03-10 10:00:00+00:00")
    conn = sqlite3.connect(org_db)
    conn.execute("UPDATE instance_assignments SET valid_to = '2026-03-05 00:00:00+00:00' WHERE instance_id = 1")
    conn.execute(
        "INSERT INTO instance_assignments (instance_id, employee_id, valid_from) VALUES (1, 3, '2026-03-05 00:00:00+00:00')"
    )
    conn.commit()
    conn.close()

    trail = database.get_audit_trail(chat_jid=CLIENTE)
    assert trail["count"] == 2  # both copies, unlike the de-duplicated view used by other tools
    by_instance = {m["instance_jid"]: m for m in trail["messages"]}
    assert by_instance[JOAO]["employee_name"] == "Maria"  # she held the number by March 10th
    assert by_instance[MARIA]["employee_name"] == "João Carlos"

    only_maria = database.get_audit_trail(employee_id=3)
    assert [m["instance_jid"] for m in only_maria["messages"]] == [JOAO]

    with pytest.raises(DatabaseError, match="at least one"):
        database.get_audit_trail()


def test_other_tools_see_each_message_once(org_db):
    add_message(org_db, JOAO, "M1", "olá", "2026-03-10 10:00:00+00:00")
    add_message(org_db, MARIA, "M1", "olá", "2026-03-10 10:00:00+00:00")
    conn = access.connect_messages(org_db)
    try:
        assert conn.execute("SELECT COUNT(*) FROM messages WHERE id = 'M1'").fetchone()[0] == 1
        assert conn.execute("SELECT COUNT(*) FROM messages_all WHERE id = 'M1'").fetchone()[0] == 2
    finally:
        conn.close()


def test_a_restricted_scope_only_sees_its_numbers_and_the_people_who_held_them(org_db):
    scope = access.Scope(client_id="comercial", windows=(access.Window(JOAO),))
    token = access.set_scope(scope)
    try:
        assert [d["name"] for d in database.list_departments()] == ["Comercial"]
        assert [e["name"] for e in database.list_employees(include_inactive=True)] == ["João Silva"]
        assert [i["alias"] for i in database.list_instances()] == ["Vendas 1"]
        add_message(org_db, MARIA, "SECRET", "confidencial", "2026-03-10 10:00:00+00:00")
        assert database.get_audit_trail(chat_jid=CLIENTE)["count"] == 0
        with pytest.raises(DatabaseError, match="unrestricted"):
            database.list_access_log()
    finally:
        access.reset_scope(token)


def test_missing_organization_tables_raise_instead_of_returning_nothing(tmp_path, monkeypatch):
    path = str(tmp_path / "old.db")
    conn = sqlite3.connect(path)
    conn.execute("CREATE TABLE messages (id TEXT, chat_jid TEXT)")
    conn.commit()
    conn.close()
    monkeypatch.setattr("lib.database.MESSAGES_DB_PATH", path)

    with pytest.raises(DatabaseError, match="migrations"):
        database.list_employees()
    with pytest.raises(DatabaseError, match="migrations"):
        database.list_instances()
