package cmd

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tinoosan/agen8/internal/app"
	"github.com/tinoosan/agen8/internal/tui/coordinator"
	"github.com/tinoosan/agen8/pkg/protocol"
)

var (
	initProjectID       string
	initRPCEndpoint     string
	initDataDirOverride string

	newProfile string
	newModel   string
	newAttach  bool
	newTeamID  string

	coordinatorSessionID string

	attachSessionID string
)

var runCoordinatorFn func(cmd *cobra.Command, sessionID string) error
var runCoordinatorShellFn func(cmd *cobra.Command, sessionID string, runID string, teamID string) error
var requireRuntimeAuthReadyFn = requireRuntimeAuthReady

var initCmd = &cobra.Command{
	Use:    "init",
	Short:  "Initialize a project-local .agen8 workspace",
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, err := app.InitProject(projectSearchDir(), app.ProjectConfig{
			ProjectID:       strings.TrimSpace(initProjectID),
			RPCEndpoint:     strings.TrimSpace(initRPCEndpoint),
			DataDirOverride: strings.TrimSpace(initDataDirOverride),
		})
		if err != nil {
			return err
		}
		cfg, err := effectiveConfig(cmd)
		if err == nil {
			registrySvc := app.NewProjectRegistryService(cfg)
			_, _ = registrySvc.RegisterProject(cmd.Context(), app.ProjectRegistrySummary{
				ProjectRoot:  strings.TrimSpace(ctx.RootDir),
				ProjectID:    strings.TrimSpace(ctx.Config.ProjectID),
				ManifestPath: filepath.Join(strings.TrimSpace(ctx.RootDir), app.ProjectDirName, app.ProjectDesiredStateFilename),
				Enabled:      true,
				Metadata: map[string]any{
					"source": "project.init",
				},
			})
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Initialized %s in %s\n", app.ProjectDirName, ctx.RootDir)
		fmt.Fprintf(cmd.OutOrStdout(), "Created %s\n", filepath.Join(app.ProjectDirName, app.ProjectDesiredStateFilename))
		return nil
	},
}

var newCmd = &cobra.Command{
	Use:    "new",
	Short:  "Create a new session for the current project",
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runNewSessionFlow(cmd, newAttach)
	},
}

var coordinatorCmd = &cobra.Command{
	Use:    "coordinator",
	Short:  "Attach to a session coordinator view",
	Hidden: true,
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
	Use:    "attach [session-id]",
	Short:  "Attach to an existing session coordinator",
	Hidden: true,
	Args:   cobra.MaximumNArgs(1),
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
	ctx := context.Background()
	if cmd != nil && cmd.Context() != nil {
		ctx = cmd.Context()
	}
	if err := requireRuntimeAuthReadyFn(ctx, cfg); err != nil {
		return err
	}
	runID, teamID, err := rpcResolveCoordinatorRun(ctx, sessionID)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "thread not found") {
			if resolved, rerr := rpcResolveThread(ctx, sessionID, ""); rerr == nil && resolved.Exists {
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
	return runCoordinatorShellFn(cmd, sessionID, runID, teamID)
}

func runNewSessionFlow(cmd *cobra.Command, attach bool) error {
	cfg, err := effectiveConfig(cmd)
	if err != nil {
		return err
	}
	ctx := context.Background()
	if cmd != nil && cmd.Context() != nil {
		ctx = cmd.Context()
	}
	if err := requireRuntimeAuthReadyFn(ctx, cfg); err != nil {
		return err
	}
	if err := rpcPing(ctx); err != nil {
		return err
	}

	projectCtx, err := loadProjectContext()
	if err != nil {
		return err
	}
	profile := strings.TrimSpace(newProfile)
	if profile == "" {
		return fmt.Errorf("profile ref is required; use `agen8 team start <profile-ref>`")
	}

	projectRoot := ""
	projectID := ""
	if projectCtx.Exists {
		projectRoot = strings.TrimSpace(projectCtx.RootDir)
		projectID = strings.TrimSpace(projectCtx.Config.ProjectID)
	}

	var out protocol.SessionStartResult
	if err := rpcCall(ctx, protocol.MethodSessionStart, protocol.SessionStartParams{
		ThreadID:    detachedThreadID,
		Profile:     profile,
		Model:       strings.TrimSpace(newModel),
		TeamID:      strings.TrimSpace(newTeamID),
		ProjectID:   projectID,
		ProjectRoot: projectRoot,
	}, &out); err != nil {
		return err
	}

	if err := updateProjectActiveSession(out.SessionID, out.TeamID, out.PrimaryRunID, "new"); err != nil {
		return err
	}

	if attach {
		return runCoordinatorFn(cmd, out.SessionID)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Started team %s\n", blankDash(out.TeamID))
	fmt.Fprintf(cmd.OutOrStdout(), "Status: active\n")
	return nil
}

func init() {
	runCoordinatorFn = runCoordinatorForSession
	runCoordinatorShellFn = func(cmd *cobra.Command, sessionID string, runID string, teamID string) error {
		return coordinator.Run(resolvedRPCEndpoint(), sessionID)
	}

	initCmd.Flags().StringVar(&initProjectID, "project-id", "", "override project identifier")
	initCmd.Flags().StringVar(&initRPCEndpoint, "rpc-endpoint", "", "default RPC endpoint for this project")
	initCmd.Flags().StringVar(&initDataDirOverride, "data-dir", "", "project-level data-dir override")

	newCmd.Flags().StringVar(&newProfile, "profile", "", "profile id/path")
	newCmd.Flags().StringVar(&newModel, "model", "", "model override")
	newCmd.Flags().BoolVar(&newAttach, "attach", true, "attach coordinator after creating session")

	coordinatorCmd.Flags().StringVar(&coordinatorSessionID, "session-id", "", "session id to attach")
	attachCmd.Flags().StringVar(&attachSessionID, "session-id", "", "session id to attach")

}
