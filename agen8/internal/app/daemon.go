package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/tinoosan/agen8/pkg/agent/state"
	"github.com/tinoosan/agen8/pkg/config"
	"github.com/tinoosan/agen8/pkg/cost"
	"github.com/tinoosan/agen8/pkg/events"
	"github.com/tinoosan/agen8/pkg/fsutil"
	"github.com/tinoosan/agen8/pkg/llm"
	llmtypes "github.com/tinoosan/agen8/pkg/llm/types"
	"github.com/tinoosan/agen8/pkg/profile"
	"github.com/tinoosan/agen8/pkg/types"
	"golang.org/x/term"
)

// RunDaemon starts a headless worker that continuously polls DB-backed tasks and emits DB-backed results/events.
// It is intended as the default autonomous entrypoint; the TUI can be used separately as a viewer.
func RunDaemon(ctx context.Context, cfg config.Config, goal string, maxContextB int, poll time.Duration, opts ...RunChatOption) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	stdinTTY := term.IsTerminal(int(os.Stdin.Fd()))
	stdoutTTY := term.IsTerminal(int(os.Stdout.Fd()))
	if _, err := ensureRuntimeConfigTemplate(cfg.DataDir); err != nil {
		return err
	}
	runtimeCfg, err := loadRuntimeConfig(cfg.DataDir)
	if err != nil {
		return err
	}
	cfg = applyRuntimeConfigHostDefaults(cfg, runtimeCfg)
	if err := cfg.Validate(); err != nil {
		return err
	}
	applyRuntimeConfigEnvDefaults(runtimeCfg)
	if err := ensureRuntimeCredentials(cfg.DataDir, stdinTTY && stdoutTTY, os.Stdin, os.Stdout); err != nil {
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
	protocolEnabled := shouldEnableProtocolStdio(
		resolved.ProtocolStdio,
		stdinTTY,
		stdoutTTY,
	)
	prof, profDir, err := resolveProfileRef(cfg, strings.TrimSpace(resolved.Profile))
	if err != nil {
		return err
	}
	if prof.Team != nil {
		return runAsTeam(ctx, cfg, prof, profDir, goal, maxContextB, poll, resolved, protocolEnabled)
	}
	builder := newDaemonBuilder(ctx, cfg, goal, maxContextB, poll, resolved, prof, profDir, protocolEnabled)
	return builder.Run()
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

func resolvePricing(modelID string, overrideIn, overrideOut float64) (inPerM float64, outPerM float64, known bool) {
	modelID = strings.TrimSpace(modelID)
	if modelID != "" {
		if in, out, ok := cost.DefaultPricing().Lookup(modelID); ok {
			return in, out, true
		}
	}
	if overrideIn != 0 || overrideOut != 0 {
		return overrideIn, overrideOut, true
	}
	return 0, 0, false
}

func withRetryDiagnostics(client llmtypes.LLMClient, emit func(context.Context, events.Event)) llmtypes.LLMClient {
	if client == nil || emit == nil {
		return client
	}
	retryClient, ok := client.(*llm.RetryClient)
	if !ok || retryClient == nil {
		return client
	}
	cfg := retryClient.Config
	cfg.OnRetry = func(ctx context.Context, info llm.RetryAttemptInfo) {
		payload := map[string]string{
			"class":      strings.TrimSpace(info.Class),
			"attempt":    fmt.Sprintf("%d", info.Attempt),
			"delayMs":    fmt.Sprintf("%d", info.Delay.Milliseconds()),
			"statusCode": fmt.Sprintf("%d", info.StatusCode),
		}
		if code := strings.TrimSpace(info.Code); code != "" {
			payload["code"] = code
		}
		if msg := strings.TrimSpace(info.Message); msg != "" {
			payload["message"] = msg
		}
		emit(context.Background(), events.Event{
			Type:    "llm.retry",
			Message: "Retrying LLM request",
			Data:    payload,
		})
	}
	return llm.NewRetryClient(retryClient.Wrapped, cfg)
}

func newCostUsageHook(cfg config.Config, run types.Run, modelID string, priceIn, priceOut float64, sessionStore SessionLoadSaver, currentModel func() string, emit func(context.Context, events.Event)) func(step int, usage llmtypes.LLMUsage) {
	tracker := newDefaultCostTracker(cfg, run, modelID, priceIn, priceOut, sessionStore, currentModel, emit)
	if tracker == nil {
		return func(int, llmtypes.LLMUsage) {}
	}
	return tracker.Track
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
		origTaskID := strings.TrimSpace(payload.TaskID)
		taskID := origTaskID
		if taskID == "" {
			taskID = "task-" + uuid.NewString()
		} else if norm, changed := types.NormalizeTaskID(taskID); changed {
			taskID = norm
			if payload.Metadata == nil {
				payload.Metadata = map[string]any{}
			}
			payload.Metadata["originalTaskId"] = origTaskID
		} else {
			taskID = norm
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
			runDir := fsutil.GetRunDir(cfg.DataDir, run)
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
