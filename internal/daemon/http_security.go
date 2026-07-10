package daemon

import (
	"net/http"
	"strings"
)

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headers := w.Header()
		headers.Set("Content-Security-Policy", "default-src 'self'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'; img-src 'self' data:; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; connect-src 'self'")
		headers.Set("Permissions-Policy", "camera=(), geolocation=(), microphone=()")
		headers.Set("Referrer-Policy", "no-referrer")
		headers.Set("X-Content-Type-Options", "nosniff")
		headers.Set("X-Frame-Options", "DENY")
		if requestUsesHTTPS(r) {
			headers.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		if containsSensitiveResponse(r.URL.Path) {
			headers.Set("Cache-Control", "no-store")
		}
		next.ServeHTTP(w, r)
	})
}

func requestUsesHTTPS(r *http.Request) bool {
	return r != nil && (r.TLS != nil || strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")), "https"))
}

func containsSensitiveResponse(path string) bool {
	switch strings.TrimSpace(path) {
	case "/rpc", "/setup", "/uploads/files":
		return true
	default:
		return false
	}
}
