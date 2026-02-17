package opformat

import (
	"testing"

	"github.com/tinoosan/workbench-core/internal/opmeta"
)

func TestFormatRequestText(t *testing.T) {
	tests := []struct {
		name string
		data map[string]string
		want string
	}{
		{name: "task create tag", data: map[string]string{"tag": "task_create"}, want: "Create task"},
		{name: "browser navigate", data: map[string]string{"op": "browser", "action": "navigate", "url": "https://example.com"}, want: "browse.navigate https://example.com"},
		{name: "browser click selector", data: map[string]string{"op": "browser", "action": "click", "selector": "#submit"}, want: "browse.click #submit"},
		{name: "email", data: map[string]string{"op": "email", "to": "team@example.com", "subject": "Daily report"}, want: "Email team@example.com: Daily report"},
		{name: "agent spawn", data: map[string]string{"op": "agent_spawn", "goal": "subtask", "model": "gpt-5-mini", "currentDepth": "0", "maxDepth": "3"}, want: "Spawn child agent: subtask (model=gpt-5-mini, depth=0/3)"},
		{name: "generic unknown", data: map[string]string{"op": "video_record", "path": "/tmp/a.mp4"}, want: "op=video_record path=/tmp/a.mp4"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := FormatRequestText(tc.data)
			if got != tc.want {
				t.Fatalf("FormatRequestText() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestFormatResponseText(t *testing.T) {
	tests := []struct {
		name string
		data map[string]string
		want string
	}{
		{name: "task create ok", data: map[string]string{"tag": "task_create", "ok": "true", "text": "Task created"}, want: "✓ Task created"},
		{name: "fs_read truncated", data: map[string]string{"op": "fs_read", "ok": "true", "truncated": "true"}, want: "✓ truncated"},
		{name: "shell exec exit", data: map[string]string{"op": "shell_exec", "ok": "true", "exitCode": "0"}, want: "✓ exit 0"},
		{name: "shell exec fail", data: map[string]string{"op": "shell_exec", "ok": "false", "exitCode": "1", "err": "boom"}, want: "✗ exit 1: boom"},
		{name: "http status", data: map[string]string{"op": "http_fetch", "ok": "true", "status": "200"}, want: "✓ 200"},
		{name: "search results", data: map[string]string{"op": "fs_search", "ok": "true", "results": "3"}, want: "✓ 3 results"},
		{name: "browser navigate title", data: map[string]string{"op": "browser.navigate", "ok": "true", "title": "Example Domain"}, want: `✓ navigated "Example Domain"`},
		{name: "browser generic", data: map[string]string{"op": "browser.custom_step", "ok": "true"}, want: "✓ custom step"},
		{name: "unknown fail", data: map[string]string{"op": "video_record", "ok": "false", "err": "not allowed"}, want: "✗ not allowed"},
		{name: "unknown ok", data: map[string]string{"op": "video_record", "ok": "true"}, want: "✓ ok"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := FormatResponseText(tc.data)
			if got != tc.want {
				t.Fatalf("FormatResponseText() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestFormatRequestText_SharedTitleOpsParityWithOpmeta(t *testing.T) {
	tests := []map[string]string{
		{"op": "fs_search", "path": "/workspace", "query": "needle"},
		{"op": "shell_exec", "argvPreview": "rg -n todo"},
		{"op": "http_fetch", "method": "POST", "url": "https://example.com", "body": "{\n\"x\":1\n}"},
		{"op": "http_fetch", "url": "https://example.com"},
		{"op": "trace_run", "traceAction": "set", "traceKey": "alpha"},
		{"op": "agent_spawn", "goal": "subtask", "currentDepth": "0", "maxDepth": "3"},
	}
	for _, tc := range tests {
		got := FormatRequestText(tc)
		want := opmeta.FormatRequestTitle(tc)
		if got != want {
			t.Fatalf("FormatRequestText(%v)=%q want %q", tc, got, want)
		}
	}
}

func TestFormatRequestText_UnknownFallbackUnchanged(t *testing.T) {
	got := FormatRequestText(map[string]string{
		"op":   "video_record",
		"path": "/tmp/a.mp4",
	})
	if got != "op=video_record path=/tmp/a.mp4" {
		t.Fatalf("FormatRequestText unknown fallback = %q", got)
	}
}
