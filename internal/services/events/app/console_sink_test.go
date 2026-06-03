package app

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/tinoosan/agen8-mcp-server/pkg/types"
)

// setupSlogCapture configures slog to write JSON to a buffer and returns
// the buffer and a cleanup function that restores the original default.
func setupSlogCapture(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	orig := slog.Default()
	handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	slog.SetDefault(slog.New(handler))
	t.Cleanup(func() { slog.SetDefault(orig) })
	return &buf
}

// parseSlogJSON parses a slog JSON log line into a generic map.
func parseSlogJSON(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	var parsed map[string]any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("unmarshal slog JSON: %v\nraw: %s", err, string(raw))
	}
	return parsed
}

func TestConsoleSink_TruncatesLongDataValues(t *testing.T) {
	buf := setupSlogCapture(t)
	sink := ConsoleSink{}

	longText := strings.Repeat("A", 500)
	event := types.EventRecord{
		Type:    "test.event",
		Message: "test message",
		Data: map[string]string{
			"short":    "hello",
			"longText": longText,
		},
	}
	enabled := true
	event.Console = &enabled

	if err := sink.Emit(context.Background(), Message{Payload: event}); err != nil {
		t.Fatalf("Emit: %v", err)
	}

	parsed := parseSlogJSON(t, buf.Bytes())

	if parsed["msg"] != "test message" {
		t.Errorf("msg=%q want %q", parsed["msg"], "test message")
	}
	if parsed["type"] != "test.event" {
		t.Errorf("type=%q want %q", parsed["type"], "test.event")
	}
	if parsed["short"] != "hello" {
		t.Errorf("short=%q want %q", parsed["short"], "hello")
	}

	truncated, ok := parsed["longText"].(string)
	if !ok {
		t.Fatal("longText attr missing")
	}
	if len(truncated) > consoleDataValueLimit+10 {
		t.Errorf("longText not truncated: len=%d (limit=%d)", len(truncated), consoleDataValueLimit)
	}
	if !strings.HasSuffix(truncated, "…") {
		t.Errorf("truncated value should end with …: %q", truncated[len(truncated)-10:])
	}
}

func TestConsoleSink_ShortValuesUnchanged(t *testing.T) {
	buf := setupSlogCapture(t)
	sink := ConsoleSink{}

	event := types.EventRecord{
		Type:    "test.event",
		Message: "ok",
		Data: map[string]string{
			"role":    "cfo",
			"spaceId": "space-123",
			"step":    "3",
		},
	}
	enabled := true
	event.Console = &enabled

	if err := sink.Emit(context.Background(), Message{Payload: event}); err != nil {
		t.Fatalf("Emit: %v", err)
	}

	parsed := parseSlogJSON(t, buf.Bytes())

	if parsed["role"] != "cfo" {
		t.Errorf("role=%q want cfo", parsed["role"])
	}
	if parsed["spaceId"] != "space-123" {
		t.Errorf("spaceId=%q want space-123", parsed["spaceId"])
	}
}

func TestConsoleSink_DoesNotMutateOriginalData(t *testing.T) {
	setupSlogCapture(t)
	sink := ConsoleSink{}

	longText := strings.Repeat("X", 500)
	data := map[string]string{"text": longText}

	event := types.EventRecord{
		Type:    "test.event",
		Message: "test",
		Data:    data,
	}
	enabled := true
	event.Console = &enabled

	if err := sink.Emit(context.Background(), Message{Payload: event}); err != nil {
		t.Fatalf("Emit: %v", err)
	}

	// Original data should be untouched.
	if len(data["text"]) != 500 {
		t.Fatalf("original data was mutated: len=%d want 500", len(data["text"]))
	}
}

func TestTruncateDataValues(t *testing.T) {
	tests := []struct {
		name   string
		input  map[string]string
		maxLen int
		check  func(t *testing.T, out map[string]string)
	}{
		{
			name:   "nil map",
			input:  nil,
			maxLen: 100,
			check:  func(t *testing.T, out map[string]string) {},
		},
		{
			name:   "empty map",
			input:  map[string]string{},
			maxLen: 100,
			check: func(t *testing.T, out map[string]string) {
				if len(out) != 0 {
					t.Error("expected empty map")
				}
			},
		},
		{
			name:   "all under limit",
			input:  map[string]string{"a": "short", "b": "also short"},
			maxLen: 100,
			check: func(t *testing.T, out map[string]string) {
				if out["a"] != "short" || out["b"] != "also short" {
					t.Error("values should be unchanged")
				}
			},
		},
		{
			name:   "truncates over limit",
			input:  map[string]string{"x": strings.Repeat("Z", 300)},
			maxLen: 50,
			check: func(t *testing.T, out map[string]string) {
				r := []rune(out["x"])
				// 50 runes + "…"
				if len(r) != 51 {
					t.Errorf("expected 51 runes, got %d", len(r))
				}
				if !strings.HasSuffix(out["x"], "…") {
					t.Error("should end with …")
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out := truncateDataValues(tc.input, tc.maxLen)
			tc.check(t, out)
		})
	}
}
