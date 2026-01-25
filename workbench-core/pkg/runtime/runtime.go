package runtime

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tinoosan/workbench-core/internal/agent"
	"github.com/tinoosan/workbench-core/internal/config"
	"github.com/tinoosan/workbench-core/internal/debuglog"
	"github.com/tinoosan/workbench-core/internal/events"
	"github.com/tinoosan/workbench-core/internal/fsutil"
	"github.com/tinoosan/workbench-core/internal/resources"
	"github.com/tinoosan/workbench-core/internal/skills"
	"github.com/tinoosan/workbench-core/internal/store"
	"github.com/tinoosan/workbench-core/internal/tools"
	"github.com/tinoosan/workbench-core/internal/types"
	"github.com/tinoosan/workbench-core/internal/vfs"
)

type Runtime struct {
	FS              *vfs.FS
	Executor        agent.HostExecutor
	Runner          *tools.Runner
	ToolManifests   []types.ToolManifest
	BuiltinInvokers tools.MapRegistry
	TraceMiddleware *agent.TraceMiddleware
	Constructor     *agent.ContextConstructor
	Updater         *agent.ContextUpdater
	WorkdirBase     string
	MemStore        store.MemoryCommitter
	ProfileStore    store.ProfileCommitter
}

type BuildConfig struct {
	Cfg             config.Config
	Run             types.Run
	WorkdirAbs      string
	Model           string
	HistoryRes      *resources.HistoryResource
	Emit            func(ctx context.Context, ev events.Event)
	IncludeHistoryOps bool
	MaxProfileBytes int
	MaxMemoryBytes  int
	MaxTraceBytes   int
	Guard           func(fs *vfs.FS, req types.HostOpRequest) *types.HostOpResponse
	ArtifactObserve func(path string)
}

func Build(cfg BuildConfig) (*Runtime, error) {
	if err := cfg.Cfg.Validate(); err != nil {
		return nil, err
	}
	if cfg.HistoryRes == nil {
		return nil, fmt.Errorf("history resource is required")
	}
	if strings.TrimSpace(cfg.WorkdirAbs) == "" {
		return nil, fmt.Errorf("workdir is required")
	}

	fs := vfs.NewFS()

	workdirRes, err := resources.NewWorkdirResource(cfg.WorkdirAbs)
	if err != nil {
		return nil, fmt.Errorf("create workdir: %w", err)
	}

	f := &resources.Factory{
		DataDir:   cfg.Cfg.DataDir,
		SessionID: cfg.Run.SessionID,
		RunID:     cfg.Run.RunId,
	}
	if hs, ok := cfg.HistoryRes.Store.(store.HistoryStore); ok {
		f.HistoryStore = hs
	} else if hs, ok := cfg.HistoryRes.Appender.(store.HistoryStore); ok {
		f.HistoryStore = hs
	}
	if err := f.MountAll(fs); err != nil {
		return nil, err
	}
	fs.Mount(vfs.MountProject, workdirRes)

	skillDir := fsutil.GetSkillsDir(cfg.Cfg.DataDir)
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		return nil, fmt.Errorf("prepare skills dir: %w", err)
	}
	workdirSkillDir := filepath.Join(cfg.WorkdirAbs, "skills")
	if err := os.MkdirAll(workdirSkillDir, 0755); err != nil {
		return nil, fmt.Errorf("prepare workdir skills dir: %w", err)
	}
	skillRoots := []string{skillDir, workdirSkillDir}
	skillMgr := skills.NewManager(skillRoots)
	skillMgr.WritableRoot = skillDir
	if err := skillMgr.Scan(); err != nil {
		return nil, fmt.Errorf("scan skills: %w", err)
	}
	if entries := skillMgr.Entries(); len(entries) > 0 {
		debuglog.Log("skills", "H13", "runtime.Build", "skills_discovered", map[string]any{
			"count": len(entries),
			"names": func() []string {
				names := make([]string, len(entries))
				for i, e := range entries {
					names[i] = e.Dir
				}
				return names
			}(),
		})
	} else {
		debuglog.Log("skills", "H13", "runtime.Build", "no_skills_discovered", map[string]any{
			"roots": skillRoots,
		})
	}
	fs.Mount(vfs.MountSkills, skills.NewResource(skillMgr))

	_, wsr, _, _ := fs.Resolve("/" + vfs.MountScratch)
	workspace := wsr.(*resources.DirResource)
	_, trr, _, _ := fs.Resolve("/" + vfs.MountLog)
	traceRes := trr.(*resources.TraceResource)
	_, hr, _, _ := fs.Resolve("/" + vfs.MountHistory)
	historyRes := hr.(*resources.HistoryResource)

	planDir := filepath.Join(workspace.BaseDir, "plan")
	if err := os.MkdirAll(planDir, 0755); err != nil {
		return nil, fmt.Errorf("prepare plan dir: %w", err)
	}
	planRes, err := resources.NewDirResource(planDir, "plan")
	if err != nil {
		return nil, fmt.Errorf("create plan resource: %w", err)
	}
	fs.Mount("plan", planRes)

	resultsStore := f.ResultsStore
	memStore := f.MemoryStore
	profileStore := f.ProfileStore

	if cfg.Emit != nil {
		cfg.Emit(context.Background(), events.Event{
			Type:    "host.mounted",
			Message: "Mounted VFS resources",
			Data: map[string]string{
				"/scratch":            workspace.BaseDir,
				"/project":            workdirRes.BaseDir,
				"/results":            "(virtual)",
				"/log":                traceRes.BaseDir,
				"/tools":              "(virtual)",
				"/plan":               "(virtual)",
				"/memory":             "(virtual)",
				"/profile":            "(global)",
				"/history":            historyRes.BaseDir,
				"/" + vfs.MountSkills: "(virtual)",
			},
			Console: boolPtr(false),
		})
	}

	absWorkdirRoot, err := filepath.Abs(workdirRes.BaseDir)
	if err != nil {
		return nil, fmt.Errorf("resolve workdir root: %w", err)
	}

	traceStore := f.TraceStore
	traceMiddleware := &agent.TraceMiddleware{
		Store: traceStore,
		FS:    fs,
	}
	builtinCfg := tools.BuiltinConfig{
		ShellRootDir:  absWorkdirRoot,
		ShellVFSMount: vfs.MountProject,
		ShellConfirm:  nil,
		TraceStore:    traceStore,
	}
	shellInvoker := tools.NewBuiltinShellInvoker(absWorkdirRoot, nil, vfs.MountProject)
	httpInvoker := tools.NewBuiltinHTTPInvoker()
	traceInvoker := tools.BuiltinTraceInvoker{Store: traceStore}

	builtinInvokers := tools.BuiltinInvokerRegistry(builtinCfg)
	if builtinInvokers == nil {
		builtinInvokers = make(tools.MapRegistry)
	}
	builtinInvokers[types.ToolID("builtin.shell")] = shellInvoker
	builtinInvokers[types.ToolID("builtin.http")] = httpInvoker
	builtinInvokers[types.ToolID("builtin.trace")] = traceInvoker

	builtinManifestProvider, err := tools.NewBuiltinManifestProvider()
	if err != nil {
		return nil, fmt.Errorf("load builtin manifests: %w", err)
	}
	toolsDir := fsutil.GetToolsDir(cfg.Cfg.DataDir)
	_ = os.MkdirAll(toolsDir, 0755)
	diskManifestProvider := tools.NewDiskManifestProvider(toolsDir)
	toolManifestRegistry := tools.NewCompositeToolManifestRegistry(builtinManifestProvider, diskManifestProvider)

	toolRuntime, err := tools.NewRuntimeWiring(toolManifestRegistry, builtinInvokers)
	if err != nil {
		return nil, err
	}
	fs.Mount(vfs.MountTools, toolRuntime.Resource)

	runner := tools.Runner{
		Results:      resultsStore,
		ToolRegistry: toolRuntime.Registry,
	}

	toolManifests := []types.ToolManifest{}
	if ids, err := builtinManifestProvider.ListToolIDs(context.Background()); err != nil {
		return nil, fmt.Errorf("list builtin manifests: %w", err)
	} else {
		for _, id := range ids {
			b, ok, err := builtinManifestProvider.GetManifest(context.Background(), id)
			if err != nil {
				return nil, fmt.Errorf("read builtin manifest %s: %w", id.String(), err)
			}
			if !ok {
				continue
			}
			m, err := types.ParseBuiltinToolManifest(b)
			if err != nil {
				return nil, fmt.Errorf("parse builtin manifest %s: %w", id.String(), err)
			}
			toolManifests = append(toolManifests, m)
		}
	}
	if ids, err := diskManifestProvider.ListToolIDs(context.Background()); err != nil {
		return nil, fmt.Errorf("list disk manifests: %w", err)
	} else {
		for _, id := range ids {
			b, ok, err := diskManifestProvider.GetManifest(context.Background(), id)
			if err != nil {
				return nil, fmt.Errorf("read disk manifest %s: %w", id.String(), err)
			}
			if !ok {
				continue
			}
			m, err := types.ParseUserToolManifest(b)
			if err != nil {
				return nil, fmt.Errorf("parse disk manifest %s: %w", id.String(), err)
			}
			toolManifests = append(toolManifests, m)
		}
	}

	executor := &agent.HostOpExecutor{
		FS:              fs,
		Runner:          &runner,
		ShellInvoker:    shellInvoker,
		HTTPInvoker:     httpInvoker,
		TraceInvoker:    traceInvoker,
		DefaultMaxBytes: 4096,
		MaxReadBytes:    256 * 1024,
	}

	constructor := &agent.ContextConstructor{
		FS:                fs,
		Cfg:               cfg.Cfg,
		RunID:             cfg.Run.RunId,
		SessionID:         cfg.Run.SessionID,
		Trace:             traceMiddleware,
		HistoryStore:      historyRes.Store,
		SkillsManager:     skillMgr,
		IncludeHistoryOps: cfg.IncludeHistoryOps,
		MaxProfileBytes:   cfg.MaxProfileBytes,
		MaxMemoryBytes:    cfg.MaxMemoryBytes,
		MaxTraceBytes:     cfg.MaxTraceBytes,
		MaxHistoryBytes:   8 * 1024,
		StatePath:         filepath.Join(fsutil.GetRunDir(cfg.Cfg.DataDir, cfg.Run.RunId), "context_constructor_state.json"),
		ManifestPath:      filepath.Join(fsutil.GetRunDir(cfg.Cfg.DataDir, cfg.Run.RunId), "context_constructor_manifest.json"),
		Emit: func(eventType, message string, data map[string]string) {
			if cfg.Emit != nil {
				cfg.Emit(context.Background(), events.Event{Type: eventType, Message: message, Data: data})
			}
		},
	}

	updater := &agent.ContextUpdater{
		FS:              fs,
		Trace:           traceMiddleware,
		MaxProfileBytes: cfg.MaxProfileBytes,
		MaxMemoryBytes:  cfg.MaxMemoryBytes,
		MaxTraceBytes:   cfg.MaxTraceBytes,
		ManifestPath:    "/scratch/context_manifest.json",
		Emit: func(eventType, message string, data map[string]string) {
			if cfg.Emit != nil {
				cfg.Emit(context.Background(), events.Event{Type: eventType, Message: message, Data: data})
			}
		},
	}

	exec := NewExecutor(executor, ExecutorOptions{
		Emit:            cfg.Emit,
		Model:           cfg.Model,
		RunID:           cfg.Run.RunId,
		SessionID:       cfg.Run.SessionID,
		FS:              fs,
		Guard: func(req types.HostOpRequest) *types.HostOpResponse {
			if cfg.Guard == nil {
				return nil
			}
			return cfg.Guard(fs, req)
		},
		Observers:       []HostOpObserver{constructor, updater},
		ArtifactObserve: cfg.ArtifactObserve,
	})

	return &Runtime{
		FS:              fs,
		Executor:        exec,
		Runner:          &runner,
		ToolManifests:   toolManifests,
		BuiltinInvokers: builtinInvokers,
		TraceMiddleware: traceMiddleware,
		Constructor:     constructor,
		Updater:         updater,
		WorkdirBase:     workdirRes.BaseDir,
		MemStore:        memStore,
		ProfileStore:    profileStore,
	}, nil
}

func boolPtr(v bool) *bool { return &v }
