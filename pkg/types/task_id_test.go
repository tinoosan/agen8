package types

import (
	"strings"
	"testing"
)

func TestNormalizeTaskID(t *testing.T) {
	tests := []struct {
		in      string
		want    string
		changed bool
	}{
		{"foo", "task-foo", true},
		{" task-abc ", "task-abc", false},
		{"heartbeat-x", "heartbeat-x", false},
		{"", "", false},
		{"   ", "", false},
	}
	for _, tt := range tests {
		got, changed := NormalizeTaskID(tt.in)
		if got != tt.want || changed != tt.changed {
			t.Fatalf("NormalizeTaskID(%q) = (%q,%v), want (%q,%v)", tt.in, got, changed, tt.want, tt.changed)
		}
	}
}

func FuzzNormalizeTaskID_Invariants(f *testing.F) {
	for _, seed := range []string{
		"",
		"   ",
		"foo",
		" task-abc ",
		"heartbeat-x",
		"already/with/slash",
		" task-with-spaces ",
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, raw string) {
		got, changed := NormalizeTaskID(raw)
		trimmed := strings.TrimSpace(raw)

		switch {
		case trimmed == "":
			if got != "" || changed {
				t.Fatalf("empty input invariant violated: got=(%q,%v)", got, changed)
			}
		case strings.HasPrefix(trimmed, "task-"), strings.HasPrefix(trimmed, "heartbeat-"):
			if got != trimmed || changed {
				t.Fatalf("prefixed input invariant violated: raw=%q got=(%q,%v)", raw, got, changed)
			}
		default:
			if got != "task-"+trimmed || !changed {
				t.Fatalf("normalization invariant violated: raw=%q got=(%q,%v)", raw, got, changed)
			}
		}

		// Normalization must be idempotent after first pass.
		got2, changed2 := NormalizeTaskID(got)
		if got2 != got || changed2 {
			t.Fatalf("idempotence violated: first=(%q,%v) second=(%q,%v)", got, changed, got2, changed2)
		}
		if got != "" && !strings.HasPrefix(got, "task-") && !strings.HasPrefix(got, "heartbeat-") {
			t.Fatalf("normalized ID must be prefixed, got=%q", got)
		}
	})
}
