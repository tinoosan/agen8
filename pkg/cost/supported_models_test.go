package cost

import (
	"testing"
)

func TestContextLengthForModel(t *testing.T) {
	tests := []struct {
		modelID  string
		wantLen  int
		wantBool bool
	}{
		{"openai/gpt-5.2", 128000, true},
		{"openai/gpt-4o", 128000, true},
		{"anthropic/claude-3.5-sonnet", 200000, true},
		{"anthropic/claude-3-opus", 200000, true},
		{"moonshotai/kimi-k2.5", 262000, true},
		{"unknown-provider/gpt-5.2", 128000, true}, // matches suffix
		{"unknown/model", 0, false},
		{"", 0, false},
	}

	for _, tt := range tests {
		got, ok := ContextLengthForModel(tt.modelID)
		if got != tt.wantLen || ok != tt.wantBool {
			t.Errorf("ContextLengthForModel(%q) = %v, %v; want %v, %v", tt.modelID, got, ok, tt.wantLen, tt.wantBool)
		}
	}
}
