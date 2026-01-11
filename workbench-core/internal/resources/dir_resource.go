package resources

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/tinoosan/workbench-core/internal/fsutil"
	"github.com/tinoosan/workbench-core/internal/vfs"
)

type DirResource struct {
	BaseDir string // e.g data/runs/run-123/workspace
	Mount   string // e.g workspace
}

// List returns a list of entries under the given subpath.
// The subpath must be relative to the base directory.
func (d *DirResource) List(subpath string) ([]vfs.Entry, error) {
	targetPath, err := d.safeJoin(subpath)
	if err != nil {
		return nil, fmt.Errorf("safeJoin %s: %w", subpath, err)
	}

	des, err := os.ReadDir(targetPath)
	if err != nil {
		return nil, fmt.Errorf("list %s: %w", targetPath, err)
	}
	entries := make([]vfs.Entry, 0, len(des))
	for _, de := range des {
		info, err := de.Info()
		if err != nil {
			return nil, fmt.Errorf("stat entry %s: %w", de.Name(), err)
		}

		childSubpath := strings.TrimLeft(filepath.ToSlash(filepath.Join(subpath, de.Name())), "/")
		childVPath := path.Join("/", d.Mount, childSubpath)

		e := vfs.Entry{
			Path:  childVPath,
			IsDir: de.IsDir(),
			Size:  info.Size(),
			ModTime: info.ModTime(),
			HasSize: true,
			HasModTime: true,
		}

		entries = append(entries, e)
	}

	return entries, nil
}

// Read returns the contents of the file at the given subpath.
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

// safeJoin converts a resource-relative subpath (no leading "/") into an OS path under baseDir.
// It rejects any subpath that would escape baseDir e.g. data/runs/run-123/workspace.
func (d *DirResource) safeJoin(subpath string) (string, error) {
	// Clean turns things like "a/../b" into "b" and "a/../../x" into "../x".
	clean := filepath.Clean(subpath)

	if filepath.IsAbs(clean) {
		return "", fmt.Errorf("absolute paths not allowed")
	}

	// filepath.Clean can return "." for empty paths; that’s fine for listing baseDir.
	// But we reject any path that starts with ".." after cleaning.
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("invalid path: escapes mount root")
	}

	joined := filepath.Join(d.BaseDir, clean)

	// Stronger containment check: compare absolute paths.
	baseAbs, err := filepath.Abs(d.BaseDir)
	if err != nil {
		return "", fmt.Errorf("abs baseDir: %w", err)
	}

	joinedAbs, err := filepath.Abs(joined)
	if err != nil {
		return "", fmt.Errorf("abs joined: %w", err)
	}

	// Ensure joinedAbs is inside baseAbs (path boundary aware)
	rel, err := filepath.Rel(baseAbs, joinedAbs)
	if err != nil {
		return "", fmt.Errorf("rel: %w", err)
	}

	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("invalid path: escapes mount root")
	}

	return joinedAbs, nil
}
