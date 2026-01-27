package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/tinoosan/workbench-core/internal/store"
	"github.com/tinoosan/workbench-core/pkg/agent"
	"github.com/tinoosan/workbench-core/pkg/config"
	"github.com/tinoosan/workbench-core/pkg/events"
	"github.com/tinoosan/workbench-core/pkg/fsutil"
	"github.com/tinoosan/workbench-core/pkg/llm"
	"github.com/tinoosan/workbench-core/pkg/resources"
	"github.com/tinoosan/workbench-core/pkg/runtime"
	pkgstore "github.com/tinoosan/workbench-core/pkg/store"
	"github.com/tinoosan/workbench-core/pkg/tools"
	"github.com/tinoosan/workbench-core/pkg/types"
	"github.com/tinoosan/workbench-core/pkg/vfs"
)

type tuiChatSetup struct {
	FS *vfs.FS

	Agent            *agent.Agent
	BaseSystemPrompt string
	Constructor      *agent.ContextConstructor

	Artifacts *ArtifactIndex

	WorkdirBase string

	MemStore     pkgstore.MemoryCommitter
	ProfileStore pkgstore.ProfileCommitter

	// BuiltinInvokers is the in-memory registry used by tool.run for builtins.
	// It is a map (reference type), so updating entries updates runner behavior.
	BuiltinInvokers tools.MapRegistry
}

// setupTUIChatRuntime performs the core runtime setup for a TUI-driven run:
// mounting VFS resources, wiring tools/executor, creating context constructor/updater,
// and instantiating the agent.
//
// Event emission and model selection happen at the call site; this function assumes
// model is already resolved/validated and will persist it into run/session state.
func setupTUIChatRuntime(
	cfg config.Config,
	run types.Run,
	opts RunChatOptions,
	model string,
	historyRes *resources.HistoryResource,
	mustEmit func(ctx context.Context, ev events.Event),
) (*tuiChatSetup, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if historyRes == nil {
		return nil, fmt.Errorf("history resource is required")
	}
	model = strings.TrimSpace(model)
	if model == "" {
		return nil, fmt.Errorf("model is required")
	}

	artifactIndex := newArtifactIndex()
	workdirAbs, err := resolveWorkDir(opts.WorkDir)
	if err != nil {
		return nil, err
	}

	resultsStore := store.NewInMemoryResultsStore()
	memStore, err := store.NewDiskMemoryStore(cfg, run.RunId)
	if err != nil {
		return nil, err
	}
	profileStore, err := store.NewDiskProfileStore(cfg)
	if err != nil {
		return nil, err
	}
	traceStore := store.DiskTraceStore{DiskStore: store.DiskStore{Dir: fsutil.GetLogDir(cfg.DataDir, run.RunId)}}

	historyStore, ok := historyRes.Appender.(pkgstore.HistoryStore)
	if !ok {
		return nil, fmt.Errorf("history store must implement HistoryStore")
	}
	constructorStore, err := store.NewSQLiteConstructorStore(cfg)
	if err != nil {
		return nil, err
	}

	rt, err := runtime.Build(runtime.BuildConfig{
		Cfg:                   cfg,
		Run:                   run,
		WorkdirAbs:            workdirAbs,
		Model:                 model,
		ReasoningEffort:       strings.TrimSpace(opts.ReasoningEffort),
		ReasoningSummary:      strings.TrimSpace(opts.ReasoningSummary),
		ApprovalsMode:         strings.TrimSpace(opts.ApprovalsMode),
		SelectedSkill:         strings.TrimSpace(opts.SelectedSkill),
		PlanMode:              opts.PlanMode,
		HistoryStore:          historyStore,
		ResultsStore:          resultsStore,
		MemoryStore:           memStore,
		ProfileStore:          profileStore,
		TraceStore:            &traceStore,
		ConstructorStore:      constructorStore,
		Emit:                  mustEmit,
		IncludeHistoryOps:     derefBool(opts.IncludeHistoryOps, true),
		RecentHistoryPairs:    opts.RecentHistoryPairs,
		MaxProfileBytes:       opts.MaxProfileBytes,
		MaxMemoryBytes:        opts.MaxMemoryBytes,
		MaxTraceBytes:         opts.MaxTraceBytes,
		PriceInPerMTokensUSD:  opts.PriceInPerMTokensUSD,
		PriceOutPerMTokensUSD: opts.PriceOutPerMTokensUSD,
		Guard: func(fs *vfs.FS, req types.HostOpRequest) *types.HostOpResponse {
			if !opts.PlanMode {
				return nil
			}
			return enforcePlanChecklist(fs, req)
		},
		ArtifactObserve: artifactIndex.ObserveWrite,
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
		return nil, err
	}
	fs := rt.FS

	client, err := llm.NewClientFromEnv()
	if err != nil {
		return nil, fmt.Errorf("create LLM client: %w", err)
	}
	// Add resilience against transient provider/network failures.
	llmClient := llm.NewRetryClient(client, llm.RetryConfig{
		MaxRetries:   3,
		InitialDelay: 250 * time.Millisecond,
		MaxDelay:     4 * time.Second,
		Multiplier:   2.0,
	})

	// Use the default system prompt embedded in the agent package.
	baseSystemPrompt := ""

	constructor := rt.Constructor

	mustEmit(context.Background(), events.Event{
		Type:    "agent.loop.start",
		Message: "Agent loop started",
		Data:    map[string]string{"model": model},
	})

	a, err := agent.New(agent.Config{
		LLM:              llmClient,
		Exec:             rt.Executor,
		Model:            model,
		ReasoningEffort:  strings.TrimSpace(opts.ReasoningEffort),
		ReasoningSummary: strings.TrimSpace(opts.ReasoningSummary),
		ApprovalsMode:    strings.TrimSpace(opts.ApprovalsMode),
		EnableWebSearch:  opts.WebSearchEnabled,
		PlanMode:         opts.PlanMode,
		SystemPrompt:     baseSystemPrompt,
		Context:          constructor,

		ToolManifests: rt.ToolManifests,
	})
	if err != nil {
		return nil, err
	}

	return &tuiChatSetup{
		FS:               fs,
		Agent:            a,
		BaseSystemPrompt: baseSystemPrompt,
		Constructor:      constructor,
		Artifacts:        artifactIndex,
		WorkdirBase:      rt.WorkdirBase,
		MemStore:         rt.MemStore,
		ProfileStore:     rt.ProfileStore,
		BuiltinInvokers:  rt.BuiltinInvokers,
	}, nil
}
