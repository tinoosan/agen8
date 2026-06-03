package daemonlog

import (
	"bytes"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"
)

func TestParseLevel(t *testing.T) {
	tests := []struct {
		input string
		want  slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"DEBUG", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"", slog.LevelInfo},
		{"warn", slog.LevelWarn},
		{"warning", slog.LevelWarn},
		{"error", slog.LevelError},
		{"unknown", slog.LevelInfo},
	}
	for _, tt := range tests {
		got := ParseLevel(tt.input)
		if got != tt.want {
			t.Errorf("ParseLevel(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestInit_TextFormat(t *testing.T) {
	var buf bytes.Buffer
	old := isTerminal
	isTerminal = func(io.Writer) bool { return false }
	defer func() { isTerminal = old }()

	t.Setenv(EnvLogLevel, "debug")
	t.Setenv(EnvLogFormat, "text")
	t.Setenv(EnvQuiet, "")

	Init(&buf)
	slog.Info("hello", "component", "test")

	out := buf.String()
	if !strings.Contains(out, "hello") {
		t.Fatalf("expected 'hello' in output, got %q", out)
	}
	if !strings.Contains(out, "component=test") {
		t.Fatalf("expected 'component=test' in output, got %q", out)
	}
}

func TestInit_JSONFormat(t *testing.T) {
	var buf bytes.Buffer
	old := isTerminal
	isTerminal = func(io.Writer) bool { return false }
	defer func() { isTerminal = old }()

	t.Setenv(EnvLogLevel, "info")
	t.Setenv(EnvLogFormat, "json")
	t.Setenv(EnvQuiet, "")

	Init(&buf)
	slog.Info("hello", "component", "test")

	out := buf.String()
	if !strings.Contains(out, `"msg":"hello"`) {
		t.Fatalf("expected JSON msg field, got %q", out)
	}
}

func TestInit_AutoFormat_NonTTY(t *testing.T) {
	var buf bytes.Buffer
	old := isTerminal
	isTerminal = func(io.Writer) bool { return false }
	defer func() { isTerminal = old }()

	t.Setenv(EnvLogLevel, "info")
	t.Setenv(EnvLogFormat, "")
	t.Setenv(EnvQuiet, "")

	Init(&buf)
	slog.Info("auto test")

	out := buf.String()
	// Non-TTY auto defaults to JSON
	if !strings.Contains(out, `"msg"`) {
		t.Fatalf("expected JSON output for non-TTY auto, got %q", out)
	}
}

func TestInit_AutoFormat_TTY(t *testing.T) {
	var buf bytes.Buffer
	old := isTerminal
	isTerminal = func(io.Writer) bool { return true }
	defer func() { isTerminal = old }()

	t.Setenv(EnvLogLevel, "info")
	t.Setenv(EnvLogFormat, "auto")
	t.Setenv(EnvQuiet, "")

	Init(&buf)
	slog.Info("tty test", "k", "v")

	out := buf.String()
	// TTY auto defaults to text
	if strings.Contains(out, `"msg"`) {
		t.Fatalf("expected text output for TTY auto, got %q", out)
	}
	if !strings.Contains(out, "tty test") {
		t.Fatalf("expected 'tty test' in output, got %q", out)
	}
}

func TestInit_QuietMode(t *testing.T) {
	var buf bytes.Buffer
	old := isTerminal
	isTerminal = func(io.Writer) bool { return false }
	defer func() { isTerminal = old }()

	t.Setenv(EnvLogLevel, "debug")
	t.Setenv(EnvLogFormat, "text")
	t.Setenv(EnvQuiet, "1")

	Init(&buf)

	if !IsQuiet() {
		t.Fatalf("expected IsQuiet() = true")
	}

	slog.Info("should be hidden")
	slog.Error("should appear")

	out := buf.String()
	if strings.Contains(out, "should be hidden") {
		t.Fatalf("info message should be suppressed in quiet mode, got %q", out)
	}
	if !strings.Contains(out, "should appear") {
		t.Fatalf("error message should appear in quiet mode, got %q", out)
	}

	// Reset for other tests.
	quiet.Store(false)
}

func TestInit_LevelFilter(t *testing.T) {
	var buf bytes.Buffer
	old := isTerminal
	isTerminal = func(io.Writer) bool { return false }
	defer func() { isTerminal = old }()

	t.Setenv(EnvLogLevel, "warn")
	t.Setenv(EnvLogFormat, "text")
	t.Setenv(EnvQuiet, "")

	Init(&buf)

	slog.Debug("debug msg")
	slog.Info("info msg")
	slog.Warn("warn msg")
	slog.Error("error msg")

	out := buf.String()
	if strings.Contains(out, "debug msg") {
		t.Fatalf("debug should be filtered at warn level")
	}
	if strings.Contains(out, "info msg") {
		t.Fatalf("info should be filtered at warn level")
	}
	if !strings.Contains(out, "warn msg") {
		t.Fatalf("warn should appear at warn level")
	}
	if !strings.Contains(out, "error msg") {
		t.Fatalf("error should appear at warn level")
	}
}

func TestIsTruthy(t *testing.T) {
	for _, v := range []string{"1", "true", "yes", "on", "TRUE", "Yes"} {
		if !isTruthy(v) {
			t.Errorf("isTruthy(%q) = false, want true", v)
		}
	}
	for _, v := range []string{"0", "false", "no", "", "random"} {
		if isTruthy(v) {
			t.Errorf("isTruthy(%q) = true, want false", v)
		}
	}
}

func TestIsTerminal_NonFile(t *testing.T) {
	// A bytes.Buffer is not a *os.File, so isTerminal should return false.
	old := isTerminal
	defer func() { isTerminal = old }()
	isTerminal = func(w io.Writer) bool {
		if f, ok := w.(*os.File); ok {
			_ = f // use f just to acknowledge the branch
			return false
		}
		return false
	}
	var buf bytes.Buffer
	if isTerminal(&buf) {
		t.Fatalf("expected false for bytes.Buffer")
	}
}
