package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tinoosan/agen8/internal/tui/activitytui"
)

var (
	activityTUISessionID string
)

var activityCmd = &cobra.Command{
	Use:   "activity",
	Short: "Full-screen activity TUI (tmux-friendly)",
	Long:  "A standalone, full-screen activity viewer for agent operations. Designed for tmux pane composition alongside the monitor and mail TUIs.",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runActivityTUI(cmd)
	},
}

func runActivityTUI(cmd *cobra.Command) error {
	sessionID := strings.TrimSpace(activityTUISessionID)
	if sessionID == "" {
		projectCtx, err := loadProjectContext()
		if err == nil && projectCtx.Exists {
			sessionID = strings.TrimSpace(projectCtx.State.ActiveSessionID)
		}
	}
	if sessionID == "" {
		return fmt.Errorf("session id is required (use --session-id or initialize project and attach a session)")
	}
	return activitytui.Run(resolvedRPCEndpoint(), sessionID)
}

func init() {
	activityCmd.Flags().StringVar(&activityTUISessionID, "session-id", "", "session id to monitor (defaults to active project session)")
	rootCmd.AddCommand(activityCmd)
}
