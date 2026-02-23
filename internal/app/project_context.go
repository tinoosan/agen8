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
	ProjectDirName        = ".agen8"
	LegacyProjectDirName  = ".agent8"
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
	DefaultMode        string // single-agent|multi-agent
	DefaultTeamProfile string
	RPCEndpoint        string
	DataDirOverride    string
	ObsidianVaultPath  string
	ObsidianEnabled    bool
	CreatedAt          string
	Version            int
}

// ProjectState stores mutable project affinity pointers.
type ProjectState struct {
	ActiveSessionID string `json:"active_session_id,omitempty"`
	ActiveTeamID    string `json:"active_team_id,omitempty"`
	ActiveRunID     string `json:"active_run_id,omitempty"`
	ActiveThreadID  string `json:"active_thread_id,omitempty"`
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
		DefaultMode: "single-agent",
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
	out.ObsidianVaultPath = strings.TrimSpace(out.ObsidianVaultPath)
	out.CreatedAt = strings.TrimSpace(out.CreatedAt)
	if out.CreatedAt == "" {
		out.CreatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	if !out.ObsidianEnabled && out.ObsidianVaultPath != "" {
		out.ObsidianEnabled = true
	}
	mode := strings.ToLower(strings.TrimSpace(out.DefaultMode))
	switch mode {
	case "team", "multi-agent":
		out.DefaultMode = "multi-agent"
	case "standalone", "single-agent":
		out.DefaultMode = "single-agent"
	default:
		out.DefaultMode = "single-agent"
	}
	return out
}

func mergeProjectConfig(base ProjectConfig, override ProjectConfig) ProjectConfig {
	out := base
	if v := strings.TrimSpace(override.ProjectID); v != "" {
		out.ProjectID = v
	}
	if v := strings.TrimSpace(override.DefaultProfile); v != "" {
		out.DefaultProfile = v
	}
	if v := strings.TrimSpace(override.DefaultMode); v != "" {
		out.DefaultMode = v
	}
	if v := strings.TrimSpace(override.DefaultTeamProfile); v != "" {
		out.DefaultTeamProfile = v
	}
	if v := strings.TrimSpace(override.RPCEndpoint); v != "" {
		out.RPCEndpoint = v
	}
	if v := strings.TrimSpace(override.DataDirOverride); v != "" {
		out.DataDirOverride = v
	}
	if v := strings.TrimSpace(override.ObsidianVaultPath); v != "" {
		out.ObsidianVaultPath = v
	}
	if override.ObsidianEnabled {
		out.ObsidianEnabled = true
	}
	if v := strings.TrimSpace(override.CreatedAt); v != "" {
		out.CreatedAt = v
	}
	if override.Version > 0 {
		out.Version = override.Version
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

// FindProjectRoot walks up from start to locate a directory that contains .agen8
// (preferred) or legacy .agent8.
func FindProjectRoot(start string) (string, bool, error) {
	dir, err := resolveStartDir(start)
	if err != nil {
		return "", false, err
	}
	for {
		if info, err := os.Stat(filepath.Join(dir, ProjectDirName)); err == nil && info.IsDir() {
			return dir, true, nil
		}
		if info, err := os.Stat(filepath.Join(dir, LegacyProjectDirName)); err == nil && info.IsDir() {
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
	projectDir = resolveProjectDir(root)
	configPath = filepath.Join(projectDir, projectConfigFilename)
	statePath = filepath.Join(projectDir, projectStateFilename)
	return projectDir, configPath, statePath
}

func resolveProjectDir(root string) string {
	newDir := filepath.Join(root, ProjectDirName)
	if info, err := os.Stat(newDir); err == nil && info.IsDir() {
		return newDir
	}
	legacyDir := filepath.Join(root, LegacyProjectDirName)
	if info, err := os.Stat(legacyDir); err == nil && info.IsDir() {
		return legacyDir
	}
	return newDir
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

// InitProject initializes .agen8 under start.
func InitProject(start string, cfg ProjectConfig) (ProjectContext, error) {
	root, err := resolveStartDir(start)
	if err != nil {
		return ProjectContext{}, err
	}
	projectDir := filepath.Join(root, ProjectDirName)
	configPath := filepath.Join(projectDir, projectConfigFilename)
	statePath := filepath.Join(projectDir, projectStateFilename)
	if err := migrateLegacyProjectDir(root); err != nil {
		return ProjectContext{}, err
	}
	if err := os.MkdirAll(filepath.Join(projectDir, projectProfilesDir), 0o755); err != nil {
		return ProjectContext{}, err
	}
	baseCfg, _ := readProjectConfig(configPath, root)
	norm := normalizeProjectConfig(mergeProjectConfig(baseCfg, cfg), root)
	if err := writeProjectConfig(configPath, norm); err != nil {
		return ProjectContext{}, err
	}
	initialState, _ := readProjectState(statePath)
	initialState.LastCommand = "init"
	if err := writeProjectState(statePath, initialState); err != nil {
		return ProjectContext{}, err
	}
	readmePath := filepath.Join(projectDir, projectReadmeFilename)
	if _, err := os.Stat(readmePath); os.IsNotExist(err) {
		_ = os.WriteFile(readmePath, []byte(strings.TrimSpace(defaultProjectReadme())+"\n"), 0o644)
	}
	return LoadProjectContext(root)
}

// SetActiveSession updates .agen8/state.json affinity values.
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
	state.ActiveThreadID = strings.TrimSpace(state.ActiveThreadID)
	if state.ActiveThreadID == "" {
		state.ActiveThreadID = state.ActiveSessionID
	}
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
	inProjectSection := false
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section := strings.ToLower(strings.TrimSpace(strings.Trim(line, "[]")))
			inProjectSection = section == "project"
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
		if strings.HasPrefix(key, "project.") {
			key = strings.TrimSpace(strings.TrimPrefix(key, "project."))
		}
		if !inProjectSection {
			switch key {
			case "project_id", "default_profile", "default_mode", "default_team_profile", "rpc_endpoint", "data_dir_override", "created_at", "obsidian_vault_path", "obsidian_enabled", "version":
			default:
				continue
			}
		}
		raw := strings.TrimSpace(parts[1])
		value := raw
		if strings.HasPrefix(raw, "\"") && strings.HasSuffix(raw, "\"") {
			if unquoted, err := strconv.Unquote(raw); err == nil {
				value = unquoted
			}
		}
		switch key {
		case "project_id", "id":
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
		case "obsidian_vault_path":
			cfg.ObsidianVaultPath = strings.TrimSpace(value)
		case "obsidian_enabled":
			cfg.ObsidianEnabled = strings.EqualFold(strings.TrimSpace(value), "true")
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
		"[project]",
		"id = " + strconv.Quote(cfg.ProjectID),
		"default_profile = " + strconv.Quote(cfg.DefaultProfile),
		"default_mode = " + strconv.Quote(cfg.DefaultMode),
		"default_team_profile = " + strconv.Quote(cfg.DefaultTeamProfile),
		"rpc_endpoint = " + strconv.Quote(cfg.RPCEndpoint),
		"data_dir_override = " + strconv.Quote(cfg.DataDirOverride),
		"obsidian_vault_path = " + strconv.Quote(cfg.ObsidianVaultPath),
		"obsidian_enabled = " + strconv.FormatBool(cfg.ObsidianEnabled),
		"created_at = " + strconv.Quote(cfg.CreatedAt),
		"version = " + strconv.Itoa(cfg.Version),
	}
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644)
}

func SaveProjectConfig(start string, cfg ProjectConfig) (ProjectContext, error) {
	ctx, err := LoadProjectContext(start)
	if err != nil {
		return ProjectContext{}, err
	}
	if !ctx.Exists {
		initCtx, ierr := InitProject(start, cfg)
		if ierr != nil {
			return ProjectContext{}, ierr
		}
		return initCtx, nil
	}
	norm := normalizeProjectConfig(cfg, ctx.RootDir)
	if err := writeProjectConfig(ctx.ConfigPath, norm); err != nil {
		return ProjectContext{}, err
	}
	return LoadProjectContext(ctx.RootDir)
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
	return `# .agen8

This directory stores project-local Agen8 defaults.

- config.toml: project defaults and optional overrides.
- state.json: active session/team/run pointer for this project.
- profiles/: optional project-local profiles.

Precedence:
1) CLI flags
2) environment variables
3) .agen8/config.toml
4) global defaults`
}

func migrateLegacyProjectDir(root string) error {
	newDir := filepath.Join(root, ProjectDirName)
	if info, err := os.Stat(newDir); err == nil && info.IsDir() {
		return nil
	}
	legacyDir := filepath.Join(root, LegacyProjectDirName)
	info, err := os.Stat(legacyDir)
	if err != nil || !info.IsDir() {
		return nil
	}
	if err := os.MkdirAll(newDir, 0o755); err != nil {
		return err
	}
	// Best-effort migration for key files and profile overrides.
	for _, name := range []string{projectConfigFilename, projectStateFilename, projectReadmeFilename} {
		src := filepath.Join(legacyDir, name)
		dst := filepath.Join(newDir, name)
		if _, err := os.Stat(src); err != nil {
			continue
		}
		if _, err := os.Stat(dst); err == nil {
			continue
		}
		b, err := os.ReadFile(src)
		if err != nil {
			return err
		}
		if err := os.WriteFile(dst, b, 0o644); err != nil {
			return err
		}
	}
	srcProfiles := filepath.Join(legacyDir, projectProfilesDir)
	if srcInfo, err := os.Stat(srcProfiles); err == nil && srcInfo.IsDir() {
		dstProfiles := filepath.Join(newDir, projectProfilesDir)
		if err := os.MkdirAll(dstProfiles, 0o755); err != nil {
			return err
		}
		entries, err := os.ReadDir(srcProfiles)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			src := filepath.Join(srcProfiles, entry.Name())
			dst := filepath.Join(dstProfiles, entry.Name())
			if _, err := os.Stat(dst); err == nil {
				continue
			}
			b, err := os.ReadFile(src)
			if err != nil {
				return err
			}
			if err := os.WriteFile(dst, b, 0o644); err != nil {
				return err
			}
		}
	}
	marker := filepath.Join(newDir, "MIGRATED_FROM_AGENT8")
	if _, err := os.Stat(marker); os.IsNotExist(err) {
		_ = os.WriteFile(marker, []byte(legacyDir+"\n"), 0o644)
	}
	return nil
}
