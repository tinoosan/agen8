package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/tinoosan/workbench-core/internal/store"
	"github.com/tinoosan/workbench-core/pkg/agent"
	"github.com/tinoosan/workbench-core/pkg/events"
	"github.com/tinoosan/workbench-core/pkg/fsutil"
	"github.com/tinoosan/workbench-core/pkg/llm"
	"github.com/tinoosan/workbench-core/pkg/orchestrator"
	"github.com/tinoosan/workbench-core/pkg/runtime"
	"github.com/tinoosan/workbench-core/pkg/types"
	"github.com/tinoosan/workbench-core/pkg/vfs"
)

func (r *tuiTurnRunner) startSwarmWorker(run types.Run) error {
	if r == nil {
		return fmt.Errorf("runner is nil")
	}
	if strings.TrimSpace(run.RunId) == "" {
		return fmt.Errorf("run id is required")
	}
	if r.swarmWorkers == nil {
		r.swarmWorkers = map[string]context.CancelFunc{}
	}
	if _, exists := r.swarmWorkers[run.RunId]; exists {
		return fmt.Errorf("worker already running for %s", run.RunId)
	}

	model := strings.TrimSpace(r.model)
	if model == "" {
		model = strings.TrimSpace(r.opts.Model)
	}
	if model == "" {
		return fmt.Errorf("model is required")
	}

	historyStore, err := store.NewDiskHistoryStore(r.cfg, run.SessionID)
	if err != nil {
		return fmt.Errorf("history store: %w", err)
	}

	workdirAbs := strings.TrimSpace(r.workdirBase)
	if workdirAbs == "" {
		return fmt.Errorf("workdir is required")
	}

	resultsStore := store.NewInMemoryResultsStore()
	memStore, err := store.NewDiskMemoryStore(r.cfg, run.RunId)
	if err != nil {
		return fmt.Errorf("memory store: %w", err)
	}
	profileStore, err := store.NewDiskProfileStore(r.cfg)
	if err != nil {
		return fmt.Errorf("profile store: %w", err)
	}
	traceStore := store.DiskTraceStore{DiskStore: store.DiskStore{Dir: fsutil.GetLogDir(r.cfg.DataDir, run.RunId)}}
	constructorStore, err := store.NewSQLiteConstructorStore(r.cfg)
	if err != nil {
		return fmt.Errorf("constructor store: %w", err)
	}

	rt, err := runtime.Build(runtime.BuildConfig{
		Cfg:                   r.cfg,
		Run:                   run,
		WorkdirAbs:            workdirAbs,
		Model:                 model,
		ReasoningEffort:       strings.TrimSpace(r.opts.ReasoningEffort),
		ReasoningSummary:      strings.TrimSpace(r.opts.ReasoningSummary),
		ApprovalsMode:         strings.TrimSpace(r.opts.ApprovalsMode),
		HistoryStore:          historyStore,
		ResultsStore:          resultsStore,
		MemoryStore:           memStore,
		ProfileStore:          profileStore,
		TraceStore:            &traceStore,
		ConstructorStore:      constructorStore,
		Emit:                  func(ctx context.Context, ev events.Event) {},
		IncludeHistoryOps:     derefBool(r.opts.IncludeHistoryOps, true),
		RecentHistoryPairs:    r.opts.RecentHistoryPairs,
		MaxProfileBytes:       r.opts.MaxProfileBytes,
		MaxMemoryBytes:        r.opts.MaxMemoryBytes,
		MaxTraceBytes:         r.opts.MaxTraceBytes,
		PriceInPerMTokensUSD:  r.opts.PriceInPerMTokensUSD,
		PriceOutPerMTokensUSD: r.opts.PriceOutPerMTokensUSD,
		Guard: func(fs *vfs.FS, req types.HostOpRequest) *types.HostOpResponse {
			return enforcePlanChecklist(fs, req)
		},
		PersistRun: func(runValue types.Run) error {
			return store.SaveRun(r.cfg, runValue)
		},
		LoadSession: func(sessionID string) (types.Session, error) {
			return store.LoadSession(r.cfg, sessionID)
		},
		SaveSession: func(session types.Session) error {
			return store.SaveSession(r.cfg, session)
		},
	})
	if err != nil {
		return fmt.Errorf("build runtime: %w", err)
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
	if sess, err := store.LoadSession(r.cfg, run.SessionID); err == nil {
		if blk := agent.SessionContextBlock(sess); strings.TrimSpace(blk) != "" {
			baseSystemPrompt = strings.TrimSpace(baseSystemPrompt) + "\n\n" + blk + "\n"
		}
	}

	a, err := agent.New(agent.Config{
		LLM:              llmClient,
		Exec:             rt.Executor,
		Model:            model,
		ReasoningEffort:  strings.TrimSpace(r.opts.ReasoningEffort),
		ReasoningSummary: strings.TrimSpace(r.opts.ReasoningSummary),
		ApprovalsMode:    strings.TrimSpace(r.opts.ApprovalsMode),
		EnableWebSearch:  r.opts.WebSearchEnabled,
		SystemPrompt:     baseSystemPrompt,
		Context:          rt.Constructor,
		ToolManifests:    rt.ToolManifests,
	})
	if err != nil {
		return fmt.Errorf("create agent: %w", err)
	}

	worker, err := agent.NewWorker(agent.WorkerConfig{
		Agent:        a,
		PollInterval: 2 * time.Second,
		InboxPath:    "/inbox",
		OutboxPath:   "/outbox",
		MaxReadBytes: 64 * 1024,
	})
	if err != nil {
		return fmt.Errorf("worker: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	r.swarmWorkers[run.RunId] = cancel
	go func() {
		_ = worker.Run(ctx)
	}()
	r.ensureSwarmSyncLoop()
	return nil
}

// ensureSwarmSyncLoop starts a best-effort background sync that writes
// registry/metrics for the orchestrator run while any swarm workers exist.
func (r *tuiTurnRunner) ensureSwarmSyncLoop() {
	if r.swarmSyncCancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	r.swarmSyncCancel = cancel
	go func() {
		t := time.NewTicker(2 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				_ = orchestrator.SyncRegistry(r.cfg, r.run.RunId)
			}
		}
	}()
}
