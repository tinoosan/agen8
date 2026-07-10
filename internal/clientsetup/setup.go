// Package clientsetup installs every Agen8 integration that belongs on an AI
// harness machine. It deliberately does not contact or mutate the daemon.
package clientsetup

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/tinoosan/agen8/internal/hookinstaller"
	"github.com/tinoosan/agen8/internal/skillinstaller"
)

const HarnessClaude = "claude"

type Options struct {
	Harness    string
	BaseURL    string
	Token      string
	ProjectDir string
	HomeDir    string
	Context    context.Context
}

type Result struct {
	Harness    string
	ProjectDir string
	SkillRoot  string
	HooksPath  string
	MCPPath    string
	MCPURL     string
}

// Install validates the complete request before writing, then performs the
// three idempotent client operations. A rerun repairs any partial prior run.
func Install(opts Options) (Result, error) {
	harness := strings.ToLower(strings.TrimSpace(opts.Harness))
	if harness != HarnessClaude {
		return Result{}, fmt.Errorf("client setup: unsupported harness %q; use claude", opts.Harness)
	}
	baseURL := strings.TrimRight(strings.TrimSpace(opts.BaseURL), "/")
	parsedURL, err := url.Parse(baseURL)
	if err != nil || parsedURL.Scheme == "" || parsedURL.Host == "" || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") {
		return Result{}, fmt.Errorf("client setup: --url must be an absolute http(s) URL")
	}
	if strings.TrimSpace(opts.Token) == "" {
		return Result{}, fmt.Errorf("client setup: --token is required")
	}
	projectDir := strings.TrimSpace(opts.ProjectDir)
	if projectDir == "" {
		projectDir, err = os.Getwd()
		if err != nil {
			return Result{}, fmt.Errorf("client setup: resolve working directory: %w", err)
		}
	}
	projectDir, err = filepath.Abs(projectDir)
	if err != nil {
		return Result{}, fmt.Errorf("client setup: resolve project directory: %w", err)
	}
	info, err := os.Stat(projectDir)
	if err != nil {
		return Result{}, fmt.Errorf("client setup: stat project directory: %w", err)
	}
	if !info.IsDir() {
		return Result{}, fmt.Errorf("client setup: project directory is not a directory")
	}

	skills, err := skillinstaller.Install(skillinstaller.Options{
		Harness: skillinstaller.HarnessClaudeCLI,
		HomeDir: strings.TrimSpace(opts.HomeDir),
	})
	if err != nil {
		return Result{}, fmt.Errorf("client setup: install skills: %w", err)
	}
	hooks, err := hookinstaller.Install(hookinstaller.Options{
		Harness:    hookinstaller.HarnessClaude,
		BaseURL:    baseURL,
		Token:      opts.Token,
		ProjectDir: projectDir,
	})
	if err != nil {
		return Result{}, fmt.Errorf("client setup: install hooks: %w", err)
	}
	mcpResult, err := hookinstaller.InstallClaudeMCP(hookinstaller.MCPOptions{
		BaseURL:    baseURL,
		Token:      opts.Token,
		ProjectDir: projectDir,
		Context:    opts.Context,
	})
	if err != nil {
		return Result{}, fmt.Errorf("client setup: install Claude MCP: %w", err)
	}
	return Result{
		Harness:    harness,
		ProjectDir: projectDir,
		SkillRoot:  skills.Root,
		HooksPath:  hooks.Path,
		MCPPath:    mcpResult.Path,
		MCPURL:     mcpResult.URL,
	}, nil
}
