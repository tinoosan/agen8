package resources

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tinoosan/workbench-core/internal/config"
	"github.com/tinoosan/workbench-core/internal/fsutil"
	"github.com/tinoosan/workbench-core/internal/vfs"
)

// MemoryResource exposes persistent, cross-run agent memory under the VFS mount "/memory".
//
// This resource is intentionally small and explicit:
//   - /memory/memory.md is the accumulated long-term memory (read-only to the agent)
//   - /memory/update.md is a short staging file for new memory updates (writeable)
//
// Host policy:
//   - The host reads /memory/memory.md and injects it into the system prompt.
//   - After each turn, the host reads /memory/update.md, appends it to memory.md,
//     and clears update.md.
//
// Keeping memory as a resource makes it discoverable and testable via the same VFS
// contract as other mounts, while preserving a simple evaluator boundary (the host
// decides what gets committed from update.md into memory.md).
type MemoryResource struct {
	// BaseDir is the OS directory backing this resource (the sandbox root).
	// All operations are confined under BaseDir. The resource must reject any
	// subpath that would escape BaseDir (e.g. "..", absolute paths).
	//
	// BaseDir is an implementation detail; callers interact via virtual paths
	// like "/memory/memory.md" through the VFS.
	BaseDir string

	// Mount is the virtual mount name used by the VFS.
	// Example: "memory" maps to the virtual namespace "/memory".
	Mount string
}

func NewMemoryResource() (*MemoryResource, error) {
	baseDir := fsutil.GetAgentDir(config.DataDir)
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("error creating memory directory %s: %w", baseDir, err)
	}
	// Ensure files exist so fs.read works even before any updates.
	for _, name := range []string{"memory.md", "update.md"} {
		p := filepath.Join(baseDir, name)
		if _, err := os.Stat(p); err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				return nil, fmt.Errorf("error checking %s: %w", p, err)
			}
			if err := os.WriteFile(p, []byte{}, 0644); err != nil {
				return nil, fmt.Errorf("error creating %s: %w", p, err)
			}
		}
	}
	return &MemoryResource{BaseDir: baseDir, Mount: vfs.MountMemory}, nil
}

// List lists entries under subpath relative to BaseDir.
// subpath is resource-relative (no leading "/").
// List("") lists the resource root.
func (mr *MemoryResource) List(subpath string) ([]vfs.Entry, error) {
	subpath = strings.TrimSpace(subpath)
	if subpath == "" || subpath == "." {
		return []vfs.Entry{
			{Path: "memory.md", IsDir: false},
			{Path: "update.md", IsDir: false},
		}, nil
	}
	return nil, fmt.Errorf("invalid subpath %q: cannot list non-root", subpath)
}

// Read reads a file at subpath relative to BaseDir.
// subpath is resource-relative (no leading "/").
func (mr *MemoryResource) Read(subpath string) ([]byte, error) {
	subpath = strings.TrimSpace(subpath)
	if subpath == "" || subpath == "." {
		return nil, fmt.Errorf("memory read: path required (try 'memory.md')")
	}
	if strings.HasPrefix(subpath, "/") {
		return nil, fmt.Errorf("memory read: absolute paths not allowed: %q", subpath)
	}
	if subpath != "memory.md" && subpath != "update.md" {
		return nil, fmt.Errorf("memory read: unknown item %q (allowed: memory.md, update.md)", subpath)
	}
	return os.ReadFile(filepath.Join(mr.BaseDir, subpath))
}

// Write replaces the file at subpath (creating parent directories if needed).
// subpath is resource-relative (no leading "/").
func (mr *MemoryResource) Write(subpath string, data []byte) error {
	subpath = strings.TrimSpace(subpath)
	if strings.HasPrefix(subpath, "/") {
		return fmt.Errorf("memory write: absolute paths not allowed: %q", subpath)
	}
	if subpath != "update.md" {
		return fmt.Errorf("memory write: only update.md is writable")
	}
	return os.WriteFile(filepath.Join(mr.BaseDir, subpath), data, 0644)
}

// Append appends bytes to the file at subpath (creating parent directories if needed).
// subpath is resource-relative (no leading "/").
func (mr *MemoryResource) Append(subpath string, data []byte) error {
	subpath = strings.TrimSpace(subpath)
	if strings.HasPrefix(subpath, "/") {
		return fmt.Errorf("memory append: absolute paths not allowed: %q", subpath)
	}
	if subpath != "update.md" {
		return fmt.Errorf("memory append: only update.md is writable")
	}
	f, err := os.OpenFile(filepath.Join(mr.BaseDir, subpath), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(data)
	return err
}
