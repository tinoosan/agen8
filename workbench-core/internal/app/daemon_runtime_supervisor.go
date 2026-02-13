package app

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	implstore "github.com/tinoosan/workbench-core/internal/store"
	"github.com/tinoosan/workbench-core/pkg/agent"
	hosttools "github.com/tinoosan/workbench-core/pkg/agent/hosttools"
	agentsession "github.com/tinoosan/workbench-core/pkg/agent/session"
	"github.com/tinoosan/workbench-core/pkg/agent/state"
	"github.com/tinoosan/workbench-core/pkg/config"
	"github.com/tinoosan/workbench-core/pkg/emit"
	"github.com/tinoosan/workbench-core/pkg/events"
	"github.com/tinoosan/workbench-core/pkg/fsutil"
	llmtypes "github.com/tinoosan/workbench-core/pkg/llm/types"
	"github.com/tinoosan/workbench-core/pkg/profile"
	"github.com/tinoosan/workbench-core/pkg/runtime"
	pkgstore "github.com/tinoosan/workbench-core/pkg/store"
	"github.com/tinoosan/workbench-core/pkg/types"
)

type runtimeSupervisorConfig struct {
	Cfg              config.Config
	Resolved         RunChatOptions
	PollInterval     time.Duration
	TaskStore        state.TaskStore
	SessionStore     pkgstore.SessionReaderWriter
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
	taskStore        state.TaskStore
	sessionStore     pkgstore.SessionReaderWriter
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
}

func newRuntimeSupervisor(cfg runtimeSupervisorConfig) *runtimeSupervisor {
	poll := cfg.PollInterval
	if poll <= 0 {
		poll = 2 * time.Second
	}
	return &runtimeSupervisor{
		cfg:              cfg.Cfg,
		resolved:         cfg.Resolved,
		pollInterval:     poll,
		taskStore:        cfg.TaskStore,
		sessionStore:     cfg.SessionStore,
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
	if s == nil {
		return nil
	}
	if s.sessionStore == nil {
		return nil
	}
	query, ok := s.sessionStore.(pkgstore.SessionQuery)
	if !ok {
		return nil
	}
	filter := pkgstore.SessionFilter{
		IncludeSystem: false,
		Limit:         200,
		Offset:        0,
		SortBy:        "updated_at",
		SortDesc:      true,
	}

	for {
		sessions, err := query.ListSessionsPaginated(ctx, filter)
		if err != nil {
			return err
		}
		if len(sessions) == 0 {
			break
		}
		for _, sess := range sessions {
			runIDs := collectSessionRunIDs(sess)
			for _, runID := range runIDs {
				if err := s.ensureRun(ctx, sess, runID); err != nil {
					log.Printf("daemon: managed run start failed for %s: %v", runID, err)
				}
			}
		}
		if len(sessions) < filter.Limit {
			break
		}
		filter.Offset += len(sessions)
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
	run, err := implstore.LoadRun(s.cfg, runID)
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
	if paused {
		return nil
	}

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
	run, err := implstore.LoadRun(s.cfg, runID)
	if err != nil {
		return nil, err
	}
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
		roles, coord, err := collectTeamRoles(prof.Team.Roles)
		if err != nil {
			return nil, err
		}
		teamRoles = roles
		coordinatorRole = coord
		if roleName == "" {
			roleName = coordinatorRole
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

	model := strings.TrimSpace(sess.ActiveModel)
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
	_ = implstore.SaveRun(s.cfg, run)

	traceStore := implstore.SQLiteTraceStore{Cfg: s.cfg, RunID: run.RunID}
	historyStore, err := implstore.NewSQLiteHistoryStore(s.cfg, run.SessionID)
	if err != nil {
		return nil, err
	}

	emitter := &events.Emitter{
		RunID: run.RunID,
		Sink: events.StoreSink{
			Store: daemonEventAppender{cfg: s.cfg},
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
			return implstore.SaveRun(s.cfg, r)
		},
		LoadSession: func(sessionID string) (types.Session, error) {
			return s.sessionStore.LoadSession(context.Background(), sessionID)
		},
		SaveSession: func(session types.Session) error {
			return s.sessionStore.SaveSession(context.Background(), session)
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
	agentCfg.SystemPrompt = agent.DefaultAutonomousSystemPrompt()
	promptSource := agent.PromptSource(rt.Constructor)
	if rt.Updater != nil {
		promptSource = rt.Updater
	}
	agentCfg.PromptSource = promptSource

	currentModel := strings.TrimSpace(model)
	var currentModelMu sync.Mutex
	agentCfg.Hooks = agent.Hooks{
		OnLLMUsage: newCostUsageHook(
			s.cfg,
			run,
			model,
			s.resolved.PriceInPerMTokensUSD,
			s.resolved.PriceOutPerMTokensUSD,
			s.sessionStore,
			func() string {
				currentModelMu.Lock()
				defer currentModelMu.Unlock()
				return currentModel
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
		Store:     s.taskStore,
		SessionID: run.SessionID,
		RunID:     run.RunID,
	}
	if teamID != "" {
		tool.TeamID = teamID
		tool.RoleName = roleName
		tool.IsCoordinator = isCoordinator
		tool.CoordinatorRole = coordinatorRole
		tool.ValidRoles = teamRoles
	}
	if err := registry.Register(tool); err != nil {
		orderedEmitter.Close()
		_ = rt.Shutdown(context.Background())
		return nil, err
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
		TaskStore:            s.taskStore,
		Events:               orderedEmitter,
		Memory:               &textMemoryAdapter{store: s.memoryStore},
		MemorySearchLimit:    3,
		Notifier:             s.notifier,
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

	workerCtx, cancel := context.WithCancel(parent)
	done := make(chan struct{})

	go func() {
		defer close(done)
		defer orderedEmitter.Close()
		defer func() { _ = rt.Shutdown(context.Background()) }()
		defer cancel()

		emitEvent(workerCtx, events.Event{
			Type:    "daemon.start",
			Message: "Autonomous agent started",
			Data: map[string]string{
				"runId":     run.RunID,
				"sessionId": run.SessionID,
				"profile":   strings.TrimSpace(activeProfile.ID),
			},
		})

		syncRuntimeControls := func() {
			loaded, err := s.sessionStore.LoadSession(workerCtx, strings.TrimSpace(run.SessionID))
			if err != nil {
				return
			}
			targetModel := strings.TrimSpace(loaded.ActiveModel)
			if targetModel != "" {
				currentModelMu.Lock()
				same := strings.EqualFold(targetModel, currentModel)
				currentModelMu.Unlock()
				if !same {
					if err := workerSession.SetModel(workerCtx, targetModel); err == nil {
						currentModelMu.Lock()
						currentModel = targetModel
						currentModelMu.Unlock()
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
			targetEffort, targetSummary := sessionReasoningForModel(
				loaded,
				targetModel,
				strings.TrimSpace(s.resolved.ReasoningEffort),
				strings.TrimSpace(s.resolved.ReasoningSummary),
			)
			_ = workerSession.SetReasoning(workerCtx, targetEffort, targetSummary)
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

	return &managedRuntime{
		runID:     strings.TrimSpace(run.RunID),
		sessionID: strings.TrimSpace(run.SessionID),
		session:   workerSession,
		cancel:    cancel,
		done:      done,
	}, nil
}

func (s *runtimeSupervisor) PauseRun(runID string) error {
	if s == nil {
		return nil
	}
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return fmt.Errorf("run id is required")
	}
	run, err := implstore.LoadRun(s.cfg, runID)
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
		return cancelActiveTasksForRun(context.Background(), s.taskStore, runID, "run paused")
	}
	run.Status = types.RunStatusPaused
	run.FinishedAt = nil
	run.Error = nil
	if err := implstore.SaveRun(s.cfg, run); err != nil {
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
	return cancelActiveTasksForRun(context.Background(), s.taskStore, runID, "run paused")
}

func (s *runtimeSupervisor) ResumeRun(ctx context.Context, runID string) error {
	if s == nil {
		return nil
	}
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return fmt.Errorf("run id is required")
	}
	run, err := implstore.LoadRun(s.cfg, runID)
	if err != nil {
		return err
	}
	run.Status = types.RunStatusRunning
	run.FinishedAt = nil
	run.Error = nil
	if err := implstore.SaveRun(s.cfg, run); err != nil {
		return err
	}

	s.mu.Lock()
	worker := s.workers[runID]
	s.mu.Unlock()
	if worker != nil && worker.session != nil {
		worker.session.SetPaused(false)
		return nil
	}
	if s.sessionStore == nil {
		return nil
	}
	sess, err := s.sessionStore.LoadSession(ctx, strings.TrimSpace(run.SessionID))
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
	run, err := implstore.LoadRun(s.cfg, runID)
	if err != nil {
		return err
	}
	run.Status = types.RunStatusPaused
	run.FinishedAt = nil
	run.Error = nil
	if err := implstore.SaveRun(s.cfg, run); err != nil {
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
	return cancelActiveTasksForRun(context.Background(), s.taskStore, runID, "run stopped")
}

func (s *runtimeSupervisor) PauseSession(ctx context.Context, sessionID string) ([]string, error) {
	if s == nil {
		return nil, nil
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, fmt.Errorf("session id is required")
	}
	if s.sessionStore == nil {
		return nil, fmt.Errorf("session store not configured")
	}
	sess, err := s.sessionStore.LoadSession(ctx, sessionID)
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
	if s.sessionStore == nil {
		return nil, fmt.Errorf("session store not configured")
	}
	sess, err := s.sessionStore.LoadSession(ctx, sessionID)
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
	if s.sessionStore == nil {
		return nil, fmt.Errorf("session store not configured")
	}
	sess, err := s.sessionStore.LoadSession(ctx, sessionID)
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
		if err := worker.session.SetModel(ctx, model); err != nil {
			return applied, err
		}
		applied = append(applied, runID)
	}
	return applied, nil
}
