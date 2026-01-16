package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tinoosan/workbench-core/internal/app"
	"github.com/tinoosan/workbench-core/internal/store"
)

var resumeCmd = &cobra.Command{
	Use:   "resume <sessionId>",
	Short: "Resume a previous session by starting a new run",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		sessionID := strings.TrimSpace(args[0])
		if sessionID == "" {
			return fmt.Errorf("sessionId is required")
		}
		sess, err := store.LoadSession(sessionID)
		if err != nil {
			return err
		}
		parent := strings.TrimSpace(sess.CurrentRunID)
		run, err := store.CreateRunInSession(sessionID, parent, "resume session", maxContextB)
		if err != nil {
			return err
		}
		return app.RunChat(cmd.Context(), run, app.RunChatOptions{
			MaxSteps:           maxSteps,
			MaxTraceBytes:      maxTraceBytes,
			MaxMemoryBytes:     maxMemoryBytes,
			MaxProfileBytes:    maxProfileBytes,
			RecentHistoryPairs: recentHistoryPairs,
		})
	},
}
