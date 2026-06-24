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

	file, err := cleanLogFilePath(file)
	if err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(".")
	if err != nil {
		return nil, fmt.Errorf("logging: open log root: %w", err)
	}
	defer root.Close()

	if dir := filepath.Dir(file); dir != "." {
		if err := root.MkdirAll(dir, 0o700); err != nil {
			return nil, fmt.Errorf("logging: create log directory: %w", err)
		}
	}

	f, err := root.OpenFile(file, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, fmt.Errorf("logging: open log file: %w", err)
	}
	if err := f.Chmod(0o600); err != nil {
		if closeErr := f.Close(); closeErr != nil {
			return nil, fmt.Errorf("logging: close log file after chmod failure: %w (chmod: %v)", closeErr, err)
		}
		return nil, fmt.Errorf("logging: secure log file permissions: %w", err)
	}

	return NewTextLogger(io.MultiWriter(os.Stderr, f), cfg)

}

func cleanLogFilePath(file string) (string, error) {
	clean := filepath.Clean(file)
	if filepath.IsAbs(clean) || clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("logging: log file path must be a relative path within the current working directory")
	}
	return clean, nil
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
