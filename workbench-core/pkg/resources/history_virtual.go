package resources

import (
	"context"
	"fmt"

	"github.com/tinoosan/workbench-core/pkg/config"
	"github.com/tinoosan/workbench-core/pkg/fsutil"
	"github.com/tinoosan/workbench-core/pkg/store"
	"github.com/tinoosan/workbench-core/pkg/validate"
	"github.com/tinoosan/workbench-core/pkg/vfs"
	"github.com/tinoosan/workbench-core/pkg/vfsutil"
)

// HistoryResource exposes an immutable, append-only history log under the VFS mount "/history".
type HistoryResource struct {
	vfs.ReadOnlyResource
	// BaseDir is the OS directory backing this resource (for debug/inspection).
	BaseDir string
	// Mount is the virtual mount name used by the VFS.
	Mount string
	// SessionID is the session this history log belongs to.
	SessionID string
	// Store is the backing store for history.jsonl.
	Store store.HistoryReader
	// Appender is the host-side append interface for history.jsonl.
	Appender store.HistoryAppender
}

// NewHistoryResource creates a history resource backed by the provided store.
func NewHistoryResource(cfg config.Config, sessionID string, s store.HistoryStore) (*HistoryResource, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if err := validate.NonEmpty("sessionId", sessionID); err != nil {
		return nil, err
	}
	if s == nil {
		return nil, fmt.Errorf("history store is required")
	}
	baseDir := fsutil.GetSessionHistoryDir(cfg.DataDir, sessionID)
	return &HistoryResource{
		ReadOnlyResource: vfs.ReadOnlyResource{Name: "history"},
		BaseDir:          baseDir,
		Mount:            vfs.MountHistory,
		SessionID:        sessionID,
		Store:            s,
		Appender:         s,
	}, nil
}

func (hr *HistoryResource) SupportsNestedList() bool {
	return false
}

// List lists entries under subpath relative to BaseDir.
func (hr *HistoryResource) List(subpath string) ([]vfs.Entry, error) {
	clean, _, err := vfsutil.NormalizeResourceSubpath(subpath)
	if err != nil {
		return nil, err
	}
	if clean == "" || clean == "." {
		return []vfs.Entry{{Path: "history.jsonl", IsDir: false}}, nil
	}
	return nil, fmt.Errorf("invalid subpath %q: cannot list non-root", subpath)
}

// Read reads a file at subpath relative to BaseDir.
func (hr *HistoryResource) Read(subpath string) ([]byte, error) {
	if hr == nil || hr.Store == nil {
		return nil, fmt.Errorf("history store not configured")
	}
	clean, _, err := vfsutil.NormalizeResourceSubpath(subpath)
	if err != nil {
		return nil, fmt.Errorf("history read: %w", err)
	}
	if clean == "" || clean == "." {
		return nil, fmt.Errorf("history read: path required (try 'history.jsonl')")
	}
	if clean != "history.jsonl" {
		return nil, fmt.Errorf("history read: unknown item %q (allowed: history.jsonl)", clean)
	}
	return hr.Store.ReadAll(context.Background())
}

// Write replaces the file at subpath (creating parent directories if needed).
func (hr *HistoryResource) Write(subpath string, _ []byte) error {
	return hr.ReadOnlyResource.Write(subpath, nil)
}

// Append appends bytes to the file at subpath (creating parent directories if needed).
func (hr *HistoryResource) Append(subpath string, _ []byte) error {
	return hr.ReadOnlyResource.Append(subpath, nil)
}
