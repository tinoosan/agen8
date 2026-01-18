package resources

import (
	"context"
	"fmt"

	"github.com/tinoosan/workbench-core/internal/store"
	"github.com/tinoosan/workbench-core/internal/vfs"
	"github.com/tinoosan/workbench-core/internal/vfsutil"
)

// MemoryResource exposes run-scoped memory under the VFS mount "/memory",
// backed by a store.MemoryVFSStore.
type MemoryResource struct {
	// BaseDir is unused by this resource, but kept for consistency/debugging.
	BaseDir string

	// Mount is the virtual mount name used by the VFS.
	// Example: "memory" maps to the virtual namespace "/memory".
	Mount string

	Store store.MemoryVFSStore
}

func NewMemoryResource(s store.MemoryVFSStore) (*MemoryResource, error) {
	if s == nil {
		return nil, fmt.Errorf("memory store is required")
	}
	return &MemoryResource{
		BaseDir: "",
		Mount:   vfs.MountMemory,
		Store:   s,
	}, nil
}

// List lists entries under subpath relative to the /memory mount.
func (mr *MemoryResource) List(subpath string) ([]vfs.Entry, error) {
	clean, _, err := vfsutil.NormalizeResourceSubpath(subpath)
	if err != nil {
		return nil, err
	}
	if clean == "" || clean == "." {
		return []vfs.Entry{
			{Path: "memory.md", IsDir: false},
			{Path: "update.md", IsDir: false},
			{Path: "commits.jsonl", IsDir: false},
		}, nil
	}
	return nil, fmt.Errorf("invalid subpath %q: cannot list non-root", subpath)
}

// Read reads a file at subpath relative to the /memory mount.
func (mr *MemoryResource) Read(subpath string) ([]byte, error) {
	if mr == nil || mr.Store == nil {
		return nil, fmt.Errorf("memory store not configured")
	}
	clean, _, err := vfsutil.NormalizeResourceSubpath(subpath)
	if err != nil {
		return nil, fmt.Errorf("memory read: %w", err)
	}
	if clean == "" || clean == "." {
		return nil, fmt.Errorf("memory read: path required (try 'memory.md')")
	}
	switch clean {
	case "memory.md":
		s, err := mr.Store.GetMemory(context.Background())
		return []byte(s), err
	case "update.md":
		s, err := mr.Store.GetUpdate(context.Background())
		return []byte(s), err
	case "commits.jsonl":
		s, err := mr.Store.GetCommitLog(context.Background())
		return []byte(s), err
	default:
		return nil, fmt.Errorf("memory read: unknown item %q (allowed: memory.md, update.md, commits.jsonl)", clean)
	}
}

// Write replaces the file at subpath.
//
// Only update.md is writable (agent staging area).
func (mr *MemoryResource) Write(subpath string, data []byte) error {
	if mr == nil || mr.Store == nil {
		return fmt.Errorf("memory store not configured")
	}
	clean, _, err := vfsutil.NormalizeResourceSubpath(subpath)
	if err != nil {
		return fmt.Errorf("memory write: %w", err)
	}
	if clean != "update.md" {
		return fmt.Errorf("memory write: only update.md is writable")
	}
	return mr.Store.SetUpdate(context.Background(), string(data))
}

// Append appends bytes to update.md.
func (mr *MemoryResource) Append(subpath string, data []byte) error {
	if mr == nil || mr.Store == nil {
		return fmt.Errorf("memory store not configured")
	}
	clean, _, err := vfsutil.NormalizeResourceSubpath(subpath)
	if err != nil {
		return fmt.Errorf("memory append: %w", err)
	}
	if clean != "update.md" {
		return fmt.Errorf("memory append: only update.md is writable")
	}
	prev, err := mr.Store.GetUpdate(context.Background())
	if err != nil {
		return err
	}
	return mr.Store.SetUpdate(context.Background(), prev+string(data))
}

