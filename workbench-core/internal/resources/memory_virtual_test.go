package resources_test

import (
	"testing"

	"github.com/tinoosan/workbench-core/internal/resources"
	"github.com/tinoosan/workbench-core/internal/store"
	"github.com/tinoosan/workbench-core/internal/vfs"
)

func TestVirtualMemoryResource_ReadWriteUpdateAndReadOnlyMemory(t *testing.T) {
	memStore, err := store.NewDiskMemoryStoreFromDir(t.TempDir())
	if err != nil {
		t.Fatalf("NewDiskMemoryStoreFromDir: %v", err)
	}
	mr, err := resources.NewVirtualMemoryResource(memStore)
	if err != nil {
		t.Fatalf("NewVirtualMemoryResource: %v", err)
	}

	fs := vfs.NewFS()
	fs.Mount(vfs.MountMemory, mr)

	entries, err := fs.List("/memory")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) == 0 {
		t.Fatalf("expected entries")
	}

	if err := fs.Write("/memory/update.md", []byte("hello")); err != nil {
		t.Fatalf("Write update.md: %v", err)
	}
	b, err := fs.Read("/memory/update.md")
	if err != nil {
		t.Fatalf("Read update.md: %v", err)
	}
	if string(b) != "hello" {
		t.Fatalf("unexpected update.md: %q", string(b))
	}

	if err := fs.Write("/memory/memory.md", []byte("nope")); err == nil {
		t.Fatalf("expected write to memory.md to be rejected")
	}
	if err := fs.Write("/memory/commits.jsonl", []byte("nope")); err == nil {
		t.Fatalf("expected write to commits.jsonl to be rejected")
	}
}
