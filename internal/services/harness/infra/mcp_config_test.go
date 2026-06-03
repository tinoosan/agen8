package infra

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	harnessapp "github.com/tinoosan/agen8-mcp-server/internal/services/harness/app"
)

func TestMCPConfigFormatterFormatsCodexOverride(t *testing.T) {
	servers, err := (MCPConfigFormatter{DataDir: t.TempDir()}).FormatMCPServers(
		context.Background(),
		harnessapp.MCPConfigRequest{
			HarnessKind: "codex",
			RawURL:      "http://127.0.0.1:7777/mcp?token=abc",
		},
	)
	if err != nil {
		t.Fatalf("FormatMCPServers: %v", err)
	}
	if len(servers) != 1 || servers[0] != `mcp_servers.agen8.url="http://127.0.0.1:7777/mcp?token=abc"` {
		t.Fatalf("servers=%v", servers)
	}
}

func TestMCPConfigFormatterWritesClaudeConfigFile(t *testing.T) {
	servers, err := (MCPConfigFormatter{DataDir: t.TempDir()}).FormatMCPServers(
		context.Background(),
		harnessapp.MCPConfigRequest{
			HarnessKind: "claude-cli",
			RawURL:      "http://127.0.0.1:7777/mcp?token=abc",
		},
	)
	if err != nil {
		t.Fatalf("FormatMCPServers: %v", err)
	}
	if len(servers) != 1 {
		t.Fatalf("servers len=%d want 1", len(servers))
	}
	raw, err := os.ReadFile(servers[0])
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	var cfg struct {
		MCPServers map[string]struct {
			Type string `json:"type"`
			URL  string `json:"url"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("unmarshal config: %v", err)
	}
	if cfg.MCPServers["agen8"].Type != "http" || cfg.MCPServers["agen8"].URL != "http://127.0.0.1:7777/mcp?token=abc" {
		t.Fatalf("config=%+v", cfg.MCPServers["agen8"])
	}
}

func TestMCPConfigFormatterBuildsURLFromBaseAndToken(t *testing.T) {
	servers, err := (MCPConfigFormatter{DataDir: t.TempDir()}).FormatMCPServers(
		context.Background(),
		harnessapp.MCPConfigRequest{
			HarnessKind: "codex",
			BaseURL:     "http://127.0.0.1:38123",
			Token:       "abc",
		},
	)
	if err != nil {
		t.Fatalf("FormatMCPServers: %v", err)
	}
	if len(servers) != 1 || servers[0] != `mcp_servers.agen8.url="http://127.0.0.1:38123/mcp?token=abc"` {
		t.Fatalf("servers=%v", servers)
	}
}

func TestMCPConfigFormatterReturnsClaudeJSONForRemoteBaseURL(t *testing.T) {
	servers, err := (MCPConfigFormatter{DataDir: t.TempDir()}).FormatMCPServers(
		context.Background(),
		harnessapp.MCPConfigRequest{
			HarnessKind: "claude-cli",
			BaseURL:     "http://127.0.0.1:38123",
			Token:       "abc",
		},
	)
	if err != nil {
		t.Fatalf("FormatMCPServers: %v", err)
	}
	if len(servers) != 1 {
		t.Fatalf("servers len=%d want 1", len(servers))
	}
	var cfg struct {
		MCPServers map[string]struct {
			Type string `json:"type"`
			URL  string `json:"url"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal([]byte(servers[0]), &cfg); err != nil {
		t.Fatalf("unmarshal config: %v", err)
	}
	if cfg.MCPServers["agen8"].URL != "http://127.0.0.1:38123/mcp?token=abc" {
		t.Fatalf("config=%+v", cfg.MCPServers["agen8"])
	}
}
