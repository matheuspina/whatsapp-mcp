package api

import (
	"crypto/subtle"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"whatsapp-bridge/internal/security"
)

// Rate limiter state
var (
	rateLimitMu     sync.Mutex
	requestCounts   = make(map[string]int)
	requestWindows  = make(map[string]time.Time)
	rateLimit       = 100 // requests per window
	rateLimitWindow = time.Minute
)

// getAllowedOrigins returns the list of allowed CORS origins
func getAllowedOrigins() map[string]bool {
	origins := map[string]bool{
		"http://localhost:8089": true, // Webhook UI (localhost)
		"http://localhost:8090": true, // Pairing UI (localhost)
		"http://127.0.0.1:8089": true, // Webhook UI (IP — browsers use IP when accessed via 127.0.0.1)
		"http://127.0.0.1:8090": true, // Pairing UI (IP)
	}

	// Allow additional origins from env var (comma-separated)
	if extra := os.Getenv("CORS_ORIGINS"); extra != "" {
		for _, origin := range strings.Split(extra, ",") {
			origins[strings.TrimSpace(origin)] = true
		}
	}

	return origins
}

// sessionCookieName is the HttpOnly cookie that carries the web UI session token.
const sessionCookieName = "wa_session"

// sessionToken returns the web UI session token from the session cookie.
func sessionToken(r *http.Request) string {
	if c, err := r.Cookie(sessionCookieName); err == nil {
		return c.Value
	}
	return ""
}

// isSafeMethod reports whether the method cannot change state.
func isSafeMethod(m string) bool {
	return m == http.MethodGet || m == http.MethodHead || m == http.MethodOptions
}

// originAllowed reports whether the request's Origin is on the CORS allowlist.
func originAllowed(r *http.Request) bool {
	return getAllowedOrigins()[r.Header.Get("Origin")]
}

// clientIP returns the caller address used for audit logs. It honours
// X-Forwarded-For, so it must not be used for security decisions.
func clientIP(r *http.Request) string {
	ip := r.RemoteAddr
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		ip = strings.Split(forwarded, ",")[0]
	}
	return ip
}

// authMiddleware authorizes a request with either a valid web UI session
// cookie or the shared API key (X-API-Key, used by the MCP server and
// scripts). Key comparison is constant-time.
//
// Cookies are sent by the browser automatically, so a session-authenticated
// request that changes state must also come from an allowlisted Origin
// (CSRF defence on top of SameSite=Strict).
func (s *Server) authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ip := clientIP(r)

		if s.sessions != nil {
			if token := sessionToken(r); token != "" {
				if sess, ok := s.sessions.Validate(token); ok {
					if !isSafeMethod(r.Method) && !originAllowed(r) {
						security.LogAuthFailure(ip, r.Header.Get("User-Agent"), "Session request from disallowed Origin")
						SendJSONError(w, "Forbidden", http.StatusForbidden)
						return
					}
					security.LogAuthSuccess(ip, r.URL.Path)
					next(w, withActor(r, "panel:"+sess.Username))
					return
				}
			}
		}

		expectedKey := os.Getenv("API_KEY")

		// Skip auth if no API_KEY is configured (dev mode)
		if expectedKey == "" {
			next(w, withActor(r, apiActor(r)))
			return
		}

		apiKey := r.Header.Get("X-API-Key")
		if subtle.ConstantTimeCompare([]byte(apiKey), []byte(expectedKey)) != 1 {
			security.LogAuthFailure(ip, r.Header.Get("User-Agent"), "Invalid API key or session")
			SendJSONError(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		security.LogAuthSuccess(ip, r.URL.Path)
		next(w, withActor(r, apiActor(r)))
	}
}

// apiActor names an API-key caller for audit trails: the MCP server sends the OAuth client it
// serves in X-Actor, anything else is just "api-key".
func apiActor(r *http.Request) string {
	if a := strings.TrimSpace(r.Header.Get("X-Actor")); a != "" {
		if len(a) > 120 {
			a = a[:120]
		}
		return "api:" + a
	}
	return "api-key"
}

// RateLimitMiddleware limits requests per IP address
func RateLimitMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Get client IP
		ip := r.RemoteAddr
		if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
			ip = strings.Split(forwarded, ",")[0]
		}

		rateLimitMu.Lock()
		now := time.Now()

		// Reset window if expired
		if window, exists := requestWindows[ip]; !exists || now.Sub(window) > rateLimitWindow {
			requestWindows[ip] = now
			requestCounts[ip] = 0
		}

		requestCounts[ip]++
		count := requestCounts[ip]
		rateLimitMu.Unlock()

		if count > rateLimit {
			security.LogRateLimitExceeded(ip)
			w.Header().Set("Retry-After", "60")
			http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
			return
		}

		next(w, r)
	}
}

// CorsMiddleware adds CORS headers with restricted origins
func CorsMiddleware(next http.HandlerFunc) http.HandlerFunc {
	allowedOrigins := getAllowedOrigins()

	return func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")

		// Check if origin is allowed
		if allowedOrigins[origin] {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
		}
		// If origin not allowed, don't set Access-Control-Allow-Origin (browser blocks)

		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-API-Key")
		w.Header().Set("Access-Control-Max-Age", "86400")

		// Handle preflight requests
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		next(w, r)
	}
}

// SecurityHeadersMiddleware adds security headers to all responses
func SecurityHeadersMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Prevent MIME type sniffing
		w.Header().Set("X-Content-Type-Options", "nosniff")
		// Prevent clickjacking
		w.Header().Set("X-Frame-Options", "DENY")
		// XSS protection (legacy but still useful)
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		// Referrer policy
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		// Content Security Policy for API
		w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		// Permissions policy
		w.Header().Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")

		next(w, r)
	}
}

// SecureMiddleware chains security headers, auth, rate limiting, and CORS middleware
func (s *Server) SecureMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return SecurityHeadersMiddleware(CorsMiddleware(RateLimitMiddleware(s.authMiddleware(next))))
}

// PublicMiddleware is SecureMiddleware without authentication, for endpoints
// that must be reachable before login (the login endpoint itself).
func (s *Server) PublicMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return SecurityHeadersMiddleware(CorsMiddleware(RateLimitMiddleware(next)))
}
