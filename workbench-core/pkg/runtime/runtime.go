package runtime

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tinoosan/workbench-core/pkg/agent"
	"github.com/tinoosan/workbench-core/pkg/config"
	"github.com/tinoosan/workbench-core/pkg/debuglog"
	"github.com/tinoosan/workbench-core/pkg/events"
	"github.com/tinoosan/workbench-core/pkg/fsutil"
	"github.com/tinoosan/workbench-core/pkg/resources"
	"github.com/tinoosan/workbench-core/pkg/skills"
	"github.com/tinoosan/workbench-core/pkg/store"
	"github.com/tinoosan/workbench-core/pkg/tools"
	"github.com/tinoosan/workbench-core/pkg/tools/builtins"
	"github.com/tinoosan/workbench-core/pkg/types"
	"github.com/tinoosan/workbench-core/pkg/vfs"
)

type Runtime struct {
	FS              *vfs.FS
	Executor        agent.HostExecutor
	Runner          *tools.Runner
	ToolManifests   []tools.ToolManifest
	BuiltinInvokers tools.MapRegistry
	TraceMiddleware *agent.TraceMiddleware
	Constructor     *agent.ContextConstructor
	Updater         *agent.ContextUpdater
	WorkdirBase     string
	MemStore        store.MemoryCommitter
	ProfileStore    store.ProfileCommitter
}

type BuildConfig struct {
	Cfg                   config.Config
	Run                   types.Run
	WorkdirAbs            string
	Model                 string
	ReasoningEffort       string
	ReasoningSummary      string
	ApprovalsMode         string
	SelectedSkill         string
	HistoryStore          store.HistoryStore
	ResultsStore          store.ResultsStore
	MemoryStore           store.MemoryStore
	ProfileStore          store.ProfileStore
	TraceStore            store.TraceStore
	ConstructorStore      store.ConstructorStateStore
	Emit                  func(ctx context.Context, ev events.Event)
	IncludeHistoryOps     bool
	RecentHistoryPairs    int
	MaxProfileBytes       int
	MaxMemoryBytes        int
	MaxTraceBytes         int
	PriceInPerMTokensUSD  float64
	PriceOutPerMTokensUSD float64
	Guard                 func(fs *vfs.FS, req types.HostOpRequest) *types.HostOpResponse
	ArtifactObserve       func(path string)
	PersistRun            func(run types.Run) error
	LoadSession           func(sessionID string) (types.Session, error)
	SaveSession           func(session types.Session) error
}

func Build(cfg BuildConfig) (*Runtime, error) {
	if err := cfg.Cfg.Validate(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(cfg.WorkdirAbs) == "" {
		return nil, fmt.Errorf("workdir is required")
	}

	run := cfg.Run
	run.Runtime = &types.RunRuntimeConfig{
		DataDir:          cfg.Cfg.DataDir,
		Model:            cfg.Model,
		ReasoningEffort:  strings.TrimSpace(cfg.ReasoningEffort),
		ReasoningSummary: strings.TrimSpace(cfg.ReasoningSummary),
		ApprovalsMode:    strings.TrimSpace(cfg.ApprovalsMode),
		SelectedSkill:    strings.TrimSpace(cfg.SelectedSkill),

		MaxTraceBytes:         cfg.MaxTraceBytes,
		MaxMemoryBytes:        cfg.MaxMemoryBytes,
		MaxProfileBytes:       cfg.MaxProfileBytes,
		RecentHistoryPairs:    cfg.RecentHistoryPairs,
		IncludeHistoryOps:     cfg.IncludeHistoryOps,
		PriceInPerMTokensUSD:  cfg.PriceInPerMTokensUSD,
		PriceOutPerMTokensUSD: cfg.PriceOutPerMTokensUSD,
	}
	if cfg.PersistRun != nil {
		_ = cfg.PersistRun(run)
	}

	if cfg.LoadSession != nil {
		if sess, err := cfg.LoadSession(run.SessionID); err == nil {
			changed := false
			if strings.TrimSpace(sess.ActiveModel) != strings.TrimSpace(cfg.Model) {
				sess.ActiveModel = strings.TrimSpace(cfg.Model)
				changed = true
			}
			if strings.TrimSpace(sess.ReasoningEffort) != strings.TrimSpace(cfg.ReasoningEffort) {
				sess.ReasoningEffort = strings.TrimSpace(cfg.ReasoningEffort)
				changed = true
			}
			if strings.TrimSpace(sess.ReasoningSummary) != strings.TrimSpace(cfg.ReasoningSummary) {
				sess.ReasoningSummary = strings.TrimSpace(cfg.ReasoningSummary)
				changed = true
			}
			if strings.TrimSpace(sess.SelectedSkill) != strings.TrimSpace(cfg.SelectedSkill) {
				sess.SelectedSkill = strings.TrimSpace(cfg.SelectedSkill)
				changed = true
			}
			approvalMode := strings.TrimSpace(cfg.ApprovalsMode)
			if approvalMode == "" {
				approvalMode = "enabled"
			}
			if strings.TrimSpace(sess.ApprovalsMode) != approvalMode {
				sess.ApprovalsMode = approvalMode
				changed = true
			}
			if changed && cfg.SaveSession != nil {
				_ = cfg.SaveSession(sess)
			}
		}
	}

	fs := vfs.NewFS()

	workdirRes, err := resources.NewWorkdirResource(cfg.WorkdirAbs)
	if err != nil {
		return nil, fmt.Errorf("create workdir: %w", err)
	}

	f := &resources.Factory{
		DataDir:      cfg.Cfg.DataDir,
		SessionID:    cfg.Run.SessionID,
		RunID:        cfg.Run.RunId,
		ResultsStore: cfg.ResultsStore,
		MemoryStore:  cfg.MemoryStore,
		ProfileStore: cfg.ProfileStore,
		HistoryStore: cfg.HistoryStore,
		TraceStore:   cfg.TraceStore,
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
	builtinCfg := builtins.BuiltinConfig{
		ShellRootDir:  absWorkdirRoot,
		ShellVFSMount: vfs.MountProject,
		ShellConfirm:  nil,
		TraceStore:    traceStore,
	}
	shellInvoker := builtins.NewBuiltinShellInvoker(absWorkdirRoot, nil, vfs.MountProject)
	httpInvoker := builtins.NewBuiltinHTTPInvoker()
	traceInvoker := builtins.BuiltinTraceInvoker{Store: traceStore}

	builtinInvokers := builtins.BuiltinInvokerRegistry(builtinCfg)
	if builtinInvokers == nil {
		builtinInvokers = make(tools.MapRegistry)
	}
	builtinInvokers[tools.ToolID("builtin.shell")] = shellInvoker
	builtinInvokers[tools.ToolID("builtin.http")] = httpInvoker
	builtinInvokers[tools.ToolID("builtin.trace")] = traceInvoker

	builtinManifestProvider, err := builtins.NewBuiltinManifestProvider()
	if err != nil {
		return nil, fmt.Errorf("load builtin manifests: %w", err)
	}
	toolsDir := fsutil.GetToolsDir(cfg.Cfg.DataDir)
	_ = os.MkdirAll(toolsDir, 0755)
	diskManifestProvider := tools.NewDiskManifestProvider(toolsDir)
	toolManifestRegistry := tools.NewCompositeToolManifestRegistry(builtinManifestProvider, diskManifestProvider)

	toolRuntime, err := builtins.NewRuntimeWiring(toolManifestRegistry, builtinInvokers)
	if err != nil {
		return nil, err
	}
	fs.Mount(vfs.MountTools, toolRuntime.Resource)

	runner := tools.Runner{
		Results:      resultsStore,
		ToolRegistry: toolRuntime.Registry,
	}

	toolManifests := []tools.ToolManifest{}
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
			m, err := tools.ParseBuiltinToolManifest(b)
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
			m, err := tools.ParseUserToolManifest(b)
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
		LoadSession:       cfg.LoadSession,
		SaveSession:       cfg.SaveSession,
		StateStore:        cfg.ConstructorStore,
		Trace:             traceMiddleware,
		HistoryStore:      historyRes.Store,
		SkillsManager:     skillMgr,
		IncludeHistoryOps: cfg.IncludeHistoryOps,
		MaxProfileBytes:   cfg.MaxProfileBytes,
		MaxMemoryBytes:    cfg.MaxMemoryBytes,
		MaxTraceBytes:     cfg.MaxTraceBytes,
		MaxHistoryBytes:   8 * 1024,
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
		Emit:      cfg.Emit,
		Model:     cfg.Model,
		RunID:     cfg.Run.RunId,
		SessionID: cfg.Run.SessionID,
		FS:        fs,
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
