package vfsutil

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"

	"github.com/tinoosan/agen8/pkg/validate"
)

func SafeJoinBaseDir(baseDir, subpath string) (string, error) {
	if err := validate.NonEmpty("baseDir", baseDir); err != nil {
		return "", err
	}
	sub := strings.TrimSpace(subpath)
	if strings.HasPrefix(sub, "/") {
		return "", fmt.Errorf("absolute paths not allowed: %q: %w", subpath, ErrInvalidPath)
	}
	clean := path.Clean(strings.TrimPrefix(sub, "./"))
	if sub == "" || clean == "." {
		clean = "."
	}
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("invalid path: escapes mount root")
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
