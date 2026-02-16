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
	hosttools "github.com/tinoosan/workbench-core/pkg/agent/hosttools"
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
	sessionStore     store.SessionReaderWriter
	taskStore        state.TaskStore

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
	if b.rt != nil {
		defer func() { _ = b.rt.Shutdown(context.Background()) }()
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

	err := b.runMainLoop()
	if b.runCtx == nil {
		b.runCtx = b.ctx
	}
	b.mustEmit(b.runCtx, events.Event{
		Type:    "daemon.stop",
		Message: "Autonomous agent stopped",
		Data:    map[string]string{"runId": b.run.RunID, "sessionId": b.run.SessionID, "profile": b.prof.ID},
	})
	b.serverWG.Wait()
	return err
}

func (b *DaemonBuilder) prepareBootstrap() error {
	if err := b.cfg.Validate(); err != nil {
		return err
	}
	if b.maxContextB <= 0 {
		b.maxContextB = 8 * 1024
	}
	if b.poll <= 0 {
		b.poll = 1 * time.Second // Faster inbox poll so callbacks and new tasks are picked up sooner
	}
	b.goal = strings.TrimSpace(b.goal)
	if b.goal == "" {
		b.goal = "autonomous agent"
	}

	// Reuse an existing daemon (system + standalone) session and its current run when present,
	// so restarts use the same run folder instead of creating a new one every time.
	profileID := strings.TrimSpace(b.prof.ID)
	if sessions, listErr := implstore.ListSessionsPaginated(b.cfg, store.SessionFilter{IncludeSystem: true, Limit: 20}); listErr == nil {
		for _, s := range sessions {
			if s.System && s.Mode == "standalone" && strings.TrimSpace(s.Profile) == profileID && strings.TrimSpace(s.CurrentRunID) != "" {
				if run, loadErr := implstore.LoadRun(b.cfg, s.CurrentRunID); loadErr == nil {
					now := time.Now().UTC()
					run.Status = types.RunStatusRunning
					run.StartedAt = &now
					run.FinishedAt = nil
					if saveErr := implstore.SaveRun(b.cfg, run); saveErr != nil {
						log.Printf("daemon: failed to persist reused run state: %v", saveErr)
					}
					b.bootstrapSession = s
					b.run = run
					break
				}
				// LoadRun failed (e.g. missing row); fall through to create new session and run
				break
			}
		}
	}

	if b.run.RunID == "" {
		// No reusable daemon session/run found; create new session and run.
		bootstrapSession, run, err := implstore.CreateSession(b.cfg, b.goal, b.maxContextB)
		if err != nil {
			return fmt.Errorf("create session: %w", err)
		}
		bootstrapSession.System = true
		bootstrapSession.Mode = "standalone"
		bootstrapSession.TeamID = ""
		bootstrapSession.Profile = profileID
		_ = implstore.SaveSession(b.cfg, bootstrapSession)
		b.bootstrapSession = bootstrapSession
		b.run = run
	}

	b.protocolInit = newProtocolInitializer(b.cfg, b.run, b.protocolEnabled)
	b.protocolInit.Initialize(context.Background())
	b.protocolSink = b.protocolInit.NewProtocolSink()

	emitter := &events.Emitter{
		RunID: b.run.RunID,
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
	b.traceStore = implstore.SQLiteTraceStore{Cfg: b.cfg, RunID: b.run.RunID}

	hs, err := implstore.NewSQLiteHistoryStore(b.cfg, b.run.SessionID)
	if err != nil {
		return fmt.Errorf("create history store: %w", err)
	}
	b.historyStore = hs

	cs, err := implstore.NewSQLiteConstructorStore(b.cfg)
	if err != nil {
		return fmt.Errorf("create constructor store: %w", err)
	}
	b.constructorStore = cs

	ss, err := implstore.NewSQLiteSessionStore(b.cfg)
	if err != nil {
		return fmt.Errorf("create session store: %w", err)
	}
	b.sessionStore = ss

	b.effectiveModel = strings.TrimSpace(b.resolved.Model)
	// Profile model takes priority over env var for standalone profiles.
	if b.prof != nil && b.prof.Model != "" {
		b.effectiveModel = b.prof.Model
	}
	loadedSession := types.Session{}
	if loaded, lerr := b.sessionStore.LoadSession(b.ctx, b.run.SessionID); lerr == nil {
		loadedSession = loaded
		// Session active model (runtime switch) takes highest priority.
		if active := strings.TrimSpace(loaded.ActiveModel); active != "" {
			b.effectiveModel = active
		}
	}
	if b.effectiveModel == "" {
		return fmt.Errorf("effective model is required")
	}
	ensureSessionReasoningForModel(&loadedSession, b.effectiveModel, strings.TrimSpace(b.resolved.ReasoningEffort), strings.TrimSpace(b.resolved.ReasoningSummary))
	if strings.TrimSpace(loadedSession.SessionID) != "" {
		loadedSession.ActiveModel = b.effectiveModel
		_ = b.sessionStore.SaveSession(b.ctx, loadedSession)
	}
	b.loadedSession = loadedSession
	b.initialReasoningEffort = loadedSession.ReasoningEffort
	b.initialReasoningSummary = loadedSession.ReasoningSummary

	b.memoryProvider = &textMemoryAdapter{store: b.memStore}

	rt, err := runtime.Build(runtime.BuildConfig{
		Cfg:                   b.cfg,
		Run:                   b.run,
		Profile:               strings.TrimSpace(b.prof.ID),
		ProfileConfig:         b.prof,
		WorkdirAbs:            b.workdirAbs,
		Model:                 b.effectiveModel,
		ReasoningEffort:       b.initialReasoningEffort,
		ReasoningSummary:      b.initialReasoningSummary,
		ApprovalsMode:         strings.TrimSpace(b.resolved.ApprovalsMode),
		HistoryStore:          b.historyStore,
		MemoryStore:           b.memStore,
		TraceStore:            b.traceStore,
		ConstructorStore:      b.constructorStore,
		Emit:                  b.mustEmit,
		IncludeHistoryOps:     derefBool(b.resolved.IncludeHistoryOps, true),
		RecentHistoryPairs:    b.resolved.RecentHistoryPairs,
		MaxMemoryBytes:        b.resolved.MaxMemoryBytes,
		MaxTraceBytes:         b.resolved.MaxTraceBytes,
		PriceInPerMTokensUSD:  b.resolved.PriceInPerMTokensUSD,
		PriceOutPerMTokensUSD: b.resolved.PriceOutPerMTokensUSD,
		Guard:                 nil,
		ArtifactObserve:       b.artifactIndex.ObserveWrite,
		PersistRun: func(r types.Run) error {
			return implstore.SaveRun(b.cfg, r)
		},
		LoadSession: func(sessionID string) (types.Session, error) {
			return b.sessionStore.LoadSession(context.Background(), sessionID)
		},
		SaveSession: func(session types.Session) error {
			return b.sessionStore.SaveSession(context.Background(), session)
		},
	})
	if err != nil {
		return err
	}
	b.rt = rt
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
	llmClient := withRetryDiagnostics(b.baseLLMClient, b.mustEmit)

	b.currentModel = strings.TrimSpace(b.effectiveModel)

	agentCfg := agent.DefaultConfig()
	agentCfg.Model = b.effectiveModel
	agentCfg.ReasoningEffort = b.initialReasoningEffort
	agentCfg.ReasoningSummary = b.initialReasoningSummary
	agentCfg.ApprovalsMode = strings.TrimSpace(b.resolved.ApprovalsMode)
	agentCfg.EnableWebSearch = b.resolved.WebSearchEnabled
	agentCfg.SystemPrompt = agent.DefaultAutonomousSystemPrompt()
	var promptSource agent.PromptSource = b.rt.Constructor
	if b.rt.Updater != nil {
		promptSource = b.rt.Updater
	}
	agentCfg.PromptSource = promptSource
	agentCfg.Hooks = agent.Hooks{
		OnLLMUsage: newCostUsageHook(
			b.cfg,
			b.run,
			b.effectiveModel,
			b.resolved.PriceInPerMTokensUSD,
			b.resolved.PriceOutPerMTokensUSD,
			b.sessionStore,
			func() string {
				b.currentModelMu.Lock()
				model := b.currentModel
				b.currentModelMu.Unlock()
				return model
			},
			b.mustEmit,
		),
		OnStep: func(step int, model, effectiveModel, summary string) {
			model = strings.TrimSpace(model)
			effectiveModel = strings.TrimSpace(effectiveModel)
			summary = strings.TrimSpace(summary)
			data := map[string]string{
				"step":  strconv.Itoa(step),
				"model": model,
			}
			if effectiveModel != "" {
				data["effectiveModel"] = effectiveModel
			}
			if summary != "" {
				data["reasoningSummary"] = summary
			}
			b.mustEmit(b.ctx, events.Event{
				Type:    "agent.step",
				Message: fmt.Sprintf("Step %d completed", step),
				Data:    data,
			})
		},
	}

	taskStore, err := state.NewSQLiteTaskStore(fsutil.GetSQLitePath(b.cfg.DataDir))
	if err != nil {
		return fmt.Errorf("create task store: %w", err)
	}
	b.taskStore = taskStore
	b.supervisor = newRuntimeSupervisor(runtimeSupervisorConfig{
		Cfg:              b.cfg,
		Resolved:         b.resolved,
		PollInterval:     b.poll,
		TaskStore:        b.taskStore,
		SessionStore:     b.sessionStore,
		MemoryStore:      b.memStore,
		ConstructorStore: b.constructorStore,
		LLMClient:        b.baseLLMClient,
		Notifier:         b.notifier,
		WorkdirAbs:       b.workdirAbs,
		BootstrapRunID:   strings.TrimSpace(b.run.RunID),
		DefaultProfile:   b.prof,
	})

	b.wakeCh = make(chan struct{}, 1)

	registry, err := agent.DefaultHostToolRegistry()
	if err != nil {
		return fmt.Errorf("create host tool registry: %w", err)
	}
	if err := registry.Register(&hosttools.TaskCreateTool{
		Store:       b.taskStore,
		SessionID:   b.run.SessionID,
		RunID:       b.run.RunID,
		SpawnWorker: b.spawnWorkerRun,
	}); err != nil {
		return fmt.Errorf("register task_create tool: %w", err)
	}
	agentCfg.HostToolRegistry = registry
	b.agentCfg = agentCfg

	a, err := agent.NewAgent(llmClient, b.rt.Executor, b.agentCfg)
	if err != nil {
		return fmt.Errorf("create agent: %w", err)
	}
	sess, err := session.New(session.Config{
		Agent:      a,
		Profile:    b.prof,
		ProfileDir: b.profDir,
		ResolveProfile: func(ref string) (*profile.Profile, string, error) {
			return resolveProfileRef(b.cfg, strings.TrimSpace(ref))
		},
		TaskStore:         b.taskStore,
		Events:            b.orderedEmitter,
		Memory:            b.memoryProvider,
		MemorySearchLimit: 3,
		Notifier:          b.notifier,
		PollInterval:      b.poll,
		WakeCh:            b.wakeCh,
		MaxReadBytes:      256 * 1024,
		LeaseTTL:          2 * time.Minute,
		MaxRetries:        3,
		MaxPending:        50,
		SessionID:         b.run.SessionID,
		RunID:             b.run.RunID,
		InstanceID:        b.run.RunID,
		Logf: func(format string, args ...any) {
			log.Printf("daemon: "+format, args...)
		},
	})
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	b.sess = sess
	return nil
}

func (b *DaemonBuilder) buildRPCServerConfig() RPCServerConfig {
	return RPCServerConfig{
		Cfg:            b.cfg,
		Run:            b.run,
		AllowAnyThread: true,
		TaskStore:      b.taskStore,
		Session:        b.sessionStore,
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
			loadedSession, err := b.sessionStore.LoadSession(ctx, threadID)
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
			if err := b.sessionStore.SaveSession(ctx, loadedSession); err != nil {
				return nil, err
			}
			if teamID := strings.TrimSpace(loadedSession.TeamID); teamID != "" {
				_ = persistTeamManifestModel(b.cfg, teamID, strings.TrimSpace(model), "rpc.control.setModel")
			}
			if threadID == strings.TrimSpace(b.run.SessionID) {
				if err := b.sess.SetModel(ctx, model); err == nil {
					b.currentModelMu.Lock()
					b.currentModel = strings.TrimSpace(model)
					b.currentModelMu.Unlock()
				}
				_ = b.sess.SetReasoning(ctx, loadedSession.ReasoningEffort, loadedSession.ReasoningSummary)
			} else {
				if _, err := b.supervisor.ApplySessionModel(ctx, threadID, targetRunID, model); err != nil {
					return nil, err
				}
			}
			return applied, nil
		},
		ControlSetReasoning: func(ctx context.Context, threadID, target, effort, summary string) ([]string, error) {
			threadID = strings.TrimSpace(threadID)
			if threadID == "" {
				return nil, &protocol.ProtocolError{Code: protocol.CodeInvalidParams, Message: "threadId is required"}
			}
			loadedSession, err := b.sessionStore.LoadSession(ctx, threadID)
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
			if err := b.sessionStore.SaveSession(ctx, loadedSession); err != nil {
				return nil, err
			}
			if threadID == strings.TrimSpace(b.run.SessionID) {
				if err := b.sess.SetReasoning(ctx, effort, summary); err != nil {
					return nil, err
				}
				return applied, nil
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
			if runID == strings.TrimSpace(b.run.RunID) && threadID == strings.TrimSpace(b.run.SessionID) {
				loaded, err := implstore.LoadRun(b.cfg, runID)
				if err != nil {
					return err
				}
				loaded.Status = types.RunStatusPaused
				loaded.FinishedAt = nil
				loaded.Error = nil
				if err := implstore.SaveRun(b.cfg, loaded); err != nil {
					return err
				}
				b.sess.SetPaused(true)
				b.runLoopMu.Lock()
				cancel := b.runLoopCancel
				b.runLoopMu.Unlock()
				if cancel != nil {
					cancel()
				}
				if err := cancelActiveTasksForRun(context.Background(), b.taskStore, runID, "run paused"); err != nil {
					return err
				}
				return nil
			}
			return b.supervisor.PauseRun(runID)
		},
		AgentResume: func(ctx context.Context, threadID, runID string) error {
			runID = strings.TrimSpace(runID)
			if _, err := b.validateAgentScope(ctx, threadID, runID); err != nil {
				return err
			}
			if runID == strings.TrimSpace(b.run.RunID) && threadID == strings.TrimSpace(b.run.SessionID) {
				loaded, err := implstore.LoadRun(b.cfg, runID)
				if err != nil {
					return err
				}
				loaded.Status = types.RunStatusRunning
				loaded.FinishedAt = nil
				loaded.Error = nil
				if err := implstore.SaveRun(b.cfg, loaded); err != nil {
					return err
				}
				b.sess.SetPaused(false)
				return nil
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
					if runID == strings.TrimSpace(b.run.RunID) {
						loaded, lerr := implstore.LoadRun(b.cfg, runID)
						if lerr == nil {
							loaded.Status = types.RunStatusPaused
							loaded.FinishedAt = nil
							loaded.Error = nil
							lerr = implstore.SaveRun(b.cfg, loaded)
						}
						if lerr != nil {
							mu.Lock()
							errs = append(errs, runID+": "+lerr.Error())
							mu.Unlock()
							return
						}
						b.sess.SetPaused(true)
						b.runLoopMu.Lock()
						cancel := b.runLoopCancel
						b.runLoopMu.Unlock()
						if cancel != nil {
							cancel()
						}
						if cerr := cancelActiveTasksForRun(context.Background(), b.taskStore, runID, "run paused"); cerr != nil {
							mu.Lock()
							errs = append(errs, runID+": "+cerr.Error())
							mu.Unlock()
							return
						}
					} else if perr := b.supervisor.PauseRun(runID); perr != nil {
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
					if runID == strings.TrimSpace(b.run.RunID) {
						loaded, lerr := implstore.LoadRun(b.cfg, runID)
						if lerr == nil {
							loaded.Status = types.RunStatusRunning
							loaded.FinishedAt = nil
							loaded.Error = nil
							lerr = implstore.SaveRun(b.cfg, loaded)
						}
						if lerr != nil {
							mu.Lock()
							errs = append(errs, runID+": "+lerr.Error())
							mu.Unlock()
							return
						}
						b.sess.SetPaused(false)
					} else if rerr := b.supervisor.ResumeRun(ctx, runID); rerr != nil {
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
					if runID == strings.TrimSpace(b.run.RunID) {
						loaded, lerr := implstore.LoadRun(b.cfg, runID)
						if lerr == nil {
							loaded.Status = types.RunStatusPaused
							loaded.FinishedAt = nil
							loaded.Error = nil
							lerr = implstore.SaveRun(b.cfg, loaded)
						}
						if lerr != nil {
							mu.Lock()
							errs = append(errs, runID+": "+lerr.Error())
							mu.Unlock()
							return
						}
						b.sess.SetPaused(true)
						b.runLoopMu.Lock()
						cancel := b.runLoopCancel
						b.runLoopMu.Unlock()
						if cancel != nil {
							cancel()
						}
						if serr := cancelActiveTasksForRun(context.Background(), b.taskStore, runID, "run stopped"); serr != nil {
							mu.Lock()
							errs = append(errs, runID+": "+serr.Error())
							mu.Unlock()
							return
						}
					} else if serr := b.supervisor.StopRun(runID); serr != nil {
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
	loadedSession, err := b.sessionStore.LoadSession(ctx, threadID)
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
	if _, err := b.sessionStore.LoadSession(ctx, threadID); err != nil {
		return types.Session{}, nil, &protocol.ProtocolError{Code: protocol.CodeThreadNotFound, Message: "thread not found"}
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		sessionID = threadID
	}
	if sessionID != threadID {
		return types.Session{}, nil, &protocol.ProtocolError{Code: protocol.CodeThreadNotFound, Message: "thread not found"}
	}
	loadedSession, err := b.sessionStore.LoadSession(ctx, sessionID)
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

	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-b.runCtx.Done():
				return
			case <-ticker.C:
				if loadedRun, rerr := implstore.LoadRun(b.cfg, b.run.RunID); rerr == nil {
					b.sess.SetPaused(strings.EqualFold(strings.TrimSpace(loadedRun.Status), types.RunStatusPaused))
				}
				loaded, lerr := b.sessionStore.LoadSession(b.runCtx, b.run.SessionID)
				if lerr != nil {
					continue
				}
				targetModel := strings.TrimSpace(loaded.ActiveModel)
				if targetModel == "" {
					continue
				}
				b.currentModelMu.Lock()
				same := strings.EqualFold(targetModel, b.currentModel)
				b.currentModelMu.Unlock()
				if same {
					continue
				}
				if err := b.sess.SetModel(b.runCtx, targetModel); err != nil {
					continue
				}
				b.currentModelMu.Lock()
				b.currentModel = targetModel
				b.currentModelMu.Unlock()
				b.mustEmit(b.runCtx, events.Event{
					Type:    "control.success",
					Message: "Model synchronized from session state",
					Data: map[string]string{
						"command": "set_model",
						"model":   targetModel,
					},
				})
			}
		}
	}()

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
		startWebhookServer(b.runCtx, webhookAddr, b.cfg, b.run, b.taskStore, b.mustEmit, &b.serverWG)
	}
	healthAddr := strings.TrimSpace(b.resolved.HealthAddr)
	if healthAddr != "" && healthAddr != webhookAddr {
		startHealthServer(b.runCtx, healthAddr, b.mustEmit, &b.serverWG)
	}

	b.mustEmit(b.runCtx, events.Event{
		Type:    "daemon.start",
		Message: "Autonomous agent started",
		Data:    map[string]string{"runId": b.run.RunID, "sessionId": b.run.SessionID, "profile": b.prof.ID},
	})
	if b.protocolEnabled {
		log.Printf("daemon: protocol control-plane ready at %s — attach with: workbench", strings.TrimSpace(b.resolved.RPCListen))
	} else {
		log.Printf("daemon: agent id %s — attach with: workbench --agent-id %s", b.run.RunID, b.run.RunID)
	}
	return nil
}

func (b *DaemonBuilder) runMainLoop() error {
	for {
		runLoopCtx, cancelRunLoop := context.WithCancel(b.runCtx)
		b.runLoopMu.Lock()
		b.runLoopCancel = cancelRunLoop
		b.runLoopMu.Unlock()
		err := b.sess.Run(runLoopCtx)
		cancelRunLoop()
		b.runLoopMu.Lock()
		b.runLoopCancel = nil
		b.runLoopMu.Unlock()
		if b.runCtx.Err() != nil {
			return nil
		}
		errMsg := "unknown error"
		if err != nil {
			errMsg = err.Error()
		}
		b.mustEmit(b.runCtx, events.Event{
			Type:    "daemon.runner.error",
			Message: "Runner exited unexpectedly; restarting",
			Data:    map[string]string{"error": errMsg},
		})
		time.Sleep(2 * time.Second)
	}
}

// spawnWorkerRun creates a child Run for a spawned worker and adds it to the session.
// The supervisor discovers the new run and starts a managed runtime for it.
func (b *DaemonBuilder) spawnWorkerRun(ctx context.Context, goal, sessionID, parentRunID string) (string, error) {
	// Count existing children to determine spawn index.
	children, _ := implstore.ListChildRuns(b.cfg, parentRunID)
	spawnIndex := len(children) + 1

	childRun := types.NewChildRun(parentRunID, goal, sessionID, spawnIndex)

	// Resolve subagent model: env var > profile-level > parent model.
	subagentModel := strings.TrimSpace(b.resolved.SubagentModel)
	if subagentModel == "" && b.prof != nil {
		subagentModel = strings.TrimSpace(b.prof.SubagentModel)
	}
	if subagentModel == "" {
		subagentModel = b.effectiveModel
	}

	childRun.Runtime = &types.RunRuntimeConfig{
		DataDir: b.cfg.DataDir,
		Profile: strings.TrimSpace(b.prof.ID),
		Model:   subagentModel,
	}

	if err := implstore.SaveRun(b.cfg, childRun); err != nil {
		return "", fmt.Errorf("save child run: %w", err)
	}

	// Add child run to session's run list so the supervisor discovers it.
	sess, err := b.sessionStore.LoadSession(ctx, sessionID)
	if err != nil {
		return "", fmt.Errorf("load session for spawn: %w", err)
	}
	sess.Runs = append(sess.Runs, childRun.RunID)
	if err := b.sessionStore.SaveSession(ctx, sess); err != nil {
		return "", fmt.Errorf("save session for spawn: %w", err)
	}

	b.mustEmit(ctx, events.Event{
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

	return childRun.RunID, nil
}
