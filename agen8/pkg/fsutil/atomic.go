package fsutil

import (
	"fmt"
	"os"
	"path/filepath"
)

// WriteFileAtomic writes data to targetpath using an atomic rename operation.
func WriteFileAtomic(targetpath string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(targetpath)

	tmpFile, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmpFile.Name()
	defer func() {
		_ = os.Remove(tmpName)
	}()

	if _, err := tmpFile.Write(data); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmpFile.Sync(); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("sync temp file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Chmod(tmpName, perm); err != nil {
		return fmt.Errorf("chmod temp file: %w", err)
	}
	if err := os.Rename(tmpName, targetpath); err != nil {
		return fmt.Errorf("rename temp to target: %w", err)
	}
	return nil
}
