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
	Use:    "activity",
	Short:  "Full-screen activity TUI (tmux-friendly)",
	Long:   "A standalone, full-screen activity viewer for agent operations. Designed for tmux pane composition alongside the monitor and mail TUIs.",
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runActivityTUI(cmd)
	},
}

func runActivityTUI(cmd *cobra.Command) error {
	followProjectState := strings.TrimSpace(activityTUISessionID) == ""
	projectRoot := projectSearchDir()
	sessionID := strings.TrimSpace(activityTUISessionID)
	if sessionID == "" {
		resolvedRoot, _, resolvedSessionID, _, err := resolveActiveProjectScope(cmd.Context())
		if err == nil {
			projectRoot = resolvedRoot
			sessionID = resolvedSessionID
		}
	}
	if sessionID == "" {
		return fmt.Errorf("active team session is required (start a team with `agen8 team start <profile-ref>` or pass --session-id)")
	}
	return activitytui.Run(resolvedRPCEndpoint(), sessionID, activitytui.Options{
		ProjectRoot:        projectRoot,
		FollowProjectState: followProjectState,
	})
}

func init() {
	activityCmd.Flags().StringVar(&activityTUISessionID, "session-id", "", "session id to monitor (defaults to active project session)")
}
