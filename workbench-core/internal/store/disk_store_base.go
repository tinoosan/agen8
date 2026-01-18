package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tinoosan/workbench-core/internal/validate"
)

// DiskStore provides shared initialization helpers for disk-backed stores.
//
// Stores can use either:
// - Path for single-file stores (e.g. history.jsonl), or
// - Dir for directory stores (e.g. memory/ with multiple files).
type DiskStore struct {
	Path string
	Dir  string
}

// EnsureFile ensures the parent directory exists and the file exists (empty if newly created).
func (d *DiskStore) EnsureFile(path string) error {
	if d == nil {
		return fmt.Errorf("disk store is nil")
	}
	if err := validate.NonEmpty("path", path); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	if _, err := os.Stat(path); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err := os.WriteFile(path, []byte{}, 0644); err != nil {
			return err
		}
	}
	return nil
}

// EnsureDir ensures dir exists and ensures each named file exists under it.
func (d *DiskStore) EnsureDir(dir string, files ...string) error {
	if d == nil {
		return fmt.Errorf("disk store is nil")
	}
	if err := validate.NonEmpty("dir", dir); err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	for _, name := range files {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if err := d.EnsureFile(filepath.Join(dir, name)); err != nil {
			return err
		}
	}
	return nil
}

