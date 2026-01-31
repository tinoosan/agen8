package runtime

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tinoosan/workbench-core/pkg/agent"
	"github.com/tinoosan/workbench-core/pkg/config"
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
			approvalMode := strings.TrimSpace(cfg.ApprovalsMode)
			if approvalMode == "" {
				// Autonomous-first: approvals are disabled by default.
				approvalMode = "disabled"
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
	fs.Mount(vfs.MountProject, workdirRes)

	runDir := fsutil.GetRunDir(cfg.Cfg.DataDir, cfg.Run.RunId)
	planDir := filepath.Join(runDir, "plan")
	if err := os.MkdirAll(planDir, 0755); err != nil {
		return nil, fmt.Errorf("prepare plan dir: %w", err)
	}
	planRes, err := resources.NewDirResource(planDir, vfs.MountPlan)
	if err != nil {
		return nil, fmt.Errorf("create plan resource: %w", err)
	}
	fs.Mount(vfs.MountPlan, planRes)

	skillDir := fsutil.GetSkillsDir(cfg.Cfg.DataDir)
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		return nil, fmt.Errorf("prepare skills dir: %w", err)
	}
	skillMgr := skills.NewManager([]string{skillDir})
	skillMgr.WritableRoot = skillDir
	if err := skillMgr.Scan(); err != nil {
		return nil, fmt.Errorf("scan skills: %w", err)
	}
	fs.Mount(vfs.MountSkills, skills.NewResource(skillMgr))

	inboxDir := filepath.Join(runDir, vfs.MountInbox)
	if err := os.MkdirAll(inboxDir, 0755); err != nil {
		return nil, fmt.Errorf("prepare inbox dir: %w", err)
	}
	inboxRes, err := resources.NewDirResource(inboxDir, vfs.MountInbox)
	if err != nil {
		return nil, fmt.Errorf("create inbox resource: %w", err)
	}
	fs.Mount(vfs.MountInbox, inboxRes)

	outboxDir := filepath.Join(runDir, vfs.MountOutbox)
	if err := os.MkdirAll(outboxDir, 0755); err != nil {
		return nil, fmt.Errorf("prepare outbox dir: %w", err)
	}
	outboxRes, err := resources.NewDirResource(outboxDir, vfs.MountOutbox)
	if err != nil {
		return nil, fmt.Errorf("create outbox resource: %w", err)
	}
	fs.Mount(vfs.MountOutbox, outboxRes)

	resultsStore := cfg.ResultsStore
	memStore := cfg.MemoryStore
	profileStore := cfg.ProfileStore

	if memStore == nil {
		return nil, fmt.Errorf("memory store is required")
	}
	memRes, err := resources.NewMemoryResource(memStore)
	if err != nil {
		return nil, fmt.Errorf("create memory resource: %w", err)
	}
	fs.Mount(vfs.MountMemory, memRes)

	if cfg.Emit != nil {
		cfg.Emit(context.Background(), events.Event{
			Type:    "host.mounted",
			Message: "Mounted VFS resources",
			Data: map[string]string{
				"/project": workdirRes.BaseDir,
				"/inbox":   inboxDir,
				"/outbox":  outboxDir,
				"/plan":    planDir,
				"/skills":  "(virtual)",
				"/memory":  "(virtual)",
			},
			Console: boolPtr(false),
		})
	}

	absWorkdirRoot, err := filepath.Abs(workdirRes.BaseDir)
	if err != nil {
		return nil, fmt.Errorf("resolve workdir root: %w", err)
	}

	traceStore := cfg.TraceStore
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

	runner := tools.Runner{
		Results:      resultsStore,
		ToolRegistry: tools.MapRegistry{},
	}

	toolManifests := []tools.ToolManifest{}

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
		FS:              fs,
		Cfg:             cfg.Cfg,
		RunID:           cfg.Run.RunId,
		SessionID:       cfg.Run.SessionID,
		LoadSession:     cfg.LoadSession,
		SaveSession:     cfg.SaveSession,
		StateStore:      cfg.ConstructorStore,
		MaxProfileBytes: cfg.MaxProfileBytes,
		MaxMemoryBytes:  cfg.MaxMemoryBytes,
		MaxTraceBytes:   cfg.MaxTraceBytes,
		Emit: func(eventType, message string, data map[string]string) {
			if cfg.Emit != nil {
				cfg.Emit(context.Background(), events.Event{Type: eventType, Message: message, Data: data})
			}
		},
	}

	auditObs := newAuditObserver(cfg.Run.RunId, cfg.Emit)

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
		Observers:       []HostOpObserver{constructor, auditObs},
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
		Updater:         nil,
		WorkdirBase:     workdirRes.BaseDir,
		MemStore:        memStore,
		ProfileStore:    profileStore,
	}, nil
}

func boolPtr(v bool) *bool { return &v }
