package api

import (
	"crypto/subtle"
	"encoding/json"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"whatsapp-bridge/internal/auth"
	"whatsapp-bridge/internal/security"
)

const (
	loginMaxFailures = 5
	loginWindow      = 5 * time.Minute
)

// loginLimiter throttles failed web UI logins per remote address. It keys on
// the TCP peer address (never X-Forwarded-For, which a client can spoof).
type loginLimiter struct {
	mu       sync.Mutex
	failures map[string][]time.Time
	now      func() time.Time
}

func newLoginLimiter() *loginLimiter {
	return &loginLimiter{failures: make(map[string][]time.Time), now: time.Now}
}

func peerHost(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// recent prunes and returns the failures still inside the window. Caller holds mu.
func (l *loginLimiter) recent(key string) []time.Time {
	cutoff := l.now().Add(-loginWindow)
	kept := l.failures[key][:0]
	for _, t := range l.failures[key] {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	if len(kept) == 0 {
		delete(l.failures, key)
		return nil
	}
	l.failures[key] = kept
	return kept
}

// blocked reports whether key has exhausted its attempts, and how long until it may retry.
func (l *loginLimiter) blocked(key string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	f := l.recent(key)
	if len(f) < loginMaxFailures {
		return false, 0
	}
	return true, f[0].Add(loginWindow).Sub(l.now())
}

func (l *loginLimiter) fail(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.recent(key)
	l.failures[key] = append(l.failures[key], l.now())
}

func (l *loginLimiter) reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.failures, key)
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// setSessionCookie stores the session token in an HttpOnly, SameSite=Strict
// cookie so page scripts never see it. When maxAge > 0, it sets a persistent
// cookie matching the session TTL so page reloads don't drop the session.
func setSessionCookie(w http.ResponseWriter, r *http.Request, token string, maxAge int) {
	cookie := &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https",
		SameSite: http.SameSiteStrictMode,
	}
	if maxAge > 0 {
		cookie.Expires = time.Now().Add(time.Duration(maxAge) * time.Second)
	} else if maxAge < 0 {
		cookie.Expires = time.Unix(1, 0)
	}
	http.SetCookie(w, cookie)
}

// handleAuthLogin exchanges the web UI username/password (WEB_UI_USERNAME /
// WEB_UI_PASSWORD) for a session, delivered as an HttpOnly cookie.
// POST /api/auth/login  {username, password}
// Response: { success, data: { username, expires_at } }
func (s *Server) handleAuthLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		SendJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.sessions == nil || s.webUIUsername == "" || s.webUIPassword == "" {
		SendJSONError(w, "Web UI login is not configured (set WEB_UI_USERNAME and WEB_UI_PASSWORD)", http.StatusNotImplemented)
		return
	}

	key := peerHost(r)
	if blocked, wait := s.loginLimiter.blocked(key); blocked {
		security.LogRateLimitExceeded(key)
		w.Header().Set("Retry-After", formatSeconds(wait))
		SendJSONError(w, "Too many failed login attempts. Try again later.", http.StatusTooManyRequests)
		return
	}

	var req loginRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
		SendJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Evaluate both comparisons before branching so timing doesn't reveal which field was wrong.
	userOK := subtle.ConstantTimeCompare([]byte(req.Username), []byte(s.webUIUsername))
	passOK := subtle.ConstantTimeCompare([]byte(req.Password), []byte(s.webUIPassword))
	if userOK&passOK != 1 {
		s.loginLimiter.fail(key)
		security.LogAuthFailure(clientIP(r), r.Header.Get("User-Agent"), "Invalid web UI credentials")
		SendJSONError(w, "Invalid username or password", http.StatusUnauthorized)
		return
	}

	s.loginLimiter.reset(key)
	sess, err := s.sessions.Create(s.webUIUsername, clientIP(r), r.Header.Get("User-Agent"))
	if err != nil {
		SendJSONError(w, "Failed to create session", http.StatusInternalServerError)
		return
	}
	security.LogAuthSuccess(clientIP(r), "/api/auth/login")

	maxAge := int(auth.DefaultSessionTTL.Seconds())
	if s.sessions != nil && s.sessions.TTL() > 0 {
		maxAge = int(s.sessions.TTL().Seconds())
	}
	setSessionCookie(w, r, sess.Token, maxAge)
	SendJSONSuccess(w, map[string]interface{}{
		"expires_at": sess.ExpiresAt,
		"username":   sess.Username,
	}, "Login successful")
}

// handleAuthLogout ends the caller's own session and clears the cookie.
// POST /api/auth/logout
func (s *Server) handleAuthLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		SendJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.sessions != nil {
		if token := sessionToken(r); token != "" {
			s.sessions.Revoke(token)
		}
	}
	setSessionCookie(w, r, "", -1)
	SendJSONSuccess(w, nil, "Logged out")
}

// handleAuthMe reports who the current session belongs to, so the web UI can
// tell whether it is logged in without holding the token itself.
// GET /api/auth/me
func (s *Server) handleAuthMe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		SendJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.sessions != nil {
		if sess, ok := s.sessions.Validate(sessionToken(r)); ok {
			SendJSONSuccess(w, map[string]interface{}{
				"username":   sess.Username,
				"expires_at": sess.ExpiresAt,
			}, "")
			return
		}
	}
	SendJSONError(w, "Unauthorized", http.StatusUnauthorized)
}

// handleAuthSessions lists the active web UI sessions. The secret token is never included.
// GET /api/auth/sessions
func (s *Server) handleAuthSessions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		SendJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.sessions == nil {
		SendJSONSuccess(w, []interface{}{}, "")
		return
	}

	var currentID string
	if cur, ok := s.sessions.Validate(sessionToken(r)); ok {
		currentID = cur.ID
	}

	list := s.sessions.List()
	out := make([]map[string]interface{}, 0, len(list))
	for _, sess := range list {
		out = append(out, map[string]interface{}{
			"id":           sess.ID,
			"username":     sess.Username,
			"created_at":   sess.CreatedAt,
			"last_seen_at": sess.LastSeenAt,
			"expires_at":   sess.ExpiresAt,
			"ip":           sess.IP,
			"user_agent":   sess.UserAgent,
			"current":      sess.ID == currentID,
		})
	}
	SendJSONSuccess(w, out, "")
}

// handleAuthSessionByID revokes one session (remote logout).
// DELETE /api/auth/sessions/{id}
func (s *Server) handleAuthSessionByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		SendJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/auth/sessions/"), "/")
	if id == "" {
		SendJSONError(w, "Session ID is required", http.StatusBadRequest)
		return
	}
	if s.sessions == nil || !s.sessions.RevokeByID(id) {
		SendJSONError(w, "Session not found", http.StatusNotFound)
		return
	}
	SendJSONSuccess(w, nil, "Session revoked")
}

func formatSeconds(d time.Duration) string {
	secs := int(d.Seconds()) + 1
	if secs < 1 {
		secs = 1
	}
	return strconv.Itoa(secs)
}
