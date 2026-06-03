package fsutil

import (
	"fmt"
	"os"
	"path/filepath"
)

func GetSQLitePath(dataDir string) string {
	return filepath.Join(dataDir, "agen8.db")
}

func GetAgentsHomeDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, ".agents"), nil
}

func GetAgentsSkillsDir() (string, error) {
	agentsHome, err := GetAgentsHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(agentsHome, "skills"), nil
}

func GetProjectWorkspaceDir(projectRoot string) string {
	return filepath.Join(projectRoot, "workspace")
}
