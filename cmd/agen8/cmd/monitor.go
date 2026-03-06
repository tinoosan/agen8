package cmd

import (
	"errors"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tinoosan/agen8/internal/tui"
)

var monitorAgentID string
var monitorTeamID string
var runMonitorFn = tui.RunMonitor
var runTeamMonitorFn = tui.RunTeamMonitor
var runDetachedMonitorFn = tui.RunMonitorDetached

var monitorCmd = &cobra.Command{
	Use:   "monitor",
	Short: "Open the primary live operator surface",
	Long:  "Start the daemon first with `agen8 daemon start`, start work with `agen8 team start <profile-ref>`, then run `agen8 monitor` to operate the live system.",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := effectiveConfig(cmd)
		if err != nil {
			return err
		}
		teamID := strings.TrimSpace(monitorTeamID)
		agentID := strings.TrimSpace(monitorAgentID)
		for {
			switch {
			case teamID != "":
				err = runTeamMonitorFn(cmd.Context(), cfg, teamID)
			case agentID != "":
				err = runMonitorFn(cmd.Context(), cfg, agentID)
			default:
				err = runDetachedMonitorFn(cmd.Context(), cfg)
			}
			if err == nil {
				return nil
			}
			var switchRun *tui.MonitorSwitchRunError
			if errors.As(err, &switchRun) {
				next := strings.TrimSpace(switchRun.RunID)
				if next == "" {
					return err
				}
				teamID = ""
				agentID = next
				continue
			}
			var switchTeam *tui.MonitorSwitchTeamError
			if errors.As(err, &switchTeam) {
				next := strings.TrimSpace(switchTeam.TeamID)
				if next == "" {
					return err
				}
				agentID = ""
				teamID = next
				continue
			}
			return err
		}
	},
}

func init() {
	monitorCmd.Flags().StringVar(&monitorAgentID, "agent-id", "", "agent ID to attach to")
	monitorCmd.Flags().StringVar(&monitorTeamID, "team-id", "", "team ID to attach to in a team-scoped session")
}
