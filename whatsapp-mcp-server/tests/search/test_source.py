"""Tests for search/source.py: read-only DB access and name resolution."""

from search import source
from tests.search.conftest import (
    ANA_JID,
    ANA_NAME,
    BARE_SENDER,
    BRUNO_LID_JID,
    BRUNO_NAME,
    CARLA_LID_JID,
    CARLA_NAME,
    GROUP_JID,
    GROUP_NAME,
    insert_message,
    ts,
)


class TestDisplayText:
    def test_returns_content_when_present(self):
        assert source.display_text("oi", None, None) == "oi"

    def test_caption_is_kept_even_with_media(self):
        assert source.display_text("olha isso", "image", None) == "olha isso"

    def test_captionless_image_gets_a_marker(self):
        assert source.display_text("", "image", None) == "[imagem]"

    def test_captionless_audio_gets_a_marker(self):
        assert source.display_text("", "audio", None) == "[áudio]"

    def test_captionless_video_gets_a_marker(self):
        assert source.display_text("", "video", None) == "[vídeo]"

    def test_captionless_document_includes_filename(self):
        assert source.display_text("", "document", "contrato.pdf") == "[documento: contrato.pdf]"

    def test_no_content_and_no_media_is_empty(self):
        assert source.display_text("", None, None) == ""


class TestShouldIndexChat:
    def test_group_chat_is_indexed(self):
        assert source.should_index_chat("123@g.us") is True

    def test_individual_chat_is_indexed(self):
        assert source.should_index_chat("123@s.whatsapp.net") is True

    def test_newsletter_is_skipped(self):
        assert source.should_index_chat("123@newsletter") is False

    def test_broadcast_is_skipped(self):
        assert source.should_index_chat("status@broadcast") is False


class TestParseTimestamp:
    def test_parses_bridge_format(self):
        assert source.parse_timestamp("2026-03-02 09:00:00+00:00") == 1772442000


class TestFetchMessagesAfter:
    def test_returns_rows_ordered_by_rowid(self, synthetic_store):
        rows = source.fetch_messages_after(synthetic_store.messages_db_path, 0)
        assert [r.rowid for r in rows] == sorted(r.rowid for r in rows)
        assert len(rows) == 10

    def test_cursor_excludes_already_seen_rows(self, synthetic_store):
        first_batch = source.fetch_messages_after(synthetic_store.messages_db_path, 0, limit=3)
        cursor = first_batch[-1].rowid

        next_batch = source.fetch_messages_after(synthetic_store.messages_db_path, cursor)

        assert all(r.rowid > cursor for r in next_batch)
        assert len(first_batch) + len(next_batch) == 10

    def test_never_writes_to_the_database(self, synthetic_store):
        # A read-only (mode=ro) connection raises on any write attempt.
        import sqlite3

        conn = sqlite3.connect(f"file:{synthetic_store.messages_db_path}?mode=ro", uri=True)
        try:
            import pytest

            with pytest.raises(sqlite3.OperationalError):
                conn.execute("DELETE FROM messages")
        finally:
            conn.close()


class TestCountMessagesAfter:
    def test_counts_remaining_rows(self, synthetic_store):
        assert source.count_messages_after(synthetic_store.messages_db_path, 0) == 10
        first = source.fetch_messages_after(synthetic_store.messages_db_path, 0, limit=1)
        assert source.count_messages_after(synthetic_store.messages_db_path, first[0].rowid) == 9


class TestGetChatNames:
    def test_looks_up_multiple_chats(self, synthetic_store):
        names = source.get_chat_names(synthetic_store.messages_db_path, {GROUP_JID, ANA_JID})
        assert names[GROUP_JID] == GROUP_NAME
        assert names[ANA_JID] == ANA_JID.split("@")[0]  # stored name is the bare number

    def test_empty_set_returns_empty_dict(self, synthetic_store):
        assert source.get_chat_names(synthetic_store.messages_db_path, set()) == {}


class TestContactResolverSenderNames:
    def test_from_me_uses_owner_label(self, synthetic_store):
        with source.ContactResolver(synthetic_store.whatsapp_db_path, owner_label="Eu") as resolver:
            assert resolver.resolve_sender_name("whatever@s.whatsapp.net", is_from_me=True) == "Eu"

    def test_direct_contact_match(self, synthetic_store):
        with source.ContactResolver(synthetic_store.whatsapp_db_path) as resolver:
            assert resolver.resolve_sender_name(ANA_JID, is_from_me=False) == ANA_NAME

    def test_lid_resolved_directly_from_contacts(self, synthetic_store):
        with source.ContactResolver(synthetic_store.whatsapp_db_path) as resolver:
            assert resolver.resolve_sender_name(CARLA_LID_JID, is_from_me=False) == CARLA_NAME

    def test_lid_resolved_via_lid_map_fallback(self, synthetic_store):
        with source.ContactResolver(synthetic_store.whatsapp_db_path) as resolver:
            assert resolver.resolve_sender_name(BRUNO_LID_JID, is_from_me=False) == BRUNO_NAME

    def test_unknown_jid_falls_back_to_user_part(self, synthetic_store):
        with source.ContactResolver(synthetic_store.whatsapp_db_path) as resolver:
            assert resolver.resolve_sender_name("999999999@s.whatsapp.net", is_from_me=False) == "999999999"

    def test_bare_sender_with_no_at_sign_falls_back_to_itself(self, synthetic_store):
        with source.ContactResolver(synthetic_store.whatsapp_db_path) as resolver:
            assert resolver.resolve_sender_name(BARE_SENDER, is_from_me=False) == BARE_SENDER

    def test_results_are_cached_across_calls(self, synthetic_store):
        with source.ContactResolver(synthetic_store.whatsapp_db_path) as resolver:
            resolver.resolve_sender_name(ANA_JID, is_from_me=False)
            assert ANA_JID in resolver._contact_cache
            # Second call must not hit the DB again; cache short-circuits it.
            assert resolver.resolve_sender_name(ANA_JID, is_from_me=False) == ANA_NAME


class TestContactResolverChatNames:
    def test_group_chat_uses_stored_name(self, synthetic_store):
        with source.ContactResolver(synthetic_store.whatsapp_db_path) as resolver:
            assert resolver.resolve_chat_name(GROUP_JID, GROUP_NAME) == GROUP_NAME

    def test_bare_number_chat_name_falls_back_to_contact_lookup(self, synthetic_store):
        with source.ContactResolver(synthetic_store.whatsapp_db_path) as resolver:
            bare_number = ANA_JID.split("@")[0]
            assert resolver.resolve_chat_name(ANA_JID, bare_number) == ANA_NAME

    def test_non_numeric_stored_name_is_kept_as_is(self, synthetic_store):
        with source.ContactResolver(synthetic_store.whatsapp_db_path) as resolver:
            assert resolver.resolve_chat_name(ANA_JID, "Apelido Custom") == "Apelido Custom"

    def test_missing_stored_name_falls_back_to_contact_lookup(self, synthetic_store):
        with source.ContactResolver(synthetic_store.whatsapp_db_path) as resolver:
            assert resolver.resolve_chat_name(ANA_JID, None) == ANA_NAME


class TestUtcFormatting:
    def test_date_str(self):
        assert source.utc_date_str(1772442000) == "02/03/2026"

    def test_time_str(self):
        assert source.utc_time_str(1772442000) == "09:00"


class TestReplacedRowGetsHigherRowid:
    def test_replace_bumps_rowid(self, synthetic_store):
        original = source.fetch_messages_after(synthetic_store.messages_db_path, 0)
        original_row = next(r for r in original if r.message_id == "MSG0001")

        insert_message(
            synthetic_store.messages_db_path,
            message_id="MSG0001",
            chat_jid=GROUP_JID,
            sender=ANA_JID,
            content="bom dia pessoal (editado)",
            timestamp=ts(0),
        )

        updated = source.fetch_messages_after(synthetic_store.messages_db_path, 0)
        updated_row = next(r for r in updated if r.message_id == "MSG0001")

        assert updated_row.rowid > original_row.rowid
        assert updated_row.content == "bom dia pessoal (editado)"
