package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds application configuration
type Config struct {
	APIPort     int
	APIBindHost string // API_BIND_HOST: default 127.0.0.1 (safe for local dev); set 0.0.0.0 in Docker

	// History sync configuration (Phase 4)
	HistorySyncDaysLimit uint32 // HISTORY_SYNC_DAYS_LIMIT env var
	HistorySyncSizeMB    uint32 // HISTORY_SYNC_SIZE_MB env var
	StorageQuotaMB       uint32 // STORAGE_QUOTA_MB env var

	// Presence ping configuration
	// PRESENCE_PING_ENABLED=false disables presence broadcasts to contacts (default true)
	// PRESENCE_PING_INTERVAL sets how often to ping (default 20m; reduce below 25m risks bot fingerprinting)
	PresencePingEnabled  bool
	PresencePingInterval time.Duration

	// Human-like presence behaviour
	// WA_PRESENCE_MODE=human (default) keeps the account offline and only goes
	// online for a short window around outgoing activity. always_online restores the
	// legacy "online while connected" behaviour.
	PresenceMode      string
	PresenceLingerMin time.Duration // PRESENCE_LINGER_MIN
	PresenceLingerMax time.Duration // PRESENCE_LINGER_MAX

	// Safety gate: comma-separated list of allowed JIDs/phone numbers (WHATSAPP_ALLOWLIST_JIDS)
	// If set, outgoing message sends to JIDs/numbers outside this allowlist are rejected.
	AllowlistJIDs []string

	// Web UI login (server-side sessions). Both must be set to enable login.
	WebUIUsername   string        // WEB_UI_USERNAME
	WebUIPassword   string        // WEB_UI_PASSWORD
	WebUISessionTTL time.Duration // WEB_UI_SESSION_TTL (default 24h, sliding expiry)

	// Governance
	// RETENTION_DAYS deletes captured messages older than this many days (0 disables).
	RetentionDays int
	// REQUIRE_CORPORATE_CONFIRMATION=true stops capturing messages from numbers whose
	// corporate-asset attestation has not been recorded.
	RequireCorporateConfirmation bool
	// INSTANCE_ALLOW_SEND_DEFAULT (default true) is the send permission given to numbers that are
	// registered without an explicit choice (the first device, or devices loaded at startup).
	InstanceAllowSendDefault bool
	// MAX_PENDING_PAIRINGS limits QR pairings in progress at the same time (default 5).
	MaxPendingPairings int

	// Media pipeline. MEDIA_DOWNLOAD_WORKERS caps simultaneous downloads from the WhatsApp CDN
	// (default 4); MEDIA_UPLOAD_WORKERS caps simultaneous uploads to object storage (default 4).
	// The bucket itself is configured in the panel or with the S3_* variables.
	MediaDownloadWorkers int
	MediaUploadWorkers   int
}

// IsPlaceholder reports whether a secret still holds the example value shipped in
// .env.example (anything starting with "CHANGEME"). Such values are public, so they
// must never be accepted as real credentials.
func IsPlaceholder(v string) bool {
	return strings.HasPrefix(strings.ToUpper(strings.TrimSpace(v)), "CHANGEME")
}

// NewConfig creates a new configuration with default values
func NewConfig() *Config {
	cfg := &Config{
		APIPort:     8080,
		APIBindHost: "127.0.0.1",
		// History sync defaults
		HistorySyncDaysLimit: 365,   // 1 year default
		HistorySyncSizeMB:    5000,  // 5GB default
		StorageQuotaMB:       10240, // 10GB default
		// Presence ping defaults
		PresencePingEnabled:  true,
		PresencePingInterval: 20 * time.Minute,
		// Human-like presence defaults
		PresenceMode:      "human",
		PresenceLingerMin: 8 * time.Second,
		PresenceLingerMax: 15 * time.Second,
		// Web UI session default
		WebUISessionTTL: 24 * time.Hour,
		// Governance defaults
		InstanceAllowSendDefault: true,
		MaxPendingPairings:       5,
		// Media pipeline defaults
		MediaDownloadWorkers: 4,
		MediaUploadWorkers:   4,
	}

	if v := os.Getenv("RETENTION_DAYS"); v != "" {
		if d, err := strconv.Atoi(v); err == nil && d >= 0 {
			cfg.RetentionDays = d
		}
	}
	cfg.RequireCorporateConfirmation = os.Getenv("REQUIRE_CORPORATE_CONFIRMATION") == "true"
	if os.Getenv("INSTANCE_ALLOW_SEND_DEFAULT") == "false" {
		cfg.InstanceAllowSendDefault = false
	}
	if v := os.Getenv("MAX_PENDING_PAIRINGS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.MaxPendingPairings = n
		}
	}

	if v := os.Getenv("MEDIA_DOWNLOAD_WORKERS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.MediaDownloadWorkers = n
		}
	}
	if v := os.Getenv("MEDIA_UPLOAD_WORKERS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.MediaUploadWorkers = n
		}
	}

	cfg.WebUIUsername = os.Getenv("WEB_UI_USERNAME")
	cfg.WebUIPassword = os.Getenv("WEB_UI_PASSWORD")
	if ttl := os.Getenv("WEB_UI_SESSION_TTL"); ttl != "" {
		if d, err := time.ParseDuration(ttl); err == nil && d > 0 {
			cfg.WebUISessionTTL = d
		}
	}

	// Override with environment variables if set
	if port := os.Getenv("API_PORT"); port != "" {
		if p, err := strconv.Atoi(port); err == nil {
			cfg.APIPort = p
		}
	}

	if bindHost := os.Getenv("API_BIND_HOST"); bindHost != "" {
		cfg.APIBindHost = bindHost
	}

	if days := os.Getenv("HISTORY_SYNC_DAYS_LIMIT"); days != "" {
		if d, err := strconv.ParseUint(days, 10, 32); err == nil {
			cfg.HistorySyncDaysLimit = uint32(d)
		}
	}

	if size := os.Getenv("HISTORY_SYNC_SIZE_MB"); size != "" {
		if s, err := strconv.ParseUint(size, 10, 32); err == nil {
			cfg.HistorySyncSizeMB = uint32(s)
		}
	}

	if quota := os.Getenv("STORAGE_QUOTA_MB"); quota != "" {
		if q, err := strconv.ParseUint(quota, 10, 32); err == nil {
			cfg.StorageQuotaMB = uint32(q)
		}
	}

	if enabled := os.Getenv("PRESENCE_PING_ENABLED"); enabled == "false" {
		cfg.PresencePingEnabled = false
	}

	if interval := os.Getenv("PRESENCE_PING_INTERVAL"); interval != "" {
		if d, err := time.ParseDuration(interval); err == nil && d > 0 {
			cfg.PresencePingInterval = d
		}
	}

	// WA_PRESENCE_MODE is the documented name; the JUNO_ prefixed variant stays readable so an
	// existing deployment keeps working after the rename.
	mode := os.Getenv("WA_PRESENCE_MODE")
	if mode == "" {
		mode = os.Getenv("JUNO_WA_PRESENCE_MODE")
	}
	if mode == "always_online" {
		cfg.PresenceMode = "always_online"
	}

	if min := os.Getenv("PRESENCE_LINGER_MIN"); min != "" {
		if d, err := time.ParseDuration(min); err == nil && d > 0 {
			cfg.PresenceLingerMin = d
		}
	}

	if max := os.Getenv("PRESENCE_LINGER_MAX"); max != "" {
		if d, err := time.ParseDuration(max); err == nil && d > 0 {
			cfg.PresenceLingerMax = d
		}
	}

	if cfg.PresenceLingerMax < cfg.PresenceLingerMin {
		cfg.PresenceLingerMax = cfg.PresenceLingerMin
	}

	allowlist := os.Getenv("WHATSAPP_ALLOWLIST_JIDS")
	if allowlist == "" {
		allowlist = os.Getenv("WHATSAPP_JID_ALLOWLIST")
	}
	if allowlist != "" {
		for _, jid := range strings.Split(allowlist, ",") {
			trimmed := strings.TrimSpace(jid)
			if trimmed != "" {
				cfg.AllowlistJIDs = append(cfg.AllowlistJIDs, trimmed)
			}
		}
	}

	return cfg
}
