package cmd

import (
	"strings"

	"github.com/spf13/cobra"
	"github.com/tinoosan/workbench-core/internal/app"
)

// chatCmd runs the legacy interactive TUI.
//
// Note: this is kept temporarily for visibility during the transition to a monitoring-first TUI.
// Approvals are forced disabled to preserve autonomous behavior.
var chatCmd = &cobra.Command{
	Use:   "chat",
	Short: "Open interactive TUI (legacy)",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := effectiveConfig(cmd)
		if err != nil {
			return err
		}
		title := ""
		goal := "interactive chat"

		modelOverride := ""
		if cmd.Root().PersistentFlags().Changed("model") {
			modelOverride = strings.TrimSpace(modelID)
		}

		opts := []app.RunChatOption{
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
			Mode:                    app.ChatStartNew,
			Title:                   title,
			Goal:                    goal,
			RespectSessionApprovals: false,
		}
		return app.RunChatTUILoop(cmd.Context(), cfg, start, maxContextB, opts...)
	},
}
