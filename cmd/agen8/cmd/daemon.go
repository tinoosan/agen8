package cmd

import (
	"time"

	"github.com/spf13/cobra"
	"github.com/tinoosan/agen8/internal/app"
)

var (
	daemonPoll time.Duration
)

var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "Run the agent runtime headlessly",
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
			app.WithProtocolStdio(protocolStdio),
			app.WithRPCListen(rpcEndpoint),
			app.WithWebhookAddr(webhookAddr),
			app.WithResultWebhookURL(resultWebhookURL),
			app.WithHealthAddr(healthAddr),
			app.WithTraceBytes(maxTraceBytes),
			app.WithMemoryBytes(maxMemoryBytes),
			app.WithRecentHistoryPairs(recentHistoryPairs),
			app.WithIncludeHistoryOps(includeHistoryOps),
		}
		if cmd.Root().PersistentFlags().Changed("auth-provider") {
			opts = append(opts, app.WithAuthProvider(authProvider))
		}
		return app.RunDaemon(cmd.Context(), cfg, "", maxContextB, daemonPoll, opts...)
	},
}

func init() {
	daemonCmd.Flags().DurationVar(&daemonPoll, "poll-interval", 2*time.Second, "deprecated: legacy polling interval (runtime now wake-driven)")
}
