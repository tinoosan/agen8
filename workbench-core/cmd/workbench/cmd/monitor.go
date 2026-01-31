package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tinoosan/workbench-core/internal/store"
	"github.com/tinoosan/workbench-core/internal/tui"
)

var monitorRunID string

var monitorCmd = &cobra.Command{
	Use:   "monitor",
	Short: "Open monitoring dashboard for the running agent",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := effectiveConfig(cmd)
		if err != nil {
			return err
		}
		runID := strings.TrimSpace(monitorRunID)
		if runID == "" {
			if r, err := store.LatestRunningRun(cfg); err == nil {
				runID = r.RunId
			} else if r, err := store.LatestRun(cfg); err == nil {
				runID = r.RunId
			} else {
				return fmt.Errorf("no runs found to monitor")
			}
		}
		return tui.RunMonitor(cmd.Context(), cfg, runID)
	},
}

func init() {
	monitorCmd.Flags().StringVar(&monitorRunID, "run-id", "", "run ID to attach to (default: latest running run)")
}
