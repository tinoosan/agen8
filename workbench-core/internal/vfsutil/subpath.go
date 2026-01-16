package vfsutil

import (
	"fmt"
	"path"
	"strings"
)

// NormalizeResourceSubpath enforces the VFS resource contract for subpaths:
//   - resource-relative (no leading "/")
//   - treats "" and "." as root
//   - rejects escape attempts ("..", "a/../x")
//
// It returns:
//   - clean: a normalized subpath (never starts with "/")
//   - parts: path segments of clean (nil when clean is "" or ".")
//
// Error message compatibility:
// This helper intentionally uses the phrases:
//   - "absolute paths not allowed"
//   - "invalid path: escapes mount root"
//
// so existing resource tests and error expectations remain stable.
func NormalizeResourceSubpath(subpath string) (clean string, parts []string, err error) {
	s := strings.TrimSpace(subpath)
	if s == "" || s == "." {
		return s, nil, nil
	}
	if strings.HasPrefix(s, "/") {
		return "", nil, fmt.Errorf("absolute paths not allowed: %q", subpath)
	}

	// Reject any explicit parent directory segments, even if they would clean away.
	for _, seg := range strings.Split(s, "/") {
		if seg == ".." {
			return "", nil, fmt.Errorf("invalid path: escapes mount root")
		}
	}

	clean = path.Clean(s)
	if clean == "." {
		return ".", nil, nil
	}
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return "", nil, fmt.Errorf("invalid path: escapes mount root")
	}

	parts = strings.Split(clean, "/")
	for _, p := range parts {
		if p == "" {
			return "", nil, fmt.Errorf("invalid path: empty segment")
		}
	}
	return clean, parts, nil
}

// CleanRelPath validates and cleans a relative path that must stay under a mount root.
//
// Unlike NormalizeResourceSubpath, this does not treat "" or "." specially; it is used
// for tool-provided artifact paths and store keys where a concrete relative filename is required.
func CleanRelPath(rel string) (string, error) {
	rel = strings.TrimSpace(rel)
	if rel == "" {
		return "", fmt.Errorf("invalid path: empty")
	}
	if strings.HasPrefix(rel, "/") {
		// Keep this phrasing stable; multiple resources/stores rely on substring checks.
		return "", fmt.Errorf("invalid path: absolute paths not allowed")
	}
	for _, seg := range strings.Split(rel, "/") {
		if seg == ".." {
			return "", fmt.Errorf("invalid path: escapes mount root")
		}
	}
	clean := path.Clean(rel)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("invalid path: escapes mount root")
	}
	return clean, nil
}
