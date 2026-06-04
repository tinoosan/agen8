package claudecli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestSetupProjectWritesMCPAndHookConfig(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.WriteFile(filepath.Join(root, ".mcp.json"), []byte(`{"mcpServers":{"docs":{"command":"docs-server"}},"other":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := SetupProject(SetupOptions{
		ProjectRoot: root,
		MCPURL:      "http://127.0.0.1:7777/mcp?token=abc",
		HookCommand: "/usr/local/bin/agen8-mcp-server",
	})
	if err != nil {
		t.Fatalf("SetupProject: %v", err)
	}
	if result.MCPConfigPath != filepath.Join(root, ".mcp.json") {
		t.Fatalf("mcp path=%q", result.MCPConfigPath)
	}
	if result.SettingsPath != filepath.Join(root, ".claude", "settings.local.json") {
		t.Fatalf("settings path=%q", result.SettingsPath)
	}
	if result.PluginPath != filepath.Join(home, ".claude", "skills", "agen8") {
		t.Fatalf("plugin path=%q", result.PluginPath)
	}
	if result.PluginRef != "plugin:agen8@skills-dir" {
		t.Fatalf("plugin ref=%q", result.PluginRef)
	}
	if !result.ChannelReady {
		t.Fatalf("channel should be marked ready after channel server implementation")
	}

	var mcpCfg struct {
		MCPServers map[string]struct {
			Type       string   `json:"type"`
			URL        string   `json:"url"`
			AlwaysLoad bool     `json:"alwaysLoad"`
			Command    string   `json:"command"`
			Args       []string `json:"args"`
		} `json:"mcpServers"`
		Other bool `json:"other"`
	}
	raw, err := os.ReadFile(filepath.Join(root, ".mcp.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &mcpCfg); err != nil {
		t.Fatalf("unmarshal mcp config: %v", err)
	}
	if !mcpCfg.Other {
		t.Fatalf("top-level keys were not preserved")
	}
	if mcpCfg.MCPServers["docs"].Command != "docs-server" {
		t.Fatalf("existing MCP server not preserved: %#v", mcpCfg.MCPServers["docs"])
	}
	if mcpCfg.MCPServers["agen8"].Type != "http" || mcpCfg.MCPServers["agen8"].URL != "http://127.0.0.1:7777/mcp?token=abc" || !mcpCfg.MCPServers["agen8"].AlwaysLoad {
		t.Fatalf("agen8 server=%#v", mcpCfg.MCPServers["agen8"])
	}
	if mcpCfg.MCPServers["agen8-channel"].Command != "/usr/local/bin/agen8-mcp-server" || !equalStrings(mcpCfg.MCPServers["agen8-channel"].Args, []string{"claude", "channel"}) {
		t.Fatalf("agen8-channel server=%#v", mcpCfg.MCPServers["agen8-channel"])
	}
	var plugin struct {
		Name       string `json:"name"`
		MCPServers string `json:"mcpServers"`
		Channels   []struct {
			Server string `json:"server"`
		} `json:"channels"`
	}
	raw, err = os.ReadFile(filepath.Join(home, ".claude", "skills", "agen8", ".claude-plugin", "plugin.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &plugin); err != nil {
		t.Fatalf("unmarshal plugin: %v", err)
	}
	if plugin.Name != "agen8" || plugin.MCPServers != "./mcp-config.json" || len(plugin.Channels) != 1 || plugin.Channels[0].Server != "agen8-channel" {
		t.Fatalf("plugin=%#v", plugin)
	}
	var pluginMCP struct {
		MCPServers map[string]struct {
			Command string   `json:"command"`
			Args    []string `json:"args"`
		} `json:"mcpServers"`
	}
	raw, err = os.ReadFile(filepath.Join(home, ".claude", "skills", "agen8", "mcp-config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &pluginMCP); err != nil {
		t.Fatalf("unmarshal plugin mcp: %v", err)
	}
	if pluginMCP.MCPServers["agen8-channel"].Command != "/usr/local/bin/agen8-mcp-server" || !equalStrings(pluginMCP.MCPServers["agen8-channel"].Args, []string{"claude", "channel"}) {
		t.Fatalf("plugin mcp=%#v", pluginMCP.MCPServers["agen8-channel"])
	}

	settings := readSettingsFile(t, filepath.Join(root, ".claude", "settings.local.json"))
	groups := settings.Hooks["SessionStart"]
	if len(groups) != 1 || groups[0].Matcher != "*" || len(groups[0].Hooks) != 1 {
		t.Fatalf("SessionStart groups=%#v", groups)
	}
	hook := groups[0].Hooks[0]
	if hook.Type != "command" || hook.Command != "/usr/local/bin/agen8-mcp-server" || !equalStrings(hook.Args, []string{"claude", "hook"}) {
		t.Fatalf("hook=%#v", hook)
	}
}

func TestSetupProjectIsIdempotent(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	opts := SetupOptions{
		ProjectRoot: root,
		MCPURL:      "http://127.0.0.1:7777/mcp?token=abc",
		HookCommand: "/bin/agen8",
		HookArgs:    []string{"claude", "hook"},
	}
	if _, err := SetupProject(opts); err != nil {
		t.Fatalf("first setup: %v", err)
	}
	if _, err := SetupProject(opts); err != nil {
		t.Fatalf("second setup: %v", err)
	}
	settings := readSettingsFile(t, filepath.Join(root, ".claude", "settings.local.json"))
	groups := settings.Hooks["SessionStart"]
	if len(groups) != 1 || len(groups[0].Hooks) != 1 {
		t.Fatalf("expected one idempotent hook, got %#v", groups)
	}
}

func TestSetupProjectUsesExistingMCPURL(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	if err := os.WriteFile(filepath.Join(root, ".mcp.json"), []byte(`{"mcpServers":{"agen8":{"type":"http","url":"http://127.0.0.1:8888/mcp?token=existing"}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := SetupProject(SetupOptions{ProjectRoot: root, HookCommand: "/bin/agen8"})
	if err != nil {
		t.Fatalf("SetupProject: %v", err)
	}
	if result.MCPURL != "http://127.0.0.1:8888/mcp?token=existing" {
		t.Fatalf("mcp url=%q", result.MCPURL)
	}
}

func readSettingsFile(t *testing.T, path string) claudeSettings {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var settings claudeSettings
	if err := json.Unmarshal(raw, &settings); err != nil {
		t.Fatalf("unmarshal settings: %v", err)
	}
	return settings
}
