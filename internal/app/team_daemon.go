package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/google/uuid"
	implstore "github.com/tinoosan/agen8/internal/store"
	"github.com/tinoosan/agen8/internal/webhook"
	"github.com/tinoosan/agen8/pkg/agent"
	hosttools "github.com/tinoosan/agen8/pkg/agent/hosttools"
	"github.com/tinoosan/agen8/pkg/agent/session"
	"github.com/tinoosan/agen8/pkg/agent/state"
	"github.com/tinoosan/agen8/pkg/config"
	"github.com/tinoosan/agen8/pkg/emit"
	"github.com/tinoosan/agen8/pkg/events"
	"github.com/tinoosan/agen8/pkg/fsutil"
	"github.com/tinoosan/agen8/pkg/llm"
	"github.com/tinoosan/agen8/pkg/profile"
	"github.com/tinoosan/agen8/pkg/prompts"
	"github.com/tinoosan/agen8/pkg/protocol"
	"github.com/tinoosan/agen8/pkg/runtime"
	pkgagent "github.com/tinoosan/agen8/pkg/services/agent"
	eventsvc "github.com/tinoosan/agen8/pkg/services/events"
	pkgsession "github.com/tinoosan/agen8/pkg/services/session"
	pkgsoul "github.com/tinoosan/agen8/pkg/services/soul"
	pkgtask "github.com/tinoosan/agen8/pkg/services/task"
	"github.com/tinoosan/agen8/pkg/services/team"
	"github.com/tinoosan/agen8/pkg/types"
)

type teamRoleRuntime struct {
	role    profile.RoleConfig
	run     types.Run
	sess    *session.Session
	cleanup func()
}

// teamRoleRunControllerAdapter adapts *teamRoleRuntime to team.RoleRunController.
type teamRoleRunControllerAdapter struct {
	rt *teamRoleRuntime
}

func (a *teamRoleRunControllerAdapter) RunID() string { return strings.TrimSpace(a.rt.run.RunID) }
func (a *teamRoleRunControllerAdapter) SessionID() string {
	return strings.TrimSpace(a.rt.run.SessionID)
}
func (a *teamRoleRunControllerAdapter) SetPaused(paused bool) { a.rt.sess.SetPaused(paused) }
func (a *teamRoleRunControllerAdapter) SetModel(ctx context.Context, model string) error {
	return a.rt.sess.SetModel(ctx, model)
}
func (a *teamRoleRunControllerAdapter) SetReasoning(ctx context.Context, effort, summary string) error {
	return a.rt.sess.SetReasoning(ctx, effort, summary)
}

// teamModelApplier applies model changes to team runtimes (implements team.ModelApplier).
type teamModelApplier struct {
	runtimes []teamRoleRuntime
}

func (a *teamModelApplier) ApplyModel(ctx context.Context, model, target string) ([]string, error) {
	model = strings.TrimSpace(model)
	if model == "" {
		return nil, fmt.Errorf("model is required")
	}
	target = strings.TrimSpace(target)
	applied := make([]string, 0, len(a.runtimes))
	for _, rt := range a.runtimes {
		runID := strings.TrimSpace(rt.run.RunID)
		role := strings.TrimSpace(rt.role.Name)
		if target != "" && target != runID && target != role && target != "run:"+runID && target != "role:"+role {
			continue
		}
		if err := rt.sess.SetModel(ctx, model); err != nil {
			return nil, fmt.Errorf("set model for role %s: %w", rt.role.Name, err)
		}
		applied = append(applied, runID)
	}
	if target != "" && len(applied) == 0 {
		return nil, &protocol.ProtocolError{Code: protocol.CodeItemNotFound, Message: "target does not match any team role/run"}
	}
	return applied, nil
}

// teamRoleRunnerAdapter adapts a run func + cleanup to team.RoleRunner.
type teamRoleRunnerAdapter struct {
	run     func(context.Context) error
	cleanup func()
}

func (a *teamRoleRunnerAdapter) Run(ctx context.Context) error {
	if a.cleanup != nil {
		defer a.cleanup()
	}
	return a.run(ctx)
}

// teamRuntimeController implements pkgagent.RuntimeController for team daemon pause/resume.
type teamRuntimeController struct {
	runtimes       []teamRoleRuntime
	sessionService pkgsession.Service
	taskService    pkgtask.ActiveTaskCanceler
}

func (c *teamRuntimeController) PauseRun(runID string) error {
	runID = strings.TrimSpace(runID)
	for i := range c.runtimes {
		if strings.TrimSpace(c.runtimes[i].run.RunID) != runID {
			continue
		}
		loaded, err := c.sessionService.LoadRun(context.Background(), runID)
		if err != nil {
			return err
		}
		loaded.Status = types.RunStatusPaused
		loaded.FinishedAt = nil
		loaded.Error = nil
		if err := c.sessionService.SaveRun(context.Background(), loaded); err != nil {
			return err
		}
		c.runtimes[i].sess.SetPaused(true)
		return cancelActiveTasksForRun(context.Background(), c.taskService, runID, "run paused")
	}
	return &protocol.ProtocolError{Code: protocol.CodeItemNotFound, Message: "run not found"}
}

func (c *teamRuntimeController) ResumeRun(ctx context.Context, runID string) error {
	runID = strings.TrimSpace(runID)
	for i := range c.runtimes {
		if strings.TrimSpace(c.runtimes[i].run.RunID) != runID {
			continue
		}
		loaded, err := c.sessionService.LoadRun(ctx, runID)
		if err != nil {
			return err
		}
		loaded.Status = types.RunStatusRunning
		loaded.FinishedAt = nil
		loaded.Error = nil
		if err := c.sessionService.SaveRun(ctx, loaded); err != nil {
			return err
		}
		c.runtimes[i].sess.SetPaused(false)
		return nil
	}
	return &protocol.ProtocolError{Code: protocol.CodeItemNotFound, Message: "run not found"}
}

func (c *teamRuntimeController) StopRun(runID string) error {
	// Team daemon does not use StopRun from agent service; session stop uses custom logic.
	return nil
}

type teamRunRequest struct {
	ctx             context.Context
	cfg             config.Config
	prof            *profile.Profile
	profDir         string
	goal            string
	maxContextB     int
	poll            time.Duration
	resolved        RunChatOptions
	protocolEnabled bool
}

type TeamOrchestrator struct {
	req          teamRunRequest
	storeBuilder StoreBuilder
	runtimePhase RuntimeBuilder
	controlLoop  ControlLoop
}

type StoreBuilder struct {
	req *teamRunRequest
}

type RuntimeBuilder struct {
	req *teamRunRequest
}

type ControlLoop struct {
	req *teamRunRequest
}

func newTeamOrchestrator(req teamRunRequest) *TeamOrchestrator {
	o := &TeamOrchestrator{req: req}
	o.storeBuilder = StoreBuilder{req: &o.req}
	o.runtimePhase = RuntimeBuilder{req: &o.req}
	o.controlLoop = ControlLoop{req: &o.req}
	return o
}

func runAsTeam(ctx context.Context, cfg config.Config, prof *profile.Profile, profDir string, goal string, maxContextB int, poll time.Duration, resolved RunChatOptions, protocolEnabled bool) error {
	orch := newTeamOrchestrator(teamRunRequest{
		ctx:             ctx,
		cfg:             cfg,
		prof:            prof,
		profDir:         profDir,
		goal:            goal,
		maxContextB:     maxContextB,
		poll:            poll,
		resolved:        resolved,
		protocolEnabled: protocolEnabled,
	})
	return orch.Run()
}

func (o *TeamOrchestrator) Run() error {
	if o == nil {
		return fmt.Errorf("team orchestrator is nil")
	}
	if err := o.storeBuilder.Validate(); err != nil {
		return err
	}
	if err := o.runtimePhase.Prepare(); err != nil {
		return err
	}
	return o.controlLoop.Run()
}

func (b *StoreBuilder) Validate() error {
	if b == nil || b.req == nil {
		return fmt.Errorf("team store builder is not configured")
	}
	if b.req.prof == nil || b.req.prof.Team == nil {
		return fmt.Errorf("team profile is required")
	}
	return nil
}

func (b *RuntimeBuilder) Prepare() error {
	if b == nil || b.req == nil {
		return fmt.Errorf("team runtime builder is not configured")
	}
	return nil
}

func (c *ControlLoop) Run() error {
	if c == nil || c.req == nil {
		return fmt.Errorf("team control loop is not configured")
	}
	return runAsTeamInternal(
		c.req.ctx,
		c.req.cfg,
		c.req.prof,
		c.req.profDir,
		c.req.goal,
		c.req.maxContextB,
		c.req.poll,
		c.req.resolved,
		c.req.protocolEnabled,
	)
}

func runAsTeamInternal(ctx context.Context, cfg config.Config, prof *profile.Profile, profDir string, goal string, maxContextB int, poll time.Duration, resolved RunChatOptions, protocolEnabled bool) (err error) {
	if prof == nil || prof.Team == nil {
		return fmt.Errorf("team profile is required")
	}
	if maxContextB <= 0 {
		maxContextB = 8 * 1024
	}
	if poll <= 0 {
		poll = 2 * time.Second
	}
	teamID := "team-" + uuid.NewString()

	roleNames, coordinatorRole, err := team.ValidateTeamRoles(prof.Team.Roles)
	if err != nil {
		return err
	}
	workingRoles, reviewerRole, _, err := team.EnsureReviewerRole(prof.Team.Roles, coordinatorRole)
	if err != nil {
		return err
	}
	roleNames, coordinatorRole, err = team.ValidateTeamRoles(workingRoles)
	if err != nil {
		return err
	}
	prevManifest, err := loadExistingTeamManifest(cfg, teamID)
	if err != nil {
		return fmt.Errorf("load existing team manifest: %w", err)
	}
	teamModel := resolveTeamModel(prevManifest, prof.Team, resolved)
	if teamModel == "" {
		return fmt.Errorf("resolve team model: empty model")
	}
	log.Printf("daemon: TEAMS MODE - profile %q with %d roles", prof.ID, len(workingRoles))

	teamWorkspaceDir := fsutil.GetTeamWorkspaceDir(cfg.DataDir, teamID)
	if err := os.MkdirAll(teamWorkspaceDir, 0o755); err != nil {
		return fmt.Errorf("prepare team workspace: %w", err)
	}
	teamLogPath := fsutil.GetTeamLogPath(cfg.DataDir, teamID)
	logFile, err := os.OpenFile(teamLogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("open team daemon log file: %w", err)
	}
	prevLogWriter := log.Writer()
	log.SetOutput(io.MultiWriter(os.Stderr, logFile))
	defer func() {
		log.SetOutput(prevLogWriter)
		_ = logFile.Close()
	}()

	taskStore, err := state.NewSQLiteTaskStore(fsutil.GetSQLitePath(cfg.DataDir))
	if err != nil {
		return fmt.Errorf("create task store: %w", err)
	}
	workdirAbs, err := resolveWorkDir(resolved.WorkDir)
	if err != nil {
		return err
	}

	client, err := llm.NewClientFromEnv()
	if err != nil {
		return fmt.Errorf("create LLM client: %w", err)
	}
	llmClient := llm.NewRetryClient(client, llm.RetryConfig{
		MaxRetries:   3,
		InitialDelay: 250 * time.Millisecond,
		MaxDelay:     4 * time.Second,
		Multiplier:   2.0,
	})

	memStore, err := implstore.NewDiskMemoryStore(cfg)
	if err != nil {
		return fmt.Errorf("create memory store: %w", err)
	}
	constructorStore, err := implstore.NewSQLiteConstructorStore(cfg)
	if err != nil {
		return fmt.Errorf("create constructor store: %w", err)
	}
	sessionStore, err := implstore.NewSQLiteSessionStore(cfg)
	if err != nil {
		return fmt.Errorf("create session store: %w", err)
	}
	var runtimeLoopCancelMu sync.Mutex
	runtimeLoopCancel := map[string]context.CancelFunc{}
	teamSupervisor := &teamRuntimeSupervisor{
		cfg:             cfg,
		runLoopCancelMu: &runtimeLoopCancelMu,
		runLoopCancel:   runtimeLoopCancel,
	}
	sessionService := pkgsession.NewManager(cfg, sessionStore, teamSupervisor)
	teamSupervisor.sessionService = sessionService
	taskService := pkgtask.NewManager(taskStore, sessionService)
	teamSupervisor.taskCreator = taskService
	var memoryProvider agent.MemoryRecallProvider = &validatingMemoryProvider{
		inner: &textMemoryAdapter{store: memStore},
		store: memStore,
	}

	var notifier agent.Notifier
	if strings.TrimSpace(resolved.ResultWebhookURL) != "" {
		notifier = WebhookNotifier{URL: strings.TrimSpace(resolved.ResultWebhookURL)}
	}
	soulService := pkgsoul.NewService(cfg.DataDir)
	soulDoc, soulErr := soulService.Get(ctx)
	if soulErr == nil && strings.EqualFold(strings.TrimSpace(os.Getenv("AGEN8_SOUL_LOCKED")), "true") && !soulDoc.Locked {
		_, _ = soulService.SetLock(ctx, true, pkgsoul.ActorDaemon, "env AGEN8_SOUL_LOCKED=true")
	}
	soulContent := ""
	soulVersion := 0
	if soulErr == nil {
		soulContent = strings.TrimSpace(soulDoc.Content)
		soulVersion = soulDoc.Version
	}

	runtimes := make([]teamRoleRuntime, 0, len(workingRoles))
	setupComplete := false
	defer func() {
		if setupComplete || err == nil {
			return
		}
		for _, rt := range runtimes {
			if rt.cleanup != nil {
				rt.cleanup()
			}
		}
	}()
	roleDescriptions := make(map[string]string, len(prof.Team.Roles))
	for _, role := range workingRoles {
		roleDescriptions[strings.TrimSpace(role.Name)] = strings.TrimSpace(role.Description)
	}
	var coordinatorRun types.Run
	codeExecSecurityWarned := false
	for _, role := range prof.Team.Roles {
		role := role
		roleModel := resolveRoleModel(role, teamModel)
		if roleModel == "" {
			return fmt.Errorf("resolve model for role %s: empty model", strings.TrimSpace(role.Name))
		}
		roleGoal := strings.TrimSpace(goal)
		if roleGoal == "" {
			roleGoal = strings.TrimSpace(role.Description)
		}
		if roleGoal == "" {
			roleGoal = "team role worker"
		}
		metaSession, run, err := sessionService.Start(ctx, pkgsession.StartOptions{Goal: roleGoal, MaxBytesForContext: maxContextB})
		if err != nil {
			return fmt.Errorf("create session for role %s: %w", role.Name, err)
		}
		metaSession.System = true
		metaSession.Mode = "team"
		metaSession.TeamID = teamID
		metaSession.Profile = strings.TrimSpace(prof.ID)
		metaSession.SoulVersionSeen = soulVersion
		if strings.TrimSpace(roleModel) != "" {
			metaSession.ActiveModel = strings.TrimSpace(roleModel)
		}
		ensureSessionReasoningForModel(&metaSession, metaSession.ActiveModel, strings.TrimSpace(resolved.ReasoningEffort), strings.TrimSpace(resolved.ReasoningSummary))
		_ = sessionService.SaveSession(ctx, metaSession)
		if run.Runtime == nil {
			run.Runtime = &types.RunRuntimeConfig{}
		}
		run.Runtime.Profile = strings.TrimSpace(prof.ID)
		run.Runtime.Model = strings.TrimSpace(roleModel)
		run.Runtime.TeamID = teamID
		run.Runtime.Role = strings.TrimSpace(role.Name)
		run.Runtime.SoulVersionSeen = soulVersion
		_ = sessionService.SaveRun(ctx, run)
		roleReasoningEffort := strings.TrimSpace(metaSession.ReasoningEffort)
		roleReasoningSummary := strings.TrimSpace(metaSession.ReasoningSummary)

		traceStore := implstore.SQLiteTraceStore{Cfg: cfg, RunID: run.RunID}
		historyStore, err := implstore.NewSQLiteHistoryStore(cfg, run.SessionID)
		if err != nil {
			return fmt.Errorf("create history store for role %s: %w", role.Name, err)
		}

		orderedEmitter, err := newTeamOrderedEmitter(eventsvc.NewService(cfg), run.RunID, teamID, role.Name)
		if err != nil {
			return fmt.Errorf("create emitter for role %s: %w", role.Name, err)
		}
		if !codeExecSecurityWarned {
			emitCodeExecProvisioningSecurityWarning(ctx, cfg, func(ctx context.Context, ev events.Event) {
				_ = orderedEmitter.Emit(ctx, ev)
			})
			codeExecSecurityWarned = true
		}

		mountedWorkspaceDir := fsutil.GetTeamWorkspaceDir(cfg.DataDir, teamID)
		if err := os.MkdirAll(mountedWorkspaceDir, 0o755); err != nil {
			return fmt.Errorf("prepare mounted workspace for %s: %w", role.Name, err)
		}
		rt, err := runtime.Build(runtime.BuildConfig{
			Cfg:                cfg,
			Run:                run,
			Profile:            strings.TrimSpace(prof.ID),
			ProfileConfig:      prof,
			WorkdirAbs:         workdirAbs,
			SharedWorkspaceDir: mountedWorkspaceDir,
			Model:              roleModel,
			ReasoningEffort:    roleReasoningEffort,
			ReasoningSummary:   roleReasoningSummary,
			ApprovalsMode:      strings.TrimSpace(resolved.ApprovalsMode),
			HistoryStore:       historyStore,
			MemoryStore:        memStore,
			TraceStore:         traceStore,
			ConstructorStore:   constructorStore,
			Emit: func(ctx context.Context, ev events.Event) {
				if ev.Data == nil {
					ev.Data = map[string]string{}
				}
				ev.Data["teamId"] = teamID
				ev.Data["role"] = role.Name
				if err := orderedEmitter.Emit(ctx, ev); err != nil && !errorsIsDropped(err) {
					log.Printf("events: emit failed: %v", err)
				}
			},
			IncludeHistoryOps:     derefBool(resolved.IncludeHistoryOps, true),
			RecentHistoryPairs:    resolved.RecentHistoryPairs,
			MaxMemoryBytes:        resolved.MaxMemoryBytes,
			MaxTraceBytes:         resolved.MaxTraceBytes,
			PriceInPerMTokensUSD:  resolved.PriceInPerMTokensUSD,
			PriceOutPerMTokensUSD: resolved.PriceOutPerMTokensUSD,
			SoulVersionSeen:       soulVersion,
			PersistRun: func(r types.Run) error {
				return sessionService.SaveRun(context.Background(), r)
			},
			LoadSession: func(sessionID string) (types.Session, error) {
				return sessionService.LoadSession(context.Background(), sessionID)
			},
			SaveSession: func(session types.Session) error {
				return sessionService.SaveSession(context.Background(), session)
			},
		})
		if err != nil {
			orderedEmitter.Close()
			return fmt.Errorf("build runtime for role %s: %w", role.Name, err)
		}

		agentCfg := agent.DefaultConfig()
		agentCfg.Model = roleModel
		agentCfg.ReasoningEffort = roleReasoningEffort
		agentCfg.ReasoningSummary = roleReasoningSummary
		agentCfg.ApprovalsMode = strings.TrimSpace(resolved.ApprovalsMode)
		agentCfg.EnableWebSearch = resolved.WebSearchEnabled
		var promptSource agent.PromptSource = rt.Constructor
		if rt.Updater != nil {
			promptSource = rt.Updater
		}
		agentCfg.PromptSource = promptSource
		agentCfg.Hooks = agent.Hooks{
			OnLLMUsage: newCostUsageHook(
				cfg,
				run,
				roleModel,
				resolved.PriceInPerMTokensUSD,
				resolved.PriceOutPerMTokensUSD,
				sessionService,
				func() string {
					if sessionService == nil {
						return strings.TrimSpace(roleModel)
					}
					sess, err := sessionService.LoadSession(context.Background(), strings.TrimSpace(run.SessionID))
					if err != nil {
						return strings.TrimSpace(roleModel)
					}
					if active := strings.TrimSpace(sess.ActiveModel); active != "" {
						return active
					}
					return strings.TrimSpace(roleModel)
				},
				func(ctx context.Context, ev events.Event) {
					if ev.Data == nil {
						ev.Data = map[string]string{}
					}
					ev.Data["teamId"] = teamID
					ev.Data["role"] = role.Name
					if err := orderedEmitter.Emit(ctx, ev); err != nil && !errorsIsDropped(err) {
						log.Printf("events: emit failed: %v", err)
					}
				},
			),
			OnStep: func(step int, model, effectiveModel, summary string) {
				data := map[string]string{
					"step":  strconv.Itoa(step),
					"model": strings.TrimSpace(model),
					"role":  role.Name,
				}
				if em := strings.TrimSpace(effectiveModel); em != "" {
					data["effectiveModel"] = em
				}
				if s := strings.TrimSpace(summary); s != "" {
					data["reasoningSummary"] = s
				}
				if err := orderedEmitter.Emit(context.Background(), events.Event{
					Type:    "agent.step",
					Message: fmt.Sprintf("Step %d completed", step),
					Data:    data,
				}); err != nil && !errorsIsDropped(err) {
					log.Printf("events: emit failed: %v", err)
				}
			},
			OnCompaction: func(step int, beforeTokens, afterTokens int, serverSide bool) {
				ev := events.Event{
					Type:    "context.compacted",
					Message: fmt.Sprintf("Context compacted (%dk → %dk tokens)", beforeTokens/1000, afterTokens/1000),
					Data: map[string]string{
						"step":         strconv.Itoa(step),
						"beforeTokens": strconv.Itoa(beforeTokens),
						"afterTokens":  strconv.Itoa(afterTokens),
						"serverSide":   strconv.FormatBool(serverSide),
						"teamId":       teamID,
						"role":         role.Name,
					},
				}
				if err := orderedEmitter.Emit(context.Background(), ev); err != nil && !errorsIsDropped(err) {
					log.Printf("events: emit failed: %v", err)
				}
			},
			OnContextSize: func(step int, currentTokens, budgetTokens int) {
				ev := events.Event{
					Type:    "context.size",
					Message: fmt.Sprintf("Context: %dk/%dk tokens", currentTokens/1000, budgetTokens/1000),
					Data: map[string]string{
						"step":          strconv.Itoa(step),
						"currentTokens": strconv.Itoa(currentTokens),
						"budgetTokens":  strconv.Itoa(budgetTokens),
						"teamId":        teamID,
						"role":          role.Name,
					},
				}
				if err := orderedEmitter.Emit(context.Background(), ev); err != nil && !errorsIsDropped(err) {
					log.Printf("events: emit failed: %v", err)
				}
			},
		}
		runLLMClient := withRetryDiagnostics(llmClient, func(ctx context.Context, ev events.Event) {
			if ev.Data == nil {
				ev.Data = map[string]string{}
			}
			ev.Data["teamId"] = teamID
			ev.Data["role"] = role.Name
			if err := orderedEmitter.Emit(ctx, ev); err != nil && !errorsIsDropped(err) {
				log.Printf("events: emit failed: %v", err)
			}
		})

		registry, err := agent.DefaultHostToolRegistry()
		if err != nil {
			orderedEmitter.Close()
			_ = rt.Shutdown(context.Background())
			return fmt.Errorf("create host tool registry for role %s: %w", role.Name, err)
		}
		if err := registry.Register(&hosttools.TaskCreateTool{
			Store:           taskService,
			SessionID:       run.SessionID,
			RunID:           run.RunID,
			TeamID:          teamID,
			RoleName:        role.Name,
			IsCoordinator:   role.Coordinator,
			CoordinatorRole: coordinatorRole,
			ValidRoles:      roleNames,
		}); err != nil {
			orderedEmitter.Close()
			_ = rt.Shutdown(context.Background())
			return fmt.Errorf("register task_create for role %s: %w", role.Name, err)
		}
		if err := registry.Register(&hosttools.TaskReviewTool{
			Store:      taskService,
			SessionID:  run.SessionID,
			RunID:      run.RunID,
			Supervisor: teamSupervisor,
		}); err != nil {
			orderedEmitter.Close()
			_ = rt.Shutdown(context.Background())
			return fmt.Errorf("register task_review for role %s: %w", role.Name, err)
		}
		if err := registry.Register(&hosttools.SoulUpdateTool{Updater: soulService, Actor: pkgsoul.ActorAgent}); err != nil {
			orderedEmitter.Close()
			_ = rt.Shutdown(context.Background())
			return fmt.Errorf("register soul_update for role %s: %w", role.Name, err)
		}
		if err := registry.Register(&hosttools.ObsidianTool{ProjectRoot: workdirAbs}); err != nil {
			orderedEmitter.Close()
			_ = rt.Shutdown(context.Background())
			return fmt.Errorf("register obsidian for role %s: %w", role.Name, err)
		}
		roleAllowedTools, removedTools := sanitizeAllowedToolsForRole(role.AllowedTools, teamID, role.Coordinator)
		if len(removedTools) > 0 {
			msg := "Removed disallowed tool(s) for non-coordinator role"
			log.Printf("daemon: [%s] %s: %s: %s", role.Name, teamID, msg, strings.Join(removedTools, ","))
			if err := orderedEmitter.Emit(context.Background(), events.Event{
				Type:    "daemon.warning",
				Message: msg,
				Data: map[string]string{
					"teamId": teamID,
					"role":   role.Name,
					"tools":  strings.Join(removedTools, ","),
				},
			}); err != nil && !errorsIsDropped(err) {
				log.Printf("events: emit failed: %v", err)
			}
		}
		codeExecOnly := resolveCodeExecOnly(prof.CodeExecOnly, role.CodeExecOnly)
		resolvedCodeExecRequiredImports := resolveCodeExecRequiredImports(cfg.CodeExec.RequiredPackages)
		modelRegistry, bridgeRegistry, err := resolveToolRegistries(registry, roleAllowedTools, codeExecOnly)
		if err != nil {
			orderedEmitter.Close()
			_ = rt.Shutdown(context.Background())
			return fmt.Errorf("resolve tool registries for role %s: %w", role.Name, err)
		}
		if err := configureCodeExecRuntime(ctx, rt, cfg, modelRegistry, bridgeRegistry, resolvedCodeExecRequiredImports, codeExecOnly, func(ctx context.Context, ev events.Event) {
			_ = orderedEmitter.Emit(ctx, ev)
		}); err != nil {
			orderedEmitter.Close()
			_ = rt.Shutdown(context.Background())
			return fmt.Errorf("configure code_exec runtime for role %s: %w", role.Name, err)
		}
		promptToolSpec := agent.PromptToolSpecFromSources(modelRegistry, nil)
		if codeExecOnly {
			promptToolSpec = agent.PromptToolSpecForCodeExecOnly(modelRegistry, bridgeRegistry, nil)
		}
		agentCfg.SystemPrompt = prompts.DefaultTeamModeSystemPromptWithTools(promptToolSpec)
		agentCfg.HostToolRegistry = modelRegistry

		a, err := agent.NewAgent(runLLMClient, rt.Executor, agentCfg)
		if err != nil {
			orderedEmitter.Close()
			_ = rt.Shutdown(context.Background())
			return fmt.Errorf("create agent for role %s: %w", role.Name, err)
		}
		roleProfile := buildRoleRuntimeProfile(role)
		roleProfile.CodeExecOnly = codeExecOnly

		runConvStore, errConv := implstore.NewSQLiteRunConversationStoreFromConfig(cfg)
		if errConv != nil {
			orderedEmitter.Close()
			_ = rt.Shutdown(context.Background())
			return fmt.Errorf("run conversation store for role %s: %w", role.Name, errConv)
		}

		roleSession, err := session.New(session.Config{
			Agent:      a,
			Profile:    roleProfile,
			ProfileDir: profDir,
			ResolveProfile: func(ref string) (*profile.Profile, string, error) {
				return resolveProfileRef(cfg, strings.TrimSpace(ref))
			},
			TaskStore:            taskService,
			Events:               orderedEmitter,
			RunConversationStore: runConvStore,
			Memory:               memoryProvider,
			MemorySearchLimit:    3,
			Notifier:             notifier,
			SoulContent:          soulContent,
			SoulVersion:          soulVersion,
			PollInterval:         poll,
			MaxReadBytes:         256 * 1024,
			LeaseTTL:             2 * time.Minute,
			MaxRetries:           3,
			MaxPending:           50,
			SessionID:            run.SessionID,
			RunID:                run.RunID,
			TeamID:               teamID,
			RoleName:             role.Name,
			IsCoordinator:        role.Coordinator,
			CoordinatorRole:      coordinatorRole,
			ReviewerRole:         reviewerRole,
			TeamRoles:            roleNames,
			TeamRoleDescriptions: roleDescriptions,
			InstanceID:           run.RunID,
			Logf: func(format string, args ...any) {
				log.Printf("daemon [%s]: "+format, append([]any{role.Name}, args...)...)
			},
		})
		if err != nil {
			orderedEmitter.Close()
			_ = rt.Shutdown(context.Background())
			return fmt.Errorf("create session for role %s: %w", role.Name, err)
		}

		_ = orderedEmitter.Emit(context.Background(), events.Event{
			Type:    "run.conversation.enabled",
			Message: "Run conversation persistence enabled",
			Data:    map[string]string{"runId": run.RunID},
		})

		runtimes = append(runtimes, teamRoleRuntime{
			role: role,
			run:  run,
			sess: roleSession,
			cleanup: func() {
				orderedEmitter.Close()
				_ = rt.Shutdown(context.Background())
			},
		})
		kind := "worker"
		if role.Name == coordinatorRole {
			kind = "coordinator"
			coordinatorRun = run
		}
		log.Printf("daemon: [%s] %s -> %s", kind, role.Name, run.RunID)
	}
	roles := make([]team.RoleRecord, 0, len(runtimes))
	for _, rt := range runtimes {
		roles = append(roles, team.RoleRecord{
			RoleName:  strings.TrimSpace(rt.role.Name),
			RunID:     strings.TrimSpace(rt.run.RunID),
			SessionID: strings.TrimSpace(rt.run.SessionID),
		})
	}
	manifest := team.BuildManifest(teamID, prof.ID, coordinatorRole, coordinatorRun.RunID, teamModel, roles, time.Now().UTC().Format(time.RFC3339Nano))
	manifestStore := team.NewFileManifestStore(cfg)
	if err := manifestStore.Save(ctx, manifest); err != nil {
		return fmt.Errorf("write team manifest: %w", err)
	}
	stateMgr := team.NewStateManager(manifestStore, manifest)

	runCtx, stopSignals := signalNotifyContext(ctx)
	defer stopSignals()
	startCodeExecConfigReloader(runCtx, cfg, nil)

	trimmedGoal := strings.TrimSpace(goal)
	if trimmedGoal != "" && !strings.EqualFold(trimmedGoal, "autonomous agent") {
		if err := team.SeedCoordinatorTask(runCtx, taskService, coordinatorRun.SessionID, coordinatorRun.RunID, teamID, coordinatorRole, trimmedGoal); err != nil {
			return fmt.Errorf("seed coordinator task: %w", err)
		}
	}

	runIDs := make([]string, len(runtimes))
	for i := range runtimes {
		runIDs[i] = strings.TrimSpace(runtimes[i].run.RunID)
	}
	controllers := make([]team.RoleRunController, len(runtimes))
	for i := range runtimes {
		controllers[i] = &teamRoleRunControllerAdapter{rt: &runtimes[i]}
	}
	applier := &teamModelApplier{runtimes: runtimes}
	teamCtrl := team.NewController(team.ControllerConfig{
		SessionService:          sessionService,
		TaskStore:               taskService,
		TaskCanceler:            taskService,
		StateMgr:                stateMgr,
		Runtimes:                controllers,
		Applier:                 applier,
		RunStopper:              teamSupervisor,
		DefaultReasoningEffort:  strings.TrimSpace(resolved.ReasoningEffort),
		DefaultReasoningSummary: strings.TrimSpace(resolved.ReasoningSummary),
	})

	var serverWG sync.WaitGroup
	webhookAddr := strings.TrimSpace(resolved.WebhookAddr)
	if webhookAddr != "" {
		archiveDir := filepath.Join(fsutil.GetTeamDir(cfg.DataDir, teamID), "inbox", "archive")
		roleSet := make(map[string]struct{})
		for _, r := range roleNames {
			if s := strings.TrimSpace(r); s != "" {
				roleSet[s] = struct{}{}
			}
		}
		ingester := webhook.NewWebhookTaskIngester(taskService, webhook.NewDiskTaskArchiveWriter(archiveDir), nil)
		buildTask := func(ctx context.Context, payload []byte) (types.Task, error) {
			return webhook.BuildTeamTask(payload, teamID, coordinatorRole, coordinatorRun, roleSet)
		}
		srv := webhook.NewServer(webhook.ServerConfig{
			Addr:      webhookAddr,
			Ingester:  ingester,
			BuildTask: buildTask,
			Emit:      nil,
		})
		srv.Run(runCtx, &serverWG)
	}
	healthAddr := strings.TrimSpace(resolved.HealthAddr)
	if healthAddr != "" && healthAddr != webhookAddr {
		startHealthServer(runCtx, healthAddr, nil, &serverWG)
	}
	if protocolEnabled {
		srvCfg := newTeamRPCServerBaseConfig(cfg, coordinatorRun, taskService)
		srvCfg = buildTeamRPCServerConfig(
			srvCfg,
			cfg,
			resolved,
			coordinatorRun,
			taskService,
			sessionService,
			runtimes,
			teamCtrl,
			soulService,
		)
		srv := NewRPCServer(srvCfg)
		go func() {
			if err := srv.Serve(runCtx, os.Stdin, os.Stdout); err != nil && runCtx.Err() == nil {
				log.Printf("daemon: team protocol server stopped: %v", err)
			}
		}()
		tcpCfg := srvCfg
		tcpSrv := NewRPCServer(tcpCfg)
		if err := serveRPCOverTCP(runCtx, strings.TrimSpace(resolved.RPCListen), tcpSrv); err != nil {
			return err
		}
	}

	go team.RunModelChangeLoop(runCtx, taskService, stateMgr, applier)
	setupComplete = true

	log.Printf("daemon: protocol control-plane ready at %s — attach with: agen8", strings.TrimSpace(resolved.RPCListen))

	for i := range runtimes {
		rt := runtimes[i]
		go func() {
			ticker := time.NewTicker(2 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-runCtx.Done():
					return
				case <-ticker.C:
					loadedRun, err := sessionService.LoadRun(runCtx, rt.run.RunID)
					if err != nil {
						continue
					}
					rt.sess.SetPaused(strings.EqualFold(strings.TrimSpace(loadedRun.Status), types.RunStatusPaused))
				}
			}
		}()
	}

	runners := make([]team.RoleRunner, len(runtimes))
	for i := range runtimes {
		rt := &runtimes[i]
		runners[i] = &teamRoleRunnerAdapter{
			run:     rt.sess.Run,
			cleanup: rt.cleanup,
		}
	}
	registerCancel := func(runID string, cancel context.CancelFunc) {
		runtimeLoopCancelMu.Lock()
		runtimeLoopCancel[runID] = cancel
		runtimeLoopCancelMu.Unlock()
	}
	err = team.RunRoleLoops(runCtx, runners, runIDs, registerCancel)
	if runCtx.Err() != nil {
		err = nil
	}
	serverWG.Wait()
	return err
}

func newTeamRPCServerBaseConfig(cfg config.Config, coordinatorRun types.Run, taskService pkgtask.TaskServiceForRPC) RPCServerConfig {
	return RPCServerConfig{
		Cfg:            cfg,
		Run:            coordinatorRun,
		AllowAnyThread: true,
		TaskService:    taskService,
		EventsService:  eventsvc.NewService(cfg),
	}
}

func mapTeamErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, team.ErrThreadNotFound) {
		return &protocol.ProtocolError{Code: protocol.CodeThreadNotFound, Message: "thread not found"}
	}
	if errors.Is(err, team.ErrRunNotFound) {
		return &protocol.ProtocolError{Code: protocol.CodeItemNotFound, Message: "run not found"}
	}
	return err
}

func buildTeamRPCServerConfig(
	base RPCServerConfig,
	cfg config.Config,
	resolved RunChatOptions,
	coordinatorRun types.Run,
	taskService pkgtask.TaskServiceForRPC,
	sessionService pkgsession.Service,
	runtimes []teamRoleRuntime,
	teamCtrl *team.Controller,
	soulService pkgsoul.Service,
) RPCServerConfig {
	base.Session = sessionService
	base.SoulService = soulService

	base.ControlSetModel = func(ctx context.Context, threadID, target, model string) ([]string, error) {
		applied, err := teamCtrl.SetModel(ctx, threadID, target, model)
		return applied, mapTeamErr(err)
	}
	base.ControlSetReasoning = func(ctx context.Context, threadID, target, effort, summary string) ([]string, error) {
		applied, err := teamCtrl.SetReasoning(ctx, threadID, target, effort, summary)
		return applied, mapTeamErr(err)
	}
	base.ControlSetProfile = func(_ context.Context, _ string, _, _ string) ([]string, error) {
		return nil, &protocol.ProtocolError{Code: protocol.CodeInvalidState, Message: "control.setProfile is unavailable in team mode"}
	}
	teamController := &teamRuntimeController{
		runtimes:       runtimes,
		sessionService: sessionService,
		taskService:    taskService,
	}
	teamAgentManager := pkgagent.NewManager(sessionService, taskService, taskService)
	teamAgentManager.SetRuntimeController(teamController)
	base.AgentService = teamAgentManager
	base.SessionPause = func(ctx context.Context, threadID, sessionID string) ([]string, error) {
		affected, err := teamCtrl.PauseRuns(ctx, threadID, sessionID)
		return affected, mapTeamErr(err)
	}
	base.SessionResume = func(ctx context.Context, threadID, sessionID string) ([]string, error) {
		affected, err := teamCtrl.ResumeRuns(ctx, threadID, sessionID)
		return affected, mapTeamErr(err)
	}
	base.SessionStop = func(ctx context.Context, threadID, sessionID string) ([]string, error) {
		affected, err := teamCtrl.StopRuns(ctx, threadID, sessionID)
		return affected, mapTeamErr(err)
	}
	return base
}

func ptrNowUTC() *time.Time {
	now := time.Now().UTC()
	return &now
}

func newTeamOrderedEmitter(store events.StoreAppender, runID, teamID, roleName string) (*emit.OrderedEmitter[events.Event], error) {
	emitter := &events.Emitter{
		RunID: runID,
		Sink: events.StoreSink{
			Store: store,
		},
	}
	ordered := emit.NewOrderedEmitter[events.Event](emitter)
	if err := ordered.Emit(context.Background(), events.Event{
		Type:    "daemon.start",
		Message: "Team role started",
		Data: map[string]string{
			"runId":  runID,
			"teamId": teamID,
			"role":   roleName,
		},
	}); err != nil && !errorsIsDropped(err) {
		ordered.Close()
		return nil, err
	}
	return ordered, nil
}

func signalNotifyContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
}

func errorsIsDropped(err error) bool {
	return errors.Is(err, events.ErrDropped)
}

func resolveTeamModel(existing *team.Manifest, teamCfg *profile.TeamConfig, resolved RunChatOptions) string {
	if existing != nil {
		if model := strings.TrimSpace(existing.TeamModel); model != "" {
			return model
		}
	}
	if teamCfg != nil {
		if model := strings.TrimSpace(teamCfg.Model); model != "" {
			return model
		}
		for _, role := range teamCfg.Roles {
			if model := strings.TrimSpace(role.Model); model != "" {
				return model
			}
		}
	}
	return strings.TrimSpace(resolved.Model)
}

func resolveRoleModel(role profile.RoleConfig, teamModel string) string {
	if model := strings.TrimSpace(role.Model); model != "" {
		return model
	}
	return strings.TrimSpace(teamModel)
}

// teamRuntimeSupervisor adapts the team daemon's runtime management to the service interface
// and implements hosttools.ReviewSupervisor so team roles can retry and escalate via task_review.
type teamRuntimeSupervisor struct {
	cfg             config.Config
	sessionService  pkgsession.Service
	taskCreator     pkgtask.RetryEscalationCreator
	runLoopCancelMu *sync.Mutex
	runLoopCancel   map[string]context.CancelFunc
}

func (s *teamRuntimeSupervisor) StopRun(runID string) error {
	s.runLoopCancelMu.Lock()
	defer s.runLoopCancelMu.Unlock()
	if cancel, ok := s.runLoopCancel[runID]; ok {
		cancel()
	}
	return nil
}

func (s *teamRuntimeSupervisor) ResumeRun(ctx context.Context, runID string) error {
	// Team daemon main loop handles resumption based on DB status.
	// We rely on the services to have updated the DB status before calling this.
	return nil
}

func (s *teamRuntimeSupervisor) RetrySubagent(ctx context.Context, childRunID, feedback string) error {
	if s == nil || s.taskCreator == nil {
		return fmt.Errorf("task service not configured for retry")
	}
	return s.taskCreator.CreateRetryTask(ctx, childRunID, feedback)
}

func (s *teamRuntimeSupervisor) EscalateTask(ctx context.Context, callbackTaskID string, data hosttools.EscalationData) error {
	if s == nil || s.taskCreator == nil {
		return fmt.Errorf("task service not configured for escalation")
	}
	return team.EscalateToCoordinator(ctx, s.taskCreator, s.sessionService, s, callbackTaskID, data)
}
