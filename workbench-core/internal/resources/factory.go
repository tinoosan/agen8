package resources

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tinoosan/workbench-core/internal/config"
	"github.com/tinoosan/workbench-core/pkg/fsutil"
	"github.com/tinoosan/workbench-core/internal/store"
	internaltools "github.com/tinoosan/workbench-core/internal/tools"
	"github.com/tinoosan/workbench-core/pkg/validate"
	"github.com/tinoosan/workbench-core/pkg/vfs"
	pkgtools "github.com/tinoosan/workbench-core/pkg/tools"
)

// Factory centralizes construction of core VFS resources for a given run/session.
//
// It also owns default store wiring for virtual resources (results/memory/profile/history)
// and exposes those stores for callers that need them (e.g. tool runner, committers).
type Factory struct {
	DataDir   string
	SessionID string
	RunID     string

	// Stores for virtual resources. If nil, defaults are constructed.
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

func (f *Factory) ensureResultsStore() store.ResultsStore {
	if f.ResultsStore == nil {
		f.ResultsStore = store.NewInMemoryResultsStore()
	}
	return f.ResultsStore
}

func (f *Factory) ensureMemoryStore(cfg config.Config) (store.MemoryStore, error) {
	if f.MemoryStore != nil {
		return f.MemoryStore, nil
	}
	ms, err := store.NewDiskMemoryStore(cfg, f.RunID)
	if err != nil {
		return nil, err
	}
	f.MemoryStore = ms
	return f.MemoryStore, nil
}

func (f *Factory) ensureProfileStore(cfg config.Config) (store.ProfileStore, error) {
	if f.ProfileStore != nil {
		return f.ProfileStore, nil
	}
	ps, err := store.NewDiskProfileStore(cfg)
	if err != nil {
		return nil, err
	}
	f.ProfileStore = ps
	return f.ProfileStore, nil
}

func (f *Factory) ensureHistoryStore(cfg config.Config) (store.HistoryStore, error) {
	if f.HistoryStore != nil {
		return f.HistoryStore, nil
	}
	hs, err := store.NewSQLiteHistoryStore(cfg, f.SessionID)
	if err != nil {
		return nil, err
	}
	f.HistoryStore = hs
	return f.HistoryStore, nil
}

func (f *Factory) ensureTraceStore(traceBaseDir string) store.TraceStore {
	if f.TraceStore == nil {
		f.TraceStore = store.DiskTraceStore{DiskStore: store.DiskStore{Dir: traceBaseDir}}
	}
	return f.TraceStore
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
	tr, err := NewTraceResource(cfg, f.RunID)
	if err != nil {
		return nil, err
	}
	_ = f.ensureTraceStore(tr.BaseDir)
	return tr, nil
}

func (f *Factory) Results() (*ResultsResource, error) {
	rs := f.ensureResultsStore()
	return NewResultsResource(rs)
}

func (f *Factory) Memory() (*MemoryResource, error) {
	cfg, err := f.cfg()
	if err != nil {
		return nil, err
	}
	if err := validate.NonEmpty("RunID", f.RunID); err != nil {
		return nil, err
	}
	ms, err := f.ensureMemoryStore(cfg)
	if err != nil {
		return nil, err
	}
	return NewMemoryResource(ms)
}

func (f *Factory) Profile() (*ProfileResource, error) {
	cfg, err := f.cfg()
	if err != nil {
		return nil, err
	}
	ps, err := f.ensureProfileStore(cfg)
	if err != nil {
		return nil, err
	}
	return NewProfileResource(ps)
}

func (f *Factory) History() (*HistoryResource, error) {
	cfg, err := f.cfg()
	if err != nil {
		return nil, err
	}
	if err := validate.NonEmpty("SessionID", f.SessionID); err != nil {
		return nil, err
	}
	hs, err := f.ensureHistoryStore(cfg)
	if err != nil {
		return nil, err
	}
	return &HistoryResource{
		BaseDir:   fsutil.GetSessionHistoryDir(cfg.DataDir, f.SessionID),
		Mount:     vfs.MountHistory,
		SessionID: f.SessionID,
		Store:     hs,
		Appender:  hs,
	}, nil
}

func (f *Factory) Tools() (*internaltools.ToolsResource, error) {
	cfg, err := f.cfg()
	if err != nil {
		return nil, err
	}
	toolsDir := fsutil.GetToolsDir(cfg.DataDir)
	_ = os.MkdirAll(toolsDir, 0755)

	builtinProvider, err := internaltools.NewBuiltinManifestProvider()
	if err != nil {
		return nil, err
	}
	diskProvider := pkgtools.NewDiskManifestProvider(toolsDir)
	toolManifests := pkgtools.NewCompositeToolManifestRegistry(builtinProvider, diskProvider)

	return internaltools.NewToolsResource(toolManifests)
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
	tres, err := f.Tools()
	if err != nil {
		return fmt.Errorf("create tools: %w", err)
	}

	traceDir := filepath.Join(ws.BaseDir, "trace")
	if err := os.MkdirAll(traceDir, 0755); err != nil {
		return fmt.Errorf("create trace dir: %w", err)
	}
	traceRes, err := NewDirResource(traceDir, vfs.MountTrace)
	if err != nil {
		return fmt.Errorf("create trace resource: %w", err)
	}

	fs.Mount(vfs.MountScratch, ws)
	fs.Mount(vfs.MountLog, tr)
	fs.Mount(vfs.MountResults, res)
	fs.Mount(vfs.MountMemory, mem)
	fs.Mount(vfs.MountProfile, prof)
	fs.Mount(vfs.MountHistory, hist)
	fs.Mount(vfs.MountTools, tres)
	fs.Mount(vfs.MountTrace, traceRes)

	return nil
}
