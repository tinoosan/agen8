package logging

import (
	"bytes"
	"strings"
	"testing"
)

func TestNewTextLoggerHonorsLevel(t *testing.T) {
	var buf bytes.Buffer
	logger, err := NewTextLogger(&buf, Config{Level: "warn"})
	if err != nil {
		t.Fatalf("NewTextLogger: %v", err)
	}

	logger.Info("hidden")
	logger.Warn("shown", "service", "task")

	out := buf.String()
	if strings.Contains(out, "hidden") {
		t.Fatalf("info log was written at warn level: %s", out)
	}
	if !strings.Contains(out, "shown") {
		t.Fatalf("warn log missing: %s", out)
	}
	if !strings.Contains(out, "service=task") {
		t.Fatalf("service field missing: %s", out)
	}
}

func TestNewTextLoggerRejectsUnknownLevel(t *testing.T) {
	var buf bytes.Buffer
	_, err := NewTextLogger(&buf, Config{Level: "verbose"})
	if err == nil {
		t.Fatal("NewTextLogger returned nil error for unknown level")
	}
}
