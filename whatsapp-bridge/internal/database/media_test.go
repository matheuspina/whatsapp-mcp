package database

import (
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestSlug(t *testing.T) {
	for in, want := range map[string]string{
		"João da Silva":          "joao-da-silva",
		"  Vendas / Comercial  ": "vendas-comercial",
		"Ação & Reação!!":        "acao-reacao",
		"日本語":                    "",
		"":                       "",
		"../../etc/passwd":       "etc-passwd",
		strings.Repeat("a", 100): strings.Repeat("a", maxSlugLen),
		"Família 👨‍👩‍👧 Pina":     "familia-pina",
		"--Already--Dashed--":    "already-dashed",
	} {
		if got := Slug(in); got != want {
			t.Errorf("Slug(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestChatFolder(t *testing.T) {
	for _, tc := range []struct{ jid, name, want string }{
		{"5511988887777@s.whatsapp.net", "João Silva", "joao-silva__5511988887777"},
		{"5511988887777@s.whatsapp.net", "", "5511988887777"},
		{"5511988887777@s.whatsapp.net", "5511988887777", "5511988887777"},
		{"120363025@g.us", "Família", "familia__120363025-g"},
		{"1234@lid", "Maria", "maria__1234-lid"},
		{"status@broadcast", "", "status-bc"},
	} {
		if got := chatFolder(tc.jid, tc.name); got != tc.want {
			t.Errorf("chatFolder(%q, %q) = %q, want %q", tc.jid, tc.name, got, tc.want)
		}
	}
}

func TestMediaObjectKey(t *testing.T) {
	ts := time.Date(2026, 9, 26, 10, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name string
		rec  MediaRecord
		want string
	}{
		{"image", MediaRecord{MessageID: "3EB028A5", MediaType: "image", Filename: "image_20260926_100000.jpg", MessageTime: ts}, "p/2026-09/3EB028A5.jpg"},
		{"audio default ext", MediaRecord{MessageID: "AB12", MediaType: "audio", MessageTime: ts}, "p/2026-09/AB12.ogg"},
		{"document keeps its name", MediaRecord{MessageID: "AB12", MediaType: "document", Filename: "Relatório Final (v2).pdf", MessageTime: ts}, "p/2026-09/AB12_Relatorio_Final__v2_.pdf"},
		{"hostile ids stay inside the folder", MediaRecord{MessageID: "../../x", MediaType: "image", Filename: "a.jpg", MessageTime: ts}, "p/2026-09/x.jpg"},
		{"no date", MediaRecord{MessageID: "AB12", MediaType: "video"}, "p/sem-data/AB12.mp4"},
	} {
		if got := MediaObjectKey("p/", &tc.rec); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
		if strings.Contains(MediaObjectKey("p/", &tc.rec), "..") {
			t.Errorf("%s: key contains '..'", tc.name)
		}
	}
}

func TestMediaContentType(t *testing.T) {
	for _, tc := range []struct{ kind, file, want string }{
		{"image", "a.JPG", "image/jpeg"},
		{"audio", "voice.ogg", "audio/ogg"},
		{"video", "", "video/mp4"},
		{"document", "x.pdf", "application/pdf"},
		{"document", "x.unknownext", "application/octet-stream"},
	} {
		if got := MediaContentType(tc.kind, tc.file); got != tc.want {
			t.Errorf("MediaContentType(%q, %q) = %q, want %q", tc.kind, tc.file, got, tc.want)
		}
	}
}

func sampleMedia(id string) MediaRecord {
	return MediaRecord{
		InstanceJID: "5511@s.whatsapp.net", ChatJID: "a@s.whatsapp.net", MessageID: id, MediaType: "image",
		Filename: "image.jpg", ContentType: "image/jpeg", MessageTime: time.Date(2026, 9, 26, 10, 0, 0, 0, time.UTC),
		Size: 10, Status: MediaPendingUpload, LocalPath: "store/media/a/" + id + ".jpg",
	}
}

func TestMediaQueueLifecycle(t *testing.T) {
	store := newTestStore(t)
	rec := sampleMedia("M1")
	if err := store.RecordMedia(rec); err != nil {
		t.Fatal(err)
	}
	if !store.MediaRegistered(rec.InstanceJID, rec.ChatJID, "M1") {
		t.Error("recorded media should count as registered")
	}

	// Claiming marks the row so a second claim finds nothing.
	claimed, err := store.ClaimPendingUploads(10)
	if err != nil || len(claimed) != 1 || claimed[0].MessageID != "M1" {
		t.Fatalf("claim: %v %v", claimed, err)
	}
	if again, _ := store.ClaimPendingUploads(10); len(again) != 0 {
		t.Errorf("a claimed row must not be handed out twice, got %d", len(again))
	}
	got, _ := store.GetMediaRecord(rec.InstanceJID, rec.ChatJID, "M1")
	if got.Status != MediaUploading {
		t.Errorf("status = %q, want uploading", got.Status)
	}

	// A crash leaves it uploading; the next start requeues it.
	if n, err := store.ResetStuckUploads(); err != nil || n != 1 {
		t.Fatalf("ResetStuckUploads = %d, %v", n, err)
	}

	// A failed attempt with a retry time in the future is not claimable yet.
	claimed, _ = store.ClaimPendingUploads(10)
	future := time.Now().Add(time.Hour)
	if err := store.MarkMediaUploadFailed(claimed[0], "boom", 1, &future); err != nil {
		t.Fatal(err)
	}
	if due, _ := store.ClaimPendingUploads(10); len(due) != 0 {
		t.Error("a row waiting for its retry time must not be claimed")
	}
	got, _ = store.GetMediaRecord(rec.InstanceJID, rec.ChatJID, "M1")
	if got.Status != MediaPendingUpload || got.Attempts != 1 || got.LastError != "boom" {
		t.Errorf("after failed attempt: %+v", got)
	}

	// A retry time in the past makes it due again.
	past := time.Now().Add(-time.Minute)
	if err := store.MarkMediaUploadFailed(got, "boom", 2, &past); err != nil {
		t.Fatal(err)
	}
	due, _ := store.ClaimPendingUploads(10)
	if len(due) != 1 {
		t.Fatalf("due retry should be claimed, got %d", len(due))
	}

	// Uploaded without keeping the local copy clears the path; the row then resolves through the bucket.
	if err := store.MarkMediaUploaded(due[0], "bkt", "a/b/M1.jpg", false); err != nil {
		t.Fatal(err)
	}
	got, _ = store.GetMediaRecord(rec.InstanceJID, rec.ChatJID, "M1")
	if got.Status != MediaUploaded || got.ObjectKey != "a/b/M1.jpg" || got.Bucket != "bkt" || got.LocalPath != "" || got.UploadedAt == nil {
		t.Errorf("after upload: %+v", got)
	}

	// The same message arriving again must not push it back through the queue.
	if err := store.RecordMedia(sampleMedia("M1")); err != nil {
		t.Fatal(err)
	}
	got, _ = store.GetMediaRecord(rec.InstanceJID, rec.ChatJID, "M1")
	if got.Status != MediaUploaded {
		t.Errorf("re-recording an uploaded file changed its status to %q", got.Status)
	}

	// Lookup without the instance prefers the uploaded copy.
	found, err := store.FindMediaRecord(rec.ChatJID, "M1")
	if err != nil || found.ObjectKey == "" {
		t.Errorf("FindMediaRecord: %+v %v", found, err)
	}
	if _, err := store.FindMediaRecord(rec.ChatJID, "nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing media should be ErrNotFound, got %v", err)
	}
}

func TestQueueLocalMediaAndFailures(t *testing.T) {
	store := newTestStore(t)
	local := sampleMedia("L1")
	local.Status = MediaLocal
	noFile := sampleMedia("L2")
	noFile.Status, noFile.LocalPath = MediaLocal, ""
	for _, r := range []MediaRecord{local, noFile} {
		if err := store.RecordMedia(r); err != nil {
			t.Fatal(err)
		}
	}
	// A failed download is recorded, is not "registered" (it is retried on the next delivery) and
	// is never queued for upload since there is no file.
	failed := sampleMedia("F1")
	if err := store.RecordMediaFailure(failed, "download: 410"); err != nil {
		t.Fatal(err)
	}
	if store.MediaRegistered(failed.InstanceJID, failed.ChatJID, "F1") {
		t.Error("a failed download must not count as registered")
	}

	n, err := store.QueueLocalMedia(10, true)
	if err != nil || n != 1 {
		t.Fatalf("QueueLocalMedia = %d, %v (only the row with a local file qualifies)", n, err)
	}
	stats, _ := store.MediaQueueStats()
	if stats[MediaPendingUpload] != 1 || stats[MediaLocal] != 1 || stats[MediaFailed] != 1 {
		t.Errorf("stats = %v", stats)
	}
}

func TestMediaFolderPrefixUsesOwnerAtMessageTime(t *testing.T) {
	store := newTestStore(t)
	inst := "5511999990000@s.whatsapp.net"
	chat := "5511988887777@s.whatsapp.net"

	dept, _ := store.CreateDepartment("Comercial", "")
	ana, _ := store.CreateEmployee(&dept.ID, "Ana Souza", "", "")
	bia, _ := store.CreateEmployee(&dept.ID, "Bia Lima", "", "")
	pending, err := store.CreatePendingInstance("Vendas", &ana.ID, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CompleteInstancePairing(pending.ID, inst); err != nil {
		t.Fatal(err)
	}
	if err := store.StoreChatWithInstance(chat, "João Cliente", time.Now(), inst); err != nil {
		t.Fatal(err)
	}

	// Ana held the number when the message was exchanged.
	msgTime := time.Now().UTC()
	prefix, err := store.MediaFolderPrefix(inst, chat, msgTime)
	if err != nil {
		t.Fatal(err)
	}
	want := "comercial-" + strconv.Itoa(dept.ID) + "/ana-souza-" + strconv.Itoa(ana.ID) + "/5511999990000/joao-cliente__5511988887777/"
	if prefix != want {
		t.Errorf("prefix = %q, want %q", prefix, want)
	}

	// The number changes hands and the contact is renamed: the folder stays put.
	if _, err := store.UpdateInstance(pending.ID, InstanceUpdate{EmployeeID: &bia.ID, SetEmployee: true}); err != nil {
		t.Fatal(err)
	}
	if err := store.StoreChatWithInstance(chat, "João Renomeado", time.Now(), inst); err != nil {
		t.Fatal(err)
	}
	again, err := store.MediaFolderPrefix(inst, chat, time.Now().UTC().Add(time.Minute))
	if err != nil || again != prefix {
		t.Errorf("folder must be frozen: %q vs %q (%v)", again, prefix, err)
	}

	// A different chat of the same number, after the hand-over, belongs to Bia.
	other, err := store.MediaFolderPrefix(inst, "5511977776666@s.whatsapp.net", time.Now().UTC().Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(other, "/bia-lima-"+strconv.Itoa(bia.ID)+"/") {
		t.Errorf("new conversations after the hand-over go to the new owner: %q", other)
	}

	// Unknown number: still a valid, browsable path.
	orphan, err := store.MediaFolderPrefix("", "x@g.us", msgTime)
	if err != nil || !strings.HasPrefix(orphan, "sem-setor/sem-responsavel/sem-numero/") {
		t.Errorf("orphan prefix = %q (%v)", orphan, err)
	}
}

func TestSettingsRoundTrip(t *testing.T) {
	store := newTestStore(t)
	if _, err := store.GetSetting("k"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing setting: %v", err)
	}
	for _, v := range []string{"one", "two"} {
		if err := store.SetSetting("k", v); err != nil {
			t.Fatal(err)
		}
	}
	if got, _ := store.GetSetting("k"); got != "two" {
		t.Errorf("got %q", got)
	}
}
