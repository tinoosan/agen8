package cmd

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"github.com/tinoosan/workbench-core/internal/store"
	agentstate "github.com/tinoosan/workbench-core/pkg/agent/state"
	"github.com/tinoosan/workbench-core/pkg/fsutil"
	"github.com/tinoosan/workbench-core/pkg/types"
)

var (
	tasksRunID   string
	tasksSession string

	tasksStatus string
	tasksLimit  int
	tasksOffset int
	tasksSortBy string
	tasksDesc   bool
)

var tasksCmd = &cobra.Command{
	Use:   "tasks",
	Short: "Query and manage tasks",
}

var tasksListCmd = &cobra.Command{
	Use:   "list",
	Short: "List tasks",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := effectiveConfig(cmd)
		if err != nil {
			return err
		}

		runID := strings.TrimSpace(tasksRunID)
		if runID == "" {
			if r, err := store.LatestRunningRun(cfg); err == nil {
				runID = r.RunID
			} else if r, err := store.LatestRun(cfg); err == nil {
				runID = r.RunID
			} else {
				return fmt.Errorf("no runs found")
			}
		}

		sessionID := strings.TrimSpace(tasksSession)
		if sessionID == "" {
			if r, err := store.LoadRun(cfg, runID); err == nil {
				sessionID = r.SessionID
			}
		}

		ts, err := agentstate.NewSQLiteTaskStore(fsutil.GetSQLitePath(cfg.DataDir))
		if err != nil {
			return err
		}

		filter := agentstate.TaskFilter{
			RunID:    runID,
			Limit:    tasksLimit,
			Offset:   tasksOffset,
			SortBy:   strings.TrimSpace(tasksSortBy),
			SortDesc: tasksDesc,
		}
		if sessionID != "" {
			filter.SessionID = sessionID
		}
		if st := strings.TrimSpace(tasksStatus); st != "" {
			parts := strings.Split(st, ",")
			for _, p := range parts {
				p = strings.TrimSpace(p)
				if p == "" {
					continue
				}
				filter.Status = append(filter.Status, types.TaskStatus(p))
			}
		}

		tasks, err := ts.ListTasks(cmd.Context(), filter)
		if err != nil {
			return err
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tSTATUS\tCOST\tTOKENS\tGOAL")
		for _, t := range tasks {
			goal := strings.TrimSpace(t.Goal)
			if len(goal) > 80 {
				goal = goal[:79] + "…"
			}
			cost := fmt.Sprintf("$%.4f", t.CostUSD)
			tokens := fmt.Sprintf("%d", t.TotalTokens)
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", t.TaskID, t.Status, cost, tokens, goal)
		}
		_ = w.Flush()
		return nil
	},
}

var tasksStatsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show aggregated run statistics",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := effectiveConfig(cmd)
		if err != nil {
			return err
		}

		runID := strings.TrimSpace(tasksRunID)
		if runID == "" {
			if r, err := store.LatestRunningRun(cfg); err == nil {
				runID = r.RunID
			} else if r, err := store.LatestRun(cfg); err == nil {
				runID = r.RunID
			} else {
				return fmt.Errorf("no runs found")
			}
		}

		ts, err := agentstate.NewSQLiteTaskStore(fsutil.GetSQLitePath(cfg.DataDir))
		if err != nil {
			return err
		}

		stats, err := ts.GetRunStats(cmd.Context(), runID)
		if err != nil {
			return err
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
		fmt.Fprintf(w, "Run:\t%s\n", runID)
		fmt.Fprintf(w, "Total tasks:\t%d\n", stats.TotalTasks)
		fmt.Fprintf(w, "Succeeded:\t%d\n", stats.Succeeded)
		fmt.Fprintf(w, "Failed:\t%d\n", stats.Failed)
		fmt.Fprintf(w, "Total tokens:\t%d\n", stats.TotalTokens)
		fmt.Fprintf(w, "Total cost:\t$%.4f\n", stats.TotalCost)
		fmt.Fprintf(w, "Total duration:\t%s\n", stats.TotalDuration.Round(time.Second).String())
		_ = w.Flush()
		return nil
	},
}

func init() {
	tasksListCmd.Flags().StringVar(&tasksRunID, "run-id", "", "filter by run id (default: latest running run)")
	tasksListCmd.Flags().StringVar(&tasksSession, "session-id", "", "filter by session id (default: inferred from run)")
	tasksListCmd.Flags().StringVar(&tasksStatus, "status", "", "comma-separated statuses (pending,active,succeeded,failed,canceled)")
	tasksListCmd.Flags().IntVar(&tasksLimit, "limit", 50, "max tasks to show")
	tasksListCmd.Flags().IntVar(&tasksOffset, "offset", 0, "skip N tasks")
	tasksListCmd.Flags().StringVar(&tasksSortBy, "sort-by", "created_at", "sort field (created_at,finished_at,completed_at,cost_usd,priority,updated_at)")
	tasksListCmd.Flags().BoolVar(&tasksDesc, "desc", false, "sort descending")

	tasksCmd.AddCommand(tasksListCmd)
	tasksStatsCmd.Flags().StringVar(&tasksRunID, "run-id", "", "filter by run id (default: latest running run)")
	tasksCmd.AddCommand(tasksStatsCmd)
	rootCmd.AddCommand(tasksCmd)
}
