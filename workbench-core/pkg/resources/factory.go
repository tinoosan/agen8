package resources

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tinoosan/workbench-core/pkg/config"
	"github.com/tinoosan/workbench-core/pkg/fsutil"
	"github.com/tinoosan/workbench-core/pkg/store"
	"github.com/tinoosan/workbench-core/pkg/validate"
	"github.com/tinoosan/workbench-core/pkg/vfs"
)

// Factory centralizes construction of core VFS resources for a given run/session.
//
// Callers must supply concrete store implementations; this package does not
// create disk/sqlite stores.
type Factory struct {
	DataDir   string
	SessionID string
	RunID     string

	ResultsStore store.ResultsStore
	MemoryStore  store.MemoryStore
	ProfileStore store.ProfileStore
	HistoryStore store.HistoryStore
	TraceStore   store.TraceStore
}

func (f *Factory) cfg() (config.Config, error) {
	cfg := config.Config{DataDir: strings.TrimSpace(f.DataDir)}
	return cfg, cfg.Validate()
}

func (f *Factory) Workspace() (*DirResource, error) {
	cfg, err := f.cfg()
	if err != nil {
		return nil, err
	}
	if err := validate.NonEmpty("RunID", f.RunID); err != nil {
		return nil, err
	}
	return NewWorkspace(cfg, f.RunID)
}

func (f *Factory) Trace() (*TraceResource, error) {
	cfg, err := f.cfg()
	if err != nil {
		return nil, err
	}
	if err := validate.NonEmpty("RunID", f.RunID); err != nil {
		return nil, err
	}
	return NewTraceResource(cfg, f.RunID)
}

func (f *Factory) Results() (*ResultsResource, error) {
	if f.ResultsStore == nil {
		return nil, fmt.Errorf("results store is required")
	}
	return NewResultsResource(f.ResultsStore)
}

func (f *Factory) Memory() (*MemoryResource, error) {
	if err := validate.NonEmpty("RunID", f.RunID); err != nil {
		return nil, err
	}
	if f.MemoryStore == nil {
		return nil, fmt.Errorf("memory store is required")
	}
	return NewMemoryResource(f.MemoryStore)
}

func (f *Factory) Profile() (*ProfileResource, error) {
	if f.ProfileStore == nil {
		return nil, fmt.Errorf("profile store is required")
	}
	return NewProfileResource(f.ProfileStore)
}

func (f *Factory) History() (*HistoryResource, error) {
	cfg, err := f.cfg()
	if err != nil {
		return nil, err
	}
	if err := validate.NonEmpty("SessionID", f.SessionID); err != nil {
		return nil, err
	}
	if f.HistoryStore == nil {
		return nil, fmt.Errorf("history store is required")
	}
	return NewHistoryResource(cfg, f.SessionID, f.HistoryStore)
}

// MountAll mounts the core resources into fs.
//
// Note: /project is intentionally excluded since it depends on a user-provided OS path.
func (f *Factory) MountAll(fs *vfs.FS) error {
	if fs == nil {
		return fmt.Errorf("fs is required")
	}

	ws, err := f.Workspace()
	if err != nil {
		return fmt.Errorf("create workspace: %w", err)
	}
	tr, err := f.Trace()
	if err != nil {
		return fmt.Errorf("create trace: %w", err)
	}
	res, err := f.Results()
	if err != nil {
		return fmt.Errorf("create results: %w", err)
	}
	mem, err := f.Memory()
	if err != nil {
		return fmt.Errorf("create memory: %w", err)
	}
	prof, err := f.Profile()
	if err != nil {
		return fmt.Errorf("create profile: %w", err)
	}
	hist, err := f.History()
	if err != nil {
		return fmt.Errorf("create history: %w", err)
	}

	traceDir := filepath.Join(ws.BaseDir, "trace")
	if err := os.MkdirAll(traceDir, 0755); err != nil {
		return fmt.Errorf("create trace dir: %w", err)
	}
	traceRes, err := NewDirResource(traceDir, vfs.MountTrace)
	if err != nil {
		return fmt.Errorf("create trace resource: %w", err)
	}

	runDir := fsutil.GetRunDir(f.DataDir, f.RunID)
	inboxDir := filepath.Join(runDir, vfs.MountInbox)
	if err := os.MkdirAll(inboxDir, 0755); err != nil {
		return fmt.Errorf("create inbox dir: %w", err)
	}
	inboxRes, err := NewDirResource(inboxDir, vfs.MountInbox)
	if err != nil {
		return fmt.Errorf("create inbox resource: %w", err)
	}

	outboxDir := filepath.Join(runDir, vfs.MountOutbox)
	if err := os.MkdirAll(outboxDir, 0755); err != nil {
		return fmt.Errorf("create outbox dir: %w", err)
	}
	outboxRes, err := NewDirResource(outboxDir, vfs.MountOutbox)
	if err != nil {
		return fmt.Errorf("create outbox resource: %w", err)
	}

	fs.Mount(vfs.MountScratch, ws)
	fs.Mount(vfs.MountLog, tr)
	fs.Mount(vfs.MountResults, res)
	fs.Mount(vfs.MountInbox, inboxRes)
	fs.Mount(vfs.MountOutbox, outboxRes)
	fs.Mount(vfs.MountMemory, mem)
	fs.Mount(vfs.MountProfile, prof)
	fs.Mount(vfs.MountHistory, hist)
	fs.Mount(vfs.MountTrace, traceRes)

	return nil
}
