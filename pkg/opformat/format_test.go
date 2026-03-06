package opformat

import (
	"testing"

	"github.com/tinoosan/agen8/internal/opmeta"
)

func TestFormatRequestText(t *testing.T) {
	tests := []struct {
		name string
		data map[string]string
		want string
	}{
		{name: "task create tag", data: map[string]string{"tag": "task_create"}, want: "Create task"},
		{name: "obsidian tag", data: map[string]string{"tag": "obsidian", "command": "search"}, want: "Obsidian search"},
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
		{name: "obsidian ok", data: map[string]string{"tag": "obsidian", "ok": "true", "command": "graph"}, want: "✓ obsidian graph"},
		{name: "fs_read truncated", data: map[string]string{"op": "fs_read", "ok": "true", "truncated": "true"}, want: "✓ truncated"},
		{name: "shell exec exit", data: map[string]string{"op": "shell_exec", "ok": "true", "exitCode": "0"}, want: "✓ exit 0"},
		{name: "shell exec fail", data: map[string]string{"op": "shell_exec", "ok": "false", "exitCode": "1", "err": "boom"}, want: "✗ exit 1: boom"},
		{name: "http status", data: map[string]string{"op": "http_fetch", "ok": "true", "status": "200"}, want: "✓ 200"},
		{name: "search results", data: map[string]string{"op": "fs_search", "ok": "true", "results": "3"}, want: "✓ 3 results"},
		{name: "search results truncated", data: map[string]string{"op": "fs_search", "ok": "true", "resultsReturned": "5", "resultsTotal": "12", "resultsTruncated": "true"}, want: "✓ 5/12 results"},
		{name: "fs write default", data: map[string]string{"op": "fs_write", "ok": "true"}, want: "✓ wrote"},
		{name: "fs write bytes and mode", data: map[string]string{"op": "fs_write", "ok": "true", "writeBytes": "0", "writeMode": "overwritten"}, want: "✓ wrote 0 bytes (overwritten)"},
		{name: "fs write verified checksum", data: map[string]string{"op": "fs_write", "ok": "true", "writeVerified": "true", "writeChecksumAlgo": "sha256"}, want: "✓ verified (sha256)"},
		{name: "fs write verify mismatch", data: map[string]string{"op": "fs_write", "ok": "false", "writeVerified": "false", "writeMismatchAt": "4", "writeExpectedBytes": "10", "writeActualBytes": "9"}, want: "✗ verify mismatch at byte 4 (10 != 9)"},
		{name: "fs write checksum mismatch", data: map[string]string{"op": "fs_write", "ok": "false", "writeChecksumMatch": "false", "writeChecksumAlgo": "md5"}, want: "✗ checksum mismatch (md5)"},
		{name: "fs batch edit dry run", data: map[string]string{"op": "fs_batch_edit", "ok": "true", "batchEditDryRun": "true", "matchedFiles": "12", "modifiedFiles": "9"}, want: "✓ batch edit dry-run 12 matched, 9 modified"},
		{name: "fs batch edit apply", data: map[string]string{"op": "fs_batch_edit", "ok": "true", "batchEditApplied": "true", "modifiedFiles": "9"}, want: "✓ batch edit applied 9 files"},
		{name: "fs batch edit failure rollback", data: map[string]string{"op": "fs_batch_edit", "ok": "false", "err": "batch edit failed after 4 files (rollback failed)"}, want: "✗ batch edit failed after 4 files (rollback failed)"},
		{name: "pipe success", data: map[string]string{"op": "pipe", "ok": "true", "steps": "3"}, want: "✓ pipe ok (3 steps)"},
		{name: "pipe failure", data: map[string]string{"op": "pipe", "ok": "false", "failedAtStep": "2"}, want: "✗ pipe failed at step 2"},
		{name: "fs patch success apply", data: map[string]string{"op": "fs_patch", "ok": "true", "patchMode": "apply", "patchHunksApplied": "1", "patchHunksTotal": "2"}, want: "✓ patched 1/2 hunks"},
		{name: "fs patch success dry run", data: map[string]string{"op": "fs_patch", "ok": "true", "patchDryRun": "true", "patchHunksApplied": "2", "patchHunksTotal": "2"}, want: "✓ dry-run 2/2 hunks"},
		{name: "fs patch failure with diagnostics", data: map[string]string{"op": "fs_patch", "ok": "false", "patchFailedHunk": "1", "patchFailureReason": "context_mismatch", "patchTargetLine": "7"}, want: "✗ hunk 1 context mismatch (line 7)"},
		{name: "fs patch failure fallback err", data: map[string]string{"op": "fs_patch", "ok": "false", "err": "patch did not apply cleanly"}, want: "✗ patch did not apply cleanly"},
		{name: "fs txn dry run success", data: map[string]string{"op": "fs_txn", "ok": "true", "txnMode": "dry_run", "txnStepsApplied": "2", "txnStepsTotal": "3"}, want: "✓ txn dry-run 2/3 steps"},
		{name: "fs txn apply success", data: map[string]string{"op": "fs_txn", "ok": "true", "txnMode": "apply", "txnStepsApplied": "3", "txnStepsTotal": "3"}, want: "✓ txn applied 3/3 steps"},
		{name: "fs txn failure", data: map[string]string{"op": "fs_txn", "ok": "false", "txnFailedStep": "2"}, want: "✗ txn failed at step 2"},
		{name: "fs txn rollback failure", data: map[string]string{"op": "fs_txn", "ok": "false", "txnFailedStep": "2", "txnRollbackFailed": "true"}, want: "✗ txn failed at step 2 (rollback failed)"},
		{name: "archive create", data: map[string]string{"op": "fs_archive_create", "ok": "true", "filesAdded": "2", "compressionRatio": "0.5000"}, want: "✓ archived 2 files (0.5000)"},
		{name: "archive extract with skipped", data: map[string]string{"op": "fs_archive_extract", "ok": "true", "filesExtracted": "3", "skippedCount": "1"}, want: "✓ extracted 3 files (1 skipped)"},
		{name: "archive list truncated", data: map[string]string{"op": "fs_archive_list", "ok": "true", "archiveEntries": "200", "truncated": "true"}, want: "✓ listed 200 entries (truncated)"},
		{name: "fs stat dir", data: map[string]string{"op": "fs_stat", "ok": "true", "isDir": "true"}, want: "✓ dir"},
		{name: "fs stat file with size", data: map[string]string{"op": "fs_stat", "ok": "true", "isDir": "false", "sizeBytes": "42"}, want: "✓ file 42 bytes"},
		{name: "fs stat file no size", data: map[string]string{"op": "fs_stat", "ok": "true", "isDir": "false"}, want: "✓ file"},
		{name: "fs stat missing", data: map[string]string{"op": "fs_stat", "ok": "true", "exists": "false"}, want: "✓ missing"},
		{name: "fs stat error", data: map[string]string{"op": "fs_stat", "ok": "false", "err": "not found"}, want: "✗ not found"},
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
		{"op": "fs_stat", "path": "/workspace/a.txt"},
		{"op": "fs_txn", "steps": "2"},
		{"op": "fs_batch_edit", "path": "/knowledge", "glob": "**/*.md"},
		{"op": "fs_archive_create", "path": "/workspace/a", "destination": "/workspace/a.tar.gz"},
		{"op": "pipe", "steps": "3"},
		{"op": "shell_exec", "argvPreview": "rg -n todo"},
		{"op": "http_fetch", "method": "POST", "url": "https://example.com", "body": "{\n\"x\":1\n}"},
		{"op": "http_fetch", "url": "https://example.com"},
		{"op": "trace_run", "traceAction": "set", "traceKey": "alpha"},
		{"op": "agent_spawn", "goal": "subtask", "currentDepth": "0", "maxDepth": "3"},
		{"op": "obsidian", "command": "search"},
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
