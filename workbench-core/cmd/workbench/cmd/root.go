package cmd

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tinoosan/workbench-core/pkg/protocol"
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
	rpcEndpoint    string

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
	Short: "Workbench monitor client",
	Long: strings.TrimSpace(`
Workbench is a local, agentic runtime built around a virtual filesystem (VFS).

Running "workbench" opens the monitor client in detached control mode.
Start the daemon with "workbench daemon" and use /new, /sessions, or /agents
from the monitor to attach context.

Protocol mode:
  - Daemon uses --protocol-stdio by default.
  - Use --protocol-stdio=false to disable JSON-RPC over stdin/stdout.
  - Protocol mode is also auto-enabled when both stdin and stdout are non-TTY (piped).
  - Monitor/daemon control-plane uses --rpc-endpoint (default 127.0.0.1:7777).

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
		if strings.TrimSpace(rpcEndpoint) != "" {
			_ = os.Setenv("WORKBENCH_RPC_ENDPOINT", strings.TrimSpace(rpcEndpoint))
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
		return runDetachedMonitorFn(cmd.Context(), cfg)
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
	protocolStdio = envBool("WORKBENCH_PROTOCOL_STDIO", true)
	rootCmd.PersistentFlags().BoolVar(&protocolStdio, "protocol-stdio", protocolStdio, "enable JSON-RPC protocol over stdin/stdout (auto-enabled when piping)")
	rpcEndpoint = strings.TrimSpace(os.Getenv("WORKBENCH_RPC_ENDPOINT"))
	if rpcEndpoint == "" {
		rpcEndpoint = protocol.DefaultRPCEndpoint
	}
	rootCmd.PersistentFlags().StringVar(&rpcEndpoint, "rpc-endpoint", rpcEndpoint, "JSON-RPC daemon endpoint for monitor/daemon control-plane")
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
