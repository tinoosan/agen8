package tui

import (
	"strings"

	"github.com/tinoosan/workbench-core/pkg/fsutil"
)

func (m *monitorModel) workspaceDir() string {
	if m == nil {
		return ""
	}
	dataDir := strings.TrimSpace(m.cfg.DataDir)
	if dataDir == "" {
		return ""
	}
	if teamID := strings.TrimSpace(m.teamID); teamID != "" {
		return fsutil.GetTeamWorkspaceDir(dataDir, teamID)
	}
	runID := strings.TrimSpace(m.runID)
	if strings.HasPrefix(runID, "team:") {
		teamID := strings.TrimSpace(strings.TrimPrefix(runID, "team:"))
		if teamID != "" {
			return fsutil.GetTeamWorkspaceDir(dataDir, teamID)
		}
	}
	if runID == "" {
		return ""
	}
	return fsutil.GetWorkspaceDir(dataDir, runID)
}
