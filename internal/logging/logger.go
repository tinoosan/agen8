package logging

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

func NewLogger(cfg Config) (*slog.Logger, error) {
	file := strings.TrimSpace(cfg.File)
	if file == "" {
		file = strings.TrimSpace(os.Getenv(EnvLogFile))
	}
	if file == "" {
		return NewTextLogger(os.Stderr, cfg)
	}
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return nil, fmt.Errorf("logging: create log directory: %w", err)
	}
	f, err := os.OpenFile(file, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("logging: open log file: %w", err)
	}
	return NewTextLogger(io.MultiWriter(os.Stderr, f), cfg)
}

func NewTextLogger(w io.Writer, cfg Config) (*slog.Logger, error) {
	if w == nil {
		return nil, fmt.Errorf("logging: writer is required")
	}
	level, err := parseLevel(cfg.Level)
	if err != nil {
		return nil, err
	}
	return slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: level})), nil
}

func parseLevel(raw string) (slog.Level, error) {
	switch strings.TrimSpace(strings.ToLower(raw)) {
	case "", "info":
		return slog.LevelInfo, nil
	case "debug":
		return slog.LevelDebug, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return slog.LevelInfo, fmt.Errorf("logging: unknown level %q (want debug, info, warn, or error)", raw)
	}
}
