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

func TestGroupBridgeToolCalls_DetectsActionAndLegacyTag(t *testing.T) {
	now := time.Now()
	turns := []conversationTurn{
		{
			kind:   turnAgent,
			role:   "reviewer",
			isText: false,
			entries: []feedEntry{
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
			},
		},
	}

	out := groupBridgeToolCalls(turns)
	if len(out) != 1 || len(out[0].entries) != 1 {
		t.Fatalf("expected single grouped parent entry, got %+v", out)
	}
	if out[0].entries[0].childCount != 2 {
		t.Fatalf("childCount=%d want 2", out[0].entries[0].childCount)
	}
}
