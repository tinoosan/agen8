package resources

import (
	"context"
	"fmt"

	"github.com/tinoosan/workbench-core/internal/store"
	"github.com/tinoosan/workbench-core/internal/vfs"
	"github.com/tinoosan/workbench-core/internal/vfsutil"
)

// StagingStore is the minimal store contract needed by a staging-style VFS resource.
//
// A staging resource exposes:
//   - <main>.md        (committed content; host-managed; agent can read)
//   - update.md        (staging file; agent can write/append)
//   - commits.jsonl    (audit log; host-managed; agent can read)
type StagingStore interface {
	store.StagingArea
	store.CommitLogReader

	// GetMain returns the committed main content for this mount (e.g. memory.md or profile.md).
	GetMain(ctx context.Context) (string, error)
}

// StagingResource is a parameterized virtual resource implementing the same
// VFS contract used by /memory and /profile.
type StagingResource struct {
	// BaseDir is unused by this resource, but kept for consistency/debugging.
	BaseDir string

	// Mount is the virtual mount name used by the VFS (e.g. "memory", "profile").
	Mount string

	// MainFile is the read-only committed file name (e.g. "memory.md", "profile.md").
	MainFile string

	Store StagingStore
}

// MemoryResource is kept for compatibility; it is an alias of StagingResource.
type MemoryResource = StagingResource

// ProfileResource is kept for compatibility; it is an alias of StagingResource.
type ProfileResource = StagingResource

type memoryStagingStore struct{ store.MemoryVFSStore }

func (s memoryStagingStore) GetMain(ctx context.Context) (string, error) {
	return s.MemoryVFSStore.GetMemory(ctx)
}

type profileStagingStore struct{ store.ProfileVFSStore }

func (s profileStagingStore) GetMain(ctx context.Context) (string, error) {
	return s.ProfileVFSStore.GetProfile(ctx)
}

func NewStagingResource(mount, mainFile string, s StagingStore) (*StagingResource, error) {
	if s == nil {
		return nil, fmt.Errorf("%s store is required", mount)
	}
	mount = mount
	mainFile = mainFile
	if mount == "" {
		return nil, fmt.Errorf("mount is required")
	}
	if mainFile == "" {
		return nil, fmt.Errorf("main file is required")
	}
	return &StagingResource{
		BaseDir:  "",
		Mount:    mount,
		MainFile: mainFile,
		Store:    s,
	}, nil
}

func NewMemoryResource(s store.MemoryVFSStore) (*MemoryResource, error) {
	if s == nil {
		return nil, fmt.Errorf("memory store is required")
	}
	return NewStagingResource(vfs.MountMemory, "memory.md", memoryStagingStore{s})
}

func NewProfileResource(s store.ProfileVFSStore) (*ProfileResource, error) {
	if s == nil {
		return nil, fmt.Errorf("profile store is required")
	}
	return NewStagingResource(vfs.MountProfile, "profile.md", profileStagingStore{s})
}

// List lists entries under subpath relative to the mount.
func (sr *StagingResource) List(subpath string) ([]vfs.Entry, error) {
	clean, _, err := vfsutil.NormalizeResourceSubpath(subpath)
	if err != nil {
		return nil, err
	}
	if clean == "" || clean == "." {
		return []vfs.Entry{
			{Path: sr.MainFile, IsDir: false},
			{Path: "update.md", IsDir: false},
			{Path: "commits.jsonl", IsDir: false},
		}, nil
	}
	return nil, fmt.Errorf("invalid subpath %q: cannot list non-root", subpath)
}

// Read reads a file at subpath relative to the mount.
func (sr *StagingResource) Read(subpath string) ([]byte, error) {
	if sr == nil || sr.Store == nil {
		return nil, fmt.Errorf("%s store not configured", srMount(sr))
	}
	clean, _, err := vfsutil.NormalizeResourceSubpath(subpath)
	if err != nil {
		return nil, fmt.Errorf("%s read: %w", sr.Mount, err)
	}
	if clean == "" || clean == "." {
		return nil, fmt.Errorf("%s read: path required (try %q)", sr.Mount, sr.MainFile)
	}
	switch clean {
	case sr.MainFile:
		s, err := sr.Store.GetMain(context.Background())
		return []byte(s), err
	case "update.md":
		s, err := sr.Store.GetUpdate(context.Background())
		return []byte(s), err
	case "commits.jsonl":
		s, err := sr.Store.GetCommitLog(context.Background())
		return []byte(s), err
	default:
		return nil, fmt.Errorf("%s read: unknown item %q (allowed: %s, update.md, commits.jsonl)", sr.Mount, clean, sr.MainFile)
	}
}

// Write replaces the file at subpath.
//
// Only update.md is writable (agent staging area).
func (sr *StagingResource) Write(subpath string, data []byte) error {
	if sr == nil || sr.Store == nil {
		return fmt.Errorf("%s store not configured", srMount(sr))
	}
	clean, _, err := vfsutil.NormalizeResourceSubpath(subpath)
	if err != nil {
		return fmt.Errorf("%s write: %w", sr.Mount, err)
	}
	if clean != "update.md" {
		return fmt.Errorf("%s write: only update.md is writable", sr.Mount)
	}
	return sr.Store.SetUpdate(context.Background(), string(data))
}

// Append appends bytes to update.md.
func (sr *StagingResource) Append(subpath string, data []byte) error {
	if sr == nil || sr.Store == nil {
		return fmt.Errorf("%s store not configured", srMount(sr))
	}
	clean, _, err := vfsutil.NormalizeResourceSubpath(subpath)
	if err != nil {
		return fmt.Errorf("%s append: %w", sr.Mount, err)
	}
	if clean != "update.md" {
		return fmt.Errorf("%s append: only update.md is writable", sr.Mount)
	}
	prev, err := sr.Store.GetUpdate(context.Background())
	if err != nil {
		return err
	}
	return sr.Store.SetUpdate(context.Background(), prev+string(data))
}

func srMount(sr *StagingResource) string {
	if sr == nil || sr.Mount == "" {
		return "resource"
	}
	return sr.Mount
}
