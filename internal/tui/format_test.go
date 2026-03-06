package tui

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/tinoosan/agen8/internal/opmeta"
	"github.com/tinoosan/agen8/pkg/events"
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

	spawn := renderOpRequest(map[string]string{
		"op":           "agent_spawn",
		"goal":         "compute checksum for all files",
		"model":        "gpt-5-mini",
		"currentDepth": "0",
		"maxDepth":     "3",
	})
	if spawn != "Spawn child agent: compute checksum for all files (model=gpt-5-mini, depth=0/3)" {
		t.Fatalf("unexpected agent_spawn request rendering: %q", spawn)
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
			name: "search results truncated",
			data: map[string]string{"op": "fs_search", "ok": "true", "resultsReturned": "5", "resultsTotal": "12", "resultsTruncated": "true"},
			want: "✓ 5/12 results",
		},
		{
			name: "write verified checksum",
			data: map[string]string{"op": "fs_write", "ok": "true", "writeVerified": "true", "writeChecksumAlgo": "sha256"},
			want: "✓ verified (sha256)",
		},
		{
			name: "write bytes and mode",
			data: map[string]string{"op": "fs_write", "ok": "true", "writeBytes": "0", "writeMode": "created"},
			want: "✓ wrote 0 bytes (created)",
		},
		{
			name: "write verify mismatch",
			data: map[string]string{"op": "fs_write", "ok": "false", "writeVerified": "false", "writeMismatchAt": "3", "writeExpectedBytes": "12", "writeActualBytes": "10"},
			want: "✗ verify mismatch at byte 3 (12 != 10)",
		},
		{
			name: "write checksum mismatch",
			data: map[string]string{"op": "fs_write", "ok": "false", "writeChecksumMatch": "false", "writeChecksumAlgo": "sha256"},
			want: "✗ checksum mismatch (sha256)",
		},
		{
			name: "patch apply summary",
			data: map[string]string{"op": "fs_patch", "ok": "true", "patchMode": "apply", "patchHunksApplied": "1", "patchHunksTotal": "1"},
			want: "✓ patched 1/1 hunks",
		},
		{
			name: "patch dry run summary",
			data: map[string]string{"op": "fs_patch", "ok": "true", "patchDryRun": "true", "patchHunksApplied": "0", "patchHunksTotal": "1"},
			want: "✓ dry-run 0/1 hunks",
		},
		{
			name: "patch diagnostic failure",
			data: map[string]string{"op": "fs_patch", "ok": "false", "patchFailedHunk": "2", "patchFailureReason": "delete_mismatch", "patchTargetLine": "14"},
			want: "✗ hunk 2 delete mismatch (line 14)",
		},
		{
			name: "txn apply success",
			data: map[string]string{"op": "fs_txn", "ok": "true", "txnMode": "apply", "txnStepsApplied": "2", "txnStepsTotal": "2"},
			want: "✓ txn applied 2/2 steps",
		},
		{
			name: "txn failure",
			data: map[string]string{"op": "fs_txn", "ok": "false", "txnFailedStep": "2"},
			want: "✗ txn failed at step 2",
		},
		{
			name: "batch edit dry run",
			data: map[string]string{"op": "fs_batch_edit", "ok": "true", "batchEditDryRun": "true", "matchedFiles": "6", "modifiedFiles": "4"},
			want: "✓ batch edit dry-run 6 matched, 4 modified",
		},
		{
			name: "archive create",
			data: map[string]string{"op": "fs_archive_create", "ok": "true", "filesAdded": "2", "compressionRatio": "0.4000"},
			want: "✓ archived 2 files (0.4000)",
		},
		{
			name: "archive list",
			data: map[string]string{"op": "fs_archive_list", "ok": "true", "archiveEntries": "5"},
			want: "✓ listed 5 entries",
		},
		{
			name: "pipe success",
			data: map[string]string{"op": "pipe", "ok": "true", "steps": "2"},
			want: "✓ pipe ok (2 steps)",
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

func TestActionCategory_RepresentativeOps(t *testing.T) {
	tests := []struct {
		op   string
		want string
	}{
		{op: "browser", want: "Browsed"},
		{op: "browser.navigate", want: "Browsed"},
		{op: "email", want: "Sent"},
		{op: "fs_read", want: "Explored"},
		{op: "fs_stat", want: "Explored"},
		{op: "fs_write", want: "Updated"},
		{op: "fs_batch_edit", want: "Updated"},
		{op: "fs_txn", want: "Updated"},
		{op: "fs_archive_create", want: "Updated"},
		{op: "fs_archive_extract", want: "Updated"},
		{op: "fs_archive_list", want: "Explored"},
		{op: "shell_exec", want: "Ran"},
		{op: "pipe", want: "Ran"},
		{op: "http_fetch", want: "Fetched"},
		{op: "trace_run", want: "Traced"},
		{op: "agent_spawn", want: "Delegated"},
		{op: "task_create", want: "Created"},
		{op: "video_record", want: "Action"},
	}

	for _, tc := range tests {
		if got := actionCategory(tc.op); got != tc.want {
			t.Fatalf("actionCategory(%s) = %q, want %q", tc.op, got, tc.want)
		}
	}
}

func TestRenderOpRequest_SharedOpParityWithOpMeta(t *testing.T) {
	tests := []map[string]string{
		{"op": "fs_search", "path": "/workspace", "query": "needle"},
		{"op": "fs_stat", "path": "/workspace/a.txt"},
		{"op": "fs_txn", "steps": "3"},
		{"op": "fs_batch_edit", "path": "/knowledge", "glob": "**/*.md"},
		{"op": "fs_archive_extract", "path": "/workspace/a.tar.gz", "destination": "/workspace/out"},
		{"op": "pipe", "steps": "3"},
		{"op": "shell_exec", "argvPreview": "rg -n todo"},
		{"op": "http_fetch", "method": "POST", "url": "https://example.com", "body": "{\n\"x\":1\n}"},
		{"op": "http_fetch", "url": "https://example.com"},
		{"op": "trace_run", "traceAction": "set", "traceKey": "alpha"},
		{"op": "agent_spawn", "goal": "subtask", "currentDepth": "0", "maxDepth": "3"},
	}
	for _, tc := range tests {
		got := renderOpRequest(tc)
		want := opmeta.FormatRequestTitle(tc)
		if got != want {
			t.Fatalf("renderOpRequest(%v)=%q want %q", tc, got, want)
		}
	}
}

func TestRenderOpRequest_PrefersRequestTextField(t *testing.T) {
	got := renderOpRequest(map[string]string{
		"requestText": "custom request text",
		"op":          "browser",
		"action":      "navigate",
		"url":         "https://example.com",
	})
	if got != "custom request text" {
		t.Fatalf("renderOpRequest should prefer requestText field, got %q", got)
	}
}

func TestRenderOpResponse_PrefersResponseTextField(t *testing.T) {
	got := renderOpResponse(map[string]string{
		"responseText": "custom response text",
		"op":           "shell_exec",
		"ok":           "true",
		"exitCode":     "0",
	})
	if got != "custom response text" {
		t.Fatalf("renderOpResponse should prefer responseText field, got %q", got)
	}
}

func TestClassifyEvent_ClassMapping(t *testing.T) {
	tests := []struct {
		name string
		ev   events.Event
		want RenderClass
	}{
		{name: "ignore host mounted", ev: events.Event{Type: "host.mounted"}, want: RenderIgnore},
		{name: "ignore workdir pwd", ev: events.Event{Type: "workdir.pwd", Data: map[string]string{"workdir": "/workspace"}}, want: RenderIgnore},
		{name: "action run warning", ev: events.Event{Type: "run.warning", Data: map[string]string{"text": "disk low"}}, want: RenderAction},
		{name: "action ui editor open", ev: events.Event{Type: "ui.editor.open", Data: map[string]string{"path": "/workspace/a.go"}}, want: RenderAction},
		{name: "action workdir changed", ev: events.Event{Type: "workdir.changed", Data: map[string]string{"to": "/workspace"}}, want: RenderAction},
		{name: "telemetry usage total", ev: events.Event{Type: "llm.usage.total"}, want: RenderTelemetry},
		{name: "outcome agent error", ev: events.Event{Type: "agent.error"}, want: RenderOutcome},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rr := classifyEvent(tc.ev)
			if rr.Class != tc.want {
				t.Fatalf("classifyEvent(%s).Class = %v, want %v", tc.ev.Type, rr.Class, tc.want)
			}
		})
	}
}

func TestClassifyEvent_TextFormattingFallbacks(t *testing.T) {
	tests := []struct {
		name string
		ev   events.Event
		want string
	}{
		{name: "run warning default text", ev: events.Event{Type: "run.warning", Data: map[string]string{}}, want: "Warning"},
		{name: "refs attached default text", ev: events.Event{Type: "refs.attached", Data: map[string]string{}}, want: "Attached referenced files"},
		{name: "ui open error path and err", ev: events.Event{Type: "ui.open.error", Data: map[string]string{"path": "/tmp/a.txt", "err": "permission denied"}}, want: "Open failed: /tmp/a.txt (permission denied)"},
		{name: "ui open error err only", ev: events.Event{Type: "ui.open.error", Data: map[string]string{"err": "permission denied"}}, want: "Open failed: permission denied"},
		{name: "ui open error default", ev: events.Event{Type: "ui.open.error", Data: map[string]string{}}, want: "Open failed"},
		{name: "workdir changed from to", ev: events.Event{Type: "workdir.changed", Data: map[string]string{"from": "/a", "to": "/b"}}, want: "Workdir changed: /a → /b"},
		{name: "workdir changed to only", ev: events.Event{Type: "workdir.changed", Data: map[string]string{"to": "/b"}}, want: "Workdir: /b"},
		{name: "workdir changed default", ev: events.Event{Type: "workdir.changed", Data: map[string]string{}}, want: "Workdir changed"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rr := classifyEvent(tc.ev)
			if rr.Text != tc.want {
				t.Fatalf("classifyEvent(%s).Text = %q, want %q", tc.ev.Type, rr.Text, tc.want)
			}
		})
	}
}

func TestClassifyEvent_UnknownTypeIgnoredAndRawPreserved(t *testing.T) {
	ev := events.Event{
		Type:    "tool.custom",
		Message: "custom event",
		Data: map[string]string{
			"k": "v",
		},
	}
	rr := classifyEvent(ev)
	if rr.Class != RenderIgnore {
		t.Fatalf("expected RenderIgnore for unknown type, got %v", rr.Class)
	}
	if rr.Text != "" {
		t.Fatalf("expected empty text for unknown type, got %q", rr.Text)
	}

	var raw struct {
		Type    string            `json:"type"`
		Message string            `json:"message"`
		Data    map[string]string `json:"data"`
	}
	if err := json.Unmarshal([]byte(rr.Raw), &raw); err != nil {
		t.Fatalf("expected valid raw JSON, got err=%v raw=%q", err, rr.Raw)
	}
	if raw.Type != ev.Type || raw.Message != ev.Message || raw.Data["k"] != "v" {
		t.Fatalf("unexpected raw payload: %+v", raw)
	}
}

func TestClassifyEvent_HiddenInboxOpIgnored(t *testing.T) {
	ev := events.Event{
		Type: "agent.op.request",
		Data: map[string]string{
			"op":   "fs_read",
			"path": "/inbox/checklist.md",
		},
	}
	rr := classifyEvent(ev)
	if rr.Class != RenderIgnore {
		t.Fatalf("expected hidden inbox op to be ignored, got %v", rr.Class)
	}
}

func TestClassifyEvent_HiddenInboxFSStatOpIgnored(t *testing.T) {
	ev := events.Event{
		Type: "agent.op.request",
		Data: map[string]string{
			"op":   "fs_stat",
			"path": "/inbox/checklist.md",
		},
	}
	rr := classifyEvent(ev)
	if rr.Class != RenderIgnore {
		t.Fatalf("expected hidden inbox op to be ignored, got %v", rr.Class)
	}
}

func TestEventRenderersWithFormattersHaveValidClass(t *testing.T) {
	for k, renderer := range registeredEventRenderers() {
		if renderer.formatter == nil {
			continue
		}
		if renderer.class < RenderIgnore || renderer.class > RenderOutcome {
			t.Fatalf("event %q has formatter but invalid class %v", k, renderer.class)
		}
	}
}
