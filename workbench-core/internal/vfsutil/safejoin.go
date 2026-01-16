package vfsutil

import (
	"fmt"
	"path/filepath"
	"strings"
)

// SafeJoinBaseDir converts a resource-relative subpath into a secure OS path under baseDir.
//
// This helper is used by filesystem-backed resources (like DirResource) that:
//   - accept a resource-relative path (no leading "/")
//   - allow benign normalization (e.g. "a/../b/file.txt" -> "b/file.txt")
//   - must reject any path that would escape baseDir
//
// This differs from NormalizeResourceSubpath:
//   - NormalizeResourceSubpath is a strict VFS helper used by "virtual" resources where
//     ".." segments are always rejected (even if they clean away).
//   - SafeJoinBaseDir is for OS filesystem joins where normalization is fine as long as
//     containment is enforced.
func SafeJoinBaseDir(baseDir, subpath string) (string, error) {
	if strings.TrimSpace(baseDir) == "" {
		return "", fmt.Errorf("baseDir is required")
	}

	// filepath.Clean turns things like "a/../b" into "b" and "a/../../x" into "../x".
	clean := filepath.Clean(subpath)

	if filepath.IsAbs(clean) {
		return "", fmt.Errorf("absolute paths not allowed")
	}

	// filepath.Clean can return "." for empty paths; that's fine (it refers to baseDir).
	// But reject any path that escapes upward after cleaning.
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("invalid path: escapes mount root")
	}

	joined := filepath.Join(baseDir, clean)

	// Strong containment check: compare absolute paths.
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
