package domain

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"
)

func TestToolResult_JSONMarshal_RoundTrips(t *testing.T) {
	t.Parallel()

	original := ToolResult{
		Text:       "ok",
		Data:       json.RawMessage(`{"count":2}`),
		SourceType: SourceMCP,
		Duration:   2 * time.Second,
		Ok:         true,
		ErrorCode:  "rate_limited",
		Retryable:  true,
		Truncated:  true,
		Terminal:   true,
	}

	encoded, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal tool result: %v", err)
	}

	var decoded ToolResult
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unmarshal tool result: %v", err)
	}

	if decoded.Text != original.Text ||
		!bytes.Equal(decoded.Data, original.Data) ||
		decoded.SourceType != original.SourceType ||
		decoded.Duration != original.Duration ||
		decoded.Ok != original.Ok ||
		decoded.ErrorCode != original.ErrorCode ||
		decoded.Retryable != original.Retryable ||
		decoded.Truncated != original.Truncated ||
		decoded.Terminal != original.Terminal {
		t.Fatalf("round trip mismatch: got %#v want %#v", decoded, original)
	}
}

func TestToolResult_TerminalFlag_DefaultsFalse(t *testing.T) {
	t.Parallel()

	var result ToolResult

	if result.Terminal {
		t.Fatal("zero-value ToolResult should default Terminal to false")
	}
}

func TestToolCategory_StringValues(t *testing.T) {
	t.Parallel()

	tests := map[string]ToolCategory{
		"Execution":     CategoryExecution,
		"File System":   CategoryFileSystem,
		"Integration":   CategoryIntegration,
		"Coordination":  CategoryCoordination,
		"Observability": CategoryObservability,
		"System":        CategorySystem,
	}

	for want, got := range tests {
		if string(got) != want {
			t.Fatalf("category mismatch: got %q want %q", got, want)
		}
	}
}

func TestSourceType_Constants(t *testing.T) {
	t.Parallel()

	tests := map[string]SourceType{
		"builtin": SourceBuiltin,
		"mcp":     SourceMCP,
		"cli":     SourceCLI,
		"http":    SourceHTTP,
	}

	for want, got := range tests {
		if string(got) != want {
			t.Fatalf("source type mismatch: got %q want %q", got, want)
		}
	}
}

func TestToolMetadata_DisplayNameRequired(t *testing.T) {
	t.Parallel()

	metadata := ToolMetadata{}

	if metadata.DisplayName != "" {
		t.Fatalf("expected zero-value display name to be empty, got %q", metadata.DisplayName)
	}
}
