package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

var (
	whoamiSessionID string
	whoamiRunID     string
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Run local diagnostics for daemon/project/config",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := effectiveConfig(cmd)
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "data_dir: %s\n", cfg.DataDir)
		if err := rpcPing(cmd.Context()); err != nil {
			fmt.Fprintf(cmd.OutOrStdout(), "rpc: down (%v)\n", err)
		} else {
			fmt.Fprintln(cmd.OutOrStdout(), "rpc: ok")
		}
		projectCtx, err := loadProjectContext()
		if err != nil {
			return err
		}
		if !projectCtx.Exists {
			fmt.Fprintln(cmd.OutOrStdout(), "project: not initialized")
			return nil
		}
		fmt.Fprintf(cmd.OutOrStdout(), "project_root: %s\n", projectCtx.RootDir)
		fmt.Fprintf(cmd.OutOrStdout(), "project_mode: %s\n", projectModeDefault(projectCtx))
		fmt.Fprintf(cmd.OutOrStdout(), "active_session: %s\n", blankDash(projectCtx.State.ActiveSessionID))
		fmt.Fprintf(cmd.OutOrStdout(), "active_run: %s\n", blankDash(projectCtx.State.ActiveRunID))
		return nil
	},
}

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Show merged effective configuration",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := effectiveConfig(cmd)
		if err != nil {
			return err
		}
		projectCtx, err := loadProjectContext()
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "data_dir=%s\n", cfg.DataDir)
		fmt.Fprintf(cmd.OutOrStdout(), "workdir=%s\n", blankDash(strings.TrimSpace(workDir)))
		fmt.Fprintf(cmd.OutOrStdout(), "rpc_endpoint=%s\n", resolvedRPCEndpoint())
		fmt.Fprintf(cmd.OutOrStdout(), "profile=%s\n", blankDash(strings.TrimSpace(profileRef)))
		fmt.Fprintf(cmd.OutOrStdout(), "model=%s\n", blankDash(strings.TrimSpace(modelID)))
		if projectCtx.Exists {
			fmt.Fprintf(cmd.OutOrStdout(), "project.root=%s\n", projectCtx.RootDir)
			fmt.Fprintf(cmd.OutOrStdout(), "project.mode=%s\n", projectModeDefault(projectCtx))
			fmt.Fprintf(cmd.OutOrStdout(), "project.default_profile=%s\n", blankDash(projectCtx.Config.DefaultProfile))
			fmt.Fprintf(cmd.OutOrStdout(), "project.default_team_profile=%s\n", blankDash(projectCtx.Config.DefaultTeamProfile))
			fmt.Fprintf(cmd.OutOrStdout(), "project.active_session=%s\n", blankDash(projectCtx.State.ActiveSessionID))
			fmt.Fprintf(cmd.OutOrStdout(), "project.active_run=%s\n", blankDash(projectCtx.State.ActiveRunID))
		} else {
			fmt.Fprintln(cmd.OutOrStdout(), "project.root=-")
		}
		return nil
	},
}

var whoamiCmd = &cobra.Command{
	Use:   "whoami",
	Short: "Show current project/session/run context",
	RunE: func(cmd *cobra.Command, args []string) error {
		projectCtx, err := loadProjectContext()
		if err != nil {
			return err
		}
		sessionID := strings.TrimSpace(whoamiSessionID)
		runID := strings.TrimSpace(whoamiRunID)
		teamID := ""
		if projectCtx.Exists {
			if sessionID == "" {
				sessionID = strings.TrimSpace(projectCtx.State.ActiveSessionID)
			}
			if runID == "" {
				runID = strings.TrimSpace(projectCtx.State.ActiveRunID)
			}
		}
		if sessionID != "" && runID == "" {
			resolvedRunID, resolvedTeamID, err := rpcResolveCoordinatorRun(cmd.Context(), sessionID)
			if err == nil {
				runID = resolvedRunID
				teamID = resolvedTeamID
			}
		}
		mode := "standalone"
		profile := ""
		if sessionID != "" {
			if item, err := rpcFindSession(cmd.Context(), sessionID); err == nil && item != nil {
				mode = fallback(item.Mode, mode)
				profile = strings.TrimSpace(item.Profile)
				if teamID == "" {
					teamID = strings.TrimSpace(item.TeamID)
				}
				if runID == "" {
					runID = strings.TrimSpace(item.CurrentRunID)
				}
			}
		}
		fmt.Fprintf(cmd.OutOrStdout(), "project=%s\n", func() string {
			if projectCtx.Exists {
				return projectCtx.RootDir
			}
			cwd, _ := os.Getwd()
			return cwd
		}())
		fmt.Fprintf(cmd.OutOrStdout(), "session=%s\n", blankDash(sessionID))
		fmt.Fprintf(cmd.OutOrStdout(), "run=%s\n", blankDash(runID))
		fmt.Fprintf(cmd.OutOrStdout(), "team=%s\n", blankDash(teamID))
		fmt.Fprintf(cmd.OutOrStdout(), "mode=%s\n", mode)
		fmt.Fprintf(cmd.OutOrStdout(), "profile=%s\n", blankDash(profile))
		return nil
	},
}

var watchCmd = &cobra.Command{
	Use:   "watch",
	Short: "Shortcut for activity TUI",
	RunE: func(cmd *cobra.Command, args []string) error {
		return activityCmd.RunE(cmd, args)
	},
}

func init() {
	whoamiCmd.Flags().StringVar(&whoamiSessionID, "session-id", "", "session id override")
	whoamiCmd.Flags().StringVar(&whoamiRunID, "run-id", "", "run id override")

	rootCmd.AddCommand(doctorCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(whoamiCmd)
	rootCmd.AddCommand(watchCmd)
}
