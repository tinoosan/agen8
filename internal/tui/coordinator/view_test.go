package coordinator

import "testing"

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
		{"Write", "Write", ""},                          // exact match, no trailing
		{"Dispatch task team", "Dispatch task", "team"}, // multi-word verb
	}
	for _, tt := range tests {
		got := stripLeadingVerb(tt.text, tt.verb)
		if got != tt.want {
			t.Errorf("stripLeadingVerb(%q, %q) = %q, want %q", tt.text, tt.verb, got, tt.want)
		}
	}
}
