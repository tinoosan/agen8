// Package resources provides concrete implementations of the vfs.Resource interface.
//
// DirResource is a filesystem-backed resource that maps a virtual mount to a real
// OS directory, with built-in path traversal protection.
package resources

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tinoosan/workbench-core/pkg/fsutil"
	"github.com/tinoosan/workbench-core/pkg/vfs"
	"github.com/tinoosan/workbench-core/pkg/vfsutil"
)

// DirResource implements vfs.Resource by mapping virtual paths to a real OS directory.
// It provides secure filesystem access with automatic path traversal protection.
//
// Example:
//
//	dr, _ := NewDirResource("/data/runs/run-123/scratch", "workspace")
//	// Virtual path "/scratch/notes.md" maps to "/data/runs/run-123/scratch/notes.md"
//
// Security:
//
//	All subpaths are validated through safeJoin to prevent directory traversal attacks.
//	Paths like "../etc/passwd" or "a/../../secrets" are rejected.
type DirResource struct {
	// BaseDir is the OS directory backing this resource (the sandbox root).
	// All operations are confined under BaseDir. The resource must reject any
	// subpath that would escape BaseDir (e.g. "..", absolute paths).
	//
	// BaseDir is an implementation detail; callers interact via virtual paths
	// like "/scratch/notes.md" through the VFS.
	//
	// Example: "/data/runs/run-123/scratch" or "/Users/alice/projects/myapp".
	BaseDir string

	// Mount is the virtual mount name used by the VFS.
	// Example: "workspace" maps to the virtual namespace "/scratch".
	Mount string
}

// NewDirResource creates a new directory-backed resource.
//
// Parameters:
//   - baseDir: the OS directory path to mount (e.g., "/data/scratch")
//   - mount: the virtual mount name (e.g., "workspace")
//
// The mount name will be sanitized by removing leading slashes.
//
// Returns an error if:
//   - baseDir is empty
//   - mount is empty or becomes empty after sanitization
//
// Example:
//
//	dr, err := NewDirResource("/data/runs/123/scratch", "workspace")
//	if err != nil {
//	    log.Fatal(err)
//	}
func NewDirResource(baseDir, mount string) (*DirResource, error) {
	if baseDir == "" {
		return nil, fmt.Errorf("baseDir cannot be empty")
	}
	mount = strings.TrimLeft(mount, "/")
	if mount == "" {
		return nil, fmt.Errorf("mount cannot be empty")
	}

	return &DirResource{
		BaseDir: baseDir,
		Mount:   mount,
	}, nil
}

// List lists entries under subpath relative to BaseDir.
// subpath is resource-relative (no leading "/").
// List("") lists the resource root.
//
// Each returned Entry has its Path field set to the resource-relative path
// (e.g., "reports/q1.md" not just "q1.md").
//
// Returns an error if:
//   - subpath attempts directory traversal (e.g., "../etc")
//   - the directory doesn't exist
//   - there are permission issues
//
// Example:
//
//	entries, err := dr.List("reports")
//	// Returns entries like "reports/q1.md", "reports/q2.md"
func (d *DirResource) List(subpath string) ([]vfs.Entry, error) {
	targetPath, err := d.safeJoin(subpath)
	if err != nil {
		return nil, fmt.Errorf("safeJoin %s: %w", subpath, err)
	}

	des, err := os.ReadDir(targetPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("does not exist: %w", err)
		}
		return nil, fmt.Errorf("list %s: %w", targetPath, err)
	}
	entries := make([]vfs.Entry, 0, len(des))
	for _, de := range des {
		info, err := de.Info()
		if err != nil {
			return nil, fmt.Errorf("stat entry %s: %w", de.Name(), err)
		}

		childSubpath := strings.TrimLeft(filepath.ToSlash(filepath.Join(subpath, de.Name())), "/")

		e := vfs.Entry{
			Path:       childSubpath,
			IsDir:      de.IsDir(),
			Size:       info.Size(),
			ModTime:    info.ModTime(),
			HasSize:    true,
			HasModTime: true,
		}

		entries = append(entries, e)
	}

	return entries, nil
}

// Read reads a file at subpath relative to BaseDir.
// subpath is resource-relative (no leading "/").
//
// Returns an error if:
//   - subpath attempts directory traversal
//   - the file doesn't exist
//   - there are permission issues
//   - the path points to a directory (not a file)
//
// Example:
//
//	data, err := dr.Read("config.json")
//	if err != nil {
//	    return err
//	}
func (d *DirResource) Read(subpath string) ([]byte, error) {
	targetPath, err := d.safeJoin(subpath)
	if err != nil {
		return nil, err
	}

	b, err := os.ReadFile(targetPath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", targetPath, err)
	}

	return b, nil
}

// Write replaces the file at subpath (creating parent directories if needed).
// subpath is resource-relative (no leading "/").
//
// Parent directories are created automatically if they don't exist.
// The write is performed atomically using a temp file + rename strategy.
//
// Returns an error if:
//   - subpath attempts directory traversal
//   - parent directory creation fails
//   - there are permission issues
//   - disk is full
//
// Example:
//
//	err := dr.Write("logs/app.log", []byte("Starting...\n"))
//	// Creates "logs" directory if needed, then writes the file
func (d *DirResource) Write(subpath string, data []byte) error {
	targetPath, err := d.safeJoin(subpath)
	if err != nil {
		return err
	}

	parentDir := filepath.Dir(targetPath)
	if err := os.MkdirAll(parentDir, 0755); err != nil {
		return fmt.Errorf("mkdir %s: %w", parentDir, err)
	}

	err = fsutil.WriteFileAtomic(targetPath, data, 0644)
	if err != nil {
		return fmt.Errorf("write %s: %w", targetPath, err)
	}

	return nil
}

// Append appends bytes to the file at subpath (creating parent directories if needed).
// subpath is resource-relative (no leading "/").
//
// If the file doesn't exist, it is created (like Write).
// Parent directories are created automatically if they don't exist.
//
// Returns an error if:
//   - subpath attempts directory traversal
//   - parent directory creation fails
//   - there are permission issues
//   - disk is full
//
// Example:
//
//	err := dr.Append("logs/events.log", []byte("2024-01-11: User logged in\n"))
//	// Appends to existing file or creates new file
func (d *DirResource) Append(subpath string, data []byte) error {
	targetPath, err := d.safeJoin(subpath)
	if err != nil {
		return err
	}

	parentDir := filepath.Dir(targetPath)
	if err := os.MkdirAll(parentDir, 0755); err != nil {
		return fmt.Errorf("mkdir %s: %w", parentDir, err)
	}

	f, err := os.OpenFile(targetPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("open for append %s: %w", targetPath, err)
	}
	defer f.Close()

	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("append %s: %w", targetPath, err)
	}

	f.Sync()

	return nil
}

// safeJoin converts a resource-relative subpath into a secure OS path under BaseDir.
//
// This is the security boundary for DirResource. It performs multiple checks:
//  1. Rejects absolute paths (e.g., "/etc/passwd")
//  2. Normalizes the path using filepath.Clean
//  3. Rejects paths with ".." components after normalization
//  4. Verifies the final absolute path is contained within BaseDir
//
// The subpath should NOT have a leading slash.
//
// Returns an error if:
//   - subpath is an absolute path
//   - subpath escapes BaseDir (e.g., "../../../etc")
//   - filepath operations fail (rare)
//
// Examples:
//   - "notes.md" -> BaseDir + "/notes.md" ✓
//   - "a/../b/file.txt" -> BaseDir + "/b/file.txt" ✓ (normalized)
//   - "../etc/passwd" -> error ✗ (escape attempt)
//   - "a/../../secrets" -> error ✗ (escape attempt)
func (d *DirResource) safeJoin(subpath string) (string, error) {
	return vfsutil.SafeJoinBaseDir(d.BaseDir, subpath)
}
