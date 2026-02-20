package cmd

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/tinoosan/agen8/pkg/protocol"
)

var dashboardCmd = &cobra.Command{
	Use:   "dashboard",
	Short: "Show high-level observability across sessions",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runDashboardFlow(cmd)
	},
}

func runDashboardFlow(cmd *cobra.Command) error {
	if err := rpcPing(cmd.Context()); err != nil {
		return err
	}
	sessions, err := rpcListSessions(cmd.Context(), 200)
	if err != nil {
		return err
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(w, "SESSION\tMODE\tTEAM\tRUN\tRUNNING\tPENDING\tACTIVE\tDONE\tTOKENS\tCOST")
	for _, s := range sessions {
		sessionID := strings.TrimSpace(s.SessionID)
		mode := fallback(strings.TrimSpace(s.Mode), "standalone")
		teamID := strings.TrimSpace(s.TeamID)
		runID := strings.TrimSpace(s.CurrentRunID)
		if runID == "" {
			runID = "-"
		}

		var totals protocol.SessionGetTotalsResult
		_ = rpcCall(cmd.Context(), protocol.MethodSessionGetTotals, protocol.SessionGetTotalsParams{
			ThreadID: protocol.ThreadID(sessionID),
			TeamID:   teamID,
			RunID:    strings.TrimSpace(s.CurrentRunID),
		}, &totals)

		pending := 0
		active := 0
		done := totals.TasksDone
		if teamID != "" {
			var teamStatus protocol.TeamGetStatusResult
			_ = rpcCall(cmd.Context(), protocol.MethodTeamGetStatus, protocol.TeamGetStatusParams{
				ThreadID: protocol.ThreadID(sessionID),
				TeamID:   teamID,
			}, &teamStatus)
			pending = teamStatus.Pending
			active = teamStatus.Active
			done = teamStatus.Done
			if totals.TotalTokens == 0 && teamStatus.TotalTokens > 0 {
				totals.TotalTokens = teamStatus.TotalTokens
			}
			if totals.TotalCostUSD == 0 && teamStatus.TotalCostUSD > 0 {
				totals.TotalCostUSD = teamStatus.TotalCostUSD
			}
		}

		cost := "$0.0000"
		if totals.TotalCostUSD > 0 {
			cost = fmt.Sprintf("$%.4f", totals.TotalCostUSD)
		}
		fmt.Fprintf(
			w,
			"%s\t%s\t%s\t%s\t%d\t%d\t%d\t%d\t%d\t%s\n",
			sessionID,
			mode,
			blankDash(teamID),
			runID,
			s.RunningAgents,
			pending,
			active,
			done,
			totals.TotalTokens,
			cost,
		)
	}
	return w.Flush()
}

func init() {
	rootCmd.AddCommand(dashboardCmd)
}
