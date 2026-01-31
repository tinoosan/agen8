package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tinoosan/workbench-core/internal/app"
)

var resumeNewRun bool

var resumeCmd = &cobra.Command{
	Use:   "resume <sessionId>",
	Short: "Resume a previous session",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := effectiveConfig(cmd)
		if err != nil {
			return err
		}
		sessionID := strings.TrimSpace(args[0])
		if sessionID == "" {
			return fmt.Errorf("sessionId is required")
		}

		modelOverride := ""
		if cmd.Root().PersistentFlags().Changed("model") {
			modelOverride = strings.TrimSpace(modelID)
		}
		opts := []app.RunChatOption{
			// Autonomous-first: approvals are disabled.
			app.WithApprovalsMode("disabled"),
			app.WithModel(modelOverride),
			app.WithRole(roleName),
			app.WithWorkDir(workDir),
			app.WithTraceBytes(maxTraceBytes),
			app.WithMemoryBytes(maxMemoryBytes),
			app.WithProfileBytes(maxProfileBytes),
			app.WithRecentHistoryPairs(recentHistoryPairs),
			app.WithIncludeHistoryOps(includeHistoryOps),
		}
		start := app.ChatStart{
			Mode:                    app.ChatStartResume,
			SessionID:               sessionID,
			ForceNewRun:             resumeNewRun,
			RespectSessionApprovals: false,
		}
		return app.RunChatTUILoop(cmd.Context(), cfg, start, maxContextB, opts...)
	},
}

func init() {
	resumeCmd.Flags().BoolVar(&resumeNewRun, "new-run", false, "start a new run instead of continuing the last run")
}
