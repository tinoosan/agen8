package daemon

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"strings"

	"github.com/tinoosan/agen8/internal/hookinstaller"
	authapp "github.com/tinoosan/agen8/internal/services/auth/app"
	userdomain "github.com/tinoosan/agen8/internal/services/user/domain"
)

// projectHooksProvisioner configures harness integrations on the machine that
// runs the daemon. Hosted deployments disable it because client configuration
// belongs on each developer machine.
//
// Best-effort by contract: a provisioning failure logs and reports false, and
// must never fail project creation. `agen8 hooks install` remains the manual
// repair path.
type projectHooksProvisioner struct {
	auth              *authapp.Service
	baseURL           string
	localInstallation bool
	logger            *slog.Logger
}

type projectClaudeMCPProvisionResult struct {
	Installed            bool
	Path                 string
	ServerName           string
	URL                  string
	RequiresClientAction bool
	ClientSetupCommand   string
}

type projectProvisionResult struct {
	HooksInstalled       bool
	ClaudeMCP            projectClaudeMCPProvisionResult
	RequiresClientAction bool
	ClientSetupCommand   string
	Warnings             []string
}

func newProjectHooksProvisionerWithBaseURL(auth *authapp.Service, baseURL string, logger *slog.Logger) *projectHooksProvisioner {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = daemonBaseURL(DefaultHTTPAddr)
	}
	if logger == nil {
		logger = slog.Default().With("service", "project-provision")
	}
	return &projectHooksProvisioner{auth: auth, baseURL: baseURL, localInstallation: true, logger: logger}
}

// ProvisionHooks mints a key and writes both harness configs. Returns whether
// hooks ended up installed.
func (p *projectHooksProvisioner) ProvisionHooks(ctx context.Context, userID, projectTitle, root string) bool {
	if p == nil {
		return false
	}
	token, err := p.mintProjectToken(ctx, userID, projectTitle, root, "hooks")
	if err != nil {
		p.logger.Warn("hooks provision: mint api key", "error", err)
		return false
	}
	return p.installHooks(root, token)
}

func (p *projectHooksProvisioner) installHooks(root, token string) bool {
	ok := true
	if _, err := hookinstaller.Install(hookinstaller.Options{
		Harness:    hookinstaller.HarnessClaude,
		BaseURL:    p.baseURL,
		Token:      token,
		ProjectDir: root,
	}); err != nil {
		p.logger.Warn("hooks provision: claude install", "root", root, "error", err)
		ok = false
	}
	// Codex hooks are user-level; cwd attribution scopes their events to the
	// right project, so one fresh install/refresh serves every project.
	if _, err := hookinstaller.Install(hookinstaller.Options{
		Harness: hookinstaller.HarnessCodex,
		BaseURL: p.baseURL,
		Token:   token,
	}); err != nil {
		p.logger.Warn("hooks provision: codex install", "error", err)
		ok = false
	}
	return ok
}

// ProvisionClaudeMCP mints a fresh project-scoped key and upserts Claude Code's
// project-local Agen8 MCP server entry. Unlike project creation hooks, this is
// caller-visible: repair failures are returned so the caller can act.
func (p *projectHooksProvisioner) ProvisionClaudeMCP(ctx context.Context, userID, projectTitle, root string) (projectClaudeMCPProvisionResult, error) {
	if p == nil {
		return projectClaudeMCPProvisionResult{}, fmt.Errorf("claude mcp provisioner is not configured")
	}
	if !p.localInstallation {
		command, err := p.claudeClientSetupCommand(ctx, userID, projectTitle, root)
		if err != nil {
			return projectClaudeMCPProvisionResult{}, err
		}
		return projectClaudeMCPProvisionResult{
			ServerName:           "agen8",
			URL:                  p.baseURL + "/mcp",
			RequiresClientAction: true,
			ClientSetupCommand:   command,
		}, nil
	}
	token, err := p.mintProjectToken(ctx, userID, projectTitle, root, "claude mcp")
	if err != nil {
		p.logger.Warn("claude mcp provision: mint api key", "error", err)
		return projectClaudeMCPProvisionResult{}, err
	}
	return p.installClaudeMCP(ctx, root, token)
}

func (p *projectHooksProvisioner) installClaudeMCP(ctx context.Context, root, token string) (projectClaudeMCPProvisionResult, error) {
	result, err := hookinstaller.InstallClaudeMCP(hookinstaller.MCPOptions{
		BaseURL:    p.baseURL,
		Token:      token,
		ProjectDir: root,
		Context:    ctx,
	})
	if err != nil {
		p.logger.Warn("claude mcp provision: install", "root", root, "error", err)
		return projectClaudeMCPProvisionResult{}, err
	}
	return projectClaudeMCPProvisionResult{
		Installed:  true,
		Path:       result.Path,
		ServerName: result.ServerName,
		URL:        result.URL,
	}, nil
}

// ProvisionProject uses one project-scoped credential for attention hooks and
// Claude MCP, then reports each outcome independently. Failures never undo the
// project record.
func (p *projectHooksProvisioner) ProvisionProject(ctx context.Context, userID, projectTitle, root string) projectProvisionResult {
	result := projectProvisionResult{}
	if p == nil {
		result.Warnings = append(result.Warnings, "Local harness setup is not configured.")
		return result
	}
	if !p.localInstallation {
		command, err := p.claudeClientSetupCommand(ctx, userID, projectTitle, root)
		if err != nil {
			p.logger.Warn("project provision: create client setup command", "error", err)
			result.Warnings = append(result.Warnings, "Could not create a client setup command.")
			return result
		}
		result.RequiresClientAction = true
		result.ClientSetupCommand = command
		return result
	}
	token, err := p.mintProjectToken(ctx, userID, projectTitle, root, "project integration")
	if err != nil {
		p.logger.Warn("project provision: mint api key", "error", err)
		result.Warnings = append(result.Warnings, "Could not create a harness credential.")
		return result
	}
	result.HooksInstalled = p.installHooks(root, token)
	if !result.HooksInstalled {
		result.Warnings = append(result.Warnings, "Attention hooks could not be installed.")
	}
	claudeMCP, err := p.installClaudeMCP(ctx, root, token)
	if err != nil {
		result.Warnings = append(result.Warnings, "Claude MCP could not be configured on the daemon machine.")
	} else {
		result.ClaudeMCP = claudeMCP
	}
	return result
}

func (p *projectHooksProvisioner) claudeClientSetupCommand(ctx context.Context, userID, projectTitle, root string) (string, error) {
	token, err := p.mintProjectToken(ctx, userID, projectTitle, root, "client setup")
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(
		"agen8 client setup --harness claude --url %s --token %s",
		shellQuote(p.baseURL),
		shellQuote(token),
	), nil
}

func (p *projectHooksProvisioner) mintProjectToken(ctx context.Context, userID, projectTitle, root, purpose string) (string, error) {
	if p == nil || p.auth == nil {
		return "", fmt.Errorf("project provisioner is not configured")
	}
	root = strings.TrimSpace(root)
	if root == "" {
		return "", fmt.Errorf("project root is required")
	}
	uid, err := userdomain.NewID(userID)
	if err != nil {
		return "", err
	}
	name := strings.TrimSpace(projectTitle)
	if name == "" {
		name = root
	}
	key, err := p.auth.CreateAPIKey(ctx, authapp.CreateAPIKeyParams{
		UserID: uid,
		Name:   strings.TrimSpace(purpose) + ": " + name,
	})
	if err != nil {
		return "", err
	}
	return key.Token, nil
}

// daemonBaseURL turns the daemon's listen address into the origin hooks should
// POST to, mirroring the web setup helper's wildcard-to-loopback rule.
func daemonBaseURL(httpAddr string) string {
	host, port, err := net.SplitHostPort(strings.TrimSpace(httpAddr))
	if err != nil {
		return "http://127.0.0.1:7777"
	}
	switch host {
	case "", "0.0.0.0", "::", "[::]":
		host = "127.0.0.1"
	}
	return fmt.Sprintf("http://%s", net.JoinHostPort(host, port))
}
