package pathutil

import (
	"fmt"
	"strings"

	"github.com/tinoosan/workbench-core/internal/vfsutil"
)

// NormalizeResourceSubpath is a thin wrapper around vfsutil.NormalizeResourceSubpath.
//
// It exists to consolidate common "VFS resource contract" validation in one place.
func NormalizeResourceSubpath(subpath string) (clean string, parts []string, err error) {
	return vfsutil.NormalizeResourceSubpath(subpath)
}

// SafeJoinBaseDir is a thin wrapper around vfsutil.SafeJoinBaseDir.
func SafeJoinBaseDir(baseDir, subpath string) (string, error) {
	return vfsutil.SafeJoinBaseDir(baseDir, subpath)
}

// CleanRelPath is a thin wrapper around vfsutil.CleanRelPath.
func CleanRelPath(rel string) (string, error) {
	return vfsutil.CleanRelPath(rel)
}

// CleanResultsArtifactPath validates and cleans a tool-provided artifact path written under
// "/results/<callId>/".
//
// This preserves runner-facing error phrasing while using shared validation underneath.
func CleanResultsArtifactPath(p string) (string, error) {
	if strings.TrimSpace(p) == "" {
		return "", fmt.Errorf("artifact path is required")
	}

	clean, err := vfsutil.CleanRelPath(p)
	if err != nil {
		// Preserve the runner-facing phrasing while reusing shared path validation.
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
