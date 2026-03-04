package coordinator

import (
	"testing"
	"time"
)

func TestStripLeadingVerb(t *testing.T) {
	tests := []struct {
		text, verb, want string
	}{
		{"Write /memory/foo.md", "Write", "/memory/foo.md"},
		{"Read src/handler.go", "Read", "src/handler.go"},
		{"Bash go test ./...", "Bash", "go test ./..."},
		{"write /memory/foo.md", "Write", "/memory/foo.md"}, // case-insensitive
		{"/memory/foo.md", "Write", "/memory/foo.md"},       // no match
		{"Writing more", "Write", "Writing more"},           // partial word, no strip
		{"", "Write", ""},
		{"Write /path", "", "Write /path"},
		{"Write", "Write", ""},                      // exact match, no trailing
		{"Create task team", "Create task", "team"}, // multi-word verb
	}
	for _, tt := range tests {
		got := stripLeadingVerb(tt.text, tt.verb)
		if got != tt.want {
			t.Errorf("stripLeadingVerb(%q, %q) = %q, want %q", tt.text, tt.verb, got, tt.want)
		}
	}
}

func TestGroupBridgeOpsInEntries_DetectsActionAndLegacyTag(t *testing.T) {
	now := time.Now()
	entries := []feedEntry{
		{
			kind:       feedAgent,
			opKind:     "code_exec",
			status:     "ok",
			timestamp:  now,
			finishedAt: now.Add(2 * time.Second),
		},
		{
			kind:      feedAgent,
			opKind:    "task_create",
			timestamp: now.Add(time.Second),
			data: map[string]string{
				"action": "code_exec_bridge",
			},
		},
		{
			kind:      feedAgent,
			opKind:    "obsidian",
			timestamp: now.Add(time.Second),
			data: map[string]string{
				"tag": "code_exec_bridge",
			},
		},
	}

	out := groupBridgeOpsInEntries(entries)
	if len(out) != 1 {
		t.Fatalf("expected 1 grouped parent entry, got %d", len(out))
	}
	if out[0].childCount != 2 {
		t.Fatalf("childCount=%d want 2", out[0].childCount)
	}
}

func TestGroupBridgeOpsInEntries_WriteOpsStoredInBridgeWriteOps(t *testing.T) {
	now := time.Now()
	entries := []feedEntry{
		{
			kind:       feedAgent,
			opKind:     "code_exec",
			status:     "ok",
			timestamp:  now,
			finishedAt: now.Add(2 * time.Second),
		},
		{
			kind:      feedAgent,
			opKind:    "fs_write",
			path:      "/workspace/output.txt",
			timestamp: now.Add(time.Second),
			data: map[string]string{
				"action": "code_exec_bridge",
			},
		},
		{
			kind:      feedAgent,
			opKind:    "fs_read",
			timestamp: now.Add(time.Second),
			data: map[string]string{
				"action": "code_exec_bridge",
			},
		},
	}

	out := groupBridgeOpsInEntries(entries)
	if len(out) != 1 {
		t.Fatalf("expected 1 grouped parent entry, got %d", len(out))
	}
	if len(out[0].bridgeWriteOps) != 1 {
		t.Fatalf("bridgeWriteOps=%d want 1", len(out[0].bridgeWriteOps))
	}
	if out[0].bridgeWriteOps[0].path != "/workspace/output.txt" {
		t.Fatalf("bridgeWriteOps[0].path=%q want /workspace/output.txt", out[0].bridgeWriteOps[0].path)
	}
	if out[0].childCount != 1 {
		t.Fatalf("childCount=%d want 1 (fs_read only)", out[0].childCount)
	}
}
