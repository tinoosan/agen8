package tui

import (
	"strings"
	"testing"

	"github.com/tinoosan/workbench-core/internal/events"
)

func TestFormatEventLine_OpRequest(t *testing.T) {
	ev := events.Event{
		Type:    "agent.op.request",
		Message: "Agent requested host op",
		Data: map[string]string{
			"op":       "fs.read",
			"path":     "/tools/builtin.bash",
			"toolId":   "",
			"actionId": "",
			"maxBytes": "4096",
		},
	}
	line := formatEventLine(ev)
	if !strings.HasPrefix(line, "* ") {
		t.Fatalf("expected bullet line, got %q", line)
	}
	for _, want := range []string{"op=fs.read", "path=/tools/builtin.bash", "maxBytes=4096"} {
		if !strings.Contains(line, want) {
			t.Fatalf("expected %q to contain %q", line, want)
		}
	}
}

func TestFormatEventLine_OpResponse(t *testing.T) {
	ev := events.Event{
		Type:    "agent.op.response",
		Message: "Host op completed",
		Data: map[string]string{
			"op":        "tool.run",
			"ok":        "true",
			"callId":    "abc",
			"bytesLen":  "123",
			"truncated": "true",
		},
	}
	line := formatEventLine(ev)
	for _, want := range []string{"op=tool.run", "ok=true", "callId=abc"} {
		if !strings.Contains(line, want) {
			t.Fatalf("expected %q to contain %q", line, want)
		}
	}
}
