package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/tinoosan/agen8/internal/app"
)

var (
	daemonPoll time.Duration
)

var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "Manage the local Agen8 runtime",
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

var daemonStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the local Agen8 runtime",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := effectiveConfig(cmd)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
			return err
		}
		pidFile := daemonPIDFile(cfg.DataDir)
		if err := os.WriteFile(pidFile, []byte(fmt.Sprintf("%d\n", os.Getpid())), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", pidFile, err)
		}
		defer func() {
			_ = os.Remove(pidFile)
		}()
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

var daemonStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show local runtime status",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := effectiveConfig(cmd)
		if err != nil {
			return err
		}
		pidBytes, _ := os.ReadFile(daemonPIDFile(cfg.DataDir))
		pid := strings.TrimSpace(string(pidBytes))
		if err := rpcPing(cmd.Context()); err != nil {
			if pid != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "daemon=down\npid=%s\n", pid)
			} else {
				fmt.Fprintln(cmd.OutOrStdout(), "daemon=down")
			}
			return nil
		}
		fmt.Fprintln(cmd.OutOrStdout(), "daemon=up")
		if pid != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "pid=%s\n", pid)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "rpc_endpoint=%s\n", resolvedRPCEndpoint())
		return nil
	},
}

var daemonStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the local runtime process recorded in the data directory",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := effectiveConfig(cmd)
		if err != nil {
			return err
		}
		pidFile := daemonPIDFile(cfg.DataDir)
		pidBytes, err := os.ReadFile(pidFile)
		if err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("daemon pid file not found; stop the runtime process manually if it was started outside `agen8 daemon start`")
			}
			return err
		}
		pid := strings.TrimSpace(string(pidBytes))
		if pid == "" {
			return fmt.Errorf("daemon pid file is empty")
		}
		pidInt, err := strconv.Atoi(pid)
		if err != nil {
			return fmt.Errorf("invalid daemon pid %q", pid)
		}
		proc, err := os.FindProcess(pidInt)
		if err != nil {
			return err
		}
		if err := proc.Signal(os.Interrupt); err != nil {
			return err
		}
		_ = os.Remove(pidFile)
		fmt.Fprintf(cmd.OutOrStdout(), "Stopped daemon process %s\n", pid)
		return nil
	},
}

func daemonPIDFile(dataDir string) string {
	return filepath.Join(strings.TrimSpace(dataDir), "daemon.pid")
}

func init() {
	daemonStartCmd.Flags().DurationVar(&daemonPoll, "poll-interval", 2*time.Second, "deprecated: legacy polling interval (runtime now wake-driven)")
	daemonCmd.AddCommand(daemonStartCmd)
	daemonCmd.AddCommand(daemonStatusCmd)
	daemonCmd.AddCommand(daemonStopCmd)
}
