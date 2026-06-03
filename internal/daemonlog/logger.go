// Package daemonlog provides centralized structured logging for the agen8 daemon.
//
// Call Init once at daemon startup to configure the global slog default.
// All daemon code then uses slog.Info/Warn/Error/Debug directly.
//
// Configuration is controlled via environment variables:
//
//   - AGEN8_LOG_LEVEL: debug | info | warn | error (default: info)
//   - AGEN8_LOG_FORMAT: text | json | auto (default: auto; auto = text on TTY, json otherwise)
//   - AGEN8_QUIET: 1 = errors only + startup banner
//
// These env vars can be seeded from config.toml's [logging] section via
// the normal runtime config pipeline.
package daemonlog

import (
	"io"
	"log/slog"
	"os"
	"strings"
	"sync/atomic"

	"golang.org/x/term"
)

// Environment variable keys.
const (
	EnvLogLevel  = "AGEN8_LOG_LEVEL"
	EnvLogFormat = "AGEN8_LOG_FORMAT"
	EnvQuiet     = "AGEN8_QUIET"
)

var quiet atomic.Bool

// Init configures the global slog logger. It should be called once at daemon
// startup. w receives all log output (typically io.MultiWriter(os.Stderr, logFile)).
//
// If AGEN8_QUIET is truthy, the level is forced to error.
func Init(w io.Writer) {
	level := ParseLevel(strings.TrimSpace(os.Getenv(EnvLogLevel)))
	format := strings.ToLower(strings.TrimSpace(os.Getenv(EnvLogFormat)))

	if isTruthy(strings.TrimSpace(os.Getenv(EnvQuiet))) {
		level = slog.LevelError
		quiet.Store(true)
	}

	opts := &slog.HandlerOptions{Level: level}
	var handler slog.Handler
	switch format {
	case "json":
		handler = slog.NewJSONHandler(w, opts)
	case "text":
		handler = slog.NewTextHandler(w, opts)
	default: // "auto" or empty
		if isTerminal(w) {
			handler = slog.NewTextHandler(w, opts)
		} else {
			handler = slog.NewJSONHandler(w, opts)
		}
	}
	slog.SetDefault(slog.New(handler))
}

// IsQuiet returns true when AGEN8_QUIET mode is active (errors only).
func IsQuiet() bool {
	return quiet.Load()
}

// ParseLevel converts a level string to slog.Level. Unrecognized values
// default to info.
func ParseLevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	case "info", "":
		return slog.LevelInfo
	default:
		return slog.LevelInfo
	}
}

// isTerminal returns true if w is backed by a TTY file descriptor.
// Exported as a variable for testing.
var isTerminal = func(w io.Writer) bool {
	if f, ok := w.(*os.File); ok {
		return term.IsTerminal(int(f.Fd()))
	}
	return false
}

func isTruthy(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
