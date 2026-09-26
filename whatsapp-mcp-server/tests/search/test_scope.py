"""Search across several monitored numbers: separation, organization filters and access scope."""

import sqlite3
from datetime import timedelta

import pytest

from lib import access
from lib.access import Scope, Window
from search.search import SearchService
from tests.bridge_schema import create_bridge_schema
from tests.search.conftest import EPOCH, FakeEmbedder, Store, build_index, init_whatsapp_db, ts

JOAO = "5511900000001@s.whatsapp.net"
MARIA = "5511900000002@s.whatsapp.net"
CLIENT = "5511977770000@s.whatsapp.net"
GROUP = "120363000000000000@g.us"

ANA, BIA, CAIO = 1, 2, 3
COMERCIAL, FINANCEIRO = 1, 2

# Ana held JOAO's number until the handover, Caio (Financeiro) after it.
HANDOVER_MINUTES = 24 * 60


def _db_ts(minutes: float) -> str:
    return ts(minutes)


def _add(conn, instance, chat, msg_id, content, minutes, from_me=0):
    conn.execute("INSERT OR IGNORE INTO chats (jid, name) VALUES (?, ?)", (chat, chat.split("@")[0]))
    conn.execute(
        """INSERT INTO messages (id, chat_jid, sender, sender_name, content, timestamp, is_from_me, instance_jid)
           VALUES (?, ?, ?, ?, ?, ?, ?, ?)""",
        (msg_id, chat, CLIENT if not from_me else instance, "x", content, _db_ts(minutes), from_me, instance),
    )


@pytest.fixture
def two_numbers(tmp_path) -> Store:
    messages_db = str(tmp_path / "messages.db")
    whatsapp_db = str(tmp_path / "whatsapp.db")
    conn = sqlite3.connect(messages_db)
    create_bridge_schema(conn)
    handover = _db_ts(HANDOVER_MINUTES)
    for stmt in [
        f"INSERT INTO departments (id, name) VALUES ({COMERCIAL}, 'Comercial'), ({FINANCEIRO}, 'Financeiro')",
        f"INSERT INTO employees (id, department_id, name) VALUES ({ANA}, {COMERCIAL}, 'Ana'), ({BIA}, {FINANCEIRO}, 'Bia'), ({CAIO}, {FINANCEIRO}, 'Caio')",
        f"INSERT INTO instances (id, phone_jid, employee_id, alias) VALUES (1, '{JOAO}', {CAIO}, 'Vendas'), (2, '{MARIA}', {BIA}, 'Cobrança')",
        f"INSERT INTO instance_assignments (instance_id, employee_id, valid_from, valid_to) VALUES (1, {ANA}, '1970-01-01 00:00:00+00:00', '{handover}')",
        f"INSERT INTO instance_assignments (instance_id, employee_id, valid_from) VALUES (1, {CAIO}, '{handover}')",
        "INSERT INTO instance_assignments (instance_id, employee_id, valid_from) VALUES (2, 2, '1970-01-01 00:00:00+00:00')",
    ]:
        conn.execute(stmt)

    _add(conn, JOAO, CLIENT, "J1", "quero um bolo de cenoura", 0)
    _add(conn, JOAO, CLIENT, "J2", "o orçamento do bolo foi aprovado", 10)
    _add(conn, JOAO, CLIENT, "J4", "bolo antes da troca", HANDOVER_MINUTES - 5)
    _add(conn, JOAO, CLIENT, "J5", "bolo depois da troca", HANDOVER_MINUTES + 5)
    _add(conn, JOAO, CLIENT, "J3", "boleto do bolo vence hoje", 2 * 24 * 60)
    _add(conn, MARIA, CLIENT, "M1", "bolo de chocolate no financeiro", 5)
    _add(conn, MARIA, CLIENT, "M2", "cobrança do orçamento em atraso", 3 * 24 * 60)
    # The same group message captured by both numbers.
    for instance in (JOAO, MARIA):
        _add(conn, instance, GROUP, "G1", "reunião sobre o bolo da festa", 20)
    conn.commit()
    conn.close()

    init_whatsapp_db(whatsapp_db)
    wa = sqlite3.connect(whatsapp_db)
    wa.executemany(
        "INSERT INTO whatsmeow_contacts (our_jid, their_jid, full_name) VALUES (?, ?, ?)",
        [
            (JOAO.replace("@", ":7@"), CLIENT, "Cliente da Vendas"),
            (MARIA.replace("@", ":3@"), CLIENT, "Cliente da Cobrança"),
        ],
    )
    wa.commit()
    wa.close()
    return Store(dir=str(tmp_path), messages_db_path=messages_db, whatsapp_db_path=whatsapp_db)


@pytest.fixture
def service(two_numbers, tmp_path):
    index_path = str(tmp_path / "index.db")
    embedder = FakeEmbedder()
    build_index(two_numbers, index_path, embedder)
    return SearchService(index_path, embedder_factory=lambda: embedder, messages_db_path=two_numbers.messages_db_path)


def hit_ids(response) -> set[str]:
    return {m["id"] for r in response["results"] for m in r["messages"] if m["keyword_hit"]}


def all_ids(response) -> set[str]:
    return {m["id"] for r in response["results"] for m in r["messages"]}


def numbers(response) -> set[str]:
    return {r["number"]["instance_jid"] for r in response["results"] if "number" in r}


def test_the_same_customer_on_two_numbers_is_two_conversations(service):
    conn = sqlite3.connect(service._index_db_path)
    rows = conn.execute("SELECT DISTINCT instance_jid FROM chunks WHERE chat_jid = ?", (CLIENT,)).fetchall()
    assert {r[0] for r in rows} == {JOAO, MARIA}
    # Excerpts never mix the two: every excerpt of the client chat belongs to exactly one number.
    for instance, message_ids in conn.execute(
        "SELECT instance_jid, message_ids FROM chunks WHERE chat_jid = ?", (CLIENT,)
    ):
        ids = set(__import__("json").loads(message_ids))
        assert not (ids & {"M1", "M2"}) if instance == JOAO else not (ids & {"J1", "J2", "J3", "J4", "J5"})
    # The group message exists once per number, not once overall.
    assert conn.execute("SELECT COUNT(*) FROM messages_idx WHERE message_id = 'G1'").fetchone()[0] == 2


def test_results_say_which_number_and_who_holds_it(service):
    response = service.search("cobrança", mode="keyword")
    assert numbers(response) == {MARIA}
    labelled = response["results"][0]["number"]
    assert labelled["alias"] == "Cobrança" and labelled["employee"] == "Bia" and labelled["department"] == "Financeiro"


def test_instance_filter(service):
    assert numbers(service.search("bolo", mode="keyword", instance_jid=MARIA)) == {MARIA}
    assert hit_ids(service.search("bolo", mode="keyword", instance_jid=MARIA)) == {"M1", "G1"}


def test_department_filter_follows_who_held_the_number_at_the_time(service):
    comercial = service.search("bolo", mode="keyword", department_id=COMERCIAL)
    # Comercial only ever operated JOAO, and only until the handover.
    assert hit_ids(comercial) == {"J1", "J2", "J4", "G1"}
    assert numbers(comercial) == {JOAO}

    financeiro = service.search("bolo", mode="keyword", department_id=FINANCEIRO)
    # Financeiro: Bia's number always, and JOAO after Caio took it over.
    assert hit_ids(financeiro) == {"M1", "G1", "J5", "J3"}


def test_employee_filter_only_returns_messages_from_the_period_they_held_the_number(service):
    ana = service.search("bolo", mode="keyword", employee_id=ANA)
    assert hit_ids(ana) == {"J1", "J2", "J4", "G1"}
    caio = service.search("bolo", mode="keyword", employee_id=CAIO)
    assert hit_ids(caio) == {"J5", "J3"}
    # J4 and J5 sit in the same excerpt around the handover: each person only receives their own side.
    assert "J5" not in all_ids(ana) and "J4" not in all_ids(caio)


def test_semantic_search_honours_the_same_windows(service):
    response = service.search("bolo de cenoura no cafe", mode="semantic", employee_id=CAIO)
    assert all_ids(response) <= {"J5", "J3"}


def test_a_restricted_scope_hides_other_numbers_everywhere(service):
    token = access.set_scope(Scope(client_id="vendas", windows=(Window(JOAO),)))
    try:
        response = service.search("bolo", mode="keyword")
        assert numbers(response) == {JOAO}
        assert not (all_ids(response) & {"M1", "M2"})
        assert service.search("cobrança", mode="keyword")["results"] == []

        # Organization filters can only narrow the scope, never widen it.
        widened = service.search("bolo", mode="keyword", department_id=FINANCEIRO)
        assert numbers(widened) <= {JOAO}
        assert hit_ids(widened) == {"J5", "J3"}
        assert service.search("bolo", mode="keyword", instance_jid=MARIA)["results"] == []

        status = service.status()
        assert status["index"]["messages_indexed"] == 6  # JOAO's messages only
        assert "source" not in status  # the global backlog says nothing about this scope
    finally:
        access.reset_scope(token)


def test_a_scope_limited_to_a_period(service):
    handover = int((EPOCH + timedelta(minutes=HANDOVER_MINUTES)).timestamp())
    token = access.set_scope(Scope(windows=(Window(JOAO, handover, None),)))
    try:
        assert hit_ids(service.search("bolo", mode="keyword")) == {"J5", "J3"}
    finally:
        access.reset_scope(token)


def test_a_scope_with_no_numbers_sees_nothing(service):
    token = access.set_scope(Scope(windows=()))
    try:
        assert service.search("bolo", mode="keyword")["results"] == []
        assert service.status()["index"]["messages_indexed"] == 0
    finally:
        access.reset_scope(token)


def test_unscoped_search_still_sees_everything(service):
    assert hit_ids(service.search("bolo", mode="keyword")) >= {"J1", "M1", "G1"}
