package whatsapp

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	waLog "go.mau.fi/whatsmeow/util/log"

	"whatsapp-bridge/internal/database"
)

func TestPathSegmentKeepsUntrustedTextInsideOneFolder(t *testing.T) {
	for in, want := range map[string]string{
		"5511@s.whatsapp.net": "5511_s.whatsapp.net",
		"../../etc/passwd":    "_.._etc_passwd",
		"3EB028A580CF7CC9AA":  "3EB028A580CF7CC9AA",
		".hidden":             "hidden",
		"":                    "_",
		"a/b\\c":              "a_b_c",
	} {
		got := pathSegment(in)
		if got != want {
			t.Errorf("pathSegment(%q) = %q, want %q", in, got, want)
		}
		if strings.ContainsAny(got, `/\`) {
			t.Errorf("pathSegment(%q) = %q contains a separator", in, got)
		}
	}
}

func TestLocalMediaPathIsStablePerMessage(t *testing.T) {
	a := localMediaPath("5511@s.whatsapp.net", "3EB0", "image", "image_20260926_100000.jpg")
	// Same message, different arrival time in the generated name: same path.
	b := localMediaPath("5511@s.whatsapp.net", "3EB0", "image", "image_20260926_100503.jpg")
	if a != b {
		t.Errorf("path depends on the arrival time: %q vs %q", a, b)
	}
	if want := filepath.Join("store", "media", "5511_s.whatsapp.net", "3EB0.jpg"); a != want {
		t.Errorf("path = %q, want %q", a, want)
	}
	if got := localMediaPath("g@g.us", "AB", "audio", ""); !strings.HasSuffix(got, "AB.ogg") {
		t.Errorf("audio default extension: %q", got)
	}
	if got := localMediaPath("g@g.us", "AB", "document", "Relatório.PDF"); !strings.HasSuffix(got, "AB.pdf") {
		t.Errorf("document keeps its extension: %q", got)
	}
	if got := localMediaPath("g@g.us", "AB", "document", "noext"); !strings.HasSuffix(got, "AB.bin") {
		t.Errorf("document without extension: %q", got)
	}
}

func TestExtractDirectPath(t *testing.T) {
	for in, want := range map[string]string{
		"https://mmg.whatsapp.net/v/t62/abc?x=1": "/v/t62/abc?x=1",
		"https://cdn.example/path/file":          "/path/file",
		"/already/a/path":                        "/already/a/path",
	} {
		if got := extractDirectPath(in); got != want {
			t.Errorf("extractDirectPath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestWhatsmeowMediaType(t *testing.T) {
	for _, kind := range []string{"image", "video", "audio", "document", "IMAGE"} {
		if _, err := whatsmeowMediaType(kind); err != nil {
			t.Errorf("%s: %v", kind, err)
		}
	}
	// An unknown kind is an error, not silently decrypted as an image.
	if _, err := whatsmeowMediaType("sticker"); err == nil {
		t.Error("unknown media kinds must be rejected")
	}
}

type recordingSink struct {
	mu         sync.Mutex
	registered bool
	media      []database.MediaRecord
	failures   []string
}

func (s *recordingSink) Registered(_, _, _ string) bool { return s.registered }
func (s *recordingSink) HandleMedia(rec database.MediaRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.media = append(s.media, rec)
}
func (s *recordingSink) HandleMediaFailure(_ database.MediaRecord, reason string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failures = append(s.failures, reason)
}

func withSink(t *testing.T, sink MediaSink) {
	t.Helper()
	prev := mediaSink
	mediaSink = sink
	t.Cleanup(func() { mediaSink = prev })
	t.Chdir(t.TempDir())
}

func testJob() mediaJob {
	return mediaJob{
		instanceJID: "5511@s.whatsapp.net", chatJID: "a@s.whatsapp.net", messageID: "M1",
		timestamp: time.Date(2026, 9, 26, 10, 0, 0, 0, time.UTC), mediaType: "image", filename: "image.jpg",
		directPath: "/v/t62/abc", mediaKey: []byte("k"),
	}
}

func TestAutoDownloadHandsAnExistingFileToTheSink(t *testing.T) {
	sink := &recordingSink{}
	withSink(t, sink)
	job := testJob()
	path := localMediaPath(job.chatJID, job.messageID, job.mediaType, job.filename)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("already here"), 0o644); err != nil {
		t.Fatal(err)
	}

	(&Client{logger: waLog.Noop}).autoDownloadMedia(job)

	if len(sink.media) != 1 || sink.media[0].LocalPath != path || sink.media[0].Size != int64(len("already here")) {
		t.Fatalf("sink got %+v", sink.media)
	}
	if rec := sink.media[0]; rec.InstanceJID != job.instanceJID || rec.MessageID != "M1" || rec.ContentType != "image/jpeg" || !rec.MessageTime.Equal(job.timestamp) {
		t.Errorf("record fields not carried over: %+v", rec)
	}
}

func TestAutoDownloadSkipsMediaAlreadyRegistered(t *testing.T) {
	sink := &recordingSink{registered: true}
	withSink(t, sink)
	(&Client{logger: waLog.Noop}).autoDownloadMedia(testJob())
	if len(sink.media) != 0 || len(sink.failures) != 0 {
		t.Errorf("a registered message must not be downloaded again: %+v", sink)
	}
	if _, err := os.Stat("store"); !os.IsNotExist(err) {
		t.Error("nothing should touch the disk for a registered message")
	}
}

func TestAutoDownloadFailureIsReportedAndLeavesNoPartialFile(t *testing.T) {
	sink := &recordingSink{}
	withSink(t, sink)
	// No WhatsApp connection behind the client: the download fails.
	(&Client{logger: waLog.Noop}).autoDownloadMedia(testJob())

	if len(sink.failures) != 1 || !strings.Contains(sink.failures[0], "download") {
		t.Fatalf("failure not reported: %+v", sink)
	}
	if len(sink.media) != 0 {
		t.Error("a failed download must not be registered as media")
	}
	entries, _ := os.ReadDir(filepath.Join("store", "media", "a_s.whatsapp.net"))
	if len(entries) != 0 {
		t.Errorf("partial file left behind: %v", entries)
	}
}

func TestAutoDownloadRejectsUnknownMediaKinds(t *testing.T) {
	sink := &recordingSink{}
	withSink(t, sink)
	job := testJob()
	job.mediaType = "hologram"
	(&Client{logger: waLog.Noop}).autoDownloadMedia(job)
	if len(sink.failures) != 1 {
		t.Errorf("unknown kind should be reported as a failure: %+v", sink)
	}
}

func TestDownloadsWaitForASlot(t *testing.T) {
	sink := &recordingSink{}
	withSink(t, sink)
	prev := mediaDownloadSlots
	mediaDownloadSlots = make(chan struct{}, 1)
	t.Cleanup(func() { mediaDownloadSlots = prev })

	mediaDownloadSlots <- struct{}{} // the only slot is taken
	done := make(chan struct{})
	go func() {
		(&Client{logger: waLog.Noop}).autoDownloadMedia(testJob())
		close(done)
	}()
	select {
	case <-done:
		t.Fatal("download ran without a free slot")
	case <-time.After(150 * time.Millisecond):
	}
	<-mediaDownloadSlots // free it
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("download never got the slot")
	}
}
