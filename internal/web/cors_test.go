package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// ── allowedOrigins ──────────────────────────────────

func TestAllowedOrigins_StandardPort(t *testing.T) {
	t.Parallel()
	origins := allowedOrigins("8080")
	want := []string{"http://localhost:8080", "http://127.0.0.1:8080", "http://localhost:5173", "http://127.0.0.1:5173"}
	if len(origins) != len(want) {
		t.Fatalf("got %d origins, want %d", len(origins), len(want))
	}
	for i, w := range want {
		if origins[i] != w {
			t.Fatalf("origins[%d]=%q want=%q", i, origins[i], w)
		}
	}
}

func TestAllowedOrigins_CustomPort(t *testing.T) {
	t.Parallel()
	origins := allowedOrigins("3000")
	if origins[0] != "http://localhost:3000" || origins[1] != "http://127.0.0.1:3000" {
		t.Fatalf("unexpected origins: %v", origins)
	}
}

func TestAllowedOrigins_Port80OmitsPort(t *testing.T) {
	t.Parallel()
	origins := allowedOrigins("80")
	if origins[0] != "http://localhost" || origins[1] != "http://127.0.0.1" {
		t.Fatalf("unexpected origins for port 80: %v", origins)
	}
}

// ── originAllowed ───────────────────────────────────

func TestOriginAllowed_MatchingOrigins(t *testing.T) {
	t.Parallel()
	allowed := []string{"http://localhost:8080", "http://127.0.0.1:8080"}

	if !originAllowed("http://localhost:8080", allowed) {
		t.Fatal("localhost should be allowed")
	}
	if !originAllowed("http://127.0.0.1:8080", allowed) {
		t.Fatal("127.0.0.1 should be allowed")
	}
}

func TestOriginAllowed_CaseInsensitive(t *testing.T) {
	t.Parallel()
	allowed := []string{"http://localhost:8080"}
	if !originAllowed("HTTP://LOCALHOST:8080", allowed) {
		t.Fatal("case-insensitive match should be allowed")
	}
}

func TestOriginAllowed_RejectsEvilOrigin(t *testing.T) {
	t.Parallel()
	allowed := []string{"http://localhost:8080", "http://127.0.0.1:8080"}

	if originAllowed("http://evil.com", allowed) {
		t.Fatal("evil origin should be rejected")
	}
	if originAllowed("http://localhost:9090", allowed) {
		t.Fatal("wrong port should be rejected")
	}
	if originAllowed("http://192.168.1.5:8080", allowed) {
		t.Fatal("LAN IP should be rejected")
	}
}

// ── checkOrigin (static + dynamic Host check) ───────

func TestCheckOrigin_NoOriginHeader_Allowed(t *testing.T) {
	t.Parallel()
	req := httptest.NewRequest(http.MethodPost, "/rpc", nil)
	origin, ok := checkOrigin(req, []string{"http://localhost:8080"})
	if !ok {
		t.Fatal("no Origin should be allowed")
	}
	if origin != "" {
		t.Fatalf("origin should be empty, got=%q", origin)
	}
}

func TestCheckOrigin_StaticAllowlist_Localhost(t *testing.T) {
	t.Parallel()
	req := httptest.NewRequest(http.MethodPost, "/rpc", nil)
	req.Header.Set("Origin", "http://localhost:8080")
	origin, ok := checkOrigin(req, []string{"http://localhost:8080"})
	if !ok {
		t.Fatal("static-allowed origin should pass")
	}
	if origin != "http://localhost:8080" {
		t.Fatalf("origin=%q want=http://localhost:8080", origin)
	}
}

func TestCheckOrigin_DynamicHostMatch_RemoteServer(t *testing.T) {
	t.Parallel()
	// Simulates: SPA served from http://remote-server:8080, fetch to /rpc.
	req := httptest.NewRequest(http.MethodPost, "http://remote-server:8080/rpc", nil)
	req.Host = "remote-server:8080"
	req.Header.Set("Origin", "http://remote-server:8080")

	origin, ok := checkOrigin(req, []string{"http://localhost:8080"})
	if !ok {
		t.Fatal("dynamic Host-matching origin should be allowed for remote access")
	}
	if origin != "http://remote-server:8080" {
		t.Fatalf("origin=%q want=http://remote-server:8080", origin)
	}
}

func TestCheckOrigin_DynamicHostMismatch_CrossOriginAttack(t *testing.T) {
	t.Parallel()
	// Simulates: evil.com fetching http://remote-server:8080/rpc
	req := httptest.NewRequest(http.MethodPost, "http://remote-server:8080/rpc", nil)
	req.Host = "remote-server:8080"
	req.Header.Set("Origin", "http://evil.com")

	_, ok := checkOrigin(req, []string{"http://localhost:8080"})
	if ok {
		t.Fatal("cross-origin attack should be blocked even for remote server")
	}
}

func TestCheckOrigin_DynamicHostMismatch_LocalhostAttack(t *testing.T) {
	t.Parallel()
	// Simulates: evil.com fetching http://localhost:8080/rpc (the original vuln)
	req := httptest.NewRequest(http.MethodPost, "http://localhost:8080/rpc", nil)
	req.Host = "localhost:8080"
	req.Header.Set("Origin", "http://evil.com")

	_, ok := checkOrigin(req, []string{"http://localhost:8080"})
	if ok {
		t.Fatal("cross-origin attack against localhost should be blocked")
	}
}

// ── originGuard middleware ───────────────────────────

func TestOriginGuard_BlocksCrossOriginRequest(t *testing.T) {
	t.Parallel()
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := originGuard([]string{"http://localhost:8080"}, inner)

	req := httptest.NewRequest(http.MethodPost, "/rpc", nil)
	req.Header.Set("Origin", "http://evil.com")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d want=%d", rec.Code, http.StatusForbidden)
	}
}

func TestOriginGuard_AllowsSameOriginWithHeader(t *testing.T) {
	t.Parallel()
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := originGuard([]string{"http://localhost:8080"}, inner)

	req := httptest.NewRequest(http.MethodPost, "/rpc", nil)
	req.Header.Set("Origin", "http://localhost:8080")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want=%d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:8080" {
		t.Fatalf("ACAO=%q want=http://localhost:8080", got)
	}
}

func TestOriginGuard_AllowsRequestWithNoOriginHeader(t *testing.T) {
	t.Parallel()
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := originGuard([]string{"http://localhost:8080"}, inner)

	req := httptest.NewRequest(http.MethodPost, "/rpc", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want=%d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("ACAO should be empty for no-origin request, got=%q", got)
	}
}

func TestOriginGuard_AllowsRemoteAccessViaDynamicHost(t *testing.T) {
	t.Parallel()
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	// Static list only has localhost, but remote access should work via Host match.
	handler := originGuard([]string{"http://localhost:8080"}, inner)

	req := httptest.NewRequest(http.MethodPost, "http://my-server:8080/rpc", nil)
	req.Host = "my-server:8080"
	req.Header.Set("Origin", "http://my-server:8080")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want=%d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://my-server:8080" {
		t.Fatalf("ACAO=%q want=http://my-server:8080", got)
	}
}

func TestOriginGuard_NeverReturnsWildcard(t *testing.T) {
	t.Parallel()
	allowed := []string{"http://localhost:8080", "http://127.0.0.1:8080"}
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := originGuard(allowed, inner)

	for _, origin := range []string{
		"http://localhost:8080",
		"http://127.0.0.1:8080",
		"http://evil.com",
		"",
	} {
		req := httptest.NewRequest(http.MethodPost, "/rpc", nil)
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if got := rec.Header().Get("Access-Control-Allow-Origin"); got == "*" {
			t.Fatalf("origin=%q: ACAO must never be wildcard '*'", origin)
		}
	}
}

// ── corsPreflightHandler ────────────────────────────

func TestCORSPreflight_RejectsEvilOrigin(t *testing.T) {
	t.Parallel()
	handler := corsPreflightHandler([]string{"http://localhost:8080"})

	req := httptest.NewRequest(http.MethodOptions, "/rpc", nil)
	req.Header.Set("Origin", "http://evil.com")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d want=%d", rec.Code, http.StatusForbidden)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("ACAO should be empty for blocked origin, got=%q", got)
	}
}

func TestCORSPreflight_AllowsMatchingOrigin(t *testing.T) {
	t.Parallel()
	handler := corsPreflightHandler([]string{"http://localhost:8080"})

	req := httptest.NewRequest(http.MethodOptions, "/rpc", nil)
	req.Header.Set("Origin", "http://localhost:8080")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status=%d want=%d", rec.Code, http.StatusNoContent)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:8080" {
		t.Fatalf("ACAO=%q want=http://localhost:8080", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Methods"); got == "" {
		t.Fatal("Access-Control-Allow-Methods should be set")
	}
	if got := rec.Header().Get("Access-Control-Allow-Headers"); got == "" {
		t.Fatal("Access-Control-Allow-Headers should be set")
	}
}

func TestCORSPreflight_AllowsRemoteOriginViaDynamicHost(t *testing.T) {
	t.Parallel()
	handler := corsPreflightHandler([]string{"http://localhost:8080"})

	req := httptest.NewRequest(http.MethodOptions, "http://remote-server:8080/rpc", nil)
	req.Host = "remote-server:8080"
	req.Header.Set("Origin", "http://remote-server:8080")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status=%d want=%d", rec.Code, http.StatusNoContent)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://remote-server:8080" {
		t.Fatalf("ACAO=%q want=http://remote-server:8080", got)
	}
}

func TestCORSPreflight_NeverReturnsWildcard(t *testing.T) {
	t.Parallel()
	handler := corsPreflightHandler([]string{"http://localhost:8080"})

	for _, origin := range []string{"http://localhost:8080", "http://evil.com", ""} {
		req := httptest.NewRequest(http.MethodOptions, "/rpc", nil)
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if got := rec.Header().Get("Access-Control-Allow-Origin"); got == "*" {
			t.Fatalf("origin=%q: preflight ACAO must never be wildcard '*'", origin)
		}
	}
}
