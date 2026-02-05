package cmd

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/tinoosan/workbench-core/internal/app"
)

var (
	dataDir        string
	workDir        string
	maxContextB    int
	modelID        string
	profileRef     string
	enableMouse    bool
	enableActivity bool
	protocolStdio  bool

	maxTraceBytes      int
	maxMemoryBytes     int
	recentHistoryPairs int
	includeHistoryOps  bool
	webhookAddr        string
	resultWebhookURL   string
	healthAddr         string
)

var rootCmd = &cobra.Command{
	Use:   "workbench",
	Short: "Workbench runs an always-on autonomous agent",
	Long: strings.TrimSpace(`
Workbench is a local, agentic runtime built around a virtual filesystem (VFS).

Running "workbench" starts a new session and run, then starts an always-on daemon
that continuously processes tasks from /inbox and writes results to /outbox.

Start the daemon first, then run "workbench monitor" so the monitor attaches to
the active agent (or use "workbench monitor --agent-id <id>" with the agent ID printed
at daemon startup).

Protocol mode:
  - Use --protocol-stdio to enable JSON-RPC 2.0 over stdin/stdout.
  - Protocol mode is also auto-enabled when both stdin and stdout are non-TTY (piped).

Each executed task can:
  - read/write run-scoped artifacts in /workspace
  - read/write shared memory in /memory (daily files; only today's file is writable)
`),
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// dataDir is resolved per-command via effectiveConfig().
		if maxContextB <= 0 {
			return fmt.Errorf("--context-bytes must be > 0")
		}

		// Mouse mode is opt-in. By default the TUI does NOT capture the mouse so users
		// can drag-select text in the transcript with their terminal's native selection.
		//
		// When enabled, Bubble Tea captures mouse events (wheel, clicks), which typically
		// disables native selection unless the terminal supports shift-drag selection.
		if enableMouse {
			_ = os.Setenv("WORKBENCH_MOUSE", "true")
		} else {
			_ = os.Unsetenv("WORKBENCH_MOUSE")
		}

		// Activity panel is opt-in (default closed).
		if enableActivity {
			_ = os.Setenv("WORKBENCH_ACTIVITY", "true")
		} else {
			_ = os.Unsetenv("WORKBENCH_ACTIVITY")
		}

		// Pricing is resolved against the effective model at runtime (after session
		// load and/or /model overrides).
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := effectiveConfig(cmd)
		if err != nil {
			return err
		}
		modelOverride := ""
		if cmd.Root().PersistentFlags().Changed("model") {
			modelOverride = strings.TrimSpace(modelID)
		}

		opts := []app.RunChatOption{
			app.WithModel(modelOverride),
			app.WithProfile(profileRef),
			app.WithWorkDir(workDir),
			app.WithProtocolStdio(protocolStdio),
			app.WithWebhookAddr(webhookAddr),
			app.WithResultWebhookURL(resultWebhookURL),
			app.WithHealthAddr(healthAddr),
			app.WithTraceBytes(maxTraceBytes),
			app.WithMemoryBytes(maxMemoryBytes),
			app.WithRecentHistoryPairs(recentHistoryPairs),
			app.WithIncludeHistoryOps(includeHistoryOps),
		}
		// Always-on autonomous daemon is the default entrypoint.
		return app.RunDaemon(cmd.Context(), cfg, "autonomous agent", maxContextB, 2*time.Second, opts...)
	},
}

// Execute runs the workbench CLI.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&dataDir, "data-dir", "", "base directory for agents/sessions (priority: --data-dir, env WORKBENCH_DATA_DIR, default: ~/.workbench or $XDG_STATE_HOME/workbench)")
	workDir = strings.TrimSpace(os.Getenv("WORKBENCH_WORKDIR"))
	rootCmd.PersistentFlags().StringVar(&workDir, "workdir", workDir, "host working directory to mount at /project (default: current directory; env WORKBENCH_WORKDIR)")
	rootCmd.PersistentFlags().IntVar(&maxContextB, "context-bytes", 8*1024, "run.maxBytesForContext (persisted in run.json)")
	enableMouse = envBool("WORKBENCH_MOUSE", false)
	rootCmd.PersistentFlags().BoolVar(&enableMouse, "mouse", enableMouse, "enable Bubble Tea mouse capture (mouse wheel scrolling; may disable native selection)")
	enableActivity = envBool("WORKBENCH_ACTIVITY", false)
	rootCmd.PersistentFlags().BoolVar(&enableActivity, "activity", enableActivity, "show activity panel by default (env WORKBENCH_ACTIVITY)")
	rootCmd.PersistentFlags().BoolVar(&protocolStdio, "protocol-stdio", false, "enable JSON-RPC protocol over stdin/stdout (auto-enabled when piping)")
	modelID = strings.TrimSpace(os.Getenv("OPENROUTER_MODEL"))
	rootCmd.PersistentFlags().StringVar(&modelID, "model", modelID, "LLM model identifier (default: env OPENROUTER_MODEL)")
	profileRef = strings.TrimSpace(os.Getenv("WORKBENCH_PROFILE"))
	rootCmd.PersistentFlags().StringVar(&profileRef, "profile", profileRef, "agent profile id or path (env WORKBENCH_PROFILE)")
	webhookAddr = strings.TrimSpace(os.Getenv("WORKBENCH_WEBHOOK_ADDR"))
	rootCmd.PersistentFlags().StringVar(&webhookAddr, "webhook-addr", webhookAddr, "listen address for task webhook server (env WORKBENCH_WEBHOOK_ADDR)")
	resultWebhookURL = strings.TrimSpace(os.Getenv("WORKBENCH_RESULT_WEBHOOK_URL"))
	rootCmd.PersistentFlags().StringVar(&resultWebhookURL, "result-webhook-url", resultWebhookURL, "POST task results to this webhook URL (env WORKBENCH_RESULT_WEBHOOK_URL)")
	healthAddr = strings.TrimSpace(os.Getenv("WORKBENCH_HEALTH_ADDR"))
	rootCmd.PersistentFlags().StringVar(&healthAddr, "health-addr", healthAddr, "listen address for health checks (env WORKBENCH_HEALTH_ADDR)")
	rootCmd.PersistentFlags().IntVar(&maxTraceBytes, "trace-bytes", 8*1024, "context updater trace budget (bytes)")
	rootCmd.PersistentFlags().IntVar(&maxMemoryBytes, "memory-bytes", 8*1024, "context updater memory budget (bytes)")
	rootCmd.PersistentFlags().IntVar(&recentHistoryPairs, "history-pairs", 8, "number of recent (user,agent) pairs injected from /history")
	includeHistoryOps = envBool("WORKBENCH_INCLUDE_HISTORY_OPS", true)
	rootCmd.PersistentFlags().BoolVar(&includeHistoryOps, "include-history-ops", includeHistoryOps, "include environment host ops from /history in prompt context (higher cost)")

	rootCmd.AddCommand(daemonCmd)
	rootCmd.AddCommand(monitorCmd)
	rootCmd.AddCommand(profilesCmd)
}

func envBool(key string, def bool) bool {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}

// Note: pricing defaults are resolved at runtime in PersistentPreRunE so the model
// picker can safely switch models without requiring code changes.
