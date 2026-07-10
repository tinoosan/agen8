package daemon

import (
	"crypto/tls"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/tinoosan/agen8/internal/config"
	"github.com/tinoosan/agen8/internal/rpc"
)

func TestRPCRequiresIdentityForEveryProtectedService(t *testing.T) {
	handler := newSecurityTestHandler(t)
	protectedMethods := []string{
		"project.list",
		"files.listDir",
		"credential.list",
		"location.list",
		"graph.search",
		"pin.list",
	}

	for _, method := range protectedMethods {
		t.Run(method, func(t *testing.T) {
			recorder := callRPCForSecurityTest(handler, method, `{}`, "")
			if recorder.Code != http.StatusUnauthorized {
				t.Fatalf("status=%d want %d body=%s", recorder.Code, http.StatusUnauthorized, recorder.Body.String())
			}
		})
	}
}

func TestRPCRejectsInvalidBearerForProtectedAndStatusMethods(t *testing.T) {
	handler := newSecurityTestHandler(t)
	for _, method := range []string{"project.list", "auth.status"} {
		recorder := callRPCForSecurityTest(handler, method, `{}`, "Bearer invalid")
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("method=%s status=%d want %d", method, recorder.Code, http.StatusUnauthorized)
		}
	}
}

func TestRPCAnonymousAllowlistIsExplicit(t *testing.T) {
	handler := newSecurityTestHandler(t)

	statusRecorder := callRPCForSecurityTest(handler, "auth.status", `{}`, "")
	if statusRecorder.Code != http.StatusOK {
		t.Fatalf("auth.status status=%d body=%s", statusRecorder.Code, statusRecorder.Body.String())
	}
	var statusResponse rpc.Response
	if err := json.Unmarshal(statusRecorder.Body.Bytes(), &statusResponse); err != nil {
		t.Fatalf("decode auth.status: %v", err)
	}
	if string(statusResponse.Result) != `{"authenticated":false}` {
		t.Fatalf("auth.status result=%s", statusResponse.Result)
	}

	loginRecorder := callRPCForSecurityTest(handler, "auth.login", `{"email":"missing@example.com","password":"wrong"}`, "")
	if loginRecorder.Code != http.StatusOK {
		t.Fatalf("auth.login status=%d body=%s", loginRecorder.Code, loginRecorder.Body.String())
	}
}

func TestSetupOpenDoesNotRedirectOrExposeToken(t *testing.T) {
	handler := newSecurityTestHandler(t)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("Accept", "text/html")
	handler.ServeHTTP(recorder, request)

	if recorder.Code == http.StatusFound {
		t.Fatalf("unexpected setup redirect location=%q", recorder.Header().Get("Location"))
	}
	if strings.Contains(recorder.Body.String(), "test-setup-token") || strings.Contains(recorder.Header().Get("Location"), "test-setup-token") {
		t.Fatal("public root response disclosed setup token")
	}
}

func TestSecurityHeadersProtectBrowserResponses(t *testing.T) {
	handler := newSecurityTestHandler(t)
	request := httptest.NewRequest(http.MethodPost, "/rpc", strings.NewReader(`{"jsonrpc":"2.0","id":"1","method":"auth.status","params":{}}`))
	request.TLS = &tls.ConnectionState{}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	wants := map[string]string{
		"Cache-Control":             "no-store",
		"Referrer-Policy":           "no-referrer",
		"Strict-Transport-Security": "max-age=31536000; includeSubDomains",
		"X-Content-Type-Options":    "nosniff",
		"X-Frame-Options":           "DENY",
	}
	for header, want := range wants {
		if got := recorder.Header().Get(header); got != want {
			t.Errorf("%s=%q want %q", header, got, want)
		}
	}
	if got := recorder.Header().Get("Content-Security-Policy"); !strings.Contains(got, "frame-ancestors 'none'") {
		t.Errorf("Content-Security-Policy=%q", got)
	}
}

func TestLoginThrottlingResetsAfterSuccess(t *testing.T) {
	handler := newSecurityTestHandler(t)
	sessionToken := setupSessionForEventsTest(t, handler)

	for attempt := 0; attempt < loginFailureLimit-1; attempt++ {
		assertLoginHTTPStatus(t, handler, "wrong", http.StatusOK)
	}
	assertLoginHTTPStatus(t, handler, "password123", http.StatusOK)

	for attempt := 0; attempt < loginFailureLimit; attempt++ {
		assertLoginHTTPStatus(t, handler, "wrong", http.StatusOK)
	}
	blocked := callRPCForSecurityTest(handler, "auth.login", loginParams("wrong"), "")
	if blocked.Code != http.StatusTooManyRequests {
		t.Fatalf("blocked status=%d want %d body=%s", blocked.Code, http.StatusTooManyRequests, blocked.Body.String())
	}
	if seconds, err := strconv.Atoi(blocked.Header().Get("Retry-After")); err != nil || seconds <= 0 {
		t.Fatalf("Retry-After=%q want positive seconds", blocked.Header().Get("Retry-After"))
	}

	authenticated := callRPCForSecurityTest(handler, "project.list", `{}`, "Bearer "+sessionToken)
	if authenticated.Code != http.StatusOK {
		t.Fatalf("authenticated project.list status=%d body=%s", authenticated.Code, authenticated.Body.String())
	}
}

func newSecurityTestHandler(t *testing.T) http.Handler {
	t.Helper()
	daemon, err := New(Config{
		AppConfig:  config.Config{DataDir: t.TempDir()},
		SetupToken: "test-setup-token",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	handler, err := daemon.httpHandler()
	if err != nil {
		t.Fatalf("httpHandler: %v", err)
	}
	return handler
}

func callRPCForSecurityTest(handler http.Handler, method, params, authorization string) *httptest.ResponseRecorder {
	body := `{"jsonrpc":"2.0","id":"1","method":` + strconv.Quote(method) + `,"params":` + params + `}`
	request := httptest.NewRequest(http.MethodPost, "/rpc", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

func assertLoginHTTPStatus(t *testing.T, handler http.Handler, password string, want int) {
	t.Helper()
	recorder := callRPCForSecurityTest(handler, "auth.login", loginParams(password), "")
	if recorder.Code != want {
		t.Fatalf("login status=%d want %d body=%s", recorder.Code, want, recorder.Body.String())
	}
}

func loginParams(password string) string {
	return `{"email":"admin@example.com","password":` + strconv.Quote(password) + `}`
}
