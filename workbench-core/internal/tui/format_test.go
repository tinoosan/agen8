package tui

import (
	"strings"
	"testing"

	"github.com/tinoosan/workbench-core/pkg/events"
)

func TestClassifyEvent_OpRequest(t *testing.T) {
	ev := events.Event{
		Type:    "agent.op.request",
		Message: "Agent requested host op",
		Data: map[string]string{
			"op":          "shell.exec",
			"argvPreview": `rg -n Example Domain`,
			"argv0":       "rg",
		},
	}
	rr := classifyEvent(ev)
	if rr.Class != RenderAction {
		t.Fatalf("expected action class, got %v", rr.Class)
	}
	for _, want := range []string{"rg -n", "Example Domain"} {
		if !strings.Contains(rr.Text, want) {
			t.Fatalf("expected %q to contain %q", rr.Text, want)
		}
	}
}

func TestClassifyEvent_OpResponse(t *testing.T) {
	ev := events.Event{
		Type:    "agent.op.response",
		Message: "Host op completed",
		Data: map[string]string{
			"op":        "fs.read",
			"ok":        "true",
			"bytesLen":  "123",
			"truncated": "true",
		},
	}
	rr := classifyEvent(ev)
	if rr.Class != RenderAction {
		t.Fatalf("expected action class, got %v", rr.Class)
	}
	if !strings.Contains(rr.Text, "✓") {
		t.Fatalf("expected completion marker, got %q", rr.Text)
	}
	if !strings.Contains(rr.Text, "truncated") {
		t.Fatalf("expected truncated marker, got %q", rr.Text)
	}
}
