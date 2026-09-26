package database

import (
	"testing"
	"time"

	"whatsapp-bridge/internal/types"
)

const (
	joaoJID  = "5511900000001@s.whatsapp.net"
	mariaJID = "5511900000002@s.whatsapp.net"
	groupJID = "120363000000000000@g.us"
)

func mustInstance(t *testing.T, store *MessageStore, jid string) {
	t.Helper()
	if _, err := store.RegisterInstanceJID(jid, true); err != nil {
		t.Fatalf("RegisterInstanceJID: %v", err)
	}
}

func TestSameGroupMessageIsStoredOncePerInstance(t *testing.T) {
	store := newTestStore(t)
	now := time.Now()
	putMessage(t, store, joaoJID, groupJID, "MSG1", "cliente", "bom dia", now)
	putMessage(t, store, mariaJID, groupJID, "MSG1", "cliente", "bom dia", now)

	if got := countRows(t, store, "SELECT COUNT(*) FROM messages WHERE id = 'MSG1'"); got != 2 {
		t.Fatalf("expected one row per instance, got %d", got)
	}
	// Readers that do not care about the instance see the message once.
	msgs, err := store.GetMessages(groupJID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 {
		t.Errorf("GetMessages should de-duplicate across instances, got %d", len(msgs))
	}
	if got := countRows(t, store, "SELECT COUNT(*) FROM chat_instances WHERE chat_jid = ?", groupJID); got != 2 {
		t.Errorf("chat should list both instances, got %d", got)
	}
}

func TestUpsertKeepsAuditState(t *testing.T) {
	store := newTestStore(t)
	now := time.Now()
	putMessage(t, store, joaoJID, "cliente@s.whatsapp.net", "M1", "cliente", "preço original", now)

	if found, err := store.RecordMessageEdit(joaoJID, "cliente@s.whatsapp.net", "M1", "preço editado", now.Add(time.Minute)); err != nil || !found {
		t.Fatalf("RecordMessageEdit: found=%v err=%v", found, err)
	}
	// History sync delivers the original again: the edit must survive.
	putMessage(t, store, joaoJID, "cliente@s.whatsapp.net", "M1", "cliente", "preço original", now)

	var content string
	var edited bool
	if err := store.db.QueryRow("SELECT content, is_edited FROM messages WHERE id = 'M1'").Scan(&content, &edited); err != nil {
		t.Fatal(err)
	}
	if content != "preço editado" || !edited {
		t.Errorf("edit lost after re-sync: content=%q edited=%v", content, edited)
	}

	versions, err := store.GetMessageVersions(joaoJID, "cliente@s.whatsapp.net", "M1")
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 1 || versions[0].Content != "preço original" || versions[0].Reason != "edit" {
		t.Errorf("unexpected versions: %+v", versions)
	}
}

func TestRevokeKeepsTextAndMovesRowID(t *testing.T) {
	store := newTestStore(t)
	now := time.Now()
	chat := "cliente@s.whatsapp.net"
	putMessage(t, store, joaoJID, chat, "M1", "cliente", "desconto de 30%", now)
	putMessage(t, store, joaoJID, chat, "M2", "cliente", "outra", now.Add(time.Second))

	var before int64
	_ = store.db.QueryRow("SELECT rowid FROM messages WHERE id = 'M1'").Scan(&before)

	found, err := store.MarkMessageRemoteDeleted(joaoJID, "M1", chat, "cliente", now.Add(time.Minute))
	if err != nil || !found {
		t.Fatalf("MarkMessageRemoteDeleted: found=%v err=%v", found, err)
	}

	var content, by string
	var deleted bool
	var after int64
	if err := store.db.QueryRow("SELECT content, is_deleted_remote, deleted_by, rowid FROM messages WHERE id = 'M1'").Scan(&content, &deleted, &by, &after); err != nil {
		t.Fatal(err)
	}
	if !deleted || content != "desconto de 30%" || by != "cliente" {
		t.Errorf("revoked message should keep its text: content=%q deleted=%v by=%q", content, deleted, by)
	}
	if after <= before {
		t.Errorf("rowid must move forward so the indexer re-reads the row: %d -> %d", before, after)
	}

	// Revoking again must not add another version.
	if _, err := store.MarkMessageRemoteDeleted(joaoJID, "M1", chat, "cliente", now.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	versions, _ := store.GetMessageVersions(joaoJID, chat, "M1")
	if len(versions) != 1 || versions[0].Reason != "delete" {
		t.Errorf("unexpected versions: %+v", versions)
	}

	// An unknown message reports found=false.
	if found, _ := store.MarkMessageRemoteDeleted(joaoJID, "NOPE", chat, "x", now); found {
		t.Error("unknown message should not be found")
	}
}

func TestFeedAttributesMessagesThroughAssignmentHistory(t *testing.T) {
	store := newTestStore(t)
	comercial, _ := store.CreateDepartment("Comercial", "")
	financeiro, _ := store.CreateDepartment("Financeiro", "")
	ana, _ := store.CreateEmployee(&comercial.ID, "Ana", "Vendedora", "")
	bia, _ := store.CreateEmployee(&financeiro.ID, "Bia", "Analista", "")

	inst, err := store.CreatePendingInstance("Vendas", &ana.ID, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CompleteInstancePairing(inst.ID, joaoJID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdateInstance(inst.ID, InstanceUpdate{SetEmployee: true, EmployeeID: &bia.ID}); err != nil {
		t.Fatal(err)
	}

	// Ana held the number until 5h ago, Bia since then.
	now := time.Now().UTC()
	mustExec(t, store, "UPDATE instance_assignments SET valid_from = ?, valid_to = ? WHERE employee_id = ?", now.Add(-10*time.Hour), now.Add(-5*time.Hour), ana.ID)
	mustExec(t, store, "UPDATE instance_assignments SET valid_from = ? WHERE employee_id = ?", now.Add(-5*time.Hour), bia.ID)

	chat := "cliente@s.whatsapp.net"
	putMessage(t, store, joaoJID, chat, "OLD", "cliente", "pedido antigo", now.Add(-8*time.Hour))
	putMessage(t, store, joaoJID, chat, "NEW", "cliente", "boleto vencido", now.Add(-2*time.Hour))

	byDept := func(id int) []string {
		msgs, err := store.ListMessageFeed(FeedFilter{DepartmentID: &id})
		if err != nil {
			t.Fatal(err)
		}
		var ids []string
		for _, m := range msgs {
			ids = append(ids, m.ID)
		}
		return ids
	}
	if got := byDept(comercial.ID); len(got) != 1 || got[0] != "OLD" {
		t.Errorf("Comercial should own only the message from Ana's period, got %v", got)
	}
	if got := byDept(financeiro.ID); len(got) != 1 || got[0] != "NEW" {
		t.Errorf("Financeiro should own only the message from Bia's period, got %v", got)
	}

	all, _ := store.ListMessageFeed(FeedFilter{Query: "boleto"})
	if len(all) != 1 || all[0].EmployeeName != "Bia" || all[0].DepartmentName != "Financeiro" || all[0].InstanceAlias != "Vendas" {
		t.Errorf("feed row should carry the organizational context: %+v", all)
	}
}

func mustExec(t *testing.T, store *MessageStore, query string, args ...any) {
	t.Helper()
	if _, err := store.db.Exec(query, args...); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
}

func TestFeedDeletedOnlyAndPagination(t *testing.T) {
	store := newTestStore(t)
	now := time.Now().UTC()
	chat := "c@s.whatsapp.net"
	for i := 0; i < 5; i++ {
		putMessage(t, store, joaoJID, chat, string(rune('A'+i)), "c", "msg", now.Add(time.Duration(i)*time.Minute))
	}
	_, _ = store.MarkMessageRemoteDeleted(joaoJID, "C", chat, "c", now)

	deleted, _ := store.ListMessageFeed(FeedFilter{DeletedOnly: true})
	if len(deleted) != 1 || deleted[0].ID != "C" || !deleted[0].IsDeletedRemote {
		t.Errorf("deleted-only feed wrong: %+v", deleted)
	}

	page1, _ := store.ListMessageFeed(FeedFilter{Limit: 2})
	if len(page1) != 2 || page1[0].ID != "E" {
		t.Fatalf("first page wrong: %+v", page1)
	}
	page2, _ := store.ListMessageFeed(FeedFilter{Limit: 2, Before: &page1[1].Timestamp})
	if len(page2) != 2 || page2[0].ID != "C" {
		t.Errorf("second page wrong: %+v", page2)
	}
}

func TestPendingInstanceLifecycle(t *testing.T) {
	store := newTestStore(t)

	pending, err := store.CreatePendingInstance("Novo", nil, false, &CorporateConfirmation{TermsVersion: "v1", ConfirmedBy: "panel:admin"})
	if err != nil {
		t.Fatal(err)
	}
	if pending.Status != InstanceStatusPairing || pending.PhoneJID != "" || !pending.CorporateAssetConfirmed || pending.CorporateConfirmedBy != "panel:admin" || pending.AllowSend {
		t.Fatalf("unexpected pending instance: %+v", pending)
	}

	paired, err := store.CompleteInstancePairing(pending.ID, joaoJID)
	if err != nil {
		t.Fatal(err)
	}
	if paired.ID != pending.ID || paired.PhoneJID != joaoJID || paired.Status != InstanceStatusConnected || paired.PhoneNumber != "5511900000001" {
		t.Errorf("unexpected paired instance: %+v", paired)
	}

	// The same number paired again folds into the existing row.
	again, _ := store.CreatePendingInstance("Segunda vez", nil, false, nil)
	merged, err := store.CompleteInstancePairing(again.ID, joaoJID)
	if err != nil {
		t.Fatal(err)
	}
	if merged.ID != paired.ID {
		t.Errorf("re-pairing should reuse the row %d, got %d", paired.ID, merged.ID)
	}
	if got := countRows(t, store, "SELECT COUNT(*) FROM instances"); got != 1 {
		t.Errorf("expected 1 instance, got %d", got)
	}

	// Stale pending rows are purged at startup.
	stale, _ := store.CreatePendingInstance("Nunca escaneado", nil, false, nil)
	if err := store.PurgeStalePendingInstances(); err != nil {
		t.Fatal(err)
	}
	if got, _ := store.GetInstance(stale.ID); got != nil {
		t.Error("stale pending instance should be gone")
	}
	if got, _ := store.GetInstance(paired.ID); got == nil {
		t.Error("paired instance must survive the purge")
	}

	// Removing keeps the row (history stays attributable) but hides it from the default list.
	if err := store.MarkInstanceRemoved(paired.ID); err != nil {
		t.Fatal(err)
	}
	if list, _ := store.ListInstances(false); len(list) != 0 {
		t.Errorf("removed instance should be hidden, got %d", len(list))
	}
	if list, _ := store.ListInstances(true); len(list) != 1 {
		t.Errorf("removed instance should be listed on request, got %d", len(list))
	}
}

func TestEmployeeChangeClosesAssignment(t *testing.T) {
	store := newTestStore(t)
	a, _ := store.CreateEmployee(nil, "Ana", "", "")
	b, _ := store.CreateEmployee(nil, "Bia", "", "")
	inst, _ := store.CreatePendingInstance("N", &a.ID, false, nil)

	if _, err := store.UpdateInstance(inst.ID, InstanceUpdate{SetEmployee: true, EmployeeID: &b.ID}); err != nil {
		t.Fatal(err)
	}
	// Setting the same owner again is a no-op.
	if _, err := store.UpdateInstance(inst.ID, InstanceUpdate{SetEmployee: true, EmployeeID: &b.ID}); err != nil {
		t.Fatal(err)
	}
	if got := countRows(t, store, "SELECT COUNT(*) FROM instance_assignments WHERE instance_id = ?", inst.ID); got != 2 {
		t.Errorf("expected 2 assignments (Ana closed, Bia open), got %d", got)
	}
	if got := countRows(t, store, "SELECT COUNT(*) FROM instance_assignments WHERE instance_id = ? AND valid_to IS NULL", inst.ID); got != 1 {
		t.Errorf("expected exactly 1 open assignment, got %d", got)
	}

	// Unlinking closes the open one and opens nothing.
	if _, err := store.UpdateInstance(inst.ID, InstanceUpdate{SetEmployee: true, EmployeeID: nil}); err != nil {
		t.Fatal(err)
	}
	if got := countRows(t, store, "SELECT COUNT(*) FROM instance_assignments WHERE instance_id = ? AND valid_to IS NULL", inst.ID); got != 0 {
		t.Errorf("expected no open assignment after unlinking, got %d", got)
	}
	if _, err := store.UpdateInstance(9999, InstanceUpdate{}); err != ErrNotFound {
		t.Errorf("updating a missing instance should return ErrNotFound, got %v", err)
	}
}

func TestBackfillLegacyInstance(t *testing.T) {
	store := newTestStore(t)
	now := time.Now()
	chat := "cliente@s.whatsapp.net"
	// A message captured before instances existed.
	if err := store.StoreChat(chat, "Cliente", now); err != nil {
		t.Fatal(err)
	}
	if err := store.StoreMessage("OLD", chat, "cliente", "cliente", "antigo", now, false, "", "", "", "", nil, nil, nil, 0); err != nil {
		t.Fatal(err)
	}
	putMessage(t, store, mariaJID, chat, "NEW", "cliente", "novo", now)

	var oldRowID, maxBefore int64
	_ = store.db.QueryRow("SELECT rowid FROM messages WHERE id = 'OLD'").Scan(&oldRowID)
	_ = store.db.QueryRow("SELECT MAX(rowid) FROM messages").Scan(&maxBefore)

	if n, _ := store.CountUnattributedMessages(); n != 1 {
		t.Fatalf("expected 1 unattributed message, got %d", n)
	}
	moved, err := store.BackfillLegacyInstance(joaoJID)
	if err != nil || moved != 1 {
		t.Fatalf("BackfillLegacyInstance: moved=%d err=%v", moved, err)
	}

	var inst string
	var rowID int64
	_ = store.db.QueryRow("SELECT instance_jid, rowid FROM messages WHERE id = 'OLD'").Scan(&inst, &rowID)
	if inst != joaoJID {
		t.Errorf("legacy message should belong to %s, got %q", joaoJID, inst)
	}
	if rowID <= maxBefore {
		t.Errorf("rowid must pass the previous maximum so the indexer re-reads it: %d <= %d", rowID, maxBefore)
	}
	if n, _ := store.CountUnattributedMessages(); n != 0 {
		t.Errorf("no unattributed messages should remain, got %d", n)
	}
}

func TestAnonymizeSubject(t *testing.T) {
	store := newTestStore(t)
	now := time.Now()
	direct := "5511977770000@s.whatsapp.net"
	putMessage(t, store, joaoJID, direct, "D1", "5511977770000", "meu CPF é 123", now)
	putMessage(t, store, joaoJID, groupJID, "G1", "5511977770000", "concordo", now)
	putMessage(t, store, joaoJID, groupJID, "G2", "outra pessoa", "ok", now)
	mustExec(t, store, "INSERT INTO contact_nicknames (jid, nickname) VALUES (?, 'Fulano')", direct)
	mustExec(t, store, "INSERT INTO message_versions (instance_jid, chat_jid, message_id, content, reason) VALUES (?, ?, 'D1', 'antigo', 'edit')", joaoJID, direct)

	res, err := store.AnonymizeSubject("5511977770000", "panel:admin")
	if err != nil {
		t.Fatal(err)
	}
	if res.Messages != 2 || len(res.ChatJIDs) != 1 || res.ChatJIDs[0] != direct {
		t.Fatalf("unexpected result: %+v", res)
	}

	var content, sender string
	_ = store.db.QueryRow("SELECT content, sender FROM messages WHERE id = 'G1'").Scan(&content, &sender)
	if content == "concordo" || sender != res.Pseudonym {
		t.Errorf("group message of the subject should be scrubbed: %q / %q", content, sender)
	}
	_ = store.db.QueryRow("SELECT content FROM messages WHERE id = 'G2'").Scan(&content)
	if content != "ok" {
		t.Errorf("other people's group messages must be untouched, got %q", content)
	}
	if got := countRows(t, store, "SELECT COUNT(*) FROM messages WHERE chat_jid = ?", direct); got != 0 {
		t.Errorf("the direct chat identifier (a phone number) should be replaced, %d rows remain", got)
	}
	if got := countRows(t, store, "SELECT COUNT(*) FROM messages WHERE chat_jid = ? AND content LIKE '%removido%'", res.Pseudonym+"@anonymized"); got != 1 {
		t.Errorf("direct chat should live under the pseudonym, got %d", got)
	}
	if got := countRows(t, store, "SELECT COUNT(*) FROM contact_nicknames"); got != 0 {
		t.Error("nickname should be deleted")
	}
	if got := countRows(t, store, "SELECT COUNT(*) FROM message_versions"); got != 0 {
		t.Error("versions of the subject's messages should be deleted")
	}
	if got := countRows(t, store, "SELECT COUNT(*) FROM privacy_log WHERE action = 'anonymize'"); got != 1 {
		t.Errorf("anonymization should be logged, got %d", got)
	}
}

func TestPurgeOlderThan(t *testing.T) {
	store := newTestStore(t)
	now := time.Now().UTC()
	chat := "c@s.whatsapp.net"
	putMessage(t, store, joaoJID, chat, "OLD", "c", "velha", now.AddDate(0, 0, -40))
	putMessage(t, store, joaoJID, chat, "NEW", "c", "recente", now.AddDate(0, 0, -1))

	removed, err := store.PurgeOlderThan(now.AddDate(0, 0, -30), "retention-policy")
	if err != nil || removed != 1 {
		t.Fatalf("PurgeOlderThan: removed=%d err=%v", removed, err)
	}
	if got := countRows(t, store, "SELECT COUNT(*) FROM messages"); got != 1 {
		t.Errorf("expected only the recent message to remain, got %d", got)
	}
	if got := countRows(t, store, "SELECT COUNT(*) FROM privacy_log WHERE action = 'purge'"); got != 1 {
		t.Errorf("purge should be logged, got %d", got)
	}
}

func TestCaptureGate(t *testing.T) {
	store := newTestStore(t)
	mustInstance(t, store, joaoJID)

	if !store.CaptureAllowed(joaoJID) {
		t.Error("capture must be allowed when confirmation is not required")
	}
	store.SetRequireCorporateConfirmation(true)
	if store.CaptureAllowed(joaoJID) {
		t.Error("capture must stop for an unconfirmed number when confirmation is required")
	}
	inst, _ := store.GetInstanceByPhone(joaoJID)
	if _, err := store.UpdateInstance(inst.ID, InstanceUpdate{Confirm: &CorporateConfirmation{TermsVersion: "v1", ConfirmedBy: "panel:admin"}}); err != nil {
		t.Fatal(err)
	}
	if !store.CaptureAllowed(joaoJID) {
		t.Error("capture must resume once the number is confirmed")
	}
}

func TestAccessLogRoundTrip(t *testing.T) {
	store := newTestStore(t)
	n := 3
	if err := store.InsertAccessLog(types.AccessLogEntry{Actor: "mcp:claude", Action: "search", ResultCount: &n}); err != nil {
		t.Fatal(err)
	}
	entries, err := store.ListAccessLog(10)
	if err != nil || len(entries) != 1 || entries[0].Action != "search" || entries[0].ResultCount == nil || *entries[0].ResultCount != 3 {
		t.Errorf("unexpected entries: %+v err=%v", entries, err)
	}
}
