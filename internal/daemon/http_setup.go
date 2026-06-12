package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"strings"

	authapp "github.com/tinoosan/agen8/internal/services/auth/app"
	userapp "github.com/tinoosan/agen8/internal/services/user/app"
)

const maxSetupRequestBodyBytes = 64 * 1024

type httpSetupHandler struct {
	auth       *authapp.Service
	users      *userapp.Service
	setupToken string
	httpAddr   string
}

func (d *Daemon) setupHandler() httpSetupHandler {
	if d == nil || d.app == nil {
		return httpSetupHandler{}
	}
	return httpSetupHandler{
		auth:       d.app.AuthSvc,
		users:      d.app.UserSvc,
		setupToken: d.cfg.SetupToken,
		httpAddr:   d.cfg.HTTPAddr,
	}
}

func (d *Daemon) setupAvailable(ctx context.Context) bool {
	return d.setupHandler().available(ctx)
}

func (h httpSetupHandler) handlePage(w http.ResponseWriter, r *http.Request) {
	if !h.available(r.Context()) || !h.validToken(r.URL.Query().Get("token")) {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = io.WriteString(w, strings.ReplaceAll(setupPageHTML, "{{TOKEN}}", html.EscapeString(h.setupToken)))
}

type setupRequest struct {
	Token           string `json:"token"`
	Email           string `json:"email"`
	Name            string `json:"name"`
	Password        string `json:"password"`
	ConfirmPassword string `json:"confirmPassword"`
	KeyName         string `json:"keyName"`
}

func (h httpSetupHandler) handleCreate(w http.ResponseWriter, r *http.Request) {
	if !h.available(r.Context()) {
		http.Error(w, "setup is closed", http.StatusConflict)
		return
	}
	if _, err := readRequestBody(r, maxSetupRequestBodyBytes); err != nil {
		if errors.Is(err, errRPCRequestBodyTooLarge) {
			http.Error(w, "setup request body is too large", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "read setup request body", http.StatusBadRequest)
		return
	}
	req, err := decodeSetupRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if !h.validToken(req.Token) {
		http.Error(w, "invalid setup token", http.StatusForbidden)
		return
	}
	if req.Password != req.ConfirmPassword {
		http.Error(w, "password confirmation does not match", http.StatusBadRequest)
		return
	}
	if err := h.auth.ValidatePassword(req.Password); err != nil {
		http.Error(w, "invalid password", http.StatusBadRequest)
		return
	}
	created, err := h.users.SetupFirstUser(r.Context(), userapp.SetupFirstUserParams{Email: req.Email, Name: req.Name})
	if err != nil {
		http.Error(w, "create setup user", http.StatusBadRequest)
		return
	}
	if err := h.auth.CreatePassword(r.Context(), authapp.CreatePasswordParams{UserID: created.User.ID, Password: req.Password}); err != nil {
		http.Error(w, "create setup credential", http.StatusInternalServerError)
		return
	}
	sessionResult, err := h.auth.CreateSession(r.Context(), authapp.CreateSessionParams{UserID: created.User.ID})
	if err != nil {
		http.Error(w, "create setup session", http.StatusInternalServerError)
		return
	}
	keyName := strings.TrimSpace(req.KeyName)
	if keyName == "" {
		keyName = "initial daemon key"
	}
	apiKeyResult, err := h.auth.CreateAPIKey(r.Context(), authapp.CreateAPIKeyParams{UserID: created.User.ID, Name: keyName})
	if err != nil {
		http.Error(w, "create setup api key", http.StatusInternalServerError)
		return
	}
	mcpArtifacts, err := h.setupMCPArtifacts(r, apiKeyResult.Token)
	if err != nil {
		http.Error(w, "create setup mcp artifacts", http.StatusInternalServerError)
		return
	}
	if !setupWantsJSON(r) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, setupCompleteHTML(sessionResult.Token, apiKeyResult.Token, mcpArtifacts))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"user":    created.User,
		"session": map[string]any{"token": sessionResult.Token, "expiresAt": sessionResult.Session.ExpiresAt},
		"apiKey":  map[string]any{"id": apiKeyResult.APIKey.ID.String(), "name": apiKeyResult.APIKey.Name, "prefix": apiKeyResult.APIKey.Prefix, "secret": apiKeyResult.Token},
		"mcp":     mcpArtifacts,
	})
}

func setupWantsJSON(r *http.Request) bool {
	contentType := strings.ToLower(r.Header.Get("Content-Type"))
	accept := strings.ToLower(r.Header.Get("Accept"))
	return strings.Contains(contentType, "application/json") || strings.Contains(accept, "application/json")
}

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
		ClaudeCommand:      "claude mcp add --transport http --scope user agen8 " + shellQuote(mcpURL) + " --header " + shellQuote("Authorization: Bearer "+token),
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
	if host == "" {
		host = "127.0.0.1:7777"
	}
	return scheme + "://" + normalizeSetupHost(host)
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
				"headers": map[string]string{
					"Authorization": "Bearer ${AGEN8_MCP_TOKEN}",
				},
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

func setupCompleteHTML(sessionToken string, apiKey string, mcp setupMCPResult) string {
	return fmt.Sprintf(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>agen8 setup complete</title>
  <style>
    body { margin: 0; min-height: 100vh; background: #1a1a1c; color: #f0f0f4; font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; }
    main { max-width: 920px; margin: 0 auto; padding: 32px; }
    h1 { margin: 0 0 10px; font-size: 32px; }
    p { color: #9898a8; line-height: 1.5; }
    section { border: 1px solid rgba(255,255,255,.12); background: #1f1f22; border-radius: 12px; padding: 18px; margin-top: 16px; }
    h2 { margin: 0 0 8px; font-size: 18px; }
    code, pre { display: block; overflow: auto; white-space: pre-wrap; word-break: break-all; background: #111114; border-radius: 8px; color: #f0f0f4; padding: 12px; font-size: 13px; }
    a { display: inline-flex; align-items: center; min-height: 40px; margin-top: 18px; padding: 0 14px; border-radius: 8px; background: #3b82f6; color: white; text-decoration: none; font-weight: 700; }
  </style>
</head>
<body>
  <main>
    <h1>Setup complete</h1>
    <p>Your Agen8 account is ready. Copy the MCP key and setup text now; the key is shown only once.</p>
    <section>
      <h2>API key</h2>
      <code>%s</code>
    </section>
    <section>
      <h2>MCP URL</h2>
      <p>Primary endpoint. Send the key as an Authorization bearer token.</p>
      <code>%s</code>
    </section>
    <section>
      <h2>Compatibility query-token URL</h2>
      <p>Use only for clients that cannot send HTTP authorization headers.</p>
      <code>%s</code>
    </section>
    <section>
      <h2>.mcp.json</h2>
      <pre>%s</pre>
    </section>
    <section>
      <h2>Codex command</h2>
      <code>%s</code>
    </section>
    <section>
      <h2>Claude Code command</h2>
      <code>%s</code>
    </section>
    <section>
      <h2>Agen8 skill commands</h2>
      <code>%s</code>
      <code>%s</code>
    </section>
    <a href="/">Open agen8</a>
  </main>
  <script>localStorage.setItem("agen8.sessionToken", %q);</script>
</body>
</html>`,
		html.EscapeString(apiKey),
		html.EscapeString(mcp.URL),
		html.EscapeString(mcp.CompatibilityURL),
		html.EscapeString(mcp.Config),
		html.EscapeString(mcp.CodexCommand),
		html.EscapeString(mcp.ClaudeCommand),
		html.EscapeString(mcp.CodexSkillCommand),
		html.EscapeString(mcp.ClaudeSkillCommand),
		sessionToken,
	)
}

func wantsHTML(r *http.Request) bool {
	accept := strings.ToLower(r.Header.Get("Accept"))
	return accept == "" || strings.Contains(accept, "text/html") || strings.Contains(accept, "*/*")
}

func decodeSetupRequest(r *http.Request) (setupRequest, error) {
	var req setupRequest
	if strings.Contains(strings.ToLower(r.Header.Get("Content-Type")), "application/json") {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			return req, fmt.Errorf("invalid setup json")
		}
		return req, nil
	}
	if err := r.ParseForm(); err != nil {
		return req, fmt.Errorf("invalid setup form")
	}
	req.Token = r.Form.Get("token")
	req.Email = r.Form.Get("email")
	req.Name = r.Form.Get("name")
	req.Password = r.Form.Get("password")
	req.ConfirmPassword = r.Form.Get("confirmPassword")
	req.KeyName = r.Form.Get("keyName")
	return req, nil
}

func (h httpSetupHandler) available(ctx context.Context) bool {
	if h.users == nil {
		return false
	}
	open, err := h.users.SetupOpen(ctx)
	return err == nil && open
}

func (h httpSetupHandler) validToken(token string) bool {
	return strings.TrimSpace(token) != "" && strings.TrimSpace(token) == strings.TrimSpace(h.setupToken)
}

const setupPageHTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <meta name="theme-color" content="#1a1a1c">
  <title>Set up agen8</title>
  <style>
    :root {
      color-scheme: dark;
      --bg-app: #1a1a1c;
      --bg-panel: #1f1f22;
      --bg-surface: #26262a;
      --bg-elevated: #2e2e33;
      --border: rgba(255, 255, 255, 0.10);
      --border-strong: rgba(255, 255, 255, 0.16);
      --text-1: #f0f0f4;
      --text-2: #9898a8;
      --text-3: #636378;
      --accent: #3b82f6;
      --accent-dim: rgba(59, 130, 246, 0.14);
      --green: #22c55e;
      --r-md: 8px;
      --r-lg: 12px;
      --font-sans: 'Aptos', 'Inter Variable', 'Inter', -apple-system, BlinkMacSystemFont, system-ui, sans-serif;
    }
    * { box-sizing: border-box; }
    body {
      margin: 0;
      min-height: 100vh;
      background:
        radial-gradient(circle at 20% 0%, rgba(59, 130, 246, 0.12), transparent 30%),
        linear-gradient(180deg, #1f1f22 0%, var(--bg-app) 58%);
      color: var(--text-1);
      font-family: var(--font-sans);
      letter-spacing: 0;
    }
    main {
      min-height: 100vh;
      display: grid;
      place-items: center;
      padding: 32px;
    }
    .shell {
      width: min(960px, 100%);
      display: grid;
      grid-template-columns: minmax(0, 1fr) 420px;
      gap: 28px;
      align-items: stretch;
    }
    .intro, .panel {
      border: 1px solid var(--border);
      background: color-mix(in srgb, var(--bg-panel) 88%, transparent);
      box-shadow: 0 24px 80px rgba(0, 0, 0, 0.35);
      border-radius: var(--r-lg);
    }
    .intro {
      padding: 34px;
      display: flex;
      flex-direction: column;
      justify-content: space-between;
      min-height: 480px;
    }
    .brand {
      display: flex;
      align-items: center;
      gap: 10px;
      color: var(--text-2);
      font-size: 13px;
      font-weight: 600;
    }
    .dot {
      width: 9px;
      height: 9px;
      border-radius: 999px;
      background: var(--accent);
      box-shadow: 0 0 0 5px var(--accent-dim);
    }
    h1 {
      margin: 70px 0 18px;
      max-width: 620px;
      font-size: clamp(42px, 7vw, 76px);
      line-height: 0.92;
      letter-spacing: 0;
    }
    .lead {
      margin: 0;
      max-width: 560px;
      color: var(--text-2);
      font-size: 17px;
      line-height: 1.55;
    }
    .steps {
      display: grid;
      gap: 10px;
      margin-top: 34px;
    }
    .step {
      display: flex;
      align-items: center;
      gap: 10px;
      color: var(--text-2);
      font-size: 13px;
    }
    .badge {
      width: 22px;
      height: 22px;
      display: grid;
      place-items: center;
      border-radius: 999px;
      background: var(--bg-surface);
      color: var(--text-1);
      font-size: 12px;
      font-weight: 700;
    }
    .panel {
      padding: 28px;
      align-self: stretch;
    }
    h2 {
      margin: 0 0 8px;
      font-size: 21px;
      letter-spacing: 0;
    }
    .hint {
      margin: 0 0 24px;
      color: var(--text-3);
      font-size: 13px;
      line-height: 1.5;
    }
    form {
      display: grid;
      gap: 16px;
    }
    label {
      display: grid;
      gap: 7px;
      color: var(--text-2);
      font-size: 12px;
      font-weight: 600;
    }
    input {
      width: 100%;
      height: 42px;
      border: 1px solid var(--border);
      border-radius: var(--r-md);
      background: var(--bg-surface);
      color: var(--text-1);
      font: inherit;
      font-size: 14px;
      padding: 0 12px;
      outline: none;
    }
    input:focus {
      border-color: color-mix(in srgb, var(--accent) 65%, var(--border));
      box-shadow: 0 0 0 3px var(--accent-dim);
    }
    button {
      height: 42px;
      border: 1px solid var(--accent);
      border-radius: var(--r-md);
      background: var(--accent);
      color: white;
      font: inherit;
      font-size: 14px;
      font-weight: 700;
      cursor: pointer;
      margin-top: 4px;
      min-width: max-content;
      padding: 0 14px;
    }
    button.secondary {
      border-color: var(--border);
      background: var(--bg-surface);
      color: var(--text-1);
      font-weight: 600;
      margin-top: 0;
    }
    button:disabled {
      cursor: wait;
      opacity: 0.68;
    }
    a.button-link {
      height: 42px;
      display: inline-flex;
      align-items: center;
      justify-content: center;
      border-radius: var(--r-md);
      background: var(--accent);
      color: white;
      font-size: 14px;
      font-weight: 700;
      text-decoration: none;
      padding: 0 14px;
    }
    .error {
      border: 1px solid rgba(239, 68, 68, 0.35);
      border-radius: var(--r-md);
      background: rgba(239, 68, 68, 0.12);
      color: #fecaca;
      font-size: 13px;
      line-height: 1.45;
      padding: 10px 12px;
    }
    .result {
      display: grid;
      gap: 16px;
    }
    .result-kicker {
      display: inline-flex;
      width: fit-content;
      align-items: center;
      border-radius: 999px;
      background: var(--accent-dim);
      color: #93c5fd;
      font-size: 11px;
      font-weight: 800;
      letter-spacing: .08em;
      padding: 4px 8px;
      text-transform: uppercase;
    }
    .result-title {
      margin: 2px 0 4px;
      font-size: 28px;
      line-height: 1.05;
    }
    .result-copy {
      margin: 0;
      color: var(--text-2);
      font-size: 14px;
      line-height: 1.55;
    }
    .command-grid {
      display: grid;
      gap: 12px;
    }
    .command-card {
      display: grid;
      gap: 12px;
      border: 1px solid color-mix(in srgb, var(--accent) 45%, var(--border));
      border-radius: var(--r-lg);
      background:
        linear-gradient(180deg, rgba(59, 130, 246, 0.10), transparent 58%),
        var(--bg-surface);
      padding: 14px;
    }
    .command-card.secondary-card {
      border-color: var(--border);
      background: var(--bg-surface);
    }
    .command-head {
      display: flex;
      align-items: flex-start;
      justify-content: space-between;
      gap: 12px;
    }
    .command-label {
      color: var(--text-1);
      font-size: 15px;
      font-weight: 800;
    }
    .command-note {
      margin: 4px 0 0;
      color: var(--text-3);
      font-size: 12px;
      line-height: 1.45;
    }
    .details-stack {
      display: grid;
      gap: 8px;
    }
    details {
      border: 1px solid var(--border);
      border-radius: var(--r-md);
      background: var(--bg-surface);
      overflow: hidden;
    }
    summary {
      cursor: pointer;
      list-style: none;
      padding: 12px;
      color: var(--text-1);
      font-size: 13px;
      font-weight: 800;
    }
    summary::-webkit-details-marker {
      display: none;
    }
    summary::after {
      content: "+";
      float: right;
      color: var(--text-3);
    }
    details[open] summary::after {
      content: "-";
    }
    .details-body {
      display: grid;
      gap: 10px;
      border-top: 1px solid var(--border);
      padding: 12px;
    }
    .snippet {
      display: grid;
      gap: 8px;
      border: 1px solid var(--border);
      border-radius: var(--r-md);
      background: var(--bg-surface);
      padding: 12px;
    }
    .snippet-head {
      display: flex;
      align-items: flex-start;
      justify-content: space-between;
      gap: 12px;
    }
    .snippet-title {
      color: var(--text-1);
      font-size: 13px;
      font-weight: 700;
    }
    .snippet-hint {
      margin: 3px 0 0;
      color: var(--text-3);
      font-size: 12px;
      line-height: 1.45;
    }
    code, pre {
      display: block;
      max-height: 220px;
      overflow: auto;
      white-space: pre;
      word-break: normal;
      border-radius: var(--r-md);
      background: var(--bg-app);
      color: var(--text-1);
      font-size: 12px;
      line-height: 1.55;
      padding: 10px;
    }
    pre {
      max-height: 260px;
    }
    .actions {
      display: flex;
      flex-wrap: wrap;
      gap: 10px;
      margin-top: 4px;
    }
    [hidden] {
      display: none !important;
    }
    @media (max-width: 820px) {
      main { padding: 18px; place-items: start center; }
      .shell { grid-template-columns: 1fr; }
      .intro { min-height: auto; padding: 26px; }
      h1 { margin-top: 44px; }
    }
  </style>
</head>
<body>
  <main>
    <div class="shell">
      <section class="intro" aria-labelledby="setup-title">
        <div>
          <div class="brand"><span class="dot"></span><span>Welcome to agen8</span></div>
          <h1 id="setup-title">Bring your AI work into focus.</h1>
          <p class="lead">agen8 gives AI harnesses a durable work-context layer for missions, key results, tasks, decisions, and graph-backed context. Create your account to open the workspace.</p>
        </div>
        <div class="steps" aria-label="Setup outcome">
          <div class="step"><span class="badge">1</span><span>Create your local sign-in account.</span></div>
          <div class="step"><span class="badge">2</span><span>Create your first project.</span></div>
          <div class="step"><span class="badge">3</span><span>Connect agents through MCP when you need them.</span></div>
        </div>
      </section>
      <section class="panel" aria-label="Account setup">
        <div id="setup-create">
          <h2>Create your account</h2>
          <p class="hint">Use these details to sign in to this agen8 daemon.</p>
          <form id="setup-form" method="post" action="/setup">
            <input type="hidden" name="token" value="{{TOKEN}}">
            <label>Email<input name="email" type="email" autocomplete="email" required></label>
            <label>Name<input name="name" autocomplete="name" required></label>
            <label>Password<input name="password" type="password" autocomplete="new-password" minlength="8" required></label>
            <label>Confirm password<input name="confirmPassword" type="password" autocomplete="new-password" minlength="8" required></label>
            <button type="submit">Enter agen8</button>
          </form>
        </div>
        <div id="setup-error" class="error" role="alert" hidden></div>
        <div id="setup-result" class="result" hidden>
          <div>
            <div class="result-kicker">Setup complete</div>
            <h2 class="result-title">Connect Agen8 to your AI client</h2>
            <p class="result-copy">Pick the command for the client you use. The raw key is tucked below and shown only once, in case you need a manual config.</p>
          </div>

          <div class="command-grid" aria-label="Recommended client setup">
            <div class="command-card">
              <div class="command-head">
                <div>
                  <div class="command-label">Codex</div>
                  <p class="command-note">Best for this workspace. Run this once in your terminal.</p>
                </div>
                <button class="secondary" type="button" data-copy-target="codex-command">Copy command</button>
              </div>
              <code id="codex-command"></code>
            </div>

            <div class="command-card secondary-card">
              <div class="command-head">
                <div>
                  <div class="command-label">Claude Code</div>
                  <p class="command-note">Adds Agen8 for Claude Code at user scope.</p>
                </div>
                <button class="secondary" type="button" data-copy-target="claude-command">Copy command</button>
              </div>
              <code id="claude-command"></code>
            </div>
          </div>

          <div class="details-stack">
            <details>
              <summary>Manual token and URL</summary>
              <div class="details-body">
                <div class="snippet">
                  <div class="snippet-head">
                    <div>
                      <div class="snippet-title">API key</div>
                      <p class="snippet-hint">Shown once. Keep it private.</p>
                    </div>
                    <button class="secondary" type="button" data-copy-target="api-key">Copy</button>
                  </div>
                  <code id="api-key"></code>
                </div>
                <div class="snippet">
                  <div class="snippet-head">
                    <div>
                      <div class="snippet-title">MCP URL</div>
                      <p class="snippet-hint">Primary endpoint. Send the key as an Authorization bearer token.</p>
                    </div>
                    <button class="secondary" type="button" data-copy-target="mcp-url">Copy</button>
                  </div>
                  <code id="mcp-url"></code>
                </div>
                <div class="snippet">
                  <div class="snippet-head">
                    <div>
                      <div class="snippet-title">Compatibility query-token URL</div>
                      <p class="snippet-hint">Use only for clients that cannot send HTTP authorization headers.</p>
                    </div>
                    <button class="secondary" type="button" data-copy-target="mcp-compatibility-url">Copy</button>
                  </div>
                  <code id="mcp-compatibility-url"></code>
                </div>
              </div>
            </details>

            <details>
              <summary>JSON config</summary>
              <div class="details-body">
                <div class="snippet">
                  <div class="snippet-head">
                    <div>
                      <div class="snippet-title">.mcp.json</div>
                      <p class="snippet-hint">Use this when your MCP client reads JSON server entries.</p>
                    </div>
                    <button class="secondary" type="button" data-copy-target="mcp-config">Copy</button>
                  </div>
                  <pre id="mcp-config"></pre>
                </div>
              </div>
            </details>

            <details>
              <summary>Agen8 skill commands</summary>
              <div class="details-body">
                <code id="codex-skill-command"></code>
                <code id="claude-skill-command"></code>
              </div>
            </details>
          </div>

          <div class="actions">
            <a class="button-link" href="/">Open agen8</a>
          </div>
        </div>
      </section>
    </div>
  </main>
  <script>
    (function () {
      var form = document.getElementById('setup-form');
      var createBox = document.getElementById('setup-create');
      var errorBox = document.getElementById('setup-error');
      var resultBox = document.getElementById('setup-result');
      var submitButton = form ? form.querySelector('button[type="submit"]') : null;

      function setText(id, value) {
        var el = document.getElementById(id);
        if (el) el.textContent = value || '';
      }

      function showError(message) {
        if (!errorBox) return;
        errorBox.textContent = message || 'Setup failed';
        errorBox.hidden = false;
      }

      function setupPayload() {
        var data = new FormData(form);
        return {
          token: String(data.get('token') || ''),
          email: String(data.get('email') || ''),
          name: String(data.get('name') || ''),
          password: String(data.get('password') || ''),
          confirmPassword: String(data.get('confirmPassword') || '')
        };
      }

      if (form) {
        form.addEventListener('submit', function (event) {
          event.preventDefault();
          if (errorBox) errorBox.hidden = true;
          if (submitButton) {
            submitButton.disabled = true;
            submitButton.textContent = 'Creating account...';
          }
          fetch('/setup', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json', 'Accept': 'application/json' },
            body: JSON.stringify(setupPayload())
          }).then(function (res) {
            if (!res.ok) {
              return res.text().then(function (body) {
                throw new Error(body || 'Setup failed');
              });
            }
            return res.json();
          }).then(function (body) {
            var apiKey = body && body.apiKey ? body.apiKey.secret : '';
            var mcp = body && body.mcp ? body.mcp : {};
            if (!apiKey) throw new Error('Setup did not return an API key');
            if (body.session && body.session.token) {
              localStorage.setItem('agen8.sessionToken', body.session.token);
            }
            setText('api-key', apiKey);
            setText('mcp-url', mcp.url);
            setText('mcp-compatibility-url', mcp.compatibilityUrl);
            setText('mcp-config', mcp.config);
            setText('codex-command', mcp.codexCommand);
            setText('claude-command', mcp.claudeCommand);
            setText('codex-skill-command', mcp.codexSkillCommand);
            setText('claude-skill-command', mcp.claudeSkillCommand);
            if (createBox) createBox.hidden = true;
            if (resultBox) resultBox.hidden = false;
          }).catch(function (err) {
            showError(err && err.message ? err.message.trim() : 'Setup failed');
          }).finally(function () {
            if (submitButton) {
              submitButton.disabled = false;
              submitButton.textContent = 'Enter agen8';
            }
          });
        });
      }

      document.addEventListener('click', function (event) {
        var button = event.target && event.target.closest ? event.target.closest('[data-copy-target]') : null;
        if (!button) return;
        var target = document.getElementById(button.getAttribute('data-copy-target'));
        var value = target ? target.textContent : '';
        if (!value || !navigator.clipboard) return;
        var original = button.textContent;
        navigator.clipboard.writeText(value).then(function () {
          button.textContent = 'Copied';
          setTimeout(function () { button.textContent = original || 'Copy'; }, 1200);
        });
      });
    })();
  </script>
</body>
</html>`
