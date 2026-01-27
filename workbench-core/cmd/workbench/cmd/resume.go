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
		cfg, err := effectiveConfig(cmd)
		if err != nil {
			return err
		}
		sessionID := strings.TrimSpace(args[0])
		if sessionID == "" {
			return fmt.Errorf("sessionId is required")
		}
		sess, err := store.LoadSession(cfg, sessionID)
		if err != nil {
			return err
		}
		parent := strings.TrimSpace(sess.CurrentRunID)
		run, err := store.CreateSubRun(cfg, sessionID, parent, "resume session", maxContextB)
		if err != nil {
			return err
		}

		approvalsOverride := approvalsMode
		if !cmd.Root().PersistentFlags().Changed("approvals-mode") {
			if v := strings.TrimSpace(sess.ApprovalsMode); v != "" {
				approvalsOverride = v
			}
		}

		modelOverride := ""
		if cmd.Root().PersistentFlags().Changed("model") {
			modelOverride = strings.TrimSpace(modelID)
		}
		opts := []app.RunChatOption{
			app.WithApprovalsMode(approvalsOverride),
			app.WithModel(modelOverride),
			app.WithWorkDir(workDir),
			app.WithTraceBytes(maxTraceBytes),
			app.WithMemoryBytes(maxMemoryBytes),
			app.WithProfileBytes(maxProfileBytes),
			app.WithRecentHistoryPairs(recentHistoryPairs),
			app.WithIncludeHistoryOps(includeHistoryOps),
		}
		return app.RunChatTUI(cmd.Context(), cfg, run, opts...)
	},
}
