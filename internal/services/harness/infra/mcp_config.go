package infra

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	harnessapp "github.com/tinoosan/agen8-mcp-server/internal/services/harness/app"
)

type MCPConfigFormatter struct {
	DataDir string
}

func (f MCPConfigFormatter) FormatMCPServers(_ context.Context, request harnessapp.MCPConfigRequest) ([]string, error) {
	rawURL, err := mcpURL(request)
	if err != nil {
		return nil, err
	}
	kind := strings.TrimSpace(request.HarnessKind)
	if rawURL == "" {
		return nil, fmt.Errorf("mcp url is required")
	}
	switch kind {
	case "codex":
		return codexMCPServerConfig(rawURL), nil
	case "claude-cli":
		if strings.TrimSpace(request.BaseURL) != "" {
			config, err := claudeMCPConfigJSON(rawURL)
			if err != nil {
				return nil, err
			}
			return []string{config}, nil
		}
		path, err := f.writeClaudeMCPConfig(rawURL)
		if err != nil {
			return nil, err
		}
		return []string{path}, nil
	default:
		return nil, fmt.Errorf("unsupported harness kind %q for mcp config", kind)
	}
}

func mcpURL(request harnessapp.MCPConfigRequest) (string, error) {
	rawURL := strings.TrimSpace(request.RawURL)
	if rawURL != "" {
		return rawURL, nil
	}
	baseURL := strings.TrimSpace(request.BaseURL)
	token := strings.TrimSpace(request.Token)
	if baseURL == "" {
		return "", fmt.Errorf("mcp base url is required")
	}
	if token == "" {
		return "", fmt.Errorf("mcp token is required")
	}
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("parse mcp base url: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("mcp base url must be absolute")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + "/mcp"
	values := parsed.Query()
	values.Set("token", token)
	parsed.RawQuery = values.Encode()
	return parsed.String(), nil
}

func codexMCPServerConfig(rawURL string) []string {
	escapedURL := strings.ReplaceAll(rawURL, `"`, `\"`)
	return []string{
		`mcp_servers.agen8.url="` + escapedURL + `"`,
	}
}

func (f MCPConfigFormatter) writeClaudeMCPConfig(rawURL string) (string, error) {
	if strings.TrimSpace(f.DataDir) == "" {
		return "", fmt.Errorf("data dir is required")
	}
	dir := filepath.Join(f.DataDir, "harness", "mcp")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create claude mcp config dir: %w", err)
	}
	values, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("parse mcp url: %w", err)
	}
	token := strings.TrimSpace(values.Query().Get("token"))
	if token == "" {
		return "", fmt.Errorf("mcp token is required in url")
	}
	path := filepath.Join(dir, token+".json")
	data, err := claudeMCPConfigData(rawURL)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", fmt.Errorf("write claude mcp config: %w", err)
	}
	return path, nil
}

func claudeMCPConfigJSON(rawURL string) (string, error) {
	data, err := claudeMCPConfigData(rawURL)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

func claudeMCPConfigData(rawURL string) ([]byte, error) {
	payload := map[string]any{
		"mcpServers": map[string]any{
			"agen8": map[string]any{
				"type": "http",
				"url":  rawURL,
			},
		},
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal claude mcp config: %w", err)
	}
	data = append(data, '\n')
	return data, nil
}
