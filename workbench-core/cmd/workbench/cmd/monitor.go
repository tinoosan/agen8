package cmd

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tinoosan/workbench-core/internal/store"
	"github.com/tinoosan/workbench-core/internal/tui"
)

var monitorAgentID string

var monitorCmd = &cobra.Command{
	Use:   "monitor",
	Short: "Open monitoring dashboard for the running agent",
	Long:  "Start the daemon first (workbench or workbench daemon), then run workbench monitor so it attaches to the active agent. Use --agent-id <id> with the agent ID printed at daemon startup if needed.",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := effectiveConfig(cmd)
		if err != nil {
			return err
		}
		agentID := strings.TrimSpace(monitorAgentID)
		if agentID == "" {
			if r, err := store.LatestRunningRun(cfg); err == nil {
				agentID = r.RunID
			} else if r, err := store.LatestRun(cfg); err == nil {
				agentID = r.RunID
			} else {
				return fmt.Errorf("no runs found to monitor")
			}
		}
		err = tui.RunMonitor(cmd.Context(), cfg, agentID)
		if err == nil {
			return nil
		}
		var e *tui.MonitorSwitchRunError
		if errors.As(err, &e) {
			return tui.RunMonitor(cmd.Context(), cfg, e.RunID)
		}
		return err
	},
}

func init() {
	monitorCmd.Flags().StringVar(&monitorAgentID, "agent-id", "", "agent ID to attach to (default: latest running agent)")
}
