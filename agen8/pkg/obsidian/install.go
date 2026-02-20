package obsidian

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

type InstallStatus struct {
	Installed bool
	Source    string
}

func DetectInstall() InstallStatus {
	if path, err := exec.LookPath("obsidian"); err == nil && strings.TrimSpace(path) != "" {
		return InstallStatus{Installed: true, Source: path}
	}

	switch runtime.GOOS {
	case "darwin":
		candidates := []string{
			"/Applications/Obsidian.app",
			filepath.Join(userHomeDir(), "Applications", "Obsidian.app"),
		}
		for _, cand := range candidates {
			if cand == "" {
				continue
			}
			if st, err := os.Stat(cand); err == nil && st.IsDir() {
				return InstallStatus{Installed: true, Source: cand}
			}
		}
	case "windows":
		candidates := []string{
			filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "Obsidian", "Obsidian.exe"),
			filepath.Join(os.Getenv("ProgramFiles"), "Obsidian", "Obsidian.exe"),
			filepath.Join(os.Getenv("ProgramFiles(x86)"), "Obsidian", "Obsidian.exe"),
		}
		for _, cand := range candidates {
			if cand == "" {
				continue
			}
			if st, err := os.Stat(cand); err == nil && !st.IsDir() {
				return InstallStatus{Installed: true, Source: cand}
			}
		}
	}

	return InstallStatus{}
}

func userHomeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return home
}
