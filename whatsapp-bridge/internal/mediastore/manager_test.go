package mediastore

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/minio/minio-go/v7"
	waLog "go.mau.fi/whatsmeow/util/log"

	"whatsapp-bridge/internal/database"
)

// fakeStore is an in-memory ObjectStore.
type fakeStore struct {
	mu      sync.Mutex
	objects map[string][]byte
	types   map[string]string
	bucket  string

	putErr      error         // returned by Put while set
	checkErr    error         // returned by Check while set
	gate        chan struct{} // when non-nil, Put blocks until it is closed
	inflight    atomic.Int32
	maxInflight atomic.Int32
}

func newFakeStore(bucket string) *fakeStore {
	return &fakeStore{objects: map[string][]byte{}, types: map[string]string{}, bucket: bucket}
}

func (f *fakeStore) Bucket() string { return f.bucket }

func (f *fakeStore) Put(ctx context.Context, key, contentType string, file *os.File, size int64) error {
	n := f.inflight.Add(1)
	defer f.inflight.Add(-1)
	for {
		max := f.maxInflight.Load()
		if n <= max || f.maxInflight.CompareAndSwap(max, n) {
			break
		}
	}
	if f.gate != nil {
		select {
		case <-f.gate:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	f.mu.Lock()
	err := f.putErr
	f.mu.Unlock()
	if err != nil {
		return err
	}
	data, err := io.ReadAll(file)
	if err != nil {
		return err
	}
	f.mu.Lock()
	f.objects[key], f.types[key] = data, contentType
	f.mu.Unlock()
	return nil
}

func (f *fakeStore) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	data, ok := f.objects[key]
	if !ok {
		return nil, errors.New("not found")
	}
	return io.NopCloser(strings.NewReader(string(data))), nil
}

func (f *fakeStore) Presign(ctx context.Context, key string, ttl time.Duration) (string, error) {
	return fmt.Sprintf("https://signed.example/%s/%s?ttl=%d", f.bucket, key, int(ttl.Seconds())), nil
}

func (f *fakeStore) Check(ctx context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.checkErr
}

func (f *fakeStore) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.objects)
}

// newTestManager opens a real database in a temp dir and a Manager that talks to fake.
func newTestManager(t *testing.T, workers int, fake *fakeStore) (*Manager, *database.MessageStore) {
	t.Helper()
	t.Chdir(t.TempDir())
	t.Setenv("S3_BUCKET", "") // never inherit the developer's environment
	db, err := database.NewMessageStore()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	m := NewManager(db, waLog.Noop, Options{APIKey: "test-api-key", UploadWorkers: workers})
	m.newStore = func(Config) (ObjectStore, error) { return fake, nil }
	return m, db
}

func validConfig() Config {
	return Config{Enabled: true, Endpoint: "https://acct.r2.cloudflarestorage.com", Region: "auto", Bucket: "media",
		AccessKeyID: "AKIA", SecretAccessKey: "s3cr3t", Prefix: "whatsapp"}
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// writeLocal creates a downloaded file and returns its media record.
func writeLocal(t *testing.T, id, content string) database.MediaRecord {
	t.Helper()
	path := filepath.Join(t.TempDir(), id+".jpg")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return database.MediaRecord{
		InstanceJID: "5511@s.whatsapp.net", ChatJID: "a@s.whatsapp.net", MessageID: id, MediaType: "image",
		Filename: "image.jpg", MessageTime: time.Date(2026, 9, 26, 10, 0, 0, 0, time.UTC), Size: int64(len(content)), LocalPath: path,
	}
}

func statusOf(t *testing.T, db *database.MessageStore, id string) *database.MediaRecord {
	t.Helper()
	rec, err := db.GetMediaRecord("5511@s.whatsapp.net", "a@s.whatsapp.net", id)
	if err != nil {
		t.Fatal(err)
	}
	return rec
}

func TestValidate(t *testing.T) {
	ok := validConfig()
	if err := ok.Validate(); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}
	if ok.Prefix != "whatsapp/" {
		t.Errorf("prefix should be normalized to a folder, got %q", ok.Prefix)
	}

	for name, mutate := range map[string]func(*Config){
		"no bucket":         func(c *Config) { c.Bucket = "" },
		"bad bucket":        func(c *Config) { c.Bucket = "UPPER_case" },
		"no access key":     func(c *Config) { c.AccessKeyID = "" },
		"no secret":         func(c *Config) { c.SecretAccessKey = "" },
		"endpoint no host":  func(c *Config) { c.Endpoint = "https://" },
		"endpoint bad type": func(c *Config) { c.Endpoint = "ftp://x.example" },
		"endpoint has path": func(c *Config) { c.Endpoint = "https://x.example/bucket" },
		"prefix traversal":  func(c *Config) { c.Prefix = "a/../b" },
		"bad public url":    func(c *Config) { c.PublicBaseURL = "javascript:alert(1)" },
	} {
		c := validConfig()
		mutate(&c)
		if err := c.Validate(); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}

	// A half-filled form can be saved while storage is off.
	off := Config{Enabled: false, Bucket: "x"}
	if err := off.Validate(); err != nil {
		t.Errorf("disabled config should not need to be complete: %v", err)
	}
	// Region defaults for R2.
	noRegion := validConfig()
	noRegion.Region = ""
	_ = noRegion.Validate()
	if noRegion.Region != "auto" {
		t.Errorf("region default = %q", noRegion.Region)
	}
}

func TestSecretSealing(t *testing.T) {
	key := deriveSealKey("api-key")
	sealed, err := seal(key, "topsecret")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(sealed, "topsecret") {
		t.Error("sealed value leaks the plaintext")
	}
	if got, err := open(key, sealed); err != nil || got != "topsecret" {
		t.Errorf("round trip = %q, %v", got, err)
	}
	if _, err := open(deriveSealKey("other-key"), sealed); err == nil {
		t.Error("a different API_KEY must not decrypt the secret")
	}
	if _, err := seal(nil, "x"); !errors.Is(err, ErrNoSecretKey) {
		t.Errorf("sealing without API_KEY = %v", err)
	}
}

func TestSaveStoresSecretEncryptedAndKeepsItWhenBlank(t *testing.T) {
	fake := newFakeStore("media")
	m, db := newTestManager(t, 2, fake)
	if err := m.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Stop)

	pub, err := m.Save(t.Context(), validConfig())
	if err != nil {
		t.Fatal(err)
	}
	if !pub.SecretSet || !m.Enabled() {
		t.Fatalf("saved config should be active: %+v", pub)
	}
	raw, _ := db.GetSetting(settingsKey)
	if strings.Contains(raw, "s3cr3t") {
		t.Errorf("secret stored in plaintext: %s", raw)
	}

	// The panel sends no secret when the field is left blank: the stored one is kept.
	edit := validConfig()
	edit.SecretAccessKey = ""
	edit.KeepLocal = true
	if _, err := m.Save(t.Context(), edit); err != nil {
		t.Fatalf("save without secret: %v", err)
	}
	if cur := m.cur.Load(); cur == nil || cur.cfg.SecretAccessKey != "s3cr3t" || !cur.cfg.KeepLocal {
		t.Errorf("secret not kept or setting not applied: %+v", cur)
	}

	// Status never carries the secret.
	st, err := m.Status()
	if err != nil {
		t.Fatal(err)
	}
	if !st.Config.SecretSet || st.Source != SourcePanel || !st.Active {
		t.Errorf("status = %+v", st)
	}

	// A new process (same API_KEY) reads it back.
	m2 := NewManager(db, waLog.Noop, Options{APIKey: "test-api-key"})
	m2.newStore = func(Config) (ObjectStore, error) { return fake, nil }
	if err := m2.reload(); err != nil || !m2.Enabled() {
		t.Errorf("reload = %v enabled=%v", err, m2.Enabled())
	}
	// With another API_KEY the secret is unreadable: storage stays off and the panel is told why.
	m3 := NewManager(db, waLog.Noop, Options{APIKey: "rotated"})
	m3.newStore = func(Config) (ObjectStore, error) { return fake, nil }
	_ = m3.reload()
	if m3.Enabled() {
		t.Error("storage must not start with an unreadable secret")
	}
	if st, _ := m3.Status(); st.Warning == "" {
		t.Error("status should warn that the stored secret cannot be decrypted")
	}
}

func TestSaveRejectsBrokenConnectionAndKeepsOldConfig(t *testing.T) {
	fake := newFakeStore("media")
	m, db := newTestManager(t, 2, fake)
	_ = m.Start()
	t.Cleanup(m.Stop)

	fake.checkErr = errors.New("access denied: the token needs write access to this bucket")
	if _, err := m.Save(t.Context(), validConfig()); err == nil || !strings.Contains(err.Error(), "access denied") {
		t.Fatalf("save should fail with the connection error, got %v", err)
	}
	if m.Enabled() {
		t.Error("a failed test must not enable storage")
	}
	if _, err := db.GetSetting(settingsKey); !errors.Is(err, database.ErrNotFound) {
		t.Error("a failed test must not save anything")
	}
}

func TestEnvironmentOwnsTheConfiguration(t *testing.T) {
	fake := newFakeStore("envbucket")
	m, _ := newTestManager(t, 2, fake)
	t.Setenv("S3_BUCKET", "envbucket")
	t.Setenv("S3_ACCESS_KEY_ID", "AKIA")
	t.Setenv("S3_SECRET_ACCESS_KEY", "envsecret")
	t.Setenv("S3_ENDPOINT", "https://acct.r2.cloudflarestorage.com")
	if err := m.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Stop)

	if !m.Enabled() {
		t.Fatal("S3_* variables should enable storage")
	}
	if _, err := m.Save(t.Context(), validConfig()); !errors.Is(err, ErrManagedByEnv) {
		t.Errorf("panel edits must be refused while the environment owns the config, got %v", err)
	}
	st, _ := m.Status()
	if st.Source != SourceEnv || !st.Config.SecretSet {
		t.Errorf("status = %+v", st)
	}
}

func TestDownloadedMediaIsUploadedAndLocalCopyRemoved(t *testing.T) {
	fake := newFakeStore("media")
	m, db := newTestManager(t, 2, fake)
	_ = m.Start()
	t.Cleanup(m.Stop)
	if _, err := m.Save(t.Context(), validConfig()); err != nil {
		t.Fatal(err)
	}

	rec := writeLocal(t, "M1", "pixels")
	m.HandleMedia(rec)

	waitFor(t, "upload", func() bool { return statusOf(t, db, "M1").Status == database.MediaUploaded })
	got := statusOf(t, db, "M1")
	if !strings.HasPrefix(got.ObjectKey, "whatsapp/sem-setor/sem-responsavel/5511/") || !strings.HasSuffix(got.ObjectKey, "/2026-09/M1.jpg") {
		t.Errorf("object key = %q", got.ObjectKey)
	}
	if got.Bucket != "media" || got.LocalPath != "" {
		t.Errorf("record after upload: %+v", got)
	}
	if string(fake.objects[got.ObjectKey]) != "pixels" || fake.types[got.ObjectKey] != "image/jpeg" {
		t.Errorf("stored object wrong: %q (%s)", fake.objects[got.ObjectKey], fake.types[got.ObjectKey])
	}
	waitFor(t, "local copy removal", func() bool { _, err := os.Stat(rec.LocalPath); return os.IsNotExist(err) })

	// Reading back goes through the bucket.
	link, err := m.URL(t.Context(), got, 10*time.Minute)
	if err != nil || !strings.Contains(link, got.ObjectKey) || !strings.Contains(link, "ttl=600") {
		t.Errorf("URL = %q, %v", link, err)
	}
	body, err := m.Open(t.Context(), got)
	if err != nil {
		t.Fatal(err)
	}
	if data, _ := io.ReadAll(body); string(data) != "pixels" {
		t.Errorf("Open returned %q", data)
	}
	// An object in another bucket cannot be read through this configuration.
	other := *got
	other.Bucket = "old-bucket"
	if _, err := m.Open(t.Context(), &other); !errors.Is(err, ErrNotStored) {
		t.Errorf("wrong bucket should be ErrNotStored, got %v", err)
	}
}

func TestKeepLocalKeepsTheFile(t *testing.T) {
	fake := newFakeStore("media")
	m, db := newTestManager(t, 1, fake)
	_ = m.Start()
	t.Cleanup(m.Stop)
	cfg := validConfig()
	cfg.KeepLocal = true
	if _, err := m.Save(t.Context(), cfg); err != nil {
		t.Fatal(err)
	}
	rec := writeLocal(t, "M1", "pixels")
	m.HandleMedia(rec)
	waitFor(t, "upload", func() bool { return statusOf(t, db, "M1").Status == database.MediaUploaded })
	if _, err := os.Stat(rec.LocalPath); err != nil {
		t.Errorf("local copy should be kept: %v", err)
	}
	if statusOf(t, db, "M1").LocalPath != rec.LocalPath {
		t.Error("kept local path should stay registered")
	}
}

func TestUploadsAreBoundedByWorkers(t *testing.T) {
	fake := newFakeStore("media")
	fake.gate = make(chan struct{})
	const workers, files = 3, 12
	m, db := newTestManager(t, workers, fake)
	_ = m.Start()
	t.Cleanup(m.Stop)
	if _, err := m.Save(t.Context(), validConfig()); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < files; i++ {
		m.HandleMedia(writeLocal(t, fmt.Sprintf("M%d", i), "x"))
	}
	waitFor(t, "workers saturated", func() bool { return fake.inflight.Load() == workers })
	time.Sleep(200 * time.Millisecond) // give an over-eager dispatcher the chance to exceed the limit
	if got := fake.maxInflight.Load(); got > workers {
		t.Fatalf("%d simultaneous uploads with %d workers", got, workers)
	}
	close(fake.gate)
	waitFor(t, "all uploaded", func() bool {
		stats, _ := db.MediaQueueStats()
		return stats[database.MediaUploaded] == files
	})
	if fake.maxInflight.Load() > workers {
		t.Errorf("max simultaneous uploads = %d, limit %d", fake.maxInflight.Load(), workers)
	}
}

func TestLocalMediaIsBackfilledWhenStorageGetsEnabled(t *testing.T) {
	fake := newFakeStore("media")
	m, db := newTestManager(t, 2, fake)
	_ = m.Start()
	t.Cleanup(m.Stop)

	// Storage is off: the file is only registered.
	m.HandleMedia(writeLocal(t, "M1", "old"))
	if got := statusOf(t, db, "M1").Status; got != database.MediaLocal {
		t.Fatalf("status with storage off = %q, want local", got)
	}
	if fake.count() != 0 {
		t.Fatal("nothing should upload while storage is off")
	}

	if _, err := m.Save(t.Context(), validConfig()); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "backfill upload", func() bool { return statusOf(t, db, "M1").Status == database.MediaUploaded })
}

func TestQueueSurvivesARestart(t *testing.T) {
	fake := newFakeStore("media")
	m, db := newTestManager(t, 2, fake)
	_ = m.Start()
	if _, err := m.Save(t.Context(), validConfig()); err != nil {
		t.Fatal(err)
	}
	// A process that died mid-upload left this row 'uploading'.
	rec := writeLocal(t, "M1", "pixels")
	rec.Status = database.MediaUploading
	if err := db.RecordMedia(rec); err != nil {
		t.Fatal(err)
	}
	m.Stop()

	m2 := NewManager(db, waLog.Noop, Options{APIKey: "test-api-key", UploadWorkers: 2})
	m2.newStore = func(Config) (ObjectStore, error) { return fake, nil }
	if err := m2.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m2.Stop)
	waitFor(t, "resumed upload", func() bool { return statusOf(t, db, "M1").Status == database.MediaUploaded })
}

func TestFailureHandling(t *testing.T) {
	fake := newFakeStore("media")
	m, db := newTestManager(t, 1, fake)
	m.maxAttempts = 3

	newRow := func(id string) *database.MediaRecord {
		rec := writeLocal(t, id, "x")
		rec.Status = database.MediaPendingUpload
		if err := db.RecordMedia(rec); err != nil {
			t.Fatal(err)
		}
		return statusOf(t, db, id)
	}

	// A network error is retried later and counts an attempt.
	r := newRow("net")
	m.finishFailed(r, "upload", errors.New("connection reset"), false)
	got := statusOf(t, db, "net")
	if got.Status != database.MediaPendingUpload || got.Attempts != 1 {
		t.Errorf("transient failure: %+v", got)
	}

	// Out of attempts: failed for good.
	r = newRow("exhausted")
	r.Attempts = 2
	m.finishFailed(r, "upload", errors.New("connection reset"), false)
	if got := statusOf(t, db, "exhausted"); got.Status != database.MediaFailed || got.Attempts != 3 {
		t.Errorf("exhausted: %+v", got)
	}

	// A wrong key/bucket is the operator's problem: wait, don't burn attempts, surface the error.
	r = newRow("config")
	r.Attempts = 2
	m.finishFailed(r, "upload", minio.ErrorResponse{Code: "AccessDenied", Message: "denied"}, false)
	got = statusOf(t, db, "config")
	if got.Status != database.MediaPendingUpload || got.Attempts != 2 {
		t.Errorf("config error should not use attempts: %+v", got)
	}
	if st, _ := m.Status(); st.LastError == "" {
		t.Error("a settings error should be visible in the status")
	}

	// A missing local file cannot be fixed by retrying.
	r = newRow("gone")
	m.finishFailed(r, "local file missing", os.ErrNotExist, true)
	if got := statusOf(t, db, "gone"); got.Status != database.MediaFailed {
		t.Errorf("permanent: %+v", got)
	}
}

func TestBackoffGrowsAndCaps(t *testing.T) {
	if backoff(1) != 30*time.Second || backoff(2) != time.Minute || backoff(3) != 2*time.Minute {
		t.Errorf("unexpected backoff: %v %v %v", backoff(1), backoff(2), backoff(3))
	}
	if backoff(50) != maxBackoff {
		t.Errorf("backoff must cap at %v, got %v", maxBackoff, backoff(50))
	}
}

func TestRetryFailedRequeues(t *testing.T) {
	fake := newFakeStore("media")
	m, db := newTestManager(t, 1, fake)
	_ = m.Start()
	t.Cleanup(m.Stop)
	if _, err := m.RetryFailed(); err == nil {
		t.Error("retry with storage off should explain itself")
	}
	if _, err := m.Save(t.Context(), validConfig()); err != nil {
		t.Fatal(err)
	}
	rec := writeLocal(t, "M1", "x")
	rec.Status = database.MediaFailed
	if err := db.RecordMedia(rec); err != nil {
		t.Fatal(err)
	}
	n, err := m.RetryFailed()
	if err != nil || n != 1 {
		t.Fatalf("RetryFailed = %d, %v", n, err)
	}
	waitFor(t, "retried upload", func() bool { return statusOf(t, db, "M1").Status == database.MediaUploaded })
}
