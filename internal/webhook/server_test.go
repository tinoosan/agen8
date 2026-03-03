package webhook

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestServer_HandleTask_BuildTaskNil(t *testing.T) {
	srv := NewServer(ServerConfig{
		Addr:      "127.0.0.1:0",
		BuildTask: nil,
	})
	req := httptest.NewRequest(http.MethodPost, "/task", strings.NewReader(`{"goal":"test"}`))
	rec := httptest.NewRecorder()

	srv.handleTask(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d want=%d", rec.Code, http.StatusInternalServerError)
	}
	if !strings.Contains(rec.Body.String(), "task parser not configured") {
		t.Fatalf("body=%q", rec.Body.String())
	}
}
