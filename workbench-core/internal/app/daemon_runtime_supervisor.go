package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	implstore "github.com/tinoosan/workbench-core/internal/store"
	"github.com/tinoosan/workbench-core/pkg/agent"
	hosttools "github.com/tinoosan/workbench-core/pkg/agent/hosttools"
	agentsession "github.com/tinoosan/workbench-core/pkg/agent/session"
	"github.com/tinoosan/workbench-core/pkg/config"
	"github.com/tinoosan/workbench-core/pkg/emit"
	"github.com/tinoosan/workbench-core/pkg/events"
	"github.com/tinoosan/workbench-core/pkg/fsutil"
	llmtypes "github.com/tinoosan/workbench-core/pkg/llm/types"
	"github.com/tinoosan/workbench-core/pkg/profile"
	"github.com/tinoosan/workbench-core/pkg/prompts"
	"github.com/tinoosan/workbench-core/pkg/runtime"
	eventsvc "github.com/tinoosan/workbench-core/pkg/services/events"
	pkgsession "github.com/tinoosan/workbench-core/pkg/services/session"
	pkgtask "github.com/tinoosan/workbench-core/pkg/services/task"
	"github.com/tinoosan/workbench-core/pkg/services/team"
	pkgstore "github.com/tinoosan/workbench-core/pkg/store"
	"github.com/tinoosan/workbench-core/pkg/types"
)

type runtimeSupervisorConfig struct {
	Cfg              config.Config
	Resolved         RunChatOptions
	PollInterval     time.Duration
	TaskService      pkgtask.TaskServiceForSupervisor
	SessionService   pkgsession.Service
	EventsStore      events.StoreAppender
	MemoryStore      pkgstore.DailyMemoryStore
	ConstructorStore pkgstore.ConstructorStateStore
	LLMClient        llmtypes.LLMClient
	Notifier         agent.Notifier
	WorkdirAbs       string
	BootstrapRunID   string
	DefaultProfile   *profile.Profile
}

type runtimeSupervisor struct {
	cfg              config.Config
	resolved         RunChatOptions
	pollInterval     time.Duration
	taskService      pkgtask.TaskServiceForSupervisor
	sessionService   pkgsession.Service
	eventsStore      events.StoreAppender
	memoryStore      pkgstore.DailyMemoryStore
	constructorStore pkgstore.ConstructorStateStore
	llmClient        llmtypes.LLMClient
	notifier         agent.Notifier
	workdirAbs       string
	bootstrapRunID   string
	defaultProfile   *profile.Profile

	mu      sync.Mutex
	workers map[string]*managedRuntime

	spawnOverride func(context.Context, types.Session, string) (*managedRuntime, error)
}

type managedRuntime struct {
	runID     string
	sessionID string
	session   *agentsession.Session
	cancel    context.CancelFunc
	done      <-chan struct{}
	modelMu   sync.Mutex
	model     string
}

func (m *managedRuntime) CurrentModel() string {
	if m == nil {
		return ""
	}
	m.modelMu.Lock()
	defer m.modelMu.Unlock()
	return strings.TrimSpace(m.model)
}

func (m *managedRuntime) SetCurrentModel(model string) {
	if m == nil {
		return
	}
	m.modelMu.Lock()
	m.model = strings.TrimSpace(model)
	m.modelMu.Unlock()
}

// subagentCleanupNotifier stops and finalizes the subagent run when the parent
// completes a subagent callback task successfully (ephemeral subagent cleanup).
type subagentCleanupNotifier struct {
	supervisor *runtimeSupervisor
	next       agent.Notifier
}

func (n *subagentCleanupNotifier) Notify(ctx context.Context, task types.Task, tr types.TaskResult) error {
	if n != nil && n.supervisor != nil {
		if source, _ := task.Metadata["source"].(string); source == "subagent.callback" &&
			tr.Status == types.TaskStatusSucceeded {
			// Only cleanup on explicit approval or legacy no-review-gate callbacks.
			// "retry" keeps the child alive; "escalate" defers cleanup to escalation resolution.
			reviewDecision, _ := task.Metadata["reviewDecision"].(string)
			if reviewDecision == "" || reviewDecision == "approve" {
				if runID, ok := task.Metadata["sourceRunId"].(string); ok && strings.TrimSpace(runID) != "" {
					runID = strings.TrimSpace(runID)
					_ = n.supervisor.StopRun(runID)
					if n.supervisor.sessionService != nil {
						_, _ = n.supervisor.sessionService.StopRun(context.Background(), runID, types.RunStatusSucceeded, "")
					}
				}
			}
		}
	}
	if n != nil && n.next != nil {
		return n.next.Notify(ctx, task, tr)
	}
	return nil
}

func newRuntimeSupervisor(cfg runtimeSupervisorConfig) *runtimeSupervisor {
	poll := cfg.PollInterval
	if poll <= 0 {
		poll = 1 * time.Second // Faster inbox poll so callbacks and new tasks are picked up sooner
	}
	return &runtimeSupervisor{
		cfg:              cfg.Cfg,
		resolved:         cfg.Resolved,
		pollInterval:     poll,
		taskService:      cfg.TaskService,
		sessionService:   cfg.SessionService,
		eventsStore:      cfg.EventsStore,
		memoryStore:      cfg.MemoryStore,
		constructorStore: cfg.ConstructorStore,
		llmClient:        cfg.LLMClient,
		notifier:         cfg.Notifier,
		workdirAbs:       cfg.WorkdirAbs,
		bootstrapRunID:   strings.TrimSpace(cfg.BootstrapRunID),
		defaultProfile:   cfg.DefaultProfile,
		workers:          map[string]*managedRuntime{},
	}
}

func (s *runtimeSupervisor) Run(ctx context.Context) {
	if s == nil {
		return
	}
	if err := s.syncOnce(ctx); err != nil {
		log.Printf("daemon: runtime supervisor sync failed: %v", err)
	}

	t := time.NewTicker(1 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			s.stopAll()
			return
		case <-t.C:
			if err := s.syncOnce(ctx); err != nil {
				log.Printf("daemon: runtime supervisor sync failed: %v", err)
			}
		}
	}
}

func (s *runtimeSupervisor) stopAll() {
	s.mu.Lock()
	workers := make([]*managedRuntime, 0, len(s.workers))
	for _, w := range s.workers {
		workers = append(workers, w)
	}
	s.workers = map[string]*managedRuntime{}
	s.mu.Unlock()
	for _, w := range workers {
		if w == nil {
			continue
		}
		if w.cancel != nil {
			w.cancel()
		}
		if w.done != nil {
			<-w.done
		}
	}
}

func (s *runtimeSupervisor) syncOnce(ctx context.Context) error {
	if s == nil || s.sessionService == nil {
		return nil
	}
	runs, err := s.sessionService.ListRunsByStatus(ctx, []string{types.RunStatusRunning, types.RunStatusPaused})
	if err != nil {
		return err
	}
	for _, run := range runs {
		sess, lerr := s.sessionService.LoadSession(ctx, run.SessionID)
		if lerr != nil {
			log.Printf("daemon: load session for run %s: %v", run.RunID, lerr)
			continue
		}
		teamID := strings.TrimSpace(sess.TeamID)
		if teamID != "" && strings.TrimSpace(run.ParentRunID) != "" {
			// Team mode does not support spawned child runs; stop legacy runs.
			log.Printf("daemon: stopping legacy team child run %s (team %s)", run.RunID, teamID)
			_ = s.StopRun(run.RunID)
			_, _ = s.sessionService.StopRun(ctx, run.RunID, types.RunStatusCanceled, "legacy team child run cleaned up")
			continue
		}
		if teamID != "" && run.Runtime != nil && strings.TrimSpace(run.Runtime.Role) == "" {
			if _, roleByRun := loadTeamManifestRunRoles(s.cfg.DataDir, teamID); len(roleByRun) != 0 {
				if role := strings.TrimSpace(roleByRun[strings.TrimSpace(run.RunID)]); role != "" {
					run.Runtime.Role = role
					_ = s.sessionService.SaveRun(ctx, run)
				}
			}
		}
		if err := s.ensureRun(ctx, sess, run.RunID); err != nil {
			log.Printf("daemon: managed run start failed for %s: %v", run.RunID, err)
		}
	}
	return nil
}

func collectSessionRunIDs(sess types.Session) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(sess.Runs)+1)
	if id := strings.TrimSpace(sess.CurrentRunID); id != "" {
		seen[id] = struct{}{}
		out = append(out, id)
	}
	for _, id := range sess.Runs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func (s *runtimeSupervisor) ensureRun(ctx context.Context, sess types.Session, runID string) error {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return nil
	}
	if runID == s.bootstrapRunID {
		return nil
	}
	run, err := s.sessionService.LoadRun(ctx, runID)
	if err != nil {
		return err
	}
	paused := strings.EqualFold(strings.TrimSpace(run.Status), types.RunStatusPaused)

	s.mu.Lock()
	if existing, ok := s.workers[runID]; ok {
		s.mu.Unlock()
		if existing != nil && existing.session != nil {
			existing.session.SetPaused(paused)
		}
		return nil
	}
	s.mu.Unlock()

	startFn := s.spawnOverride
	if startFn == nil {
		startFn = s.spawnManagedRun
	}
	managed, err := startFn(ctx, sess, runID)
	if err != nil {
		return err
	}
	if managed == nil {
		return fmt.Errorf("managed runtime is nil")
	}

	s.mu.Lock()
	if existing, ok := s.workers[runID]; ok {
		s.mu.Unlock()
		if managed.cancel != nil {
			managed.cancel()
		}
		if managed.done != nil {
			<-managed.done
		}
		if existing != nil {
			return nil
		}
		return nil
	}
	s.workers[runID] = managed
	s.mu.Unlock()

	if managed.done != nil {
		go func() {
			<-managed.done
			s.mu.Lock()
			delete(s.workers, runID)
			s.mu.Unlock()
		}()
	}
	return nil
}

func (s *runtimeSupervisor) spawnManagedRun(parent context.Context, sess types.Session, runID string) (*managedRuntime, error) {
	run, err := s.sessionService.LoadRun(parent, runID)
	if err != nil {
		return nil, err
	}
	paused := strings.EqualFold(strings.TrimSpace(run.Status), types.RunStatusPaused)
	if run.Runtime == nil {
		run.Runtime = &types.RunRuntimeConfig{}
	}
	if strings.TrimSpace(run.SessionID) == "" {
		run.SessionID = strings.TrimSpace(sess.SessionID)
	}

	profRef := strings.TrimSpace(run.Runtime.Profile)
	if profRef == "" {
		profRef = strings.TrimSpace(sess.Profile)
	}
	if profRef == "" && s.defaultProfile != nil {
		profRef = strings.TrimSpace(s.defaultProfile.ID)
	}
	if profRef == "" {
		profRef = "general"
	}
	prof, profDir, err := resolveProfileRef(s.cfg, profRef)
	if err != nil {
		return nil, fmt.Errorf("resolve profile %q: %w", profRef, err)
	}
	if prof == nil {
		return nil, fmt.Errorf("profile %q not found", profRef)
	}

	teamID := strings.TrimSpace(run.Runtime.TeamID)
	if teamID == "" {
		teamID = strings.TrimSpace(sess.TeamID)
	}
	roleName := strings.TrimSpace(run.Runtime.Role)
	isTeam := teamID != ""

	activeProfile := prof
	coordinatorRole := ""
	teamRoles := []string{}
	teamRoleDescriptions := map[string]string{}
	isCoordinator := false
	if isTeam {
		if prof.Team == nil {
			return nil, fmt.Errorf("team run %s requires a team profile", runID)
		}
		roles, coord, err := team.ValidateTeamRoles(prof.Team.Roles)
		if err != nil {
			return nil, err
		}
		teamRoles = roles
		coordinatorRole = coord
		if roleName == "" {
			_, roleByRun := loadTeamManifestRunRoles(s.cfg.DataDir, teamID)
			roleName = strings.TrimSpace(roleByRun[strings.TrimSpace(run.RunID)])
		}
		if roleName == "" {
			return nil, fmt.Errorf("team run %s has no role mapping", runID)
		}
		var roleCfg *profile.RoleConfig
		for i := range prof.Team.Roles {
			r := prof.Team.Roles[i]
			name := strings.TrimSpace(r.Name)
			if name != "" {
				teamRoleDescriptions[name] = strings.TrimSpace(r.Description)
			}
			if strings.EqualFold(name, roleName) {
				copy := r
				roleCfg = &copy
			}
		}
		if roleCfg == nil {
			return nil, fmt.Errorf("role %q not found in team profile %q", roleName, prof.ID)
		}
		isCoordinator = strings.EqualFold(strings.TrimSpace(roleCfg.Name), coordinatorRole)
		activeProfile = &profile.Profile{
			ID:          strings.TrimSpace(roleCfg.Name),
			Description: strings.TrimSpace(roleCfg.Description),
			Prompts:     roleCfg.Prompts,
			Skills:      append([]string(nil), roleCfg.Skills...),
			Heartbeat:   append([]profile.HeartbeatJob(nil), roleCfg.Heartbeat...),
		}
	}

	if run.Runtime == nil {
		run.Runtime = &types.RunRuntimeConfig{}
	}
	run.Runtime.Profile = strings.TrimSpace(prof.ID)
	run.Runtime.TeamID = teamID
	run.Runtime.Role = roleName

	// For child runs, the run's explicit model (set at spawn time from subagent_model
	// config) takes precedence over the session's active model, which reflects the
	// parent's model choice.
	var model string
	if strings.TrimSpace(run.ParentRunID) != "" {
		model = strings.TrimSpace(run.Runtime.Model)
	}
	if model == "" {
		model = strings.TrimSpace(sess.ActiveModel)
	}
	if model == "" {
		model = strings.TrimSpace(run.Runtime.Model)
	}
	if model == "" {
		model = strings.TrimSpace(s.resolved.Model)
	}
	if model == "" {
		return nil, fmt.Errorf("run %s has no configured model", runID)
	}
	resolvedEffort, resolvedSummary := sessionReasoningForModel(
		sess,
		model,
		strings.TrimSpace(s.resolved.ReasoningEffort),
		strings.TrimSpace(s.resolved.ReasoningSummary),
	)
	run.Runtime.Model = model
	_ = s.sessionService.SaveRun(parent, run)

	traceStore := implstore.SQLiteTraceStore{Cfg: s.cfg, RunID: run.RunID}
	historyStore, err := implstore.NewSQLiteHistoryStore(s.cfg, run.SessionID)
	if err != nil {
		return nil, err
	}

	store := s.eventsStore
	if store == nil {
		store = eventsvc.NewService(s.cfg)
	}
	emitter := &events.Emitter{
		RunID: run.RunID,
		Sink: events.StoreSink{
			Store: store,
		},
	}
	orderedEmitter := emit.NewOrderedEmitter[events.Event](emitter)
	emitEvent := func(ctx context.Context, ev events.Event) {
		if ev.Data == nil {
			ev.Data = map[string]string{}
		}
		if teamID != "" {
			ev.Data["teamId"] = teamID
		}
		if roleName != "" {
			ev.Data["role"] = roleName
		}
		if err := orderedEmitter.Emit(ctx, ev); err != nil && !errorsIsDropped(err) {
			log.Printf("daemon: emit failed (%s): %v", runID, err)
		}
	}

	sharedWorkspaceDir := ""
	if teamID != "" {
		sharedWorkspaceDir = fsutil.GetTeamWorkspaceDir(s.cfg.DataDir, teamID)
		if err := os.MkdirAll(sharedWorkspaceDir, 0o755); err != nil {
			orderedEmitter.Close()
			return nil, fmt.Errorf("prepare team workspace mount: %w", err)
		}
	}

	rt, err := runtime.Build(runtime.BuildConfig{
		Cfg:                   s.cfg,
		Run:                   run,
		Profile:               strings.TrimSpace(prof.ID),
		ProfileConfig:         prof,
		WorkdirAbs:            s.workdirAbs,
		SharedWorkspaceDir:    sharedWorkspaceDir,
		Model:                 model,
		ReasoningEffort:       resolvedEffort,
		ReasoningSummary:      resolvedSummary,
		ApprovalsMode:         strings.TrimSpace(s.resolved.ApprovalsMode),
		HistoryStore:          historyStore,
		MemoryStore:           s.memoryStore,
		TraceStore:            traceStore,
		ConstructorStore:      s.constructorStore,
		Emit:                  emitEvent,
		IncludeHistoryOps:     derefBool(s.resolved.IncludeHistoryOps, true),
		RecentHistoryPairs:    s.resolved.RecentHistoryPairs,
		MaxMemoryBytes:        s.resolved.MaxMemoryBytes,
		MaxTraceBytes:         s.resolved.MaxTraceBytes,
		PriceInPerMTokensUSD:  s.resolved.PriceInPerMTokensUSD,
		PriceOutPerMTokensUSD: s.resolved.PriceOutPerMTokensUSD,
		PersistRun: func(r types.Run) error {
			return s.sessionService.SaveRun(context.Background(), r)
		},
		LoadSession: func(sessionID string) (types.Session, error) {
			return s.sessionService.LoadSession(context.Background(), sessionID)
		},
		SaveSession: func(session types.Session) error {
			return s.sessionService.SaveSession(context.Background(), session)
		},
	})
	if err != nil {
		orderedEmitter.Close()
		return nil, err
	}

	agentCfg := agent.DefaultConfig()
	agentCfg.Model = model
	agentCfg.ReasoningEffort = resolvedEffort
	agentCfg.ReasoningSummary = resolvedSummary
	agentCfg.ApprovalsMode = strings.TrimSpace(s.resolved.ApprovalsMode)
	agentCfg.EnableWebSearch = s.resolved.WebSearchEnabled
	isChildRun := strings.TrimSpace(run.ParentRunID) != ""
	if isChildRun {
		agentCfg.SystemPrompt = prompts.DefaultSubAgentSystemPrompt()
	} else {
		agentCfg.SystemPrompt = prompts.DefaultAutonomousSystemPrompt()
	}
	promptSource := agent.PromptSource(rt.Constructor)
	if rt.Updater != nil {
		promptSource = rt.Updater
	}
	agentCfg.PromptSource = promptSource

	managed := &managedRuntime{
		runID:     strings.TrimSpace(run.RunID),
		sessionID: strings.TrimSpace(run.SessionID),
		model:     strings.TrimSpace(model),
	}
	agentCfg.Hooks = agent.Hooks{
		OnLLMUsage: newCostUsageHook(
			s.cfg,
			run,
			model,
			s.resolved.PriceInPerMTokensUSD,
			s.resolved.PriceOutPerMTokensUSD,
			s.sessionService,
			func() string {
				return managed.CurrentModel()
			},
			emitEvent,
		),
		OnStep: func(step int, model, effectiveModel, summary string) {
			data := map[string]string{
				"step":  strconv.Itoa(step),
				"model": strings.TrimSpace(model),
			}
			if em := strings.TrimSpace(effectiveModel); em != "" {
				data["effectiveModel"] = em
			}
			if s := strings.TrimSpace(summary); s != "" {
				data["reasoningSummary"] = s
			}
			emitEvent(context.Background(), events.Event{Type: "agent.step", Message: fmt.Sprintf("Step %d completed", step), Data: data})
		},
	}

	registry, err := agent.DefaultHostToolRegistry()
	if err != nil {
		orderedEmitter.Close()
		_ = rt.Shutdown(context.Background())
		return nil, err
	}
	tool := &hosttools.TaskCreateTool{
		Store:      s.taskService,
		SessionID:  run.SessionID,
		RunID:      run.RunID,
		IsChildRun: isChildRun,
	}
	if teamID != "" {
		tool.TeamID = teamID
		tool.RoleName = roleName
		tool.IsCoordinator = isCoordinator
		tool.CoordinatorRole = coordinatorRole
		tool.ValidRoles = teamRoles
	} else if run.ParentRunID == "" {
		// Standalone mode (non-team, non-child): enable spawn_worker.
		tool.SpawnWorker = s.makeSpawnWorkerFunc(run, model, emitEvent)
	}
	if err := registry.Register(tool); err != nil {
		orderedEmitter.Close()
		_ = rt.Shutdown(context.Background())
		return nil, err
	}
	// Register task_review tool for agents that can receive callbacks (non-child runs).
	if !isChildRun {
		reviewTool := &hosttools.TaskReviewTool{
			Store:      s.taskService,
			SessionID:  run.SessionID,
			RunID:      run.RunID,
			Supervisor: s,
		}
		if err := registry.Register(reviewTool); err != nil {
			orderedEmitter.Close()
			_ = rt.Shutdown(context.Background())
			return nil, err
		}
	}
	agentCfg.HostToolRegistry = registry

	runLLMClient := withRetryDiagnostics(s.llmClient, emitEvent)
	a, err := agent.NewAgent(runLLMClient, rt.Executor, agentCfg)
	if err != nil {
		orderedEmitter.Close()
		_ = rt.Shutdown(context.Background())
		return nil, err
	}
	workerSession, err := agentsession.New(agentsession.Config{
		Agent:      a,
		Profile:    activeProfile,
		ProfileDir: profDir,
		ResolveProfile: func(ref string) (*profile.Profile, string, error) {
			return resolveProfileRef(s.cfg, strings.TrimSpace(ref))
		},
		TaskStore:            s.taskService,
		Events:               orderedEmitter,
		Memory:               &textMemoryAdapter{store: s.memoryStore},
		MemorySearchLimit:    3,
		Notifier:             &subagentCleanupNotifier{supervisor: s, next: s.notifier},
		PollInterval:         s.pollInterval,
		MaxReadBytes:         256 * 1024,
		LeaseTTL:             2 * time.Minute,
		MaxRetries:           3,
		MaxPending:           50,
		SessionID:            run.SessionID,
		RunID:                run.RunID,
		TeamID:               teamID,
		RoleName:             roleName,
		IsCoordinator:        isCoordinator,
		CoordinatorRole:      coordinatorRole,
		TeamRoles:            teamRoles,
		TeamRoleDescriptions: teamRoleDescriptions,
		ParentRunID:          strings.TrimSpace(run.ParentRunID),
		SingleTask:           isChildRun,
		InstanceID:           run.RunID,
		Logf: func(format string, args ...any) {
			log.Printf("daemon [%s]: "+format, append([]any{run.RunID}, args...)...)
		},
	})
	if err != nil {
		orderedEmitter.Close()
		_ = rt.Shutdown(context.Background())
		return nil, err
	}
	workerSession.SetPaused(paused)

	workerCtx, cancel := context.WithCancel(parent)
	done := make(chan struct{})

	go func() {
		defer close(done)
		defer orderedEmitter.Close()
		defer func() { _ = rt.Shutdown(context.Background()) }()
		defer cancel()

		emitEvent(workerCtx, events.Event{
			Type:    "run.start",
			Message: "Agent started",
			Data: map[string]string{
				"runId":     run.RunID,
				"sessionId": run.SessionID,
				"profile":   strings.TrimSpace(activeProfile.ID),
			},
		})

		syncRuntimeControls := func() {
			loaded, err := s.sessionService.LoadSession(workerCtx, strings.TrimSpace(run.SessionID))
			if err != nil {
				return
			}
			// Only sync model from session for top-level runs. Child runs (sub-agents) keep the
			// model set at spawn (profile/env); session ActiveModel reflects the parent's choice.
			isChildRun := strings.TrimSpace(run.ParentRunID) != ""
			if !isChildRun {
				targetModel := strings.TrimSpace(loaded.ActiveModel)
				if targetModel != "" {
					same := strings.EqualFold(targetModel, managed.CurrentModel())
					if !same {
						if err := workerSession.SetModel(workerCtx, targetModel); err == nil {
							managed.SetCurrentModel(targetModel)
							emitEvent(workerCtx, events.Event{
								Type:    "control.success",
								Message: "Model synchronized from session state",
								Data: map[string]string{
									"command": "set_model",
									"model":   targetModel,
								},
							})
						}
					}
				}
			}
			// Only sync reasoning from session for top-level runs; subagents keep profile/env settings from spawn.
			if !isChildRun {
				targetModel := strings.TrimSpace(loaded.ActiveModel)
				targetEffort, targetSummary := sessionReasoningForModel(
					loaded,
					targetModel,
					strings.TrimSpace(s.resolved.ReasoningEffort),
					strings.TrimSpace(s.resolved.ReasoningSummary),
				)
				_ = workerSession.SetReasoning(workerCtx, targetEffort, targetSummary)
			}
		}

		backoff := 2 * time.Second
		for {
			runLoopCtx, stopRunLoop := context.WithCancel(workerCtx)
			errCh := make(chan error, 1)
			go func() { errCh <- workerSession.Run(runLoopCtx) }()

			ticker := time.NewTicker(2 * time.Second)
			stopped := false
			for !stopped {
				select {
				case <-workerCtx.Done():
					stopRunLoop()
					<-errCh
					ticker.Stop()
					return
				case <-ticker.C:
					syncRuntimeControls()
				case err := <-errCh:
					stopRunLoop()
					ticker.Stop()
					if workerCtx.Err() != nil {
						return
					}
					if errors.Is(err, agentsession.ErrSingleTaskComplete) {
						emitEvent(workerCtx, events.Event{
							Type:    "subagent.finished",
							Message: "Spawned worker completed its task",
							Data:    map[string]string{"runId": run.RunID},
						})
						_, _ = s.sessionService.StopRun(context.Background(), run.RunID, types.RunStatusSucceeded, "")
						s.mu.Lock()
						delete(s.workers, run.RunID)
						s.mu.Unlock()
						return
					}
					errMsg := "unknown error"
					if err != nil {
						errMsg = err.Error()
					}
					emitEvent(workerCtx, events.Event{
						Type:    "daemon.runner.error",
						Message: "Runner exited unexpectedly; restarting",
						Data:    map[string]string{"error": errMsg},
					})
					time.Sleep(backoff)
					if backoff < 60*time.Second {
						backoff *= 2
						if backoff > 60*time.Second {
							backoff = 60 * time.Second
						}
					}
					stopped = true
				}
			}
		}
	}()

	managed.session = workerSession
	managed.cancel = cancel
	managed.done = done
	return managed, nil
}

func (s *runtimeSupervisor) PauseRun(runID string) error {
	if s == nil {
		return nil
	}
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return fmt.Errorf("run id is required")
	}
	run, err := s.sessionService.LoadRun(context.Background(), runID)
	if err != nil {
		return err
	}
	if strings.EqualFold(strings.TrimSpace(run.Status), types.RunStatusPaused) {
		s.mu.Lock()
		worker := s.workers[runID]
		s.mu.Unlock()
		if worker != nil && worker.session != nil {
			worker.session.SetPaused(true)
		}
		if worker != nil && worker.cancel != nil {
			worker.cancel()
		}
		if worker != nil && worker.done != nil {
			<-worker.done
		}
		s.mu.Lock()
		delete(s.workers, runID)
		s.mu.Unlock()
		_, err := s.taskService.CancelActiveTasksByRun(context.Background(), runID, "run paused")
		return err
	}
	run.Status = types.RunStatusPaused
	run.FinishedAt = nil
	run.Error = nil
	if err := s.sessionService.SaveRun(context.Background(), run); err != nil {
		return err
	}

	s.mu.Lock()
	worker := s.workers[runID]
	s.mu.Unlock()
	if worker != nil && worker.session != nil {
		worker.session.SetPaused(true)
	}
	if worker != nil && worker.cancel != nil {
		worker.cancel()
	}
	if worker != nil && worker.done != nil {
		<-worker.done
	}
	s.mu.Lock()
	delete(s.workers, runID)
	s.mu.Unlock()
	_, err = s.taskService.CancelActiveTasksByRun(context.Background(), runID, "run paused")
	return err
}

func (s *runtimeSupervisor) ResumeRun(ctx context.Context, runID string) error {
	if s == nil {
		return nil
	}
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return fmt.Errorf("run id is required")
	}
	run, err := s.sessionService.LoadRun(ctx, runID)
	if err != nil {
		return err
	}
	run.Status = types.RunStatusRunning
	run.FinishedAt = nil
	run.Error = nil
	if err := s.sessionService.SaveRun(ctx, run); err != nil {
		return err
	}

	s.mu.Lock()
	worker := s.workers[runID]
	s.mu.Unlock()
	if worker != nil && worker.session != nil {
		worker.session.SetPaused(false)
		return nil
	}
	if s.sessionService == nil {
		return nil
	}
	sess, err := s.sessionService.LoadSession(ctx, strings.TrimSpace(run.SessionID))
	if err != nil {
		return err
	}
	return s.ensureRun(ctx, sess, runID)
}

func (s *runtimeSupervisor) StopRun(runID string) error {
	if s == nil {
		return nil
	}
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return fmt.Errorf("run id is required")
	}
	run, err := s.sessionService.LoadRun(context.Background(), runID)
	if err != nil {
		return err
	}
	run.Status = types.RunStatusPaused
	run.FinishedAt = nil
	run.Error = nil
	if err := s.sessionService.SaveRun(context.Background(), run); err != nil {
		return err
	}

	s.mu.Lock()
	worker := s.workers[runID]
	s.mu.Unlock()

	if worker != nil && worker.session != nil {
		worker.session.SetPaused(true)
	}
	if worker != nil && worker.cancel != nil {
		worker.cancel()
	}
	if worker != nil && worker.done != nil {
		<-worker.done
	}
	s.mu.Lock()
	delete(s.workers, runID)
	s.mu.Unlock()
	_, err = s.taskService.CancelActiveTasksByRun(context.Background(), runID, "run stopped")
	return err
}

func (s *runtimeSupervisor) PauseSession(ctx context.Context, sessionID string) ([]string, error) {
	if s == nil {
		return nil, nil
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, fmt.Errorf("session id is required")
	}
	if s.sessionService == nil {
		return nil, fmt.Errorf("session store not configured")
	}
	sess, err := s.sessionService.LoadSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	runIDs := collectSessionRunIDs(sess)
	affected := make([]string, 0, len(runIDs))
	var mu sync.Mutex
	var wg sync.WaitGroup
	errs := make([]string, 0, len(runIDs))
	for _, runID := range runIDs {
		runID := strings.TrimSpace(runID)
		if runID == "" {
			continue
		}
		wg.Add(1)
		go func(rid string) {
			defer wg.Done()
			if err := s.PauseRun(rid); err != nil {
				mu.Lock()
				errs = append(errs, rid+": "+err.Error())
				mu.Unlock()
				return
			}
			mu.Lock()
			affected = append(affected, rid)
			mu.Unlock()
		}(runID)
	}
	wg.Wait()
	if len(errs) != 0 {
		return affected, fmt.Errorf("pause session partial failure: %s", strings.Join(errs, "; "))
	}
	return affected, nil
}

func (s *runtimeSupervisor) ResumeSession(ctx context.Context, sessionID string) ([]string, error) {
	if s == nil {
		return nil, nil
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, fmt.Errorf("session id is required")
	}
	if s.sessionService == nil {
		return nil, fmt.Errorf("session store not configured")
	}
	sess, err := s.sessionService.LoadSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	runIDs := collectSessionRunIDs(sess)
	affected := make([]string, 0, len(runIDs))
	var mu sync.Mutex
	var wg sync.WaitGroup
	errs := make([]string, 0, len(runIDs))
	for _, runID := range runIDs {
		runID := strings.TrimSpace(runID)
		if runID == "" {
			continue
		}
		wg.Add(1)
		go func(rid string) {
			defer wg.Done()
			if err := s.ResumeRun(ctx, rid); err != nil {
				mu.Lock()
				errs = append(errs, rid+": "+err.Error())
				mu.Unlock()
				return
			}
			mu.Lock()
			affected = append(affected, rid)
			mu.Unlock()
		}(runID)
	}
	wg.Wait()
	if len(errs) != 0 {
		return affected, fmt.Errorf("resume session partial failure: %s", strings.Join(errs, "; "))
	}
	return affected, nil
}

func (s *runtimeSupervisor) StopSession(ctx context.Context, sessionID string) ([]string, error) {
	if s == nil {
		return nil, nil
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, fmt.Errorf("session id is required")
	}
	if s.sessionService == nil {
		return nil, fmt.Errorf("session store not configured")
	}
	sess, err := s.sessionService.LoadSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	runIDs := collectSessionRunIDs(sess)
	affected := make([]string, 0, len(runIDs))
	var mu sync.Mutex
	var wg sync.WaitGroup
	errs := make([]string, 0, len(runIDs))
	for _, runID := range runIDs {
		runID := strings.TrimSpace(runID)
		if runID == "" {
			continue
		}
		wg.Add(1)
		go func(rid string) {
			defer wg.Done()
			if err := s.StopRun(rid); err != nil {
				mu.Lock()
				errs = append(errs, rid+": "+err.Error())
				mu.Unlock()
				return
			}
			mu.Lock()
			affected = append(affected, rid)
			mu.Unlock()
		}(runID)
	}
	wg.Wait()
	if len(errs) != 0 {
		return affected, fmt.Errorf("stop session partial failure: %s", strings.Join(errs, "; "))
	}
	return affected, nil
}

func (s *runtimeSupervisor) ApplySessionReasoning(ctx context.Context, sessionID, targetRunID, effort, summary string) ([]string, error) {
	if s == nil {
		return nil, nil
	}
	sessionID = strings.TrimSpace(sessionID)
	targetRunID = strings.TrimSpace(targetRunID)
	if sessionID == "" {
		return nil, fmt.Errorf("session id is required")
	}
	effort = strings.TrimSpace(effort)
	summary = strings.TrimSpace(summary)
	if effort == "" && summary == "" {
		return nil, nil
	}
	s.mu.Lock()
	workers := make([]*managedRuntime, 0, len(s.workers))
	for _, w := range s.workers {
		workers = append(workers, w)
	}
	s.mu.Unlock()
	applied := make([]string, 0, len(workers))
	for _, worker := range workers {
		if worker == nil || worker.session == nil {
			continue
		}
		if strings.TrimSpace(worker.sessionID) != sessionID {
			continue
		}
		runID := strings.TrimSpace(worker.runID)
		if targetRunID != "" && targetRunID != runID {
			continue
		}
		if err := worker.session.SetReasoning(ctx, effort, summary); err != nil {
			return applied, err
		}
		applied = append(applied, runID)
	}
	return applied, nil
}

func (s *runtimeSupervisor) ApplySessionModel(ctx context.Context, sessionID, targetRunID, model string) ([]string, error) {
	if s == nil {
		return nil, nil
	}
	sessionID = strings.TrimSpace(sessionID)
	targetRunID = strings.TrimSpace(targetRunID)
	model = strings.TrimSpace(model)
	if sessionID == "" {
		return nil, fmt.Errorf("session id is required")
	}
	if model == "" {
		return nil, fmt.Errorf("model is required")
	}
	s.mu.Lock()
	workers := make([]*managedRuntime, 0, len(s.workers))
	for _, w := range s.workers {
		workers = append(workers, w)
	}
	s.mu.Unlock()
	applied := make([]string, 0, len(workers))
	for _, worker := range workers {
		if worker == nil || worker.session == nil {
			continue
		}
		if strings.TrimSpace(worker.sessionID) != sessionID {
			continue
		}
		runID := strings.TrimSpace(worker.runID)
		if targetRunID != "" && targetRunID != runID {
			continue
		}
		// Do not apply session model to child runs (sub-agents); they keep the model set at spawn.
		wr, err := s.sessionService.LoadRun(ctx, runID)
		if err != nil {
			continue
		}
		if strings.TrimSpace(wr.ParentRunID) != "" {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(worker.CurrentModel()), model) {
			continue
		}
		if err := worker.session.SetModel(ctx, model); err != nil {
			return applied, err
		}
		worker.SetCurrentModel(model)
		applied = append(applied, runID)
	}
	return applied, nil
}

// makeSpawnWorkerFunc returns a SpawnWorkerFunc that creates a child Run and
// adds it to the session. The supervisor discovers the new run via syncOnce.
func (s *runtimeSupervisor) makeSpawnWorkerFunc(
	parentRun types.Run,
	parentModel string,
	parentEmit events.EmitFunc,
) hosttools.SpawnWorkerFunc {
	return func(ctx context.Context, goal, sessionID, parentRunID string) (string, error) {
		if s.sessionService != nil {
			if sess, err := s.sessionService.LoadSession(ctx, sessionID); err == nil {
				if strings.TrimSpace(sess.TeamID) != "" {
					return "", fmt.Errorf("spawn_worker unavailable in team mode")
				}
			}
		}
		// Count existing children to determine spawn index.
		children, _ := s.sessionService.ListChildRuns(ctx, parentRunID)
		spawnIndex := len(children) + 1

		childRun := types.NewChildRun(parentRunID, goal, sessionID, spawnIndex)

		// Resolve subagent model: env var > profile-level > parent model.
		subagentModel := strings.TrimSpace(s.resolved.SubagentModel)
		if subagentModel == "" && s.defaultProfile != nil {
			subagentModel = strings.TrimSpace(s.defaultProfile.SubagentModel)
		}
		if subagentModel == "" {
			subagentModel = parentModel
		}

		childRun.Runtime = &types.RunRuntimeConfig{
			DataDir: s.cfg.DataDir,
			Model:   subagentModel,
		}
		if parentRun.Runtime != nil {
			childRun.Runtime.Profile = parentRun.Runtime.Profile
		}

		if err := s.sessionService.SaveRun(ctx, childRun); err != nil {
			return "", fmt.Errorf("save child run: %w", err)
		}

		// Add child run to session's run list so the supervisor discovers it.
		sess, err := s.sessionService.LoadSession(ctx, sessionID)
		if err != nil {
			return "", fmt.Errorf("load session for spawn: %w", err)
		}
		sess.Runs = append(sess.Runs, childRun.RunID)
		if err := s.sessionService.SaveSession(ctx, sess); err != nil {
			return "", fmt.Errorf("save session for spawn: %w", err)
		}

		if parentEmit != nil {
			parentEmit(ctx, events.Event{
				Type:    "subagent.spawned",
				Message: fmt.Sprintf("Spawned worker agent #%d: %s", spawnIndex, goal),
				Data: map[string]string{
					"childRunId":  childRun.RunID,
					"parentRunId": parentRunID,
					"spawnIndex":  strconv.Itoa(spawnIndex),
					"goal":        goal,
					"model":       subagentModel,
				},
			})
		}

		return childRun.RunID, nil
	}
}

// RetrySubagent creates a new retry task for an existing child run so the
// sub-agent can re-attempt its work with the parent's feedback.
func (s *runtimeSupervisor) RetrySubagent(ctx context.Context, childRunID, feedback string) error {
	return s.taskService.CreateRetryTask(ctx, childRunID, feedback)
}

// EscalateTask creates an escalation task with structured metadata. In team mode
// it routes to the coordinator; in standalone mode it routes to the parent run
// and emits a user-facing event.
func (s *runtimeSupervisor) EscalateTask(ctx context.Context, callbackTaskID string, data hosttools.EscalationData) error {
	if err := s.taskService.CreateEscalationTask(ctx, callbackTaskID, data); err != nil {
		return err
	}
	// Stop the child run after escalation.
	childRunID := strings.TrimSpace(data.SourceRunID)
	if childRunID != "" {
		_ = s.StopRun(childRunID)
		if s.sessionService != nil {
			_, _ = s.sessionService.StopRun(ctx, childRunID, types.RunStatusFailed, "escalated")
		}
	}
	return nil
}
