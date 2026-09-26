package mediastore

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"sync/atomic"
	"time"

	waLog "go.mau.fi/whatsmeow/util/log"

	"whatsapp-bridge/internal/database"
)

const (
	defaultWorkers     = 4
	defaultMaxAttempts = 8
	pollInterval       = 10 * time.Second
	maxBackoff         = time.Hour
	// configErrorDelay is how long an upload that hit a settings problem (bad key, missing
	// bucket) waits before the queue tries it again.
	configErrorDelay = 5 * time.Minute
	// backfillChunk is how many local files are queued per step when storage gets enabled.
	backfillChunk = 500
)

// ErrNotStored is returned when a file cannot be read from the bucket: storage is off, or the
// file lives in a bucket other than the configured one.
var ErrNotStored = errors.New("media is not available in the configured bucket")

// Options tunes a Manager.
type Options struct {
	// APIKey is the secret the encryption key for the stored credentials is derived from.
	APIKey string
	// UploadWorkers is how many files upload at once (default 4).
	UploadWorkers int
	// MaxAttempts is how many failed attempts a file gets before it is marked failed (default 8).
	MaxAttempts int
	// NewStore builds the client for a configuration. Nil uses the S3-compatible client; tests
	// inject a fake.
	NewStore func(Config) (ObjectStore, error)
}

// active is the configuration in force and the client built from it.
type active struct {
	cfg   Config
	store ObjectStore
}

// Manager owns the upload queue and the storage configuration.
//
// Downloaded media is registered in message_media as 'pending_upload'; one dispatcher claims due
// rows and hands them to a fixed number of upload goroutines. Everything the queue needs is in
// the database, so a restart loses nothing, and no more than UploadWorkers files are ever open at
// once no matter how many messages arrive.
type Manager struct {
	db          *database.MessageStore
	log         waLog.Logger
	sealKey     []byte
	workers     int
	maxAttempts int
	newStore    func(Config) (ObjectStore, error)

	cur atomic.Pointer[active] // nil while uploads are off

	mu      sync.Mutex // guards source, warning and configuration changes
	source  string
	warning string

	lastErrMu sync.Mutex
	lastErr   string

	wake   chan struct{}
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup // the dispatcher
	upWG   sync.WaitGroup // uploads in flight
}

// NewManager creates a manager. Call Start to load the configuration and begin uploading.
func NewManager(db *database.MessageStore, log waLog.Logger, opts Options) *Manager {
	if opts.UploadWorkers <= 0 {
		opts.UploadWorkers = defaultWorkers
	}
	if opts.MaxAttempts <= 0 {
		opts.MaxAttempts = defaultMaxAttempts
	}
	if opts.NewStore == nil {
		opts.NewStore = newMinioStore
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{
		db:          db,
		log:         log,
		sealKey:     deriveSealKey(opts.APIKey),
		workers:     opts.UploadWorkers,
		maxAttempts: opts.MaxAttempts,
		newStore:    opts.NewStore,
		source:      SourceNone,
		wake:        make(chan struct{}, 1),
		ctx:         ctx,
		cancel:      cancel,
	}
}

// Start loads the configuration (environment first, then the panel's) and starts the dispatcher.
func (m *Manager) Start() error {
	if n, err := m.db.ResetStuckUploads(); err != nil {
		m.log.Warnf("media: could not requeue interrupted uploads: %v", err)
	} else if n > 0 {
		m.log.Infof("media: %d interrupted uploads requeued", n)
	}

	if err := m.reload(); err != nil {
		// A broken configuration must not stop the bridge: media just stays local.
		m.log.Warnf("media: object storage not started: %v", err)
		m.setLastError(err.Error())
	}

	m.wg.Add(1)
	go m.run()
	return nil
}

// Stop cancels running uploads (their rows return to the queue on the next start) and waits.
func (m *Manager) Stop() {
	m.cancel()
	m.wg.Wait()
	m.upWG.Wait()
}

// reload resolves the configuration from its sources and applies it.
func (m *Manager) reload() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if cfg, ok := configFromEnv(); ok {
		m.source, m.warning = SourceEnv, ""
		return m.applyLocked(cfg)
	}
	cfg, warning, err := loadStored(m.db, m.sealKey)
	if err != nil {
		return err
	}
	m.warning = warning
	if warning != "" {
		m.log.Warnf("media: %s", warning)
	}
	m.source = SourcePanel
	if cfg == (Config{}) {
		m.source = SourceNone
	}
	return m.applyLocked(cfg)
}

// applyLocked makes cfg the active configuration. The caller holds m.mu.
func (m *Manager) applyLocked(cfg Config) error {
	if err := cfg.Validate(); err != nil {
		m.cur.Store(nil)
		return err
	}
	if !cfg.Enabled {
		m.cur.Store(nil)
		m.setLastError("")
		return nil
	}
	store, err := m.newStore(cfg)
	if err != nil {
		m.cur.Store(nil)
		return err
	}
	m.cur.Store(&active{cfg: cfg, store: store})
	m.setLastError("")
	m.log.Infof("media: object storage on (bucket %s, workers %d)", cfg.Bucket, m.workers)
	go m.backfillLocal()
	m.signal()
	return nil
}

// Enabled reports whether uploads are on.
func (m *Manager) Enabled() bool { return m.cur.Load() != nil }

// Save validates, tests, stores and applies a configuration edited in the panel. An empty
// SecretAccessKey keeps the stored one. Nothing is saved when the test fails.
func (m *Manager) Save(ctx context.Context, in Config) (PublicConfig, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.source == SourceEnv {
		return PublicConfig{}, ErrManagedByEnv
	}
	if in.SecretAccessKey == "" {
		if prev, _, err := loadStored(m.db, m.sealKey); err == nil {
			in.SecretAccessKey = prev.SecretAccessKey
		}
	}
	if err := in.Validate(); err != nil {
		return PublicConfig{}, err
	}
	if in.SecretAccessKey != "" && m.sealKey == nil {
		return PublicConfig{}, ErrNoSecretKey
	}
	if in.Enabled {
		if err := m.check(ctx, in); err != nil {
			return PublicConfig{}, err
		}
	}
	if err := saveStored(m.db, m.sealKey, in); err != nil {
		return PublicConfig{}, fmt.Errorf("save media storage settings: %w", err)
	}
	m.source, m.warning = SourcePanel, ""
	if err := m.applyLocked(in); err != nil {
		return PublicConfig{}, err
	}
	return in.Public(), nil
}

// Test checks a configuration without saving it. An empty secret falls back to the stored one.
func (m *Manager) Test(ctx context.Context, in Config) error {
	if in.SecretAccessKey == "" {
		m.mu.Lock()
		if m.source == SourceEnv {
			if cur := m.cur.Load(); cur != nil {
				in.SecretAccessKey = cur.cfg.SecretAccessKey
			}
		} else if prev, _, err := loadStored(m.db, m.sealKey); err == nil {
			in.SecretAccessKey = prev.SecretAccessKey
		}
		m.mu.Unlock()
	}
	in.Enabled = true // the test is about connecting, whatever the switch says
	if err := in.Validate(); err != nil {
		return err
	}
	return m.check(ctx, in)
}

func (m *Manager) check(ctx context.Context, cfg Config) error {
	store, err := m.newStore(cfg)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	return store.Check(ctx)
}

// Status is what the panel shows next to the form.
type Status struct {
	Config    PublicConfig   `json:"config"`
	Source    string         `json:"source"` // none | env | panel
	Active    bool           `json:"active"`
	Warning   string         `json:"warning,omitempty"`
	LastError string         `json:"last_error,omitempty"`
	Queue     map[string]int `json:"queue"`
	Workers   int            `json:"workers"`
}

// Status returns the current configuration (without the secret), where it came from and the queue depth.
func (m *Manager) Status() (Status, error) {
	m.mu.Lock()
	source, warning := m.source, m.warning
	m.mu.Unlock()

	var cfg Config
	if cur := m.cur.Load(); cur != nil {
		cfg = cur.cfg
	} else if source == SourceEnv {
		cfg, _ = configFromEnv()
	} else {
		cfg, _, _ = loadStored(m.db, m.sealKey)
	}
	queue, err := m.db.MediaQueueStats()
	if err != nil {
		return Status{}, err
	}
	m.lastErrMu.Lock()
	lastErr := m.lastErr
	m.lastErrMu.Unlock()
	return Status{
		Config:    cfg.Public(),
		Source:    source,
		Active:    m.Enabled(),
		Warning:   warning,
		LastError: lastErr,
		Queue:     queue,
		Workers:   m.workers,
	}, nil
}

func (m *Manager) setLastError(s string) {
	m.lastErrMu.Lock()
	m.lastErr = s
	m.lastErrMu.Unlock()
}

// signal wakes the dispatcher without blocking.
func (m *Manager) signal() {
	select {
	case m.wake <- struct{}{}:
	default:
	}
}

// Registered reports whether a message's media is already tracked. It implements whatsapp.MediaSink.
func (m *Manager) Registered(instanceJID, chatJID, messageID string) bool {
	return m.db.MediaRegistered(instanceJID, chatJID, messageID)
}

// HandleMedia registers a downloaded file and, with storage on, queues it for upload. It
// implements whatsapp.MediaSink. Registering never blocks on the upload itself.
func (m *Manager) HandleMedia(rec database.MediaRecord) {
	rec.Status = database.MediaLocal
	queued := m.Enabled()
	if queued {
		rec.Status = database.MediaPendingUpload
	}
	if err := m.db.RecordMedia(rec); err != nil {
		m.log.Warnf("media: register %s/%s: %v", rec.ChatJID, rec.MessageID, err)
		return
	}
	if queued {
		m.signal()
	}
}

// HandleMediaFailure records a download that did not work. It implements whatsapp.MediaSink.
func (m *Manager) HandleMediaFailure(rec database.MediaRecord, reason string) {
	if err := m.db.RecordMediaFailure(rec, reason); err != nil {
		m.log.Warnf("media: register failure %s/%s: %v", rec.ChatJID, rec.MessageID, err)
	}
}

// RetryFailed puts files that failed back in the queue.
func (m *Manager) RetryFailed() (int64, error) {
	if !m.Enabled() {
		return 0, errors.New("object storage is not enabled")
	}
	n, err := m.db.QueueLocalMedia(1<<30, true)
	if err == nil && n > 0 {
		m.signal()
	}
	return n, err
}

// backfillLocal queues files downloaded while storage was off.
func (m *Manager) backfillLocal() {
	total := int64(0)
	for m.ctx.Err() == nil && m.Enabled() {
		n, err := m.db.QueueLocalMedia(backfillChunk, false)
		if err != nil {
			m.log.Warnf("media: backfill: %v", err)
			return
		}
		if n == 0 {
			break
		}
		total += n
		m.signal()
	}
	if total > 0 {
		m.log.Infof("media: %d local files queued for upload", total)
	}
}

// run is the dispatcher loop.
func (m *Manager) run() {
	defer m.wg.Done()
	sem := make(chan struct{}, m.workers)
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		if a := m.cur.Load(); a != nil {
			for m.ctx.Err() == nil {
				batch, err := m.db.ClaimPendingUploads(m.workers)
				if err != nil {
					m.log.Warnf("media: claim uploads: %v", err)
					break
				}
				for _, rec := range batch {
					select {
					case sem <- struct{}{}:
					case <-m.ctx.Done():
						return
					}
					m.upWG.Add(1)
					go func() {
						defer m.upWG.Done()
						defer func() { <-sem }()
						m.upload(a, rec)
					}()
				}
				if len(batch) < m.workers {
					break
				}
			}
		}
		select {
		case <-m.ctx.Done():
			return
		case <-m.wake:
		case <-ticker.C:
		}
	}
}

// uploadTimeout allows a minimum of 2 minutes, plus time for the file at a modest 256 KiB/s.
func uploadTimeout(size int64) time.Duration {
	return 2*time.Minute + time.Duration(size/(256<<10))*time.Second
}

// upload sends one file and records the outcome.
func (m *Manager) upload(a *active, rec *database.MediaRecord) {
	fail := func(reason string, err error, permanent bool) {
		m.finishFailed(rec, reason, err, permanent)
	}

	f, err := os.Open(rec.LocalPath)
	if err != nil {
		fail("local file missing", err, true)
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		fail("local file unreadable", err, true)
		return
	}

	prefix, err := m.db.MediaFolderPrefix(rec.InstanceJID, rec.ChatJID, rec.MessageTime)
	if err != nil {
		fail("resolve folder", err, false)
		return
	}
	key := a.cfg.Prefix + database.MediaObjectKey(prefix, rec)
	contentType := rec.ContentType
	if contentType == "" {
		contentType = database.MediaContentType(rec.MediaType, rec.Filename)
	}

	ctx, cancel := context.WithTimeout(m.ctx, uploadTimeout(info.Size()))
	defer cancel()
	if err := a.store.Put(ctx, key, contentType, f, info.Size()); err != nil {
		if m.ctx.Err() != nil {
			return // shutting down: the row is requeued at the next start
		}
		fail("upload", err, false)
		return
	}

	if err := m.db.MarkMediaUploaded(rec, a.store.Bucket(), key, a.cfg.KeepLocal); err != nil {
		// The object is stored; only the bookkeeping failed. The row is retried and the same key
		// is overwritten, which is harmless.
		m.log.Warnf("media: uploaded %s but could not record it: %v", key, err)
		return
	}
	m.setLastError("")
	if !a.cfg.KeepLocal {
		if err := os.Remove(rec.LocalPath); err != nil && !os.IsNotExist(err) {
			m.log.Warnf("media: remove local copy %s: %v", rec.LocalPath, err)
		}
	}
	m.log.Debugf("media: uploaded %s (%d bytes)", key, info.Size())
}

// finishFailed records a failed attempt and schedules the retry.
func (m *Manager) finishFailed(rec *database.MediaRecord, what string, err error, permanent bool) {
	reason := fmt.Sprintf("%s: %v", what, err)
	m.log.Warnf("media: %s/%s: %s", rec.ChatJID, rec.MessageID, reason)

	attempts := rec.Attempts + 1
	var retryAt *time.Time
	switch {
	case permanent:
		// No retry: the file itself is gone.
	case isConfigError(err):
		// The settings are wrong, not the file: wait for the operator without using up attempts.
		attempts = rec.Attempts
		t := time.Now().Add(configErrorDelay)
		retryAt = &t
		m.setLastError(reason)
	case attempts < m.maxAttempts:
		t := time.Now().Add(backoff(attempts))
		retryAt = &t
	}
	if dbErr := m.db.MarkMediaUploadFailed(rec, reason, attempts, retryAt); dbErr != nil {
		m.log.Warnf("media: could not record failure of %s/%s: %v", rec.ChatJID, rec.MessageID, dbErr)
	}
}

// backoff doubles from 30 seconds up to an hour.
func backoff(attempt int) time.Duration {
	d := 30 * time.Second
	for i := 1; i < attempt && d < maxBackoff; i++ {
		d *= 2
	}
	if d > maxBackoff {
		d = maxBackoff
	}
	return d
}

// current returns the store for reading rec, or ErrNotStored.
func (m *Manager) current(rec *database.MediaRecord) (*active, error) {
	a := m.cur.Load()
	if a == nil || rec.ObjectKey == "" || (rec.Bucket != "" && rec.Bucket != a.store.Bucket()) {
		return nil, ErrNotStored
	}
	return a, nil
}

// Open reads an uploaded file from the bucket.
func (m *Manager) Open(ctx context.Context, rec *database.MediaRecord) (io.ReadCloser, error) {
	a, err := m.current(rec)
	if err != nil {
		return nil, err
	}
	return a.store.Get(ctx, rec.ObjectKey)
}

// URL returns a link to an uploaded file: the public address when the bucket is public, else a
// signed link that expires after ttl.
func (m *Manager) URL(ctx context.Context, rec *database.MediaRecord, ttl time.Duration) (string, error) {
	a, err := m.current(rec)
	if err != nil {
		return "", err
	}
	if a.cfg.PublicBaseURL != "" {
		return a.cfg.PublicBaseURL + "/" + rec.ObjectKey, nil
	}
	return a.store.Presign(ctx, rec.ObjectKey, ttl)
}
