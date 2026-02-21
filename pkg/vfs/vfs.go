// Package vfs provides a virtual filesystem abstraction that lets the agent/host
// interact with multiple resources through a single path-based interface.
package vfs

import (
	"context"
	"errors"
	"fmt"
	iofs "io/fs"
	"sort"
	"strings"

	"github.com/tinoosan/agen8/pkg/store"
	"github.com/tinoosan/agen8/pkg/types"
)

// FS is a virtual filesystem that manages a collection of mounted resources.
type FS struct {
	mounts map[string]Resource
}

// NewFS creates and returns a new empty virtual filesystem.
func NewFS() *FS {
	return &FS{mounts: make(map[string]Resource)}
}

// Mount registers a resource under the given name.
func (fs *FS) Mount(name string, r Resource) error {
	if err := validateMountPath(name); err != nil {
		return err
	}
	if fs.mounts == nil {
		fs.mounts = make(map[string]Resource)
	}
	fs.mounts[name] = r
	return nil
}

func validateMountPath(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("mount path cannot be empty")
	}
	if strings.HasPrefix(name, "/") {
		return fmt.Errorf("mount path must not start with /")
	}
	for _, seg := range strings.Split(name, "/") {
		if seg == "" {
			return fmt.Errorf("mount path cannot contain empty segments")
		}
		switch seg {
		case ".", "..":
			return fmt.Errorf("mount path contains invalid segment %q", seg)
		}
	}
	return nil
}

// Resolve takes a virtual path and returns the mount name, mounted resource, subpath, and any error.
func (fs *FS) Resolve(vpath string) (mountName string, r Resource, subpath string, err error) {
	if vpath == "" {
		return "", nil, "", fmt.Errorf("path cannot be empty")
	}
	if !strings.HasPrefix(vpath, "/") {
		return "", nil, "", fmt.Errorf("path must start with '/'")
	}

	trimmed := strings.TrimLeft(vpath, "/")
	if trimmed == "" {
		return "", nil, "", fmt.Errorf("path must include a mount, e.g. /workspace")
	}

	best := ""
	for mn := range fs.mounts {
		if trimmed == mn || strings.HasPrefix(trimmed, mn+"/") {
			if len(mn) > len(best) {
				best = mn
			}
		}
	}
	if best == "" {
		return "", nil, "", errors.Join(store.ErrNotFound, iofs.ErrNotExist, fmt.Errorf("not found: unknown mount for path %q", vpath))
	}

	r = fs.mounts[best]
	subpath = ""
	if trimmed != best {
		subpath = strings.TrimPrefix(trimmed, best+"/")
	}
	return best, r, subpath, nil
}

// Read reads the contents of a file at the given VFS path.
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
func (fs *FS) Append(vpath string, data []byte) error {
	mountName, r, subpath, err := fs.Resolve(vpath)
	if err != nil {
		return err
	}

	if err := r.Append(subpath, data); err != nil {
		return fmt.Errorf("append %s:%s: %w", mountName, subpath, err)
	}
	return nil
}

// Search searches a mounted resource (if supported) for a query.
func (fs *FS) Search(ctx context.Context, vpath, query string, limit int) ([]types.SearchResult, error) {
	mountName, r, subpath, err := fs.Resolve(vpath)
	if err != nil {
		return nil, err
	}
	searchable, ok := r.(Searchable)
	if !ok {
		return nil, errors.Join(store.ErrInvalid, fmt.Errorf("search not supported for mount %q", mountName))
	}
	return searchable.Search(ctx, subpath, query, limit)
}

// List returns a list of entries under the given subpath.
func (fs *FS) List(vpath string) ([]Entry, error) {
	if vpath == "/" {
		return fs.rootEntries()
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

func (fs *FS) rootEntries() ([]Entry, error) {
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
