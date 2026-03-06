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
	explicitSession := strings.TrimSpace(activityTUISessionID) != ""
	projectRoot := projectSearchDir()
	sessionID := strings.TrimSpace(activityTUISessionID)
	if sessionID == "" {
		resolvedRoot, _, resolvedSessionID, _, err := resolveActiveProjectScope(cmd.Context())
		if err == nil {
			projectRoot = resolvedRoot
			sessionID = resolvedSessionID
		}
	}

	// Interactive mode: project-first when we have a project root and no
	// explicit --session-id; session-first otherwise.
	if !explicitSession && strings.TrimSpace(projectRoot) != "" {
		return activitytui.Run(resolvedRPCEndpoint(), activitytui.Options{
			ProjectRoot:        projectRoot,
			FollowProjectState: true,
			SessionID:          sessionID,
			SessionExplicit:    false,
		})
	}

	if sessionID == "" {
		return fmt.Errorf("active team is required (start a team with `agen8 team start <profile-ref>`)")
	}
	return activitytui.Run(resolvedRPCEndpoint(), activitytui.Options{
		ProjectRoot:        projectRoot,
		FollowProjectState: !explicitSession,
		SessionID:          sessionID,
		SessionExplicit:    explicitSession,
	})
}

func init() {
	activityCmd.Flags().StringVar(&activityTUISessionID, "session-id", "", "session id to monitor (defaults to active project session)")
}
