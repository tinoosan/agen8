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
	MemoryStore  store.DailyMemoryStore
	UserProfileStore store.UserProfileStore
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

func (f *Factory) Memory() (*DailyMemoryResource, error) {
	if strings.TrimSpace(f.DataDir) == "" {
		return nil, fmt.Errorf("DataDir is required for memory resource")
	}
	memoryDir := fsutil.GetMemoryDir(f.DataDir)
	return NewDailyMemoryResource(memoryDir, nil, nil)
}

func (f *Factory) UserProfile() (*UserProfileResource, error) {
	if f.UserProfileStore == nil {
		return nil, fmt.Errorf("user profile store is required")
	}
	return NewUserProfileResource(f.UserProfileStore)
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
	prof, err := f.UserProfile()
	if err != nil {
		return fmt.Errorf("create user profile: %w", err)
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

	if err := fs.Mount(vfs.MountWorkspace, ws); err != nil {
		return fmt.Errorf("mount %s: %w", vfs.MountWorkspace, err)
	}
	if err := fs.Mount(vfs.MountLog, tr); err != nil {
		return fmt.Errorf("mount %s: %w", vfs.MountLog, err)
	}
	if err := fs.Mount(vfs.MountResults, res); err != nil {
		return fmt.Errorf("mount %s: %w", vfs.MountResults, err)
	}
	if err := fs.Mount(vfs.MountInbox, inboxRes); err != nil {
		return fmt.Errorf("mount %s: %w", vfs.MountInbox, err)
	}
	if err := fs.Mount(vfs.MountOutbox, outboxRes); err != nil {
		return fmt.Errorf("mount %s: %w", vfs.MountOutbox, err)
	}
	if err := fs.Mount(vfs.MountMemory, mem); err != nil {
		return fmt.Errorf("mount %s: %w", vfs.MountMemory, err)
	}
	if err := fs.Mount(vfs.MountUserProfile, prof); err != nil {
		return fmt.Errorf("mount %s: %w", vfs.MountUserProfile, err)
	}
	// Legacy alias for older agents/tools/tests.
	_ = fs.Mount("profile", prof)
	if err := fs.Mount(vfs.MountHistory, hist); err != nil {
		return fmt.Errorf("mount %s: %w", vfs.MountHistory, err)
	}
	if err := fs.Mount(vfs.MountTrace, traceRes); err != nil {
		return fmt.Errorf("mount %s: %w", vfs.MountTrace, err)
	}

	return nil
}
