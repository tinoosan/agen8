// Package vfs provides a virtual filesystem abstraction that allows mounting
// multiple resources under named paths and resolving virtual paths to their
// underlying resources.
package vfs

import (
	"fmt"
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
// identifier without slashes (e.g., "workspace", "data").
func (fs *FS) Mount(name string, r Resource) {
	if fs.mounts == nil {
		fs.mounts = make(map[string]Resource)
	}
	fs.mounts[name] = r
}

// Resolve takes a virtual path and returns the corresponding mounted resource,
// the subpath within that resource, and any error encountered.
//
// The path must start with "/" and include a mount name (e.g., "/workspace/a/b").
// For the path "/workspace/a/b", it returns the resource mounted as "workspace"
// and the subpath "a/b".
//
// Returns an error if the path is empty, doesn't start with "/", or references
// an unknown mount.
func (fs *FS) Resolve(vpath string) (Resource, string, string, error) {
	if vpath == "" {
		return nil, "", "", fmt.Errorf("path cannot be empty")
	}
	if !strings.HasPrefix(vpath, "/") {
		return nil, "", "", fmt.Errorf("path must start with '/'")
	}

	// Trim leading slashes so "/workspace/a/b" -> "workspace/a/b"
	trimmed := strings.TrimLeft(vpath, "/")
	if trimmed == "" {
		return nil, "", "", fmt.Errorf("path must include a mount, e.g. /workspace")
	}

	// Split into /workspace and a/b
	parts := strings.SplitN(trimmed, "/", 2)
	mountName := parts[0]

	r, ok := fs.mounts[mountName]
	if !ok {
		return nil, "", "", fmt.Errorf("unknown mount %q", mountName)
	}

	subpath := ""
	if len(parts) == 2 {
		subpath = parts[1] // "a/b"
	}
	return r, subpath, mountName, nil
}
