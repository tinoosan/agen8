package cmd

import (
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

func projectModeDefault(ctx app.ProjectContext) string {
	if !ctx.Exists {
		return "standalone"
	}
	mode := strings.ToLower(strings.TrimSpace(ctx.Config.DefaultMode))
	if mode == "team" {
		return "team"
	}
	return "standalone"
}

func projectProfileDefault(ctx app.ProjectContext, mode string) string {
	if !ctx.Exists {
		return ""
	}
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode == "team" {
		if p := strings.TrimSpace(ctx.Config.DefaultTeamProfile); p != "" {
			return p
		}
	}
	return strings.TrimSpace(ctx.Config.DefaultProfile)
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
			if !rootFlags.Changed("profile") && strings.TrimSpace(os.Getenv("AGEN8_PROFILE")) == "" && strings.TrimSpace(profileRef) == "" {
				if p := projectProfileDefault(ctx, projectModeDefault(ctx)); p != "" {
					profileRef = p
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
