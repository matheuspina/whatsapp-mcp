"""Indexer follow-through on privacy actions, schema upgrades, and employee activity summaries."""

import json
import sqlite3
from datetime import datetime, timedelta
from zoneinfo import ZoneInfo

import pytest

from lib import access
from lib.access import Scope, Window
from search import analytics, indexer, source
from search.index_store import SCHEMA_VERSION, IndexStore
from search.search import SearchError
from tests.bridge_schema import create_bridge_schema
from tests.search.conftest import EPOCH, Store, build_index, init_whatsapp_db, ts

JOAO = "5511900000001@s.whatsapp.net"
MARIA = "5511900000002@s.whatsapp.net"
SUBJECT = "5511955550000"
DIRECT = f"{SUBJECT}@s.whatsapp.net"
GROUP = "120363000000000000@g.us"
TZ = "America/Bahia"


def add(conn, instance, chat, msg_id, sender, content, minutes, from_me=0):
    conn.execute("INSERT OR IGNORE INTO chats (jid, name) VALUES (?, ?)", (chat, chat.split("@")[0]))
    conn.execute(
        """INSERT INTO messages (id, chat_jid, sender, sender_name, content, timestamp, is_from_me, instance_jid)
           VALUES (?, ?, ?, ?, ?, ?, ?, ?)""",
        (msg_id, chat, sender, sender, content, ts(minutes), from_me, instance),
    )


@pytest.fixture
def store(tmp_path) -> Store:
    messages_db = str(tmp_path / "messages.db")
    whatsapp_db = str(tmp_path / "whatsapp.db")
    conn = sqlite3.connect(messages_db)
    create_bridge_schema(conn)
    conn.commit()
    conn.close()
    init_whatsapp_db(whatsapp_db)
    return Store(dir=str(tmp_path), messages_db_path=messages_db, whatsapp_db_path=whatsapp_db)


def index_all(store: Store, index_path: str) -> None:
    build_index(store, index_path)


class TestSchemaUpgrade:
    def test_an_index_from_an_older_schema_is_rebuilt(self, tmp_path):
        path = str(tmp_path / "index.db")
        old = sqlite3.connect(path)
        old.execute("CREATE TABLE meta (key TEXT PRIMARY KEY, value TEXT)")
        old.execute("INSERT INTO meta VALUES ('schema_version', '1'), ('source_cursor', '99')")
        old.execute("CREATE TABLE messages_idx (message_id TEXT, chat_jid TEXT)")
        old.commit()
        old.close()

        with IndexStore(path) as index:
            assert index.get_meta("schema_version") == SCHEMA_VERSION
            assert index.get_cursor() == 0  # starts over from messages.db
            cols = [r[1] for r in index._conn.execute("PRAGMA table_info(messages_idx)")]
            assert "instance_jid" in cols

    def test_a_current_index_is_left_alone(self, tmp_path):
        path = str(tmp_path / "index.db")
        with IndexStore(path) as index:
            index.set_cursor(42)
        with IndexStore(path) as index:
            assert index.get_cursor() == 42


class TestPrivacyEvents:
    def bridge_anonymizes(self, store: Store, pseudonym: str = "anon-1234") -> None:
        """What the bridge's AnonymizeSubject does to messages.db, plus its privacy_log entry."""
        conn = sqlite3.connect(store.messages_db_path)
        top = conn.execute("SELECT MAX(rowid) FROM messages").fetchone()[0]
        conn.execute(
            "UPDATE messages SET sender = ?, sender_name = '[anonimizado]', content = '[conteúdo removido]', rowid = rowid + ? "
            "WHERE sender = ? OR chat_jid = ?",
            (pseudonym, top, SUBJECT, DIRECT),
        )
        conn.execute("UPDATE messages SET chat_jid = ? WHERE chat_jid = ?", (f"{pseudonym}@anonymized", DIRECT))
        conn.execute("UPDATE chats SET jid = ? WHERE jid = ?", (f"{pseudonym}@anonymized", DIRECT))
        conn.execute(
            "INSERT INTO privacy_log (actor, action, subject, details) VALUES ('panel:admin', 'anonymize', ?, ?)",
            (pseudonym, json.dumps({"chat_jids": [DIRECT]})),
        )
        conn.commit()
        conn.close()

    def test_an_anonymized_person_disappears_from_the_index(self, store, tmp_path):
        conn = sqlite3.connect(store.messages_db_path)
        add(conn, JOAO, DIRECT, "D1", SUBJECT, "meu CPF é 123456", 0)
        add(conn, JOAO, DIRECT, "D2", JOAO, "anotado, obrigado", 1, from_me=1)
        add(conn, JOAO, GROUP, "G1", SUBJECT, "concordo com o orçamento", 2)
        add(conn, JOAO, GROUP, "G2", "5511911111111", "eu também", 3)
        conn.commit()
        conn.close()
        index_path = str(tmp_path / "index.db")
        index_all(store, index_path)

        idx = IndexStore(index_path)
        assert idx.search_fts('text : "CPF"*')
        idx.close()

        self.bridge_anonymizes(store)
        index_all(store, index_path)  # the indexer's next pass

        idx = IndexStore(index_path)
        try:
            assert not idx._conn.execute("SELECT 1 FROM messages_idx WHERE chat_jid = ?", (DIRECT,)).fetchone()
            assert not idx._conn.execute("SELECT 1 FROM chunks WHERE chat_jid = ?", (DIRECT,)).fetchone()
            assert not idx.search_fts('text : "CPF"*'), "the anonymized text must not stay searchable"
            assert not idx.search_fts('text : "concordo"*')
            # Other people's messages in the group are untouched, and the scrubbed rows are indexed.
            assert idx.search_fts('text : "também"*')
            assert (
                idx._conn.execute(
                    "SELECT COUNT(*) FROM messages_idx WHERE chat_jid = 'anon-1234@anonymized'"
                ).fetchone()[0]
                == 2
            )
            # Every chunk that outlived the purge still has its messages.
            assert idx.unchunked_ranges() == []
            assert idx.get_meta("privacy_cursor") == "1"
        finally:
            idx.close()

    def test_a_purge_removes_old_messages_and_rebuilds_the_chunks_that_straddled_the_cutoff(self, store, tmp_path):
        conn = sqlite3.connect(store.messages_db_path)
        add(conn, JOAO, GROUP, "OLD1", "a", "conversa antiga sobre pizza", 0)
        add(conn, JOAO, GROUP, "OLD2", "b", "ainda a conversa antiga", 5)
        add(conn, JOAO, GROUP, "NEW1", "a", "conversa recente sobre bolo", 10)  # same chunk as OLD1/OLD2
        add(conn, JOAO, GROUP, "NEW2", "b", "outra recente", 3 * 24 * 60)
        conn.commit()
        conn.close()
        index_path = str(tmp_path / "index.db")
        index_all(store, index_path)

        cutoff = (EPOCH + timedelta(minutes=8)).isoformat()
        conn = sqlite3.connect(store.messages_db_path)
        conn.execute("DELETE FROM messages WHERE timestamp < ?", (ts(8),))
        conn.execute(
            "INSERT INTO privacy_log (actor, action, subject, details) VALUES ('retention-policy', 'purge', 'retention', ?)",
            (json.dumps({"cutoff": cutoff}),),
        )
        conn.commit()
        conn.close()
        index_all(store, index_path)

        idx = IndexStore(index_path)
        try:
            ids = {r[0] for r in idx._conn.execute("SELECT message_id FROM messages_idx")}
            assert ids == {"NEW1", "NEW2"}
            assert not idx.search_fts('text : "pizza"*')
            assert idx.search_fts('text : "bolo"*')
            assert idx.unchunked_ranges() == []
            # The kept message did not lose its chunk.
            texts = " ".join(r[0] for r in idx._conn.execute("SELECT text FROM chunks"))
            assert "pizza" not in texts and "bolo" in texts
        finally:
            idx.close()

    def test_events_are_applied_once(self, store, tmp_path):
        conn = sqlite3.connect(store.messages_db_path)
        add(conn, JOAO, DIRECT, "D1", SUBJECT, "oi", 0)
        conn.commit()
        conn.close()
        index_path = str(tmp_path / "index.db")
        index_all(store, index_path)
        self.bridge_anonymizes(store)
        with IndexStore(index_path) as idx:
            assert indexer.apply_privacy_events(idx, store.messages_db_path) == 1
            assert indexer.apply_privacy_events(idx, store.messages_db_path) == 0

    def test_an_older_bridge_without_a_privacy_log_is_fine(self, tmp_path):
        path = str(tmp_path / "old.db")
        sqlite3.connect(path).close()
        assert source.fetch_privacy_events(path, 0) == []


class TestActivitySummary:
    @pytest.fixture
    def scenario(self, store, tmp_path):
        """Ana holds JOAO's number from the start; Caio takes it on the 4th day."""
        conn = sqlite3.connect(store.messages_db_path)
        handover = ts(3 * 24 * 60)
        for stmt in [
            "INSERT INTO departments (id, name) VALUES (1, 'Comercial')",
            "INSERT INTO employees (id, department_id, name, role) VALUES (1, 1, 'Ana', 'Vendedora'), (2, 1, 'Caio', 'Vendedor')",
            f"INSERT INTO instances (id, phone_jid, employee_id, alias) VALUES (1, '{JOAO}', 2, 'Vendas')",
            f"INSERT INTO instance_assignments (instance_id, employee_id, valid_from, valid_to) VALUES (1, 1, '1970-01-01 00:00:00+00:00', '{handover}')",
            f"INSERT INTO instance_assignments (instance_id, employee_id, valid_from) VALUES (1, 2, '{handover}')",
        ]:
            conn.execute(stmt)
        c1, c2 = "5511911110001@s.whatsapp.net", "5511911110002@s.whatsapp.net"
        # Customer 1: answered after 10 and after 30 minutes.
        add(conn, JOAO, c1, "A1", "c1", "preciso de orçamento", 0)
        add(conn, JOAO, c1, "A2", JOAO, "claro, segue", 10, from_me=1)
        add(conn, JOAO, c1, "A3", "c1", "fechado", 60)
        add(conn, JOAO, c1, "A4", JOAO, "ótimo", 90, from_me=1)
        # Customer 2: never answered.
        add(conn, JOAO, c2, "B1", "c2", "alguém aí?", 120)
        # One message the customer took back.
        add(conn, JOAO, c1, "A5", "c1", "esqueça", 130)
        conn.execute("UPDATE messages SET is_deleted_remote = 1 WHERE id = 'A5'")
        # After the handover: Caio's period.
        add(conn, JOAO, c1, "C1", "c1", "de novo", 3 * 24 * 60 + 5)
        add(conn, JOAO, c1, "C2", JOAO, "olá", 3 * 24 * 60 + 8, from_me=1)
        conn.commit()
        conn.close()
        index_path = str(tmp_path / "index.db")
        index_all(store, index_path)
        return store, index_path

    def summary(self, scenario, employee_id, period):
        store, index_path = scenario
        return analytics.employee_activity_summary(
            employee_id, period, index_db_path=index_path, messages_db_path=store.messages_db_path, display_tz=TZ
        )

    def test_counts_only_the_period_the_person_held_the_number(self, scenario):
        ana = self.summary(scenario, 1, "2026-03-01..2026-03-31")
        assert ana["employee"]["name"] == "Ana" and ana["employee"]["department"]["name"] == "Comercial"
        assert ana["totals"] == {
            "messages_received": 4,
            "messages_sent": 2,
            "conversations": 2,
            "active_days": 1,
            "deleted_by_sender": 1,
        }
        caio = self.summary(scenario, 2, "2026-03-01..2026-03-31")
        assert caio["totals"]["messages_received"] == 1 and caio["totals"]["messages_sent"] == 1

    def test_customer_facing_metrics(self, scenario):
        ana = self.summary(scenario, 1, "2026-03-01..2026-03-31")
        r = ana["responsiveness"]
        assert r["replies_measured"] == 2
        assert r["avg_first_response_minutes"] == 20.0  # 10 and 30 minutes
        assert r["median_first_response_minutes"] == 20.0
        assert r["conversations_waiting_for_reply"] == 1  # customer 2
        top = ana["top_conversations"][0]
        assert top["chat_jid"].startswith("5511911110001") and top["received"] == 3 and top["sent"] == 2
        assert ana["by_day"] == [{"day": "2026-03-02", "received": 4, "sent": 2}]
        assert ana["numbers"] == [{"instance_jid": JOAO, "alias": "Vendas"}]

    def test_a_person_without_a_number_in_the_period(self, scenario):
        caio_before = self.summary(scenario, 2, "2026-03-02..2026-03-02")
        assert caio_before["totals"]["messages_received"] == 0 and "note" in caio_before

    def test_the_access_scope_narrows_the_summary(self, scenario):
        after = int((EPOCH + timedelta(days=3)).timestamp())
        token = access.set_scope(Scope(windows=(Window(JOAO, after, None),)))
        try:
            ana = self.summary(scenario, 1, "2026-03-01..2026-03-31")
            assert ana["totals"]["messages_received"] == 0  # Ana's period is outside the scope
            caio = self.summary(scenario, 2, "2026-03-01..2026-03-31")
            assert caio["totals"]["messages_received"] == 1
        finally:
            access.reset_scope(token)

    def test_unknown_employee(self, scenario):
        with pytest.raises(SearchError, match="No employee"):
            self.summary(scenario, 999, "7d")

    def test_missing_index(self, store, tmp_path):
        conn = sqlite3.connect(store.messages_db_path)
        conn.execute("INSERT INTO employees (id, name) VALUES (1, 'Ana')")
        conn.execute(f"INSERT INTO instances (id, phone_jid, employee_id) VALUES (1, '{JOAO}', 1)")
        conn.execute(
            "INSERT INTO instance_assignments (instance_id, employee_id, valid_from) VALUES (1, 1, '1970-01-01 00:00:00+00:00')"
        )
        conn.commit()
        conn.close()
        with pytest.raises(SearchError, match="index is not available"):
            analytics.employee_activity_summary(
                1,
                "2026-03-01..2026-03-31",
                index_db_path=str(tmp_path / "nope.db"),
                messages_db_path=store.messages_db_path,
            )


class TestPeriods:
    NOW = datetime(2026, 3, 11, 15, 30, tzinfo=ZoneInfo(TZ))  # a Wednesday

    def parse(self, text):
        start, end, label = analytics.parse_period(text, ZoneInfo(TZ), self.NOW)
        return start.date().isoformat(), end.date().isoformat(), label

    def test_named_periods(self):
        assert self.parse("today")[:2] == ("2026-03-11", "2026-03-12")
        assert self.parse("yesterday")[:2] == ("2026-03-10", "2026-03-11")
        assert self.parse("this_week")[:2] == ("2026-03-09", "2026-03-16")
        assert self.parse("last_week")[:2] == ("2026-03-02", "2026-03-09")
        assert self.parse("this_month")[:2] == ("2026-03-01", "2026-04-01")
        assert self.parse("last_month")[:2] == ("2026-02-01", "2026-03-01")

    def test_last_n_days_include_today(self):
        assert self.parse("7d")[:2] == ("2026-03-05", "2026-03-12")

    def test_explicit_range_is_inclusive(self):
        assert self.parse("2026-03-01..2026-03-03")[:2] == ("2026-03-01", "2026-03-04")

    @pytest.mark.parametrize("bad", ["soon", "0d", "2026-03-05..2026-03-01", "x..y", ""])
    def test_bad_periods_explain_what_is_accepted(self, bad):
        with pytest.raises(SearchError):
            analytics.parse_period(bad, ZoneInfo(TZ), self.NOW)
