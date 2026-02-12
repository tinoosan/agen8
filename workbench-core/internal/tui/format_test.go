package tui

import (
	"strings"
	"testing"

	"github.com/tinoosan/workbench-core/internal/opmeta"
	"github.com/tinoosan/workbench-core/pkg/events"
)

func TestClassifyEvent_OpRequest(t *testing.T) {
	ev := events.Event{
		Type:    "agent.op.request",
		Message: "Agent requested host op",
		Data: map[string]string{
			"op":          "shell_exec",
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
			"op":        "fs_read",
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

func TestRenderOpRequest_BrowserAndEmail(t *testing.T) {
	browser := renderOpRequest(map[string]string{
		"op":       "browser",
		"action":   "navigate",
		"url":      "https://example.com",
		"selector": "#ignored",
	})
	if browser != "browse.navigate https://example.com" {
		t.Fatalf("unexpected browser request rendering: %q", browser)
	}

	email := renderOpRequest(map[string]string{
		"op":      "email",
		"to":      "team@example.com",
		"subject": "Daily report",
	})
	if email != "Email team@example.com: Daily report" {
		t.Fatalf("unexpected email request rendering: %q", email)
	}
}

func TestRenderOpResponse_ToolSpecific(t *testing.T) {
	tests := []struct {
		name string
		data map[string]string
		want string
	}{
		{
			name: "shell exec exit code",
			data: map[string]string{"op": "shell_exec", "ok": "true", "exitCode": "0"},
			want: "✓ exit 0",
		},
		{
			name: "http status",
			data: map[string]string{"op": "http_fetch", "ok": "true", "status": "200"},
			want: "✓ 200",
		},
		{
			name: "browser navigate",
			data: map[string]string{"op": "browser.navigate", "ok": "true", "title": "Example Domain"},
			want: `✓ navigated "Example Domain"`,
		},
		{
			name: "search results",
			data: map[string]string{"op": "fs_search", "ok": "true", "results": "3"},
			want: "✓ 3 results",
		},
		{
			name: "email sent",
			data: map[string]string{"op": "email", "ok": "true"},
			want: "✓ sent",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := renderOpResponse(tc.data)
			if got != tc.want {
				t.Fatalf("renderOpResponse() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestActionCategory_BrowserAndEmail(t *testing.T) {
	if got := actionCategory("browser"); got != "Browsed" {
		t.Fatalf("actionCategory(browser) = %q, want %q", got, "Browsed")
	}
	if got := actionCategory("email"); got != "Sent" {
		t.Fatalf("actionCategory(email) = %q, want %q", got, "Sent")
	}
}

func TestRenderOpRequest_SharedOpParityWithOpMeta(t *testing.T) {
	tests := []map[string]string{
		{"op": "fs_search", "path": "/workspace", "query": "needle"},
		{"op": "shell_exec", "argvPreview": "rg -n todo"},
		{"op": "http_fetch", "method": "POST", "url": "https://example.com", "body": "{\n\"x\":1\n}"},
		{"op": "http_fetch", "url": "https://example.com"},
		{"op": "trace_run", "traceAction": "set", "traceKey": "alpha"},
	}
	for _, tc := range tests {
		got := renderOpRequest(tc)
		want := opmeta.FormatRequestTitle(tc)
		if got != want {
			t.Fatalf("renderOpRequest(%v)=%q want %q", tc, got, want)
		}
	}
}
