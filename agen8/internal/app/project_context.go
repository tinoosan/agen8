package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	ProjectDirName        = ".agent8"
	projectConfigFilename = "config.toml"
	projectStateFilename  = "state.json"
	projectReadmeFilename = "README.md"
	projectProfilesDir    = "profiles"
	ProjectConfigVersion  = 1
)

// ProjectConfig stores per-project defaults.
type ProjectConfig struct {
	ProjectID          string
	DefaultProfile     string
	DefaultMode        string // standalone|team
	DefaultTeamProfile string
	RPCEndpoint        string
	DataDirOverride    string
	CreatedAt          string
	Version            int
}

// ProjectState stores mutable project affinity pointers.
type ProjectState struct {
	ActiveSessionID string `json:"active_session_id,omitempty"`
	ActiveTeamID    string `json:"active_team_id,omitempty"`
	ActiveRunID     string `json:"active_run_id,omitempty"`
	LastAttachedAt  string `json:"last_attached_at,omitempty"`
	LastCommand     string `json:"last_command,omitempty"`
}

// ProjectContext is the discovered context for a cwd.
type ProjectContext struct {
	Cwd        string
	RootDir    string
	ProjectDir string
	ConfigPath string
	StatePath  string
	Exists     bool
	Config     ProjectConfig
	State      ProjectState
}

func defaultProjectConfig(baseDir string) ProjectConfig {
	projectID := strings.TrimSpace(filepath.Base(baseDir))
	if projectID == "" || projectID == "." || projectID == string(filepath.Separator) {
		projectID = "agen8-project"
	}
	return ProjectConfig{
		ProjectID:   projectID,
		DefaultMode: "standalone",
		CreatedAt:   time.Now().UTC().Format(time.RFC3339Nano),
		Version:     ProjectConfigVersion,
	}
}

func normalizeProjectConfig(cfg ProjectConfig, baseDir string) ProjectConfig {
	out := cfg
	if out.Version <= 0 {
		out.Version = ProjectConfigVersion
	}
	out.ProjectID = strings.TrimSpace(out.ProjectID)
	if out.ProjectID == "" {
		out.ProjectID = defaultProjectConfig(baseDir).ProjectID
	}
	out.DefaultProfile = strings.TrimSpace(out.DefaultProfile)
	out.DefaultTeamProfile = strings.TrimSpace(out.DefaultTeamProfile)
	out.RPCEndpoint = strings.TrimSpace(out.RPCEndpoint)
	out.DataDirOverride = strings.TrimSpace(out.DataDirOverride)
	out.CreatedAt = strings.TrimSpace(out.CreatedAt)
	if out.CreatedAt == "" {
		out.CreatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	mode := strings.ToLower(strings.TrimSpace(out.DefaultMode))
	switch mode {
	case "team", "standalone":
		out.DefaultMode = mode
	default:
		out.DefaultMode = "standalone"
	}
	return out
}

func resolveStartDir(start string) (string, error) {
	start = strings.TrimSpace(start)
	if start == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		start = cwd
	}
	abs, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	st, err := os.Stat(abs)
	if err == nil && !st.IsDir() {
		return filepath.Dir(abs), nil
	}
	return abs, nil
}

// FindProjectRoot walks up from start to locate a directory that contains .agent8.
func FindProjectRoot(start string) (string, bool, error) {
	dir, err := resolveStartDir(start)
	if err != nil {
		return "", false, err
	}
	for {
		candidate := filepath.Join(dir, ProjectDirName)
		info, err := os.Stat(candidate)
		if err == nil && info.IsDir() {
			return dir, true, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false, nil
		}
		dir = parent
	}
}

func projectPaths(root string) (projectDir string, configPath string, statePath string) {
	projectDir = filepath.Join(root, ProjectDirName)
	configPath = filepath.Join(projectDir, projectConfigFilename)
	statePath = filepath.Join(projectDir, projectStateFilename)
	return projectDir, configPath, statePath
}

// LoadProjectContext loads .agent8 context for start if present.
func LoadProjectContext(start string) (ProjectContext, error) {
	cwd, err := resolveStartDir(start)
	if err != nil {
		return ProjectContext{}, err
	}
	root, ok, err := FindProjectRoot(cwd)
	if err != nil {
		return ProjectContext{}, err
	}
	if !ok {
		return ProjectContext{Cwd: cwd}, nil
	}
	projectDir, configPath, statePath := projectPaths(root)
	cfg := defaultProjectConfig(root)
	loadedCfg, err := readProjectConfig(configPath, root)
	if err != nil {
		return ProjectContext{}, err
	}
	cfg = normalizeProjectConfig(loadedCfg, root)
	state, err := readProjectState(statePath)
	if err != nil {
		return ProjectContext{}, err
	}
	return ProjectContext{
		Cwd:        cwd,
		RootDir:    root,
		ProjectDir: projectDir,
		ConfigPath: configPath,
		StatePath:  statePath,
		Exists:     true,
		Config:     cfg,
		State:      state,
	}, nil
}

// InitProject initializes .agent8 under start.
func InitProject(start string, cfg ProjectConfig) (ProjectContext, error) {
	root, err := resolveStartDir(start)
	if err != nil {
		return ProjectContext{}, err
	}
	projectDir, configPath, statePath := projectPaths(root)
	if err := os.MkdirAll(filepath.Join(projectDir, projectProfilesDir), 0o755); err != nil {
		return ProjectContext{}, err
	}
	norm := normalizeProjectConfig(cfg, root)
	if err := writeProjectConfig(configPath, norm); err != nil {
		return ProjectContext{}, err
	}
	initialState := ProjectState{
		LastCommand: "init",
	}
	if err := writeProjectState(statePath, initialState); err != nil {
		return ProjectContext{}, err
	}
	readmePath := filepath.Join(projectDir, projectReadmeFilename)
	if _, err := os.Stat(readmePath); os.IsNotExist(err) {
		_ = os.WriteFile(readmePath, []byte(strings.TrimSpace(defaultProjectReadme())+"\n"), 0o644)
	}
	return LoadProjectContext(root)
}

// SetActiveSession updates .agent8/state.json affinity values.
func SetActiveSession(start string, state ProjectState) (ProjectContext, error) {
	ctx, err := LoadProjectContext(start)
	if err != nil {
		return ProjectContext{}, err
	}
	if !ctx.Exists {
		return ProjectContext{}, fmt.Errorf("%s not initialized in this project", ProjectDirName)
	}
	state.ActiveSessionID = strings.TrimSpace(state.ActiveSessionID)
	state.ActiveTeamID = strings.TrimSpace(state.ActiveTeamID)
	state.ActiveRunID = strings.TrimSpace(state.ActiveRunID)
	state.LastCommand = strings.TrimSpace(state.LastCommand)
	if state.LastAttachedAt == "" {
		state.LastAttachedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	if err := writeProjectState(ctx.StatePath, state); err != nil {
		return ProjectContext{}, err
	}
	return LoadProjectContext(ctx.RootDir)
}

func readProjectConfig(path string, root string) (ProjectConfig, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return defaultProjectConfig(root), nil
		}
		return ProjectConfig{}, err
	}
	cfg := defaultProjectConfig(root)
	lines := strings.Split(string(b), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if i := strings.Index(line, "#"); i >= 0 {
			line = strings.TrimSpace(line[:i])
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		raw := strings.TrimSpace(parts[1])
		value := raw
		if strings.HasPrefix(raw, "\"") && strings.HasSuffix(raw, "\"") {
			if unquoted, err := strconv.Unquote(raw); err == nil {
				value = unquoted
			}
		}
		switch key {
		case "project_id":
			cfg.ProjectID = strings.TrimSpace(value)
		case "default_profile":
			cfg.DefaultProfile = strings.TrimSpace(value)
		case "default_mode":
			cfg.DefaultMode = strings.TrimSpace(value)
		case "default_team_profile":
			cfg.DefaultTeamProfile = strings.TrimSpace(value)
		case "rpc_endpoint":
			cfg.RPCEndpoint = strings.TrimSpace(value)
		case "data_dir_override":
			cfg.DataDirOverride = strings.TrimSpace(value)
		case "created_at":
			cfg.CreatedAt = strings.TrimSpace(value)
		case "version":
			if n, err := strconv.Atoi(strings.TrimSpace(value)); err == nil {
				cfg.Version = n
			}
		}
	}
	return normalizeProjectConfig(cfg, root), nil
}

func writeProjectConfig(path string, cfg ProjectConfig) error {
	cfg = normalizeProjectConfig(cfg, filepath.Dir(filepath.Dir(path)))
	lines := []string{
		"# Agen8 project defaults",
		"project_id = " + strconv.Quote(cfg.ProjectID),
		"default_profile = " + strconv.Quote(cfg.DefaultProfile),
		"default_mode = " + strconv.Quote(cfg.DefaultMode),
		"default_team_profile = " + strconv.Quote(cfg.DefaultTeamProfile),
		"rpc_endpoint = " + strconv.Quote(cfg.RPCEndpoint),
		"data_dir_override = " + strconv.Quote(cfg.DataDirOverride),
		"created_at = " + strconv.Quote(cfg.CreatedAt),
		"version = " + strconv.Itoa(cfg.Version),
	}
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644)
}

func readProjectState(path string) (ProjectState, error) {
	var state ProjectState
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return state, nil
		}
		return state, err
	}
	if len(strings.TrimSpace(string(b))) == 0 {
		return state, nil
	}
	if err := json.Unmarshal(b, &state); err != nil {
		return ProjectState{}, err
	}
	return state, nil
}

func writeProjectState(path string, state ProjectState) error {
	b, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}

func defaultProjectReadme() string {
	return `# .agent8

This directory stores project-local Agen8 defaults.

- config.toml: project defaults and optional overrides.
- state.json: active session/team/run pointer for this project.
- profiles/: optional project-local profiles.

Precedence:
1) CLI flags
2) environment variables
3) .agent8/config.toml
4) global defaults`
}
