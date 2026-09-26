package mediastore

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// ObjectStore is the slice of an S3 API the bridge needs.
type ObjectStore interface {
	// Put stores the file under key. The file is read by offset, so a large video streams from
	// disk instead of being held in memory.
	Put(ctx context.Context, key, contentType string, f *os.File, size int64) error
	// Get opens an object for reading.
	Get(ctx context.Context, key string) (io.ReadCloser, error)
	// Presign returns a temporary download link for a private bucket.
	Presign(ctx context.Context, key string, ttl time.Duration) (string, error)
	// Check proves the credentials can write to the bucket by storing and deleting a probe object.
	Check(ctx context.Context) error
	// Bucket names the bucket this store writes to.
	Bucket() string
}

// minioStore implements ObjectStore for any S3-compatible service.
type minioStore struct {
	client *minio.Client
	bucket string
}

// newMinioStore builds the S3 client for cfg. It does not contact the service.
func newMinioStore(cfg Config) (ObjectStore, error) {
	host := "s3.amazonaws.com"
	secure := true
	if cfg.Endpoint != "" {
		u, err := url.Parse(cfg.Endpoint)
		if err != nil {
			return nil, fmt.Errorf("parse endpoint: %w", err)
		}
		host = u.Host
		secure = u.Scheme == "https"
	}
	lookup := minio.BucketLookupAuto
	if cfg.PathStyle {
		lookup = minio.BucketLookupPath
	}
	client, err := minio.New(host, &minio.Options{
		Creds:        credentials.NewStaticV4(cfg.AccessKeyID, cfg.SecretAccessKey, ""),
		Secure:       secure,
		Region:       cfg.Region,
		BucketLookup: lookup,
	})
	if err != nil {
		return nil, fmt.Errorf("create S3 client: %w", err)
	}
	return &minioStore{client: client, bucket: cfg.Bucket}, nil
}

func (s *minioStore) Bucket() string { return s.bucket }

func (s *minioStore) Put(ctx context.Context, key, contentType string, f *os.File, size int64) error {
	_, err := s.client.PutObject(ctx, s.bucket, key, f, size, minio.PutObjectOptions{
		ContentType: contentType,
		// One part in flight per upload: the bridge runs several uploads at once in a small
		// container, so parallelism comes from the workers, not from each upload.
		NumThreads: 1,
	})
	return err
}

func (s *minioStore) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	obj, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, err
	}
	// GetObject is lazy: Stat surfaces "not found" and access errors now instead of on first read.
	if _, err := obj.Stat(); err != nil {
		_ = obj.Close()
		return nil, err
	}
	return obj, nil
}

func (s *minioStore) Presign(ctx context.Context, key string, ttl time.Duration) (string, error) {
	u, err := s.client.PresignedGetObject(ctx, s.bucket, key, ttl, nil)
	if err != nil {
		return "", err
	}
	return u.String(), nil
}

func (s *minioStore) Check(ctx context.Context) error {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return err
	}
	key := ".probe/" + hex.EncodeToString(b[:])
	body := strings.NewReader("whatsapp-mcp connection test")
	if _, err := s.client.PutObject(ctx, s.bucket, key, body, int64(body.Len()), minio.PutObjectOptions{ContentType: "text/plain"}); err != nil {
		return explain(err)
	}
	if err := s.client.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{}); err != nil {
		return fmt.Errorf("the bucket accepts uploads but deleting the test object failed: %w", explain(err))
	}
	return nil
}

// explain turns an S3 error response into a sentence the operator can act on.
func explain(err error) error {
	resp := minio.ToErrorResponse(err)
	switch resp.Code {
	case "NoSuchBucket":
		return fmt.Errorf("bucket not found (check the bucket name and the endpoint)")
	case "InvalidAccessKeyId":
		return fmt.Errorf("access key id not recognised")
	case "SignatureDoesNotMatch":
		return fmt.Errorf("secret access key rejected (or wrong region)")
	case "AccessDenied":
		return fmt.Errorf("access denied: the token needs write access to this bucket")
	}
	return err
}

// isConfigError reports errors that retrying cannot fix until someone changes the settings.
// Uploads that hit one wait for the operator instead of using up their attempts.
func isConfigError(err error) bool {
	switch minio.ToErrorResponse(err).Code {
	case "NoSuchBucket", "InvalidAccessKeyId", "SignatureDoesNotMatch", "AccessDenied":
		return true
	}
	return false
}
