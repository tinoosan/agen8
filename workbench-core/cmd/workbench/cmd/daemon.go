package cmd

import (
	"time"

	"github.com/spf13/cobra"
	"github.com/tinoosan/workbench-core/internal/app"
)

var (
	daemonGoal string
	daemonPoll time.Duration
)

var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "Run the autonomous agent loop headlessly",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := effectiveConfig(cmd)
		if err != nil {
			return err
		}
		modelOverride := ""
		if cmd.Root().PersistentFlags().Changed("model") {
			modelOverride = modelID
		}
		opts := []app.RunChatOption{
			// Autonomous-first: approvals are disabled.
			app.WithApprovalsMode("disabled"),
			app.WithModel(modelOverride),
			app.WithProfile(profileRef),
			app.WithWorkDir(workDir),
			app.WithWebhookAddr(webhookAddr),
			app.WithResultWebhookURL(resultWebhookURL),
			app.WithHealthAddr(healthAddr),
			app.WithTraceBytes(maxTraceBytes),
			app.WithMemoryBytes(maxMemoryBytes),
			app.WithUserProfileBytes(maxUserProfileBytes),
			app.WithRecentHistoryPairs(recentHistoryPairs),
			app.WithIncludeHistoryOps(includeHistoryOps),
		}
		return app.RunDaemon(cmd.Context(), cfg, daemonGoal, maxContextB, daemonPoll, opts...)
	},
}

func init() {
	daemonCmd.Flags().StringVar(&daemonGoal, "goal", "autonomous agent", "default goal/intent for the daemon run")
	daemonCmd.Flags().DurationVar(&daemonPoll, "poll-interval", 2*time.Second, "inbox polling interval")
}
