package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunHTTPBridgeAddsNativeSessionHeader(t *testing.T) {
	seenHeader := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenHeader = r.Header.Get("Agen8-Native-Session-Id")
		if r.Header.Get("Accept") != "application/json, text/event-stream" {
			t.Fatalf("Accept=%q", r.Header.Get("Accept"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"ok":true}}`))
	}))
	t.Cleanup(server.Close)

	var out strings.Builder
	err := RunHTTPBridge(context.Background(), HTTPBridgeOptions{
		MCPURL:          server.URL + "/mcp?token=agen8-local",
		NativeSessionID: "claude-session-1",
		In:              strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}` + "\n"),
		Out:             &out,
	})
	if err != nil {
		t.Fatalf("RunHTTPBridge: %v", err)
	}
	if seenHeader != "claude-session-1" {
		t.Fatalf("native header=%q", seenHeader)
	}
	if strings.TrimSpace(out.String()) != `{"jsonrpc":"2.0","id":1,"result":{"ok":true}}` {
		t.Fatalf("out=%q", out.String())
	}
}

func TestRunHTTPBridgeRelaysSSEDataMessages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("event: message\n"))
		_, _ = w.Write([]byte(`data: {"jsonrpc":"2.0","id":2,"result":{"ok":true}}` + "\n\n"))
	}))
	t.Cleanup(server.Close)

	var out strings.Builder
	err := RunHTTPBridge(context.Background(), HTTPBridgeOptions{
		MCPURL:          server.URL + "/mcp?token=agen8-local",
		NativeSessionID: "claude-session-2",
		In:              strings.NewReader(`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}` + "\n"),
		Out:             &out,
	})
	if err != nil {
		t.Fatalf("RunHTTPBridge: %v", err)
	}
	if strings.TrimSpace(out.String()) != `{"jsonrpc":"2.0","id":2,"result":{"ok":true}}` {
		t.Fatalf("out=%q", out.String())
	}
}

func TestRunHTTPBridgeWritesRPCErrorForHTTPFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad token", http.StatusUnauthorized)
	}))
	t.Cleanup(server.Close)

	var out strings.Builder
	err := RunHTTPBridge(context.Background(), HTTPBridgeOptions{
		MCPURL:          server.URL + "/mcp?token=bad",
		NativeSessionID: "claude-session-3",
		In:              strings.NewReader(`{"jsonrpc":"2.0","id":"abc","method":"tools/list","params":{}}` + "\n"),
		Out:             &out,
	})
	if err != nil {
		t.Fatalf("RunHTTPBridge: %v", err)
	}
	var resp struct {
		ID    string `json:"id"`
		Error struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(out.String())), &resp); err != nil {
		t.Fatalf("unmarshal output: %v: %q", err, out.String())
	}
	if resp.ID != "abc" || resp.Error.Code != -32001 || !strings.Contains(resp.Error.Message, "bad token") {
		t.Fatalf("resp=%+v", resp)
	}
}

func TestRunHTTPBridgeSuppressesAcceptedNotifications(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}))
	t.Cleanup(server.Close)

	var out strings.Builder
	err := RunHTTPBridge(context.Background(), HTTPBridgeOptions{
		MCPURL:          server.URL + "/mcp?token=agen8-local",
		NativeSessionID: "claude-session-4",
		In:              strings.NewReader(`{"jsonrpc":"2.0","method":"notifications/initialized","params":{}}` + "\n"),
		Out:             &out,
	})
	if err != nil {
		t.Fatalf("RunHTTPBridge: %v", err)
	}
	if out.String() != "" {
		t.Fatalf("out=%q want empty", out.String())
	}
}

func TestRunHTTPBridgeCachesNativeSessionPerProjectRoot(t *testing.T) {
	t.Setenv(EnvBridgeSessionDir, t.TempDir())
	projectRoot := filepath.Join(t.TempDir(), "repo")
	headers := make([]string, 0, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headers = append(headers, r.Header.Get("Agen8-Native-Session-Id"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"ok":true}}`))
	}))
	t.Cleanup(server.Close)

	for i := 0; i < 2; i++ {
		var out strings.Builder
		err := RunHTTPBridge(context.Background(), HTTPBridgeOptions{
			MCPURL:      server.URL + "/mcp?token=agen8-local",
			ProjectRoot: projectRoot,
			In:          strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}` + "\n"),
			Out:         &out,
		})
		if err != nil {
			t.Fatalf("RunHTTPBridge %d: %v", i, err)
		}
	}
	if len(headers) != 2 {
		t.Fatalf("headers=%v", headers)
	}
	if headers[0] == "" || headers[0] != headers[1] {
		t.Fatalf("headers=%v want same cached native session", headers)
	}
}

func TestRunHTTPBridgeEphemeralSkipsProjectCache(t *testing.T) {
	t.Setenv(EnvBridgeSessionDir, t.TempDir())
	projectRoot := filepath.Join(t.TempDir(), "repo")
	headers := make([]string, 0, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headers = append(headers, r.Header.Get("Agen8-Native-Session-Id"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"ok":true}}`))
	}))
	t.Cleanup(server.Close)

	for i := 0; i < 2; i++ {
		var out strings.Builder
		err := RunHTTPBridge(context.Background(), HTTPBridgeOptions{
			MCPURL:      server.URL + "/mcp?token=agen8-local",
			ProjectRoot: projectRoot,
			Ephemeral:   true,
			In:          strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}` + "\n"),
			Out:         &out,
		})
		if err != nil {
			t.Fatalf("RunHTTPBridge %d: %v", i, err)
		}
	}
	if len(headers) != 2 {
		t.Fatalf("headers=%v", headers)
	}
	if headers[0] == "" || headers[1] == "" || headers[0] == headers[1] {
		t.Fatalf("headers=%v want distinct ephemeral native sessions", headers)
	}
}
