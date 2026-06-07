package mcp

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestReadAndRestoreBodyReadsAndRestoresWithinLimit(t *testing.T) {
	payload := []byte(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`)
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(payload))

	body, err := readAndRestoreBody(req)
	if err != nil {
		t.Fatalf("readAndRestoreBody: %v", err)
	}
	if !bytes.Equal(body, payload) {
		t.Fatalf("body=%q want %q", body, payload)
	}

	reread, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("re-read restored body: %v", err)
	}
	if !bytes.Equal(reread, payload) {
		t.Fatalf("restored body=%q want %q", reread, payload)
	}
}

func TestReadAndRestoreBodyRejectsWhenContentLengthTooLarge(t *testing.T) {
	payload := strings.Repeat("a", maxMCPRequestBodyBytes+1)
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(payload))

	body, err := readAndRestoreBody(req)
	if err == nil {
		t.Fatalf("want error for oversized content-length, got body=%q", body)
	}
	if !strings.Contains(err.Error(), "request body too large") {
		t.Fatalf("got error=%v, want too-large error", err)
	}
}

func TestReadAndRestoreBodyRejectsWhenChunkedBodyTooLarge(t *testing.T) {
	payload := strings.Repeat("a", maxMCPRequestBodyBytes+1)
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(payload))
	req.ContentLength = -1

	body, err := readAndRestoreBody(req)
	if err == nil {
		t.Fatalf("want error for oversized chunked body, got body=%q", body)
	}
	if !strings.Contains(err.Error(), "request body too large") {
		t.Fatalf("got error=%v, want too-large error", err)
	}
}

func TestNewMCPHTTPServerUsesReadHeaderTimeout(t *testing.T) {
	srv := newMCPHTTPServer(http.NewServeMux())
	if got, want := srv.ReadHeaderTimeout, 5*time.Second; got != want {
		t.Fatalf("ReadHeaderTimeout=%s want %s", got, want)
	}
}
