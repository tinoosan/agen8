package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/tinoosan/agen8/internal/app"
	"github.com/tinoosan/agen8/pkg/fsutil"
	"github.com/tinoosan/agen8/pkg/protocol"
)

var (
	teamStartModel string
	teamDeleteID   string
	taskSendRole   string
	taskListView   string
	taskListLimit  int
	taskListOffset int
)

var projectCmd = &cobra.Command{
	Use:   "project",
	Short: "Manage Agen8 project setup",
}

var projectInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Bind the current project to Agen8",
	RunE: func(cmd *cobra.Command, args []string) error {
		return initCmd.RunE(cmd, args)
	},
}

var projectStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show project runtime status",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := effectiveConfig(cmd)
		if err != nil {
			return err
		}
		projectCtx, err := loadProjectContext()
		if err != nil {
			return err
		}

		fmt.Fprintf(cmd.OutOrStdout(), "project=%s\n", projectSearchDir())
		fmt.Fprintf(cmd.OutOrStdout(), "initialized=%t\n", projectCtx.Exists)
		if err := rpcPing(cmd.Context()); err != nil {
			fmt.Fprintln(cmd.OutOrStdout(), "daemon=down")
		} else {
			fmt.Fprintln(cmd.OutOrStdout(), "daemon=up")
		}
		fmt.Fprintf(cmd.OutOrStdout(), "rpc_endpoint=%s\n", resolvedRPCEndpoint())
		fmt.Fprintf(cmd.OutOrStdout(), "data_dir=%s\n", cfg.DataDir)
		if !projectCtx.Exists {
			return nil
		}
		fmt.Fprintf(cmd.OutOrStdout(), "root=%s\n", projectCtx.RootDir)
		fmt.Fprintf(cmd.OutOrStdout(), "active_session=%s\n", blankDash(projectCtx.State.ActiveSessionID))
		fmt.Fprintf(cmd.OutOrStdout(), "active_team=%s\n", blankDash(projectCtx.State.ActiveTeamID))
		fmt.Fprintf(cmd.OutOrStdout(), "active_run=%s\n", blankDash(projectCtx.State.ActiveRunID))
		return nil
	},
}

var projectDeleteTeamsCmd = &cobra.Command{
	Use:   "delete-teams",
	Short: "Delete all teams in the current project and their related runtime data",
	RunE: func(cmd *cobra.Command, args []string) error {
		projectCtx, err := loadProjectContext()
		if err != nil {
			return err
		}
		if !projectCtx.Exists {
			return fmt.Errorf("project is not initialized; run `agen8 project init` first")
		}
		out, err := rpcDeleteProjectTeams(cmd.Context(), strings.TrimSpace(projectCtx.RootDir))
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Deleted %d team(s)\n", len(out.DeletedTeamIDs))
		if len(out.DeletedTeamIDs) > 0 {
			fmt.Fprintf(cmd.OutOrStdout(), "Teams: %s\n", strings.Join(out.DeletedTeamIDs, ", "))
		}
		if len(out.DeletedSessionIDs) > 0 {
			fmt.Fprintf(cmd.OutOrStdout(), "Sessions: %s\n", strings.Join(out.DeletedSessionIDs, ", "))
		}
		return nil
	},
}

var teamCmd = &cobra.Command{
	Use:   "team",
	Short: "Start and inspect available profile-backed teams",
}

var teamListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available profile-backed team definitions",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := effectiveConfig(cmd)
		if err != nil {
			return err
		}
		projectCtx, err := loadProjectContext()
		if err != nil {
			return err
		}
		if !projectCtx.Exists {
			return fmt.Errorf("project is not initialized; run `agen8 project init` first")
		}

		type teamEntry struct {
			Name   string
			Source string
			Active bool
		}
		entries := []teamEntry{}
		seen := map[string]struct{}{}
		add := func(name, source string, isActive bool) {
			name = strings.TrimSpace(name)
			if name == "" {
				return
			}
			if _, ok := seen[name]; ok {
				for i := range entries {
					if entries[i].Name == name {
						entries[i].Active = entries[i].Active || isActive
					}
				}
				return
			}
			seen[name] = struct{}{}
			entries = append(entries, teamEntry{Name: name, Source: source, Active: isActive})
		}

		activeProfile := ""
		if projectRoot := strings.TrimSpace(projectCtx.RootDir); projectRoot != "" {
			if teamID := strings.TrimSpace(projectCtx.State.ActiveTeamID); teamID != "" {
				if teamInfo, err := rpcGetProjectTeam(cmd.Context(), projectRoot, teamID); err == nil {
					activeProfile = strings.TrimSpace(teamInfo.ProfileID)
				}
			}
		}
		for _, name := range listProfileRefs(fsutil.GetProfilesDir(cfg.DataDir)) {
			add(name, "shared-profile", activeProfile != "" && activeProfile == name)
		}
		for _, name := range listProfileRefs(filepath.Join(projectCtx.ProjectDir, "profiles")) {
			add(name, "project-profile", activeProfile != "" && activeProfile == name)
		}

		if activeProfile != "" {
			add(activeProfile, "active-session", true)
		}

		if len(entries) == 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "No available team profiles found in shared or project-local profiles directories.")
			return nil
		}

		slices.SortFunc(entries, func(a, b teamEntry) int {
			return strings.Compare(a.Name, b.Name)
		})
		w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 2, 2, ' ', 0)
		fmt.Fprintln(w, "TEAM\tSOURCE\tACTIVE")
		for _, entry := range entries {
			fmt.Fprintf(w, "%s\t%s\t%t\n", entry.Name, entry.Source, entry.Active)
		}
		return w.Flush()
	},
}

var teamStartCmd = &cobra.Command{
	Use:   "start <profile-ref>",
	Short: "Start work with a profile-backed team definition",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		newProfile = strings.TrimSpace(args[0])
		newAttach = false
		if strings.TrimSpace(teamStartModel) != "" {
			newModel = strings.TrimSpace(teamStartModel)
		}
		if err := runNewSessionFlow(cmd, false); err != nil {
			return err
		}
		projectCtx, err := loadProjectContext()
		if err != nil || !projectCtx.Exists {
			return nil
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Next: `agen8 monitor`\n")
		return nil
	},
}

var teamDeleteCmd = &cobra.Command{
	Use:   "delete [team-id]",
	Short: "Delete a team and all related runtime data",
	Args:  cobra.RangeArgs(0, 1),
	RunE: func(cmd *cobra.Command, args []string) error {
		projectCtx, err := loadProjectContext()
		if err != nil {
			return err
		}
		teamID := strings.TrimSpace(teamDeleteID)
		if len(args) > 0 {
			teamID = strings.TrimSpace(args[0])
		}
		if teamID == "" && projectCtx.Exists {
			teamID = strings.TrimSpace(projectCtx.State.ActiveTeamID)
		}
		if teamID == "" {
			return fmt.Errorf("team id is required")
		}
		projectRoot := ""
		if projectCtx.Exists {
			projectRoot = strings.TrimSpace(projectCtx.RootDir)
		}
		out, err := rpcDeleteTeam(cmd.Context(), teamID, projectRoot)
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Deleted team %s\n", blankDash(out.TeamID))
		if len(out.DeletedSessionIDs) > 0 {
			fmt.Fprintf(cmd.OutOrStdout(), "Deleted sessions: %s\n", strings.Join(out.DeletedSessionIDs, ", "))
		}
		return nil
	},
}

var taskCmd = &cobra.Command{
	Use:   "task",
	Short: "Send and inspect work for the active team",
}

var taskSendCmd = &cobra.Command{
	Use:   "send <goal>",
	Short: "Send a task to the active team",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		scope, err := loadActiveTaskScope(cmd)
		if err != nil {
			return err
		}
		var out protocol.TaskCreateResult
		params := protocol.TaskCreateParams{
			ThreadID: protocol.ThreadID(scope.ActiveSessionID),
			TeamID:   scope.ActiveTeamID,
			RunID:    scope.ActiveRunID,
			Goal:     strings.TrimSpace(args[0]),
		}
		if role := strings.TrimSpace(taskSendRole); role != "" {
			params.AssignedToType = "role"
			params.AssignedTo = role
			params.AssignedRole = role
		}
		if err := rpcCall(cmd.Context(), protocol.MethodTaskCreate, params, &out); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Sent task %s\n", blankDash(out.Task.ID))
		return nil
	},
}

var taskListCmd = &cobra.Command{
	Use:   "list",
	Short: "List tasks for the active team",
	RunE: func(cmd *cobra.Command, args []string) error {
		scope, err := loadActiveTaskScope(cmd)
		if err != nil {
			return err
		}
		view := strings.ToLower(strings.TrimSpace(taskListView))
		if view == "" {
			view = "inbox"
		}
		var out protocol.TaskListResult
		if err := rpcCall(cmd.Context(), protocol.MethodTaskList, protocol.TaskListParams{
			ThreadID: protocol.ThreadID(scope.ActiveSessionID),
			TeamID:   scope.ActiveTeamID,
			RunID:    scope.ActiveRunID,
			View:     view,
			Limit:    taskListLimit,
			Offset:   taskListOffset,
		}, &out); err != nil {
			return err
		}
		w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 2, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tSTATUS\tASSIGNEE\tGOAL")
		for _, task := range out.Tasks {
			assignee := strings.TrimSpace(task.AssignedRole)
			if assignee == "" {
				assignee = strings.TrimSpace(task.AssignedTo)
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", blankDash(task.ID), blankDash(task.Status), blankDash(assignee), strings.TrimSpace(task.Goal))
		}
		return w.Flush()
	},
}

var viewCmd = &cobra.Command{
	Use:   "view",
	Short: "Open focused operational views",
}

var viewDashboardCmd = &cobra.Command{
	Use:   "dashboard",
	Short: "Open the dashboard view",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runDashboardFlow(cmd)
	},
}

var viewActivityCmd = &cobra.Command{
	Use:   "activity",
	Short: "Open the activity view",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runActivityTUI(cmd)
	},
}

var viewMailCmd = &cobra.Command{
	Use:   "mail",
	Short: "Open the mail view",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runMailTUI(cmd)
	},
}

func loadActiveTaskScope(cmd *cobra.Command) (app.ProjectState, error) {
	_, teamID, sessionID, runID, err := resolveActiveProjectScope(cmd.Context())
	if err != nil {
		return app.ProjectState{}, err
	}
	return app.ProjectState{
		ActiveSessionID: sessionID,
		ActiveTeamID:    teamID,
		ActiveRunID:     runID,
	}, nil
}

func listProfileRefs(dir string) []string {
	dir = strings.TrimSpace(dir)
	items, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		if !item.IsDir() {
			continue
		}
		name := strings.TrimSpace(item.Name())
		if name == "" {
			continue
		}
		if _, err := os.Stat(filepath.Join(dir, name, "profile.yaml")); err != nil {
			continue
		}
		out = append(out, name)
	}
	slices.Sort(out)
	return out
}

func init() {
	projectInitCmd.Flags().StringVar(&initProjectID, "project-id", "", "override project identifier")
	projectInitCmd.Flags().StringVar(&initRPCEndpoint, "rpc-endpoint", "", "default RPC endpoint for this project")
	projectInitCmd.Flags().StringVar(&initDataDirOverride, "data-dir", "", "project-level data-dir override")

	teamStartCmd.Flags().StringVar(&teamStartModel, "model", "", "model override for the started team")
	teamDeleteCmd.Flags().StringVar(&teamDeleteID, "team-id", "", "team id to delete (defaults to active project team)")

	taskSendCmd.Flags().StringVar(&taskSendRole, "role", "", "assign the task to a specific role")
	taskListCmd.Flags().StringVar(&taskListView, "view", "inbox", "task view to inspect (inbox|outbox)")
	taskListCmd.Flags().IntVar(&taskListLimit, "limit", 50, "max tasks to show")
	taskListCmd.Flags().IntVar(&taskListOffset, "offset", 0, "skip N tasks")

	projectCmd.AddCommand(projectInitCmd)
	projectCmd.AddCommand(projectStatusCmd)
	projectCmd.AddCommand(projectDeleteTeamsCmd)
	teamCmd.AddCommand(teamListCmd)
	teamCmd.AddCommand(teamStartCmd)
	teamCmd.AddCommand(teamDeleteCmd)
	taskCmd.AddCommand(taskSendCmd)
	taskCmd.AddCommand(taskListCmd)
	viewCmd.AddCommand(viewDashboardCmd)
	viewCmd.AddCommand(viewActivityCmd)
	viewCmd.AddCommand(viewMailCmd)

	rootCmd.AddCommand(projectCmd)
	rootCmd.AddCommand(teamCmd)
	rootCmd.AddCommand(taskCmd)
	rootCmd.AddCommand(viewCmd)
}
