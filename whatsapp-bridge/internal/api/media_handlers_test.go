package api

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	waLog "go.mau.fi/whatsmeow/util/log"

	"whatsapp-bridge/internal/database"
	"whatsapp-bridge/internal/mediastore"
)

// fakeBucket is an in-memory object store.
type fakeBucket struct {
	mu       sync.Mutex
	objects  map[string][]byte
	checkErr error
}

func (f *fakeBucket) Bucket() string { return "media" }
func (f *fakeBucket) Put(_ context.Context, key, _ string, file *os.File, _ int64) error {
	data, err := io.ReadAll(file)
	f.mu.Lock()
	f.objects[key] = data
	f.mu.Unlock()
	return err
}
func (f *fakeBucket) Get(_ context.Context, key string) (io.ReadCloser, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	data, ok := f.objects[key]
	if !ok {
		return nil, errors.New("not found")
	}
	return io.NopCloser(strings.NewReader(string(data))), nil
}
func (f *fakeBucket) Presign(_ context.Context, key string, _ time.Duration) (string, error) {
	return "https://signed.example/" + key, nil
}
func (f *fakeBucket) Check(context.Context) error { return f.checkErr }

func newMediaServer(t *testing.T) (*Server, *fakeBucket) {
	t.Helper()
	t.Setenv("S3_BUCKET", "")
	s := newTestServer(t)
	bucket := &fakeBucket{objects: map[string][]byte{}}
	m := mediastore.NewManager(s.messageStore, waLog.Noop, mediastore.Options{
		APIKey:   "test-key",
		NewStore: func(mediastore.Config) (mediastore.ObjectStore, error) { return bucket, nil },
	})
	if err := m.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Stop)
	s.SetMediaStore(m)
	return s, bucket
}

func storageBody() map[string]any {
	return map[string]any{
		"enabled": true, "endpoint": "https://acct.r2.cloudflarestorage.com", "region": "auto", "bucket": "media",
		"access_key_id": "AKIA", "secret_access_key": "s3cr3t-value", "prefix": "whatsapp",
	}
}

func TestMediaStorageSettingsNeverReturnTheSecret(t *testing.T) {
	s, _ := newMediaServer(t)

	rec := call(t, s, s.handleMediaStorage, http.MethodGet, "/api/settings/media-storage", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET = %d: %s", rec.Code, rec.Body)
	}
	if data := decode(t, rec)["data"].(map[string]any); data["active"] != false || data["source"] != "none" {
		t.Errorf("fresh install should have storage off: %v", data)
	}

	rec = call(t, s, s.handleMediaStorage, http.MethodPut, "/api/settings/media-storage", storageBody())
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT = %d: %s", rec.Code, rec.Body)
	}
	if strings.Contains(rec.Body.String(), "s3cr3t-value") {
		t.Fatal("the response leaked the secret")
	}
	data := decode(t, rec)["data"].(map[string]any)
	cfg := data["config"].(map[string]any)
	if data["active"] != true || data["source"] != "panel" || cfg["secret_set"] != true || cfg["bucket"] != "media" || cfg["prefix"] != "whatsapp/" {
		t.Errorf("status after save: %v", data)
	}

	rec = call(t, s, s.handleMediaStorage, http.MethodGet, "/api/settings/media-storage", nil)
	if strings.Contains(rec.Body.String(), "s3cr3t-value") {
		t.Fatal("GET leaked the secret")
	}
}

func TestMediaStorageSaveValidatesAndTests(t *testing.T) {
	s, bucket := newMediaServer(t)

	bad := storageBody()
	bad["bucket"] = "Not A Bucket"
	if rec := call(t, s, s.handleMediaStorage, http.MethodPut, "/api/settings/media-storage", bad); rec.Code != http.StatusBadRequest {
		t.Errorf("invalid bucket should be 400, got %d: %s", rec.Code, rec.Body)
	}

	bucket.checkErr = errors.New("access denied: the token needs write access to this bucket")
	rec := call(t, s, s.handleMediaStorage, http.MethodPut, "/api/settings/media-storage", storageBody())
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "access denied") {
		t.Errorf("a failing connection should be reported: %d %s", rec.Code, rec.Body)
	}
	if s.mediaStore.Enabled() {
		t.Error("nothing should be enabled after a failed test")
	}

	// The test endpoint reports the same without saving.
	if rec := call(t, s, s.handleMediaStorageTest, http.MethodPost, "/api/settings/media-storage/test", storageBody()); rec.Code != http.StatusBadRequest {
		t.Errorf("test endpoint should fail too: %d", rec.Code)
	}
	bucket.checkErr = nil
	if rec := call(t, s, s.handleMediaStorageTest, http.MethodPost, "/api/settings/media-storage/test", storageBody()); rec.Code != http.StatusOK {
		t.Errorf("test endpoint = %d: %s", rec.Code, rec.Body)
	}
	if s.mediaStore.Enabled() {
		t.Error("testing must not enable storage")
	}
	if rec := call(t, s, s.handleMediaStorage, http.MethodDelete, "/api/settings/media-storage", nil); rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("DELETE should be 405, got %d", rec.Code)
	}
}

func TestMediaStorageRoutesRequireAuthentication(t *testing.T) {
	s, _ := newMediaServer(t)
	rec := call(t, s, s.handleMediaStorage, http.MethodGet, "/api/settings/media-storage", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("authenticated GET = %d", rec.Code)
	}
	// Same handler, no key: the middleware refuses before the handler runs.
	req, _ := http.NewRequest(http.MethodGet, "/api/settings/media-storage", nil)
	out := httptest.NewRecorder()
	s.SecureMiddleware(s.handleMediaStorage)(out, req)
	if out.Code != http.StatusUnauthorized {
		t.Errorf("unauthenticated GET = %d, want 401", out.Code)
	}
}

func TestMediaStorageUnavailableWithoutManager(t *testing.T) {
	s := newTestServer(t)
	if rec := call(t, s, s.handleMediaStorage, http.MethodGet, "/api/settings/media-storage", nil); rec.Code != http.StatusServiceUnavailable {
		t.Errorf("GET without a manager = %d, want 503", rec.Code)
	}
}

func recordUploaded(t *testing.T, s *Server, bucket *fakeBucket, id, content string) *database.MediaRecord {
	t.Helper()
	rec := database.MediaRecord{
		InstanceJID: "5511@s.whatsapp.net", ChatJID: "a@s.whatsapp.net", MessageID: id, MediaType: "image", Filename: "x.jpg",
		ContentType: "image/jpeg", MessageTime: time.Now(), Status: database.MediaLocal, // no local path: the upload queue leaves it alone
	}
	if err := s.messageStore.RecordMedia(rec); err != nil {
		t.Fatal(err)
	}
	key := "whatsapp/d/e/n/c/2026-09/" + id + ".jpg"
	bucket.objects[key] = []byte(content)
	got, _ := s.messageStore.GetMediaRecord(rec.InstanceJID, rec.ChatJID, id)
	if err := s.messageStore.MarkMediaUploaded(got, "media", key, false); err != nil {
		t.Fatal(err)
	}
	got, _ = s.messageStore.GetMediaRecord(rec.InstanceJID, rec.ChatJID, id)
	return got
}

func TestMediaURL(t *testing.T) {
	s, bucket := newMediaServer(t)
	if _, err := s.mediaStore.Save(t.Context(), mediastore.Config{Enabled: true, Bucket: "media", AccessKeyID: "a", SecretAccessKey: "b"}); err != nil {
		t.Fatal(err)
	}
	rec := recordUploaded(t, s, bucket, "M1", "pixels")

	res := call(t, s, s.handleMediaURL, http.MethodGet, "/api/media/url?chat_jid="+rec.ChatJID+"&message_id=M1&ttl=120", nil)
	if res.Code != http.StatusOK {
		t.Fatalf("GET = %d: %s", res.Code, res.Body)
	}
	body := decode(t, res)
	if body["url"] != "https://signed.example/"+rec.ObjectKey || body["expires_in"] != float64(120) || body["content_type"] != "image/jpeg" {
		t.Errorf("response = %v", body)
	}

	// The lifetime of a link is capped.
	res = call(t, s, s.handleMediaURL, http.MethodGet, "/api/media/url?chat_jid="+rec.ChatJID+"&message_id=M1&ttl=99999999", nil)
	if got := decode(t, res)["expires_in"]; got != float64(maxLinkTTL.Seconds()) {
		t.Errorf("ttl not capped: %v", got)
	}

	for name, target := range map[string]string{
		"missing params": "/api/media/url",
		"bad ttl":        "/api/media/url?chat_jid=a&message_id=M1&ttl=abc",
	} {
		if res := call(t, s, s.handleMediaURL, http.MethodGet, target, nil); res.Code != http.StatusBadRequest {
			t.Errorf("%s = %d, want 400", name, res.Code)
		}
	}
	if res := call(t, s, s.handleMediaURL, http.MethodGet, "/api/media/url?chat_jid=a@s.whatsapp.net&message_id=nope", nil); res.Code != http.StatusNotFound {
		t.Errorf("unknown message = %d, want 404", res.Code)
	}

	// Media still waiting for its upload has no link yet.
	pending := database.MediaRecord{InstanceJID: rec.InstanceJID, ChatJID: rec.ChatJID, MessageID: "M2", MediaType: "image",
		Status: database.MediaPendingUpload, LocalPath: "x", MessageTime: time.Now()}
	_ = s.messageStore.RecordMedia(pending)
	if res := call(t, s, s.handleMediaURL, http.MethodGet, "/api/media/url?chat_jid="+rec.ChatJID+"&message_id=M2", nil); res.Code != http.StatusConflict {
		t.Errorf("not uploaded yet = %d, want 409", res.Code)
	}
}

func TestDownloadServesStoredMediaWithoutTheCDN(t *testing.T) {
	s, bucket := newMediaServer(t)

	// A file still on disk is served as is, with no bucket involved.
	local := filepath.Join(t.TempDir(), "local.jpg")
	_ = os.WriteFile(local, []byte("local"), 0o644)
	other := database.MediaRecord{InstanceJID: "5511@s.whatsapp.net", ChatJID: "a@s.whatsapp.net", MessageID: "M3", MediaType: "image",
		Status: database.MediaLocal, LocalPath: local, MessageTime: time.Now()}
	if err := s.messageStore.RecordMedia(other); err != nil {
		t.Fatal(err)
	}
	res := call(t, s, s.handleDownload, http.MethodPost, "/api/download", map[string]string{"chat_jid": other.ChatJID, "message_id": "M3"})
	if got := decode(t, res)["path"]; res.Code != http.StatusOK || got != local {
		t.Fatalf("local file not served: %d %v", res.Code, got)
	}

	// Now with storage on: the local copy is gone (deleted after upload), so the file comes back
	// from the bucket, at the path the MCP server has always been given.
	if _, err := s.mediaStore.Save(t.Context(), mediastore.Config{Enabled: true, Bucket: "media", AccessKeyID: "a", SecretAccessKey: "b"}); err != nil {
		t.Fatal(err)
	}
	rec := recordUploaded(t, s, bucket, "M1", "from the bucket")
	res = call(t, s, s.handleDownload, http.MethodPost, "/api/download", map[string]string{"chat_jid": rec.ChatJID, "message_id": "M1"})
	if res.Code != http.StatusOK {
		t.Fatalf("download = %d: %s", res.Code, res.Body)
	}
	body := decode(t, res)
	path, _ := body["path"].(string)
	if body["success"] != true || !strings.HasPrefix(path, filepath.Join("store", "media")) || !strings.HasSuffix(path, ".jpg") {
		t.Fatalf("response = %v", body)
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != "from the bucket" {
		t.Errorf("file content = %q, %v", data, err)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Error("temporary file left behind")
	}
}
