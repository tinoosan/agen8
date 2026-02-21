package cmd

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"github.com/tinoosan/agen8/internal/tui/dashboardtui"
	"github.com/tinoosan/agen8/pkg/protocol"
)

var (
	dashboardSessionID string
	dashboardOnce      bool
	dashboardInterval  time.Duration
)

var dashboardCmd = &cobra.Command{
	Use:   "dashboard",
	Short: "Live per-agent dashboard for the active session",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runDashboardFlow(cmd)
	},
}

func runDashboardFlow(cmd *cobra.Command) error {
	followProjectState := strings.TrimSpace(dashboardSessionID) == ""
	projectRoot := projectSearchDir()
	sessionID := strings.TrimSpace(dashboardSessionID)
	if sessionID == "" {
		projectCtx, err := loadProjectContext()
		if err == nil && projectCtx.Exists {
			projectRoot = strings.TrimSpace(projectCtx.RootDir)
			sessionID = strings.TrimSpace(projectCtx.State.ActiveSessionID)
		}
	}
	if sessionID == "" {
		return fmt.Errorf("session id is required (use --session-id or initialize project and attach a session)")
	}

	if dashboardOnce || !isInteractiveTerminal() {
		return renderDashboardOnce(cmd, sessionID)
	}
	return dashboardtui.Run(resolvedRPCEndpoint(), sessionID, dashboardtui.Options{
		ProjectRoot:        projectRoot,
		FollowProjectState: followProjectState,
	})
}

func renderDashboardOnce(cmd *cobra.Command, sessionID string) error {
	item, err := rpcFindSession(cmd.Context(), sessionID)
	if err != nil {
		return err
	}
	var agents protocol.AgentListResult
	if err := rpcCall(cmd.Context(), protocol.MethodAgentList, protocol.AgentListParams{
		ThreadID:  protocol.ThreadID(sessionID),
		SessionID: sessionID,
	}, &agents); err != nil {
		return err
	}

	var totals protocol.SessionGetTotalsResult
	_ = rpcCall(cmd.Context(), protocol.MethodSessionGetTotals, protocol.SessionGetTotalsParams{
		ThreadID: protocol.ThreadID(sessionID),
		TeamID:   strings.TrimSpace(item.TeamID),
		RunID:    strings.TrimSpace(item.CurrentRunID),
	}, &totals)

	pending := 0
	active := 0
	done := totals.TasksDone
	if strings.TrimSpace(item.TeamID) != "" {
		var teamStatus protocol.TeamGetStatusResult
		if err := rpcCall(cmd.Context(), protocol.MethodTeamGetStatus, protocol.TeamGetStatusParams{
			ThreadID: protocol.ThreadID(sessionID),
			TeamID:   strings.TrimSpace(item.TeamID),
		}, &teamStatus); err == nil {
			pending = teamStatus.Pending
			active = teamStatus.Active
			done = teamStatus.Done
			if totals.TotalTokens == 0 {
				totals.TotalTokens = teamStatus.TotalTokens
			}
			if totals.TotalCostUSD == 0 {
				totals.TotalCostUSD = teamStatus.TotalCostUSD
			}
		}
	}

	if !dashboardOnce && isInteractiveTerminal() {
		fmt.Fprint(cmd.OutOrStdout(), "\033[H\033[2J")
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Session %s (%s)\n", strings.TrimSpace(item.SessionID), fallback(item.Mode, "standalone"))
	fmt.Fprintf(cmd.OutOrStdout(), "Run %s  Team %s\n", blankDash(strings.TrimSpace(item.CurrentRunID)), blankDash(strings.TrimSpace(item.TeamID)))

	effectiveByRun := map[string]protocol.RuntimeRunState{}
	var runtimeState protocol.RuntimeGetSessionStateResult
	if err := rpcCall(cmd.Context(), protocol.MethodRuntimeGetSessionState, protocol.RuntimeGetSessionStateParams{
		SessionID: sessionID,
	}, &runtimeState); err == nil {
		for _, rs := range runtimeState.Runs {
			rid := strings.TrimSpace(rs.RunID)
			if rid == "" {
				continue
			}
			effectiveByRun[rid] = rs
		}
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(w, "ROLE\tRUN\tSTATUS\tPROFILE\tWORKER\tHEARTBEAT\tSTARTED")
	for _, agent := range agents.Agents {
		role := strings.TrimSpace(agent.Role)
		if strings.EqualFold(strings.TrimSpace(item.Mode), "standalone") {
			role = "-"
		}
		effective := strings.TrimSpace(agent.Status)
		worker := "-"
		heartbeat := "-"
		if rs, ok := effectiveByRun[strings.TrimSpace(agent.RunID)]; ok {
			if v := strings.TrimSpace(rs.EffectiveStatus); v != "" {
				effective = v
			}
			if rs.WorkerPresent {
				worker = "yes"
			} else {
				worker = "no"
			}
			if hb := strings.TrimSpace(rs.LastHeartbeatAt); hb != "" {
				heartbeat = hb
			}
		}
		fmt.Fprintf(
			w,
			"%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			blankDash(role),
			blankDash(strings.TrimSpace(agent.RunID)),
			blankDash(effective),
			blankDash(strings.TrimSpace(agent.Profile)),
			worker,
			heartbeat,
			blankDash(strings.TrimSpace(agent.StartedAt)),
		)
	}
	_ = w.Flush()

	fmt.Fprintf(
		cmd.OutOrStdout(),
		"Totals: tokens=%d cost=$%.4f pending=%d active=%d done=%d running=%d\n",
		totals.TotalTokens,
		totals.TotalCostUSD,
		pending,
		active,
		done,
		item.RunningAgents,
	)
	return nil
}

func init() {
	dashboardCmd.Flags().StringVar(&dashboardSessionID, "session-id", "", "session id to inspect (default: active project session)")
	dashboardCmd.Flags().BoolVar(&dashboardOnce, "once", false, "render once and exit")
	dashboardCmd.Flags().DurationVar(&dashboardInterval, "interval", 2*time.Second, "refresh interval for live mode")
	rootCmd.AddCommand(dashboardCmd)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
