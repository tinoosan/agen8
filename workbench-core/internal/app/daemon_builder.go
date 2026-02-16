package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	implstore "github.com/tinoosan/workbench-core/internal/store"
	"github.com/tinoosan/workbench-core/pkg/agent"
	"github.com/tinoosan/workbench-core/pkg/agent/session"
	"github.com/tinoosan/workbench-core/pkg/agent/state"
	"github.com/tinoosan/workbench-core/pkg/config"
	"github.com/tinoosan/workbench-core/pkg/emit"
	"github.com/tinoosan/workbench-core/pkg/events"
	"github.com/tinoosan/workbench-core/pkg/fsutil"
	"github.com/tinoosan/workbench-core/pkg/llm"
	llmtypes "github.com/tinoosan/workbench-core/pkg/llm/types"
	"github.com/tinoosan/workbench-core/pkg/profile"
	"github.com/tinoosan/workbench-core/pkg/protocol"
	"github.com/tinoosan/workbench-core/pkg/runtime"
	pkgtask "github.com/tinoosan/workbench-core/pkg/services/task"
	pkgsession "github.com/tinoosan/workbench-core/pkg/services/session"
	"github.com/tinoosan/workbench-core/pkg/store"
	"github.com/tinoosan/workbench-core/pkg/types"
)

type DaemonBuilder struct {
	ctx context.Context

	cfg             config.Config
	goal            string
	maxContextB     int
	poll            time.Duration
	resolved        RunChatOptions
	prof            *profile.Profile
	profDir         string
	protocolEnabled bool

	bootstrapSession types.Session
	run              types.Run

	protocolInit *ProtocolInitializer
	protocolSink *protocol.EventSink

	orderedEmitter *emit.OrderedEmitter[events.Event]
	mustEmit       func(context.Context, events.Event)

	artifactIndex *ArtifactIndex
	workdirAbs    string

	memStore         store.DailyMemoryStore
	traceStore       store.TraceStore
	historyStore     store.HistoryStore
	constructorStore store.ConstructorStateStore
	sessionStore     pkgsession.Store
	sessionService   pkgsession.Service
	taskStore        state.TaskStore
	taskManager      *pkgtask.Manager
	taskService      pkgtask.TaskServiceForSupervisor

	effectiveModel          string
	loadedSession           types.Session
	initialReasoningEffort  string
	initialReasoningSummary string

	notifier       agent.Notifier
	memoryProvider agent.MemoryRecallProvider

	rt *runtime.Runtime

	baseLLMClient  llmtypes.LLMClient
	currentModel   string
	currentModelMu sync.Mutex

	supervisor *runtimeSupervisor
	wakeCh     chan struct{}
	agentCfg   agent.AgentConfig
	sess       *session.Session

	runCtx      context.Context
	stopSignals context.CancelFunc
	serverWG    sync.WaitGroup

	runLoopMu     sync.Mutex
	runLoopCancel context.CancelFunc
}

func newDaemonBuilder(ctx context.Context, cfg config.Config, goal string, maxContextB int, poll time.Duration, resolved RunChatOptions, prof *profile.Profile, profDir string, protocolEnabled bool) *DaemonBuilder {
	return &DaemonBuilder{
		ctx:             ctx,
		cfg:             cfg,
		goal:            goal,
		maxContextB:     maxContextB,
		poll:            poll,
		resolved:        resolved,
		prof:            prof,
		profDir:         profDir,
		protocolEnabled: protocolEnabled,
	}
}

func (b *DaemonBuilder) Run() error {
	if err := b.prepareBootstrap(); err != nil {
		return err
	}
	if b.orderedEmitter != nil {
		defer b.orderedEmitter.Close()
	}

	if err := b.buildStoresAndRuntime(); err != nil {
		return err
	}

	if err := b.buildAgentAndSupervisor(); err != nil {
		return err
	}

	if err := b.startBackgroundServices(); err != nil {
		return err
	}
	if b.stopSignals != nil {
		defer b.stopSignals()
	}
	if b.supervisor != nil {
		defer b.supervisor.stopAll()
	}

	<-b.runCtx.Done()
	b.mustEmit(b.ctx, events.Event{
		Type:    "daemon.stop",
		Message: "Daemon stopped",
		Data:    map[string]string{},
	})
	b.serverWG.Wait()
	return nil
}

func (b *DaemonBuilder) prepareBootstrap() error {
	if err := b.cfg.Validate(); err != nil {
		return err
	}
	if b.maxContextB <= 0 {
		b.maxContextB = 8 * 1024
	}
	if b.poll <= 0 {
		b.poll = 1 * time.Second
	}
	b.goal = strings.TrimSpace(b.goal)
	if b.goal == "" {
		b.goal = "autonomous agent"
	}

	// No session or run created by daemon; all creation happens via RPC.
	b.protocolInit = newProtocolInitializer(b.cfg, types.Run{}, b.protocolEnabled)
	b.protocolInit.Initialize(context.Background())
	b.protocolSink = b.protocolInit.NewProtocolSink()

	emitter := &events.Emitter{
		RunID: "",
		Sink: events.MultiSink{
			events.StoreSink{Store: daemonEventAppender{cfg: b.cfg}},
			b.protocolSink,
		},
	}
	b.orderedEmitter = emit.NewOrderedEmitter[events.Event](emitter)
	b.mustEmit = func(ctx context.Context, ev events.Event) {
		if err := b.orderedEmitter.Emit(ctx, ev); err != nil && !errors.Is(err, events.ErrDropped) {
			log.Printf("events: emit failed: %v", err)
		}
	}

	b.artifactIndex = newArtifactIndex()
	workdirAbs, err := resolveWorkDir(b.resolved.WorkDir)
	if err != nil {
		return err
	}
	b.workdirAbs = workdirAbs

	if cwd, err := os.Getwd(); err == nil {
		if derr := loadDotEnvFromDir(cwd); derr != nil {
			b.mustEmit(b.ctx, events.Event{
				Type:    "daemon.warning",
				Message: ".env load failed (cwd); continuing",
				Data:    map[string]string{"error": derr.Error()},
			})
		}
		if strings.TrimSpace(b.workdirAbs) != "" && strings.TrimSpace(b.workdirAbs) != strings.TrimSpace(cwd) {
			if derr := loadDotEnvFromDir(b.workdirAbs); derr != nil {
				b.mustEmit(b.ctx, events.Event{
					Type:    "daemon.warning",
					Message: ".env load failed (workdir); continuing",
					Data:    map[string]string{"error": derr.Error()},
				})
			}
		}
	} else if strings.TrimSpace(b.workdirAbs) != "" {
		if derr := loadDotEnvFromDir(b.workdirAbs); derr != nil {
			b.mustEmit(b.ctx, events.Event{
				Type:    "daemon.warning",
				Message: ".env load failed (workdir); continuing",
				Data:    map[string]string{"error": derr.Error()},
			})
		}
	}
	if strings.TrimSpace(b.resolved.ResultWebhookURL) != "" {
		b.notifier = WebhookNotifier{URL: b.resolved.ResultWebhookURL}
	}
	return nil
}

func (b *DaemonBuilder) buildStoresAndRuntime() error {
	ms, err := implstore.NewDiskMemoryStore(b.cfg)
	if err != nil {
		return fmt.Errorf("create memory store: %w", err)
	}
	b.memStore = ms

	cs, err := implstore.NewSQLiteConstructorStore(b.cfg)
	if err != nil {
		return fmt.Errorf("create constructor store: %w", err)
	}
	b.constructorStore = cs

	ss, err := implstore.NewSQLiteSessionStore(b.cfg)
	if err != nil {
		return fmt.Errorf("create session store: %w", err)
	}
	// Implements pkgsession.Store
	b.sessionStore = ss
	return nil
}

func (b *DaemonBuilder) buildAgentAndSupervisor() error {
	client, err := llm.NewClientFromEnv()
	if err != nil {
		return fmt.Errorf("create LLM client: %w", err)
	}
	b.baseLLMClient = llm.NewRetryClient(client, llm.RetryConfig{
		MaxRetries:   3,
		InitialDelay: 250 * time.Millisecond,
		MaxDelay:     4 * time.Second,
		Multiplier:   2.0,
	})

	taskStore, err := state.NewSQLiteTaskStore(fsutil.GetSQLitePath(b.cfg.DataDir))
	if err != nil {
		return fmt.Errorf("create task store: %w", err)
	}
	b.taskStore = taskStore
	b.taskManager = pkgtask.NewManager(b.taskStore, nil)
	b.taskService = b.taskManager
	b.supervisor = newRuntimeSupervisor(runtimeSupervisorConfig{
		Cfg:              b.cfg,
		Resolved:         b.resolved,
		PollInterval:     b.poll,
		TaskService:      b.taskService,
		SessionStore:     b.sessionStore,
		MemoryStore:      b.memStore,
		ConstructorStore: b.constructorStore,
		LLMClient:        b.baseLLMClient,
		Notifier:         b.notifier,
		WorkdirAbs:       b.workdirAbs,
		BootstrapRunID:   "",
		DefaultProfile:   b.prof,
	})
	b.wakeCh = make(chan struct{}, 1)
	b.sessionService = pkgsession.NewManager(b.cfg, b.sessionStore, b.supervisor)
	b.taskManager.SetRunLoader(b.sessionService)
	return nil
}

func (b *DaemonBuilder) buildRPCServerConfig() RPCServerConfig {
	return RPCServerConfig{
		Cfg:            b.cfg,
		Run:            b.run,
		AllowAnyThread: true,
		TaskService:    b.taskManager,
		Session:        b.sessionService,
		NotifyCh:       b.protocolInit.NotifyCh(),
		Index:          b.protocolInit.Index(),
		Wake: func() {
			select {
			case b.wakeCh <- struct{}{}:
			default:
			}
		},
		ControlSetModel: func(ctx context.Context, threadID, target, model string) ([]string, error) {
			threadID = strings.TrimSpace(threadID)
			if threadID == "" {
				return nil, &protocol.ProtocolError{Code: protocol.CodeInvalidParams, Message: "threadId is required"}
			}
			loadedSession, err := b.sessionService.LoadSession(ctx, threadID)
			if err != nil || strings.TrimSpace(loadedSession.SessionID) != threadID {
				return nil, &protocol.ProtocolError{Code: protocol.CodeThreadNotFound, Message: "thread not found"}
			}
			target = strings.TrimSpace(target)
			applied := collectSessionRunIDs(loadedSession)
			targetRunID := ""
			if target != "" && target != threadID {
				found := false
				for _, rid := range applied {
					if strings.TrimSpace(rid) == target {
						found = true
						targetRunID = target
						applied = []string{target}
						break
					}
				}
				if !found {
					return nil, &protocol.ProtocolError{Code: protocol.CodeInvalidParams, Message: "target does not match active run"}
				}
			}
			loadedSession.ActiveModel = strings.TrimSpace(model)
			ensureSessionReasoningForModel(&loadedSession, loadedSession.ActiveModel, strings.TrimSpace(b.resolved.ReasoningEffort), strings.TrimSpace(b.resolved.ReasoningSummary))
			if err := b.sessionService.SaveSession(ctx, loadedSession); err != nil {
				return nil, err
			}
			if teamID := strings.TrimSpace(loadedSession.TeamID); teamID != "" {
				_ = persistTeamManifestModel(b.cfg, teamID, strings.TrimSpace(model), "rpc.control.setModel")
			}
			if _, err := b.supervisor.ApplySessionModel(ctx, threadID, targetRunID, model); err != nil {
				return nil, err
			}
			return applied, nil
		},
		ControlSetReasoning: func(ctx context.Context, threadID, target, effort, summary string) ([]string, error) {
			threadID = strings.TrimSpace(threadID)
			if threadID == "" {
				return nil, &protocol.ProtocolError{Code: protocol.CodeInvalidParams, Message: "threadId is required"}
			}
			loadedSession, err := b.sessionService.LoadSession(ctx, threadID)
			if err != nil || strings.TrimSpace(loadedSession.SessionID) != threadID {
				return nil, &protocol.ProtocolError{Code: protocol.CodeThreadNotFound, Message: "thread not found"}
			}
			target = strings.TrimSpace(target)
			applied := collectSessionRunIDs(loadedSession)
			targetRunID := ""
			if target != "" && target != threadID {
				found := false
				for _, rid := range applied {
					if strings.TrimSpace(rid) == target {
						found = true
						targetRunID = target
						applied = []string{target}
						break
					}
				}
				if !found {
					return nil, &protocol.ProtocolError{Code: protocol.CodeInvalidParams, Message: "target does not match active run"}
				}
			}
			effort = strings.ToLower(strings.TrimSpace(effort))
			summary = normalizeReasoningSummaryValue(summary)
			storeSessionReasoningPreference(&loadedSession, strings.TrimSpace(loadedSession.ActiveModel), effort, summary)
			if err := b.sessionService.SaveSession(ctx, loadedSession); err != nil {
				return nil, err
			}
			if _, err := b.supervisor.ApplySessionReasoning(ctx, threadID, targetRunID, effort, summary); err != nil {
				return nil, err
			}
			return applied, nil
		},
		ControlSetProfile: func(ctx context.Context, threadID, target, profileRef string) ([]string, error) {
			_ = ctx
			_ = threadID
			_ = target
			_ = profileRef
			return nil, &protocol.ProtocolError{Code: protocol.CodeInvalidState, Message: "control.setProfile is disabled; use /new"}
		},
		AgentPause: func(ctx context.Context, threadID, runID string) error {
			runID = strings.TrimSpace(runID)
			if _, err := b.validateAgentScope(ctx, threadID, runID); err != nil {
				return err
			}
			return b.supervisor.PauseRun(runID)
		},
		AgentResume: func(ctx context.Context, threadID, runID string) error {
			runID = strings.TrimSpace(runID)
			if _, err := b.validateAgentScope(ctx, threadID, runID); err != nil {
				return err
			}
			return b.supervisor.ResumeRun(ctx, runID)
		},
		SessionPause: func(ctx context.Context, threadID, sessionID string) ([]string, error) {
			_, runIDs, err := b.validateSessionScope(ctx, threadID, sessionID)
			if err != nil {
				return nil, err
			}
			affected := make([]string, 0, len(runIDs))
			var mu sync.Mutex
			var wg sync.WaitGroup
			errs := make([]string, 0, len(runIDs))
			for _, rid := range runIDs {
				rid := strings.TrimSpace(rid)
				if rid == "" {
					continue
				}
				wg.Add(1)
				go func(runID string) {
					defer wg.Done()
					if perr := b.supervisor.PauseRun(runID); perr != nil {
						mu.Lock()
						errs = append(errs, runID+": "+perr.Error())
						mu.Unlock()
						return
					}
					mu.Lock()
					affected = append(affected, runID)
					mu.Unlock()
				}(rid)
			}
			wg.Wait()
			if len(errs) != 0 {
				return affected, fmt.Errorf("pause session partial failure: %s", strings.Join(errs, "; "))
			}
			return affected, nil
		},
		SessionResume: func(ctx context.Context, threadID, sessionID string) ([]string, error) {
			_, runIDs, err := b.validateSessionScope(ctx, threadID, sessionID)
			if err != nil {
				return nil, err
			}
			affected := make([]string, 0, len(runIDs))
			var mu sync.Mutex
			var wg sync.WaitGroup
			errs := make([]string, 0, len(runIDs))
			for _, rid := range runIDs {
				rid := strings.TrimSpace(rid)
				if rid == "" {
					continue
				}
				wg.Add(1)
				go func(runID string) {
					defer wg.Done()
					if rerr := b.supervisor.ResumeRun(ctx, runID); rerr != nil {
						mu.Lock()
						errs = append(errs, runID+": "+rerr.Error())
						mu.Unlock()
						return
					}
					mu.Lock()
					affected = append(affected, runID)
					mu.Unlock()
				}(rid)
			}
			wg.Wait()
			if len(errs) != 0 {
				return affected, fmt.Errorf("resume session partial failure: %s", strings.Join(errs, "; "))
			}
			return affected, nil
		},
		SessionStop: func(ctx context.Context, threadID, sessionID string) ([]string, error) {
			_, runIDs, err := b.validateSessionScope(ctx, threadID, sessionID)
			if err != nil {
				return nil, err
			}
			affected := make([]string, 0, len(runIDs))
			var mu sync.Mutex
			var wg sync.WaitGroup
			errs := make([]string, 0, len(runIDs))
			for _, rid := range runIDs {
				rid := strings.TrimSpace(rid)
				if rid == "" {
					continue
				}
				wg.Add(1)
				go func(runID string) {
					defer wg.Done()
					if serr := b.supervisor.StopRun(runID); serr != nil {
						mu.Lock()
						errs = append(errs, runID+": "+serr.Error())
						mu.Unlock()
						return
					}
					mu.Lock()
					affected = append(affected, runID)
					mu.Unlock()
				}(rid)
			}
			wg.Wait()
			if len(errs) != 0 {
				return affected, fmt.Errorf("stop session partial failure: %s", strings.Join(errs, "; "))
			}
			return affected, nil
		},
	}
}

func (b *DaemonBuilder) validateAgentScope(ctx context.Context, threadID, runID string) (types.Session, error) {
	threadID = strings.TrimSpace(threadID)
	if threadID == "" {
		return types.Session{}, &protocol.ProtocolError{Code: protocol.CodeInvalidParams, Message: "threadId is required"}
	}
	loadedSession, err := b.sessionService.LoadSession(ctx, threadID)
	if err != nil || strings.TrimSpace(loadedSession.SessionID) != threadID {
		return types.Session{}, &protocol.ProtocolError{Code: protocol.CodeThreadNotFound, Message: "thread not found"}
	}
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return types.Session{}, &protocol.ProtocolError{Code: protocol.CodeInvalidParams, Message: "runId is required"}
	}
	for _, rid := range collectSessionRunIDs(loadedSession) {
		if strings.TrimSpace(rid) == runID {
			return loadedSession, nil
		}
	}
	return types.Session{}, &protocol.ProtocolError{Code: protocol.CodeThreadNotFound, Message: "thread not found"}
}

func (b *DaemonBuilder) validateSessionScope(ctx context.Context, threadID, sessionID string) (types.Session, []string, error) {
	threadID = strings.TrimSpace(threadID)
	if threadID == "" {
		return types.Session{}, nil, &protocol.ProtocolError{Code: protocol.CodeInvalidParams, Message: "threadId is required"}
	}
	if _, err := b.sessionService.LoadSession(ctx, threadID); err != nil {
		return types.Session{}, nil, &protocol.ProtocolError{Code: protocol.CodeThreadNotFound, Message: "thread not found"}
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		sessionID = threadID
	}
	if sessionID != threadID {
		return types.Session{}, nil, &protocol.ProtocolError{Code: protocol.CodeThreadNotFound, Message: "thread not found"}
	}
	loadedSession, err := b.sessionService.LoadSession(ctx, sessionID)
	if err != nil {
		return types.Session{}, nil, err
	}
	runIDs := collectSessionRunIDs(loadedSession)
	return loadedSession, runIDs, nil
}

func (b *DaemonBuilder) startBackgroundServices() error {
	runCtx, stopSignals := signal.NotifyContext(b.ctx, os.Interrupt, syscall.SIGTERM)
	b.runCtx = runCtx
	b.stopSignals = stopSignals

	go b.supervisor.Run(b.runCtx)

	if b.protocolEnabled {
		srvCfg := b.buildRPCServerConfig()
		if err := b.protocolInit.StartServers(b.runCtx, srvCfg, strings.TrimSpace(b.resolved.RPCListen)); err != nil {
			return err
		}
	}

	{
		const (
			sessionRetention = 30 * 24 * time.Hour
			pruneInterval    = 6 * time.Hour
		)
		go func() {
			pruneOnce := func() {
				pruneCtx, cancel := context.WithTimeout(b.runCtx, 2*time.Minute)
				defer cancel()
				res, err := implstore.PruneOldSessions(pruneCtx, b.cfg, sessionRetention)
				if err != nil {
					log.Printf("daemon: prune sessions failed: %v", err)
					b.mustEmit(b.runCtx, events.Event{
						Type:    "daemon.prune.error",
						Message: "Session pruning failed",
						Data:    map[string]string{"error": err.Error()},
					})
					return
				}
				if res.Sessions <= 0 {
					return
				}
				log.Printf(
					"daemon: pruned %d sessions (runs=%d events=%d history=%d activities=%d constructor_state=%d constructor_manifest=%d)",
					res.Sessions,
					res.Runs,
					res.Events,
					res.History,
					res.Activities,
					res.ConstructorState,
					res.ConstructorManifest,
				)
				b.mustEmit(b.runCtx, events.Event{
					Type:    "daemon.prune",
					Message: "Pruned old sessions",
					Data: map[string]string{
						"sessions":             strconv.FormatInt(res.Sessions, 10),
						"runs":                 strconv.FormatInt(res.Runs, 10),
						"events":               strconv.FormatInt(res.Events, 10),
						"history":              strconv.FormatInt(res.History, 10),
						"activities":           strconv.FormatInt(res.Activities, 10),
						"constructor_state":    strconv.FormatInt(res.ConstructorState, 10),
						"constructor_manifest": strconv.FormatInt(res.ConstructorManifest, 10),
					},
				})
			}

			pruneOnce()
			ticker := time.NewTicker(pruneInterval)
			defer ticker.Stop()
			for {
				select {
				case <-b.runCtx.Done():
					return
				case <-ticker.C:
					pruneOnce()
				}
			}
		}()
	}

	webhookAddr := strings.TrimSpace(b.resolved.WebhookAddr)
	if webhookAddr != "" {
		startWebhookServer(b.runCtx, webhookAddr, b.cfg, types.Run{}, b.taskService, b.mustEmit, &b.serverWG)
	}
	healthAddr := strings.TrimSpace(b.resolved.HealthAddr)
	if healthAddr != "" && healthAddr != webhookAddr {
		startHealthServer(b.runCtx, healthAddr, b.mustEmit, &b.serverWG)
	}

	b.mustEmit(b.runCtx, events.Event{
		Type:    "daemon.start",
		Message: "Daemon started",
		Data:    map[string]string{},
	})
	if b.protocolEnabled {
		log.Printf("daemon: protocol control-plane ready at %s — attach with: workbench", strings.TrimSpace(b.resolved.RPCListen))
	} else {
		log.Printf("daemon: ready — attach with: workbench")
	}
	return nil
}
