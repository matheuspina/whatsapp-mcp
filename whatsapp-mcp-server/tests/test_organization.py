"""Tests for organizational models, database queries, and MCP tools."""

import sqlite3
import pytest
from lib.database import (
    list_departments,
    list_employees,
    resolve_employee,
    list_instances,
    get_audit_deleted_messages,
)
from whatsapp import (
    list_departments as whatsapp_list_departments,
    list_employees as whatsapp_list_employees,
    resolve_employee as whatsapp_resolve_employee,
    list_instances as whatsapp_list_instances,
    get_audit_deleted_messages as whatsapp_get_audit_deleted_messages,
)


def seed_org_data(db_path: str):
    conn = sqlite3.connect(db_path)
    cur = conn.cursor()

    # Insert departments
    cur.execute("INSERT INTO departments (id, name, description) VALUES (1, 'Comercial', 'Vendas e prospecção')")
    cur.execute("INSERT INTO departments (id, name, description) VALUES (2, 'Suporte', 'Atendimento ao cliente')")

    # Insert employees
    cur.execute(
        "INSERT INTO employees (id, name, role, department_id, phone_number) VALUES (1, 'Matheus Pina', 'Gerente Comercial', 1, '5511999998888')"
    )
    cur.execute(
        "INSERT INTO employees (id, name, role, department_id, phone_number) VALUES (2, 'Carlos Souza', 'Atendente N1', 2, '5511888887777')"
    )

    # Insert instance
    cur.execute(
        "INSERT INTO instances (jid, phone_number, alias, employee_id, status, is_active) VALUES ('inst1@s.whatsapp.net', '5511999998888', 'Vendas 01', 1, 'connected', 1)"
    )

    # Insert deleted message
    cur.execute(
        """INSERT INTO messages (id, chat_jid, sender, content, timestamp, is_from_me, instance_jid, is_deleted_remote)
           VALUES ('del_msg1', '123456789@s.whatsapp.net', '123456789@s.whatsapp.net', 'Preço confidencial: R$ 5000', '2026-03-26T10:00:00', 0, 'inst1@s.whatsapp.net', 1)"""
    )
    conn.commit()
    conn.close()


def test_list_departments(temp_messages_db, monkeypatch):
    monkeypatch.setenv("MESSAGES_DB_PATH", temp_messages_db)
    monkeypatch.setattr("lib.database.MESSAGES_DB_PATH", temp_messages_db)
    monkeypatch.setattr("whatsapp.MESSAGES_DB_PATH", temp_messages_db)
    seed_org_data(temp_messages_db)

    depts = list_departments()
    assert len(depts) == 2
    assert depts[0]["name"] == "Comercial"
    assert depts[1]["name"] == "Suporte"

    # Test via whatsapp wrapper
    wrapped_depts = whatsapp_list_departments()
    assert len(wrapped_depts) == 2


def test_list_and_resolve_employees(temp_messages_db, monkeypatch):
    monkeypatch.setenv("MESSAGES_DB_PATH", temp_messages_db)
    monkeypatch.setattr("lib.database.MESSAGES_DB_PATH", temp_messages_db)
    monkeypatch.setattr("whatsapp.MESSAGES_DB_PATH", temp_messages_db)
    seed_org_data(temp_messages_db)

    # List all
    employees = list_employees()
    assert len(employees) == 2
    assert employees[0]["name"] == "Carlos Souza"
    assert employees[1]["name"] == "Matheus Pina"
    assert employees[1]["department_name"] == "Comercial"

    # Filter by department
    comercial_emps = list_employees(department_id=1)
    assert len(comercial_emps) == 1
    assert comercial_emps[0]["name"] == "Matheus Pina"

    # Resolve by partial name
    resolved = resolve_employee("Matheus")
    assert resolved["found"] is True
    assert len(resolved["employees"]) == 1
    assert resolved["employees"][0]["role"] == "Gerente Comercial"

    # Resolve by role
    resolved_role = whatsapp_resolve_employee("Atendente")
    assert resolved_role["found"] is True
    assert resolved_role["employees"][0]["name"] == "Carlos Souza"


def test_list_instances(temp_messages_db, monkeypatch):
    monkeypatch.setenv("MESSAGES_DB_PATH", temp_messages_db)
    monkeypatch.setattr("lib.database.MESSAGES_DB_PATH", temp_messages_db)
    monkeypatch.setattr("whatsapp.MESSAGES_DB_PATH", temp_messages_db)
    seed_org_data(temp_messages_db)

    instances = list_instances()
    assert len(instances) == 1
    inst = instances[0]
    assert inst["alias"] == "Vendas 01"
    assert inst["employee_name"] == "Matheus Pina"
    assert inst["department_name"] == "Comercial"
    assert inst["status"] == "connected"


def test_get_audit_deleted_messages(temp_messages_db, monkeypatch):
    monkeypatch.setenv("MESSAGES_DB_PATH", temp_messages_db)
    monkeypatch.setattr("lib.database.MESSAGES_DB_PATH", temp_messages_db)
    monkeypatch.setattr("whatsapp.MESSAGES_DB_PATH", temp_messages_db)
    seed_org_data(temp_messages_db)

    deleted_msgs = get_audit_deleted_messages()
    assert len(deleted_msgs) == 1
    d_msg = deleted_msgs[0]
    assert d_msg["id"] == "del_msg1"
    assert d_msg["content"] == "Preço confidencial: R$ 5000"
    assert d_msg["is_deleted_remote"] is True
    assert d_msg["instance_jid"] == "inst1@s.whatsapp.net"
