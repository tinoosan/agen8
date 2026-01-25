package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tinoosan/workbench-core/internal/store"
	"github.com/tinoosan/workbench-core/pkg/types"
)

var showRunCmd = &cobra.Command{
	Use:   "run <runId>",
	Short: "Show run.json for a run",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := effectiveConfig(cmd)
		if err != nil {
			return err
		}
		runID := strings.TrimSpace(args[0])
		if runID == "" {
			return fmt.Errorf("runId is required")
		}
		run, err := store.LoadRun(cfg, runID)
		if err != nil {
			return err
		}
		b, err := types.MarshalPretty(run)
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(b))
		return nil
	},
}
