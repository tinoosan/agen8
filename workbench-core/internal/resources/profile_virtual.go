package resources

import (
	"context"
	"fmt"

	"github.com/tinoosan/workbench-core/internal/store"
	"github.com/tinoosan/workbench-core/internal/vfs"
	"github.com/tinoosan/workbench-core/internal/vfsutil"
)

// VirtualProfileResource exposes user-scoped profile memory under the VFS mount "/profile",
// backed by a pluggable store.ProfileStore.
//
// Profile is global across runs and sessions and is intended for durable user facts and
// preferences (e.g. timezone, writing style, birthday). It mirrors the /memory contract,
// but with a different scope:
//   - /profile/profile.md     (read-only to agent; host-managed)
//   - /profile/update.md      (writable by agent; host evaluates + commits)
//   - /profile/commits.jsonl  (read-only, for debugging/audit)
type VirtualProfileResource struct {
	// BaseDir is the OS directory backing this resource (the sandbox root).
	//
	// VirtualProfileResource is store-backed; BaseDir is typically empty and only
	// exists to keep resources consistent and debuggable.
	BaseDir string

	// Mount is the virtual mount name used by the VFS.
	// Example: "profile" maps to the virtual namespace "/profile".
	Mount string

	Store store.ProfileStore
}

func NewVirtualProfileResource(s store.ProfileStore) (*VirtualProfileResource, error) {
	if s == nil {
		return nil, fmt.Errorf("profile store is required")
	}
	return &VirtualProfileResource{
		BaseDir: "",
		Mount:   vfs.MountProfile,
		Store:   s,
	}, nil
}

// List lists entries under subpath relative to the /profile mount.
func (pr *VirtualProfileResource) List(subpath string) ([]vfs.Entry, error) {
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
func (pr *VirtualProfileResource) Read(subpath string) ([]byte, error) {
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
func (pr *VirtualProfileResource) Write(subpath string, data []byte) error {
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
func (pr *VirtualProfileResource) Append(subpath string, data []byte) error {
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
