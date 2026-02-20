package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tinoosan/agen8/internal/app"
	"github.com/tinoosan/agen8/pkg/protocol"
)

var (
	initProjectID          string
	initDefaultProfile     string
	initDefaultMode        string
	initDefaultTeamProfile string
	initRPCEndpoint        string
	initDataDirOverride    string

	newMode    string
	newProfile string
	newModel   string
	newAttach  bool

	coordinatorSessionID string

	attachSessionID string
)

var runCoordinatorFn func(cmd *cobra.Command, sessionID string) error
var runCoordinatorShellFn func(cmd *cobra.Command, sessionID string, runID string, teamID string) error

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize a project-local .agen8 workspace",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, err := app.InitProject(projectSearchDir(), app.ProjectConfig{
			ProjectID:          strings.TrimSpace(initProjectID),
			DefaultProfile:     strings.TrimSpace(initDefaultProfile),
			DefaultMode:        strings.TrimSpace(initDefaultMode),
			DefaultTeamProfile: strings.TrimSpace(initDefaultTeamProfile),
			RPCEndpoint:        strings.TrimSpace(initRPCEndpoint),
			DataDirOverride:    strings.TrimSpace(initDataDirOverride),
		})
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Initialized %s in %s\n", app.ProjectDirName, ctx.RootDir)
		return nil
	},
}

var newCmd = &cobra.Command{
	Use:   "new",
	Short: "Create a new session for the current project",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runNewSessionFlow(cmd, newAttach)
	},
}

var coordinatorCmd = &cobra.Command{
	Use:   "coordinator",
	Short: "Attach to a session coordinator view",
	RunE: func(cmd *cobra.Command, args []string) error {
		sessionID := strings.TrimSpace(coordinatorSessionID)
		if sessionID == "" {
			projectCtx, err := loadProjectContext()
			if err == nil && projectCtx.Exists {
				sessionID = strings.TrimSpace(projectCtx.State.ActiveSessionID)
			}
		}
		if sessionID == "" {
			return fmt.Errorf("no active session; use `agen8 new` or pass --session-id")
		}
		return runCoordinatorFn(cmd, sessionID)
	},
}

var attachCmd = &cobra.Command{
	Use:   "attach [session-id]",
	Short: "Attach to an existing session coordinator",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		sessionID := strings.TrimSpace(attachSessionID)
		if sessionID == "" && len(args) > 0 {
			sessionID = strings.TrimSpace(args[0])
		}
		if sessionID == "" {
			return fmt.Errorf("session id is required")
		}
		return runCoordinatorFn(cmd, sessionID)
	},
}

func runCoordinatorForSession(cmd *cobra.Command, sessionID string) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return fmt.Errorf("session id is required")
	}
	cfg, err := effectiveConfig(cmd)
	if err != nil {
		return err
	}
	runID, teamID, err := rpcResolveCoordinatorRun(cmd.Context(), sessionID)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "thread not found") {
			if resolved, rerr := rpcResolveThread(cmd.Context(), sessionID, ""); rerr == nil && resolved.Exists {
				runID = strings.TrimSpace(resolved.RunID)
				teamID = strings.TrimSpace(resolved.TeamID)
			} else {
				return err
			}
		} else {
			return err
		}
	}
	if runID == "" {
		return fmt.Errorf("no run found for session %s", sessionID)
	}
	if err := updateProjectActiveSession(sessionID, teamID, runID, "coordinator"); err != nil {
		return err
	}
	_ = cfg
	return runCoordinatorShellFn(cmd, sessionID, runID, teamID)
}

func runNewSessionFlow(cmd *cobra.Command, attach bool) error {
	if err := rpcPing(cmd.Context()); err != nil {
		return err
	}

	projectCtx, err := loadProjectContext()
	if err != nil {
		return err
	}
	mode := strings.ToLower(strings.TrimSpace(newMode))
	if mode == "" {
		mode = projectModeDefault(projectCtx)
	}
	if mode != "team" && mode != "standalone" {
		return fmt.Errorf("--mode must be standalone or team")
	}
	profile := strings.TrimSpace(newProfile)
	if profile == "" {
		profile = projectProfileDefault(projectCtx, mode)
	}
	if profile == "" {
		profile = strings.TrimSpace(profileRef)
	}
	if mode == "team" && profile == "" {
		return fmt.Errorf("team mode requires --profile or project default_team_profile")
	}

	var out protocol.SessionStartResult
	if err := rpcCall(cmd.Context(), protocol.MethodSessionStart, protocol.SessionStartParams{
		ThreadID: detachedThreadID,
		Mode:     mode,
		Profile:  profile,
		Model:    strings.TrimSpace(newModel),
	}, &out); err != nil {
		return err
	}

	if err := updateProjectActiveSession(out.SessionID, out.TeamID, out.PrimaryRunID, "new"); err != nil {
		return err
	}

	if attach {
		return runCoordinatorFn(cmd, out.SessionID)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Created %s session %s (run %s)\n", mode, out.SessionID, out.PrimaryRunID)
	return nil
}

func init() {
	runCoordinatorFn = runCoordinatorForSession
	runCoordinatorShellFn = runCoordinatorShell

	initCmd.Flags().StringVar(&initProjectID, "project-id", "", "override project identifier")
	initCmd.Flags().StringVar(&initDefaultProfile, "profile", "", "default standalone profile for this project")
	initCmd.Flags().StringVar(&initDefaultMode, "mode", "standalone", "default mode (standalone|team)")
	initCmd.Flags().StringVar(&initDefaultTeamProfile, "team-profile", "", "default team profile for this project")
	initCmd.Flags().StringVar(&initRPCEndpoint, "rpc-endpoint", "", "default RPC endpoint for this project")
	initCmd.Flags().StringVar(&initDataDirOverride, "data-dir", "", "project-level data-dir override")

	newCmd.Flags().StringVar(&newMode, "mode", "", "session mode (standalone|team)")
	newCmd.Flags().StringVar(&newProfile, "profile", "", "profile id/path")
	newCmd.Flags().StringVar(&newModel, "model", "", "model override")
	newCmd.Flags().BoolVar(&newAttach, "attach", true, "attach coordinator after creating session")

	coordinatorCmd.Flags().StringVar(&coordinatorSessionID, "session-id", "", "session id to attach")
	attachCmd.Flags().StringVar(&attachSessionID, "session-id", "", "session id to attach")

	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(newCmd)
	rootCmd.AddCommand(coordinatorCmd)
	rootCmd.AddCommand(attachCmd)
}
