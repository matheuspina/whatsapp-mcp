package whatsapp

import (
	"testing"
	"time"

	"go.mau.fi/whatsmeow/proto/waCommon"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"

	"whatsapp-bridge/internal/database"
)

func textEvent(chat, sender, id, text string) *events.Message {
	chatJID := types.JID{User: chat, Server: types.DefaultUserServer}
	return &events.Message{
		Info: types.MessageInfo{
			MessageSource: types.MessageSource{Chat: chatJID, Sender: types.JID{User: sender, Server: types.DefaultUserServer}},
			ID:            id,
			Timestamp:     time.Now(),
			PushName:      "Cliente",
		},
		Message: &waE2E.Message{Conversation: proto.String(text)},
	}
}

func protocolEvent(chat, sender string, pm *waE2E.ProtocolMessage) *events.Message {
	ev := textEvent(chat, sender, "PROTO-"+time.Now().Format("150405.000000"), "")
	ev.Message = &waE2E.Message{ProtocolMessage: pm}
	return ev
}

func TestHandleMessageTagsTheInstanceAndTracksRevokeAndEdit(t *testing.T) {
	mgr, store := newManagerEnv(t, testJIDA)
	c, _ := mgr.ClientByJID(testJIDA)
	instance := testJIDA + "@s.whatsapp.net"
	chat := "5511977770000"
	chatJID := chat + "@s.whatsapp.net"

	c.HandleMessage(store, nil, textEvent(chat, chat, "M1", "o preço é 100"))
	c.HandleMessage(store, nil, textEvent(chat, chat, "M2", "fechado"))

	msgs, err := store.ListMessageFeed(database.FeedFilter{InstanceJID: instance})
	if err != nil || len(msgs) != 2 {
		t.Fatalf("both messages should be attributed to %s: %d (err=%v)", instance, len(msgs), err)
	}

	// The customer edits M1.
	c.HandleMessage(store, nil, protocolEvent(chat, chat, &waE2E.ProtocolMessage{
		Type:          waE2E.ProtocolMessage_MESSAGE_EDIT.Enum(),
		Key:           &waCommon.MessageKey{ID: proto.String("M1")},
		EditedMessage: &waE2E.Message{Conversation: proto.String("o preço é 90")},
	}))
	versions, _ := store.GetMessageVersions(instance, chatJID, "M1")
	if len(versions) != 1 || versions[0].Content != "o preço é 100" {
		t.Errorf("the pre-edit text should be kept as a version: %+v", versions)
	}

	// ...and revokes M2.
	c.HandleMessage(store, nil, protocolEvent(chat, chat, &waE2E.ProtocolMessage{
		Type: waE2E.ProtocolMessage_REVOKE.Enum(),
		Key:  &waCommon.MessageKey{ID: proto.String("M2")},
	}))

	feed, _ := store.ListMessageFeed(database.FeedFilter{InstanceJID: instance})
	byID := map[string]bool{}
	edited := map[string]string{}
	for _, m := range feed {
		byID[m.ID] = m.IsDeletedRemote
		edited[m.ID] = m.Content
	}
	if len(feed) != 2 {
		t.Fatalf("protocol messages must not become messages of their own: %d rows", len(feed))
	}
	if !byID["M2"] || byID["M1"] {
		t.Errorf("only M2 should be flagged deleted: %v", byID)
	}
	if edited["M2"] != "fechado" {
		t.Errorf("the revoked text must be kept, got %q", edited["M2"])
	}
	if edited["M1"] != "o preço é 90" {
		t.Errorf("the message should show its latest text, got %q", edited["M1"])
	}
}

func TestHandleMessageSkipsUnconfirmedNumbersWhenRequired(t *testing.T) {
	mgr, store := newManagerEnv(t, testJIDA)
	c, _ := mgr.ClientByJID(testJIDA)
	store.SetRequireCorporateConfirmation(true)

	c.HandleMessage(store, nil, textEvent("5511977770000", "5511977770000", "M1", "oi"))
	if n, _ := store.GetMessageCount(); n != 0 {
		t.Fatalf("nothing should be captured before the number is confirmed, got %d", n)
	}

	inst, _ := store.GetInstanceByPhone(testJIDA + "@s.whatsapp.net")
	if _, err := store.UpdateInstance(inst.ID, database.InstanceUpdate{Confirm: &database.CorporateConfirmation{TermsVersion: "v1", ConfirmedBy: "panel:admin"}}); err != nil {
		t.Fatal(err)
	}
	c.HandleMessage(store, nil, textEvent("5511977770000", "5511977770000", "M2", "oi de novo"))
	if n, _ := store.GetMessageCount(); n != 1 {
		t.Errorf("capture should resume after confirmation, got %d", n)
	}
}
