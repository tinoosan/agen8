package resources

import (
	"context"
	"fmt"

	"github.com/tinoosan/workbench-core/internal/store"
	"github.com/tinoosan/workbench-core/internal/vfs"
	"github.com/tinoosan/workbench-core/internal/vfsutil"
)

// ProfileResource exposes user-scoped profile memory under the VFS mount "/profile",
// backed by a store.ProfileVFSStore.
type ProfileResource struct {
	// BaseDir is unused by this resource, but kept for consistency/debugging.
	BaseDir string

	// Mount is the virtual mount name used by the VFS.
	// Example: "profile" maps to the virtual namespace "/profile".
	Mount string

	Store store.ProfileVFSStore
}

func NewProfileResource(s store.ProfileVFSStore) (*ProfileResource, error) {
	if s == nil {
		return nil, fmt.Errorf("profile store is required")
	}
	return &ProfileResource{
		BaseDir: "",
		Mount:   vfs.MountProfile,
		Store:   s,
	}, nil
}

// List lists entries under subpath relative to the /profile mount.
func (pr *ProfileResource) List(subpath string) ([]vfs.Entry, error) {
	clean, _, err := vfsutil.NormalizeResourceSubpath(subpath)
	if err != nil {
		return nil, err
	}
	if clean == "" || clean == "." {
		return []vfs.Entry{
			{Path: "profile.md", IsDir: false},
			{Path: "update.md", IsDir: false},
			{Path: "commits.jsonl", IsDir: false},
		}, nil
	}
	return nil, fmt.Errorf("invalid subpath %q: cannot list non-root", subpath)
}

// Read reads a file at subpath relative to the /profile mount.
func (pr *ProfileResource) Read(subpath string) ([]byte, error) {
	if pr == nil || pr.Store == nil {
		return nil, fmt.Errorf("profile store not configured")
	}
	clean, _, err := vfsutil.NormalizeResourceSubpath(subpath)
	if err != nil {
		return nil, fmt.Errorf("profile read: %w", err)
	}
	if clean == "" || clean == "." {
		return nil, fmt.Errorf("profile read: path required (try 'profile.md')")
	}
	switch clean {
	case "profile.md":
		s, err := pr.Store.GetProfile(context.Background())
		return []byte(s), err
	case "update.md":
		s, err := pr.Store.GetUpdate(context.Background())
		return []byte(s), err
	case "commits.jsonl":
		s, err := pr.Store.GetCommitLog(context.Background())
		return []byte(s), err
	default:
		return nil, fmt.Errorf("profile read: unknown item %q (allowed: profile.md, update.md, commits.jsonl)", clean)
	}
}

// Write replaces the file at subpath.
//
// Only update.md is writable (agent staging area).
func (pr *ProfileResource) Write(subpath string, data []byte) error {
	if pr == nil || pr.Store == nil {
		return fmt.Errorf("profile store not configured")
	}
	clean, _, err := vfsutil.NormalizeResourceSubpath(subpath)
	if err != nil {
		return fmt.Errorf("profile write: %w", err)
	}
	if clean != "update.md" {
		return fmt.Errorf("profile write: only update.md is writable")
	}
	return pr.Store.SetUpdate(context.Background(), string(data))
}

// Append appends bytes to update.md.
func (pr *ProfileResource) Append(subpath string, data []byte) error {
	if pr == nil || pr.Store == nil {
		return fmt.Errorf("profile store not configured")
	}
	clean, _, err := vfsutil.NormalizeResourceSubpath(subpath)
	if err != nil {
		return fmt.Errorf("profile append: %w", err)
	}
	if clean != "update.md" {
		return fmt.Errorf("profile append: only update.md is writable")
	}
	prev, err := pr.Store.GetUpdate(context.Background())
	if err != nil {
		return err
	}
	return pr.Store.SetUpdate(context.Background(), prev+string(data))
}

