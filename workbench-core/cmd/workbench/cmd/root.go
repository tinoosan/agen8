package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tinoosan/workbench-core/internal/app"
	"github.com/tinoosan/workbench-core/internal/config"
	"github.com/tinoosan/workbench-core/internal/store"
)

var (
	dataDir      string
	maxContextB  int
	defaultGoal  string
	defaultTitle string
)

var rootCmd = &cobra.Command{
	Use:   "workbench",
	Short: "Workbench starts an interactive agent session",
	Long: strings.TrimSpace(`
Workbench is a local, agentic runtime built around a virtual filesystem (VFS).

Running "workbench" starts a new session and a new run, then opens an interactive
chat REPL. Each message you submit becomes one agent turn that can:
  - discover tools via /tools (fs.list + fs.read manifests)
  - execute tools via tool.run (writing /results/<callId>/response.json)
  - read/write run-scoped artifacts in /workspace
  - write proposed memory updates to /memory/update.md (host decides commits)

Use "workbench resume <sessionId>" to continue a previous session by creating a
new run in that session (workspaces remain run-scoped).
`),
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if dataDir != "" {
			config.DataDir = dataDir
		}
		if maxContextB <= 0 {
			return fmt.Errorf("--context-bytes must be > 0")
		}
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		title := defaultTitle
		if title == "" {
			title = "workbench"
		}
		goal := defaultGoal
		if goal == "" {
			goal = "interactive chat"
		}

		sess, err := store.CreateSession(title)
		if err != nil {
			return err
		}
		run, err := store.CreateRunInSession(sess.SessionID, "", goal, maxContextB)
		if err != nil {
			return err
		}
		return app.RunChat(cmd.Context(), run)
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
	rootCmd.PersistentFlags().StringVar(&dataDir, "data-dir", config.DataDir, "base directory for runs/sessions (default: data)")
	rootCmd.PersistentFlags().IntVar(&maxContextB, "context-bytes", 8*1024, "run.maxBytesForContext (persisted in run.json)")
	rootCmd.PersistentFlags().StringVar(&defaultTitle, "title", "workbench", "title for new sessions (workbench only)")
	rootCmd.PersistentFlags().StringVar(&defaultGoal, "goal", "interactive chat", "initial goal for the run (workbench only)")

	rootCmd.AddCommand(resumeCmd)
	rootCmd.AddCommand(listCmd)
}
