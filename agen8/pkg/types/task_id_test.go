package types

import "testing"

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
