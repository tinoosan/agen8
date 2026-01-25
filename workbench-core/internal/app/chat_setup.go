package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/tinoosan/workbench-core/internal/config"
	"github.com/tinoosan/workbench-core/pkg/events"
	"github.com/tinoosan/workbench-core/pkg/llm"
	"github.com/tinoosan/workbench-core/internal/resources"
	"github.com/tinoosan/workbench-core/internal/store"
	"github.com/tinoosan/workbench-core/internal/types"
	"github.com/tinoosan/workbench-core/pkg/vfs"
	"github.com/tinoosan/workbench-core/pkg/agent"
	"github.com/tinoosan/workbench-core/pkg/runtime"
	"github.com/tinoosan/workbench-core/pkg/tools"
)

type tuiChatSetup struct {
	FS *vfs.FS

	Agent            *agent.Agent
	BaseSystemPrompt string
	Constructor      *agent.ContextConstructor

	Artifacts *ArtifactIndex

	WorkdirBase string

	MemStore     store.MemoryCommitter
	ProfileStore store.ProfileCommitter

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

	rt, err := runtime.Build(runtime.BuildConfig{
		Cfg:               cfg,
		Run:               run,
		WorkdirAbs:        workdirAbs,
		Model:             model,
		HistoryRes:        historyRes,
		Emit:              mustEmit,
		IncludeHistoryOps: derefBool(opts.IncludeHistoryOps, true),
		MaxProfileBytes:   opts.MaxProfileBytes,
		MaxMemoryBytes:    opts.MaxMemoryBytes,
		MaxTraceBytes:     opts.MaxTraceBytes,
		Guard: func(fs *vfs.FS, req types.HostOpRequest) *types.HostOpResponse {
			return enforcePlanChecklist(fs, req)
		},
		ArtifactObserve: artifactIndex.ObserveWrite,
	})
	if err != nil {
		return nil, err
	}
	fs := rt.FS

	run.Runtime = &types.RunRuntimeConfig{
		DataDir:          cfg.DataDir,
		Model:            model,
		ReasoningEffort:  strings.TrimSpace(opts.ReasoningEffort),
		ReasoningSummary: strings.TrimSpace(opts.ReasoningSummary),
		ApprovalsMode:    strings.TrimSpace(opts.ApprovalsMode),
		PlanMode:         opts.PlanMode,

		MaxTraceBytes:         opts.MaxTraceBytes,
		MaxMemoryBytes:        opts.MaxMemoryBytes,
		MaxProfileBytes:       opts.MaxProfileBytes,
		RecentHistoryPairs:    opts.RecentHistoryPairs,
		IncludeHistoryOps:     derefBool(opts.IncludeHistoryOps, true),
		PriceInPerMTokensUSD:  opts.PriceInPerMTokensUSD,
		PriceOutPerMTokensUSD: opts.PriceOutPerMTokensUSD,
	}
	_ = store.SaveRun(cfg, run)

	// Persist the active model at the session level so resume is deterministic.
	if sess, err := store.LoadSession(cfg, run.SessionID); err == nil {
		changed := false
		if strings.TrimSpace(sess.ActiveModel) != model {
			sess.ActiveModel = model
			changed = true
		}
		if strings.TrimSpace(sess.ReasoningEffort) != strings.TrimSpace(opts.ReasoningEffort) {
			sess.ReasoningEffort = strings.TrimSpace(opts.ReasoningEffort)
			changed = true
		}
		if strings.TrimSpace(sess.ReasoningSummary) != strings.TrimSpace(opts.ReasoningSummary) {
			sess.ReasoningSummary = strings.TrimSpace(opts.ReasoningSummary)
			changed = true
		}
		approvalMode := strings.TrimSpace(opts.ApprovalsMode)
		if approvalMode == "" {
			approvalMode = "enabled"
		}
		if strings.TrimSpace(sess.ApprovalsMode) != approvalMode {
			sess.ApprovalsMode = approvalMode
			changed = true
		}
		if sess.PlanMode == nil || *sess.PlanMode != opts.PlanMode {
			nextPlanMode := opts.PlanMode
			sess.PlanMode = &nextPlanMode
			changed = true
		}
		if changed {
			_ = store.SaveSession(cfg, sess)
		}
	}

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

