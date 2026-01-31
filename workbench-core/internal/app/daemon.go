package app

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/tinoosan/workbench-core/internal/store"
	"github.com/tinoosan/workbench-core/pkg/agent"
	"github.com/tinoosan/workbench-core/pkg/config"
	"github.com/tinoosan/workbench-core/pkg/events"
	"github.com/tinoosan/workbench-core/pkg/fsutil"
	"github.com/tinoosan/workbench-core/pkg/llm"
	"github.com/tinoosan/workbench-core/pkg/resources"
	"github.com/tinoosan/workbench-core/pkg/role"
	"github.com/tinoosan/workbench-core/pkg/runtime"
	"github.com/tinoosan/workbench-core/pkg/types"
)

// RunDaemon starts a headless worker that continuously polls /inbox and writes results to /outbox.
// It is intended as the default autonomous entrypoint; the TUI can be used separately as a viewer.
func RunDaemon(ctx context.Context, cfg config.Config, goal string, maxContextB int, poll time.Duration, opts ...RunChatOption) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	resolved := resolveRunChatOptions(opts...)
	if maxContextB <= 0 {
		maxContextB = 8 * 1024
	}
	if poll <= 0 {
		poll = 2 * time.Second
	}
	goal = strings.TrimSpace(goal)
	if goal == "" {
		goal = "autonomous agent"
	}

	// Create session/run up front.
	sess, run, err := store.CreateSession(cfg, goal, maxContextB)
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}

	historyStore, err := store.NewSQLiteHistoryStore(cfg, sess.SessionID)
	if err != nil {
		return fmt.Errorf("create history store: %w", err)
	}
	historyRes, err := resources.NewHistoryResource(cfg, sess.SessionID, historyStore)
	if err != nil {
		return fmt.Errorf("create history: %w", err)
	}
	historySink := &events.HistorySink{Store: historyRes.Appender}
	mustEmit := func(ctx context.Context, ev events.Event) {
		if historySink != nil {
			_ = historySink.Emit(ctx, "daemon", ev)
		}
	}

	artifactIndex := newArtifactIndex()
	workdirAbs, err := resolveWorkDir(resolved.WorkDir)
	if err != nil {
		return err
	}

	resultsStore := store.NewInMemoryResultsStore()
	memStore, err := store.NewDiskMemoryStore(cfg, run.RunId)
	if err != nil {
		return err
	}
	profileStore, err := store.NewDiskProfileStore(cfg)
	if err != nil {
		return err
	}
	traceStore := store.DiskTraceStore{DiskStore: store.DiskStore{Dir: fsutil.GetLogDir(cfg.DataDir, run.RunId)}}
	constructorStore, err := store.NewSQLiteConstructorStore(cfg)
	if err != nil {
		return err
	}

	// Vector memory store (SQLite-backed) for semantic recall.
	// Best-effort: daemon can still run without this, but loses long-term recall.
	var memoryProvider agent.MemoryProvider
	if vm, err := store.NewVectorMemoryStore(cfg); err == nil {
		memoryProvider = &vectorMemoryAdapter{store: vm}
	} else {
		mustEmit(context.Background(), events.Event{
			Type:    "daemon.warning",
			Message: "Vector memory disabled",
			Data:    map[string]string{"error": err.Error()},
		})
	}

	rt, err := runtime.Build(runtime.BuildConfig{
		Cfg:                   cfg,
		Run:                   run,
		WorkdirAbs:            workdirAbs,
		Model:                 resolved.Model,
		ReasoningEffort:       strings.TrimSpace(resolved.ReasoningEffort),
		ReasoningSummary:      strings.TrimSpace(resolved.ReasoningSummary),
		ApprovalsMode:         strings.TrimSpace(resolved.ApprovalsMode),
		HistoryStore:          historyStore,
		ResultsStore:          resultsStore,
		MemoryStore:           memStore,
		ProfileStore:          profileStore,
		TraceStore:            &traceStore,
		ConstructorStore:      constructorStore,
		Emit:                  mustEmit,
		IncludeHistoryOps:     derefBool(resolved.IncludeHistoryOps, true),
		RecentHistoryPairs:    resolved.RecentHistoryPairs,
		MaxProfileBytes:       resolved.MaxProfileBytes,
		MaxMemoryBytes:        resolved.MaxMemoryBytes,
		MaxTraceBytes:         resolved.MaxTraceBytes,
		PriceInPerMTokensUSD:  resolved.PriceInPerMTokensUSD,
		PriceOutPerMTokensUSD: resolved.PriceOutPerMTokensUSD,
		Guard:                 nil,
		ArtifactObserve:       artifactIndex.ObserveWrite,
		PersistRun: func(r types.Run) error {
			return store.SaveRun(cfg, r)
		},
		LoadSession: func(sessionID string) (types.Session, error) {
			return store.LoadSession(cfg, sessionID)
		},
		SaveSession: func(session types.Session) error {
			return store.SaveSession(cfg, session)
		},
	})
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

	baseSystemPrompt := agent.DefaultSystemPrompt()
	agentCfg := agent.DefaultConfig()
	agentCfg.Model = resolved.Model
	agentCfg.ReasoningEffort = strings.TrimSpace(resolved.ReasoningEffort)
	agentCfg.ReasoningSummary = strings.TrimSpace(resolved.ReasoningSummary)
	agentCfg.ApprovalsMode = strings.TrimSpace(resolved.ApprovalsMode)
	agentCfg.EnableWebSearch = resolved.WebSearchEnabled
	agentCfg.SystemPrompt = baseSystemPrompt
	agentCfg.Context = rt.Constructor
	agentCfg.ToolManifests = rt.ToolManifests

	a, err := agent.NewAgent(llmClient, rt.Executor, agentCfg)
	if err != nil {
		return err
	}

	runner, err := agent.NewAutonomousRunner(agent.AutonomousRunnerConfig{
		Agent:             a,
		Role:              role.Get(resolved.Role),
		Memory:            memoryProvider,
		MemorySearchLimit: 3,
		InboxPath:         "/inbox",
		OutboxPath:        "/outbox",
		PollInterval:      poll,
		ProactiveInterval: 30 * time.Second,
		InitialGoal:       goal,
		MaxReadBytes:      96 * 1024,
		Logf: func(format string, args ...any) {
			log.Printf("daemon: "+format, args...)
		},
		Emit: mustEmit,
	})
	if err != nil {
		return err
	}

	runCtx, stopSignals := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stopSignals()

	mustEmit(context.Background(), events.Event{
		Type:    "daemon.start",
		Message: "Autonomous agent started",
		Data:    map[string]string{"runId": run.RunId, "sessionId": run.SessionID, "role": resolved.Role},
	})
	err = runner.Run(runCtx)
	if err != nil && runCtx.Err() != nil {
		// context cancellation is expected on shutdown
		err = nil
	}
	mustEmit(context.Background(), events.Event{
		Type:    "daemon.stop",
		Message: "Autonomous agent stopped",
		Data:    map[string]string{"runId": run.RunId, "sessionId": run.SessionID, "role": resolved.Role},
	})
	return err
}
