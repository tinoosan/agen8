package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/tinoosan/workbench-core/internal/store"
	"github.com/tinoosan/workbench-core/pkg/agent"
	"github.com/tinoosan/workbench-core/pkg/config"
	"github.com/tinoosan/workbench-core/pkg/events"
	"github.com/tinoosan/workbench-core/pkg/fsutil"
	"github.com/tinoosan/workbench-core/pkg/llm"
	"github.com/tinoosan/workbench-core/pkg/role"
	"github.com/tinoosan/workbench-core/pkg/runtime"
	pkgstore "github.com/tinoosan/workbench-core/pkg/store"
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
	_, run, err := store.CreateSession(cfg, goal, maxContextB)
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}

	emitter := &events.Emitter{
		RunID: run.RunId,
		Sink: events.MultiSink{
			events.StoreSink{Store: daemonEventAppender{cfg: cfg}},
		},
	}
	mustEmit := func(ctx context.Context, ev events.Event) {
		_ = emitter.Emit(ctx, ev)
	}

	artifactIndex := newArtifactIndex()
	workdirAbs, err := resolveWorkDir(resolved.WorkDir)
	if err != nil {
		return err
	}

	var resultsStore pkgstore.ResultsStore
	var memStore pkgstore.MemoryStore
	var profileStore pkgstore.ProfileStore
	var traceStore pkgstore.TraceStore
	var constructorStore pkgstore.ConstructorStateStore

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
		ResultsStore:          resultsStore,
		MemoryStore:           memStore,
		ProfileStore:          profileStore,
		TraceStore:            traceStore,
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
		Notifier:          notifier,
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

	webhookAddr := strings.TrimSpace(resolved.WebhookAddr)
	if webhookAddr != "" {
		startWebhookServer(runCtx, webhookAddr, cfg, run, mustEmit)
	}
	healthAddr := strings.TrimSpace(resolved.HealthAddr)
	if healthAddr != "" && healthAddr != webhookAddr {
		startHealthServer(runCtx, healthAddr, mustEmit)
	}

	mustEmit(context.Background(), events.Event{
		Type:    "daemon.start",
		Message: "Autonomous agent started",
		Data:    map[string]string{"runId": run.RunId, "sessionId": run.SessionID, "role": resolved.Role},
	})
	for {
		err = runner.Run(runCtx)
		if runCtx.Err() != nil {
			// context cancellation is expected on shutdown
			err = nil
			break
		}
		errMsg := "unknown error"
		if err != nil {
			errMsg = err.Error()
		}
		mustEmit(context.Background(), events.Event{
			Type:    "daemon.runner.error",
			Message: "Runner exited unexpectedly; restarting",
			Data:    map[string]string{"error": errMsg},
		})
		time.Sleep(2 * time.Second)
	}
	mustEmit(context.Background(), events.Event{
		Type:    "daemon.stop",
		Message: "Autonomous agent stopped",
		Data:    map[string]string{"runId": run.RunId, "sessionId": run.SessionID, "role": resolved.Role},
	})
	return err
}

// daemonEventAppender adapts store.AppendEvent to events.StoreSink (daemon context).
type daemonEventAppender struct {
	cfg config.Config
}

func (s daemonEventAppender) AppendEvent(ctx context.Context, runID, eventType, message string, data map[string]string) error {
	_ = ctx
	return store.AppendEvent(s.cfg, runID, eventType, message, data)
}

func startWebhookServer(ctx context.Context, addr string, cfg config.Config, run types.Run, emit func(context.Context, events.Event)) {
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
			Goal:      goal,
			Priority:  payload.Priority,
			Status:    "pending",
			CreatedAt: &now,
			Inputs:    payload.Inputs,
			Metadata:  payload.Metadata,
		}
		runDir := fsutil.GetRunDir(cfg.DataDir, run.RunId)
		inboxDir := filepath.Join(runDir, "inbox")
		if err := os.MkdirAll(inboxDir, 0755); err != nil {
			http.Error(w, "inbox error: "+err.Error(), http.StatusInternalServerError)
			return
		}
		b, err := json.MarshalIndent(task, "", "  ")
		if err != nil {
			http.Error(w, "encode error: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if err := os.WriteFile(filepath.Join(inboxDir, taskID+".json"), b, 0644); err != nil {
			http.Error(w, "write error: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if emit != nil {
			emit(context.Background(), events.Event{
				Type:    "webhook.task.queued",
				Message: "Webhook task queued",
				Data:    map[string]string{"taskId": taskID},
			})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"taskId": taskID, "status": "queued"})
	})

	srv := &http.Server{Addr: addr, Handler: mux}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			if emit != nil {
				emit(context.Background(), events.Event{
					Type:    "webhook.error",
					Message: "Webhook server error",
					Data:    map[string]string{"error": err.Error()},
				})
			}
		}
	}()
}

func startHealthServer(ctx context.Context, addr string, emit func(context.Context, events.Event)) {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	srv := &http.Server{Addr: addr, Handler: mux}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			if emit != nil {
				emit(context.Background(), events.Event{
					Type:    "health.error",
					Message: "Health server error",
					Data:    map[string]string{"error": err.Error()},
				})
			}
		}
	}()
}
