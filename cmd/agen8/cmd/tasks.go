package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"github.com/tinoosan/agen8/internal/app"
	"github.com/tinoosan/agen8/internal/tui/rpcscope"
	agentstate "github.com/tinoosan/agen8/pkg/agent/state"
	"github.com/tinoosan/agen8/pkg/fsutil"
	"github.com/tinoosan/agen8/pkg/protocol"
	"github.com/tinoosan/agen8/pkg/types"
)

var (
	tasksRunID   string
	tasksSession string

	tasksStatus string
	tasksLimit  int
	tasksOffset int
	tasksSortBy string
	tasksDesc   bool

	mailWatchSessionID string
	mailWatchView      string
	mailWatchInterval  time.Duration
)

var tasksCmd = &cobra.Command{
	Use:    "mail",
	Short:  "Query and manage task inbox/outbox",
	Long:   "Mail is the task inbox/outbox surface.",
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runMailTUI(cmd)
	},
}

var tasksListCmd = &cobra.Command{
	Use:   "list",
	Short: "List tasks",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := effectiveConfig(cmd)
		if err != nil {
			return err
		}

		sessionSvc, err := app.NewSessionServiceForCLI(cfg)
		if err != nil {
			return err
		}
		ctx := cmd.Context()

		runID := strings.TrimSpace(tasksRunID)
		if runID == "" {
			if r, err := sessionSvc.LatestRunningRun(ctx); err == nil {
				runID = r.RunID
			} else if r, err := sessionSvc.LatestRun(ctx); err == nil {
				runID = r.RunID
			} else {
				return fmt.Errorf("no runs found")
			}
		}

		sessionID := strings.TrimSpace(tasksSession)
		if sessionID == "" {
			if r, err := sessionSvc.LoadRun(ctx, runID); err == nil {
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
		fmt.Fprintln(w, "ID\tSTATUS\tCOST\tTOKENS")
		for _, t := range tasks {
			cost := fmt.Sprintf("$%.4f", t.CostUSD)
			tokens := fmt.Sprintf("%d", t.TotalTokens)
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", t.TaskID, t.Status, cost, tokens)
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

		sessionSvc, err := app.NewSessionServiceForCLI(cfg)
		if err != nil {
			return err
		}
		ctx := cmd.Context()

		runID := strings.TrimSpace(tasksRunID)
		if runID == "" {
			if r, err := sessionSvc.LatestRunningRun(ctx); err == nil {
				runID = r.RunID
			} else if r, err := sessionSvc.LatestRun(ctx); err == nil {
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

var mailWatchCmd = &cobra.Command{
	Use:   "watch",
	Short: "Live inbox/outbox stream with reconnect",
	RunE: func(cmd *cobra.Command, args []string) error {
		sessionID := strings.TrimSpace(mailWatchSessionID)
		if sessionID == "" {
			projectCtx, err := loadProjectContext()
			if err == nil && projectCtx.Exists {
				sessionID = strings.TrimSpace(projectCtx.State.ActiveSessionID)
			}
		}
		if sessionID == "" {
			return fmt.Errorf("session id is required (use --session-id or initialize project and attach a session)")
		}
		view := strings.ToLower(strings.TrimSpace(mailWatchView))
		if view == "" {
			view = "inbox"
		}
		if view != "inbox" && view != "outbox" {
			return fmt.Errorf("--view must be inbox or outbox")
		}
		interval := mailWatchInterval
		if interval <= 0 {
			interval = 2 * time.Second
		}
		retries := 0
		for {
			if err := renderMailWatchOnce(cmd, sessionID, view); err != nil {
				if !isRetryableLiveError(err) {
					return err
				}
				retries++
				backoff := time.Duration(minInt(8, retries)) * 300 * time.Millisecond
				fmt.Fprintf(cmd.ErrOrStderr(), "mail: reconnecting (%v)\n", err)
				time.Sleep(backoff)
				continue
			}
			retries = 0
			if !isInteractiveTerminal() {
				return nil
			}
			time.Sleep(interval)
		}
	},
}

func renderMailWatchOnce(cmd *cobra.Command, sessionID string, view string) error {
	client := rpcscope.NewClient(resolvedRPCEndpoint(), sessionID).WithTimeout(5 * time.Second)
	ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Second)
	defer cancel()
	scope, err := client.RefreshScope(ctx)
	if err != nil {
		return err
	}
	var out protocol.TaskListResult
	if err := client.Call(ctx, protocol.MethodTaskList, protocol.TaskListParams{
		ThreadID: protocol.ThreadID(scope.ThreadID),
		TeamID:   strings.TrimSpace(scope.TeamID),
		RunID:    strings.TrimSpace(scope.RunID),
		View:     view,
		Limit:    200,
		Offset:   0,
	}, &out); err != nil {
		return err
	}
	if isInteractiveTerminal() {
		fmt.Fprint(cmd.OutOrStdout(), "\033[H\033[2J")
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Mail %s (count=%d)\n", view, out.TotalCount)
	w := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tSTATUS\tASSIGNEE\tRUN")
	for _, task := range out.Tasks {
		assignee := strings.TrimSpace(task.AssignedRole)
		if assignee == "" {
			assignee = strings.TrimSpace(task.AssignedTo)
		}
		fmt.Fprintf(
			w,
			"%s\t%s\t%s\t%s\n",
			blankDash(task.ID),
			blankDash(task.Status),
			blankDash(assignee),
			blankDash(string(task.RunID)),
		)
	}
	_ = w.Flush()
	return nil
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
	mailWatchCmd.Flags().StringVar(&mailWatchSessionID, "session-id", "", "session id to stream (default: active project session)")
	mailWatchCmd.Flags().StringVar(&mailWatchView, "view", "inbox", "task view (inbox|outbox)")
	mailWatchCmd.Flags().DurationVar(&mailWatchInterval, "interval", 2*time.Second, "refresh interval")
	tasksCmd.AddCommand(mailWatchCmd)
}
