// Package auth provides server-side session management for the web UI login.
//
// Sessions live in memory in the bridge process: they are not persisted, so a
// bridge restart logs everyone out. The bearer token is the only credential;
// the short public ID is safe to display and is used to list/revoke sessions.
package auth

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sort"
	"sync"
	"time"
)

// DefaultSessionTTL is how long a session stays valid without being used.
const DefaultSessionTTL = 24 * time.Hour

// Session describes one authenticated web UI login.
type Session struct {
	Token      string // secret bearer credential — never returned by List
	ID         string // short public identifier, safe to display and log
	Username   string
	CreatedAt  time.Time
	LastSeenAt time.Time
	ExpiresAt  time.Time
	IP         string
	UserAgent  string
}

// Manager keeps the set of active sessions.
type Manager struct {
	mu       sync.RWMutex
	byToken  map[string]*Session
	tokenOf  map[string]string // public ID -> token
	ttl      time.Duration
	now      func() time.Time
	stopOnce sync.Once
	stopCh   chan struct{}
}

// NewManager creates a session manager. A non-positive ttl uses DefaultSessionTTL.
// Sessions use a sliding expiry: every successful Validate pushes ExpiresAt out by ttl.
func NewManager(ttl time.Duration) *Manager {
	if ttl <= 0 {
		ttl = DefaultSessionTTL
	}
	return &Manager{
		byToken: make(map[string]*Session),
		tokenOf: make(map[string]string),
		ttl:     ttl,
		now:     time.Now,
		stopCh:  make(chan struct{}),
	}
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate random bytes: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// Create starts a new session for username and returns it (including the secret token).
func (m *Manager) Create(username, ip, userAgent string) (*Session, error) {
	token, err := randomHex(32)
	if err != nil {
		return nil, err
	}
	id, err := randomHex(6)
	if err != nil {
		return nil, err
	}

	now := m.now()
	s := &Session{
		Token:      token,
		ID:         id,
		Username:   username,
		CreatedAt:  now,
		LastSeenAt: now,
		ExpiresAt:  now.Add(m.ttl),
		IP:         ip,
		UserAgent:  userAgent,
	}

	m.mu.Lock()
	m.byToken[token] = s
	m.tokenOf[id] = token
	m.mu.Unlock()
	return s, nil
}

// Validate returns the session for token if it exists and has not expired,
// extending its expiry. Expired sessions are removed.
func (m *Manager) Validate(token string) (*Session, bool) {
	if token == "" {
		return nil, false
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	s, ok := m.byToken[token]
	if !ok {
		return nil, false
	}
	now := m.now()
	if !now.Before(s.ExpiresAt) {
		m.removeLocked(s)
		return nil, false
	}
	s.LastSeenAt = now
	s.ExpiresAt = now.Add(m.ttl)
	cp := *s
	return &cp, true
}

// Revoke ends the session identified by its secret token.
func (m *Manager) Revoke(token string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.byToken[token]
	if !ok {
		return false
	}
	m.removeLocked(s)
	return true
}

// RevokeByID ends the session identified by its public ID.
func (m *Manager) RevokeByID(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	token, ok := m.tokenOf[id]
	if !ok {
		return false
	}
	m.removeLocked(m.byToken[token])
	return true
}

// List returns the active (non-expired) sessions, newest first, with the
// secret Token blanked out.
func (m *Manager) List() []Session {
	m.mu.RLock()
	defer m.mu.RUnlock()

	now := m.now()
	out := make([]Session, 0, len(m.byToken))
	for _, s := range m.byToken {
		if !now.Before(s.ExpiresAt) {
			continue
		}
		cp := *s
		cp.Token = ""
		out = append(out, cp)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

func (m *Manager) removeLocked(s *Session) {
	delete(m.byToken, s.Token)
	delete(m.tokenOf, s.ID)
}

// purgeExpired drops every expired session.
func (m *Manager) purgeExpired() {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.now()
	for _, s := range m.byToken {
		if !now.Before(s.ExpiresAt) {
			m.removeLocked(s)
		}
	}
}

// StartCleanup runs a background goroutine that purges expired sessions every
// interval until Stop is called.
func (m *Manager) StartCleanup(interval time.Duration) {
	if interval <= 0 {
		interval = time.Minute
	}
	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				m.purgeExpired()
			case <-m.stopCh:
				return
			}
		}
	}()
}

// Stop halts the cleanup goroutine. Safe to call more than once.
func (m *Manager) Stop() {
	m.stopOnce.Do(func() { close(m.stopCh) })
}
