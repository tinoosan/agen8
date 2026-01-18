package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/tinoosan/workbench-core/internal/agent"
	"github.com/tinoosan/workbench-core/internal/config"
	"github.com/tinoosan/workbench-core/internal/cost"
	"github.com/tinoosan/workbench-core/internal/events"
	"github.com/tinoosan/workbench-core/internal/fsutil"
	"github.com/tinoosan/workbench-core/internal/llm"
	"github.com/tinoosan/workbench-core/internal/resources"
	"github.com/tinoosan/workbench-core/internal/store"
	"github.com/tinoosan/workbench-core/internal/tools"
	"github.com/tinoosan/workbench-core/internal/tui"
	"github.com/tinoosan/workbench-core/internal/types"
	"github.com/tinoosan/workbench-core/internal/vfs"
	"github.com/tinoosan/workbench-core/internal/vfsutil"
)

// RunNewChatTUI starts the TUI immediately, but defers creating a new session/run
// until the first user message is submitted.
//
// This avoids creating on-disk sessions/runs when the user opens Workbench and exits
// without doing anything.
func RunNewChatTUI(ctx context.Context, title, goal string, maxContextB int, opts RunChatOptions) (retErr error) {
	opts = opts.withDefaults()

	// The TUI owns stdout/stderr. Avoid mixing standard log output into the screen.
	oldLogWriter := log.Writer()
	log.SetOutput(io.Discard)
	defer log.SetOutput(oldLogWriter)

	runCtx, stopSignals := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stopSignals()

	evCh := make(chan events.Event, 2048)
	lazy := &lazyNewSessionTurnRunner{
		ctx:         runCtx,
		opts:        opts,
		maxContextB: maxContextB,
		title:       strings.TrimSpace(title),
		goal:        strings.TrimSpace(goal),
		evCh:        evCh,
	}

	// If we created a run, ensure it transitions to a terminal state and is persisted.
	defer func() {
		if strings.TrimSpace(lazy.run.RunId) == "" {
			return
		}
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
		_, _ = store.StopRun(lazy.run.RunId, status, errMsg)
	}()

	// Start the UI. The runner will emit run/session events only after the first message.
	err := tui.Run(runCtx, lazy, evCh)
	retErr = err

	// Best-effort: emit run.completed if we created a run and have an emitter.
	if lazy.mustEmit != nil && strings.TrimSpace(lazy.run.RunId) != "" {
		boolp := func(b bool) *bool { return &b }
		lazy.mustEmit(context.Background(), events.Event{
			Type:    "run.completed",
			Message: "Run finished",
			Data: map[string]string{
				"sessionId": lazy.run.SessionID,
				"runId":     lazy.run.RunId,
			},
			Console: boolp(false),
		})
	}

	close(evCh)
	return retErr
}

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
	sessionModel := ""
	if sess, err := store.LoadSession(run.SessionID); err == nil {
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
		return fmt.Errorf("OPENROUTER_API_KEY and OPENROUTER_MODEL (or --model or session.activeModel) are required")
	}
	opts.Model = model

	// Resolve pricing against the effective model (session-aware).
	//
	// If pricing is not known for the model, cost is shown as "unknown".
	if opts.PriceInPerMTokensUSD <= 0 && opts.PriceOutPerMTokensUSD <= 0 {
		if inPerM, outPerM, ok, _ := pricingForModel(model, opts.PricingFile); ok {
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

	traceRes, err := resources.NewTraceResource(run.RunId)
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
	fs.Mount(vfs.MountWorkdir, workdirRes)
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
			"/workdir":   workdirRes.BaseDir,
			"/results":   "(virtual)",
			"/trace":     traceRes.BaseDir,
			"/tools":     "(virtual)",
			"/memory":    memoryRes.BaseDir,
			"/profile":   "(global)",
			"/history":   historyRes.BaseDir,
		},
		Console: boolp(false),
	})

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

	builtinInvokers := tools.BuiltinInvokerRegistry(builtinCfg)
	runner := tools.Runner{
		Results:      resultsStore,
		ToolRegistry: builtinInvokers,
	}

	executor := &agent.HostOpExecutor{
		FS:              fs,
		Runner:          &runner,
		DefaultMaxBytes: 4096,
		MaxReadBytes:    16 * 1024,
	}

	historySink.Model = model
	setHistoryModel := func(next string) { historySink.Model = strings.TrimSpace(next) }

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

	// Persist the active model at the session level so "resume session" is deterministic.
	if sess, err := store.LoadSession(run.SessionID); err == nil {
		if strings.TrimSpace(sess.ActiveModel) != model {
			sess.ActiveModel = model
			_ = store.SaveSession(sess)
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
	artifactIndex := newArtifactIndex()

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
		if resp.Ok && (req.Op == types.HostOpFSWrite || req.Op == types.HostOpFSAppend) {
			artifactIndex.ObserveWrite(req.Path)
		}
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
		setHistoryModel:  setHistoryModel,
		workdirBase:      workdirRes.BaseDir,
		builtinInvokers:  builtinInvokers,
		artifacts:        artifactIndex,
		constructor:      constructor,
	}

	err = tui.Run(runCtx, engine, evCh)
	mustEmit(context.Background(), events.Event{
		Type:    "run.completed",
		Message: "Run finished",
		Data: map[string]string{
			"sessionId": run.SessionID,
			"runId":     run.RunId,
		},
		Console: boolp(false),
	})
	close(evCh)
	return err
}

type lazyNewSessionTurnRunner struct {
	ctx context.Context

	opts        RunChatOptions
	maxContextB int
	title       string
	goal        string

	evCh chan events.Event

	initialized bool
	run         types.Run
	engine      *tuiTurnRunner

	mustEmit func(ctx context.Context, ev events.Event)
}

func (r *lazyNewSessionTurnRunner) RunTurn(ctx context.Context, userMsg string) (string, error) {
	if r == nil {
		return "", fmt.Errorf("runner is nil")
	}
	if strings.TrimSpace(userMsg) == "" {
		return "", nil
	}

	// Slash commands are host-side commands and should not create a new session/run.
	if resp, handled, err := r.handleHostCommandPreInit(userMsg); handled {
		return resp, err
	}
	if !r.initialized {
		if err := r.initForFirstTurn(userMsg); err != nil {
			return "", err
		}
	}
	return r.engine.RunTurn(ctx, userMsg)
}

func (r *lazyNewSessionTurnRunner) handleHostCommandPreInit(userMsg string) (resp string, handled bool, err error) {
	line := strings.TrimSpace(userMsg)
	if !strings.HasPrefix(line, "/") {
		return "", false, nil
	}
	boolp := func(b bool) *bool { return &b }

	cmd, arg := splitSlashCommand(line)
	switch cmd {
	case "editor":
		cur, err := resolveWorkDir(r.opts.WorkDir)
		if err != nil {
			return "", true, err
		}
		vpath, display, err := resolveWorkdirVPathFromArg(cur, arg)
		if err != nil {
			r.emitPreInit(events.Event{
				Type:    "ui.editor.error",
				Message: "Editor",
				Data: map[string]string{
					"err": err.Error(),
				},
				Store:   boolp(false),
				Console: boolp(false),
			})
			return "", true, nil
		}
		r.emitPreInit(events.Event{
			Type:    "ui.editor.open",
			Message: "Open editor",
			Data: map[string]string{
				"vpath": vpath,
				"path":  display,
				// Include the absolute workdir so the TUI can open $EDITOR without
				// requiring a session/run to exist.
				"workdir": cur,
			},
			Store:   boolp(false),
			Console: boolp(false),
		})
		return "", true, nil
	case "open":
		cur, err := resolveWorkDir(r.opts.WorkDir)
		if err != nil {
			return "", true, err
		}
		_, display, err := resolveWorkdirVPathFromArg(cur, arg)
		if err != nil {
			r.emitPreInit(events.Event{
				Type:    "ui.open.error",
				Message: "Open",
				Data: map[string]string{
					"err": err.Error(),
				},
				Store:   boolp(false),
				Console: boolp(false),
			})
			return "", true, nil
		}

		in, _ := json.Marshal(map[string]string{"path": display})
		inv := tools.NewBuiltinOpenInvoker(cur)
		_, invErr := inv.Invoke(r.ctx, types.ToolRequest{
			Version:  "v1",
			CallID:   "preinit",
			ToolID:   types.ToolID("builtin.open"),
			ActionID: "open",
			Input:    in,
		})
		if invErr != nil {
			r.emitPreInit(events.Event{
				Type:    "ui.open.error",
				Message: "Open",
				Data: map[string]string{
					"path": display,
					"err":  invErr.Error(),
				},
				Store:   boolp(false),
				Console: boolp(false),
			})
			return "", true, nil
		}
		r.emitPreInit(events.Event{
			Type:    "ui.open.ok",
			Message: "Opened file",
			Data: map[string]string{
				"path": display,
			},
			Store:   boolp(false),
			Console: boolp(false),
		})
		return "", true, nil
	case "model":
		cur := strings.TrimSpace(r.opts.Model)
		if cur == "" {
			cur = strings.TrimSpace(os.Getenv("OPENROUTER_MODEL"))
		}
		next, out, ok := handleModelCommandPreInit(cur, r.opts.PricingFile, arg)
		if !ok {
			return "", false, nil
		}
		if strings.TrimSpace(out) != "" {
			r.emitPreInit(events.Event{
				Type:    "model.info",
				Message: "Model",
				Data: map[string]string{
					"text": out,
				},
				Store:   boolp(false),
				Console: boolp(false),
			})
			resp = out
		}
		if strings.TrimSpace(next) != "" && next != cur {
			r.opts.Model = next
			inPerM, outPerM, known, source := pricingForModel(next, r.opts.PricingFile)
			if known {
				r.opts.PriceInPerMTokensUSD = inPerM
				r.opts.PriceOutPerMTokensUSD = outPerM
			} else {
				r.opts.PriceInPerMTokensUSD = 0
				r.opts.PriceOutPerMTokensUSD = 0
			}
			r.emitPreInit(events.Event{
				Type:    "model.changed",
				Message: "Model changed",
				Data: map[string]string{
					"from":          cur,
					"to":            next,
					"pricingKnown":  fmtBool(known),
					"pricingSource": source,
				},
				Store:   boolp(false),
				Console: boolp(false),
			})
		}
		return resp, true, nil
	case "cd":
		cur, err := resolveWorkDir(r.opts.WorkDir)
		if err != nil {
			return "", true, err
		}
		next, err := resolveWorkDirChange(cur, arg)
		if err != nil {
			r.emitPreInit(events.Event{
				Type:    "workdir.error",
				Message: "Workdir change failed",
				Data: map[string]string{
					"from": cur,
					"to":   strings.TrimSpace(arg),
					"err":  err.Error(),
				},
				Store:   boolp(false),
				Console: boolp(false),
			})
			return "", true, nil
		}
		r.opts.WorkDir = next
		r.emitPreInit(events.Event{
			Type:    "workdir.changed",
			Message: "Workdir changed",
			Data: map[string]string{
				"from": cur,
				"to":   next,
			},
			Store:   boolp(false),
			Console: boolp(false),
		})
		return "", true, nil
	case "pwd", "workdir":
		if cmd == "workdir" && strings.TrimSpace(arg) != "" {
			// /workdir <path> behaves like /cd <path>.
			cur, err := resolveWorkDir(r.opts.WorkDir)
			if err != nil {
				return "", true, err
			}
			next, err := resolveWorkDirChange(cur, arg)
			if err != nil {
				r.emitPreInit(events.Event{
					Type:    "workdir.error",
					Message: "Workdir change failed",
					Data: map[string]string{
						"from": cur,
						"to":   strings.TrimSpace(arg),
						"err":  err.Error(),
					},
					Store:   boolp(false),
					Console: boolp(false),
				})
				return "", true, nil
			}
			r.opts.WorkDir = next
			r.emitPreInit(events.Event{
				Type:    "workdir.changed",
				Message: "Workdir changed",
				Data: map[string]string{
					"from": cur,
					"to":   next,
				},
				Store:   boolp(false),
				Console: boolp(false),
			})
			return "", true, nil
		}
		cur, err := resolveWorkDir(r.opts.WorkDir)
		if err != nil {
			return "", true, err
		}
		r.emitPreInit(events.Event{
			Type:    "workdir.pwd",
			Message: "Current workdir",
			Data: map[string]string{
				"workdir": cur,
			},
			Store:   boolp(false),
			Console: boolp(false),
		})
		return cur, true, nil
	default:
		return "", false, nil
	}
}

func (r *lazyNewSessionTurnRunner) emitPreInit(ev events.Event) {
	if r == nil || r.evCh == nil {
		return
	}
	select {
	case r.evCh <- ev:
	default:
		// Best-effort: pre-run commands should never block the UI.
	}
}

// ReadVFS reads a virtual filesystem path from the currently active run.
//
// This is used by the TUI to preview "openable" artifacts (e.g. /results/<callId>/response.json
// or /workspace/<file>) without changing the agent loop contract or introducing a new host op.
//
// For lazyNewSessionTurnRunner, this is only available after the first turn initializes
// the session/run and mounts the VFS.
func (r *lazyNewSessionTurnRunner) ReadVFS(ctx context.Context, path string, maxBytes int) (text string, bytesLen int, truncated bool, err error) {
	if r == nil {
		return "", 0, false, fmt.Errorf("runner is nil")
	}
	// Support reading from /workdir before the first user message, so /editor can be
	// used without forcing a session/run to be created.
	if !r.initialized || r.engine == nil {
		if !strings.HasPrefix(strings.TrimSpace(path), "/workdir/") {
			return "", 0, false, fmt.Errorf("no active run (send a message first)")
		}
		base, err := resolveWorkDir(r.opts.WorkDir)
		if err != nil {
			return "", 0, false, err
		}
		res, err := resources.NewWorkdirResource(base)
		if err != nil {
			return "", 0, false, err
		}
		sub := strings.TrimPrefix(strings.TrimSpace(path), "/workdir/")
		sub, _, err = vfsutil.NormalizeResourceSubpath(sub)
		if err != nil || sub == "" || sub == "." {
			return "", 0, false, fmt.Errorf("invalid workdir path: %s", path)
		}
		b, err := res.Read(sub)
		if err != nil {
			return "", 0, false, err
		}
		bytesLen = len(b)
		if maxBytes <= 0 {
			maxBytes = 16 * 1024
		}
		if bytesLen > maxBytes {
			b = b[:maxBytes]
			truncated = true
		}
		_ = ctx
		return string(b), bytesLen, truncated, nil
	}
	return r.engine.ReadVFS(ctx, path, maxBytes)
}

func (r *lazyNewSessionTurnRunner) WriteVFS(ctx context.Context, path string, data []byte) error {
	if r == nil {
		return fmt.Errorf("runner is nil")
	}
	// Support writing under /workdir before the first user message, so /editor can
	// save without creating a session/run.
	if !r.initialized || r.engine == nil {
		if !strings.HasPrefix(strings.TrimSpace(path), "/workdir/") {
			return fmt.Errorf("no active run (send a message first)")
		}
		base, err := resolveWorkDir(r.opts.WorkDir)
		if err != nil {
			return err
		}
		res, err := resources.NewWorkdirResource(base)
		if err != nil {
			return err
		}
		sub := strings.TrimPrefix(strings.TrimSpace(path), "/workdir/")
		sub, _, err = vfsutil.NormalizeResourceSubpath(sub)
		if err != nil || sub == "" || sub == "." {
			return fmt.Errorf("invalid workdir path: %s", path)
		}
		_ = ctx
		return res.Write(sub, data)
	}
	return r.engine.WriteVFS(ctx, path, data)
}

func (r *lazyNewSessionTurnRunner) initForFirstTurn(firstUserMsg string) error {
	if r.initialized {
		return nil
	}

	title := strings.TrimSpace(r.title)
	if title == "" || strings.EqualFold(title, "workbench") {
		title = strings.TrimSpace(firstLineForTitle(firstUserMsg))
		if title == "" {
			title = "workbench"
		}
	}
	goal := strings.TrimSpace(r.goal)
	if goal == "" {
		goal = "interactive chat"
	}

	sess, err := store.CreateSession(title)
	if err != nil {
		return err
	}
	run, err := store.CreateRunInSession(sess.SessionID, "", goal, r.maxContextB)
	if err != nil {
		return err
	}
	r.run = run

	historyRes, err := resources.NewSessionHistoryResource(run.SessionID)
	if err != nil {
		return fmt.Errorf("create history: %w", err)
	}
	historySink := &events.HistorySink{Store: historyRes.Store}

	emitter := &events.Emitter{
		RunID: run.RunId,
		Sink: events.MultiSink{
			events.StoreSink{},
			historySink,
			tui.EventSink{Ch: r.evCh},
		},
	}
	r.mustEmit = func(ctx context.Context, ev events.Event) {
		_ = emitter.Emit(ctx, ev)
	}
	boolp := func(b bool) *bool { return &b }

	model := strings.TrimSpace(r.opts.Model)
	if model == "" {
		model = strings.TrimSpace(os.Getenv("OPENROUTER_MODEL"))
	}
	if strings.TrimSpace(os.Getenv("OPENROUTER_API_KEY")) == "" || model == "" {
		return fmt.Errorf("OPENROUTER_API_KEY and OPENROUTER_MODEL (or /model) are required")
	}
	r.opts.Model = model

	// Resolve pricing against the effective model (pre-run /model is allowed).
	if r.opts.PriceInPerMTokensUSD <= 0 && r.opts.PriceOutPerMTokensUSD <= 0 {
		if inPerM, outPerM, ok, _ := pricingForModel(model, r.opts.PricingFile); ok {
			r.opts.PriceInPerMTokensUSD = inPerM
			r.opts.PriceOutPerMTokensUSD = outPerM
		}
	}

	historySink.Model = model
	setHistoryModel := func(next string) { historySink.Model = strings.TrimSpace(next) }

	r.mustEmit(context.Background(), events.Event{
		Type:    "run.started",
		Message: "Run started",
		Data: map[string]string{
			"sessionId":    run.SessionID,
			"sessionTitle": title,
			"runId":        run.RunId,
			"userId":       strings.TrimSpace(r.opts.UserID),
		},
		Console: boolp(false),
	})

	// Persist the session's active model as early as possible.
	sess.ActiveModel = model
	_ = store.SaveSession(sess)

	traceRes, err := resources.NewTraceResource(run.RunId)
	if err != nil {
		return fmt.Errorf("create trace: %w", err)
	}

	fs := vfs.NewFS()

	workdirAbs, err := resolveWorkDir(r.opts.WorkDir)
	if err != nil {
		return err
	}
	workdirRes, err := resources.NewWorkdirResource(workdirAbs)
	if err != nil {
		return fmt.Errorf("create workdir: %w", err)
	}

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
	fs.Mount(vfs.MountWorkdir, workdirRes)
	fs.Mount(vfs.MountResults, resultsRes)
	fs.Mount(vfs.MountTrace, traceRes)
	fs.Mount(vfs.MountTools, toolsResource)
	fs.Mount(vfs.MountMemory, memoryRes)
	fs.Mount(vfs.MountProfile, profileRes)
	fs.Mount(vfs.MountHistory, historyRes)

	r.mustEmit(context.Background(), events.Event{
		Type:    "host.mounted",
		Message: "Mounted VFS resources",
		Data: map[string]string{
			"/workspace": workspace.BaseDir,
			"/workdir":   workdirRes.BaseDir,
			"/results":   "(virtual)",
			"/trace":     traceRes.BaseDir,
			"/tools":     "(virtual)",
			"/memory":    memoryRes.BaseDir,
			"/profile":   "(global)",
			"/history":   historyRes.BaseDir,
		},
		Console: boolp(false),
	})

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

	builtinInvokers := tools.BuiltinInvokerRegistry(builtinCfg)
	runner := tools.Runner{
		Results:      resultsStore,
		ToolRegistry: builtinInvokers,
	}

	executor := &agent.HostOpExecutor{
		FS:              fs,
		Runner:          &runner,
		DefaultMaxBytes: 4096,
		MaxReadBytes:    16 * 1024,
	}

	run.Runtime = &types.RunRuntimeConfig{
		DataDir:               config.DataDir,
		Model:                 model,
		MaxSteps:              r.opts.MaxSteps,
		MaxTraceBytes:         r.opts.MaxTraceBytes,
		MaxMemoryBytes:        r.opts.MaxMemoryBytes,
		MaxProfileBytes:       r.opts.MaxProfileBytes,
		RecentHistoryPairs:    r.opts.RecentHistoryPairs,
		IncludeHistoryOps:     derefBool(r.opts.IncludeHistoryOps, true),
		PriceInPerMTokensUSD:  r.opts.PriceInPerMTokensUSD,
		PriceOutPerMTokensUSD: r.opts.PriceOutPerMTokensUSD,
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
		IncludeHistoryOps: derefBool(r.opts.IncludeHistoryOps, true),
		MaxProfileBytes:   r.opts.MaxProfileBytes,
		MaxMemoryBytes:    r.opts.MaxMemoryBytes,
		MaxTraceBytes:     r.opts.MaxTraceBytes,
		MaxHistoryBytes:   8 * 1024,
		StatePath:         "/workspace/context_constructor_state.json",
		ManifestPath:      "/workspace/context_constructor_manifest.json",
		Emit: func(eventType, message string, data map[string]string) {
			r.mustEmit(context.Background(), events.Event{Type: eventType, Message: message, Data: data})
		},
	}
	artifactIndex := newArtifactIndex()

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

		r.mustEmit(ctx, events.Event{
			Type:      "agent.op.request",
			Message:   "Agent requested host op",
			Data:      reqData,
			StoreData: map[string]string{"op": req.Op, "path": req.Path, "toolId": req.ToolID.String(), "actionId": req.ActionID},
		})
		resp := executor.Exec(ctx, req)
		if resp.Ok && (req.Op == types.HostOpFSWrite || req.Op == types.HostOpFSAppend) {
			artifactIndex.ObserveWrite(req.Path)
		}
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
		r.mustEmit(ctx, events.Event{
			Type:      "agent.op.response",
			Message:   "Host op completed",
			Data:      respData,
			StoreData: map[string]string{"op": resp.Op, "ok": fmtBool(resp.Ok), "err": resp.Error},
		})
		return resp
	}

	r.mustEmit(context.Background(), events.Event{
		Type:    "agent.loop.start",
		Message: "Agent loop started",
		Data:    map[string]string{"model": model},
	})

	updater = &agent.ContextUpdater{
		FS:              fs,
		TraceStore:      traceStore,
		MaxProfileBytes: r.opts.MaxProfileBytes,
		MaxMemoryBytes:  r.opts.MaxMemoryBytes,
		MaxTraceBytes:   r.opts.MaxTraceBytes,
		ManifestPath:    "/workspace/context_manifest.json",
		Emit: func(eventType, message string, data map[string]string) {
			r.mustEmit(context.Background(), events.Event{Type: eventType, Message: message, Data: data})
		},
	}

	a, err := agent.New(agent.Config{
		LLM:          client,
		Exec:         agent.HostExecFunc(execWithEvents),
		Model:        model,
		SystemPrompt: baseSystemPrompt,
		Context:      constructor,
		MaxSteps:     r.opts.MaxSteps,
	})
	if err != nil {
		return err
	}

	r.engine = &tuiTurnRunner{
		ctx:              r.ctx,
		run:              run,
		opts:             r.opts,
		fs:               fs,
		agent:            a,
		baseSystemPrompt: baseSystemPrompt,
		mustEmit:         r.mustEmit,
		memStore:         memStore,
		profileStore:     profileStore,
		memEval:          agent.DefaultMemoryEvaluator(),
		profileEval:      agent.DefaultProfileEvaluator(),
		model:            model,
		setHistoryModel:  setHistoryModel,
		workdirBase:      workdirRes.BaseDir,
		builtinInvokers:  builtinInvokers,
		artifacts:        artifactIndex,
		constructor:      constructor,
	}
	r.initialized = true
	return nil
}

func firstLineForTitle(s string) string {
	s = strings.ReplaceAll(s, "\r", "")
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}

type tuiTurnRunner struct {
	ctx context.Context

	run  types.Run
	opts RunChatOptions

	fs *vfs.FS

	agent            *agent.Agent
	baseSystemPrompt string
	model            string
	setHistoryModel  func(model string)

	mustEmit func(ctx context.Context, ev events.Event)

	memStore     store.MemoryStore
	profileStore store.ProfileStore
	memEval      *agent.MemoryEvaluator
	profileEval  *agent.ProfileEvaluator

	workdirBase string
	// builtinInvokers is the in-memory registry used by tool.run for builtins.
	//
	// It is a map (reference type), so updating entries updates the runner behavior
	// without reconstructing the entire agent loop.
	builtinInvokers tools.MapRegistry
	artifacts       *ArtifactIndex
	constructor     *agent.ContextConstructor

	turn         int
	conversation []types.LLMMessage
}

// ReadVFS reads a virtual filesystem path via the run's mounted VFS.
//
// The TUI uses this to preview artifacts referenced by Activity items (e.g.
// /results/<callId>/response.json and /workspace/<file>). This keeps the "open
// artifact" UX purely in the UI layer without adding new host primitives.
func (r *tuiTurnRunner) ReadVFS(ctx context.Context, path string, maxBytes int) (text string, bytesLen int, truncated bool, err error) {
	if r == nil || r.fs == nil {
		return "", 0, false, fmt.Errorf("vfs not available")
	}
	if maxBytes <= 0 {
		maxBytes = 16 * 1024
	}
	b, err := r.fs.Read(path)
	if err != nil {
		return "", 0, false, err
	}
	bytesLen = len(b)
	if bytesLen > maxBytes {
		b = b[:maxBytes]
		truncated = true
	}
	_ = ctx
	return string(b), bytesLen, truncated, nil
}

func (r *tuiTurnRunner) WriteVFS(ctx context.Context, path string, data []byte) error {
	if r == nil || r.fs == nil {
		return fmt.Errorf("vfs not available")
	}
	_ = ctx
	return r.fs.Write(path, data)
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

	if strings.HasPrefix(strings.TrimSpace(userMsg), ":publish") {
		return r.handlePublish(userMsg)
	}

	// Slash commands are host-side commands and do not call the agent.
	//
	// These commands operate on the host's /workdir mount and are safe to run
	// during an interactive session (no restart required).
	if resp, handled := r.handleSlashCommand(userMsg); handled {
		return resp, nil
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

	// Resolve @file references into bounded attachments for the constructor.
	//
	// This is what enables coding-agent workflows:
	//   user: "Update @go.mod ..."
	//   host: resolves /workdir/go.mod and injects it into the system prompt
	refs, err := ResolveAtRefs(r.fs, r.workdirBase, r.artifacts, userMsg, 6, 48*1024, 12*1024)
	if err != nil {
		return "", err
	}
	if r.constructor != nil {
		r.constructor.SetFileAttachments(refs.Attachments)
		defer r.constructor.ClearFileAttachments()
	}
	if len(refs.AttachedSummaries) != 0 {
		r.mustEmit(context.Background(), events.Event{
			Type:    "refs.attached",
			Message: "Attached file references",
			Data: map[string]string{
				"count": strconv.Itoa(len(refs.AttachedSummaries)),
				"files": strings.Join(refs.AttachedSummaries, ", "),
			},
			Store: boolp(false),
		})
	}
	for tok, cands := range refs.Ambiguous {
		r.mustEmit(context.Background(), events.Event{
			Type:    "refs.ambiguous",
			Message: "Ambiguous @reference",
			Data: map[string]string{
				"token":      tok,
				"candidates": strings.Join(cands, ", "),
			},
			Store: boolp(false),
		})
	}
	if len(refs.Unresolved) != 0 {
		r.mustEmit(context.Background(), events.Event{
			Type:    "refs.unresolved",
			Message: "Unresolved @references",
			Data: map[string]string{
				"tokens": strings.Join(refs.Unresolved, ", "),
			},
			Store: boolp(false),
		})
	}

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

		// Cost estimate is optional: if pricing is unknown, emit a marker so the UI
		// can clear stale values (and display "?" if desired).
		pricingKnown := r.opts.PriceInPerMTokensUSD > 0 || r.opts.PriceOutPerMTokensUSD > 0
		costUSD := estimateTurnCostUSD(turnUsage, r.opts.PriceInPerMTokensUSD, r.opts.PriceOutPerMTokensUSD)
		data := map[string]string{
			"turn":         strconv.Itoa(r.turn),
			"input":        strconv.Itoa(turnUsage.InputTokens),
			"output":       strconv.Itoa(turnUsage.OutputTokens),
			"total":        strconv.Itoa(turnUsage.TotalTokens),
			"known":        fmtBool(pricingKnown),
			"priceInPerM":  fmtUSD(r.opts.PriceInPerMTokensUSD),
			"priceOutPerM": fmtUSD(r.opts.PriceOutPerMTokensUSD),
		}
		if pricingKnown && costUSD > 0 {
			data["costUsd"] = fmtUSD(costUSD)
			data["pricingSource"] = "host_config"
		}
		r.mustEmit(context.Background(), events.Event{
			Type:    "llm.cost.total",
			Message: "Turn cost estimate",
			Data:    data,
			Store:   boolp(false),
		})
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

func (r *tuiTurnRunner) handleSlashCommand(userMsg string) (resp string, handled bool) {
	if r == nil || r.fs == nil {
		return "", false
	}
	line := strings.TrimSpace(userMsg)
	if !strings.HasPrefix(line, "/") {
		return "", false
	}
	boolp := func(b bool) *bool { return &b }

	cmd, arg := splitSlashCommand(line)
	switch cmd {
	case "editor":
		vpath, display, err := resolveWorkdirVPathFromArg(r.workdirBase, arg)
		if err != nil {
			r.mustEmit(context.Background(), events.Event{
				Type:    "ui.editor.error",
				Message: "Editor",
				Data: map[string]string{
					"err": err.Error(),
				},
				Store:   boolp(false),
				Console: boolp(false),
			})
			return "", true
		}
		r.mustEmit(context.Background(), events.Event{
			Type:    "ui.editor.open",
			Message: "Open editor",
			Data: map[string]string{
				"vpath":   vpath,
				"path":    display,
				"workdir": strings.TrimSpace(r.workdirBase),
			},
			Store:   boolp(false),
			Console: boolp(false),
		})
		return "", true
	case "open":
		_, display, err := resolveWorkdirVPathFromArg(r.workdirBase, arg)
		if err != nil {
			r.mustEmit(context.Background(), events.Event{
				Type:    "ui.open.error",
				Message: "Open",
				Data: map[string]string{
					"err": err.Error(),
				},
				Store:   boolp(false),
				Console: boolp(false),
			})
			return "", true
		}
		in, _ := json.Marshal(map[string]string{"path": display})
		// Execute via the normal host primitive pathway so this appears in the Activity feed
		// (tool.run) without involving the model/agent loop.
		_ = r.agent.Exec.Exec(context.Background(), types.HostOpRequest{
			Op:       types.HostOpToolRun,
			ToolID:   types.ToolID("builtin.open"),
			ActionID: "open",
			Input:    in,
		})
		return "", true

	case "model":
		cur := strings.TrimSpace(r.model)
		if cur == "" {
			cur = strings.TrimSpace(r.opts.Model)
		}
		next, out, ok := handleModelCommandPreInit(cur, r.opts.PricingFile, arg)
		if !ok {
			return "", false
		}
		if strings.TrimSpace(out) != "" {
			r.mustEmit(context.Background(), events.Event{
				Type:    "model.info",
				Message: "Model",
				Data: map[string]string{
					"text": out,
				},
				Console: boolp(false),
			})
			resp = out
		}
		if strings.TrimSpace(next) == "" || next == cur {
			return resp, true
		}

		inPerM, outPerM, known, source := pricingForModel(next, r.opts.PricingFile)
		if known {
			r.opts.PriceInPerMTokensUSD = inPerM
			r.opts.PriceOutPerMTokensUSD = outPerM
		} else {
			r.opts.PriceInPerMTokensUSD = 0
			r.opts.PriceOutPerMTokensUSD = 0
		}

		// Update the active model for subsequent LLM calls.
		r.model = next
		r.opts.Model = next
		r.agent.Model = next
		if r.setHistoryModel != nil {
			r.setHistoryModel(next)
		}

		if r.run.Runtime == nil {
			r.run.Runtime = &types.RunRuntimeConfig{}
		}
		r.run.Runtime.Model = next
		r.run.Runtime.PriceInPerMTokensUSD = r.opts.PriceInPerMTokensUSD
		r.run.Runtime.PriceOutPerMTokensUSD = r.opts.PriceOutPerMTokensUSD
		_ = store.SaveRun(r.run)

		// Persist at session scope so resume uses the selected model.
		if sess, err := store.LoadSession(r.run.SessionID); err == nil {
			sess.ActiveModel = next
			_ = store.SaveSession(sess)
		}

		r.mustEmit(context.Background(), events.Event{
			Type:    "model.changed",
			Message: "Model changed",
			Data: map[string]string{
				"from":          cur,
				"to":            next,
				"pricingKnown":  fmtBool(known),
				"pricingSource": source,
			},
			Console: boolp(false),
		})
		return resp, true
	case "cd":
		from := strings.TrimSpace(r.workdirBase)
		if from == "" {
			// Should not happen: /workdir is always mounted for a run.
			from = "."
		}
		to, err := resolveWorkDirChange(from, arg)
		if err != nil {
			r.mustEmit(context.Background(), events.Event{
				Type:    "workdir.error",
				Message: "Workdir change failed",
				Data: map[string]string{
					"from": from,
					"to":   strings.TrimSpace(arg),
					"err":  err.Error(),
				},
				Console: boolp(false),
			})
			return "", true
		}

		workdirRes, err := resources.NewWorkdirResource(to)
		if err != nil {
			r.mustEmit(context.Background(), events.Event{
				Type:    "workdir.error",
				Message: "Workdir change failed",
				Data: map[string]string{
					"from": from,
					"to":   to,
					"err":  err.Error(),
				},
				Console: boolp(false),
			})
			return "", true
		}

		// Rebind /workdir to the new root. All file operations immediately target the new directory.
		r.fs.Mount(vfs.MountWorkdir, workdirRes)
		r.workdirBase = workdirRes.BaseDir
		r.opts.WorkDir = workdirRes.BaseDir

		// Update builtin sandbox roots (builtin.bash + builtin.ripgrep) to follow the active workdir.
		if r.builtinInvokers != nil {
			r.builtinInvokers[types.ToolID("builtin.bash")] = tools.NewBuiltinBashInvoker(workdirRes.BaseDir)
			r.builtinInvokers[types.ToolID("builtin.ripgrep")] = tools.NewBuiltinRipgrepInvoker(workdirRes.BaseDir)
			r.builtinInvokers[types.ToolID("builtin.open")] = tools.NewBuiltinOpenInvoker(workdirRes.BaseDir)
		}

		r.mustEmit(context.Background(), events.Event{
			Type:    "workdir.changed",
			Message: "Workdir changed",
			Data: map[string]string{
				"from": from,
				"to":   workdirRes.BaseDir,
			},
			Console: boolp(false),
		})
		return "", true

	case "pwd", "workdir":
		// /workdir with no args behaves like /pwd.
		// /workdir <path> is an alias for /cd <path>.
		if cmd == "workdir" && strings.TrimSpace(arg) != "" {
			return r.handleSlashCommand("/cd " + arg)
		}
		r.mustEmit(context.Background(), events.Event{
			Type:    "workdir.pwd",
			Message: "Current workdir",
			Data: map[string]string{
				"workdir": strings.TrimSpace(r.workdirBase),
			},
			Console: boolp(false),
		})
		return strings.TrimSpace(r.workdirBase), true
	default:
		return "", false
	}
}

func splitSlashCommand(line string) (cmd string, arg string) {
	line = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "/"))
	if line == "" {
		return "", ""
	}
	parts := strings.Fields(line)
	if len(parts) == 0 {
		return "", ""
	}
	cmd = strings.TrimSpace(parts[0])
	if len(parts) == 1 {
		return cmd, ""
	}
	i := strings.Index(line, parts[0])
	if i >= 0 {
		arg = strings.TrimSpace(line[i+len(parts[0]):])
	}
	return cmd, arg
}

func resolveWorkdirVPathFromArg(workdirBase, arg string) (vpath string, display string, err error) {
	workdirBase = strings.TrimSpace(workdirBase)
	arg = strings.TrimSpace(arg)
	if arg == "" {
		return "", "", fmt.Errorf("usage: /editor <path> or /editor @<path>")
	}

	// Prefer @token parsing (supports quoting).
	ref := arg
	if strings.HasPrefix(strings.TrimSpace(arg), "@") {
		toks := ExtractAtRefs(arg)
		if len(toks) == 0 {
			return "", "", fmt.Errorf("invalid @reference")
		}
		ref = toks[0]
	} else {
		ref = strings.Trim(ref, `"'“”‘’`)
	}
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", "", fmt.Errorf("path is required")
	}

	// Allow absolute paths, but only if they remain within the current workdir.
	if filepath.IsAbs(ref) {
		if workdirBase == "" {
			return "", "", fmt.Errorf("workdir is not set")
		}
		abs := filepath.Clean(ref)
		rel, err := filepath.Rel(workdirBase, abs)
		if err != nil {
			return "", "", fmt.Errorf("path must be within workdir")
		}
		rel = filepath.ToSlash(rel)
		if rel == ".." || strings.HasPrefix(rel, "../") {
			return "", "", fmt.Errorf("path must be within workdir")
		}
		ref = rel
	}

	clean, _, err := vfsutil.NormalizeResourceSubpath(ref)
	if err != nil || clean == "" || clean == "." {
		return "", "", fmt.Errorf("invalid path: %s", ref)
	}
	return "/workdir/" + clean, clean, nil
}

func handleModelCommandPreInit(currentModel, pricingFile, arg string) (nextModel string, out string, handled bool) {
	arg = strings.TrimSpace(arg)

	cur := strings.TrimSpace(currentModel)
	if cur == "" {
		cur = "unknown"
	}

	if arg == "" || strings.EqualFold(arg, "show") {
		return "", "Current model: " + cur, true
	}
	if strings.EqualFold(arg, "list") {
		lines := []string{"Supported models:"}
		for _, m := range cost.SupportedModels() {
			prefix := "  - "
			if m == currentModel {
				prefix = "  * "
			}
			lines = append(lines, prefix+m)
		}
		return "", strings.Join(lines, "\n"), true
	}

	// Allow both:
	//   /model set <id>
	//   /model <id>
	lower := strings.ToLower(arg)
	if strings.HasPrefix(lower, "set ") {
		arg = strings.TrimSpace(arg[4:])
	}
	if strings.TrimSpace(arg) == "" {
		return "", "Usage:\n  /model show\n  /model list\n  /model <modelId>", true
	}

	if !cost.IsSupportedModel(arg) {
		return "", fmt.Sprintf("Unsupported model %q. Use /model list.", arg), true
	}

	_, _, known, _ := pricingForModel(arg, pricingFile)
	msg := "Model set to " + arg
	if !known {
		msg += " (pricing unknown)"
	}
	return arg, msg, true
}

func (r *tuiTurnRunner) handlePublish(userMsg string) (string, error) {
	if r == nil || r.fs == nil {
		return "", fmt.Errorf("vfs not available")
	}
	boolp := func(b bool) *bool { return &b }

	parts := strings.Fields(strings.TrimSpace(userMsg))
	// parts[0] is ":publish"
	src := ""
	dst := ""
	if len(parts) >= 2 {
		src = parts[1]
	}
	if len(parts) >= 3 {
		dst = parts[2]
	}
	if strings.TrimSpace(src) == "" {
		src = "last"
	}
	srcVPath, ok := r.resolvePublishSource(src)
	if !ok {
		return "", fmt.Errorf("publish: unknown source %q (use :publish <vfsPath> or :publish <artifactName>)", src)
	}

	dstVPath, err := r.publishToWorkdir(srcVPath, dst)
	if err != nil {
		return "", err
	}
	if r.artifacts != nil {
		r.artifacts.RecordPublish(srcVPath, dstVPath)
		r.artifacts.ObserveWrite(dstVPath)
	}
	r.mustEmit(context.Background(), events.Event{
		Type:    "artifact.published",
		Message: "Published artifact to workdir",
		Data: map[string]string{
			"source": srcVPath,
			"dest":   dstVPath,
		},
		Store: boolp(false),
	})
	return "done", nil
}

func (r *tuiTurnRunner) resolvePublishSource(src string) (vpath string, ok bool) {
	src = strings.TrimSpace(src)
	if src == "" {
		return "", false
	}
	if strings.HasPrefix(src, "/") {
		return src, true
	}
	if r.artifacts == nil {
		return "", false
	}
	return r.artifacts.Resolve(src)
}

func (r *tuiTurnRunner) publishToWorkdir(srcVPath string, dstRel string) (string, error) {
	srcVPath = strings.TrimSpace(srcVPath)
	if srcVPath == "" {
		return "", fmt.Errorf("publish: source is required")
	}
	b, err := r.fs.Read(srcVPath)
	if err != nil {
		return "", fmt.Errorf("publish: read %s: %w", srcVPath, err)
	}

	dstRel = strings.TrimSpace(dstRel)
	if dstRel == "" {
		dstRel = path.Base(srcVPath)
	}
	clean, _, err := vfsutil.NormalizeResourceSubpath(dstRel)
	if err != nil || clean == "" || clean == "." {
		return "", fmt.Errorf("publish: invalid destination %q", dstRel)
	}

	dstVPath := "/workdir/" + clean
	// Avoid overwriting existing files by default. If it exists, publish under
	// "/workdir/.workbench/<name>".
	if _, err := r.fs.Read(dstVPath); err == nil {
		dstVPath = "/workdir/.workbench/" + clean
	}
	if err := r.fs.Write(dstVPath, b); err != nil {
		return "", fmt.Errorf("publish: write %s: %w", dstVPath, err)
	}
	return dstVPath, nil
}
