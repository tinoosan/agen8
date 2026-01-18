package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/chzyer/readline"
	"github.com/tinoosan/workbench-core/internal/agent"
	"github.com/tinoosan/workbench-core/internal/config"
	"github.com/tinoosan/workbench-core/internal/cost"
	"github.com/tinoosan/workbench-core/internal/events"
	"github.com/tinoosan/workbench-core/internal/fsutil"
	"github.com/tinoosan/workbench-core/internal/llm"
	"github.com/tinoosan/workbench-core/internal/repl"
	"github.com/tinoosan/workbench-core/internal/resources"
	"github.com/tinoosan/workbench-core/internal/store"
	"github.com/tinoosan/workbench-core/internal/tools"
	"github.com/tinoosan/workbench-core/internal/types"
	"github.com/tinoosan/workbench-core/internal/vfs"
)

func pricingForModel(modelID, pricingFile string) (inPerM, outPerM float64, known bool, source string) {
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return 0, 0, false, ""
	}

	source = "builtin"
	pf := cost.DefaultPricing()
	if strings.TrimSpace(pricingFile) != "" {
		if fromFile, err := cost.LoadPricingFile(pricingFile); err == nil {
			for k, v := range fromFile.Models {
				pf.Models[k] = v
			}
			source = "file"
		}
	}
	inPerM, outPerM, ok := pf.Lookup(modelID)
	if !ok {
		return 0, 0, false, source
	}
	return inPerM, outPerM, true, source
}

// RunChatOptions controls host-side limits and prompt injection behavior for RunChat.
type RunChatOptions struct {
	// Model is the model identifier used for LLM requests.
	//
	// If empty, the host falls back to OPENROUTER_MODEL.
	Model string

	// WorkDir is the host working directory to mount at /workdir.
	//
	// If empty, the host uses os.Getwd() at startup.
	WorkDir string

	// MaxSteps caps how many agent loop steps are allowed per user turn.
	MaxSteps int

	// MaxTraceBytes caps how many trace bytes ContextUpdater will consider per step.
	MaxTraceBytes int
	// MaxMemoryBytes caps how many run-scoped memory bytes are injected per step.
	MaxMemoryBytes int
	// MaxProfileBytes caps how many global profile bytes are injected per step.
	MaxProfileBytes int

	// RecentHistoryPairs is the number of (user,agent) message pairs to include
	// in the "Recent Conversation" block injected into the system prompt.
	RecentHistoryPairs int

	// UserID is an optional stable identifier for the end user.
	// If set, it is recorded into history/events for provenance.
	UserID string

	// IncludeHistoryOps controls whether the constructor includes environment/host ops
	// from /history in addition to user/agent messages.
	IncludeHistoryOps *bool

	// PriceInPerMTokensUSD is the input token price in USD per 1M tokens.
	//
	// If both PriceInPerMTokensUSD and PriceOutPerMTokensUSD are > 0 and the model returns
	// usage metrics, the host will emit a per-turn cost estimate.
	PriceInPerMTokensUSD float64

	// PriceOutPerMTokensUSD is the output token price in USD per 1M tokens.
	PriceOutPerMTokensUSD float64

	// PricingFile is an optional path to a pricing JSON file.
	//
	// This is used by runtime /model switching to recompute per-turn cost estimation.
	// Pricing is still optional: if no entry exists for a model, cost is "unknown".
	PricingFile string
}

func (o RunChatOptions) withDefaults() RunChatOptions {
	if strings.TrimSpace(o.WorkDir) == "" {
		o.WorkDir = strings.TrimSpace(os.Getenv("WORKBENCH_WORKDIR"))
	}
	if o.MaxSteps <= 0 {
		o.MaxSteps = 200
	}
	if o.MaxTraceBytes <= 0 {
		o.MaxTraceBytes = 8 * 1024
	}
	if o.MaxMemoryBytes <= 0 {
		o.MaxMemoryBytes = 8 * 1024
	}
	if o.MaxProfileBytes <= 0 {
		o.MaxProfileBytes = 4 * 1024
	}
	if o.RecentHistoryPairs <= 0 {
		o.RecentHistoryPairs = 8
	}
	if o.IncludeHistoryOps == nil {
		o.IncludeHistoryOps = boolPtr(true)
	}
	if strings.TrimSpace(o.PricingFile) == "" {
		o.PricingFile = strings.TrimSpace(os.Getenv("WORKBENCH_PRICING_FILE"))
	}
	return o
}

func boolPtr(v bool) *bool { return &v }

func derefBool(p *bool, def bool) bool {
	if p == nil {
		return def
	}
	return *p
}

func estimateTurnCostUSD(usage types.LLMUsage, priceInPerM, priceOutPerM float64) float64 {
	if usage.InputTokens <= 0 && usage.OutputTokens <= 0 {
		return 0
	}
	if priceInPerM <= 0 && priceOutPerM <= 0 {
		return 0
	}
	in := float64(usage.InputTokens) / 1_000_000.0 * priceInPerM
	out := float64(usage.OutputTokens) / 1_000_000.0 * priceOutPerM
	return in + out
}

func fmtUSD(v float64) string {
	// Keep it stable and compact; this is an estimate based on token usage.
	return fmt.Sprintf("%.6f", v)
}

// RunChat starts the interactive REPL-driven agent loop for a run.
//
// This is the main "Workbench" experience:
//   - mounts run-scoped resources (/workspace, /trace, /memory)
//   - mounts virtual resources (/results, /tools)
//   - mounts session-scoped history (/history)
//   - starts a readline-based chat session
//
// The CLI (cmd/workbench) decides how runs/sessions are created or resumed.
func RunChat(ctx context.Context, cfg config.Config, run types.Run, opts RunChatOptions) (retErr error) {
	if err := cfg.Validate(); err != nil {
		return err
	}
	opts = opts.withDefaults()

	runCtx, stopSignals := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stopSignals()

	// Ensure the run is transitioned to a terminal state and persisted to run.json.
	//
	// This fixes the current WIP behavior where a run can remain "running" forever if the
	// process exits without calling store.StopRun (e.g. Ctrl-C).
	defer func() {
		status := types.StatusDone
		errMsg := ""
		if runCtx.Err() != nil {
			status = types.StatusCanceled
			errMsg = "interrupted"
		}
		if retErr != nil {
			status = types.StatusFailed
			errMsg = retErr.Error()
		}
		_, _ = store.StopRun(cfg, run.RunId, status, errMsg)
	}()

	log.Printf("== Workbench chat ==")
	log.Printf("sessionId=%s", run.SessionID)
	log.Printf("runId=%s", run.RunId)

	historyRes, err := resources.NewSessionHistoryResource(cfg, run.SessionID)
	if err != nil {
		return fmt.Errorf("create history: %w", err)
	}
	historySink := &events.HistorySink{Store: historyRes.Store}

	emitter := &events.Emitter{
		RunID: run.RunId,
		Sink: events.MultiSink{
			events.ConsoleSink{},
			events.StoreSink{Cfg: cfg},
			historySink,
		},
	}
	mustEmit := func(ctx context.Context, ev events.Event) {
		if err := emitter.Emit(ctx, ev); err != nil {
			log.Fatalf("error emitting event: %v", err)
		}
	}
	boolp := func(b bool) *bool { return &b }

	sessionTitle := ""
	sessionModel := ""
	if sess, err := store.LoadSession(cfg, run.SessionID); err == nil {
		sessionTitle = strings.TrimSpace(sess.Title)
		sessionModel = strings.TrimSpace(sess.ActiveModel)
	}

	model := strings.TrimSpace(opts.Model)
	if model == "" {
		model = sessionModel
	}
	if model == "" {
		model = strings.TrimSpace(os.Getenv("OPENROUTER_MODEL"))
	}
	if strings.TrimSpace(os.Getenv("OPENROUTER_API_KEY")) == "" || model == "" {
		return fmt.Errorf("OPENROUTER_API_KEY and OPENROUTER_MODEL (or --model or session.activeModel) are required to run the non-scripted agent loop")
	}
	opts.Model = model
	historySink.Model = model

	// Resolve pricing against the effective model (session-aware).
	//
	// If pricing is not known for the model, cost is shown as "unknown".
	if opts.PriceInPerMTokensUSD <= 0 && opts.PriceOutPerMTokensUSD <= 0 {
		inPerM, outPerM, known, _ := pricingForModel(model, opts.PricingFile)
		if known {
			opts.PriceInPerMTokensUSD = inPerM
			opts.PriceOutPerMTokensUSD = outPerM
		}
	}

	mustEmit(context.Background(), events.Event{
		Type:    "run.started",
		Message: "Run started",
		Data: map[string]string{
			"sessionId":    run.SessionID,
			"sessionTitle": sessionTitle,
			"runId":        run.RunId,
			"userId":       strings.TrimSpace(opts.UserID),
		},
		Console: boolp(false),
	})

	traceRes, err := resources.NewTraceResource(cfg, run.RunId)
	if err != nil {
		return fmt.Errorf("create trace: %w", err)
	}

	fs := vfs.NewFS()

	workdirAbs, err := resolveWorkDir(opts.WorkDir)
	if err != nil {
		return err
	}
	workdirRes, err := resources.NewWorkdirResource(workdirAbs)
	if err != nil {
		return fmt.Errorf("create workdir: %w", err)
	}

	workspace, err := resources.NewRunWorkspace(cfg, run.RunId)
	if err != nil {
		return fmt.Errorf("create workspace: %w", err)
	}

	// /tools is virtual and does not require a disk directory.
	// If data/tools exists, it is used as an optional provider.
	toolsDir := fsutil.GetToolsDir(cfg.DataDir)
	_ = os.MkdirAll(toolsDir, 0755)

	builtinProvider, err := tools.NewBuiltinManifestProvider()
	if err != nil {
		return fmt.Errorf("load builtin tool manifests: %w", err)
	}
	diskProvider := tools.NewDiskManifestProvider(toolsDir)
	diskProvider.Logf = log.Printf

	toolManifests := tools.NewCompositeToolManifestRegistry(builtinProvider, diskProvider)
	toolManifests.Logf = log.Printf

	toolsResource, err := resources.NewVirtualToolsResource(toolManifests)
	if err != nil {
		return fmt.Errorf("create tools resource: %w", err)
	}

	resultsStore := store.NewInMemoryResultsStore()
	resultsRes, err := resources.NewVirtualResultsResource(resultsStore)
	if err != nil {
		return fmt.Errorf("create results: %w", err)
	}

	memStore, err := store.NewDiskMemoryStore(cfg, run.RunId)
	if err != nil {
		return fmt.Errorf("create memory store: %w", err)
	}
	memoryRes, err := resources.NewVirtualMemoryResource(memStore)
	if err != nil {
		return fmt.Errorf("create memory resource: %w", err)
	}

	profileStore, err := store.NewDiskProfileStore(cfg)
	if err != nil {
		return fmt.Errorf("create profile store: %w", err)
	}
	profileRes, err := resources.NewVirtualProfileResource(profileStore)
	if err != nil {
		return fmt.Errorf("create profile resource: %w", err)
	}

	fs.Mount(vfs.MountWorkspace, workspace)
	log.Printf("mounted /workspace => %s", workspace.BaseDir)
	fs.Mount(vfs.MountWorkdir, workdirRes)
	log.Printf("mounted /workdir => %s", workdirRes.BaseDir)
	fs.Mount(vfs.MountResults, resultsRes)
	log.Printf("mounted /results => (virtual)")
	fs.Mount(vfs.MountTrace, traceRes)
	log.Printf("mounted /trace => %s", traceRes.BaseDir)
	fs.Mount(vfs.MountTools, toolsResource)
	log.Printf("mounted /tools => (virtual; disk provider: %s)", toolsDir)
	fs.Mount(vfs.MountMemory, memoryRes)
	log.Printf("mounted /memory => %s", memoryRes.BaseDir)
	fs.Mount(vfs.MountProfile, profileRes)
	log.Printf("mounted /profile => (global; disk store)")
	fs.Mount(vfs.MountHistory, historyRes)
	log.Printf("mounted /history => %s", historyRes.BaseDir)

	absWorkdirRoot, err := filepath.Abs(workdirRes.BaseDir)
	if err != nil {
		return fmt.Errorf("resolve workdir root: %w", err)
	}

	traceStore := store.DiskTraceStore{Dir: traceRes.BaseDir}
	builtinCfg := tools.BuiltinConfig{
		BashRootDir:    absWorkdirRoot,
		RipgrepRootDir: absWorkdirRoot,
		TraceStore:     traceStore,
	}

	runner := tools.Runner{
		Results:      resultsStore,
		ToolRegistry: tools.BuiltinInvokerRegistry(builtinCfg),
	}

	executor := &agent.HostOpExecutor{
		FS:              fs,
		Runner:          &runner,
		DefaultMaxBytes: 4096,
		MaxReadBytes:    16 * 1024,
	}

	// Persist runtime config for reproducibility/debugging.
	run.Runtime = &types.RunRuntimeConfig{
		DataDir:               cfg.DataDir,
		Model:                 model,
		MaxSteps:              opts.MaxSteps,
		MaxTraceBytes:         opts.MaxTraceBytes,
		MaxMemoryBytes:        opts.MaxMemoryBytes,
		MaxProfileBytes:       opts.MaxProfileBytes,
		RecentHistoryPairs:    opts.RecentHistoryPairs,
		IncludeHistoryOps:     derefBool(opts.IncludeHistoryOps, true),
		PriceInPerMTokensUSD:  opts.PriceInPerMTokensUSD,
		PriceOutPerMTokensUSD: opts.PriceOutPerMTokensUSD,
	}
	_ = store.SaveRun(cfg, run)

	// Persist the active model at the session level so "resume session" is deterministic.
	if sess, err := store.LoadSession(cfg, run.SessionID); err == nil {
		if strings.TrimSpace(sess.ActiveModel) != model {
			sess.ActiveModel = model
			_ = store.SaveSession(cfg, sess)
		}
	}

	client, err := llm.NewOpenRouterClientFromEnv()
	if err != nil {
		return fmt.Errorf("create OpenRouter client: %w", err)
	}

	systemPromptBytes, err := os.ReadFile("internal/agent/INITIAL_PROMPT.md")
	if err != nil {
		return fmt.Errorf("read internal/agent/INITIAL_PROMPT.md: %w", err)
	}
	baseSystemPrompt := string(systemPromptBytes)

	// Constructor builds bounded, auditable context blocks from /profile, /memory, /trace, /history.
	// It persists its state and manifest to /workspace so context assembly is reproducible.
	constructor := &agent.ContextConstructor{
		FS:                fs,
		Cfg:               cfg,
		RunID:             run.RunId,
		SessionID:         run.SessionID,
		TraceStore:        traceStore,
		HistoryStore:      historyRes.Store,
		IncludeHistoryOps: derefBool(opts.IncludeHistoryOps, true),
		MaxProfileBytes:   opts.MaxProfileBytes,
		MaxMemoryBytes:    opts.MaxMemoryBytes,
		MaxTraceBytes:     opts.MaxTraceBytes,
		MaxHistoryBytes:   8 * 1024,
		StatePath:         "/workspace/context_constructor_state.json",
		ManifestPath:      "/workspace/context_constructor_manifest.json",
		Emit: func(eventType, message string, data map[string]string) {
			mustEmit(context.Background(), events.Event{Type: eventType, Message: message, Data: data})
		},
	}

	var updater *agent.ContextUpdater
	execWithEvents := func(ctx context.Context, req types.HostOpRequest) types.HostOpResponse {
		reqData := map[string]string{
			"op":       req.Op,
			"path":     req.Path,
			"toolId":   req.ToolID.String(),
			"actionId": req.ActionID,
		}
		if req.Op == types.HostOpFSRead && req.MaxBytes != 0 {
			reqData["maxBytes"] = strconv.Itoa(req.MaxBytes)
		}
		if req.Op == types.HostOpToolRun && req.TimeoutMs != 0 {
			reqData["timeoutMs"] = strconv.Itoa(req.TimeoutMs)
		}
		if req.Op == types.HostOpToolRun && len(req.Input) != 0 {
			s, tr, n := toolRunInputForEvent(req.Input)
			if s != "" {
				reqData["input"] = s
			}
			if tr {
				reqData["inputTruncated"] = "true"
			}
			if n != 0 {
				reqData["inputBytes"] = strconv.Itoa(n)
			}
		}
		if (req.Op == types.HostOpFSWrite || req.Op == types.HostOpFSAppend) && strings.TrimSpace(req.Text) != "" {
			p, tr, red, n, isJSON := fsWriteTextPreviewForEvent(req.Path, req.Text)
			if p != "" {
				reqData["textPreview"] = p
			}
			if tr {
				reqData["textTruncated"] = "true"
			}
			if red {
				reqData["textRedacted"] = "true"
			}
			if n != 0 {
				reqData["textBytes"] = strconv.Itoa(n)
			}
			if isJSON {
				reqData["textIsJSON"] = "true"
			}
		}

		mustEmit(ctx, events.Event{
			Type:      "agent.op.request",
			Message:   "Agent requested host op",
			Data:      reqData,
			StoreData: map[string]string{"op": req.Op, "path": req.Path, "toolId": req.ToolID.String(), "actionId": req.ActionID},
		})
		resp := executor.Exec(ctx, req)
		if updater != nil {
			updater.ObserveHostOp(req, resp)
		}
		constructor.ObserveHostOp(req, resp)

		respData := map[string]string{
			"op":  resp.Op,
			"ok":  fmtBool(resp.Ok),
			"err": resp.Error,
		}
		if resp.BytesLen != 0 {
			respData["bytesLen"] = strconv.Itoa(resp.BytesLen)
		}
		if resp.Truncated {
			respData["truncated"] = "true"
		}
		if resp.ToolResponse != nil && resp.ToolResponse.CallID != "" {
			respData["callId"] = resp.ToolResponse.CallID
		}
		if resp.Op == types.HostOpToolRun && resp.ToolResponse != nil && len(resp.ToolResponse.Output) != 0 {
			if p := toolRunOutputPreviewForEvent(resp.ToolResponse.ToolID.String(), resp.ToolResponse.ActionID, resp.ToolResponse.Output); strings.TrimSpace(p) != "" {
				respData["outputPreview"] = p
			}
		}
		mustEmit(ctx, events.Event{
			Type:      "agent.op.response",
			Message:   "Host op completed",
			Data:      respData,
			StoreData: map[string]string{"op": resp.Op, "ok": fmtBool(resp.Ok), "err": resp.Error},
		})
		return resp
	}

	mustEmit(context.Background(), events.Event{
		Type:    "agent.loop.start",
		Message: "Agent loop started",
		Data:    map[string]string{"model": model},
	})

	updater = &agent.ContextUpdater{
		FS:              fs,
		TraceStore:      traceStore,
		MaxProfileBytes: opts.MaxProfileBytes,
		MaxMemoryBytes:  opts.MaxMemoryBytes,
		MaxTraceBytes:   opts.MaxTraceBytes,
		ManifestPath:    "/workspace/context_manifest.json",
		Emit: func(eventType, message string, data map[string]string) {
			mustEmit(context.Background(), events.Event{Type: eventType, Message: message, Data: data})
		},
	}

	a, err := agent.New(agent.Config{
		LLM:          client,
		Exec:         agent.HostExecFunc(execWithEvents),
		Model:        model,
		SystemPrompt: baseSystemPrompt,
		Context:      constructor,
		MaxSteps:     opts.MaxSteps,
	})
	if err != nil {
		return err
	}

	log.Printf("== Chat session started (type 'exit' to quit) ==")
	historyPath := filepath.Join(cfg.DataDir, "runs", run.RunId, "repl_history.txt")
	rr, err := repl.NewReader(historyPath)
	if err != nil {
		return fmt.Errorf("start readline: %w", err)
	}
	oldLogWriter := log.Writer()
	log.SetOutput(rr)
	defer rr.Close()
	defer log.SetOutput(oldLogWriter)

	// Print REPL help once at session start (not on every user turn).
	_, _ = io.WriteString(rr, userInputHelp())

	memEval := agent.DefaultMemoryEvaluator()
	profileEval := agent.DefaultProfileEvaluator()
	turn := 0
	var conversation []types.LLMMessage
	for {
		userMsg, exit, err := readUserMessage(rr, rr)
		if err != nil {
			log.Printf("read stdin: %v", err)
			break
		}
		if exit {
			break
		}
		if strings.TrimSpace(userMsg) == "" {
			continue
		}
		if strings.TrimSpace(userMsg) == ":reset" {
			conversation = nil
			mustEmit(context.Background(), events.Event{
				Type:    "chat.reset",
				Message: "Cleared conversation history",
				Store:   boolp(false),
			})
			continue
		}

		// Refresh session state and inject it so the agent stays coherent across runs.
		if sess, err := store.LoadSession(cfg, run.SessionID); err == nil {
			if blk := agent.SessionContextBlock(sess); strings.TrimSpace(blk) != "" {
				a.SystemPrompt = strings.TrimSpace(baseSystemPrompt) + "\n\n" + blk + "\n"
			} else {
				a.SystemPrompt = baseSystemPrompt
			}
		} else {
			a.SystemPrompt = baseSystemPrompt
		}
		// Recent conversation injection is handled by ContextConstructor (via /history).

		turn++
		mustEmit(context.Background(), events.Event{
			Type:    "user.message",
			Message: "User message received",
			Data:    map[string]string{"text": userMsg},
			Console: boolp(false),
		})

		var turnUsage types.LLMUsage
		a.Hooks.OnLLMUsage = func(step int, usage types.LLMUsage) {
			turnUsage.InputTokens += usage.InputTokens
			turnUsage.OutputTokens += usage.OutputTokens
			turnUsage.TotalTokens += usage.TotalTokens
			mustEmit(context.Background(), events.Event{
				Type:    "llm.usage",
				Message: "Model usage",
				Data: map[string]string{
					"step":      strconv.Itoa(step),
					"input":     strconv.Itoa(usage.InputTokens),
					"output":    strconv.Itoa(usage.OutputTokens),
					"total":     strconv.Itoa(usage.TotalTokens),
					"turnTotal": strconv.Itoa(turnUsage.TotalTokens),
				},
				Store: boolp(false),
			})
		}

		conversation = append(conversation, types.LLMMessage{Role: "user", Content: userMsg})
		start := time.Now()
		final, updated, steps, err := a.RunConversation(runCtx, conversation)
		dur := time.Since(start)
		conversation = updated
		if err != nil {
			mustEmit(context.Background(), events.Event{
				Type:    "agent.error",
				Message: "Agent loop error",
				Data:    map[string]string{"err": err.Error()},
				Store:   boolp(false),
			})
			log.Printf("agent error: %v", err)
			continue
		}
		mustEmit(context.Background(), events.Event{
			Type:    "agent.turn.complete",
			Message: "Agent completed user request",
			Data: map[string]string{
				"turn":       strconv.Itoa(turn),
				"steps":      strconv.Itoa(steps),
				"durationMs": strconv.FormatInt(dur.Milliseconds(), 10),
				"duration":   dur.Truncate(time.Millisecond).String(),
			},
			Store: boolp(false),
		})
		if turnUsage.TotalTokens != 0 {
			mustEmit(context.Background(), events.Event{
				Type:    "llm.usage.total",
				Message: "Turn usage total",
				Data: map[string]string{
					"input":  strconv.Itoa(turnUsage.InputTokens),
					"output": strconv.Itoa(turnUsage.OutputTokens),
					"total":  strconv.Itoa(turnUsage.TotalTokens),
				},
				Store: boolp(false),
			})
			if cost := estimateTurnCostUSD(turnUsage, opts.PriceInPerMTokensUSD, opts.PriceOutPerMTokensUSD); cost > 0 {
				mustEmit(context.Background(), events.Event{
					Type:    "llm.cost.total",
					Message: "Turn cost estimate",
					Data: map[string]string{
						"turn":          strconv.Itoa(turn),
						"input":         strconv.Itoa(turnUsage.InputTokens),
						"output":        strconv.Itoa(turnUsage.OutputTokens),
						"total":         strconv.Itoa(turnUsage.TotalTokens),
						"costUsd":       fmtUSD(cost),
						"priceInPerM":   fmtUSD(opts.PriceInPerMTokensUSD),
						"priceOutPerM":  fmtUSD(opts.PriceOutPerMTokensUSD),
						"pricingSource": "host_config",
					},
					Store: boolp(false),
				})
			}
		}

		// Ingest memory update if the agent wrote one.
		if b, err := fs.Read("/memory/update.md"); err == nil {
			updateRaw := string(b)
			if strings.TrimSpace(updateRaw) == "" {
				mustEmit(context.Background(), events.Event{
					Type:    "memory.evaluate",
					Message: "No memory update written",
					Data: map[string]string{
						"turn":     strconv.Itoa(turn),
						"accepted": "false",
						"reason":   "no_update",
						"bytes":    "0",
					},
				})
			} else {
				trimmed := strings.TrimSpace(updateRaw)
				hash := agent.SHA256Hex(trimmed)

				accepted, reason, cleaned := memEval.Evaluate(updateRaw)

				mustEmit(context.Background(), events.Event{
					Type:    "memory.evaluate",
					Message: "Evaluated memory update",
					Data: map[string]string{
						"turn":     strconv.Itoa(turn),
						"accepted": fmtBool(accepted),
						"reason":   reason,
						"bytes":    strconv.Itoa(len(trimmed)),
						"sha256":   hash[:12],
					},
				})

				if accepted {
					if err := memStore.AppendMemory(context.Background(), formatRunMemoryAppend(strings.TrimSpace(cleaned))); err != nil {
						mustEmit(context.Background(), events.Event{
							Type:    "memory.commit.error",
							Message: "Failed to commit memory update",
							Data:    map[string]string{"err": err.Error()},
							Store:   boolp(false),
						})
					} else {
						mustEmit(context.Background(), events.Event{
							Type:    "memory.commit",
							Message: "Committed memory update",
							Data: map[string]string{
								"turn":   strconv.Itoa(turn),
								"bytes":  strconv.Itoa(len(strings.TrimSpace(cleaned))),
								"sha256": hash[:12],
							},
						})
					}
				}

				if err := memStore.AppendCommitLog(context.Background(), types.MemoryCommitLine{
					Scope:     "memory",
					SessionID: run.SessionID,
					RunID:     run.RunId,
					Model:     model,
					Turn:      turn,
					Accepted:  accepted,
					Reason:    reason,
					Bytes:     len(trimmed),
					SHA256:    hash,
				}); err != nil {
					mustEmit(context.Background(), events.Event{
						Type:    "memory.audit.error",
						Message: "Failed to append memory audit log",
						Data: map[string]string{
							"turn": strconv.Itoa(turn),
							"err":  err.Error(),
						},
						Store: boolp(false),
					})
				} else {
					mustEmit(context.Background(), events.Event{
						Type:    "memory.audit.append",
						Message: "Appended memory audit log",
						Data: map[string]string{
							"turn":     strconv.Itoa(turn),
							"accepted": fmtBool(accepted),
							"reason":   reason,
							"sha256":   hash[:12],
						},
						Store: boolp(false),
					})
				}
			}

			_ = fs.Write("/memory/update.md", []byte{})
		}

		// Ingest profile update if the agent wrote one.
		if b, err := fs.Read("/profile/update.md"); err == nil {
			updateRaw := string(b)
			if strings.TrimSpace(updateRaw) == "" {
				mustEmit(context.Background(), events.Event{
					Type:    "profile.evaluate",
					Message: "No profile update written",
					Data: map[string]string{
						"turn":     strconv.Itoa(turn),
						"accepted": "false",
						"reason":   "no_update",
						"bytes":    "0",
					},
				})
			} else {
				trimmed := strings.TrimSpace(updateRaw)
				hash := agent.SHA256Hex(trimmed)

				accepted, reason, cleaned := profileEval.Evaluate(updateRaw)

				mustEmit(context.Background(), events.Event{
					Type:    "profile.evaluate",
					Message: "Evaluated profile update",
					Data: map[string]string{
						"turn":     strconv.Itoa(turn),
						"accepted": fmtBool(accepted),
						"reason":   reason,
						"bytes":    strconv.Itoa(len(trimmed)),
						"sha256":   hash[:12],
					},
				})

				if accepted {
					if err := profileStore.AppendProfile(context.Background(), formatRunMemoryAppend(strings.TrimSpace(cleaned))); err != nil {
						mustEmit(context.Background(), events.Event{
							Type:    "profile.commit.error",
							Message: "Failed to commit profile update",
							Data:    map[string]string{"err": err.Error()},
							Store:   boolp(false),
						})
					} else {
						mustEmit(context.Background(), events.Event{
							Type:    "profile.commit",
							Message: "Committed profile update",
							Data: map[string]string{
								"turn":   strconv.Itoa(turn),
								"bytes":  strconv.Itoa(len(strings.TrimSpace(cleaned))),
								"sha256": hash[:12],
							},
						})
					}
				}

				if err := profileStore.AppendCommitLog(context.Background(), types.MemoryCommitLine{
					Scope:     "profile",
					SessionID: run.SessionID,
					RunID:     run.RunId,
					Model:     model,
					Turn:      turn,
					Accepted:  accepted,
					Reason:    reason,
					Bytes:     len(trimmed),
					SHA256:    hash,
				}); err != nil {
					mustEmit(context.Background(), events.Event{
						Type:    "profile.audit.error",
						Message: "Failed to append profile audit log",
						Data: map[string]string{
							"turn": strconv.Itoa(turn),
							"err":  err.Error(),
						},
						Store: boolp(false),
					})
				} else {
					mustEmit(context.Background(), events.Event{
						Type:    "profile.audit.append",
						Message: "Appended profile audit log",
						Data: map[string]string{
							"turn":     strconv.Itoa(turn),
							"accepted": fmtBool(accepted),
							"reason":   reason,
							"sha256":   hash[:12],
						},
						Store: boolp(false),
					})
				}
			}

			_ = fs.Write("/profile/update.md", []byte{})
		}

		if _, err := store.RecordTurnInSession(cfg, run.SessionID, run.RunId, userMsg, final); err != nil {
			mustEmit(context.Background(), events.Event{
				Type:    "session.update.error",
				Message: "Failed to update session state",
				Data:    map[string]string{"err": err.Error()},
				Store:   boolp(false),
			})
		} else {
			mustEmit(context.Background(), events.Event{
				Type:    "session.update",
				Message: "Updated session state",
				Data:    map[string]string{"sessionId": run.SessionID, "runId": run.RunId},
				Store:   boolp(false),
			})
		}

		rr.Printf("agent> %s\n", final)
		mustEmit(context.Background(), events.Event{
			Type:    "agent.final",
			Message: "Agent produced final answer",
			Data:    map[string]string{"text": final},
			Console: boolp(false),
		})
	}

	mustEmit(context.Background(), events.Event{
		Type:    "run.completed",
		Message: "Run finished",
		Data: map[string]string{
			"sessionId": run.SessionID,
			"runId":     run.RunId,
		},
		Console: boolp(false),
	})

	log.Printf("== Workbench complete ==")
	return nil
}

// Note: recent conversation injection is handled by agent.ContextConstructor.

func fmtBool(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// formatRunMemoryAppend produces the exact block appended to memory.md when a memory update
// is accepted by the host.
func formatRunMemoryAppend(update string) string {
	update = strings.TrimSpace(update)
	if update == "" {
		return ""
	}
	return "\n\n—\n" + time.Now().UTC().Format(time.RFC3339Nano) + "\n\n" + update + "\n"
}

type lineReader interface {
	ReadLine(prompt string) (string, error)
}

const (
	userPrompt         = "you> "
	continuationPrompt = "...> "
)

func readUserMessage(lr lineReader, out io.Writer) (msg string, exit bool, err error) {
	line, err := lr.ReadLine(userPrompt)
	if err != nil && !errors.Is(err, io.EOF) {
		if errors.Is(err, readline.ErrInterrupt) {
			return "", true, nil
		}
		return "", false, err
	}
	if errors.Is(err, io.EOF) {
		return "", true, nil
	}

	line = strings.ReplaceAll(line, "\r", "")
	line = strings.TrimRight(line, "\n")

	trim := strings.TrimSpace(line)
	switch trim {
	case "exit", "quit":
		return "", true, nil
	case ":paste":
		msg, exit, err := readMultilinePaste(lr, out)
		if err != nil || exit {
			return "", exit, err
		}
		edited, err := maybeEditMessage(lr, out, msg)
		return edited, false, err
	case ":compose":
		edited, err := maybeEditMessage(lr, out, "")
		return edited, false, err
	default:
		// If the terminal paste bracket leaked into the input, strip it.
		line = strings.TrimPrefix(line, "\x1b[200~")
		line = strings.TrimPrefix(line, "\x1b[201~")
		return line, false, nil
	}
}

func userInputHelp() string {
	return strings.TrimSpace(`
Commands:
  - exit / quit: exit the chat session
  - :reset: clear in-process conversation (does not delete /history)
  - :paste: multi-line paste mode (end with a line containing only ".")
  - :compose: open $EDITOR/$VISUAL to compose a message

Notes:
  - Workbench uses a REPL; each submitted message becomes one agent turn.
  - For long or multi-line messages, use :paste or :compose.
`) + "\n\n"
}

func readMultilinePaste(lr lineReader, out io.Writer) (msg string, exit bool, err error) {
	_, _ = io.WriteString(out, "paste mode (end with a line containing only \".\"; :abort cancels)\n")
	var b strings.Builder
	for {
		line, readErr := lr.ReadLine(continuationPrompt)
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			return "", false, readErr
		}
		if errors.Is(readErr, io.EOF) && line == "" {
			return "", true, nil
		}
		line = strings.ReplaceAll(line, "\r", "")
		line = strings.TrimRight(line, "\n")
		trim := strings.TrimSpace(line)
		switch trim {
		case ".":
			return b.String(), false, nil
		case ":abort":
			return "", false, nil
		case "exit", "quit":
			return "", true, nil
		default:
			b.WriteString(line)
			b.WriteString("\n")
		}
	}
}

func maybeEditMessage(lr lineReader, out io.Writer, initial string) (string, error) {
	edited, err := editMessageInEditor(initial)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(edited) == "" {
		return "", nil
	}
	ok, err := confirmSend(lr, out)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", nil
	}
	return edited, nil
}

func confirmSend(lr lineReader, out io.Writer) (bool, error) {
	_, _ = io.WriteString(out, "press Enter to send (or type 'abort' to cancel)\n")
	line, err := lr.ReadLine("send> ")
	if err != nil && !errors.Is(err, io.EOF) {
		return false, err
	}
	if strings.TrimSpace(line) == "abort" {
		return false, nil
	}
	return true, nil
}

func editMessageInEditor(initial string) (string, error) {
	editor := strings.TrimSpace(os.Getenv("VISUAL"))
	if editor == "" {
		editor = strings.TrimSpace(os.Getenv("EDITOR"))
	}
	if editor == "" {
		return initial, nil
	}

	tmp, err := os.CreateTemp("", "workbench-message-*.md")
	if err != nil {
		return "", err
	}
	name := tmp.Name()
	_ = os.Chmod(name, 0600)
	if _, err := tmp.WriteString(initial); err != nil {
		_ = tmp.Close()
		_ = os.Remove(name)
		return "", err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(name)
		return "", err
	}
	defer os.Remove(name)

	fields := strings.Fields(editor)
	if len(fields) == 0 {
		return initial, nil
	}
	cmd := exec.Command(fields[0], append(fields[1:], name)...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", err
	}

	b, err := os.ReadFile(name)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
