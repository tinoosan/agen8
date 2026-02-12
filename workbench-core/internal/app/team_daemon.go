package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/google/uuid"
	implstore "github.com/tinoosan/workbench-core/internal/store"
	"github.com/tinoosan/workbench-core/pkg/agent"
	hosttools "github.com/tinoosan/workbench-core/pkg/agent/hosttools"
	"github.com/tinoosan/workbench-core/pkg/agent/session"
	"github.com/tinoosan/workbench-core/pkg/agent/state"
	"github.com/tinoosan/workbench-core/pkg/config"
	"github.com/tinoosan/workbench-core/pkg/emit"
	"github.com/tinoosan/workbench-core/pkg/events"
	"github.com/tinoosan/workbench-core/pkg/fsutil"
	"github.com/tinoosan/workbench-core/pkg/llm"
	"github.com/tinoosan/workbench-core/pkg/profile"
	"github.com/tinoosan/workbench-core/pkg/protocol"
	"github.com/tinoosan/workbench-core/pkg/runtime"
	"github.com/tinoosan/workbench-core/pkg/types"
	"golang.org/x/sync/errgroup"
)

type teamRoleRuntime struct {
	role    profile.RoleConfig
	run     types.Run
	sess    *session.Session
	cleanup func()
}

type teamManifest struct {
	TeamID          string           `json:"teamId"`
	ProfileID       string           `json:"profileId"`
	TeamModel       string           `json:"teamModel,omitempty"`
	ModelChange     *teamModelChange `json:"modelChange,omitempty"`
	CoordinatorRole string           `json:"coordinatorRole"`
	CoordinatorRun  string           `json:"coordinatorRunId"`
	Roles           []teamRoleRecord `json:"roles"`
	CreatedAt       string           `json:"createdAt"`
}

type teamModelChange struct {
	RequestedModel string `json:"requestedModel,omitempty"`
	Status         string `json:"status,omitempty"` // pending|applied|failed
	RequestedAt    string `json:"requestedAt,omitempty"`
	AppliedAt      string `json:"appliedAt,omitempty"`
	Reason         string `json:"reason,omitempty"`
	Error          string `json:"error,omitempty"`
}

type teamRoleRecord struct {
	RoleName  string `json:"roleName"`
	RunID     string `json:"runId"`
	SessionID string `json:"sessionId"`
}

func runAsTeam(ctx context.Context, cfg config.Config, prof *profile.Profile, profDir string, goal string, maxContextB int, poll time.Duration, resolved RunChatOptions, protocolEnabled bool) (err error) {
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

	roleNames, coordinatorRole, err := collectTeamRoles(prof.Team.Roles)
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
	log.Printf("daemon: TEAMS MODE - profile %q with %d roles", prof.ID, len(prof.Team.Roles))

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
	var memoryProvider agent.MemoryRecallProvider = &textMemoryAdapter{store: memStore}

	var notifier agent.Notifier
	if strings.TrimSpace(resolved.ResultWebhookURL) != "" {
		notifier = WebhookNotifier{URL: strings.TrimSpace(resolved.ResultWebhookURL)}
	}

	runtimes := make([]teamRoleRuntime, 0, len(prof.Team.Roles))
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
	for _, role := range prof.Team.Roles {
		roleDescriptions[strings.TrimSpace(role.Name)] = strings.TrimSpace(role.Description)
	}
	var coordinatorRun types.Run
	for _, role := range prof.Team.Roles {
		role := role
		roleGoal := strings.TrimSpace(goal)
		if roleGoal == "" {
			roleGoal = strings.TrimSpace(role.Description)
		}
		if roleGoal == "" {
			roleGoal = "team role worker"
		}
		metaSession, run, err := implstore.CreateSession(cfg, roleGoal, maxContextB)
		if err != nil {
			return fmt.Errorf("create session for role %s: %w", role.Name, err)
		}
		metaSession.System = true
		metaSession.Mode = "team"
		metaSession.TeamID = teamID
		metaSession.Profile = strings.TrimSpace(prof.ID)
		if strings.TrimSpace(teamModel) != "" {
			metaSession.ActiveModel = strings.TrimSpace(teamModel)
		}
		ensureSessionReasoningForModel(&metaSession, metaSession.ActiveModel, strings.TrimSpace(resolved.ReasoningEffort), strings.TrimSpace(resolved.ReasoningSummary))
		_ = implstore.SaveSession(cfg, metaSession)
		if run.Runtime == nil {
			run.Runtime = &types.RunRuntimeConfig{}
		}
		run.Runtime.Profile = strings.TrimSpace(prof.ID)
		run.Runtime.Model = strings.TrimSpace(teamModel)
		run.Runtime.TeamID = teamID
		run.Runtime.Role = strings.TrimSpace(role.Name)
		_ = implstore.SaveRun(cfg, run)
		roleReasoningEffort := strings.TrimSpace(metaSession.ReasoningEffort)
		roleReasoningSummary := strings.TrimSpace(metaSession.ReasoningSummary)

		traceStore := implstore.SQLiteTraceStore{Cfg: cfg, RunID: run.RunID}
		historyStore, err := implstore.NewSQLiteHistoryStore(cfg, run.SessionID)
		if err != nil {
			return fmt.Errorf("create history store for role %s: %w", role.Name, err)
		}

		orderedEmitter, err := newTeamOrderedEmitter(cfg, run.RunID, teamID, role.Name)
		if err != nil {
			return fmt.Errorf("create emitter for role %s: %w", role.Name, err)
		}

		rt, err := runtime.Build(runtime.BuildConfig{
			Cfg:                cfg,
			Run:                run,
			Profile:            strings.TrimSpace(prof.ID),
			WorkdirAbs:         workdirAbs,
			SharedWorkspaceDir: teamWorkspaceDir,
			Model:              teamModel,
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
			PersistRun: func(r types.Run) error {
				return implstore.SaveRun(cfg, r)
			},
			LoadSession: func(sessionID string) (types.Session, error) {
				return sessionStore.LoadSession(context.Background(), sessionID)
			},
			SaveSession: func(session types.Session) error {
				return sessionStore.SaveSession(context.Background(), session)
			},
		})
		if err != nil {
			orderedEmitter.Close()
			return fmt.Errorf("build runtime for role %s: %w", role.Name, err)
		}

		agentCfg := agent.DefaultConfig()
		agentCfg.Model = teamModel
		agentCfg.ReasoningEffort = roleReasoningEffort
		agentCfg.ReasoningSummary = roleReasoningSummary
		agentCfg.ApprovalsMode = strings.TrimSpace(resolved.ApprovalsMode)
		agentCfg.EnableWebSearch = resolved.WebSearchEnabled
		agentCfg.SystemPrompt = agent.DefaultAutonomousSystemPrompt()
		var promptSource agent.PromptSource = rt.Constructor
		if rt.Updater != nil {
			promptSource = rt.Updater
		}
		agentCfg.PromptSource = promptSource
		agentCfg.Hooks = agent.Hooks{
			OnLLMUsage: newCostUsageHook(
				cfg,
				run,
				teamModel,
				resolved.PriceInPerMTokensUSD,
				resolved.PriceOutPerMTokensUSD,
				sessionStore,
				func() string {
					if sessionStore == nil {
						return strings.TrimSpace(teamModel)
					}
					sess, err := sessionStore.LoadSession(context.Background(), strings.TrimSpace(run.SessionID))
					if err != nil {
						return strings.TrimSpace(teamModel)
					}
					if active := strings.TrimSpace(sess.ActiveModel); active != "" {
						return active
					}
					return strings.TrimSpace(teamModel)
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
			Store:           taskStore,
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
		agentCfg.HostToolRegistry = registry

		a, err := agent.NewAgent(runLLMClient, rt.Executor, agentCfg)
		if err != nil {
			orderedEmitter.Close()
			_ = rt.Shutdown(context.Background())
			return fmt.Errorf("create agent for role %s: %w", role.Name, err)
		}

		roleProfile := &profile.Profile{
			ID:          role.Name,
			Description: role.Description,
			Prompts:     role.Prompts,
			Skills:      append([]string(nil), role.Skills...),
			Heartbeat:   append([]profile.HeartbeatJob(nil), role.Heartbeat...),
		}
		roleSession, err := session.New(session.Config{
			Agent:      a,
			Profile:    roleProfile,
			ProfileDir: profDir,
			ResolveProfile: func(ref string) (*profile.Profile, string, error) {
				return resolveProfileRef(cfg, strings.TrimSpace(ref))
			},
			TaskStore:            taskStore,
			Events:               orderedEmitter,
			Memory:               memoryProvider,
			MemorySearchLimit:    3,
			Notifier:             notifier,
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
	manifest := buildTeamManifest(teamID, prof.ID, coordinatorRole, coordinatorRun.RunID, teamModel, runtimes)
	if err := writeTeamManifestFile(cfg, manifest); err != nil {
		return fmt.Errorf("write team manifest: %w", err)
	}
	stateMgr := newTeamStateManager(cfg, manifest)

	runCtx, stopSignals := signalNotifyContext(ctx)
	defer stopSignals()

	trimmedGoal := strings.TrimSpace(goal)
	if trimmedGoal != "" && !strings.EqualFold(trimmedGoal, "autonomous agent") {
		initialTask := types.Task{
			TaskID:       "task-" + uuid.NewString(),
			SessionID:    coordinatorRun.SessionID,
			RunID:        coordinatorRun.RunID,
			TeamID:       teamID,
			AssignedRole: coordinatorRole,
			CreatedBy:    "user",
			Goal:         trimmedGoal,
			Priority:     0,
			Status:       types.TaskStatusPending,
			CreatedAt:    ptrNowUTC(),
			Inputs:       map[string]any{},
			Metadata:     map[string]any{"source": "team.goal"},
		}
		if err := taskStore.CreateTask(runCtx, initialTask); err != nil {
			return fmt.Errorf("seed coordinator task: %w", err)
		}
	}

	var serverWG sync.WaitGroup
	webhookAddr := strings.TrimSpace(resolved.WebhookAddr)
	if webhookAddr != "" {
		startTeamWebhookServer(runCtx, webhookAddr, cfg, taskStore, teamID, coordinatorRole, coordinatorRun, roleNames, &serverWG)
	}
	healthAddr := strings.TrimSpace(resolved.HealthAddr)
	if healthAddr != "" && healthAddr != webhookAddr {
		startHealthServer(runCtx, healthAddr, nil, &serverWG)
	}
	if protocolEnabled {
		srvCfg := RPCServerConfig{
			Cfg:            cfg,
			Run:            coordinatorRun,
			AllowAnyThread: true,
			TaskStore:      taskStore,
			Session:        sessionStore,
			ControlSetModel: func(ctx context.Context, threadID, target, model string) ([]string, error) {
				if strings.TrimSpace(threadID) != strings.TrimSpace(coordinatorRun.SessionID) {
					return nil, &protocol.ProtocolError{Code: protocol.CodeThreadNotFound, Message: "thread not found"}
				}
				loadedSession, err := sessionStore.LoadSession(ctx, strings.TrimSpace(threadID))
				if err != nil || strings.TrimSpace(loadedSession.SessionID) != strings.TrimSpace(threadID) {
					return nil, &protocol.ProtocolError{Code: protocol.CodeThreadNotFound, Message: "thread not found"}
				}
				loadedSession.ActiveModel = strings.TrimSpace(model)
				ensureSessionReasoningForModel(&loadedSession, loadedSession.ActiveModel, strings.TrimSpace(resolved.ReasoningEffort), strings.TrimSpace(resolved.ReasoningSummary))
				if err := sessionStore.SaveSession(ctx, loadedSession); err != nil {
					return nil, err
				}
				applied, err := requestTeamModelChange(ctx, taskStore, runtimes, stateMgr, model, target, "rpc.control.setModel")
				if err != nil {
					return nil, err
				}
				for i := range runtimes {
					runID := strings.TrimSpace(runtimes[i].run.RunID)
					if runID == "" {
						continue
					}
					if len(applied) != 0 {
						match := false
						for _, id := range applied {
							if strings.TrimSpace(id) == runID {
								match = true
								break
							}
						}
						if !match {
							continue
						}
					}
					_ = runtimes[i].sess.SetReasoning(ctx, loadedSession.ReasoningEffort, loadedSession.ReasoningSummary)
				}
				return applied, nil
			},
			ControlSetReasoning: func(ctx context.Context, threadID, target, effort, summary string) ([]string, error) {
				threadID = strings.TrimSpace(threadID)
				if threadID != strings.TrimSpace(coordinatorRun.SessionID) {
					return nil, &protocol.ProtocolError{Code: protocol.CodeThreadNotFound, Message: "thread not found"}
				}
				loadedSession, err := sessionStore.LoadSession(ctx, threadID)
				if err != nil || strings.TrimSpace(loadedSession.SessionID) != threadID {
					return nil, &protocol.ProtocolError{Code: protocol.CodeThreadNotFound, Message: "thread not found"}
				}
				effort = strings.ToLower(strings.TrimSpace(effort))
				summary = normalizeReasoningSummaryValue(summary)
				storeSessionReasoningPreference(&loadedSession, strings.TrimSpace(loadedSession.ActiveModel), effort, summary)
				if err := sessionStore.SaveSession(ctx, loadedSession); err != nil {
					return nil, err
				}

				target = strings.TrimSpace(target)
				applied := make([]string, 0, len(runtimes))
				for i := range runtimes {
					runID := strings.TrimSpace(runtimes[i].run.RunID)
					if runID == "" {
						continue
					}
					if target != "" && target != threadID && target != runID {
						continue
					}
					if err := runtimes[i].sess.SetReasoning(ctx, effort, summary); err != nil {
						return applied, err
					}
					applied = append(applied, runID)
				}
				if target != "" && target != threadID && len(applied) == 0 {
					return nil, &protocol.ProtocolError{Code: protocol.CodeItemNotFound, Message: "run not found"}
				}
				return applied, nil
			},
			ControlSetProfile: func(_ context.Context, _ string, _, _ string) ([]string, error) {
				return nil, &protocol.ProtocolError{Code: protocol.CodeInvalidState, Message: "control.setProfile is unavailable in team mode"}
			},
			AgentPause: func(_ context.Context, threadID, runID string) error {
				if strings.TrimSpace(threadID) != strings.TrimSpace(coordinatorRun.SessionID) {
					return &protocol.ProtocolError{Code: protocol.CodeThreadNotFound, Message: "thread not found"}
				}
				runID = strings.TrimSpace(runID)
				if runID == "" {
					return &protocol.ProtocolError{Code: protocol.CodeInvalidParams, Message: "runId is required"}
				}
				for i := range runtimes {
					if strings.TrimSpace(runtimes[i].run.RunID) != runID {
						continue
					}
					loaded, err := implstore.LoadRun(cfg, runID)
					if err != nil {
						return err
					}
					loaded.Status = types.RunStatusPaused
					loaded.FinishedAt = nil
					loaded.Error = nil
					if err := implstore.SaveRun(cfg, loaded); err != nil {
						return err
					}
					runtimes[i].sess.SetPaused(true)
					return nil
				}
				return &protocol.ProtocolError{Code: protocol.CodeItemNotFound, Message: "run not found"}
			},
			AgentResume: func(_ context.Context, threadID, runID string) error {
				if strings.TrimSpace(threadID) != strings.TrimSpace(coordinatorRun.SessionID) {
					return &protocol.ProtocolError{Code: protocol.CodeThreadNotFound, Message: "thread not found"}
				}
				runID = strings.TrimSpace(runID)
				if runID == "" {
					return &protocol.ProtocolError{Code: protocol.CodeInvalidParams, Message: "runId is required"}
				}
				for i := range runtimes {
					if strings.TrimSpace(runtimes[i].run.RunID) != runID {
						continue
					}
					loaded, err := implstore.LoadRun(cfg, runID)
					if err != nil {
						return err
					}
					loaded.Status = types.RunStatusRunning
					loaded.FinishedAt = nil
					loaded.Error = nil
					if err := implstore.SaveRun(cfg, loaded); err != nil {
						return err
					}
					runtimes[i].sess.SetPaused(false)
					return nil
				}
				return &protocol.ProtocolError{Code: protocol.CodeItemNotFound, Message: "run not found"}
			},
			SessionPause: func(_ context.Context, threadID, sessionID string) ([]string, error) {
				if strings.TrimSpace(threadID) != strings.TrimSpace(coordinatorRun.SessionID) {
					return nil, &protocol.ProtocolError{Code: protocol.CodeThreadNotFound, Message: "thread not found"}
				}
				sessionID = strings.TrimSpace(sessionID)
				if sessionID != "" && sessionID != strings.TrimSpace(coordinatorRun.SessionID) {
					return nil, &protocol.ProtocolError{Code: protocol.CodeThreadNotFound, Message: "thread not found"}
				}
				affected := make([]string, 0, len(runtimes))
				for i := range runtimes {
					runID := strings.TrimSpace(runtimes[i].run.RunID)
					if runID == "" {
						continue
					}
					loaded, err := implstore.LoadRun(cfg, runID)
					if err != nil {
						return affected, err
					}
					loaded.Status = types.RunStatusPaused
					loaded.FinishedAt = nil
					loaded.Error = nil
					if err := implstore.SaveRun(cfg, loaded); err != nil {
						return affected, err
					}
					runtimes[i].sess.SetPaused(true)
					affected = append(affected, runID)
				}
				return affected, nil
			},
			SessionResume: func(_ context.Context, threadID, sessionID string) ([]string, error) {
				if strings.TrimSpace(threadID) != strings.TrimSpace(coordinatorRun.SessionID) {
					return nil, &protocol.ProtocolError{Code: protocol.CodeThreadNotFound, Message: "thread not found"}
				}
				sessionID = strings.TrimSpace(sessionID)
				if sessionID != "" && sessionID != strings.TrimSpace(coordinatorRun.SessionID) {
					return nil, &protocol.ProtocolError{Code: protocol.CodeThreadNotFound, Message: "thread not found"}
				}
				affected := make([]string, 0, len(runtimes))
				for i := range runtimes {
					runID := strings.TrimSpace(runtimes[i].run.RunID)
					if runID == "" {
						continue
					}
					loaded, err := implstore.LoadRun(cfg, runID)
					if err != nil {
						return affected, err
					}
					loaded.Status = types.RunStatusRunning
					loaded.FinishedAt = nil
					loaded.Error = nil
					if err := implstore.SaveRun(cfg, loaded); err != nil {
						return affected, err
					}
					runtimes[i].sess.SetPaused(false)
					affected = append(affected, runID)
				}
				return affected, nil
			},
		}
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

	go runTeamControlLoop(runCtx, taskStore, runtimes, stateMgr)
	setupComplete = true

	log.Printf("daemon: protocol control-plane ready at %s — attach with: workbench", strings.TrimSpace(resolved.RPCListen))

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
					loadedRun, err := implstore.LoadRun(cfg, rt.run.RunID)
					if err != nil {
						continue
					}
					rt.sess.SetPaused(strings.EqualFold(strings.TrimSpace(loadedRun.Status), types.RunStatusPaused))
				}
			}
		}()
	}

	g, gctx := errgroup.WithContext(runCtx)
	for _, rt := range runtimes {
		rt := rt
		g.Go(func() error {
			defer func() {
				if rt.cleanup != nil {
					rt.cleanup()
				}
			}()
			backoff := 2 * time.Second
			for {
				err := rt.sess.Run(gctx)
				if gctx.Err() != nil {
					return nil
				}
				errMsg := "unknown error"
				if err != nil {
					errMsg = err.Error()
				}
				log.Printf("daemon [%s]: runner exited unexpectedly; restarting in %s: %s", rt.role.Name, backoff, errMsg)
				time.Sleep(backoff)
				if backoff < 60*time.Second {
					backoff *= 2
					if backoff > 60*time.Second {
						backoff = 60 * time.Second
					}
				}
			}
		})
	}
	err = g.Wait()
	if runCtx.Err() != nil {
		err = nil
	}
	serverWG.Wait()
	return err
}

func collectTeamRoles(roles []profile.RoleConfig) ([]string, string, error) {
	out := make([]string, 0, len(roles))
	seen := map[string]struct{}{}
	coordinatorRole := ""
	for _, role := range roles {
		name := strings.TrimSpace(role.Name)
		if name == "" {
			return nil, "", fmt.Errorf("team role name is required")
		}
		if _, ok := seen[name]; ok {
			return nil, "", fmt.Errorf("duplicate team role name %q", name)
		}
		seen[name] = struct{}{}
		out = append(out, name)
		if role.Coordinator {
			coordinatorRole = name
		}
	}
	if coordinatorRole == "" {
		return nil, "", fmt.Errorf("team profile must define one coordinator role")
	}
	return out, coordinatorRole, nil
}

func ptrNowUTC() *time.Time {
	now := time.Now().UTC()
	return &now
}

func newTeamOrderedEmitter(cfg config.Config, runID, teamID, roleName string) (*emit.OrderedEmitter[events.Event], error) {
	emitter := &events.Emitter{
		RunID: runID,
		Sink: events.StoreSink{
			Store: daemonEventAppender{cfg: cfg},
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

func startTeamWebhookServer(ctx context.Context, addr string, cfg config.Config, taskStore state.TaskStore, teamID, coordinatorRole string, coordinatorRun types.Run, validRoles []string, wg *sync.WaitGroup) {
	roleSet := map[string]struct{}{}
	for _, role := range validRoles {
		role = strings.TrimSpace(role)
		if role == "" {
			continue
		}
		roleSet[role] = struct{}{}
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	mux.HandleFunc("/task", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		defer r.Body.Close()
		raw, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			http.Error(w, "read error: "+err.Error(), http.StatusBadRequest)
			return
		}
		var payload struct {
			TaskID       string         `json:"taskId"`
			AssignedRole string         `json:"assignedRole,omitempty"`
			Goal         string         `json:"goal"`
			Priority     int            `json:"priority,omitempty"`
			Inputs       map[string]any `json:"inputs,omitempty"`
			Metadata     map[string]any `json:"metadata,omitempty"`
		}
		if err := json.Unmarshal(raw, &payload); err != nil {
			http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}
		goal := strings.TrimSpace(payload.Goal)
		if goal == "" {
			http.Error(w, "goal is required", http.StatusBadRequest)
			return
		}
		taskID := strings.TrimSpace(payload.TaskID)
		if taskID == "" {
			taskID = "task-" + uuid.NewString()
		} else if normalized, _ := types.NormalizeTaskID(taskID); strings.TrimSpace(normalized) != "" {
			taskID = normalized
		}
		assignedRole := strings.TrimSpace(payload.AssignedRole)
		if assignedRole == "" {
			assignedRole = coordinatorRole
		}
		if len(roleSet) != 0 {
			if _, ok := roleSet[assignedRole]; !ok {
				http.Error(w, "assignedRole is not a valid team role", http.StatusBadRequest)
				return
			}
		}
		now := time.Now().UTC()
		task := types.Task{
			TaskID:       taskID,
			SessionID:    coordinatorRun.SessionID,
			RunID:        coordinatorRun.RunID,
			TeamID:       teamID,
			AssignedRole: assignedRole,
			CreatedBy:    "webhook",
			Goal:         goal,
			Priority:     payload.Priority,
			Status:       types.TaskStatusPending,
			CreatedAt:    &now,
			Inputs:       payload.Inputs,
			Metadata:     payload.Metadata,
		}
		if task.Inputs == nil {
			task.Inputs = map[string]any{}
		}
		if task.Metadata == nil {
			task.Metadata = map[string]any{}
		}
		task.Metadata["source"] = "webhook"

		if err := taskStore.CreateTask(ctx, task); err != nil {
			http.Error(w, "create task error: "+err.Error(), http.StatusInternalServerError)
			return
		}
		archiveDir := filepath.Join(fsutil.GetTeamDir(cfg.DataDir, teamID), "inbox", "archive")
		_ = os.MkdirAll(archiveDir, 0o755)
		if b, err := json.MarshalIndent(task, "", "  "); err == nil {
			_ = os.WriteFile(filepath.Join(archiveDir, taskID+".json"), b, 0o644)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"taskId": taskID, "status": "queued"})
	})

	srv := &http.Server{Addr: addr, Handler: mux}
	if wg != nil {
		wg.Add(2)
	}
	go func() {
		if wg != nil {
			defer wg.Done()
		}
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()
	go func() {
		if wg != nil {
			defer wg.Done()
		}
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("daemon: team webhook server error: %v", err)
		}
	}()
}

func signalNotifyContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
}

func errorsIsDropped(err error) bool {
	return errors.Is(err, events.ErrDropped)
}

func buildTeamManifest(teamID, profileID, coordinatorRole, coordinatorRunID, teamModel string, runtimes []teamRoleRuntime) teamManifest {
	records := make([]teamRoleRecord, 0, len(runtimes))
	for _, rt := range runtimes {
		records = append(records, teamRoleRecord{
			RoleName:  strings.TrimSpace(rt.role.Name),
			RunID:     strings.TrimSpace(rt.run.RunID),
			SessionID: strings.TrimSpace(rt.run.SessionID),
		})
	}
	return teamManifest{
		TeamID:          strings.TrimSpace(teamID),
		ProfileID:       strings.TrimSpace(profileID),
		TeamModel:       strings.TrimSpace(teamModel),
		CoordinatorRole: strings.TrimSpace(coordinatorRole),
		CoordinatorRun:  strings.TrimSpace(coordinatorRunID),
		Roles:           records,
		CreatedAt:       time.Now().UTC().Format(time.RFC3339Nano),
	}
}

func resolveTeamModel(existing *teamManifest, teamCfg *profile.TeamConfig, resolved RunChatOptions) string {
	if existing != nil {
		if model := strings.TrimSpace(existing.TeamModel); model != "" {
			return model
		}
	}
	if teamCfg != nil {
		if model := strings.TrimSpace(teamCfg.Model); model != "" {
			return model
		}
	}
	return strings.TrimSpace(resolved.Model)
}

func teamIsIdle(ctx context.Context, store state.TaskStore, teamID string) bool {
	active, err := store.CountTasks(ctx, state.TaskFilter{
		TeamID: strings.TrimSpace(teamID),
		Status: []types.TaskStatus{types.TaskStatusPending, types.TaskStatusActive},
	})
	return err == nil && active == 0
}

func applyTeamModel(ctx context.Context, runtimes []teamRoleRuntime, model string, target string) ([]string, error) {
	model = strings.TrimSpace(model)
	if model == "" {
		return nil, fmt.Errorf("model is required")
	}
	target = strings.TrimSpace(target)
	applied := make([]string, 0, len(runtimes))
	for _, rt := range runtimes {
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
		return nil, &protocol.ProtocolError{Code: protocol.CodeInvalidParams, Message: "target does not match any team role/run"}
	}
	return applied, nil
}

func requestTeamModelChange(ctx context.Context, taskStore state.TaskStore, runtimes []teamRoleRuntime, stateMgr *teamStateManager, model string, target string, reason string) ([]string, error) {
	model = strings.TrimSpace(model)
	if model == "" {
		return nil, fmt.Errorf("model is required")
	}
	target = strings.TrimSpace(target)
	if target != "" {
		appliedTo, err := applyTeamModel(ctx, runtimes, model, target)
		if err != nil {
			_ = stateMgr.markModelFailed(model, err)
			return nil, err
		}
		return appliedTo, stateMgr.markModelApplied(model)
	}
	if teamIsIdle(ctx, taskStore, stateMgr.teamID) {
		appliedTo, err := applyTeamModel(ctx, runtimes, model, "")
		if err != nil {
			_ = stateMgr.markModelFailed(model, err)
			return nil, err
		}
		return appliedTo, stateMgr.markModelApplied(model)
	}
	if err := stateMgr.queueModelChange(model, reason); err != nil {
		return nil, err
	}
	return []string{}, nil
}

func runTeamControlLoop(ctx context.Context, taskStore state.TaskStore, runtimes []teamRoleRuntime, stateMgr *teamStateManager) {
	if stateMgr == nil {
		return
	}
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			manifest := stateMgr.manifestSnapshot()
			if manifest.ModelChange != nil &&
				strings.EqualFold(strings.TrimSpace(manifest.ModelChange.Status), "pending") &&
				teamIsIdle(ctx, taskStore, manifest.TeamID) {
				model := strings.TrimSpace(manifest.ModelChange.RequestedModel)
				if model == "" {
					continue
				}
				if _, err := applyTeamModel(ctx, runtimes, model, ""); err != nil {
					_ = stateMgr.markModelFailed(model, err)
					log.Printf("daemon: apply queued team model failed: %v", err)
					continue
				}
				_ = stateMgr.markModelApplied(model)
				log.Printf("daemon: applied queued team model: %s", model)
			}
		}
	}
}
