package app

import (
	"context"
	"encoding/json"
	"errors"
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

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tinoosan/workbench-core/internal/agent"
	"github.com/tinoosan/workbench-core/internal/atref"
	"github.com/tinoosan/workbench-core/internal/config"
	"github.com/tinoosan/workbench-core/internal/cost"
	"github.com/tinoosan/workbench-core/internal/events"
	"github.com/tinoosan/workbench-core/internal/resources"
	"github.com/tinoosan/workbench-core/internal/store"
	"github.com/tinoosan/workbench-core/internal/tools"
	"github.com/tinoosan/workbench-core/internal/tui"
	"github.com/tinoosan/workbench-core/internal/types"
	"github.com/tinoosan/workbench-core/internal/vfs"
	"github.com/tinoosan/workbench-core/internal/vfsutil"
)

func cursorDebugLog(hypothesisId, location, message string, data map[string]any) {
	// #region agent log
	const logPath = "/Users/santinoonyeme/personal/dev/Projects/workbench/.cursor/debug.log"
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	payload := map[string]any{
		"sessionId":    "debug-session",
		"runId":        "pre-fix",
		"hypothesisId": hypothesisId,
		"location":     location,
		"message":      message,
		"data":         data,
		"timestamp":    time.Now().UnixMilli(),
	}
	if b, err := json.Marshal(payload); err == nil {
		_, _ = f.Write(append(b, '\n'))
	}
	// #endregion
}

// RunNewChatTUI starts the TUI immediately, but defers creating a new session/run
// until the first user message is submitted.
//
// This avoids creating on-disk sessions/runs when the user opens Workbench and exits
// without doing anything.
func RunNewChatTUI(ctx context.Context, cfg config.Config, title, goal string, maxContextB int, opts ...RunChatOption) (retErr error) {
	if err := cfg.Validate(); err != nil {
		return err
	}
	resolved := resolveRunChatOptions(opts...)

	// The TUI owns stdout/stderr. Avoid mixing standard log output into the screen.
	oldLogWriter := log.Writer()
	log.SetOutput(io.Discard)
	defer log.SetOutput(oldLogWriter)

	runCtx, stopSignals := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stopSignals()

	evCh := make(chan events.Event, 2048)
	lazy := &lazyNewSessionTurnRunner{
		ctx:         runCtx,
		cfg:         cfg,
		opts:        resolved,
		maxContextB: maxContextB,
		title:       strings.TrimSpace(title),
		goal:        strings.TrimSpace(goal),
		evCh:        evCh,
	}
	wasInterrupted := false

	// If we created a run, ensure it transitions to a terminal state and is persisted.
	defer func() {
		if strings.TrimSpace(lazy.run.RunId) == "" {
			return
		}
		status := types.StatusDone
		errMsg := ""
		if runCtx.Err() != nil || wasInterrupted {
			status = types.StatusCanceled
			errMsg = "interrupted"
		}
		if retErr != nil {
			status = types.StatusFailed
			errMsg = retErr.Error()
		}
		_, _ = store.StopRun(cfg, lazy.run.RunId, status, errMsg)
	}()

	// Start the UI. The runner will emit run/session events only after the first message.
	err := tui.Run(runCtx, lazy, evCh)
	wasInterrupted = errors.Is(err, tea.ErrInterrupted)
	retErr = err

	// If the user interrupted the session (Ctrl-C), print a convenient resume command
	// after the TUI exits.
	if shouldPrintResumeHint(runCtx, err) && strings.TrimSpace(lazy.run.SessionID) != "" {
		fmt.Fprintln(os.Stderr, "Resume with: workbench resume "+lazy.run.SessionID)
	}
	// Treat Ctrl-C as a successful exit so Cobra doesn't print usage/errors.
	if wasInterrupted {
		retErr = nil
	}

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
func RunChatTUI(ctx context.Context, cfg config.Config, run types.Run, opts ...RunChatOption) (retErr error) {
	if err := cfg.Validate(); err != nil {
		return err
	}
	resolved := resolveRunChatOptions(opts...)

	// The TUI owns stdout/stderr. Avoid mixing standard log output into the screen.
	oldLogWriter := log.Writer()
	log.SetOutput(io.Discard)
	defer log.SetOutput(oldLogWriter)

	runCtx, stopSignals := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stopSignals()
	wasInterrupted := false

	defer func() {
		status := types.StatusDone
		errMsg := ""
		if runCtx.Err() != nil || wasInterrupted {
			status = types.StatusCanceled
			errMsg = "interrupted"
		}
		if retErr != nil {
			status = types.StatusFailed
			errMsg = retErr.Error()
		}
		_, _ = store.StopRun(cfg, run.RunId, status, errMsg)
	}()

	historyRes, err := resources.NewHistoryResource(cfg, run.SessionID)
	if err != nil {
		return fmt.Errorf("create history: %w", err)
	}
	historySink := &events.HistorySink{Store: historyRes.Appender}

	// Stream events into the TUI, while still persisting them to the run event log
	// and session history.
	evCh := make(chan events.Event, 2048)
	_, mustEmit := newTUIEmitter(cfg, run.RunId, historySink, evCh, true)
	boolp := func(b bool) *bool { return &b }

	sessionTitle := ""
	sessionModel := ""
	sessionReasoningEffort := ""
	sessionReasoningSummary := ""
	if sess, err := store.LoadSession(cfg, run.SessionID); err == nil {
		sessionTitle = strings.TrimSpace(sess.Title)
		sessionModel = strings.TrimSpace(sess.ActiveModel)
		sessionReasoningEffort = strings.TrimSpace(sess.ReasoningEffort)
		sessionReasoningSummary = strings.TrimSpace(sess.ReasoningSummary)
	}

	model := strings.TrimSpace(resolved.Model)
	if model == "" {
		model = sessionModel
	}
	if model == "" {
		model = strings.TrimSpace(os.Getenv("OPENROUTER_MODEL"))
	}
	if strings.TrimSpace(os.Getenv("OPENROUTER_API_KEY")) == "" || model == "" {
		return fmt.Errorf("OPENROUTER_API_KEY and OPENROUTER_MODEL (or --model or session.activeModel) are required")
	}
	resolved.Model = model

	// Session-scoped reasoning defaults: prefer session preference over env/default.
	// (We don't currently expose a CLI flag, so session should win for deterministic resume.)
	if strings.TrimSpace(sessionReasoningEffort) != "" {
		resolved.ReasoningEffort = sessionReasoningEffort
	}
	if strings.TrimSpace(sessionReasoningSummary) != "" {
		resolved.ReasoningSummary = sessionReasoningSummary
	}

	// Resolve pricing against the effective model (session-aware).
	//
	// If pricing is not known for the model, cost is shown as "unknown".
	if resolved.PriceInPerMTokensUSD <= 0 && resolved.PriceOutPerMTokensUSD <= 0 {
		if inPerM, outPerM, ok, _ := pricingForModel(model, resolved.PricingFile); ok {
			resolved.PriceInPerMTokensUSD = inPerM
			resolved.PriceOutPerMTokensUSD = outPerM
		}
	}

	mustEmit(context.Background(), events.Event{
		Type:    "run.started",
		Message: "Run started",
		Data: map[string]string{
			"sessionId":    run.SessionID,
			"sessionTitle": sessionTitle,
			"runId":        run.RunId,
			"userId":       strings.TrimSpace(resolved.UserID),
		},
		Console: boolp(false),
	})

	historySink.Model = model
	setHistoryModel := func(next string) { historySink.Model = strings.TrimSpace(next) }
	setup, err := setupTUIChatRuntime(cfg, run, resolved, model, historyRes, mustEmit)
	if err != nil {
		return err
	}

	engine := &tuiTurnRunner{
		ctx:              runCtx,
		cfg:              cfg,
		run:              run,
		opts:             resolved,
		fs:               setup.FS,
		agent:            setup.Agent,
		baseSystemPrompt: setup.BaseSystemPrompt,
		mustEmit:         mustEmit,
		memStore:         setup.MemStore,
		profileStore:     setup.ProfileStore,
		memEval:          agent.DefaultMemoryEvaluator(),
		profileEval:      agent.DefaultProfileEvaluator(),
		model:            model,
		setHistoryModel:  setHistoryModel,
		workdirBase:      setup.WorkdirBase,
		builtinInvokers:  setup.BuiltinInvokers,
		artifacts:        setup.Artifacts,
		constructor:      setup.Constructor,
	}

	err = tui.Run(runCtx, engine, evCh)
	wasInterrupted = errors.Is(err, tea.ErrInterrupted)
	// If the user interrupted the session (Ctrl-C), print a convenient resume command
	// after the TUI exits.
	if shouldPrintResumeHint(runCtx, err) && strings.TrimSpace(run.SessionID) != "" {
		fmt.Fprintln(os.Stderr, "Resume with: workbench resume "+run.SessionID)
	}
	// Treat Ctrl-C as a successful exit so Cobra doesn't print usage/errors.
	if wasInterrupted {
		err = nil
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
	close(evCh)
	return err
}

func shouldPrintResumeHint(runCtx context.Context, err error) bool {
	// Prefer the context signal when it exists, but Bubble Tea typically returns
	// tea.ErrInterrupted on Ctrl-C without necessarily canceling our NotifyContext.
	if runCtx != nil && runCtx.Err() != nil {
		return true
	}
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, tea.ErrInterrupted) {
		return true
	}
	return false
}

type lazyNewSessionTurnRunner struct {
	ctx context.Context
	cfg config.Config

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
	case "datadir":
		return strings.TrimSpace(r.cfg.DataDir), true, nil
	case "editor":
		cur, err := resolveWorkDir(r.opts.WorkDir)
		if err != nil {
			return "", true, err
		}
		if strings.TrimSpace(arg) == "" {
			// /editor (no args): compose a message in $EDITOR.
			composeAbs := filepath.Join(strings.TrimSpace(r.cfg.DataDir), "compose.md")
			_ = os.MkdirAll(filepath.Dir(composeAbs), 0755)
			_ = os.WriteFile(composeAbs, []byte{}, 0644) // start blank each time

			r.emitPreInit(events.Event{
				Type:    "ui.editor.open",
				Message: "Compose message",
				Data: map[string]string{
					"vpath":   "/project/.workbench/compose.md", // legacy; TUI prefers absPath for compose
					"path":    "compose.md",
					"purpose": "compose",
					"absPath": composeAbs,
					// Include the absolute workdir so the TUI can open $EDITOR without
					// requiring a session/run to exist.
					"workdir": cur,
				},
				Store:   boolp(false),
				Console: boolp(false),
			})
			return "", true, nil
		}

		// /editor <path>: edit a file under workdir.
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
	case "web":
		// `/web` (no args): toggle web search for the next run (run-scoped).
		if strings.TrimSpace(arg) != "" {
			return "Usage: /web", true, nil
		}
		r.opts.WebSearchEnabled = !r.opts.WebSearchEnabled
		state := "off"
		if r.opts.WebSearchEnabled {
			state = "on"
		}
		r.emitPreInit(events.Event{
			Type:    "web.changed",
			Message: "Web search toggled",
			Data: map[string]string{
				"enabled": fmtBool(r.opts.WebSearchEnabled),
				"state":   state,
			},
			Store:   boolp(false),
			History: boolp(false),
			Console: boolp(false),
		})
		return "web search: " + state, true, nil
	case "reasoning":
		// Pre-init reasoning commands should not create a new session/run.
		curEffort := strings.TrimSpace(r.opts.ReasoningEffort)
		curSummary := strings.TrimSpace(r.opts.ReasoningSummary)

		if strings.TrimSpace(arg) == "" {
			out := "Reasoning:\n" +
				"  effort:  " + defaultIfEmpty(curEffort, "(default)") + "\n" +
				"  summary: " + defaultIfEmpty(curSummary, "(default)")
			r.emitPreInit(events.Event{
				Type:    "reasoning.info",
				Message: "Reasoning",
				Data:    map[string]string{"text": out},
				Store:   boolp(false),
				Console: boolp(false),
			})
			return out, true, nil
		}

		fields := strings.Fields(arg)
		if len(fields) < 2 {
			return "Usage: /reasoning effort <none|minimal|low|medium|high|xhigh> OR /reasoning summary <off|auto|concise|detailed>", true, nil
		}

		kind := strings.ToLower(strings.TrimSpace(fields[0]))
		val := strings.ToLower(strings.TrimSpace(fields[1]))
		switch kind {
		case "effort":
			switch val {
			case "none", "minimal", "low", "medium", "high", "xhigh":
				r.opts.ReasoningEffort = val
				r.emitPreInit(events.Event{
					Type:    "reasoning.changed",
					Message: "Reasoning effort changed",
					Data:    map[string]string{"effort": val, "summary": curSummary},
					Store:   boolp(false),
					Console: boolp(false),
				})
				return "", true, nil
			default:
				return "Invalid effort. Use: none|minimal|low|medium|high|xhigh", true, nil
			}
		case "summary":
			switch val {
			case "off", "auto", "concise", "detailed":
				r.opts.ReasoningSummary = val
				r.emitPreInit(events.Event{
					Type:    "reasoning.changed",
					Message: "Reasoning summary changed",
					Data:    map[string]string{"effort": curEffort, "summary": val},
					Store:   boolp(false),
					Console: boolp(false),
				})
				return "", true, nil
			default:
				return "Invalid summary. Use: off|auto|concise|detailed", true, nil
			}
		default:
			return "Usage: /reasoning effort <...> OR /reasoning summary <...>", true, nil
		}
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
			// /project <path> behaves like /cd <path>.
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
// or /scratch/<file>) without changing the agent loop contract or introducing a new host op.
//
// For lazyNewSessionTurnRunner, this is only available after the first turn initializes
// the session/run and mounts the VFS.
func (r *lazyNewSessionTurnRunner) ReadVFS(ctx context.Context, path string, maxBytes int) (text string, bytesLen int, truncated bool, err error) {
	if r == nil {
		return "", 0, false, fmt.Errorf("runner is nil")
	}
	// Support reading from /project before the first user message, so /editor can be
	// used without forcing a session/run to be created.
	if !r.initialized || r.engine == nil {
		if !strings.HasPrefix(strings.TrimSpace(path), "/project/") {
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
		sub := strings.TrimPrefix(strings.TrimSpace(path), "/project/")
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
	// Support writing under /project before the first user message, so /editor can
	// save without creating a session/run.
	if !r.initialized || r.engine == nil {
		if !strings.HasPrefix(strings.TrimSpace(path), "/project/") {
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
		sub := strings.TrimPrefix(strings.TrimSpace(path), "/project/")
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
	if err := r.cfg.Validate(); err != nil {
		return err
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

	cfg := r.cfg
	sess, run, err := store.CreateSession(cfg, goal, r.maxContextB)
	if err != nil {
		return err
	}
	r.run = run
	sess.Title = title

	historyRes, err := resources.NewHistoryResource(cfg, run.SessionID)
	if err != nil {
		return fmt.Errorf("create history: %w", err)
	}
	historySink := &events.HistorySink{Store: historyRes.Appender}

	_, r.mustEmit = newTUIEmitter(cfg, run.RunId, historySink, r.evCh, false)
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
	_ = store.SaveSession(cfg, sess)

	setup, err := setupTUIChatRuntime(cfg, run, r.opts, model, historyRes, r.mustEmit)
	if err != nil {
		return err
	}

	r.engine = &tuiTurnRunner{
		ctx:              r.ctx,
		cfg:              cfg,
		run:              run,
		opts:             r.opts,
		fs:               setup.FS,
		agent:            setup.Agent,
		baseSystemPrompt: setup.BaseSystemPrompt,
		mustEmit:         r.mustEmit,
		memStore:         setup.MemStore,
		profileStore:     setup.ProfileStore,
		memEval:          agent.DefaultMemoryEvaluator(),
		profileEval:      agent.DefaultProfileEvaluator(),
		model:            model,
		setHistoryModel:  setHistoryModel,
		workdirBase:      setup.WorkdirBase,
		builtinInvokers:  setup.BuiltinInvokers,
		artifacts:        setup.Artifacts,
		constructor:      setup.Constructor,
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

	cfg config.Config

	run  types.Run
	opts RunChatOptions

	fs *vfs.FS

	agent            *agent.Agent
	baseSystemPrompt string
	model            string
	setHistoryModel  func(model string)

	mustEmit func(ctx context.Context, ev events.Event)

	memStore     store.MemoryCommitter
	profileStore store.ProfileCommitter
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
// /results/<callId>/response.json and /scratch/<file>). This keeps the "open
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
	// These commands operate on the host's /project mount and are safe to run
	// during an interactive session (no restart required).
	if resp, handled := r.handleSlashCommand(userMsg); handled {
		return resp, nil
	}

	// Refresh session state and inject it so the agent stays coherent across runs.
	if sess, err := store.LoadSession(r.cfg, r.run.SessionID); err == nil {
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
	//   host: resolves /project/go.mod and injects it into the system prompt
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

	// Emit a UI-only activity entry when the provider returns web citations.
	// This is the best available signal that web-search grounding actually occurred.
	r.agent.Hooks.OnWebSearch = func(step int, citations []types.LLMCitation) {
		if len(citations) == 0 {
			return
		}
		var b strings.Builder
		for _, c := range citations {
			u := strings.TrimSpace(c.URL)
			if u == "" {
				continue
			}
			title := strings.TrimSpace(c.Title)
			if title == "" {
				title = u
			}
			b.WriteString("- [")
			b.WriteString(title)
			b.WriteString("](")
			b.WriteString(u)
			b.WriteString(")\n")
		}
		sources := strings.TrimSpace(b.String())
		if sources == "" {
			return
		}
		r.mustEmit(context.Background(), events.Event{
			Type:    "llm.web.search",
			Message: "Web search used",
			Data: map[string]string{
				"step":    strconv.Itoa(step),
				"count":   strconv.Itoa(len(citations)),
				"sources": sources,
			},
			Store:   boolp(false),
			History: boolp(false),
			Console: boolp(false),
		})
	}

	// Phase 1 streaming: emit UI-only token events (not persisted).
	//
	// Note: the agent loop already decodes and streams only "final.text".
	var tokenBuf strings.Builder
	lastTokenEmit := time.Now()
	emitTokenBuf := func() {
		if tokenBuf.Len() == 0 {
			return
		}
		txt := tokenBuf.String()
		tokenBuf.Reset()
		lastTokenEmit = time.Now()
		r.mustEmit(context.Background(), events.Event{
			Type:    "model.token",
			Message: "Model token",
			Data:    map[string]string{"text": txt},
			Store:   boolp(false),
			History: boolp(false),
			Console: boolp(false),
		})
	}
	r.agent.Hooks.OnToken = func(step int, text string) {
		_ = step
		if text == "" {
			return
		}
		tokenBuf.WriteString(text)
		// Coalesce to avoid flooding the TUI channel.
		if tokenBuf.Len() >= 256 || time.Since(lastTokenEmit) >= 40*time.Millisecond {
			emitTokenBuf()
		}
	}

	// Phase 2 thinking: emit UI-only thinking events (not persisted).
	//
	// We never display raw reasoning content; we only show an indicator + optional
	// provider-supplied summary.
	var thinkingSummaryBuf strings.Builder
	thinkingActive := false
	thinkingStep := 0
	sawReasoningChunk := false
	sawNonReasoningText := false
	lastThinkingEmit := time.Now()
	emitThinkingSummary := func() {
		if thinkingSummaryBuf.Len() == 0 || thinkingStep == 0 {
			return
		}
		txt := thinkingSummaryBuf.String()
		thinkingSummaryBuf.Reset()
		lastThinkingEmit = time.Now()
		r.mustEmit(context.Background(), events.Event{
			Type:    "model.thinking.summary",
			Message: "Model thinking summary",
			Data: map[string]string{
				"step": strconv.Itoa(thinkingStep),
				"text": txt,
			},
			Store:   boolp(false),
			History: boolp(false),
			Console: boolp(false),
		})
	}
	emitThinkingStart := func(step int) {
		thinkingActive = true
		thinkingStep = step
		lastThinkingEmit = time.Now()
		// #region agent log
		cursorDebugLog("H4", "chat_tui.go:emitThinkingStart", "thinking_start_emit", map[string]any{
			"model": strings.TrimSpace(r.model),
			"step":  step,
		})
		// #endregion
		r.mustEmit(context.Background(), events.Event{
			Type:    "model.thinking.start",
			Message: "Model thinking",
			Data:    map[string]string{"step": strconv.Itoa(step)},
			Store:   boolp(false),
			History: boolp(false),
			Console: boolp(false),
		})
	}
	emitThinkingEnd := func(step int) {
		emitThinkingSummary()
		thinkingActive = false
		thinkingStep = 0
		// #region agent log
		cursorDebugLog("H4", "chat_tui.go:emitThinkingEnd", "thinking_end_emit", map[string]any{
			"model": strings.TrimSpace(r.model),
			"step":  step,
		})
		// #endregion
		r.mustEmit(context.Background(), events.Event{
			Type:    "model.thinking.end",
			Message: "Model thinking ended",
			Data:    map[string]string{"step": strconv.Itoa(step)},
			Store:   boolp(false),
			History: boolp(false),
			Console: boolp(false),
		})
	}

	// Some providers/models (notably Anthropic via OpenRouter) may not emit separate
	// reasoning/thinking stream chunks that we can classify as `IsReasoning`.
	// For a consistent UX, proactively start a Thinking block for models we consider
	// reasoning-capable, and then:
	// - fill it with provider-supplied summaries when available
	// - otherwise, show only the duration (ended on stream Done)
	if cost.SupportsReasoningSummary(strings.TrimSpace(r.model)) {
		emitThinkingStart(1)
		// #region agent log
		cursorDebugLog("H3", "chat_tui.go:RunTurn", "auto_thinking_start", map[string]any{
			"model": strings.TrimSpace(r.model),
			"step":  1,
		})
		// #endregion
	}

	r.agent.Hooks.OnStreamChunk = func(step int, chunk types.LLMStreamChunk) {
		if chunk.Done {
			if thinkingActive {
				emitThinkingEnd(step)
			}
			return
		}
		if !chunk.IsReasoning {
			if !sawNonReasoningText && chunk.Text != "" {
				sawNonReasoningText = true
				// #region agent log
				cursorDebugLog("H3", "chat_tui.go:OnStreamChunk", "first_non_reasoning_text_chunk", map[string]any{
					"step":    step,
					"model":   strings.TrimSpace(r.model),
					"textLen": len(chunk.Text),
				})
				// #endregion
			}
			return
		}
		if !sawReasoningChunk {
			sawReasoningChunk = true
			// #region agent log
			cursorDebugLog("H3", "chat_tui.go:OnStreamChunk", "first_reasoning_chunk", map[string]any{
				"step":    step,
				"model":   strings.TrimSpace(r.model),
				"hasText": strings.TrimSpace(chunk.Text) != "",
				"textLen": len(chunk.Text),
			})
			// #endregion
		}
		if !thinkingActive || thinkingStep != step {
			// If a prior step was thinking, close it before starting a new one.
			if thinkingActive && thinkingStep != 0 {
				emitThinkingEnd(thinkingStep)
			}
			emitThinkingStart(step)
		}
		if chunk.Text == "" {
			// Raw reasoning signal: indicator only.
			return
		}
		thinkingSummaryBuf.WriteString(chunk.Text)
		// Coalesce to avoid flooding the TUI channel.
		if thinkingSummaryBuf.Len() >= 256 || time.Since(lastThinkingEmit) >= 80*time.Millisecond {
			emitThinkingSummary()
		}
	}
	defer func() {
		emitTokenBuf()
		// Best-effort flush of thinking summary/indicator.
		if thinkingActive && thinkingStep != 0 {
			emitThinkingEnd(thinkingStep)
		}
		// Avoid leaking the callback into subsequent turns.
		r.agent.Hooks.OnToken = nil
		r.agent.Hooks.OnStreamChunk = nil
	}()

	final := ""
	steps := 0
	dur := time.Duration(0)

	// Normal turn: add the user's message and run the agent.
	r.conversation = append(r.conversation, types.LLMMessage{Role: "user", Content: userMsg})
	start := time.Now()
	out, updated, stepCount, err := r.agent.RunConversation(ctx, r.conversation)
	dur = time.Since(start)
	steps = stepCount
	r.conversation = updated
	if err != nil {
		// User-initiated stop should not be surfaced as an agent error event.
		if errors.Is(err, context.Canceled) {
			return "", err
		}
		r.mustEmit(context.Background(), events.Event{
			Type:    "agent.error",
			Message: "Agent loop error",
			Data:    map[string]string{"err": err.Error()},
			Store:   boolp(false),
		})
		return "", err
	}

	final = out

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

	// Memory/profile update ingestion (via shared ContentUpdateProcessor).
	_ = (&ContentUpdateProcessor{
		FS:         r.fs,
		Evaluator:  r.memEval,
		Store:      memoryCommitterAdapter{r.memStore},
		Scope:      "memory",
		UpdatePath: "/memory/update.md",
		Emit:       r.mustEmit,
		EmitAudit:  false,
	}).ProcessUpdate(context.Background(), r.turn, r.run.SessionID, r.run.RunId, r.model)

	_ = (&ContentUpdateProcessor{
		FS:         r.fs,
		Evaluator:  r.profileEval,
		Store:      profileCommitterAdapter{r.profileStore},
		Scope:      "profile",
		UpdatePath: "/profile/update.md",
		Emit:       r.mustEmit,
		EmitAudit:  false,
	}).ProcessUpdate(context.Background(), r.turn, r.run.SessionID, r.run.RunId, r.model)

	if _, err := store.RecordTurnInSession(r.cfg, r.run.SessionID, r.run.RunId, userMsg, final); err != nil {
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
	case "datadir":
		return strings.TrimSpace(r.cfg.DataDir), true
	case "editor":
		if strings.TrimSpace(arg) == "" {
			// /editor (no args): compose a message in $EDITOR.
			wd := strings.TrimSpace(r.workdirBase)
			composeAbs := filepath.Join(strings.TrimSpace(r.cfg.DataDir), "compose.md")
			_ = os.MkdirAll(filepath.Dir(composeAbs), 0755)
			_ = os.WriteFile(composeAbs, []byte{}, 0644) // start blank each time

			r.mustEmit(context.Background(), events.Event{
				Type:    "ui.editor.open",
				Message: "Compose message",
				Data: map[string]string{
					"vpath":   "/project/.workbench/compose.md", // legacy; TUI prefers absPath for compose
					"path":    "compose.md",
					"purpose": "compose",
					"absPath": composeAbs,
					"workdir": wd,
				},
				Store:   boolp(false),
				Console: boolp(false),
			})
			return "", true
		}

		// /editor <path>: edit a file under workdir.
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
	case "web":
		// `/web` (no args): toggle web search for this run (run-scoped).
		if strings.TrimSpace(arg) != "" {
			return "Usage: /web", true
		}
		r.opts.WebSearchEnabled = !r.opts.WebSearchEnabled
		if r.agent != nil {
			r.agent.EnableWebSearch = r.opts.WebSearchEnabled
		}
		state := "off"
		if r.opts.WebSearchEnabled {
			state = "on"
		}
		r.mustEmit(context.Background(), events.Event{
			Type:    "web.changed",
			Message: "Web search toggled",
			Data: map[string]string{
				"enabled": fmtBool(r.opts.WebSearchEnabled),
				"state":   state,
			},
			Store:   boolp(false),
			History: boolp(false),
			Console: boolp(false),
		})
		return "web search: " + state, true
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
		_ = store.SaveRun(r.cfg, r.run)

		// Persist at session scope so resume uses the selected model.
		if sess, err := store.LoadSession(r.cfg, r.run.SessionID); err == nil {
			sess.ActiveModel = next
			_ = store.SaveSession(r.cfg, sess)
		}

		// #region agent log
		cursorDebugLog("H1", "chat_tui.go:handleSlashCommand", "model_changed_applied", map[string]any{
			"from":        strings.TrimSpace(cur),
			"to":          strings.TrimSpace(next),
			"runnerModel": strings.TrimSpace(r.model),
			"optsModel":   strings.TrimSpace(r.opts.Model),
			"agentModel":  strings.TrimSpace(r.agent.Model),
			"runRuntimeModel": func() string {
				if r.run.Runtime == nil {
					return ""
				}
				return strings.TrimSpace(r.run.Runtime.Model)
			}(),
		})
		// #endregion

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
	case "reasoning":
		curEffort := strings.TrimSpace(r.opts.ReasoningEffort)
		curSummary := strings.TrimSpace(r.opts.ReasoningSummary)

		// `/reasoning` (no args): show current settings.
		if strings.TrimSpace(arg) == "" {
			out := "Reasoning:\n" +
				"  effort:  " + defaultIfEmpty(curEffort, "(default)") + "\n" +
				"  summary: " + defaultIfEmpty(curSummary, "(default)")
			r.mustEmit(context.Background(), events.Event{
				Type:    "reasoning.info",
				Message: "Reasoning",
				Data:    map[string]string{"text": out},
				Console: boolp(false),
			})
			return out, true
		}

		fields := strings.Fields(arg)
		if len(fields) < 2 {
			return "Usage: /reasoning effort <none|minimal|low|medium|high|xhigh> OR /reasoning summary <off|auto|concise|detailed>", true
		}

		kind := strings.ToLower(strings.TrimSpace(fields[0]))
		val := strings.ToLower(strings.TrimSpace(fields[1]))

		applyAndPersist := func() {
			// Apply to opts + live agent instance
			r.opts.ReasoningEffort = strings.TrimSpace(curEffort)
			r.opts.ReasoningSummary = strings.TrimSpace(curSummary)
			if r.agent != nil {
				r.agent.ReasoningEffort = strings.TrimSpace(curEffort)
				r.agent.ReasoningSummary = strings.TrimSpace(curSummary)
			}

			// Persist to run + session state (best-effort)
			if r.run.Runtime == nil {
				r.run.Runtime = &types.RunRuntimeConfig{}
			}
			r.run.Runtime.ReasoningEffort = strings.TrimSpace(curEffort)
			r.run.Runtime.ReasoningSummary = strings.TrimSpace(curSummary)
			_ = store.SaveRun(r.cfg, r.run)

			if sess, err := store.LoadSession(r.cfg, r.run.SessionID); err == nil {
				sess.ReasoningEffort = strings.TrimSpace(curEffort)
				sess.ReasoningSummary = strings.TrimSpace(curSummary)
				_ = store.SaveSession(r.cfg, sess)
			}
		}

		switch kind {
		case "effort":
			switch val {
			case "none", "minimal", "low", "medium", "high", "xhigh":
				curEffort = val
				applyAndPersist()
				r.mustEmit(context.Background(), events.Event{
					Type:    "reasoning.changed",
					Message: "Reasoning effort changed",
					Data:    map[string]string{"effort": curEffort, "summary": curSummary},
					Console: boolp(false),
				})
				return "", true
			default:
				return "Invalid effort. Use: none|minimal|low|medium|high|xhigh", true
			}
		case "summary":
			switch val {
			case "off", "auto", "concise", "detailed":
				curSummary = val
				applyAndPersist()
				r.mustEmit(context.Background(), events.Event{
					Type:    "reasoning.changed",
					Message: "Reasoning summary changed",
					Data:    map[string]string{"effort": curEffort, "summary": curSummary},
					Console: boolp(false),
				})
				return "", true
			default:
				return "Invalid summary. Use: off|auto|concise|detailed", true
			}
		default:
			return "Usage: /reasoning effort <...> OR /reasoning summary <...>", true
		}
	case "cd":
		from := strings.TrimSpace(r.workdirBase)
		if from == "" {
			// Should not happen: /project is always mounted for a run.
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

		// Rebind /project to the new root. All file operations immediately target the new directory.
		r.fs.Mount(vfs.MountProject, workdirRes)
		r.workdirBase = workdirRes.BaseDir
		r.opts.WorkDir = workdirRes.BaseDir

		// Update builtin sandbox roots (builtin.shell) to follow the active workdir.
		if r.builtinInvokers != nil {
			r.builtinInvokers[types.ToolID("builtin.shell")] = tools.NewBuiltinShellInvoker(workdirRes.BaseDir, nil, vfs.MountProject)
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
		// /project with no args behaves like /pwd.
		// /project <path> is an alias for /cd <path>.
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

func defaultIfEmpty(v, fallback string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return fallback
	}
	return v
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
		toks := atref.ExtractAtRefs(arg)
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
		rel, err := vfsutil.RelUnderBaseDir(workdirBase, ref)
		if err != nil {
			return "", "", fmt.Errorf("path must be within workdir")
		}
		ref = rel
	}

	clean, _, err := vfsutil.NormalizeResourceSubpath(ref)
	if err != nil || clean == "" || clean == "." {
		return "", "", fmt.Errorf("invalid path: %s", ref)
	}
	return "/project/" + clean, clean, nil
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

	dstVPath := "/project/" + clean
	// Avoid overwriting existing files by default. If it exists, publish under
	// "/project/.workbench/<name>".
	if _, err := r.fs.Read(dstVPath); err == nil {
		dstVPath = "/project/.workbench/" + clean
	}
	if err := r.fs.Write(dstVPath, b); err != nil {
		return "", fmt.Errorf("publish: write %s: %w", dstVPath, err)
	}
	return dstVPath, nil
}
