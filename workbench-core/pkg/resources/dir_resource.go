// Package resources provides concrete implementations of the vfs.Resource interface.
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
type DirResource struct {
	BaseDir string
	Mount   string
}

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

	if err := fsutil.WriteFileAtomic(targetPath, data, 0644); err != nil {
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

	_ = f.Sync()
	return nil
}

func (d *DirResource) safeJoin(subpath string) (string, error) {
	return vfsutil.SafeJoinBaseDir(d.BaseDir, subpath)
}
