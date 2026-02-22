package app

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

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

// ResolveRoleAllowSubagents returns whether the current run's role allows spawning sub-agents.
// Used by TUI to show/hide the Subagents tab.
// For standalone (profile without team), returns true. For team profiles, returns the role's AllowSubagents.
func ResolveRoleAllowSubagents(cfg config.Config, profileID, roleName string) bool {
	profileID = strings.TrimSpace(profileID)
	if profileID == "" {
		return false
	}
	prof, _, err := resolveProfileRef(cfg, profileID)
	if err != nil || prof == nil {
		return false
	}
	if prof.Team == nil {
		return true // standalone profile: allow spawn (current behavior)
	}
	roleName = strings.TrimSpace(roleName)
	if roleName == "" {
		return false
	}
	for i := range prof.Team.Roles {
		r := &prof.Team.Roles[i]
		if strings.EqualFold(strings.TrimSpace(r.Name), roleName) {
			return r.AllowSubagents
		}
	}
	return false
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
