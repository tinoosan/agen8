package store

import (
	"testing"

	"github.com/tinoosan/workbench-core/internal/opmeta"
)

func TestActivityTitleFromRequest_ParityWithOpMeta(t *testing.T) {
	tests := []map[string]string{
		{"op": "fs_search", "path": "/workspace", "query": "needle"},
		{"op": "shell_exec", "argvPreview": "rg -n todo"},
		{"op": "http_fetch", "method": "POST", "url": "https://example.com", "body": "{\n\"x\":1\n}"},
		{"op": "http_fetch", "url": "https://example.com"},
		{"op": "trace_run", "traceAction": "set", "traceKey": "alpha"},
	}
	for _, tc := range tests {
		got := activityTitleFromRequest(tc)
		want := opmeta.FormatRequestTitle(tc)
		if got != want {
			t.Fatalf("activityTitleFromRequest(%v)=%q want %q", tc, got, want)
		}
	}
}

func TestShouldHideRoutingNoiseOp_ParityWithOpMeta(t *testing.T) {
	tests := []struct {
		op   string
		path string
	}{
		{op: "fs_list", path: "/workspace/deliverables/a.txt"},
		{op: "fs_read", path: "/workspace/quarantine/a.txt"},
		{op: "fs_read", path: "/workspace/src/main.go"},
		{op: "shell_exec", path: "/workspace/deliverables/a.txt"},
	}
	for _, tc := range tests {
		got := shouldHideRoutingNoiseOp(tc.op, tc.path)
		want := opmeta.ShouldHideRoutingNoiseOp(tc.op, tc.path)
		if got != want {
			t.Fatalf("shouldHideRoutingNoiseOp(%q,%q)=%v want %v", tc.op, tc.path, got, want)
		}
	}
}
