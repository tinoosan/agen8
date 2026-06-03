package web

import (
	"net/http"
	"strings"
)

// DefaultWebHost is the default bind host for the web server.
// Binding to loopback prevents LAN-level access by default.
const DefaultWebHost = "127.0.0.1"

// allowedOrigins returns the set of static permitted CORS origins for the given
// port. These cover the common localhost access patterns.
func allowedOrigins(port string) []string {
	if port == "" || port == "80" {
		return []string{"http://localhost", "http://127.0.0.1"}
	}
	return []string{
		"http://localhost:" + port,
		"http://127.0.0.1:" + port,
		// Vite dev server runs on 5173 and proxies to the daemon.
		// Allow it so make dev (daemon + Vite) works without CORS errors.
		"http://localhost:5173",
		"http://127.0.0.1:5173",
	}
}

// originAllowed checks whether origin is in the allowed set (case-insensitive).
func originAllowed(origin string, allowed []string) bool {
	for _, a := range allowed {
		if strings.EqualFold(origin, a) {
			return true
		}
	}
	return false
}

// checkOrigin validates a request's Origin header. Returns the validated origin
// and true if the request should be allowed. The check passes when:
//   - No Origin header is present (same-origin browser or non-browser client)
//   - Origin matches the static allowed list (localhost variants)
//   - Origin matches scheme://Host of the request (enables remote access)
func checkOrigin(r *http.Request, staticAllowed []string) (string, bool) {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return "", true
	}
	// Static allowlist (localhost variants).
	if originAllowed(origin, staticAllowed) {
		return origin, true
	}
	// Dynamic: Origin must match the request's own scheme://Host.
	// Browsers set Host to the target URL and Origin to the source page's
	// origin. Same-origin requests have Origin == scheme://Host.
	// Cross-origin attacks will have a mismatched Origin.
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if r.Host != "" && strings.EqualFold(origin, scheme+"://"+r.Host) {
		return origin, true
	}
	return "", false
}

// originGuard wraps an http.Handler and rejects requests whose Origin header
// fails the checkOrigin validation. Allowed responses get the matching
// Access-Control-Allow-Origin header (never a wildcard).
func originGuard(allowed []string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin, ok := checkOrigin(r, allowed)
		if !ok {
			http.Error(w, "cross-origin request blocked", http.StatusForbidden)
			return
		}
		if origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
		}
		next.ServeHTTP(w, r)
	})
}

// corsPreflightHandler returns an OPTIONS handler that validates the Origin
// header and returns CORS headers only for allowed origins.
func corsPreflightHandler(allowed []string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		origin, ok := checkOrigin(r, allowed)
		if !ok || origin == "" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Vary", "Origin")
		w.WriteHeader(http.StatusNoContent)
	}
}
