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
	Use:    "dashboard",
	Short:  "Live per-agent dashboard for the active session",
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runDashboardFlow(cmd)
	},
}

func runDashboardFlow(cmd *cobra.Command) error {
	explicitSession := strings.TrimSpace(dashboardSessionID) != ""
	projectRoot := projectSearchDir()
	sessionID := strings.TrimSpace(dashboardSessionID)
	if sessionID == "" {
		resolvedRoot, _, resolvedSessionID, _, err := resolveActiveProjectScope(cmd.Context())
		if err == nil {
			projectRoot = resolvedRoot
			sessionID = resolvedSessionID
		}
	}

	if dashboardOnce || !isInteractiveTerminal() {
		if sessionID == "" {
			return fmt.Errorf("active team is required (start a team with `agen8 team start <profile-ref>`)")
		}
		return renderDashboardOnce(cmd, sessionID)
	}

	// Interactive mode: project-first when we have a project root and no
	// explicit --session-id; session-first otherwise.
	if !explicitSession && strings.TrimSpace(projectRoot) != "" {
		return dashboardtui.Run(resolvedRPCEndpoint(), dashboardtui.Options{
			ProjectRoot:        projectRoot,
			FollowProjectState: true,
			RefreshInterval:    effectiveDashboardInterval(),
			SessionID:          sessionID,
			SessionExplicit:    false,
		})
	}

	if sessionID == "" {
		return fmt.Errorf("active team is required (start a team with `agen8 team start <profile-ref>`)")
	}
	return dashboardtui.Run(resolvedRPCEndpoint(), dashboardtui.Options{
		ProjectRoot:        projectRoot,
		FollowProjectState: !explicitSession,
		RefreshInterval:    effectiveDashboardInterval(),
		SessionID:          sessionID,
		SessionExplicit:    explicitSession,
	})
}

func effectiveDashboardInterval() time.Duration {
	if dashboardInterval <= 0 {
		return 2 * time.Second
	}
	return dashboardInterval
}

func renderDashboardOnce(cmd *cobra.Command, sessionID string) error {
	teamID := ""
	currentRunID := ""
	if _, resolvedTeamID, resolvedSessionID, resolvedRunID, err := resolveActiveProjectScope(cmd.Context()); err == nil && strings.TrimSpace(resolvedSessionID) == strings.TrimSpace(sessionID) {
		teamID = strings.TrimSpace(resolvedTeamID)
		currentRunID = strings.TrimSpace(resolvedRunID)
	} else if resolved, err := rpcResolveThread(cmd.Context(), sessionID, ""); err == nil {
		teamID = strings.TrimSpace(resolved.TeamID)
		currentRunID = strings.TrimSpace(resolved.RunID)
	}
	var agents protocol.AgentListResult
	if err := rpcCall(cmd.Context(), protocol.MethodAgentList, protocol.AgentListParams{
		ThreadID:  protocol.ThreadID(sessionID),
		SessionID: sessionID,
	}, &agents); err != nil {
		return err
	}
	runningAgents := 0
	for _, agent := range agents.Agents {
		if strings.EqualFold(strings.TrimSpace(agent.Status), "running") {
			runningAgents++
		}
	}

	var totals protocol.SessionGetTotalsResult
	_ = rpcCall(cmd.Context(), protocol.MethodSessionGetTotals, protocol.SessionGetTotalsParams{
		ThreadID: protocol.ThreadID(sessionID),
		TeamID:   teamID,
		RunID:    currentRunID,
	}, &totals)

	pending := 0
	active := 0
	done := totals.TasksDone
	if teamID != "" {
		var teamStatus protocol.TeamGetStatusResult
		if err := rpcCall(cmd.Context(), protocol.MethodTeamGetStatus, protocol.TeamGetStatusParams{
			ThreadID: protocol.ThreadID(sessionID),
			TeamID:   teamID,
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
	fmt.Fprintf(cmd.OutOrStdout(), "Team %s\n", blankDash(teamID))

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
		runningAgents,
	)
	return nil
}

func init() {
	dashboardCmd.Flags().StringVar(&dashboardSessionID, "session-id", "", "session id to inspect (default: active project session)")
	dashboardCmd.Flags().BoolVar(&dashboardOnce, "once", false, "render once and exit")
	dashboardCmd.Flags().DurationVar(&dashboardInterval, "interval", 2*time.Second, "refresh interval for live mode")
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
