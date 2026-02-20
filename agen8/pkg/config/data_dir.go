package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	// EnvDataDir overrides the agen8 data directory.
	EnvDataDir = "AGEN8_DATA_DIR"

	// EnvXDGStateHome is the XDG base directory for state data.
	// If set, agen8 defaults to "$XDG_STATE_HOME/agen8".
	EnvXDGStateHome = "XDG_STATE_HOME"
)

// ResolveDataDir resolves the agen8 DataDir used by the host runtime.
//
// Precedence:
//  1. CLI flag (only if explicitly set)
//  2. env var AGEN8_DATA_DIR
//  4. default: ~/.agen8 (or $XDG_STATE_HOME/agen8 when XDG_STATE_HOME is set)
//     If the new default does not exist but the legacy default exists, use the legacy path.
//
// It also ensures the directory exists and is writable.
func ResolveDataDir(cliValue string, cliWasSet bool) (string, error) {
	var base string
	switch {
	case cliWasSet:
		base = strings.TrimSpace(cliValue)
		if base == "" {
			return "", fmt.Errorf("--data-dir was set but is empty")
		}
	case strings.TrimSpace(os.Getenv(EnvDataDir)) != "":
		base = strings.TrimSpace(os.Getenv(EnvDataDir))
	default:
		var err error
		base, err = defaultDataDir()
		if err != nil {
			return "", err
		}
	}

	expanded, err := expandTilde(base)
	if err != nil {
		return "", err
	}
	base = filepath.Clean(expanded)

	if err := ensureDirWritable(base); err != nil {
		return "", err
	}
	return base, nil
}

func defaultDataDir() (string, error) {
	if xdg := strings.TrimSpace(os.Getenv(EnvXDGStateHome)); xdg != "" {
		return preferExistingLegacy(filepath.Join(xdg, "agen8"), filepath.Join(xdg, "workbench")), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve default data dir: cannot determine home directory: %w", err)
	}
	home = strings.TrimSpace(home)
	if home == "" {
		return "", fmt.Errorf("resolve default data dir: home directory is empty")
	}
	return preferExistingLegacy(filepath.Join(home, ".agen8"), filepath.Join(home, ".workbench")), nil
}

func preferExistingLegacy(preferred, legacy string) string {
	if preferred == "" || legacy == "" {
		return preferred
	}
	if _, err := os.Stat(preferred); err == nil {
		return preferred
	}
	if _, err := os.Stat(legacy); err == nil {
		return legacy
	}
	return preferred
}

// expandTilde expands "~" and "~/" prefixes using the current user's home directory.
// Only "~" and "~/" are supported; "~someone" is treated as a literal path.
func expandTilde(p string) (string, error) {
	p = strings.TrimSpace(p)
	if p == "" {
		return "", fmt.Errorf("data dir is empty")
	}
	if p != "~" && !strings.HasPrefix(p, "~/") {
		return p, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("expand data dir %q: cannot determine home directory: %w", p, err)
	}
	home = strings.TrimSpace(home)
	if home == "" {
		return "", fmt.Errorf("expand data dir %q: home directory is empty", p)
	}
	if p == "~" {
		return home, nil
	}
	// p starts with "~/"
	return filepath.Join(home, strings.TrimPrefix(p, "~/")), nil
}

func ensureDirWritable(dir string) error {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return fmt.Errorf("data dir is empty")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create data dir %q: %w", dir, err)
	}
	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("stat data dir %q: %w", dir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("data dir %q is not a directory", dir)
	}

	f, err := os.CreateTemp(dir, ".agen8_write_test_*")
	if err != nil {
		return fmt.Errorf("data dir %q is not writable: %w", dir, err)
	}
	name := f.Name()
	_, writeErr := f.Write([]byte("ok"))
	closeErr := f.Close()
	_ = os.Remove(name)
	if writeErr != nil {
		return fmt.Errorf("data dir %q is not writable: %w", dir, writeErr)
	}
	if closeErr != nil {
		return fmt.Errorf("data dir %q is not writable: %w", dir, closeErr)
	}
	return nil
}
