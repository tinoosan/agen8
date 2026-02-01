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
	"github.com/tinoosan/workbench-core/pkg/role"
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
	Runner          *tools.Orchestrator
	ToolManifests   []tools.ToolManifest
	BuiltinInvokers tools.MapRegistry
	TraceMiddleware *agent.TraceMiddleware
	Constructor     *agent.PromptBuilder
	Updater         *agent.PromptUpdater
	WorkdirBase     string
	MemStore        store.MemoryStore
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
	MemoryReindexer       resources.MemoryReindexer
	ConstructorStore      store.ConstructorStateStore
	Emit                  func(ctx context.Context, ev events.Event)
	IncludeHistoryOps     bool
	RecentHistoryPairs    int
	MaxProfileBytes       int
	MaxMemoryBytes        int
	MaxTraceBytes         int
	PriceInPerMTokensUSD  float64
	PriceOutPerMTokensUSD float64
	AuditReads            bool
	Guard                 func(fs *vfs.FS, req types.HostOpRequest) *types.HostOpResponse
	ArtifactObserve       func(path string)
	PersistRun            func(run types.Run) error
	LoadSession           func(sessionID string) (types.Session, error)
	SaveSession           func(session types.Session) error
}

func (cfg BuildConfig) Validate() error {
	if err := cfg.Cfg.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(cfg.WorkdirAbs) == "" {
		return fmt.Errorf("workdir is required")
	}
	if cfg.ResultsStore == nil {
		return fmt.Errorf("results store is required")
	}
	if cfg.MemoryStore == nil {
		return fmt.Errorf("memory store is required")
	}
	if cfg.ProfileStore == nil {
		return fmt.Errorf("profile store is required")
	}
	if cfg.HistoryStore == nil {
		return fmt.Errorf("history store is required")
	}
	if cfg.TraceStore == nil {
		return fmt.Errorf("trace store is required")
	}
	if cfg.ConstructorStore == nil {
		return fmt.Errorf("constructor store is required")
	}
	return nil
}

func Build(cfg BuildConfig) (*Runtime, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
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
		if err := cfg.PersistRun(run); err != nil && cfg.Emit != nil {
			cfg.Emit(context.Background(), events.Event{
				Type:    "runtime.warning",
				Message: "Failed to persist run state",
				Data:    map[string]string{"error": err.Error()},
			})
		}
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
				if err := cfg.SaveSession(sess); err != nil && cfg.Emit != nil {
					cfg.Emit(context.Background(), events.Event{
						Type:    "runtime.warning",
						Message: "Failed to persist session state",
						Data:    map[string]string{"error": err.Error()},
					})
				}
			}
		}
	}

	fs := vfs.NewFS()

	workdirRes, err := resources.NewWorkdirResource(cfg.WorkdirAbs)
	if err != nil {
		return nil, fmt.Errorf("create workdir: %w", err)
	}
	if err := fs.Mount(vfs.MountProject, workdirRes); err != nil {
		return nil, fmt.Errorf("mount %s: %w", vfs.MountProject, err)
	}

	runDir := fsutil.GetRunDir(cfg.Cfg.DataDir, cfg.Run.RunId)
	planDir := filepath.Join(runDir, "plan")
	if err := os.MkdirAll(planDir, 0755); err != nil {
		return nil, fmt.Errorf("prepare plan dir: %w", err)
	}
	ensureFile := func(path string, contents string) error {
		if st, err := os.Stat(path); err == nil {
			if st.IsDir() {
				return fmt.Errorf("path %s is a directory", path)
			}
			return nil
		} else if !os.IsNotExist(err) {
			return err
		}
		return os.WriteFile(path, []byte(contents), 0644)
	}
	if err := ensureFile(filepath.Join(planDir, "HEAD.md"), "# Current Plan\n\n_No active plan._\n"); err != nil {
		return nil, fmt.Errorf("prepare plan details: %w", err)
	}
	if err := ensureFile(filepath.Join(planDir, "CHECKLIST.md"), "# Task Checklist\n\n_No tasks yet._\n"); err != nil {
		return nil, fmt.Errorf("prepare plan checklist: %w", err)
	}
	planRes, err := resources.NewDirResource(planDir, vfs.MountPlan)
	if err != nil {
		return nil, fmt.Errorf("create plan resource: %w", err)
	}
	if err := fs.Mount(vfs.MountPlan, planRes); err != nil {
		return nil, fmt.Errorf("mount %s: %w", vfs.MountPlan, err)
	}

	skillDir := fsutil.GetSkillsDir(cfg.Cfg.DataDir)
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		return nil, fmt.Errorf("prepare skills dir: %w", err)
	}
	roleDir := fsutil.GetRolesDir(cfg.Cfg.DataDir)
	if err := os.MkdirAll(roleDir, 0755); err != nil {
		return nil, fmt.Errorf("prepare roles dir: %w", err)
	}
	roleMgr := role.NewManager([]string{roleDir})
	if err := roleMgr.Scan(); err != nil {
		return nil, fmt.Errorf("scan roles: %w", err)
	}
	role.SetDefaultManager(roleMgr)
	skillMgr := skills.NewManager([]string{skillDir})
	skillMgr.WritableRoot = skillDir
	if err := skillMgr.Scan(); err != nil {
		return nil, fmt.Errorf("scan skills: %w", err)
	}
	if err := fs.Mount(vfs.MountSkills, skills.NewResource(skillMgr)); err != nil {
		return nil, fmt.Errorf("mount %s: %w", vfs.MountSkills, err)
	}

	inboxDir := filepath.Join(runDir, vfs.MountInbox)
	if err := os.MkdirAll(inboxDir, 0755); err != nil {
		return nil, fmt.Errorf("prepare inbox dir: %w", err)
	}
	inboxRes, err := resources.NewDirResource(inboxDir, vfs.MountInbox)
	if err != nil {
		return nil, fmt.Errorf("create inbox resource: %w", err)
	}
	if err := fs.Mount(vfs.MountInbox, inboxRes); err != nil {
		return nil, fmt.Errorf("mount %s: %w", vfs.MountInbox, err)
	}

	outboxDir := filepath.Join(runDir, vfs.MountOutbox)
	if err := os.MkdirAll(outboxDir, 0755); err != nil {
		return nil, fmt.Errorf("prepare outbox dir: %w", err)
	}
	outboxRes, err := resources.NewDirResource(outboxDir, vfs.MountOutbox)
	if err != nil {
		return nil, fmt.Errorf("create outbox resource: %w", err)
	}
	if err := fs.Mount(vfs.MountOutbox, outboxRes); err != nil {
		return nil, fmt.Errorf("mount %s: %w", vfs.MountOutbox, err)
	}

	resultsStore := cfg.ResultsStore
	memStore := cfg.MemoryStore
	profileStore := cfg.ProfileStore

	if memStore == nil {
		return nil, fmt.Errorf("memory store is required")
	}
	memRes, err := resources.NewDailyMemoryResource(fsutil.GetMemoryDir(cfg.Cfg.DataDir), cfg.MemoryReindexer, cfg.Emit)
	if err != nil {
		return nil, fmt.Errorf("create memory resource: %w", err)
	}
	if err := fs.Mount(vfs.MountMemory, memRes); err != nil {
		return nil, fmt.Errorf("mount %s: %w", vfs.MountMemory, err)
	}

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

	runner := tools.Orchestrator{
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

	constructor := &agent.PromptBuilder{
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

	auditObs := newAuditObserver(cfg.Run.RunId, cfg.Emit, cfg.AuditReads)

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
