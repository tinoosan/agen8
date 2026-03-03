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
	"strings"
	"sync"
	"syscall"
	"time"

	implstore "github.com/tinoosan/agen8/internal/store"
	"github.com/tinoosan/agen8/internal/webhook"
	"github.com/tinoosan/agen8/pkg/agent"
	"github.com/tinoosan/agen8/pkg/agent/state"
	"github.com/tinoosan/agen8/pkg/config"
	"github.com/tinoosan/agen8/pkg/emit"
	"github.com/tinoosan/agen8/pkg/events"
	"github.com/tinoosan/agen8/pkg/fsutil"
	"github.com/tinoosan/agen8/pkg/llm"
	"github.com/tinoosan/agen8/pkg/profile"
	"github.com/tinoosan/agen8/pkg/protocol"
	pkgagent "github.com/tinoosan/agen8/pkg/services/agent"
	eventsvc "github.com/tinoosan/agen8/pkg/services/events"
	pkgsession "github.com/tinoosan/agen8/pkg/services/session"
	pkgsoul "github.com/tinoosan/agen8/pkg/services/soul"
	pkgtask "github.com/tinoosan/agen8/pkg/services/task"
	"github.com/tinoosan/agen8/pkg/services/team"
	"github.com/tinoosan/agen8/pkg/types"
)

type webhookRoutingContext struct {
	run             types.Run
	teamID          string
	coordinatorRole string
	validRoles      map[string]struct{}
}

func runAsTeam(ctx context.Context, cfg config.Config, prof *profile.Profile, profDir string, goal string, maxContextB int, poll time.Duration, resolved RunChatOptions, protocolEnabled bool) error {
	if prof == nil {
		return fmt.Errorf("profile is required")
	}
	return runAsTeamInternal(
		ctx,
		cfg,
		prof,
		profDir,
		goal,
		maxContextB,
		poll,
		resolved,
		protocolEnabled,
	)
}

func runAsTeamInternal(ctx context.Context, cfg config.Config, prof *profile.Profile, profDir string, goal string, maxContextB int, poll time.Duration, resolved RunChatOptions, protocolEnabled bool) (err error) {
	if prof == nil {
		return fmt.Errorf("profile is required")
	}
	if maxContextB <= 0 {
		maxContextB = 8 * 1024
	}
	if poll <= 0 {
		poll = 2 * time.Second
	}

	daemonLogPath := fsutil.GetTeamLogPath(cfg.DataDir, "daemon")
	if err := os.MkdirAll(filepath.Dir(daemonLogPath), 0o755); err != nil {
		return fmt.Errorf("prepare daemon log dir: %w", err)
	}
	logFile, err := os.OpenFile(daemonLogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("open daemon log file: %w", err)
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

	realEventsStore := eventsvc.NewService(cfg)
	eventBroadcaster, eventBroadcastCh := NewEventBroadcaster()
	eventsWithBroadcast := NewBroadcastingEventsAppender(realEventsStore, eventBroadcastCh)

	supervisor := newRuntimeSupervisor(runtimeSupervisorConfig{
		Cfg:              cfg,
		Resolved:         resolved,
		PollInterval:     poll,
		TaskService:      nil, // set after taskService is created
		SessionService:   nil, // set after sessionService is created
		EventsStore:      eventsWithBroadcast,
		MemoryStore:      memStore,
		ConstructorStore: constructorStore,
		LLMClient:        llmClient,
		Notifier:         nil,
		WorkdirAbs:       workdirAbs,
		DefaultProfile:   prof,
		SoulService:      nil,
	})
	var notifier agent.Notifier
	if strings.TrimSpace(resolved.ResultWebhookURL) != "" {
		notifier = WebhookNotifier{URL: strings.TrimSpace(resolved.ResultWebhookURL)}
	}
	soulService := pkgsoul.NewService(cfg.DataDir)
	soulDoc, soulErr := soulService.Get(ctx)
	if soulErr == nil && strings.EqualFold(strings.TrimSpace(os.Getenv("AGEN8_SOUL_LOCKED")), "true") && !soulDoc.Locked {
		_, _ = soulService.SetLock(ctx, true, pkgsoul.ActorDaemon, "env AGEN8_SOUL_LOCKED=true")
	}
	supervisor.soulService = soulService
	supervisor.notifier = notifier

	sessionService := pkgsession.NewManager(cfg, sessionStore, supervisor)
	supervisor.sessionService = sessionService
	taskService := pkgtask.NewManager(taskStore, sessionService)
	taskService.SetRoutingOracle(pkgtask.NewRoutingOracle())
	taskService.SetEventsStore(eventsWithBroadcast)
	supervisor.taskService = taskService

	runCtx, stopSignals := signalNotifyContext(ctx)
	defer stopSignals()
	startCodeExecConfigReloader(runCtx, cfg, nil)

	go supervisor.Run(runCtx)
	go eventBroadcaster.Run(runCtx)

	agentManager := pkgagent.NewManager(sessionService, taskService, taskService)
	agentManager.SetRuntimeController(supervisor)

	var serverWG sync.WaitGroup
	healthAddr := strings.TrimSpace(resolved.HealthAddr)
	if healthAddr != "" {
		startHealthServer(runCtx, healthAddr, nil, &serverWG)
	}
	webhookAddr := strings.TrimSpace(resolved.WebhookAddr)
	if webhookAddr != "" {
		webhookServer := webhook.NewServer(webhook.ServerConfig{
			Addr:     webhookAddr,
			Ingester: webhook.NewWebhookTaskIngester(taskService, nil, nil),
			BuildTask: func(ctx context.Context, payload []byte) (types.Task, error) {
				route, err := resolveWebhookRoutingContext(ctx, sessionService, prof)
				if err != nil {
					return types.Task{}, err
				}
				if strings.TrimSpace(route.teamID) != "" {
					return webhook.BuildTeamTask(payload, route.teamID, route.coordinatorRole, route.run, route.validRoles)
				}
				return webhook.BuildStandaloneTask(payload, route.run)
			},
		})
		webhookServer.Run(runCtx, &serverWG)
	}
	if protocolEnabled {
		baseCfg := RPCServerConfig{
			Cfg:            cfg,
			Run:            types.Run{},
			AllowAnyThread: true,
			TaskService:    taskService,
			Session:        sessionService,
			AgentService:   agentManager,
			RuntimeState:   supervisor,
			SoulService:    soulService,
			EventsService:  eventsWithBroadcast,
			SessionPause: func(ctx context.Context, _, sessionID string) ([]string, error) {
				return supervisor.PauseSession(ctx, sessionID)
			},
			SessionResume: func(ctx context.Context, _, sessionID string) ([]string, error) {
				return supervisor.ResumeSession(ctx, sessionID)
			},
			SessionStop: func(ctx context.Context, _, sessionID string) ([]string, error) {
				return supervisor.StopSession(ctx, sessionID)
			},
			ControlSetModel: func(ctx context.Context, threadID, target, model string) ([]string, error) {
				return supervisor.ApplySessionModel(ctx, threadID, target, model)
			},
			ControlSetReasoning: func(ctx context.Context, threadID, target, effort, summary string) ([]string, error) {
				return supervisor.ApplySessionReasoning(ctx, threadID, target, effort, summary)
			},
			ControlSetProfile: func(_ context.Context, _ string, _, _ string) ([]string, error) {
				return nil, &protocol.ProtocolError{Code: protocol.CodeInvalidState, Message: "control.setProfile is unavailable in team mode"}
			},
		}
		srv := NewRPCServer(baseCfg)
		go func() {
			if err := srv.Serve(runCtx, os.Stdin, os.Stdout); err != nil && runCtx.Err() == nil {
				log.Printf("daemon: team protocol server stopped: %v", err)
			}
		}()
		if err := serveRPCOverTCPWithBroadcaster(runCtx, strings.TrimSpace(resolved.RPCListen), eventBroadcaster, func(notifyCh <-chan protocol.Message) RPCServerConfig {
			c := baseCfg
			c.NotifyCh = notifyCh
			return c
		}); err != nil {
			return err
		}
	}

	log.Printf("daemon: protocol control-plane ready at %s — attach with: agen8", strings.TrimSpace(resolved.RPCListen))

	<-runCtx.Done()
	serverWG.Wait()
	return nil
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

// resolveTeamModelFromProfile resolves the team model from manifest, profile, or resolved options.
// Supports both team (prof.Team != nil) and standalone (prof.Team == nil) profiles.
func resolveTeamModelFromProfile(existing *team.Manifest, prof *profile.Profile, resolved RunChatOptions) string {
	if existing != nil {
		if model := strings.TrimSpace(existing.TeamModel); model != "" {
			return model
		}
	}
	if m := prof.TeamModelForSession(); m != "" {
		return m
	}
	return strings.TrimSpace(resolved.Model)
}

func resolveRoleModel(role profile.RoleConfig, teamModel string) string {
	if model := strings.TrimSpace(role.Model); model != "" {
		return model
	}
	return strings.TrimSpace(teamModel)
}

func resolveWebhookRoutingContext(ctx context.Context, sessionService pkgsession.Service, prof *profile.Profile) (webhookRoutingContext, error) {
	if sessionService == nil {
		return webhookRoutingContext{}, fmt.Errorf("session service is not configured")
	}
	runs, err := sessionService.ListRunsByStatus(ctx, []string{types.RunStatusRunning, types.RunStatusPaused})
	if err != nil {
		return webhookRoutingContext{}, fmt.Errorf("list active runs: %w", err)
	}
	roleSet, coordinatorRole, _ := profileWebhookRoleHints(prof)
	return resolveWebhookRoutingContextFromRuns(ctx, runs, roleSet, coordinatorRole, sessionService)
}

func resolveWebhookRoutingContextFromRuns(ctx context.Context, runs []types.Run, roleSet map[string]struct{}, coordinatorRole string, sessionLoader interface {
	LoadSession(context.Context, string) (types.Session, error)
}) (webhookRoutingContext, error) {
	var rootRuns []types.Run
	for _, run := range runs {
		if strings.TrimSpace(run.ParentRunID) != "" {
			continue
		}
		rootRuns = append(rootRuns, run)
	}
	if len(rootRuns) == 0 {
		return webhookRoutingContext{}, fmt.Errorf("no active root run available for webhook routing")
	}

	selected := pickLatestRun(rootRuns, func(run types.Run) bool {
		if run.Runtime == nil {
			return false
		}
		if strings.TrimSpace(run.Runtime.TeamID) == "" {
			return false
		}
		if strings.TrimSpace(coordinatorRole) == "" {
			return true
		}
		return strings.EqualFold(strings.TrimSpace(run.Runtime.Role), strings.TrimSpace(coordinatorRole))
	})
	if selected.RunID == "" {
		selected = pickLatestRun(rootRuns, func(run types.Run) bool {
			return run.Runtime != nil && strings.TrimSpace(run.Runtime.TeamID) != ""
		})
	}
	if selected.RunID == "" {
		selected = pickLatestRun(rootRuns, nil)
	}
	if selected.RunID == "" {
		return webhookRoutingContext{}, fmt.Errorf("no active run available for webhook routing")
	}

	teamID := ""
	role := strings.TrimSpace(coordinatorRole)
	if selected.Runtime != nil {
		teamID = strings.TrimSpace(selected.Runtime.TeamID)
		if role == "" {
			role = strings.TrimSpace(selected.Runtime.Role)
		}
	}
	if teamID == "" && sessionLoader != nil {
		if sess, err := sessionLoader.LoadSession(ctx, strings.TrimSpace(selected.SessionID)); err == nil {
			teamID = strings.TrimSpace(sess.TeamID)
		}
	}
	if teamID != "" && role == "" && len(roleSet) == 1 {
		for only := range roleSet {
			role = only
		}
	}

	validRoles := map[string]struct{}{}
	for name := range roleSet {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		validRoles[name] = struct{}{}
	}
	if role != "" {
		validRoles[role] = struct{}{}
	}
	if len(validRoles) == 0 {
		validRoles = nil
	}

	return webhookRoutingContext{
		run:             selected,
		teamID:          teamID,
		coordinatorRole: role,
		validRoles:      validRoles,
	}, nil
}

func profileWebhookRoleHints(prof *profile.Profile) (map[string]struct{}, string, error) {
	if prof == nil {
		return nil, "", nil
	}
	sessionRoles, err := prof.RolesForSession()
	if err != nil {
		return nil, "", err
	}
	roleNames, coordinatorRole, err := team.ValidateTeamRoles(sessionRoles)
	if err != nil {
		return nil, "", err
	}
	roleSet := make(map[string]struct{}, len(roleNames))
	for _, roleName := range roleNames {
		name := strings.TrimSpace(roleName)
		if name == "" {
			continue
		}
		roleSet[name] = struct{}{}
	}
	return roleSet, strings.TrimSpace(coordinatorRole), nil
}

func pickLatestRun(runs []types.Run, match func(types.Run) bool) types.Run {
	var selected types.Run
	var selectedStart time.Time
	for _, run := range runs {
		if match != nil && !match(run) {
			continue
		}
		startedAt := time.Time{}
		if run.StartedAt != nil {
			startedAt = run.StartedAt.UTC()
		}
		if selected.RunID == "" || startedAt.After(selectedStart) {
			selected = run
			selectedStart = startedAt
		}
	}
	return selected
}
