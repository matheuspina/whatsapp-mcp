// Package mediastore moves downloaded WhatsApp media to an S3-compatible bucket (Cloudflare R2,
// MinIO, AWS S3, ...) through a durable, asynchronous upload queue.
package mediastore

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/crypto/hkdf"

	"whatsapp-bridge/internal/database"
)

const (
	// settingsKey is the app_settings row that holds the panel-edited configuration.
	settingsKey = "media_storage"

	// SourceNone, SourceEnv and SourcePanel say where the active configuration came from.
	SourceNone  = "none"
	SourceEnv   = "env"
	SourcePanel = "panel"
)

// ErrManagedByEnv is returned when the panel tries to change a configuration that the
// environment (S3_* variables) owns.
var ErrManagedByEnv = errors.New("media storage is configured through environment variables")

// ErrNoSecretKey is returned when a credential would have to be stored but API_KEY, the secret
// its encryption key is derived from, is not set.
var ErrNoSecretKey = errors.New("API_KEY is required to store the secret access key; set it or use the S3_* environment variables")

// Config is the object storage configuration.
type Config struct {
	// Enabled turns uploads on. Off, media stays in the local store folder.
	Enabled bool `json:"enabled"`
	// Endpoint is the S3 API URL, e.g. https://<account>.r2.cloudflarestorage.com. Empty means AWS S3.
	Endpoint string `json:"endpoint"`
	// Region is "auto" for Cloudflare R2; AWS regions otherwise.
	Region          string `json:"region"`
	Bucket          string `json:"bucket"`
	AccessKeyID     string `json:"access_key_id"`
	SecretAccessKey string `json:"-"`
	// PathStyle addresses buckets as host/bucket instead of bucket.host (MinIO, some proxies).
	PathStyle bool `json:"path_style"`
	// Prefix is an optional folder inside the bucket that holds everything, e.g. "whatsapp/".
	Prefix string `json:"prefix"`
	// KeepLocal keeps the local copy after the upload. Off, it is deleted once the object is stored.
	KeepLocal bool `json:"keep_local"`
	// PublicBaseURL serves objects without signing when the bucket is public or has a custom
	// domain. Leave empty for a private bucket: links are then signed and expire.
	PublicBaseURL string `json:"public_base_url"`
}

var bucketNameRE = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]$`)

// Validate normalizes the configuration and rejects values that cannot work. Disabled
// configurations are only normalized, so a half-filled form can be saved while off.
func (c *Config) Validate() error {
	c.Endpoint = strings.TrimSpace(c.Endpoint)
	c.Region = strings.TrimSpace(c.Region)
	c.Bucket = strings.TrimSpace(c.Bucket)
	c.AccessKeyID = strings.TrimSpace(c.AccessKeyID)
	c.PublicBaseURL = strings.TrimRight(strings.TrimSpace(c.PublicBaseURL), "/")

	prefix := strings.Trim(strings.TrimSpace(c.Prefix), "/")
	if strings.Contains(prefix, "..") || strings.ContainsAny(prefix, "\\\x00") {
		return errors.New("prefix must not contain '..', backslashes or control characters")
	}
	if prefix != "" {
		prefix += "/"
	}
	c.Prefix = prefix

	if c.Endpoint != "" {
		u, err := url.Parse(c.Endpoint)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return errors.New("endpoint must be an http(s) URL such as https://<account>.r2.cloudflarestorage.com")
		}
		if u.Path != "" && u.Path != "/" {
			return errors.New("endpoint must not contain a path; use the prefix field for a folder")
		}
	}
	if c.PublicBaseURL != "" {
		u, err := url.Parse(c.PublicBaseURL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return errors.New("public base URL must be an http(s) URL")
		}
	}
	if !c.Enabled {
		return nil
	}
	switch {
	case c.Bucket == "":
		return errors.New("bucket is required")
	case !bucketNameRE.MatchString(c.Bucket):
		return errors.New("bucket name is not valid (3-63 lowercase letters, digits, dots and dashes)")
	case c.AccessKeyID == "":
		return errors.New("access key id is required")
	case c.SecretAccessKey == "":
		return errors.New("secret access key is required")
	}
	if c.Region == "" {
		c.Region = "auto"
	}
	return nil
}

// PublicConfig is what the panel is shown: the configuration without the secret.
type PublicConfig struct {
	Config
	SecretSet bool `json:"secret_set"`
}

// Public returns the configuration safe to send to the browser.
func (c Config) Public() PublicConfig {
	return PublicConfig{Config: c, SecretSet: c.SecretAccessKey != ""}
}

// configFromEnv reads the S3_* variables. It reports false unless S3_BUCKET is set, which is what
// hands the configuration over to the environment.
func configFromEnv() (Config, bool) {
	bucket := strings.TrimSpace(os.Getenv("S3_BUCKET"))
	if bucket == "" {
		return Config{}, false
	}
	boolEnv := func(name string, def bool) bool {
		v, err := strconv.ParseBool(strings.TrimSpace(os.Getenv(name)))
		if err != nil {
			return def
		}
		return v
	}
	return Config{
		Enabled:         boolEnv("S3_ENABLED", true),
		Endpoint:        os.Getenv("S3_ENDPOINT"),
		Region:          os.Getenv("S3_REGION"),
		Bucket:          bucket,
		AccessKeyID:     os.Getenv("S3_ACCESS_KEY_ID"),
		SecretAccessKey: os.Getenv("S3_SECRET_ACCESS_KEY"),
		PathStyle:       boolEnv("S3_PATH_STYLE", false),
		Prefix:          os.Getenv("S3_PREFIX"),
		KeepLocal:       boolEnv("S3_KEEP_LOCAL", false),
		PublicBaseURL:   os.Getenv("S3_PUBLIC_BASE_URL"),
	}, true
}

// stored is the JSON kept in app_settings: the configuration plus the sealed secret.
type stored struct {
	Config
	SecretEnc string `json:"secret_enc,omitempty"`
}

// deriveSealKey derives the AES key that protects the secret access key at rest from API_KEY.
// Rotating API_KEY makes the stored secret unreadable, and the panel then asks for it again.
func deriveSealKey(apiKey string) []byte {
	if apiKey == "" {
		return nil
	}
	key := make([]byte, 32)
	r := hkdf.New(sha256.New, []byte(apiKey), nil, []byte("whatsapp-mcp media storage secret"))
	_, _ = io.ReadFull(r, key)
	return key
}

func seal(key []byte, plaintext string) (string, error) {
	if key == nil {
		return "", ErrNoSecretKey
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(gcm.Seal(nonce, nonce, []byte(plaintext), nil)), nil
}

func open(key []byte, sealed string) (string, error) {
	if key == nil {
		return "", ErrNoSecretKey
	}
	raw, err := base64.StdEncoding.DecodeString(sealed)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(raw) < gcm.NonceSize() {
		return "", errors.New("sealed secret is too short")
	}
	plain, err := gcm.Open(nil, raw[:gcm.NonceSize()], raw[gcm.NonceSize():], nil)
	if err != nil {
		return "", errors.New("stored secret cannot be decrypted (API_KEY changed?)")
	}
	return string(plain), nil
}

// loadStored reads the panel-edited configuration. The second result is a warning when the
// saved secret could not be decrypted; the configuration is then returned without it.
func loadStored(db *database.MessageStore, sealKey []byte) (Config, string, error) {
	raw, err := db.GetSetting(settingsKey)
	if errors.Is(err, database.ErrNotFound) {
		return Config{}, "", nil
	}
	if err != nil {
		return Config{}, "", fmt.Errorf("read media storage settings: %w", err)
	}
	var st stored
	if err := json.Unmarshal([]byte(raw), &st); err != nil {
		return Config{}, "", fmt.Errorf("parse media storage settings: %w", err)
	}
	cfg := st.Config
	if st.SecretEnc != "" {
		secret, err := open(sealKey, st.SecretEnc)
		if err != nil {
			return cfg, err.Error(), nil
		}
		cfg.SecretAccessKey = secret
	}
	return cfg, "", nil
}

func saveStored(db *database.MessageStore, sealKey []byte, cfg Config) error {
	st := stored{Config: cfg}
	if cfg.SecretAccessKey != "" {
		enc, err := seal(sealKey, cfg.SecretAccessKey)
		if err != nil {
			return err
		}
		st.SecretEnc = enc
	}
	raw, err := json.Marshal(st)
	if err != nil {
		return err
	}
	return db.SetSetting(settingsKey, string(raw))
}
