// Package vfs provides a virtual filesystem abstraction that lets the agent/host
// interact with multiple resources through a single path-based interface.
//
// # Mounting
//
// Each Resource is mounted under a mount name and accessed via VFS paths:
//   - Mount "workspace" => VFS paths like "/workspace/notes.md"
//   - Mount "tools"     => VFS paths like "/tools/<toolId>"
//   - Mount "trace"     => VFS paths like "/trace/events.latest/3"
//
// Path conventions
//   - VFS paths always start with "/" and include a mount name.
//   - Resource methods receive a "subpath" relative to the mount, with no leading "/".
//   - Example: Resolve("/workspace/notes.md") => mount="workspace", subpath="notes.md".
//
// Listing
//   - List("/") returns the mounted namespaces as directory-like entries:
//     "/workspace", "/tools", "/trace", ...
//   - List("/<mount>") delegates to the resource and rewrites returned Entry.Path values
//     to include the mount prefix (so callers always see full VFS paths).
package vfs

import (
	"fmt"
	"sort"
	"strings"
)

// FS is a virtual filesystem that manages a collection of mounted resources.
// Each resource is mounted under a unique name and can be accessed via virtual
// paths like "/workspace/path/to/file".
type FS struct {
	mounts map[string]Resource
}

// NewFS creates and returns a new empty virtual filesystem.
func NewFS() *FS {
	return &FS{mounts: make(map[string]Resource)}
}

// Mount registers a resource under the given name. If a resource with the same
// name already exists, it will be replaced. The name should be a simple
// identifier (e.g., "workspace", "data").
//
// Mount names may include "/" to support nested mounts (e.g. "workspace/cache").
// When resolving a VFS path, the longest matching mount prefix wins.
func (fs *FS) Mount(name string, r Resource) {
	if fs.mounts == nil {
		fs.mounts = make(map[string]Resource)
	}
	fs.mounts[name] = r
}

// Resolve takes a virtual path and returns the mount name, corresponding mounted resource,
// the subpath within that resource, and any error encountered.
//
// The path must start with "/" and include a mount name (e.g., "/workspace/a/b").
// For the path "/workspace/a/b", it returns the mount name "workspace", the resource
// mounted as "workspace", and the subpath "a/b".
//
// Returns an error if the path is empty, doesn't start with "/", or references
// an unknown mount.
func (fs *FS) Resolve(vpath string) (mountName string, r Resource, subpath string, err error) {
	if vpath == "" {
		return "", nil, "", fmt.Errorf("path cannot be empty")
	}
	if !strings.HasPrefix(vpath, "/") {
		return "", nil, "", fmt.Errorf("path must start with '/'")
	}

	// Trim leading slashes so "/workspace/a/b" -> "workspace/a/b".
	trimmed := strings.TrimLeft(vpath, "/")
	if trimmed == "" {
		return "", nil, "", fmt.Errorf("path must include a mount, e.g. /workspace")
	}

	// Find the longest matching mount prefix.
	//
	// This allows nested mounts like "workspace/cache" to override "workspace"
	// when resolving "/workspace/cache/x".
	best := ""
	for mn := range fs.mounts {
		if trimmed == mn || strings.HasPrefix(trimmed, mn+"/") {
			if len(mn) > len(best) {
				best = mn
			}
		}
	}
	if best == "" {
		return "", nil, "", fmt.Errorf("not found: unknown mount for path %q", vpath)
	}

	r = fs.mounts[best]
	subpath = ""
	if trimmed != best {
		subpath = strings.TrimPrefix(trimmed, best+"/")
	}
	return best, r, subpath, nil
}

// Read reads the contents of a file at the given VFS path.
// It returns an error if the path is invalid or if the file cannot be read.
func (fs *FS) Read(vpath string) ([]byte, error) {
	mountName, r, subpath, err := fs.Resolve(vpath)
	if err != nil {
		return nil, err
	}

	b, err := r.Read(subpath)
	if err != nil {
		return nil, fmt.Errorf("read %s:%s: %w", mountName, subpath, err)
	}
	return b, nil
}

// Write writes the given data to a file at the given VFS path.
// It returns an error if the path is invalid or if the file cannot be written.
func (fs *FS) Write(vpath string, data []byte) error {
	mountName, r, subpath, err := fs.Resolve(vpath)
	if err != nil {
		return err
	}

	if err := r.Write(subpath, data); err != nil {
		return fmt.Errorf("write %s:%s: %w", mountName, subpath, err)
	}
	return nil
}

// Append appends the given data to a file at the given VFS path.
// It returns an error if the path is invalid or if the file cannot be appended to.
func (fs *FS) Append(vpath string, data []byte) error {
	mountName, r, subpath, err := fs.Resolve(vpath)
	if err != nil {
		return err
	}

	err = r.Append(subpath, data)
	if err != nil {
		return fmt.Errorf("append %s:%s: %w", mountName, subpath, err)
	}
	return nil
}

// List returns a list of entries under the given subpath.
// The subpath must be relative to the base directory.
func (fs *FS) List(vpath string) ([]Entry, error) {

	if vpath == "/" {
		return fs.listRoot()
	}
	mountName, r, subpath, err := fs.Resolve(vpath)
	if err != nil {
		return nil, err
	}

	entries, err := r.List(subpath)
	if err != nil {
		return nil, fmt.Errorf("list %s:%s: %w", mountName, subpath, err)
	}
	out := make([]Entry, 0, len(entries))
	for _, e := range entries {
		vp := "/" + mountName
		if e.Path != "" {
			vp += "/" + e.Path
		}
		e.Path = vp
		out = append(out, e)
	}
	return out, nil
}

func (fs *FS) listRoot() ([]Entry, error) {
	if len(fs.mounts) == 0 {
		return []Entry{}, nil
	}
	mountNames := make([]string, 0, len(fs.mounts))
	for mn := range fs.mounts {
		mountNames = append(mountNames, mn)
	}
	sort.Strings(mountNames)

	entries := make([]Entry, 0, len(mountNames))
	for _, mn := range mountNames {
		entries = append(entries, Entry{
			Path:  "/" + mn,
			IsDir: true,
		})
	}
	return entries, nil
}
