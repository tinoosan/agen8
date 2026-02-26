package cmd

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/tinoosan/agen8/pkg/protocol"
)

var (
	statusSessionID string

	feedRunID     string
	feedSessionID string
	feedAgentID   string
	feedHarnessID string
	feedTypes     []string
	feedLimit     int

	traceRunID     string
	traceSessionID string
	traceAgentID   string
	traceHarnessID string
	traceLimit     int

	costsSessionID string
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Alias for dashboard --once",
	RunE: func(cmd *cobra.Command, args []string) error {
		prevSessionID := dashboardSessionID
		prevOnce := dashboardOnce
		dashboardSessionID = strings.TrimSpace(statusSessionID)
		dashboardOnce = true
		defer func() {
			dashboardSessionID = prevSessionID
			dashboardOnce = prevOnce
		}()
		return runDashboardFlow(cmd)
	},
}

var feedCmd = &cobra.Command{
	Use:   "feed",
	Short: "Live run/task/harness event feed",
	RunE: func(cmd *cobra.Command, args []string) error {
		typesFilter := normalizeTypeFilter(feedTypes)
		if len(typesFilter) == 0 {
			typesFilter = []string{
				"task.start",
				"task.done",
				"harness.selected",
				"harness.run.start",
				"harness.run.complete",
				"harness.run.error",
				"harness.usage.reported",
			}
		}
		runIDs, err := resolveTargetRunIDs(cmd.Context(), strings.TrimSpace(feedRunID), strings.TrimSpace(feedSessionID), strings.TrimSpace(feedAgentID))
		if err != nil {
			return err
		}
		return followEventRuns(cmd, runIDs, true, feedLimit, func(cmd *cobra.Command, runID string, afterSeq int64, limit int) (int64, error) {
			return printLogsBatch(cmd, runID, typesFilter, strings.TrimSpace(feedHarnessID), afterSeq, limit)
		})
	},
}

var traceCmd = &cobra.Command{
	Use:   "trace",
	Short: "Live reasoning/tool/harness trace feed",
	RunE: func(cmd *cobra.Command, args []string) error {
		typesFilter := []string{
			"agent.op.request",
			"agent.op.response",
			"model.thinking.summary",
			"harness.selected",
			"harness.run.start",
			"harness.run.complete",
			"harness.run.error",
		}
		runIDs, err := resolveTargetRunIDs(cmd.Context(), strings.TrimSpace(traceRunID), strings.TrimSpace(traceSessionID), strings.TrimSpace(traceAgentID))
		if err != nil {
			return err
		}
		return followEventRuns(cmd, runIDs, true, traceLimit, func(cmd *cobra.Command, runID string, afterSeq int64, limit int) (int64, error) {
			return printLogsBatch(cmd, runID, typesFilter, strings.TrimSpace(traceHarnessID), afterSeq, limit)
		})
	},
}

var costsCmd = &cobra.Command{
	Use:   "costs",
	Short: "Show session and per-run token/cost totals",
	RunE: func(cmd *cobra.Command, args []string) error {
		sessionID := strings.TrimSpace(costsSessionID)
		if sessionID == "" {
			projectCtx, err := loadProjectContext()
			if err == nil && projectCtx.Exists {
				sessionID = strings.TrimSpace(projectCtx.State.ActiveSessionID)
			}
		}
		if sessionID == "" {
			return fmt.Errorf("session id is required (use --session-id or initialize project and attach a session)")
		}

		item, err := rpcFindSession(cmd.Context(), sessionID)
		if err != nil {
			return err
		}

		var totals protocol.SessionGetTotalsResult
		if err := rpcCall(cmd.Context(), protocol.MethodSessionGetTotals, protocol.SessionGetTotalsParams{
			ThreadID: protocol.ThreadID(sessionID),
			TeamID:   strings.TrimSpace(item.TeamID),
			RunID:    strings.TrimSpace(item.CurrentRunID),
		}, &totals); err != nil {
			return fmt.Errorf("session.getTotals: %w", err)
		}

		var runtimeState protocol.RuntimeGetSessionStateResult
		if err := rpcCall(cmd.Context(), protocol.MethodRuntimeGetSessionState, protocol.RuntimeGetSessionStateParams{
			SessionID: sessionID,
		}, &runtimeState); err != nil {
			return fmt.Errorf("runtime.getSessionState: %w", err)
		}

		fmt.Fprintf(cmd.OutOrStdout(), "Session %s totals: tokens=%d cost=$%.4f\n", sessionID, totals.TotalTokens, totals.TotalCostUSD)
		w := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
		fmt.Fprintln(w, "RUN\tHARNESS\tSTATUS\tTOKENS\tCOST")
		for _, rs := range runtimeState.Runs {
			fmt.Fprintf(
				w,
				"%s\t%s\t%s\t%d\t$%.4f\n",
				blankDash(strings.TrimSpace(rs.RunID)),
				blankDash(strings.TrimSpace(rs.HarnessID)),
				blankDash(strings.TrimSpace(rs.EffectiveStatus)),
				rs.RunTotalTokens,
				rs.RunTotalCostUSD,
			)
		}
		_ = w.Flush()
		return nil
	},
}

func init() {
	statusCmd.Flags().StringVar(&statusSessionID, "session-id", "", "session id to inspect (default: active project session)")

	feedCmd.Flags().StringVar(&feedRunID, "run-id", "", "run id to query")
	feedCmd.Flags().StringVar(&feedSessionID, "session-id", "", "session id scope (defaults to active project session)")
	feedCmd.Flags().StringVar(&feedAgentID, "agent", "", "agent/run id alias for filtering")
	feedCmd.Flags().StringVar(&feedHarnessID, "harness-id", "", "filter events by harness id")
	feedCmd.Flags().StringSliceVar(&feedTypes, "type", nil, "event type filter (repeat or comma-separated)")
	feedCmd.Flags().IntVar(&feedLimit, "limit", 200, "max events per poll per run")

	traceCmd.Flags().StringVar(&traceRunID, "run-id", "", "run id to query")
	traceCmd.Flags().StringVar(&traceSessionID, "session-id", "", "session id scope (defaults to active project session)")
	traceCmd.Flags().StringVar(&traceAgentID, "agent", "", "agent/run id alias for filtering")
	traceCmd.Flags().StringVar(&traceHarnessID, "harness-id", "", "filter events by harness id")
	traceCmd.Flags().IntVar(&traceLimit, "limit", 200, "max events per poll per run")

	costsCmd.Flags().StringVar(&costsSessionID, "session-id", "", "session id to inspect (default: active project session)")

	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(feedCmd)
	rootCmd.AddCommand(traceCmd)
	rootCmd.AddCommand(costsCmd)
}
