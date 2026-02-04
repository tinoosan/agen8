package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/google/uuid"
	implstore "github.com/tinoosan/workbench-core/internal/store"
	"github.com/tinoosan/workbench-core/pkg/agent"
	agentevents "github.com/tinoosan/workbench-core/pkg/agent/events"
	hosttools "github.com/tinoosan/workbench-core/pkg/agent/hosttools"
	"github.com/tinoosan/workbench-core/pkg/agent/session"
	"github.com/tinoosan/workbench-core/pkg/agent/state"
	"github.com/tinoosan/workbench-core/pkg/config"
	"github.com/tinoosan/workbench-core/pkg/cost"
	"github.com/tinoosan/workbench-core/pkg/events"
	"github.com/tinoosan/workbench-core/pkg/fsutil"
	"github.com/tinoosan/workbench-core/pkg/llm"
	llmtypes "github.com/tinoosan/workbench-core/pkg/llm/types"
	"github.com/tinoosan/workbench-core/pkg/profile"
	"github.com/tinoosan/workbench-core/pkg/runtime"
	"github.com/tinoosan/workbench-core/pkg/store"
	"github.com/tinoosan/workbench-core/pkg/types"
)

// RunDaemon starts a headless worker that continuously polls /inbox and writes results to /outbox.
// It is intended as the default autonomous entrypoint; the TUI can be used separately as a viewer.
func RunDaemon(ctx context.Context, cfg config.Config, goal string, maxContextB int, poll time.Duration, opts ...RunChatOption) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	resolved, err := resolveRunChatOptions(opts...)
	if err != nil {
		return err
	}
	if err := maybeSeedRepoDefaults(cfg.DataDir); err != nil {
		return err
	}
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
	_, run, err := implstore.CreateSession(cfg, goal, maxContextB)
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}

	emitter := &events.Emitter{
		RunID: run.RunID,
		Sink: events.MultiSink{
			events.StoreSink{Store: daemonEventAppender{cfg: cfg}},
		},
	}
	mustEmit := func(ctx context.Context, ev events.Event) {
		if err := emitter.Emit(ctx, ev); err != nil && !errors.Is(err, events.ErrDropped) {
			log.Printf("events: emit failed: %v", err)
		}
	}

	artifactIndex := newArtifactIndex()
	workdirAbs, err := resolveWorkDir(resolved.WorkDir)
	if err != nil {
		return err
	}

	// Load .env (best-effort) so onboarding can be "drop a file and go".
	// Existing environment variables always win (no override).
	//
	// We load from both:
	//   - the current working directory
	//   - the resolved workdir (mounted at /project)
	//
	// This supports workflows where users run the daemon from a different directory
	// than the mounted repo.
	if cwd, err := os.Getwd(); err == nil {
		if derr := loadDotEnvFromDir(cwd); derr != nil {
			mustEmit(ctx, events.Event{
				Type:    "daemon.warning",
				Message: ".env load failed (cwd); continuing",
				Data:    map[string]string{"error": derr.Error()},
			})
		}
		if strings.TrimSpace(workdirAbs) != "" && strings.TrimSpace(workdirAbs) != strings.TrimSpace(cwd) {
			if derr := loadDotEnvFromDir(workdirAbs); derr != nil {
				mustEmit(ctx, events.Event{
					Type:    "daemon.warning",
					Message: ".env load failed (workdir); continuing",
					Data:    map[string]string{"error": derr.Error()},
				})
			}
		}
	} else if strings.TrimSpace(workdirAbs) != "" {
		if derr := loadDotEnvFromDir(workdirAbs); derr != nil {
			mustEmit(ctx, events.Event{
				Type:    "daemon.warning",
				Message: ".env load failed (workdir); continuing",
				Data:    map[string]string{"error": derr.Error()},
			})
		}
	}

	var memStore store.DailyMemoryStore
	var traceStore store.TraceStore
	var historyStore store.HistoryStore
	var constructorStore store.ConstructorStateStore

	ms, err := implstore.NewDiskMemoryStore(cfg)
	if err != nil {
		return fmt.Errorf("create memory store: %w", err)
	}
	memStore = ms

	traceStore = implstore.DiskTraceStore{DiskStore: implstore.DiskStore{Dir: fsutil.GetLogDir(cfg.DataDir, run.RunID)}}

	hs, err := implstore.NewSQLiteHistoryStore(cfg, run.SessionID)
	if err != nil {
		return fmt.Errorf("create history store: %w", err)
	}
	historyStore = hs

	cs, err := implstore.NewSQLiteConstructorStore(cfg)
	if err != nil {
		return fmt.Errorf("create constructor store: %w", err)
	}
	constructorStore = cs

	// Vector memory store (SQLite-backed) for semantic recall.
	// Best-effort: daemon can still run without this, but loses long-term recall.
	var memoryProvider agent.MemoryRecallProvider
	var vectorStore *implstore.VectorMemoryStore
	if vm, err := implstore.NewVectorMemoryStore(cfg); err == nil {
		vectorStore = vm
		memoryProvider = &vectorMemoryAdapter{store: vm}
	} else {
		mustEmit(ctx, events.Event{
			Type:    "daemon.warning",
			Message: "Vector memory disabled",
			Data:    map[string]string{"error": err.Error()},
		})
	}

	var notifier agent.Notifier
	if strings.TrimSpace(resolved.ResultWebhookURL) != "" {
		notifier = WebhookNotifier{URL: resolved.ResultWebhookURL}
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
		MemoryStore:           memStore,
		TraceStore:            traceStore,
		MemoryReindexer:       vectorStore,
		ConstructorStore:      constructorStore,
		Emit:                  mustEmit,
		IncludeHistoryOps:     derefBool(resolved.IncludeHistoryOps, true),
		RecentHistoryPairs:    resolved.RecentHistoryPairs,
		MaxMemoryBytes:        resolved.MaxMemoryBytes,
		MaxTraceBytes:         resolved.MaxTraceBytes,
		PriceInPerMTokensUSD:  resolved.PriceInPerMTokensUSD,
		PriceOutPerMTokensUSD: resolved.PriceOutPerMTokensUSD,
		Guard:                 nil,
		ArtifactObserve:       artifactIndex.ObserveWrite,
		PersistRun: func(r types.Run) error {
			return implstore.SaveRun(cfg, r)
		},
		LoadSession: func(sessionID string) (types.Session, error) {
			return implstore.LoadSession(cfg, sessionID)
		},
		SaveSession: func(session types.Session) error {
			return implstore.SaveSession(cfg, session)
		},
	})
	if err != nil {
		return err
	}
	defer func() { _ = rt.Shutdown(context.Background()) }()

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

	baseSystemPrompt := agent.DefaultAutonomousSystemPrompt()
	agentCfg := agent.DefaultConfig()
	agentCfg.Model = resolved.Model
	agentCfg.ReasoningEffort = strings.TrimSpace(resolved.ReasoningEffort)
	agentCfg.ReasoningSummary = strings.TrimSpace(resolved.ReasoningSummary)
	agentCfg.ApprovalsMode = strings.TrimSpace(resolved.ApprovalsMode)
	agentCfg.EnableWebSearch = resolved.WebSearchEnabled
	agentCfg.SystemPrompt = baseSystemPrompt
	var promptSource agent.PromptSource = rt.Constructor
	if rt.Updater != nil {
		promptSource = rt.Updater
	}
	agentCfg.PromptSource = promptSource
	agentCfg.Hooks = agent.Hooks{
		OnLLMUsage: newCostUsageHook(cfg, run, resolved.Model, resolved.PriceInPerMTokensUSD, resolved.PriceOutPerMTokensUSD, mustEmit),
		OnStep: func(step int, model, summary string) {
			model = strings.TrimSpace(model)
			summary = strings.TrimSpace(summary)
			data := map[string]string{
				"step":  strconv.Itoa(step),
				"model": model,
			}
			if summary != "" {
				data["reasoningSummary"] = summary
			}
			mustEmit(ctx, events.Event{
				Type:    "agent.step",
				Message: fmt.Sprintf("Step %d completed", step),
				Data:    data,
			})
		},
	}

	taskStore, err := state.NewSQLiteTaskStore(fsutil.GetSQLitePath(cfg.DataDir))
	if err != nil {
		return fmt.Errorf("create task store: %w", err)
	}

	// Seed default tools and inject the DB-backed task_create tool.
	registry, err := agent.DefaultHostToolRegistry()
	if err != nil {
		return fmt.Errorf("create host tool registry: %w", err)
	}
	if err := registry.Register(&hosttools.TaskCreateTool{
		Store:     taskStore,
		SessionID: run.SessionID,
		RunID:     run.RunID,
		InboxPath: "/inbox",
	}); err != nil {
		return fmt.Errorf("register task_create tool: %w", err)
	}
	agentCfg.HostToolRegistry = registry

	a, err := agent.NewAgent(llmClient, rt.Executor, agentCfg)
	if err != nil {
		return fmt.Errorf("create agent: %w", err)
	}
	prof, profDir, err := resolveProfileRef(cfg, strings.TrimSpace(resolved.Profile))
	if err != nil {
		return err
	}
	emitBlocking := func(ctx context.Context, ev events.Event) error {
		return emitter.Emit(ctx, ev)
	}
	ordered := agentevents.NewWriter(emitBlocking)

	sess, err := session.New(session.Config{
		Agent:      a,
		Profile:    prof,
		ProfileDir: profDir,
		ResolveProfile: func(ref string) (*profile.Profile, string, error) {
			return resolveProfileRef(cfg, strings.TrimSpace(ref))
		},
		TaskStore:         taskStore,
		Events:            ordered,
		Memory:            memoryProvider,
		MemorySearchLimit: 3,
		Notifier:          notifier,
		InboxPath:         "/inbox",
		OutboxPath:        "/outbox",
		PollInterval:      poll,
		MaxReadBytes:      256 * 1024,
		LeaseTTL:          2 * time.Minute,
		MaxRetries:        3,
		MaxPending:        50,
		SessionID:         run.SessionID,
		RunID:             run.RunID,
		InstanceID:        run.RunID,
		Logf: func(format string, args ...any) {
			log.Printf("daemon: "+format, args...)
		},
	})
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}

	runCtx, stopSignals := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stopSignals()

	if vectorStore != nil {
		go func() {
			ticker := time.NewTicker(5 * time.Minute)
			defer ticker.Stop()
			for {
				select {
				case <-runCtx.Done():
					return
				case <-ticker.C:
					_ = vectorStore.IndexAllDailyFiles(runCtx)
				}
			}
		}()
	}

	var serverWG sync.WaitGroup
	webhookAddr := strings.TrimSpace(resolved.WebhookAddr)
	if webhookAddr != "" {
		startWebhookServer(runCtx, webhookAddr, cfg, run, taskStore, mustEmit, &serverWG)
	}
	healthAddr := strings.TrimSpace(resolved.HealthAddr)
	if healthAddr != "" && healthAddr != webhookAddr {
		startHealthServer(runCtx, healthAddr, mustEmit, &serverWG)
	}

	mustEmit(runCtx, events.Event{
		Type:    "daemon.start",
		Message: "Autonomous agent started",
		Data:    map[string]string{"runId": run.RunID, "sessionId": run.SessionID, "profile": prof.ID},
	})
	log.Printf("daemon: agent id %s — attach monitor with: workbench monitor --agent-id %s", run.RunID, run.RunID)
	for {
		err = sess.Run(runCtx)
		if runCtx.Err() != nil {
			// context cancellation is expected on shutdown
			err = nil
			break
		}
		errMsg := "unknown error"
		if err != nil {
			errMsg = err.Error()
		}
		mustEmit(runCtx, events.Event{
			Type:    "daemon.runner.error",
			Message: "Runner exited unexpectedly; restarting",
			Data:    map[string]string{"error": errMsg},
		})
		time.Sleep(2 * time.Second)
	}
	mustEmit(runCtx, events.Event{
		Type:    "daemon.stop",
		Message: "Autonomous agent stopped",
		Data:    map[string]string{"runId": run.RunID, "sessionId": run.SessionID, "profile": prof.ID},
	})
	serverWG.Wait()
	return err
}

func resolveProfileRef(cfg config.Config, requested string) (*profile.Profile, string, error) {
	if err := cfg.Validate(); err != nil {
		return nil, "", err
	}
	requested = strings.TrimSpace(requested)
	if requested == "" {
		requested = "general"
	}
	if st, err := os.Stat(requested); err == nil {
		if st.IsDir() {
			p, err := profile.Load(requested)
			return p, requested, err
		}
		dir := filepath.Dir(requested)
		p, err := profile.Load(requested)
		return p, dir, err
	}
	dir := filepath.Join(fsutil.GetProfilesDir(cfg.DataDir), requested)
	p, err := profile.Load(dir)
	return p, dir, err
}

func newCostUsageHook(cfg config.Config, run types.Run, modelID string, priceIn, priceOut float64, emit func(context.Context, events.Event)) func(step int, usage llmtypes.LLMUsage) {
	pricingKnown := false
	if priceIn == 0 && priceOut == 0 {
		if in, out, ok := cost.DefaultPricing().Lookup(modelID); ok {
			priceIn = in
			priceOut = out
		}
	}
	if priceIn != 0 || priceOut != 0 {
		pricingKnown = true
	}

	var mu sync.Mutex
	var session types.Session
	sessionLoaded := false

	emitUsage := func(input, output, total int) {
		if emit == nil {
			return
		}
		// Use background context so usage events persist even if the run cancels.
		emit(context.Background(), events.Event{
			Type:    "llm.usage.total",
			Message: "LLM usage totals",
			Data: map[string]string{
				"input":  fmt.Sprintf("%d", input),
				"output": fmt.Sprintf("%d", output),
				"total":  fmt.Sprintf("%d", total),
			},
		})
	}
	emitCost := func(costUSD float64, known bool) {
		if emit == nil {
			return
		}
		// Use background context so cost events persist even if the run cancels.
		payload := map[string]string{
			"known": fmt.Sprintf("%t", known),
		}
		if known {
			payload["costUsd"] = fmt.Sprintf("%.4f", costUSD)
		}
		emit(context.Background(), events.Event{
			Type:    "llm.cost.total",
			Message: "LLM cost totals",
			Data:    payload,
		})
	}

	return func(step int, usage llmtypes.LLMUsage) {
		_ = step
		input := usage.InputTokens
		output := usage.OutputTokens
		total := usage.TotalTokens
		if total == 0 {
			total = input + output
		}

		emitUsage(input, output, total)

		costUSD := 0.0
		if pricingKnown {
			costUSD = (float64(input)/1_000_000.0)*priceIn + (float64(output)/1_000_000.0)*priceOut
		}
		emitCost(costUSD, pricingKnown)

		mu.Lock()
		defer mu.Unlock()
		run.TotalTokensIn += input
		run.TotalTokensOut += output
		run.TotalTokens += total
		if pricingKnown {
			run.TotalCostUSD += costUSD
		}
		if err := implstore.SaveRun(cfg, run); err != nil {
			log.Printf("daemon: warning: failed to save run: %v", err)
			if emit != nil {
				emit(context.Background(), events.Event{
					Type:    "daemon.warning",
					Message: "Failed to persist run state",
					Data:    map[string]string{"error": err.Error()},
				})
			}
		}

		if !sessionLoaded {
			if s, err := implstore.LoadSession(cfg, run.SessionID); err == nil {
				session = s
				sessionLoaded = true
			}
		}
		if sessionLoaded {
			session.TotalTokensIn += input
			session.TotalTokensOut += output
			session.TotalTokens += total
			if pricingKnown {
				session.TotalCostUSD += costUSD
			}
			if err := implstore.SaveSession(cfg, session); err != nil {
				log.Printf("daemon: warning: failed to save session: %v", err)
				if emit != nil {
					emit(context.Background(), events.Event{
						Type:    "daemon.warning",
						Message: "Failed to persist session state",
						Data:    map[string]string{"error": err.Error()},
					})
				}
			}
		}
	}
}

// daemonEventAppender adapts internal store.AppendEvent to events.StoreSink (daemon context).
type daemonEventAppender struct {
	cfg config.Config
}

func (s daemonEventAppender) AppendEvent(ctx context.Context, event types.EventRecord) error {
	return implstore.AppendEvent(ctx, s.cfg, event)
}

func startWebhookServer(ctx context.Context, addr string, cfg config.Config, run types.Run, taskStore state.TaskStore, emit func(context.Context, events.Event), wg *sync.WaitGroup) {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	mux.HandleFunc("/task", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		defer r.Body.Close()
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "read error: "+err.Error(), http.StatusBadRequest)
			return
		}
		var payload struct {
			TaskID   string         `json:"taskId"`
			Goal     string         `json:"goal"`
			Priority int            `json:"priority,omitempty"`
			Inputs   map[string]any `json:"inputs,omitempty"`
			Metadata map[string]any `json:"metadata,omitempty"`
		}
		if err := json.Unmarshal(raw, &payload); err != nil {
			http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}
		goal := strings.TrimSpace(payload.Goal)
		if goal == "" {
			http.Error(w, "goal is required", http.StatusBadRequest)
			return
		}
		taskID := strings.TrimSpace(payload.TaskID)
		if taskID == "" {
			taskID = "task-" + uuid.NewString()
		}
		now := time.Now()
		task := types.Task{
			TaskID:    taskID,
			SessionID: run.SessionID,
			RunID:     run.RunID,
			Goal:      goal,
			Priority:  payload.Priority,
			Status:    types.TaskStatusPending,
			CreatedAt: &now,
			Inputs:    payload.Inputs,
			Metadata:  payload.Metadata,
		}

		if taskStore == nil {
			http.Error(w, "task store not configured", http.StatusInternalServerError)
			return
		}
		if err := taskStore.CreateTask(ctx, task); err != nil {
			http.Error(w, "create task error: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// Best-effort archive of the inbound payload for external integrations/debugging.
		{
			runDir := fsutil.GetAgentDir(cfg.DataDir, run.RunID)
			archiveDir := filepath.Join(runDir, "inbox", "archive")
			_ = os.MkdirAll(archiveDir, 0o755)
			if b, err := json.MarshalIndent(task, "", "  "); err == nil {
				_ = os.WriteFile(filepath.Join(archiveDir, taskID+".json"), b, 0o644)
			}
		}

		if emit != nil {
			emit(ctx, events.Event{
				Type:    "webhook.task.queued",
				Message: "Webhook task queued",
				Data:    map[string]string{"taskId": taskID},
			})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"taskId": taskID, "status": "queued"})
	})

	srv := &http.Server{Addr: addr, Handler: mux}
	if wg != nil {
		wg.Add(2)
	}
	go func() {
		if wg != nil {
			defer wg.Done()
		}
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()
	go func() {
		if wg != nil {
			defer wg.Done()
		}
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			if emit != nil {
				emit(ctx, events.Event{
					Type:    "webhook.error",
					Message: "Webhook server error",
					Data:    map[string]string{"error": err.Error()},
				})
			}
		}
	}()
}

func startHealthServer(ctx context.Context, addr string, emit func(context.Context, events.Event), wg *sync.WaitGroup) {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	srv := &http.Server{Addr: addr, Handler: mux}
	if wg != nil {
		wg.Add(2)
	}
	go func() {
		if wg != nil {
			defer wg.Done()
		}
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()
	go func() {
		if wg != nil {
			defer wg.Done()
		}
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			if emit != nil {
				emit(ctx, events.Event{
					Type:    "health.error",
					Message: "Health server error",
					Data:    map[string]string{"error": err.Error()},
				})
			}
		}
	}()
}
