package daemon

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
)

type setupMCPResult struct {
	URL                string `json:"url"`
	CompatibilityURL   string `json:"compatibilityUrl,omitempty"`
	Config             string `json:"config"`
	CodexCommand       string `json:"codexCommand"`
	ClaudeCommand      string `json:"claudeCommand"`
	CodexSkillCommand  string `json:"codexSkillCommand"`
	ClaudeSkillCommand string `json:"claudeSkillCommand"`
}

func (h httpSetupHandler) setupMCPArtifacts(r *http.Request, token string) (setupMCPResult, error) {
	baseURL := h.setupRequestOrigin(r)
	mcpURL := h.setupMCPURL(r)
	compatibilityURL := h.setupMCPCompatibilityURL(r, token)
	config, err := setupMCPConfig(mcpURL)
	if err != nil {
		return setupMCPResult{}, err
	}
	return setupMCPResult{
		URL:                mcpURL,
		CompatibilityURL:   compatibilityURL,
		Config:             config,
		CodexCommand:       "export AGEN8_MCP_TOKEN=" + shellQuote(token) + "\n" + "codex mcp add agen8 --url " + shellQuote(mcpURL) + " --bearer-token-env-var AGEN8_MCP_TOKEN",
		ClaudeCommand:      "agen8 client setup --harness claude --url " + shellQuote(baseURL) + " --token " + shellQuote(token),
		CodexSkillCommand:  "agen8 skill install --harness codex",
		ClaudeSkillCommand: "agen8 skill install --harness claude-cli",
	}, nil
}

func (h httpSetupHandler) setupMCPURL(r *http.Request) string {
	return h.setupRequestOrigin(r) + "/mcp"
}

func (h httpSetupHandler) setupMCPCompatibilityURL(r *http.Request, token string) string {
	return h.setupRequestOrigin(r) + "/mcp?token=" + url.QueryEscape(token)
}

func (h httpSetupHandler) setupRequestOrigin(r *http.Request) string {
	if publicURL := strings.TrimRight(strings.TrimSpace(h.publicURL), "/"); publicURL != "" {
		return publicURL
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	host := strings.TrimSpace(r.Host)
	if host == "" {
		host = strings.TrimSpace(r.URL.Host)
	}
	if host == "" {
		host = strings.TrimSpace(h.httpAddr)
	}
	if !safeSetupRequestHost(host) {
		host = strings.TrimSpace(h.httpAddr)
		if !safeSetupRequestHost(host) {
			host = ""
		}
	}
	if host == "" {
		host = "127.0.0.1:7777"
	}
	return scheme + "://" + normalizeSetupHost(host)
}

func safeSetupRequestHost(host string) bool {
	host = strings.TrimSpace(host)
	if host == "" {
		return false
	}
	if strings.ContainsAny(host, "/\\@%") {
		return false
	}
	name := host
	if parsed, _, err := net.SplitHostPort(host); err == nil {
		name = parsed
	}
	name = strings.Trim(strings.ToLower(strings.TrimSpace(name)), "[]")
	if name == "" {
		return false
	}
	if name == "localhost" || name == "0.0.0.0" || name == "::" {
		return true
	}
	if ip := net.ParseIP(name); ip != nil {
		return ip.IsLoopback() || ip.IsUnspecified()
	}
	return false
}

func normalizeSetupHost(host string) string {
	switch {
	case strings.HasPrefix(host, "0.0.0.0:"):
		return "127.0.0.1:" + strings.TrimPrefix(host, "0.0.0.0:")
	case host == "0.0.0.0":
		return "127.0.0.1"
	case strings.HasPrefix(host, "[::]:"):
		return "127.0.0.1:" + strings.TrimPrefix(host, "[::]:")
	case host == "[::]" || host == "::":
		return "127.0.0.1"
	default:
		return host
	}
}

func setupMCPConfig(mcpURL string) (string, error) {
	config := map[string]any{
		"mcpServers": map[string]any{
			"agen8": map[string]any{
				"type": "http",
				"url":  mcpURL,
				// #nosec G101 -- this is the client-side environment variable name, not a bearer token value.
				"bearer_token_env_var": "AGEN8_MCP_TOKEN",
			},
		},
	}
	encoded, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal setup mcp config: %w", err)
	}
	return string(encoded), nil
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
