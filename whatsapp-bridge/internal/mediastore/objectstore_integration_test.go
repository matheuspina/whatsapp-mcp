package mediastore

import (
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/minio/minio-go/v7"

	"whatsapp-bridge/internal/database"
)

// TestObjectStoreAgainstRealService runs the S3 client against a live S3-compatible service
// (MinIO, R2, ...). It is skipped unless S3_TEST_ENDPOINT is set:
//
//	docker run -d -p 9000:9000 -e MINIO_ROOT_USER=test -e MINIO_ROOT_PASSWORD=testtest123 minio/minio server /data
//	S3_TEST_ENDPOINT=http://127.0.0.1:9000 S3_TEST_BUCKET=media S3_TEST_ACCESS_KEY=test S3_TEST_SECRET_KEY=testtest123 go test ./internal/mediastore
//
// The bucket is created when it does not exist.
func TestObjectStoreAgainstRealService(t *testing.T) {
	endpoint := os.Getenv("S3_TEST_ENDPOINT")
	if endpoint == "" {
		t.Skip("S3_TEST_ENDPOINT not set")
	}
	region := os.Getenv("S3_TEST_REGION")
	if region == "" {
		region = "auto" // Cloudflare R2
	}
	cfg := Config{
		Enabled: true, Endpoint: endpoint, Region: region, Bucket: os.Getenv("S3_TEST_BUCKET"),
		AccessKeyID: os.Getenv("S3_TEST_ACCESS_KEY"), SecretAccessKey: os.Getenv("S3_TEST_SECRET_KEY"), PathStyle: true,
	}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	store, err := newMinioStore(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if mc, ok := store.(*minioStore); ok {
		if err := mc.client.MakeBucket(t.Context(), cfg.Bucket, minio.MakeBucketOptions{}); err != nil {
			if code := minio.ToErrorResponse(err).Code; code != "BucketAlreadyOwnedByYou" && code != "BucketAlreadyExists" {
				t.Fatalf("create bucket: %v", err)
			}
		}
	}

	if err := store.Check(t.Context()); err != nil {
		t.Fatalf("Check: %v", err)
	}

	// Wrong credentials are explained, and recognised as a settings problem (not retried as a network error).
	bad := cfg
	bad.SecretAccessKey = "wrong-secret-value"
	badStore, _ := newMinioStore(bad)
	err = badStore.Check(t.Context())
	if err == nil {
		t.Fatal("Check with a wrong secret should fail")
	}
	t.Logf("wrong secret is reported as: %v", err)

	// A file larger than one multipart part, streamed from disk.
	f, err := os.CreateTemp(t.TempDir(), "big-*.bin")
	if err != nil {
		t.Fatal(err)
	}
	payload := strings.Repeat("0123456789abcdef", 1<<20) // 16 MiB
	if _, err := f.WriteString(payload); err != nil {
		t.Fatal(err)
	}
	info, _ := f.Stat()
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}

	key := "integration-test/" + time.Now().Format("150405.000") + "/big.bin"
	if err := store.Put(t.Context(), key, "application/octet-stream", f, info.Size()); err != nil {
		t.Fatalf("Put: %v", err)
	}
	body, err := store.Get(t.Context(), key)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	got, _ := io.ReadAll(body)
	_ = body.Close()
	if string(got) != payload {
		t.Errorf("round trip changed the file (%d bytes back, want %d)", len(got), len(payload))
	}
	link, err := store.Presign(t.Context(), key, time.Minute)
	if err != nil || !strings.Contains(link, "X-Amz-Signature") {
		t.Errorf("Presign = %q, %v", link, err)
	}
	if _, err := store.Get(t.Context(), "does/not/exist"); err == nil {
		t.Error("Get of a missing object should fail")
	}
}

// TestManagerAgainstRealService drives the whole pipeline (registered file -> queue -> upload ->
// signed link -> read back) against a live service, with the real S3 client.
func TestManagerAgainstRealService(t *testing.T) {
	endpoint := os.Getenv("S3_TEST_ENDPOINT")
	if endpoint == "" {
		t.Skip("S3_TEST_ENDPOINT not set")
	}
	region := os.Getenv("S3_TEST_REGION")
	if region == "" {
		region = "auto"
	}
	m, db := newTestManager(t, 2, nil)
	m.newStore = newMinioStore
	if err := m.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Stop)

	cfg := Config{Enabled: true, Endpoint: endpoint, Region: region, Bucket: os.Getenv("S3_TEST_BUCKET"),
		AccessKeyID: os.Getenv("S3_TEST_ACCESS_KEY"), SecretAccessKey: os.Getenv("S3_TEST_SECRET_KEY"), PathStyle: true, Prefix: "e2e"}
	if _, err := m.Save(t.Context(), cfg); err != nil {
		t.Fatalf("Save against the real service: %v", err)
	}

	rec := writeLocal(t, "E2E"+time.Now().Format("150405"), "real bytes over the wire")
	m.HandleMedia(rec)
	waitFor(t, "real upload", func() bool { return statusOf(t, db, rec.MessageID).Status == database.MediaUploaded })
	got := statusOf(t, db, rec.MessageID)
	t.Logf("stored at %s/%s", got.Bucket, got.ObjectKey)

	body, err := m.Open(t.Context(), got)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := io.ReadAll(body)
	_ = body.Close()
	if string(data) != "real bytes over the wire" {
		t.Errorf("read back %q", data)
	}

	link, err := m.URL(t.Context(), got, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.Get(link)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	fetched, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK || string(fetched) != "real bytes over the wire" {
		t.Errorf("signed link returned %d %q", resp.StatusCode, fetched)
	}
	if resp.Header.Get("Content-Type") != "image/jpeg" {
		t.Errorf("content type = %q", resp.Header.Get("Content-Type"))
	}
}
