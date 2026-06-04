package claudecli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
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
	if !strings.Contains(initResp.Result.Instructions, "Agen8 coordination events") {
		t.Fatalf("unexpected instructions: %q", initResp.Result.Instructions)
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
