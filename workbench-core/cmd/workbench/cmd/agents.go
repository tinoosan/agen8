package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tinoosan/workbench-core/internal/store"
	"github.com/tinoosan/workbench-core/pkg/types"
)

var agentsCmd = &cobra.Command{
	Use:   "agents",
	Short: "Manage swarm agents",
}

var agentsListCmd = &cobra.Command{
	Use:   "list <sessionId>",
	Short: "List agent runs for a session",
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
				fmt.Fprintf(out, "%s\t%s\n", runID, "error:"+err.Error())
				continue
			}
			if strings.TrimSpace(run.ParentRunID) == "" {
				continue
			}
			cost := ""
			if run.TotalCostUSD > 0 {
				cost = fmt.Sprintf("$%.4f", run.TotalCostUSD)
			}
			tokens := ""
			if run.TotalTokens != 0 {
				tokens = fmt.Sprintf("%d tok", run.TotalTokens)
			}
			if cost == "" && tokens == "" {
				fmt.Fprintf(out, "%s\t%s\tparent=%s\n", run.RunId, run.Status, run.ParentRunID)
				continue
			}
			fmt.Fprintf(out, "%s\t%s\tparent=%s\t%s\t%s\n", run.RunId, run.Status, run.ParentRunID, strings.TrimSpace(tokens), strings.TrimSpace(cost))
		}
		return nil
	},
}

var agentsShowCmd = &cobra.Command{
	Use:   "show <runId>",
	Short: "Show agent run.json for a run",
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

func init() {
	agentsCmd.AddCommand(agentsListCmd)
	agentsCmd.AddCommand(agentsShowCmd)
}
