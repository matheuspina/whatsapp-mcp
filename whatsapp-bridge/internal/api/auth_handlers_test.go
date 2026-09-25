package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"whatsapp-bridge/internal/auth"
)

func newAuthTestServer() *Server {
	return &Server{
		sessions:      auth.NewManager(time.Hour),
		webUIUsername: "admin",
		webUIPassword: "s3cret-pass",
		loginLimiter:  newLoginLimiter(),
	}
}

type apiEnvelope struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data"`
	Error   string          `json:"error"`
}

func doLogin(t *testing.T, s *Server, remote, user, pass string) (*httptest.ResponseRecorder, apiEnvelope) {
	t.Helper()
	body := `{"username":` + jsonStr(user) + `,"password":` + jsonStr(pass) + `}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(body))
	req.RemoteAddr = remote
	rec := httptest.NewRecorder()
	s.handleAuthLogin(rec, req)
	var env apiEnvelope
	_ = json.Unmarshal(rec.Body.Bytes(), &env)
	return rec, env
}

func jsonStr(v string) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func sessionCookie(rec *httptest.ResponseRecorder) *http.Cookie {
	for _, c := range rec.Result().Cookies() {
		if c.Name == sessionCookieName {
			return c
		}
	}
	return nil
}

// loginToken logs in and returns the session token taken from the cookie.
func loginToken(t *testing.T, s *Server) string {
	t.Helper()
	rec, env := doLogin(t, s, "192.0.2.10:5000", "admin", "s3cret-pass")
	if rec.Code != http.StatusOK || !env.Success {
		t.Fatalf("login failed: %d %s", rec.Code, rec.Body.String())
	}
	c := sessionCookie(rec)
	if c == nil || c.Value == "" {
		t.Fatalf("no session cookie set: %v", rec.Result().Header)
	}
	return c.Value
}

func withSession(r *http.Request, token string) {
	r.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
}

func TestLoginSetsHardenedCookieAndNoTokenInBody(t *testing.T) {
	s := newAuthTestServer()
	rec, _ := doLogin(t, s, "192.0.2.10:5000", "admin", "s3cret-pass")
	c := sessionCookie(rec)
	if c == nil {
		t.Fatal("no session cookie")
	}
	if !c.HttpOnly || c.SameSite != http.SameSiteStrictMode || c.Path != "/" {
		t.Errorf("cookie not hardened: %+v", c)
	}
	if strings.Contains(rec.Body.String(), c.Value) {
		t.Error("session token must not appear in the response body")
	}
	if _, ok := s.sessions.Validate(c.Value); !ok {
		t.Error("issued token should validate")
	}
}

func TestLoginRejectsBadCredentialsWithJSON(t *testing.T) {
	s := newAuthTestServer()
	for _, c := range [][2]string{{"admin", "wrong"}, {"root", "s3cret-pass"}, {"", ""}} {
		rec, env := doLogin(t, s, "192.0.2.11:5000", c[0], c[1])
		if rec.Code != http.StatusUnauthorized || env.Success || env.Error == "" {
			t.Errorf("creds %v: got %d %s", c, rec.Code, rec.Body.String())
		}
	}
}

func TestLoginNotConfigured(t *testing.T) {
	s := &Server{loginLimiter: newLoginLimiter()}
	rec, _ := doLogin(t, s, "192.0.2.12:5000", "admin", "x")
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("got %d, want 501", rec.Code)
	}

	s = newAuthTestServer()
	s.webUIPassword = ""
	rec, _ = doLogin(t, s, "192.0.2.12:5000", "admin", "")
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("empty password must not enable login, got %d", rec.Code)
	}
}

func TestLoginRejectsNonPost(t *testing.T) {
	s := newAuthTestServer()
	rec := httptest.NewRecorder()
	s.handleAuthLogin(rec, httptest.NewRequest(http.MethodGet, "/api/auth/login", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("got %d", rec.Code)
	}
}

func TestLoginRateLimitBlocksEvenCorrectPassword(t *testing.T) {
	s := newAuthTestServer()
	for i := 0; i < loginMaxFailures; i++ {
		if rec, _ := doLogin(t, s, "192.0.2.20:1", "admin", "bad"); rec.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: got %d", i, rec.Code)
		}
	}
	rec, _ := doLogin(t, s, "192.0.2.20:2", "admin", "s3cret-pass") // different port, same host
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") == "" {
		t.Fatalf("got %d retry-after=%q", rec.Code, rec.Header().Get("Retry-After"))
	}
	// A different host is unaffected.
	if rec, _ := doLogin(t, s, "192.0.2.21:1", "admin", "s3cret-pass"); rec.Code != http.StatusOK {
		t.Fatalf("other host got %d", rec.Code)
	}
}

func TestLoginRateLimitIgnoresXForwardedFor(t *testing.T) {
	s := newAuthTestServer()
	for i := 0; i < loginMaxFailures; i++ {
		body := `{"username":"admin","password":"bad"}`
		req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(body))
		req.RemoteAddr = "192.0.2.30:1"
		req.Header.Set("X-Forwarded-For", "203.0.113."+string(rune('1'+i)))
		s.handleAuthLogin(httptest.NewRecorder(), req)
	}
	rec, _ := doLogin(t, s, "192.0.2.30:1", "admin", "s3cret-pass")
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("spoofed X-Forwarded-For must not evade the limit, got %d", rec.Code)
	}
}

func TestLoginLimiterWindowExpires(t *testing.T) {
	l := newLoginLimiter()
	now := time.Now()
	l.now = func() time.Time { return now }
	for i := 0; i < loginMaxFailures; i++ {
		l.fail("h")
	}
	if b, _ := l.blocked("h"); !b {
		t.Fatal("should be blocked")
	}
	now = now.Add(loginWindow + time.Second)
	if b, _ := l.blocked("h"); b {
		t.Fatal("block should lapse after the window")
	}
}

func TestSuccessfulLoginResetsFailures(t *testing.T) {
	s := newAuthTestServer()
	for i := 0; i < loginMaxFailures-1; i++ {
		doLogin(t, s, "192.0.2.40:1", "admin", "bad")
	}
	if rec, _ := doLogin(t, s, "192.0.2.40:1", "admin", "s3cret-pass"); rec.Code != http.StatusOK {
		t.Fatalf("got %d", rec.Code)
	}
	if b, _ := s.loginLimiter.blocked("192.0.2.40"); b {
		t.Fatal("success should clear the failure count")
	}
}

func protectedStatus(s *Server, mutate func(*http.Request)) (int, apiEnvelope) {
	h := s.authMiddleware(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	req := httptest.NewRequest(http.MethodGet, "/api/anything", nil)
	req.RemoteAddr = "192.0.2.50:1"
	mutate(req)
	rec := httptest.NewRecorder()
	h(rec, req)
	var env apiEnvelope
	_ = json.Unmarshal(rec.Body.Bytes(), &env)
	return rec.Code, env
}

func TestMiddlewareAcceptsSessionOrAPIKey(t *testing.T) {
	t.Setenv("API_KEY", "machine-key")
	s := newAuthTestServer()
	tok := loginToken(t, s)

	if code, _ := protectedStatus(s, func(r *http.Request) { withSession(r, tok) }); code != http.StatusOK {
		t.Errorf("valid session: %d", code)
	}
	if code, _ := protectedStatus(s, func(r *http.Request) { r.Header.Set("X-API-Key", "machine-key") }); code != http.StatusOK {
		t.Errorf("valid api key: %d", code)
	}
}

func TestMiddlewareRejectsWithJSON401(t *testing.T) {
	t.Setenv("API_KEY", "machine-key")
	s := newAuthTestServer()

	cases := map[string]func(*http.Request){
		"no credentials": func(r *http.Request) {},
		"wrong api key":  func(r *http.Request) { r.Header.Set("X-API-Key", "nope") },
		"bad cookie":     func(r *http.Request) { withSession(r, "deadbeef") },
	}
	for name, mutate := range cases {
		code, env := protectedStatus(s, mutate)
		if code != http.StatusUnauthorized || env.Success || env.Error == "" {
			t.Errorf("%s: got %d %+v (want JSON 401)", name, code, env)
		}
	}
}

func TestRevokedSessionStopsWorking(t *testing.T) {
	t.Setenv("API_KEY", "machine-key")
	s := newAuthTestServer()
	tok := loginToken(t, s)

	req := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
	withSession(req, tok)
	rec := httptest.NewRecorder()
	s.handleAuthLogout(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("logout: %d", rec.Code)
	}
	if c := sessionCookie(rec); c == nil || c.MaxAge >= 0 {
		t.Fatalf("logout must expire the cookie, got %+v", c)
	}
	if code, _ := protectedStatus(s, func(r *http.Request) { withSession(r, tok) }); code != http.StatusUnauthorized {
		t.Fatalf("revoked token got %d", code)
	}
}

func TestSessionsListHidesTokenAndMarksCurrent(t *testing.T) {
	s := newAuthTestServer()
	tokA := loginToken(t, s)
	tokB := loginToken(t, s)

	req := httptest.NewRequest(http.MethodGet, "/api/auth/sessions", nil)
	withSession(req, tokA)
	rec := httptest.NewRecorder()
	s.handleAuthSessions(rec, req)

	raw := rec.Body.String()
	if strings.Contains(raw, tokA) || strings.Contains(raw, tokB) {
		t.Fatal("session list must never contain bearer tokens")
	}
	var env struct {
		Data []struct {
			ID      string `json:"id"`
			Current bool   `json:"current"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if len(env.Data) != 2 {
		t.Fatalf("want 2 sessions, got %d", len(env.Data))
	}
	current := 0
	for _, d := range env.Data {
		if d.Current {
			current++
		}
	}
	if current != 1 {
		t.Fatalf("exactly one session should be current, got %d", current)
	}
}

func TestRevokeSessionByID(t *testing.T) {
	s := newAuthTestServer()
	tok := loginToken(t, s)
	other := loginToken(t, s)
	sess, _ := s.sessions.Validate(other)

	req := httptest.NewRequest(http.MethodDelete, "/api/auth/sessions/"+sess.ID, nil)
	withSession(req, tok)
	rec := httptest.NewRecorder()
	s.handleAuthSessionByID(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("revoke: %d %s", rec.Code, rec.Body.String())
	}
	if _, ok := s.sessions.Validate(other); ok {
		t.Fatal("revoked session should be gone")
	}
	if _, ok := s.sessions.Validate(tok); !ok {
		t.Fatal("the caller's own session must survive")
	}

	rec = httptest.NewRecorder()
	s.handleAuthSessionByID(rec, httptest.NewRequest(http.MethodDelete, "/api/auth/sessions/unknown", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown id: %d", rec.Code)
	}
}

func TestSessionCookieRequiresAllowedOriginForStateChanges(t *testing.T) {
	t.Setenv("API_KEY", "machine-key")
	s := newAuthTestServer()
	tok := loginToken(t, s)

	run := func(method, origin string) int {
		h := s.authMiddleware(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
		req := httptest.NewRequest(method, "/api/send", nil)
		req.RemoteAddr = "192.0.2.60:1"
		withSession(req, tok)
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		rec := httptest.NewRecorder()
		h(rec, req)
		return rec.Code
	}

	if c := run(http.MethodGet, ""); c != http.StatusOK {
		t.Errorf("GET without Origin: %d", c)
	}
	if c := run(http.MethodPost, ""); c != http.StatusForbidden {
		t.Errorf("POST without Origin must be rejected, got %d", c)
	}
	if c := run(http.MethodPost, "http://evil.example"); c != http.StatusForbidden {
		t.Errorf("POST from foreign Origin must be rejected, got %d", c)
	}
	if c := run(http.MethodDelete, "http://127.0.0.1:8090"); c != http.StatusOK {
		t.Errorf("DELETE from the web UI Origin: %d", c)
	}
}

func TestAPIKeyIsNotSubjectToOriginCheck(t *testing.T) {
	t.Setenv("API_KEY", "machine-key")
	s := newAuthTestServer()
	h := s.authMiddleware(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	req := httptest.NewRequest(http.MethodPost, "/api/send", nil)
	req.RemoteAddr = "192.0.2.61:1"
	req.Header.Set("X-API-Key", "machine-key")
	rec := httptest.NewRecorder()
	h(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("MCP server (API key, no Origin) must keep working, got %d", rec.Code)
	}
}

func TestAuthMe(t *testing.T) {
	s := newAuthTestServer()
	tok := loginToken(t, s)

	req := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	withSession(req, tok)
	rec := httptest.NewRecorder()
	s.handleAuthMe(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"username":"admin"`) {
		t.Fatalf("me with session: %d %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	s.handleAuthMe(rec, httptest.NewRequest(http.MethodGet, "/api/auth/me", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("me without session: %d", rec.Code)
	}
}
