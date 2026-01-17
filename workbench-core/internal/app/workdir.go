package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// resolveWorkDir returns the absolute OS path for the host working directory.
//
// If spec is empty, it uses os.Getwd(). The returned path must exist and be a directory.
func resolveWorkDir(spec string) (string, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		wd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("getwd: %w", err)
		}
		spec = wd
	}
	abs, err := filepath.Abs(spec)
	if err != nil {
		return "", fmt.Errorf("abs workdir: %w", err)
	}
	st, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("stat workdir: %w", err)
	}
	if !st.IsDir() {
		return "", fmt.Errorf("workdir is not a directory: %s", abs)
	}
	return abs, nil
}

// resolveWorkDirChange resolves a workdir change request to an absolute directory path.
//
// Rules:
//   - If spec is relative, it is resolved against currentAbs.
//   - If spec is absolute, it is used directly.
//   - The resulting path must exist and be a directory.
func resolveWorkDirChange(currentAbs, spec string) (string, error) {
	currentAbs = strings.TrimSpace(currentAbs)
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return "", fmt.Errorf("path is required")
	}
	if currentAbs == "" {
		wd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("getwd: %w", err)
		}
		currentAbs = wd
	}

	target := spec
	if !filepath.IsAbs(target) {
		target = filepath.Join(currentAbs, target)
	}

	abs, err := filepath.Abs(target)
	if err != nil {
		return "", fmt.Errorf("abs workdir: %w", err)
	}
	st, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("stat workdir: %w", err)
	}
	if !st.IsDir() {
		return "", fmt.Errorf("workdir is not a directory: %s", abs)
	}
	return abs, nil
}
