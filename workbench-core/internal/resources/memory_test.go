package resources

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tinoosan/workbench-core/internal/config"
	"github.com/tinoosan/workbench-core/internal/vfs"
)

func TestMemoryResource_ReadWriteUpdate(t *testing.T) {
	tmp := t.TempDir()
	old := config.DataDir
	config.DataDir = tmp
	defer func() { config.DataDir = old }()

	mr, err := NewMemoryResource()
	if err != nil {
		t.Fatalf("NewMemoryResource: %v", err)
	}
	if mr.Mount != vfs.MountMemory {
		t.Fatalf("unexpected mount %q", mr.Mount)
	}

	if _, err := mr.Read("memory.md"); err != nil {
		t.Fatalf("Read memory.md: %v", err)
	}

	if err := mr.Write("update.md", []byte("hello")); err != nil {
		t.Fatalf("Write update.md: %v", err)
	}
	b, err := mr.Read("update.md")
	if err != nil {
		t.Fatalf("Read update.md: %v", err)
	}
	if string(b) != "hello" {
		t.Fatalf("unexpected update.md: %q", string(b))
	}

	// memory.md is read-only
	if err := mr.Write("memory.md", []byte("nope")); err == nil {
		t.Fatalf("expected error writing memory.md")
	}

	// Ensure on disk
	if _, err := os.Stat(filepath.Join(mr.BaseDir, "memory.md")); err != nil {
		t.Fatalf("expected memory.md to exist: %v", err)
	}
	if _, err := os.Stat(filepath.Join(mr.BaseDir, "update.md")); err != nil {
		t.Fatalf("expected update.md to exist: %v", err)
	}
}
