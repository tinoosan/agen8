package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
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
//  3. platform default:
//     macOS:   ~/.agen8
//     Linux:   $XDG_STATE_HOME/agen8 or ~/.local/state/agen8
//     Windows: %AppData%\agen8
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
	base, err = filepath.Abs(filepath.Clean(expanded))
	if err != nil {
		return "", fmt.Errorf("resolve data dir %q: %w", expanded, err)
	}

	if err := ensureDirWritable(base); err != nil {
		return "", err
	}
	return base, nil
}

func defaultDataDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve default data dir: cannot determine home directory: %w", err)
	}
	userConfigDir, _ := os.UserConfigDir()
	return defaultDataDirFor(runtime.GOOS, home, os.Getenv(EnvXDGStateHome), userConfigDir)
}

func defaultDataDirFor(goos, home, xdgStateHome, userConfigDir string) (string, error) {
	home = strings.TrimSpace(home)
	xdgStateHome = strings.TrimSpace(xdgStateHome)
	userConfigDir = strings.TrimSpace(userConfigDir)
	switch strings.TrimSpace(goos) {
	case "darwin":
		if home == "" {
			return "", fmt.Errorf("resolve default data dir: home directory is empty")
		}
		return filepath.Join(home, ".agen8"), nil
	case "windows":
		if userConfigDir != "" {
			return filepath.Join(userConfigDir, "agen8"), nil
		}
		if home == "" {
			return "", fmt.Errorf("resolve default data dir: user config directory and home directory are empty")
		}
		return filepath.Join(home, "AppData", "Roaming", "agen8"), nil
	default:
		if xdgStateHome != "" {
			return filepath.Join(xdgStateHome, "agen8"), nil
		}
		if home == "" {
			return "", fmt.Errorf("resolve default data dir: home directory is empty")
		}
		return filepath.Join(home, ".local", "state", "agen8"), nil
	}
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
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create data dir %q: %w", dir, err)
	}
	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("stat data dir %q: %w", dir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("data dir %q is not a directory", dir)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return fmt.Errorf("harden data dir %q permissions: %w", dir, err)
	}

	f, err := os.CreateTemp(dir, ".agen8_write_test_*")
	if err != nil {
		return fmt.Errorf("data dir %q is not writable: %w", dir, err)
	}
	name := f.Name()
	_, writeErr := f.Write([]byte("ok"))
	closeErr := f.Close()
	os.Remove(name)
	if writeErr != nil {
		return fmt.Errorf("data dir %q is not writable: %w", dir, writeErr)
	}
	if closeErr != nil {
		return fmt.Errorf("data dir %q is not writable: %w", dir, closeErr)
	}
	return nil
}
