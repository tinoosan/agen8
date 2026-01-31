package vfsutil

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/tinoosan/workbench-core/pkg/validate"
)

func SafeJoinBaseDir(baseDir, subpath string) (string, error) {
	if err := validate.NonEmpty("baseDir", baseDir); err != nil {
		return "", err
	}
	clean, _, err := validateSubpath(subpath)
	if err != nil {
		return "", err
	}
	if clean == "" {
		clean = "."
	}

	joined := filepath.Join(baseDir, filepath.FromSlash(clean))

	baseAbs, err := filepath.Abs(baseDir)
	if err != nil {
		return "", fmt.Errorf("abs baseDir: %w", err)
	}
	joinedAbs, err := filepath.Abs(joined)
	if err != nil {
		return "", fmt.Errorf("abs joined: %w", err)
	}
	rel, err := filepath.Rel(baseAbs, joinedAbs)
	if err != nil {
		return "", fmt.Errorf("rel: %w", err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("invalid path: escapes mount root")
	}
	return joinedAbs, nil
}

func RelUnderBaseDir(baseDir, absPath string) (string, error) {
	if err := validate.NonEmpty("baseDir", baseDir); err != nil {
		return "", err
	}
	absPath = strings.TrimSpace(absPath)
	if err := validate.NonEmpty("absPath", absPath); err != nil {
		return "", err
	}
	if !filepath.IsAbs(absPath) {
		return "", fmt.Errorf("absolute paths required")
	}

	baseAbs, err := filepath.Abs(baseDir)
	if err != nil {
		return "", fmt.Errorf("abs baseDir: %w", err)
	}
	targetAbs, err := filepath.Abs(filepath.Clean(absPath))
	if err != nil {
		return "", fmt.Errorf("abs path: %w", err)
	}

	rel, err := filepath.Rel(baseAbs, targetAbs)
	if err != nil {
		return "", fmt.Errorf("rel: %w", err)
	}
	rel = filepath.ToSlash(rel)
	if rel == ".." || strings.HasPrefix(rel, "../") {
		return "", fmt.Errorf("invalid path: escapes mount root")
	}
	return rel, nil
}
