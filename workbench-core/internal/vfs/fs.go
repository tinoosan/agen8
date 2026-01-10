package vfs

import (
	"fmt"
	"strings"
)

type FS struct {
	mounts map[string]Resource
}

func NewFS() *FS {
	return &FS{mounts: make(map[string]Resource)}
}

func (fs *FS) Mount(name string, r Resource) {
	if fs.mounts == nil {
		fs.mounts = make(map[string]Resource)
	}
	fs.mounts[name] = r
}

func (fs *FS) Resolve(vpath string) (Resource, string, error) {
	if vpath == "" {
		return nil, "", fmt.Errorf("path cannot be empty")
	}
	if !strings.HasPrefix(vpath, "/") {
		return nil, "", fmt.Errorf("path must start with '/'")
	}

	// Trim leading slashes so "/workspace/a/b" -> "workspace/a/b"
	trimmed := strings.TrimLeft(vpath, "/")
	if trimmed == "" {
		return nil, "", fmt.Errorf("path must include a mount, e.g. /workspace")
	}

	// Split into /workspace and a/b
	parts := strings.SplitN(trimmed, "/", 2)
	mountName := parts[0]

	r, ok := fs.mounts[mountName]
	if !ok {
		return nil, "", fmt.Errorf("unknown mount %q", mountName)
	}

	subpath := ""
	if len(parts) == 2 {
		subpath = parts[1] // "a/b"
	}
	return r, subpath, nil
}
