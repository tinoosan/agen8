package claudecli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunChannelInitializesClaudeChannelCapabilityAndEmitsNotification(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stdin, stdinWriter := io.Pipe()
	var stdout bytes.Buffer
	ready := make(chan ChannelReady, 1)
	done := make(chan error, 1)
	go func() {
		done <- RunChannel(ctx, ChannelOptions{
			ListenAddr: "127.0.0.1:0",
			In:         stdin,
			Out:        &stdout,
			Ready:      ready,
		})
	}()
	if _, err := stdinWriter.Write([]byte(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18"}}` + "\n")); err != nil {
		t.Fatalf("write initialize: %v", err)
	}
	if _, err := stdinWriter.Write([]byte(`{"jsonrpc":"2.0","id":2,"method":"tools/list"}` + "\n")); err != nil {
		t.Fatalf("write tools/list: %v", err)
	}

	var channelReady ChannelReady
	select {
	case channelReady = <-ready:
	case <-time.After(2 * time.Second):
		t.Fatal("channel did not become ready")
	}
	resp, err := http.Post(channelReady.NotifyURL, "application/json", strings.NewReader(`{"content":"Task assigned","meta":{"source":"agen8","taskId":"task-1"}}`))
	if err != nil {
		t.Fatalf("post notify: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("notify status=%d", resp.StatusCode)
	}
	if err := stdinWriter.Close(); err != nil {
		t.Fatalf("close stdin: %v", err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("RunChannel: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("channel did not stop after stdin EOF")
	}

	scanner := bufio.NewScanner(bytes.NewReader(stdout.Bytes()))
	if !scanner.Scan() {
		t.Fatal("missing initialize response")
	}
	var initResp struct {
		Result struct {
			Capabilities struct {
				Experimental map[string]map[string]any `json:"experimental"`
				Tools        map[string]any            `json:"tools"`
			} `json:"capabilities"`
			Instructions string `json:"instructions"`
		} `json:"result"`
	}
	if err := json.Unmarshal(scanner.Bytes(), &initResp); err != nil {
		t.Fatalf("decode initialize response: %v", err)
	}
	if _, ok := initResp.Result.Capabilities.Experimental["claude/channel"]; !ok {
		t.Fatalf("missing claude/channel capability: %#v", initResp.Result.Capabilities.Experimental)
	}
	if initResp.Result.Capabilities.Tools == nil {
		t.Fatal("missing tools capability")
	}
	if !strings.Contains(initResp.Result.Instructions, "Agen8 coordination events") {
		t.Fatalf("unexpected instructions: %q", initResp.Result.Instructions)
	}

	if !scanner.Scan() {
		t.Fatal("missing tools/list response")
	}
	var toolsResp struct {
		Result struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal(scanner.Bytes(), &toolsResp); err != nil {
		t.Fatalf("decode tools/list response: %v", err)
	}
	if len(toolsResp.Result.Tools) != 1 || toolsResp.Result.Tools[0].Name != "message" {
		t.Fatalf("tools=%#v", toolsResp.Result.Tools)
	}

	if !scanner.Scan() {
		t.Fatal("missing channel notification")
	}
	var notification struct {
		Method string `json:"method"`
		Params struct {
			Content string         `json:"content"`
			Meta    map[string]any `json:"meta"`
		} `json:"params"`
	}
	if err := json.Unmarshal(scanner.Bytes(), &notification); err != nil {
		t.Fatalf("decode notification: %v", err)
	}
	if notification.Method != "notifications/claude/channel" {
		t.Fatalf("method=%q", notification.Method)
	}
	if notification.Params.Content != "Task assigned" {
		t.Fatalf("content=%q", notification.Params.Content)
	}
	if notification.Params.Meta["taskId"] != "task-1" || notification.Params.Meta["source"] != "agen8" {
		t.Fatalf("meta=%#v", notification.Params.Meta)
	}
}

func TestHandleChannelToolCallSendsMessageWithSessionBinding(t *testing.T) {
	root := t.TempDir()
	received := make(chan map[string]any, 1)
	daemon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/harness/claude-channel/message" {
			t.Fatalf("path=%q", r.URL.Path)
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		received <- payload
		_, _ = w.Write([]byte(`{"ok":true,"text":"sent","result":{"messageId":"msg-1"}}`))
	}))
	t.Cleanup(daemon.Close)

	writeBindingForTest(t, root, claudeSessionBindingFile{
		MCPURL:    daemon.URL + "/mcp?token=token-1",
		Token:     "token-1",
		MemberID:  "member-claude",
		SessionID: "claude-session-1",
	})
	result, err := handleChannelToolCall(context.Background(), root, json.RawMessage(`{
		"name":"message",
		"arguments":{
			"action":"send",
			"destination_member_id":"member-codex",
			"kind":"ack",
			"subject":"Re: UAT",
			"body":"confirmed",
			"correlation_id":"corr-1"
		}
	}`))
	if err != nil {
		t.Fatalf("handleChannelToolCall: %v", err)
	}
	if result["structuredContent"] == nil {
		t.Fatalf("missing structured content: %#v", result)
	}
	select {
	case payload := <-received:
		if payload["token"] != "token-1" || payload["memberId"] != "member-claude" || payload["sessionId"] != "claude-session-1" {
			t.Fatalf("binding payload=%#v", payload)
		}
		args, ok := payload["arguments"].(map[string]any)
		if !ok {
			t.Fatalf("arguments=%#v", payload["arguments"])
		}
		if args["action"] != "send" || args["destination_member_id"] != "member-codex" || args["kind"] != "ack" || args["correlation_id"] != "corr-1" {
			t.Fatalf("arguments=%#v", args)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for message request")
	}
}

func TestRegisterClaudeChannelRouteDoesNotFollowBindingChangesAfterSuccess(t *testing.T) {
	root := t.TempDir()
	received := make(chan map[string]any, 2)
	daemon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/harness/claude-channel/register" {
			t.Fatalf("path=%q", r.URL.Path)
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		received <- payload
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(daemon.Close)

	writeBindingForTest(t, root, claudeSessionBindingFile{
		MCPURL:    daemon.URL + "/mcp?token=token-1",
		Token:     "token-1",
		MemberID:  "member-1",
		SessionID: "session-ref-1",
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	startedAt := time.Date(2026, 6, 4, 10, 0, 0, 123, time.UTC)
	instance := channelRouteInstance{ID: "channel-instance-1", StartedAt: startedAt, ProcessID: 1234}
	go registerClaudeChannelRoute(ctx, ChannelOptions{ProjectRoot: root}, "http://127.0.0.1:4555/notify", instance)

	first := waitForChannelRoutePayload(t, received)
	if first["memberId"] != "member-1" || first["sessionId"] != "session-ref-1" {
		t.Fatalf("first payload=%#v", first)
	}
	if first["channelInstanceId"] != "channel-instance-1" || first["channelStartedAt"] != startedAt.Format(time.RFC3339Nano) || first["processId"] != float64(1234) {
		t.Fatalf("route metadata=%#v", first)
	}
	writeBindingForTest(t, root, claudeSessionBindingFile{
		MCPURL:    daemon.URL + "/mcp?token=token-2",
		Token:     "token-2",
		MemberID:  "member-2",
		SessionID: "session-ref-2",
	})
	select {
	case second := <-received:
		t.Fatalf("stale channel process followed changed binding: %#v", second)
	case <-time.After(250 * time.Millisecond):
	}
}

func writeBindingForTest(t *testing.T, root string, binding claudeSessionBindingFile) {
	t.Helper()
	data, err := json.Marshal(binding)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, ".agen8", "claude-session.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func waitForChannelRoutePayload(t *testing.T, received <-chan map[string]any) map[string]any {
	t.Helper()
	select {
	case payload := <-received:
		return payload
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for route registration")
		return nil
	}
}
