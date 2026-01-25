package vfsutil

import (
	"errors"
	"fmt"
	"path"
	"strings"
)

func NormalizeResourceSubpath(subpath string) (clean string, parts []string, err error) {
	s := strings.TrimSpace(subpath)
	if s == "" || s == "." {
		return s, nil, nil
	}
	if strings.HasPrefix(s, "/") {
		return "", nil, fmt.Errorf("absolute paths not allowed: %q: %w", subpath, ErrInvalidPath)
	}
	for _, seg := range strings.Split(s, "/") {
		if seg == ".." {
			return "", nil, fmt.Errorf("invalid path: escapes mount root: %w", errors.Join(ErrInvalidPath, ErrEscapesRoot))
		}
	}
	clean = path.Clean(s)
	if clean == "." {
		return ".", nil, nil
	}
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return "", nil, fmt.Errorf("invalid path: escapes mount root: %w", errors.Join(ErrInvalidPath, ErrEscapesRoot))
	}
	parts = strings.Split(clean, "/")
	for _, p := range parts {
		if p == "" {
			return "", nil, fmt.Errorf("invalid path: empty segment")
		}
	}
	return clean, parts, nil
}

func CleanRelPath(rel string) (string, error) {
	rel = strings.TrimSpace(rel)
	if rel == "" {
		return "", fmt.Errorf("invalid path: empty")
	}
	if strings.HasPrefix(rel, "/") {
		return "", fmt.Errorf("invalid path: absolute paths not allowed: %w", ErrInvalidPath)
	}
	for _, seg := range strings.Split(rel, "/") {
		if seg == ".." {
			return "", fmt.Errorf("invalid path: escapes mount root: %w", errors.Join(ErrInvalidPath, ErrEscapesRoot))
		}
	}
	clean := path.Clean(rel)
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("invalid path: escapes mount root: %w", errors.Join(ErrInvalidPath, ErrEscapesRoot))
	}
	return clean, nil
}

func CleanResultsArtifactPath(p string) (string, error) {
	if strings.TrimSpace(p) == "" {
		return "", fmt.Errorf("artifact path is required")
	}
	clean, err := CleanRelPath(p)
	if err != nil {
		msg := err.Error()
		switch {
		case strings.Contains(msg, "absolute paths not allowed"):
			return "", fmt.Errorf("artifact path must be relative")
		case strings.Contains(msg, "escapes mount root"):
			return "", fmt.Errorf("artifact path escapes results directory")
		default:
			return "", err
		}
	}
	if clean == "." {
		return "", fmt.Errorf("artifact path is invalid")
	}
	return clean, nil
}
