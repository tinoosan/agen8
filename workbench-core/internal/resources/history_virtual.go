package resources

import (
	"context"
	"fmt"
	"strings"

	"github.com/tinoosan/workbench-core/internal/config"
	"github.com/tinoosan/workbench-core/internal/fsutil"
	"github.com/tinoosan/workbench-core/internal/store"
	"github.com/tinoosan/workbench-core/internal/vfs"
	"github.com/tinoosan/workbench-core/internal/vfsutil"
)

// HistoryResource exposes an immutable, append-only history log under the VFS mount "/history".
//
// High-level model (session-scoped)
//
// History records raw interactions between:
//   - users (inputs)
//   - agents (outputs and tool decisions)
//   - the environment/host (host ops, policy decisions, etc)
//
// Each record is appended as one JSON object per line (JSONL) with metadata:
//   - timestamp
//   - origin (user/agent/env)
//   - model (when applicable)
//
// This forms a verifiable source of truth for post-hoc analysis, debugging, and
// compliance verification.
//
// Scope
//   - History is scoped to a session:
//     data/sessions/<sessionId>/history/history.jsonl
//   - A session can contain multiple runs (including sub-agent runs).
//
// Access policy
// - The agent can read history via VFS, but cannot write to it.
// - The host appends to history via its event emitter sinks.
type HistoryResource struct {
	// BaseDir is the OS directory backing this resource (the sandbox root).
	// All operations are confined under BaseDir. The resource must reject any
	// subpath that would escape BaseDir (e.g. "..", absolute paths).
	//
	// BaseDir is an implementation detail; callers interact via virtual paths
	// like "/history/history.jsonl" through the VFS.
	BaseDir string

	// Mount is the virtual mount name used by the VFS.
	// Example: "history" maps to the virtual namespace "/history".
	Mount string

	// SessionID is the session this history log belongs to.
	SessionID string

	// Store is the backing store for history.jsonl.
	//
	// This is the storage boundary; HistoryResource does not perform direct filesystem IO.
	Store store.HistoryStore
}

func NewSessionHistoryResource(sessionID string) (*HistoryResource, error) {
	if strings.TrimSpace(sessionID) == "" {
		return nil, fmt.Errorf("sessionId cannot be empty")
	}
	// Keep BaseDir for debug output / inspection, but store owns the IO.
	baseDir := fsutil.GetSessionHistoryDir(config.DataDir, sessionID)

	s, err := store.NewDiskHistoryStore(sessionID)
	if err != nil {
		return nil, err
	}
	return &HistoryResource{
		BaseDir:   baseDir,
		Mount:     vfs.MountHistory,
		SessionID: sessionID,
		Store:     s,
	}, nil
}

// List lists entries under subpath relative to BaseDir.
// subpath is resource-relative (no leading "/").
// List("") lists the resource root.
func (hr *HistoryResource) List(subpath string) ([]vfs.Entry, error) {
	clean, _, err := vfsutil.NormalizeResourceSubpath(subpath)
	if err != nil {
		return nil, err
	}
	if clean == "" || clean == "." {
		return []vfs.Entry{
			{Path: "history.jsonl", IsDir: false},
		}, nil
	}
	return nil, fmt.Errorf("invalid subpath %q: cannot list non-root", subpath)
}

// Read reads a file at subpath relative to BaseDir.
// subpath is resource-relative (no leading "/").
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
// subpath is resource-relative (no leading "/").
func (hr *HistoryResource) Write(subpath string, _ []byte) error {
	_ = subpath
	return fmt.Errorf("history write: not supported (history is host-owned and append-only)")
}

// Append appends bytes to the file at subpath (creating parent directories if needed).
// subpath is resource-relative (no leading "/").
func (hr *HistoryResource) Append(subpath string, _ []byte) error {
	_ = subpath
	return fmt.Errorf("history append: not supported (history is host-owned and append-only)")
}

