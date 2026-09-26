package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"whatsapp-bridge/internal/database"
	"whatsapp-bridge/internal/mediastore"
)

// SetMediaStore wires the object storage manager. Without one, the media routes report that
// storage is unavailable and downloads keep working from the local folder and the CDN.
func (s *Server) SetMediaStore(m *mediastore.Manager) {
	s.mediaStore = m
}

// maxLinkTTL and defaultLinkTTL bound the lifetime of signed media links.
const (
	defaultLinkTTL = 15 * time.Minute
	maxLinkTTL     = 24 * time.Hour
)

// storageInput is the body of PUT and POST /api/settings/media-storage. The secret is optional:
// left empty it keeps the stored one.
type storageInput struct {
	Enabled         bool   `json:"enabled"`
	Endpoint        string `json:"endpoint"`
	Region          string `json:"region"`
	Bucket          string `json:"bucket"`
	AccessKeyID     string `json:"access_key_id"`
	SecretAccessKey string `json:"secret_access_key"`
	PathStyle       bool   `json:"path_style"`
	Prefix          string `json:"prefix"`
	KeepLocal       bool   `json:"keep_local"`
	PublicBaseURL   string `json:"public_base_url"`
}

func (in storageInput) config() mediastore.Config {
	return mediastore.Config{
		Enabled:         in.Enabled,
		Endpoint:        in.Endpoint,
		Region:          in.Region,
		Bucket:          in.Bucket,
		AccessKeyID:     in.AccessKeyID,
		SecretAccessKey: in.SecretAccessKey,
		PathStyle:       in.PathStyle,
		Prefix:          in.Prefix,
		KeepLocal:       in.KeepLocal,
		PublicBaseURL:   in.PublicBaseURL,
	}
}

// requireMediaStore answers 503 when the bridge runs without the object storage manager.
func (s *Server) requireMediaStore(w http.ResponseWriter) bool {
	if s.mediaStore == nil {
		s.fail(w, http.StatusServiceUnavailable, "object storage is not available")
		return false
	}
	return true
}

// handleMediaStorage handles GET and PUT /api/settings/media-storage.
//
// GET returns the configuration without the secret, where it comes from (environment or panel),
// and the upload queue depth. PUT validates the settings, proves they can write to the bucket,
// then saves and applies them; nothing is saved when the test fails.
func (s *Server) handleMediaStorage(w http.ResponseWriter, r *http.Request) {
	if !s.requireMediaStore(w) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		status, err := s.mediaStore.Status()
		if err != nil {
			s.serverError(w, "media storage status", err)
			return
		}
		s.writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "data": status})

	case http.MethodPut:
		var in storageInput
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&in); err != nil {
			s.fail(w, http.StatusBadRequest, "invalid request body")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		if _, err := s.mediaStore.Save(ctx, in.config()); err != nil {
			s.mediaStoreError(w, "save media storage settings", err)
			return
		}
		// The secret is never logged.
		log.Printf("api: media storage settings changed by %s (enabled=%v bucket=%s)", actorOf(r), in.Enabled, in.Bucket)
		status, err := s.mediaStore.Status()
		if err != nil {
			s.serverError(w, "media storage status", err)
			return
		}
		s.writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "data": status})

	default:
		s.fail(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// mediaStoreError maps a manager error to a response. Validation and connection errors carry
// text the operator needs, so they are returned as 400; anything else is an internal error.
func (s *Server) mediaStoreError(w http.ResponseWriter, what string, err error) {
	switch {
	case errors.Is(err, mediastore.ErrManagedByEnv):
		s.fail(w, http.StatusConflict, err.Error())
	case errors.Is(err, mediastore.ErrNoSecretKey):
		s.fail(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, database.ErrNotFound):
		s.fail(w, http.StatusNotFound, "not found")
	default:
		log.Printf("api: %s: %v", what, err)
		s.fail(w, http.StatusBadRequest, err.Error())
	}
}

// handleMediaStorageTest handles POST /api/settings/media-storage/test: it connects with the
// submitted settings (or the saved secret when none is submitted) and reports whether the bucket
// accepts uploads. Nothing is saved.
func (s *Server) handleMediaStorageTest(w http.ResponseWriter, r *http.Request) {
	if !s.requireMediaStore(w) {
		return
	}
	if r.Method != http.MethodPost {
		s.fail(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var in storageInput
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&in); err != nil {
		s.fail(w, http.StatusBadRequest, "invalid request body")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	if err := s.mediaStore.Test(ctx, in.config()); err != nil {
		s.mediaStoreError(w, "test media storage", err)
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "message": "connection works: a test object was written and deleted"})
}

// handleMediaStorageRetry handles POST /api/settings/media-storage/retry: files that failed
// (after their retries, or because the download failed with a local copy) go back to the queue.
func (s *Server) handleMediaStorageRetry(w http.ResponseWriter, r *http.Request) {
	if !s.requireMediaStore(w) {
		return
	}
	if r.Method != http.MethodPost {
		s.fail(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	n, err := s.mediaStore.RetryFailed()
	if err != nil {
		s.mediaStoreError(w, "retry failed uploads", err)
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "queued": n})
}

// handleMediaURL handles GET /api/media/url?chat_jid=&message_id=[&instance_jid=][&ttl=seconds].
//
// It returns a link to the media of a message in object storage (signed and expiring for a
// private bucket) together with what is known about the file. The bucket stays private: access
// goes through the bridge, which has already authenticated the caller.
func (s *Server) handleMediaURL(w http.ResponseWriter, r *http.Request) {
	if !s.requireMediaStore(w) {
		return
	}
	if r.Method != http.MethodGet {
		s.fail(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	q := r.URL.Query()
	chatJID, messageID := q.Get("chat_jid"), q.Get("message_id")
	if chatJID == "" || messageID == "" {
		s.fail(w, http.StatusBadRequest, "chat_jid and message_id are required")
		return
	}
	ttl := defaultLinkTTL
	if v := q.Get("ttl"); v != "" {
		secs, err := strconv.Atoi(v)
		if err != nil || secs <= 0 {
			s.fail(w, http.StatusBadRequest, "ttl must be a positive number of seconds")
			return
		}
		ttl = min(time.Duration(secs)*time.Second, maxLinkTTL)
	}

	var (
		rec *database.MediaRecord
		err error
	)
	if inst := q.Get("instance_jid"); inst != "" {
		rec, err = s.messageStore.GetMediaRecord(inst, chatJID, messageID)
	} else {
		rec, err = s.messageStore.FindMediaRecord(chatJID, messageID)
	}
	if err != nil {
		s.serverError(w, "find media", err)
		return
	}
	if rec.Status != database.MediaUploaded {
		s.writeJSON(w, http.StatusConflict, map[string]interface{}{
			"success": false, "error": "media is not in object storage yet", "status": rec.Status,
		})
		return
	}
	link, err := s.mediaStore.URL(r.Context(), rec, ttl)
	if err != nil {
		s.mediaStoreError(w, "media url", err)
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"success":      true,
		"url":          link,
		"expires_in":   int(ttl.Seconds()),
		"object_key":   rec.ObjectKey,
		"content_type": rec.ContentType,
		"size":         rec.Size,
		"media_type":   rec.MediaType,
	})
}

// storedMediaPath finds a message's media in the places this bridge keeps it, without touching
// the WhatsApp CDN: the local file first, then the bucket (copied into the local media folder so
// the caller gets a path, as it always has). ok is false when the CDN is the only source left.
func (s *Server) storedMediaPath(ctx context.Context, chatJID, messageID string) (path string, size int64, ok bool) {
	rec, err := s.messageStore.FindMediaRecord(chatJID, messageID)
	if err != nil {
		return "", 0, false
	}
	if rec.LocalPath != "" {
		if info, err := os.Stat(rec.LocalPath); err == nil && info.Size() > 0 {
			return rec.LocalPath, info.Size(), true
		}
	}
	if s.mediaStore == nil || rec.Status != database.MediaUploaded {
		return "", 0, false
	}

	ext := filepath.Ext(rec.ObjectKey)
	out := filepath.Join("store", "media", sanitizePath(chatJID), sanitizePath(messageID)+ext)
	if info, err := os.Stat(out); err == nil && info.Size() > 0 {
		return out, info.Size(), true // cached by an earlier request
	}
	body, err := s.mediaStore.Open(ctx, rec)
	if err != nil {
		log.Printf("api: read %s from object storage: %v", rec.ObjectKey, err)
		return "", 0, false
	}
	defer body.Close()

	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return "", 0, false
	}
	// Same atomic pattern as the CDN path: never expose a half-written file.
	tmp := out + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return "", 0, false
	}
	n, copyErr := io.Copy(f, io.LimitReader(body, maxMediaBytes+1))
	closeErr := f.Close()
	if copyErr != nil || closeErr != nil || n > maxMediaBytes || strings.TrimSpace(out) == "" {
		_ = os.Remove(tmp)
		return "", 0, false
	}
	if err := os.Rename(tmp, out); err != nil {
		_ = os.Remove(tmp)
		return "", 0, false
	}
	return out, n, true
}
