package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tinoosan/agen8/internal/app"
)

func projectSearchDir() string {
	if wd := strings.TrimSpace(workDir); wd != "" {
		return wd
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return cwd
}

func loadProjectContext() (app.ProjectContext, error) {
	return app.LoadProjectContext(projectSearchDir())
}

func applyProjectDefaults(cmd *cobra.Command) error {
	ctx, err := loadProjectContext()
	if err != nil {
		return err
	}
	if !ctx.Exists {
		return nil
	}

	if cmd != nil {
		rootFlags := cmd.Root().PersistentFlags()
		if rootFlags != nil {
			if !rootFlags.Changed("rpc-endpoint") && strings.TrimSpace(os.Getenv("AGEN8_RPC_ENDPOINT")) == "" {
				if endpoint := strings.TrimSpace(ctx.Config.RPCEndpoint); endpoint != "" {
					rpcEndpoint = endpoint
				}
			}
		}
	}
	return nil
}

func updateProjectActiveSession(sessionID, teamID, runID, lastCommand string) error {
	ctx, err := loadProjectContext()
	if err != nil {
		return err
	}
	if !ctx.Exists {
		return nil
	}
	_, err = app.SetActiveSession(ctx.RootDir, app.ProjectState{
		ActiveSessionID: strings.TrimSpace(sessionID),
		ActiveTeamID:    strings.TrimSpace(teamID),
		ActiveRunID:     strings.TrimSpace(runID),
		LastCommand:     strings.TrimSpace(lastCommand),
	})
	if err != nil {
		return fmt.Errorf("update %s/state.json: %w", app.ProjectDirName, err)
	}
	return nil
}

func resolveActiveProjectScope(ctx context.Context) (projectRoot, teamID, sessionID, runID string, err error) {
	projectCtx, err := loadProjectContext()
	if err != nil {
		return "", "", "", "", err
	}
	if !projectCtx.Exists {
		return "", "", "", "", fmt.Errorf("project is not initialized; run `agen8 project init` first")
	}
	projectRoot = strings.TrimSpace(projectCtx.RootDir)
	teamID = strings.TrimSpace(projectCtx.State.ActiveTeamID)
	sessionID = strings.TrimSpace(projectCtx.State.ActiveSessionID)
	runID = strings.TrimSpace(projectCtx.State.ActiveRunID)
	if teamID == "" {
		return "", "", "", "", fmt.Errorf("no active team; run `agen8 team start <profile-ref>` first")
	}
	teamInfo, err := rpcGetProjectTeam(ctx, projectRoot, teamID)
	if err != nil {
		return "", "", "", "", err
	}
	if sessionID == "" {
		sessionID = strings.TrimSpace(teamInfo.PrimarySessionID)
	}
	if sessionID == "" {
		return "", "", "", "", fmt.Errorf("team %s is not ready (no active control session)", teamID)
	}
	if runID == "" {
		runID = strings.TrimSpace(teamInfo.CoordinatorRunID)
	}
	if runID == "" {
		if resolvedRunID, _, rerr := rpcResolveCoordinatorRun(ctx, sessionID); rerr == nil {
			runID = strings.TrimSpace(resolvedRunID)
		}
	}
	if runID == "" {
		return "", "", "", "", fmt.Errorf("coordinator run unavailable for active team %s", teamID)
	}
	return projectRoot, teamID, sessionID, runID, nil
}
