package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tinoosan/workbench-core/internal/store"
)

var listRunsCmd = &cobra.Command{
	Use:   "runs <sessionId>",
	Short: "List runs for a session",
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

		out := cmd.OutOrStdout()
		for _, runID := range sess.Runs {
			run, err := store.LoadRun(cfg, runID)
			if err != nil {
				// Keep listing even if one run is missing/corrupt; session index is best-effort.
				fmt.Fprintf(out, "%s\t%s\n", runID, "error:"+err.Error())
				continue
			}
			fmt.Fprintf(out, "%s\t%s\n", run.RunId, run.Status)
		}
		return nil
	},
}
