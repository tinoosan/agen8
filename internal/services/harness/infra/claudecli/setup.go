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
	PluginPath        string   `json:"pluginPath"`
	PluginRef         string   `json:"pluginRef"`
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
	pluginPath, err := writeClaudeAgen8Plugin(channelCommand, channelArgs)
	if err != nil {
		return SetupResult{}, err
	}
	if err := writeClaudeHookSettings(projectRoot, hookCommand, hookArgs); err != nil {
		return SetupResult{}, err
	}
	return SetupResult{
		ProjectRoot:    projectRoot,
		MCPConfigPath:  filepath.Join(projectRoot, ".mcp.json"),
		SettingsPath:   filepath.Join(projectRoot, ".claude", "settings.local.json"),
		PluginPath:     pluginPath,
		PluginRef:      defaultClaudeChannelPluginRef,
		MCPURL:         rawURL,
		HookCommand:    hookCommand,
		HookArgs:       hookArgs,
		ChannelCommand: channelCommand,
		ChannelArgs:    channelArgs,
		ChannelReady:   true,
		ChannelStatus:  "Claude Code Agen8 channel adapter installed. Agen8 launches remote-control sessions with the local channel enabled so Claude desktop can attach to the running session and receive Agen8 messages.",
		ClaudeLaunchHints: []string{
			"Restart Claude Code in this project so it reloads .mcp.json and .claude/settings.local.json.",
			"Launch the desktop-visible remote-control path with: agen8-mcp-server claude launch --project-root " + shellQuote(projectRoot),
			"Equivalent current Claude Code command: claude --remote-control \"Agen8: " + filepath.Base(projectRoot) + "\" --dangerously-load-development-channels server:agen8-channel.",
			"When Agen8 is installed from an approved Claude marketplace or org allowlist, use: agen8-mcp-server claude launch --development-channel=false --channel plugin:agen8@skills-dir.",
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
		"type":       "http",
		"url":        rawURL,
		"alwaysLoad": true,
	}
	cfg.MCPServers["agen8-channel"] = map[string]any{
		"command": channelCommand,
		"args":    channelArgs,
	}
	return writeJSONFile(path, cfg.toMap(), 0o600)
}

func writeClaudeAgen8Plugin(channelCommand string, channelArgs []string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve Claude user skills directory: %w", err)
	}
	pluginRoot := filepath.Join(home, ".claude", "skills", "agen8")
	manifest := map[string]any{
		"name":        "agen8",
		"displayName": "Agen8",
		"version":     "0.1.0",
		"description": "Agen8 work-context and coordination channel for Claude Code.",
		"author": map[string]any{
			"name": "Agen8",
		},
		"mcpServers": "./mcp-config.json",
		"channels": []map[string]any{
			{"server": "agen8-channel"},
		},
	}
	if err := writeJSONFile(filepath.Join(pluginRoot, ".claude-plugin", "plugin.json"), manifest, 0o600); err != nil {
		return "", err
	}
	mcpConfig := map[string]any{
		"mcpServers": map[string]any{
			"agen8-channel": map[string]any{
				"command": channelCommand,
				"args":    channelArgs,
			},
		},
	}
	if err := writeJSONFile(filepath.Join(pluginRoot, "mcp-config.json"), mcpConfig, 0o600); err != nil {
		return "", err
	}
	skill := strings.Join([]string{
		"---",
		"name: agen8",
		"description: Use Agen8 for durable work context, task coordination, and channel-delivered messages.",
		"---",
		"",
		"# Agen8",
		"",
		"Agen8 is the durable work-context layer for this project. When channel messages arrive, treat them as coordination events for this Claude Code session and respond through the Agen8 channel message tool when appropriate.",
		"",
	}, "\n")
	if err := os.WriteFile(filepath.Join(pluginRoot, "SKILL.md"), []byte(skill), 0o600); err != nil {
		return "", fmt.Errorf("write %s: %w", filepath.Join(pluginRoot, "SKILL.md"), err)
	}
	return pluginRoot, nil
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
	Type    string         `json:"type"`
	Command string         `json:"command,omitempty"`
	Args    []string       `json:"args,omitempty"`
	Server  string         `json:"server,omitempty"`
	Tool    string         `json:"tool,omitempty"`
	Input   map[string]any `json:"input,omitempty"`
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
	settings.Hooks["SessionStart"] = removeClaudeMCPToolHooks(settings.Hooks["SessionStart"])
	return writeJSONFile(path, settings.toMap(), 0o600)
}

func removeClaudeMCPToolSessionStartHook(projectRoot string) error {
	path := filepath.Join(projectRoot, ".claude", "settings.local.json")
	settings, err := readClaudeSettings(path)
	if err != nil {
		return err
	}
	if settings.Hooks == nil {
		return nil
	}
	settings.Hooks["SessionStart"] = removeClaudeMCPToolHooks(settings.Hooks["SessionStart"])
	return writeJSONFile(path, settings.toMap(), 0o600)
}

func removeClaudeMCPToolHooks(groups []claudeHookGroup) []claudeHookGroup {
	for i := range groups {
		out := groups[i].Hooks[:0]
		for _, hook := range groups[i].Hooks {
			if hook.Type == "mcp_tool" && hook.Server == "agen8" && hook.Tool == "space" {
				continue
			}
			out = append(out, hook)
		}
		groups[i].Hooks = out
	}
	return groups
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
		if sameClaudeHook(existing, hook) {
			continue
		}
		out = append(out, existing)
	}
	return out
}

func sameClaudeHook(a claudeHookCommand, b claudeHookCommand) bool {
	if strings.TrimSpace(a.Type) != strings.TrimSpace(b.Type) {
		return false
	}
	switch strings.TrimSpace(a.Type) {
	case "command":
		return strings.TrimSpace(a.Command) == strings.TrimSpace(b.Command) && equalStrings(a.Args, b.Args)
	case "mcp_tool":
		return strings.TrimSpace(a.Server) == strings.TrimSpace(b.Server) && strings.TrimSpace(a.Tool) == strings.TrimSpace(b.Tool)
	default:
		return strings.TrimSpace(a.Command) == strings.TrimSpace(b.Command) &&
			strings.TrimSpace(a.Server) == strings.TrimSpace(b.Server) &&
			strings.TrimSpace(a.Tool) == strings.TrimSpace(b.Tool) &&
			equalStrings(a.Args, b.Args)
	}
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
