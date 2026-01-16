package app

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/tinoosan/workbench-core/internal/agent"
	"github.com/tinoosan/workbench-core/internal/config"
	"github.com/tinoosan/workbench-core/internal/events"
	"github.com/tinoosan/workbench-core/internal/fsutil"
	"github.com/tinoosan/workbench-core/internal/llm"
	"github.com/tinoosan/workbench-core/internal/resources"
	"github.com/tinoosan/workbench-core/internal/store"
	"github.com/tinoosan/workbench-core/internal/tools"
	"github.com/tinoosan/workbench-core/internal/tui"
	"github.com/tinoosan/workbench-core/internal/types"
	"github.com/tinoosan/workbench-core/internal/vfs"
)

// RunChatTUI starts the interactive Workbench chat session using a Bubble Tea UI.
//
// The TUI renders a single integrated timeline containing:
//   - user messages
//   - host events (op requests/responses, context updates, commits, etc)
//   - agent final responses
//
// The underlying agent loop contract, tool execution model, and store policies are
// unchanged. Only presentation and input handling differ from RunChat's REPL.
func RunChatTUI(ctx context.Context, run types.Run, opts RunChatOptions) (retErr error) {
	opts = opts.withDefaults()

	// The TUI owns stdout/stderr. Avoid mixing standard log output into the screen.
	oldLogWriter := log.Writer()
	log.SetOutput(io.Discard)
	defer log.SetOutput(oldLogWriter)

	runCtx, stopSignals := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stopSignals()

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
		_, _ = store.StopRun(run.RunId, status, errMsg)
	}()

	historyRes, err := resources.NewSessionHistoryResource(run.SessionID)
	if err != nil {
		return fmt.Errorf("create history: %w", err)
	}
	historySink := &events.HistorySink{Store: historyRes.Store}

	// Stream events into the TUI, while still persisting them to the run event log
	// and session history.
	evCh := make(chan events.Event, 2048)
	emitter := &events.Emitter{
		RunID: run.RunId,
		Sink: events.MultiSink{
			events.StoreSink{},
			historySink,
			tui.EventSink{Ch: evCh},
		},
	}
	mustEmit := func(ctx context.Context, ev events.Event) {
		if err := emitter.Emit(ctx, ev); err != nil {
			// In the TUI we can't safely print. Fail fast; this indicates a host bug.
			panic(fmt.Errorf("emit event: %w", err))
		}
	}
	boolp := func(b bool) *bool { return &b }

	sessionTitle := ""
	if sess, err := store.LoadSession(run.SessionID); err == nil {
		sessionTitle = strings.TrimSpace(sess.Title)
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

	traceRes, err := resources.NewTraceResource(run.RunId)
	if err != nil {
		return fmt.Errorf("create trace: %w", err)
	}

	fs := vfs.NewFS()

	workspace, err := resources.NewRunWorkspace(run.RunId)
	if err != nil {
		return fmt.Errorf("create workspace: %w", err)
	}

	toolsDir := fsutil.GetToolsDir(config.DataDir)
	_ = os.MkdirAll(toolsDir, 0755)

	builtinProvider, err := tools.NewBuiltinManifestProvider()
	if err != nil {
		return fmt.Errorf("load builtin tool manifests: %w", err)
	}
	diskProvider := tools.NewDiskManifestProvider(toolsDir)
	toolManifests := tools.NewCompositeToolManifestRegistry(builtinProvider, diskProvider)

	toolsResource, err := resources.NewVirtualToolsResource(toolManifests)
	if err != nil {
		return fmt.Errorf("create tools resource: %w", err)
	}

	resultsStore := store.NewInMemoryResultsStore()
	resultsRes, err := resources.NewVirtualResultsResource(resultsStore)
	if err != nil {
		return fmt.Errorf("create results: %w", err)
	}

	memStore, err := store.NewDiskMemoryStore(run.RunId)
	if err != nil {
		return fmt.Errorf("create memory store: %w", err)
	}
	memoryRes, err := resources.NewVirtualMemoryResource(memStore)
	if err != nil {
		return fmt.Errorf("create memory resource: %w", err)
	}

	profileStore, err := store.NewDiskProfileStore()
	if err != nil {
		return fmt.Errorf("create profile store: %w", err)
	}
	profileRes, err := resources.NewVirtualProfileResource(profileStore)
	if err != nil {
		return fmt.Errorf("create profile resource: %w", err)
	}

	fs.Mount(vfs.MountWorkspace, workspace)
	fs.Mount(vfs.MountResults, resultsRes)
	fs.Mount(vfs.MountTrace, traceRes)
	fs.Mount(vfs.MountTools, toolsResource)
	fs.Mount(vfs.MountMemory, memoryRes)
	fs.Mount(vfs.MountProfile, profileRes)
	fs.Mount(vfs.MountHistory, historyRes)

	mustEmit(context.Background(), events.Event{
		Type:    "host.mounted",
		Message: "Mounted VFS resources",
		Data: map[string]string{
			"/workspace": workspace.BaseDir,
			"/results":   "(virtual)",
			"/trace":     traceRes.BaseDir,
			"/tools":     "(virtual)",
			"/memory":    memoryRes.BaseDir,
			"/profile":   "(global)",
			"/history":   historyRes.BaseDir,
		},
		Console: boolp(false),
	})

	absWorkspaceRoot, err := filepath.Abs(workspace.BaseDir)
	if err != nil {
		return fmt.Errorf("resolve workspace root: %w", err)
	}

	traceStore := store.DiskTraceStore{Dir: traceRes.BaseDir}
	builtinCfg := tools.BuiltinConfig{
		BashRootDir: absWorkspaceRoot,
		TraceStore:  traceStore,
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

	model := strings.TrimSpace(os.Getenv("OPENROUTER_MODEL"))
	if strings.TrimSpace(os.Getenv("OPENROUTER_API_KEY")) == "" || model == "" {
		return fmt.Errorf("OPENROUTER_API_KEY and OPENROUTER_MODEL are required")
	}
	historySink.Model = model

	run.Runtime = &types.RunRuntimeConfig{
		DataDir:               config.DataDir,
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
	_ = store.SaveRun(run)

	client, err := llm.NewOpenRouterClientFromEnv()
	if err != nil {
		return fmt.Errorf("create OpenRouter client: %w", err)
	}

	systemPromptBytes, err := os.ReadFile("internal/agent/INITIAL_PROMPT.md")
	if err != nil {
		return fmt.Errorf("read internal/agent/INITIAL_PROMPT.md: %w", err)
	}
	baseSystemPrompt := string(systemPromptBytes)

	constructor := &agent.ContextConstructor{
		FS:                fs,
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

	engine := &tuiTurnRunner{
		ctx:              runCtx,
		run:              run,
		opts:             opts,
		fs:               fs,
		agent:            a,
		baseSystemPrompt: baseSystemPrompt,
		mustEmit:         mustEmit,
		memStore:         memStore,
		profileStore:     profileStore,
		memEval:          agent.DefaultMemoryEvaluator(),
		profileEval:      agent.DefaultProfileEvaluator(),
		model:            model,
	}

	err = tui.Run(runCtx, engine, evCh)
	close(evCh)

	mustEmit(context.Background(), events.Event{
		Type:    "run.completed",
		Message: "Run finished",
		Data: map[string]string{
			"sessionId": run.SessionID,
			"runId":     run.RunId,
		},
		Console: boolp(false),
	})
	return err
}

type tuiTurnRunner struct {
	ctx context.Context

	run  types.Run
	opts RunChatOptions

	fs *vfs.FS

	agent            *agent.Agent
	baseSystemPrompt string
	model            string

	mustEmit func(ctx context.Context, ev events.Event)

	memStore     store.MemoryStore
	profileStore store.ProfileStore
	memEval      *agent.MemoryEvaluator
	profileEval  *agent.ProfileEvaluator

	turn         int
	conversation []types.LLMMessage
}

func (r *tuiTurnRunner) RunTurn(ctx context.Context, userMsg string) (string, error) {
	if r == nil {
		return "", fmt.Errorf("runner is nil")
	}
	if strings.TrimSpace(userMsg) == "" {
		return "", nil
	}
	boolp := func(b bool) *bool { return &b }

	if strings.TrimSpace(userMsg) == ":reset" {
		r.conversation = nil
		r.mustEmit(context.Background(), events.Event{
			Type:    "chat.reset",
			Message: "Cleared conversation history",
			Store:   boolp(false),
		})
		return "done", nil
	}

	// Refresh session state and inject it so the agent stays coherent across runs.
	if sess, err := store.LoadSession(r.run.SessionID); err == nil {
		if blk := agent.SessionContextBlock(sess); strings.TrimSpace(blk) != "" {
			r.agent.SystemPrompt = strings.TrimSpace(r.baseSystemPrompt) + "\n\n" + blk + "\n"
		} else {
			r.agent.SystemPrompt = r.baseSystemPrompt
		}
	} else {
		r.agent.SystemPrompt = r.baseSystemPrompt
	}

	r.turn++
	r.mustEmit(context.Background(), events.Event{
		Type:    "user.message",
		Message: "User message received",
		Data:    map[string]string{"text": userMsg},
		Console: boolp(false),
	})

	var turnUsage types.LLMUsage
	r.agent.Hooks.OnLLMUsage = func(step int, usage types.LLMUsage) {
		turnUsage.InputTokens += usage.InputTokens
		turnUsage.OutputTokens += usage.OutputTokens
		turnUsage.TotalTokens += usage.TotalTokens
		r.mustEmit(context.Background(), events.Event{
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

	r.conversation = append(r.conversation, types.LLMMessage{Role: "user", Content: userMsg})
	start := time.Now()
	final, updated, steps, err := r.agent.RunConversation(ctx, r.conversation)
	dur := time.Since(start)
	r.conversation = updated
	if err != nil {
		r.mustEmit(context.Background(), events.Event{
			Type:    "agent.error",
			Message: "Agent loop error",
			Data:    map[string]string{"err": err.Error()},
			Store:   boolp(false),
		})
		return "", err
	}

	r.mustEmit(context.Background(), events.Event{
		Type:    "agent.turn.complete",
		Message: "Agent completed user request",
		Data: map[string]string{
			"turn":       strconv.Itoa(r.turn),
			"steps":      strconv.Itoa(steps),
			"durationMs": strconv.FormatInt(dur.Milliseconds(), 10),
			"duration":   dur.Truncate(time.Millisecond).String(),
		},
		Store: boolp(false),
	})

	if turnUsage.TotalTokens != 0 {
		r.mustEmit(context.Background(), events.Event{
			Type:    "llm.usage.total",
			Message: "Turn usage total",
			Data: map[string]string{
				"input":  strconv.Itoa(turnUsage.InputTokens),
				"output": strconv.Itoa(turnUsage.OutputTokens),
				"total":  strconv.Itoa(turnUsage.TotalTokens),
			},
			Store: boolp(false),
		})
		if cost := estimateTurnCostUSD(turnUsage, r.opts.PriceInPerMTokensUSD, r.opts.PriceOutPerMTokensUSD); cost > 0 {
			r.mustEmit(context.Background(), events.Event{
				Type:    "llm.cost.total",
				Message: "Turn cost estimate",
				Data: map[string]string{
					"turn":          strconv.Itoa(r.turn),
					"input":         strconv.Itoa(turnUsage.InputTokens),
					"output":        strconv.Itoa(turnUsage.OutputTokens),
					"total":         strconv.Itoa(turnUsage.TotalTokens),
					"costUsd":       fmtUSD(cost),
					"priceInPerM":   fmtUSD(r.opts.PriceInPerMTokensUSD),
					"priceOutPerM":  fmtUSD(r.opts.PriceOutPerMTokensUSD),
					"pricingSource": "host_config",
				},
				Store: boolp(false),
			})
		}
	}

	// Memory update ingestion (run-scoped).
	if b, err := r.fs.Read("/memory/update.md"); err == nil {
		updateRaw := string(b)
		if strings.TrimSpace(updateRaw) == "" {
			r.mustEmit(context.Background(), events.Event{
				Type:    "memory.evaluate",
				Message: "No memory update written",
				Data: map[string]string{
					"turn":     strconv.Itoa(r.turn),
					"accepted": "false",
					"reason":   "no_update",
					"bytes":    "0",
				},
			})
		} else {
			trimmed := strings.TrimSpace(updateRaw)
			hash := agent.SHA256Hex(trimmed)

			accepted, reason, cleaned := r.memEval.Evaluate(updateRaw)
			r.mustEmit(context.Background(), events.Event{
				Type:    "memory.evaluate",
				Message: "Evaluated memory update",
				Data: map[string]string{
					"turn":     strconv.Itoa(r.turn),
					"accepted": fmtBool(accepted),
					"reason":   reason,
					"bytes":    strconv.Itoa(len(trimmed)),
					"sha256":   hash[:12],
				},
			})

			if accepted {
				if err := r.memStore.AppendMemory(context.Background(), formatRunMemoryAppend(strings.TrimSpace(cleaned))); err != nil {
					r.mustEmit(context.Background(), events.Event{
						Type:    "memory.commit.error",
						Message: "Failed to commit memory update",
						Data:    map[string]string{"err": err.Error()},
						Store:   boolp(false),
					})
				} else {
					r.mustEmit(context.Background(), events.Event{
						Type:    "memory.commit",
						Message: "Committed memory update",
						Data: map[string]string{
							"turn":   strconv.Itoa(r.turn),
							"bytes":  strconv.Itoa(len(strings.TrimSpace(cleaned))),
							"sha256": hash[:12],
						},
					})
				}
			}

			_ = r.memStore.AppendCommitLog(context.Background(), types.MemoryCommitLine{
				Scope:     "memory",
				SessionID: r.run.SessionID,
				RunID:     r.run.RunId,
				Model:     r.model,
				Turn:      r.turn,
				Accepted:  accepted,
				Reason:    reason,
				Bytes:     len(trimmed),
				SHA256:    hash,
			})
		}
		_ = r.fs.Write("/memory/update.md", []byte{})
	}

	// Profile update ingestion (global).
	if b, err := r.fs.Read("/profile/update.md"); err == nil {
		updateRaw := string(b)
		if strings.TrimSpace(updateRaw) == "" {
			r.mustEmit(context.Background(), events.Event{
				Type:    "profile.evaluate",
				Message: "No profile update written",
				Data: map[string]string{
					"turn":     strconv.Itoa(r.turn),
					"accepted": "false",
					"reason":   "no_update",
					"bytes":    "0",
				},
			})
		} else {
			trimmed := strings.TrimSpace(updateRaw)
			hash := agent.SHA256Hex(trimmed)

			accepted, reason, cleaned := r.profileEval.Evaluate(updateRaw)
			r.mustEmit(context.Background(), events.Event{
				Type:    "profile.evaluate",
				Message: "Evaluated profile update",
				Data: map[string]string{
					"turn":     strconv.Itoa(r.turn),
					"accepted": fmtBool(accepted),
					"reason":   reason,
					"bytes":    strconv.Itoa(len(trimmed)),
					"sha256":   hash[:12],
				},
			})

			if accepted {
				if err := r.profileStore.AppendProfile(context.Background(), formatRunMemoryAppend(strings.TrimSpace(cleaned))); err != nil {
					r.mustEmit(context.Background(), events.Event{
						Type:    "profile.commit.error",
						Message: "Failed to commit profile update",
						Data:    map[string]string{"err": err.Error()},
						Store:   boolp(false),
					})
				} else {
					r.mustEmit(context.Background(), events.Event{
						Type:    "profile.commit",
						Message: "Committed profile update",
						Data: map[string]string{
							"turn":   strconv.Itoa(r.turn),
							"bytes":  strconv.Itoa(len(strings.TrimSpace(cleaned))),
							"sha256": hash[:12],
						},
					})
				}
			}

			_ = r.profileStore.AppendCommitLog(context.Background(), types.MemoryCommitLine{
				Scope:     "profile",
				SessionID: r.run.SessionID,
				RunID:     r.run.RunId,
				Model:     r.model,
				Turn:      r.turn,
				Accepted:  accepted,
				Reason:    reason,
				Bytes:     len(trimmed),
				SHA256:    hash,
			})
		}
		_ = r.fs.Write("/profile/update.md", []byte{})
	}

	if _, err := store.RecordTurnInSession(r.run.SessionID, r.run.RunId, userMsg, final); err != nil {
		r.mustEmit(context.Background(), events.Event{
			Type:    "session.update.error",
			Message: "Failed to update session state",
			Data:    map[string]string{"err": err.Error()},
			Store:   boolp(false),
		})
	} else {
		r.mustEmit(context.Background(), events.Event{
			Type:    "session.update",
			Message: "Updated session state",
			Data:    map[string]string{"sessionId": r.run.SessionID, "runId": r.run.RunId},
			Store:   boolp(false),
		})
	}

	// NOTE: Do not emit agent.final here. The TUI renders the final response as a
	// chat message, not as an event line.
	return final, nil
}
