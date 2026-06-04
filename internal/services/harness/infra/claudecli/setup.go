package claudecli

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

const defaultLocalMCPURL = "http://127.0.0.1:7777/mcp?token=agen8-local"

type SetupOptions struct {
	ProjectRoot    string
	MCPURL         string
	HookCommand    string
	HookArgs       []string
	ChannelCommand string
	ChannelArgs    []string
}

type SetupResult struct {
	ProjectRoot       string   `json:"projectRoot"`
	MCPConfigPath     string   `json:"mcpConfigPath"`
	SettingsPath      string   `json:"settingsPath"`
	MCPURL            string   `json:"mcpUrl"`
	HookCommand       string   `json:"hookCommand"`
	HookArgs          []string `json:"hookArgs"`
	ChannelCommand    string   `json:"channelCommand"`
	ChannelArgs       []string `json:"channelArgs"`
	ChannelReady      bool     `json:"channelReady"`
	ChannelStatus     string   `json:"channelStatus"`
	ClaudeLaunchHints []string `json:"claudeLaunchHints"`
}

func SetupProject(opts SetupOptions) (SetupResult, error) {
	projectRoot, err := resolveProjectRoot(opts.ProjectRoot)
	if err != nil {
		return SetupResult{}, err
	}
	hookCommand := strings.TrimSpace(opts.HookCommand)
	if hookCommand == "" {
		return SetupResult{}, fmt.Errorf("hook command is required")
	}
	hookArgs := compactHookArgs(opts.HookArgs)
	if len(hookArgs) == 0 {
		hookArgs = []string{"claude", "hook"}
	}
	channelCommand := strings.TrimSpace(opts.ChannelCommand)
	if channelCommand == "" {
		channelCommand = hookCommand
	}
	channelArgs := compactHookArgs(opts.ChannelArgs)
	if len(channelArgs) == 0 {
		channelArgs = []string{"claude", "channel"}
	}
	rawURL, err := resolveSetupMCPURL(projectRoot, opts.MCPURL)
	if err != nil {
		return SetupResult{}, err
	}
	if err := writeClaudeMCPProjectConfig(projectRoot, rawURL, channelCommand, channelArgs); err != nil {
		return SetupResult{}, err
	}
	if err := writeClaudeHookSettings(projectRoot, hookCommand, hookArgs); err != nil {
		return SetupResult{}, err
	}
	return SetupResult{
		ProjectRoot:    projectRoot,
		MCPConfigPath:  filepath.Join(projectRoot, ".mcp.json"),
		SettingsPath:   filepath.Join(projectRoot, ".claude", "settings.local.json"),
		MCPURL:         rawURL,
		HookCommand:    hookCommand,
		HookArgs:       hookArgs,
		ChannelCommand: channelCommand,
		ChannelArgs:    channelArgs,
		ChannelReady:   true,
		ChannelStatus:  "Claude Code channel adapter installed as mcpServers.agen8-channel. Claude Code must be launched with channel support enabled during the research preview.",
		ClaudeLaunchHints: []string{
			"Restart Claude Code in this project so it reloads .mcp.json and .claude/settings.local.json.",
			"During the Claude Code channel research preview, launch Claude Code with the Agen8 channel enabled, for example: claude --dangerously-load-development-channels server:agen8-channel.",
		},
	}, nil
}

func resolveProjectRoot(raw string) (string, error) {
	root := strings.TrimSpace(raw)
	if root == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		root = cwd
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("stat project root: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("project root must be a directory")
	}
	return abs, nil
}

func resolveSetupMCPURL(projectRoot string, raw string) (string, error) {
	if strings.TrimSpace(raw) != "" {
		return normalizeMCPURL(raw)
	}
	if raw := strings.TrimSpace(os.Getenv(EnvAgen8MCPURL)); raw != "" {
		return normalizeMCPURL(raw)
	}
	if token := strings.TrimSpace(os.Getenv(EnvAgen8Token)); token != "" {
		return normalizeMCPURL("http://127.0.0.1:7777/mcp?token=" + url.QueryEscape(token))
	}
	if raw, ok := readExistingProjectMCPURL(projectRoot); ok {
		return normalizeMCPURL(raw)
	}
	return normalizeMCPURL(defaultLocalMCPURL)
}

func readExistingProjectMCPURL(projectRoot string) (string, bool) {
	raw, err := os.ReadFile(filepath.Join(projectRoot, ".mcp.json"))
	if err != nil {
		return "", false
	}
	return agen8URLFromMCPConfig(raw)
}

type claudeMCPProjectConfig struct {
	MCPServers map[string]map[string]any  `json:"mcpServers,omitempty"`
	Extra      map[string]json.RawMessage `json:"-"`
}

func writeClaudeMCPProjectConfig(projectRoot string, rawURL string, channelCommand string, channelArgs []string) error {
	path := filepath.Join(projectRoot, ".mcp.json")
	cfg, err := readClaudeMCPProjectConfig(path)
	if err != nil {
		return err
	}
	if cfg.MCPServers == nil {
		cfg.MCPServers = map[string]map[string]any{}
	}
	cfg.MCPServers["agen8"] = map[string]any{
		"type": "http",
		"url":  rawURL,
	}
	cfg.MCPServers["agen8-channel"] = map[string]any{
		"command": channelCommand,
		"args":    channelArgs,
	}
	return writeJSONFile(path, cfg.toMap(), 0o600)
}

func readClaudeMCPProjectConfig(path string) (claudeMCPProjectConfig, error) {
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return claudeMCPProjectConfig{Extra: map[string]json.RawMessage{}}, nil
	}
	if err != nil {
		return claudeMCPProjectConfig{}, fmt.Errorf("read %s: %w", path, err)
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(raw, &top); err != nil {
		return claudeMCPProjectConfig{}, fmt.Errorf("parse %s: %w", path, err)
	}
	cfg := claudeMCPProjectConfig{Extra: map[string]json.RawMessage{}}
	for key, value := range top {
		if key == "mcpServers" {
			if err := json.Unmarshal(value, &cfg.MCPServers); err != nil {
				return claudeMCPProjectConfig{}, fmt.Errorf("parse %s mcpServers: %w", path, err)
			}
			continue
		}
		cfg.Extra[key] = value
	}
	return cfg, nil
}

func (c claudeMCPProjectConfig) toMap() map[string]any {
	out := map[string]any{}
	for key, value := range c.Extra {
		out[key] = value
	}
	out["mcpServers"] = c.MCPServers
	return out
}

type claudeSettings struct {
	Hooks map[string][]claudeHookGroup `json:"hooks,omitempty"`
	Extra map[string]json.RawMessage   `json:"-"`
}

type claudeHookGroup struct {
	Matcher string              `json:"matcher,omitempty"`
	Hooks   []claudeHookCommand `json:"hooks"`
}

type claudeHookCommand struct {
	Type    string   `json:"type"`
	Command string   `json:"command"`
	Args    []string `json:"args,omitempty"`
}

func writeClaudeHookSettings(projectRoot string, command string, args []string) error {
	path := filepath.Join(projectRoot, ".claude", "settings.local.json")
	settings, err := readClaudeSettings(path)
	if err != nil {
		return err
	}
	if settings.Hooks == nil {
		settings.Hooks = map[string][]claudeHookGroup{}
	}
	settings.Hooks["SessionStart"] = upsertSessionStartHook(settings.Hooks["SessionStart"], claudeHookCommand{
		Type:    "command",
		Command: command,
		Args:    args,
	})
	return writeJSONFile(path, settings.toMap(), 0o600)
}

func readClaudeSettings(path string) (claudeSettings, error) {
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return claudeSettings{Extra: map[string]json.RawMessage{}}, nil
	}
	if err != nil {
		return claudeSettings{}, fmt.Errorf("read %s: %w", path, err)
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(raw, &top); err != nil {
		return claudeSettings{}, fmt.Errorf("parse %s: %w", path, err)
	}
	settings := claudeSettings{Extra: map[string]json.RawMessage{}}
	for key, value := range top {
		if key == "hooks" {
			if err := json.Unmarshal(value, &settings.Hooks); err != nil {
				return claudeSettings{}, fmt.Errorf("parse %s hooks: %w", path, err)
			}
			continue
		}
		settings.Extra[key] = value
	}
	return settings, nil
}

func (s claudeSettings) toMap() map[string]any {
	out := map[string]any{}
	for key, value := range s.Extra {
		out[key] = value
	}
	out["hooks"] = s.Hooks
	return out
}

func upsertSessionStartHook(groups []claudeHookGroup, hook claudeHookCommand) []claudeHookGroup {
	for i := range groups {
		groups[i].Hooks = removeMatchingHook(groups[i].Hooks, hook)
	}
	for i := range groups {
		if strings.TrimSpace(groups[i].Matcher) == "*" {
			groups[i].Hooks = append(groups[i].Hooks, hook)
			return groups
		}
	}
	return append(groups, claudeHookGroup{Matcher: "*", Hooks: []claudeHookCommand{hook}})
}

func removeMatchingHook(hooks []claudeHookCommand, hook claudeHookCommand) []claudeHookCommand {
	out := hooks[:0]
	for _, existing := range hooks {
		if strings.TrimSpace(existing.Type) == strings.TrimSpace(hook.Type) &&
			strings.TrimSpace(existing.Command) == strings.TrimSpace(hook.Command) &&
			equalStrings(existing.Args, hook.Args) {
			continue
		}
		out = append(out, existing)
	}
	return out
}

func compactHookArgs(args []string) []string {
	var out []string
	for _, arg := range args {
		if strings.TrimSpace(arg) != "" {
			out = append(out, strings.TrimSpace(arg))
		}
	}
	return out
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func writeJSONFile(path string, value any, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create %s parent: %w", path, err)
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal %s: %w", path, err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, perm); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
