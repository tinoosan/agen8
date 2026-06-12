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

// projectHooksProvisioner auto-installs the attention hooks when a project is
// created (dec-2055c067). The daemon is local to the machine, so it can write
// the project's .claude/settings.local.json and refresh the user-level Codex
// hooks directly — no agent session in the loop. Stored API keys are hashed,
// so each project creation mints a fresh project-scoped key to embed.
//
// Best-effort by contract: a provisioning failure logs and reports false, and
// must never fail project creation. `agen8 hooks install` remains the manual
// repair path.
type projectHooksProvisioner struct {
	auth    *authapp.Service
	baseURL string
	logger  *slog.Logger
}

func newProjectHooksProvisioner(auth *authapp.Service, httpAddr string, logger *slog.Logger) *projectHooksProvisioner {
	return &projectHooksProvisioner{auth: auth, baseURL: daemonBaseURL(httpAddr), logger: logger}
}

// ProvisionHooks mints a key and writes both harness configs. Returns whether
// hooks ended up installed.
func (p *projectHooksProvisioner) ProvisionHooks(ctx context.Context, userID, projectTitle, root string) bool {
	if p == nil || p.auth == nil {
		return false
	}
	root = strings.TrimSpace(root)
	if root == "" {
		return false
	}
	uid, err := userdomain.NewID(userID)
	if err != nil {
		p.logger.Warn("hooks provision: invalid user id", "error", err)
		return false
	}
	name := strings.TrimSpace(projectTitle)
	if name == "" {
		name = root
	}
	key, err := p.auth.CreateAPIKey(ctx, authapp.CreateAPIKeyParams{
		UserID: uid,
		Name:   "hooks: " + name,
	})
	if err != nil {
		p.logger.Warn("hooks provision: mint api key", "error", err)
		return false
	}

	ok := true
	if _, err := hookinstaller.Install(hookinstaller.Options{
		Harness:    hookinstaller.HarnessClaude,
		BaseURL:    p.baseURL,
		Token:      key.Token,
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
		Token:   key.Token,
	}); err != nil {
		p.logger.Warn("hooks provision: codex install", "error", err)
		ok = false
	}
	return ok
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
