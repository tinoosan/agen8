package store

import (
	"testing"

	"github.com/tinoosan/agen8/internal/opmeta"
	"github.com/tinoosan/agen8/pkg/types"
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

func TestActivityOpID_RunScoped(t *testing.T) {
	if got := activityOpID("run-a", "op-1"); got != "run-a|op-1" {
		t.Fatalf("activityOpID = %q, want run-a|op-1", got)
	}
	if got := activityOpID("run-b", "op-1"); got != "run-b|op-1" {
		t.Fatalf("activityOpID = %q, want run-b|op-1", got)
	}
	if got := activityOpID("", "op-1"); got != "" {
		t.Fatalf("activityOpID empty run should be blank, got %q", got)
	}
}

func TestActivityIDFromEvent_UsesRunScopedOpID(t *testing.T) {
	ev := types.EventRecord{
		EventID: "event-1",
		Data:    map[string]string{"opId": "op-7"},
	}
	if got := activityIDFromEvent("run-x", ev); got != "run-x|op-7" {
		t.Fatalf("activityIDFromEvent = %q, want run-x|op-7", got)
	}
}
