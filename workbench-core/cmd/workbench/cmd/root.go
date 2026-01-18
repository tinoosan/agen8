package cmd

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tinoosan/workbench-core/internal/app"
	"github.com/tinoosan/workbench-core/internal/config"
)

var (
	dataDir      string
	workDir      string
	maxContextB  int
	defaultGoal  string
	defaultTitle string
	pricingFile  string
	modelID      string
	enableMouse  bool
	enableActivity bool

	maxSteps           int
	maxTraceBytes      int
	maxMemoryBytes     int
	maxProfileBytes    int
	recentHistoryPairs int
	userID             string
	includeHistoryOps  bool
	priceInPerM        float64
	priceOutPerM       float64
)

var rootCmd = &cobra.Command{
	Use:   "workbench",
	Short: "Workbench starts an interactive agent session",
	Long: strings.TrimSpace(`
Workbench is a local, agentic runtime built around a virtual filesystem (VFS).

Running "workbench" starts a new session and a new run, then opens an interactive
TUI. Each message you submit becomes one agent turn that can:
  - discover tools via /tools (fs.list + fs.read manifests)
  - execute tools via tool.run (writing /results/<callId>/response.json)
  - read/write run-scoped artifacts in /workspace
  - write proposed memory updates to /memory/update.md (host decides commits)

Use "workbench resume <sessionId>" to continue a previous session by creating a
new run in that session (workspaces remain run-scoped).
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
		//
		// Here we only respect explicit user-provided overrides via flags/env.
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := effectiveConfig()
		if err != nil {
			return err
		}
		title := defaultTitle
		if title == "" {
			title = "workbench"
		}
		goal := defaultGoal
		if goal == "" {
			goal = "interactive chat"
		}

		modelOverride := ""
		if cmd.Root().PersistentFlags().Changed("model") {
			modelOverride = strings.TrimSpace(modelID)
		}

		opts := []app.RunChatOption{
			app.WithModel(modelOverride),
			app.WithWorkDir(workDir),
			app.WithMaxSteps(maxSteps),
			app.WithTraceBytes(maxTraceBytes),
			app.WithMemoryBytes(maxMemoryBytes),
			app.WithProfileBytes(maxProfileBytes),
			app.WithRecentHistoryPairs(recentHistoryPairs),
			app.WithUserID(userID),
			app.WithIncludeHistoryOps(includeHistoryOps),
			app.WithPricingUSDPerMTokens(priceInPerM, priceOutPerM),
			app.WithPricingFile(pricingFile),
		}
		return app.RunNewChatTUI(cmd.Context(), cfg, title, goal, maxContextB, opts...)
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
	rootCmd.PersistentFlags().StringVar(&dataDir, "data-dir", config.Default().DataDir, "base directory for runs/sessions (default: data)")
	workDir = strings.TrimSpace(os.Getenv("WORKBENCH_WORKDIR"))
	rootCmd.PersistentFlags().StringVar(&workDir, "workdir", workDir, "host working directory to mount at /workdir (default: current directory; env WORKBENCH_WORKDIR)")
	rootCmd.PersistentFlags().IntVar(&maxContextB, "context-bytes", 8*1024, "run.maxBytesForContext (persisted in run.json)")
	rootCmd.PersistentFlags().StringVar(&defaultTitle, "title", "workbench", "title for new sessions (workbench only)")
	rootCmd.PersistentFlags().StringVar(&defaultGoal, "goal", "interactive chat", "initial goal for the run (workbench only)")
	enableMouse = envBool("WORKBENCH_MOUSE", false)
	rootCmd.PersistentFlags().BoolVar(&enableMouse, "mouse", enableMouse, "enable Bubble Tea mouse capture (mouse wheel scrolling; may disable native selection)")
	enableActivity = envBool("WORKBENCH_ACTIVITY", false)
	rootCmd.PersistentFlags().BoolVar(&enableActivity, "activity", enableActivity, "show activity panel by default (env WORKBENCH_ACTIVITY)")
	pricingFile = strings.TrimSpace(os.Getenv("WORKBENCH_PRICING_FILE"))
	rootCmd.PersistentFlags().StringVar(&pricingFile, "pricing-file", pricingFile, "optional path to pricing json (env WORKBENCH_PRICING_FILE)")
	modelID = strings.TrimSpace(os.Getenv("OPENROUTER_MODEL"))
	rootCmd.PersistentFlags().StringVar(&modelID, "model", modelID, "LLM model identifier (default: env OPENROUTER_MODEL)")
	rootCmd.PersistentFlags().IntVar(&maxSteps, "max-steps", 200, "max agent steps per user turn")
	rootCmd.PersistentFlags().IntVar(&maxTraceBytes, "trace-bytes", 8*1024, "context updater trace budget (bytes)")
	rootCmd.PersistentFlags().IntVar(&maxMemoryBytes, "memory-bytes", 8*1024, "context updater memory budget (bytes)")
	rootCmd.PersistentFlags().IntVar(&maxProfileBytes, "profile-bytes", 4*1024, "context updater profile budget (bytes)")
	rootCmd.PersistentFlags().IntVar(&recentHistoryPairs, "history-pairs", 8, "number of recent (user,agent) pairs injected from /history")
	rootCmd.PersistentFlags().StringVar(&userID, "user-id", strings.TrimSpace(os.Getenv("WORKBENCH_USER_ID")), "optional stable user identifier (also supports env WORKBENCH_USER_ID)")

	includeHistoryOps = envBool("WORKBENCH_INCLUDE_HISTORY_OPS", true)
	rootCmd.PersistentFlags().BoolVar(&includeHistoryOps, "include-history-ops", includeHistoryOps, "include environment host ops from /history in prompt context (higher cost)")

	// Pricing (USD per 1M tokens). If not set, the host uses the built-in table.
	priceInPerM = envFloat("WORKBENCH_PRICE_IN_PER_M", 0)
	priceOutPerM = envFloat("WORKBENCH_PRICE_OUT_PER_M", 0)
	rootCmd.PersistentFlags().Float64Var(&priceInPerM, "price-in-per-m", priceInPerM, "USD per 1M input tokens (cost estimate)")
	rootCmd.PersistentFlags().Float64Var(&priceOutPerM, "price-out-per-m", priceOutPerM, "USD per 1M output tokens (cost estimate)")

	rootCmd.AddCommand(resumeCmd)
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(showCmd)
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

func envFloat(key string, def float64) float64 {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return def
	}
	return f
}

// Note: pricing defaults are resolved at runtime in PersistentPreRunE so the model
// picker can safely switch models without requiring code changes.
