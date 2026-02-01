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
	Long:  "Start the daemon first (workbench or workbench daemon), then run workbench monitor so it attaches to the active run. Use --run-id <id> with the run ID printed at daemon startup if needed.",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := effectiveConfig(cmd)
		if err != nil {
			return err
		}
		runID := strings.TrimSpace(monitorRunID)
		if runID == "" {
			if r, err := store.LatestRunningRun(cfg); err == nil {
				runID = r.RunID
			} else if r, err := store.LatestRun(cfg); err == nil {
				runID = r.RunID
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
