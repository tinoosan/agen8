package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/tinoosan/agen8-mcp-server/internal/app"
	"github.com/tinoosan/agen8-mcp-server/internal/caller"
	"github.com/tinoosan/agen8-mcp-server/internal/core/types"
	"github.com/tinoosan/agen8-mcp-server/internal/logging"
	"github.com/tinoosan/agen8-mcp-server/internal/mcp"
	projecttool "github.com/tinoosan/agen8-mcp-server/internal/mcp/tools/project"
	"github.com/tinoosan/agen8-mcp-server/internal/rpc"
	authapp "github.com/tinoosan/agen8-mcp-server/internal/services/auth/app"
	projectapp "github.com/tinoosan/agen8-mcp-server/internal/services/project/app"
	"github.com/tinoosan/agen8-mcp-server/internal/services/project/domain/member"
	userapp "github.com/tinoosan/agen8-mcp-server/internal/services/user/app"
	userdomain "github.com/tinoosan/agen8-mcp-server/internal/services/user/domain"
	"github.com/tinoosan/agen8-mcp-server/internal/web"
)

type Daemon struct {
	cfg       Config
	app       *app.Application
	rpc       *rpc.Server
	mcpTokens *mcp.TokenStore
	mcp       *mcp.Server
	logger    *slog.Logger
}

func New(cfg Config) (*Daemon, error) {
	cfg, err := cfg.withDefaults()
	if err != nil {
		return nil, err
	}
	application, err := app.NewApplication(app.Config{
		Host:           cfg.AppConfig,
		Logging:        cfg.Logging,
		DaemonHTTPAddr: cfg.HTTPAddr,
	})
	if err != nil {
		return nil, fmt.Errorf("build application: %w", err)
	}
	reg := rpc.NewRegistry()
	for _, register := range []func() error{
		func() error { return rpc.RegisterAuth(reg, application.AuthSvc) },
		func() error { return rpc.RegisterUser(reg, application.UserSvc) },
		func() error { return rpc.RegisterCredential(reg, application.CredentialSvc) },
		func() error { return rpc.RegisterTask(reg, application.TaskSvc, application.ProjectSvc) },
		func() error {
			return rpc.RegisterDecision(reg, application.DecisionSvc, application.ProjectSvc, application.UserSvc)
		},
		func() error { return rpc.RegisterGraph(reg, application.GraphSvc, application.GraphLinks) },
		func() error { return rpc.RegisterMission(reg, application.MissionSvc) },
		func() error { return rpc.RegisterProject(reg, application.ProjectSvc) },
		func() error { return rpc.RegisterFile(reg, application.FileSvc) },
		func() error { return rpc.RegisterLocation(reg, application.LocationSvc) },
	} {
		if err := register(); err != nil {
			return nil, err
		}
	}
	rpcServer, err := rpc.NewServer(reg)
	if err != nil {
		return nil, err
	}
	tokenStore := mcp.NewTokenStore()
	mcpServer, err := mcp.NewServer(tokenStore)
	if err != nil {
		return nil, fmt.Errorf("build mcp server: %w", err)
	}
	logger, err := logging.NewLogger(cfg.Logging)
	if err != nil {
		return nil, fmt.Errorf("build daemon logger: %w", err)
	}
	d := &Daemon{
		cfg:       cfg,
		app:       application,
		rpc:       rpcServer,
		mcpTokens: tokenStore,
		mcp:       mcpServer,
		logger:    logger.With("service", "daemon"),
	}
	mcpServer.SetSessionResolver(d.resolveMCPSession)
	if err := d.registerBootstrapMCPToken(); err != nil {
		return nil, fmt.Errorf("register bootstrap mcp token: %w", err)
	}
	return d, nil
}

type HTTPStrategy struct{}

func (HTTPStrategy) Run(ctx context.Context, d *Daemon) error {
	ln, err := net.Listen("tcp", d.cfg.HTTPAddr)
	if err != nil {
		return fmt.Errorf("listen http daemon: %w", err)
	}
	defer ln.Close()
	return d.serveHTTP(ctx, ln)
}

func (d *Daemon) serveHTTP(ctx context.Context, ln net.Listener) error {
	handler, err := d.httpHandler()
	if err != nil {
		return err
	}
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second}
	if d.cfg.Out != nil {
		if d.setupAvailable(ctx) {
			fmt.Fprintf(d.cfg.Out, "agen8 setup: http://%s/setup?token=%s\n", ln.Addr().String(), d.cfg.SetupToken)
		}
		fmt.Fprintf(d.cfg.Out, "agen8 daemon listening on http://%s\n", ln.Addr().String())
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()
	if err := server.Serve(ln); err != nil && err != http.ErrServerClosed && ctx.Err() == nil {
		return fmt.Errorf("serve http daemon: %w", err)
	}
	return nil
}

func (d *Daemon) httpHandler() (http.Handler, error) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", d.handleHealthz)
	mux.HandleFunc("POST /rpc", d.handleRPC)
	mux.Handle("/mcp", d.mcp.Handler())
	mux.HandleFunc("GET /events", d.handleEvents)
	mux.HandleFunc("GET /setup", d.handleSetupPage)
	mux.HandleFunc("POST /setup", d.handleSetupCreate)
	webHandler, err := d.webHandler()
	if err != nil {
		return nil, fmt.Errorf("mount web ui: %w", err)
	}
	mux.Handle("/", d.handleWeb(webHandler))
	return mux, nil
}

func (d *Daemon) webHandler() (http.Handler, error) {
	devWebURL := strings.TrimSpace(os.Getenv(EnvDevWebURL))
	if devWebURL == "" {
		return web.Handler()
	}
	target, err := url.Parse(devWebURL)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", EnvDevWebURL, err)
	}
	if target.Scheme == "" || target.Host == "" {
		return nil, fmt.Errorf("%s must be an absolute URL", EnvDevWebURL)
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	director := proxy.Director
	proxy.Director = func(r *http.Request) {
		director(r)
		r.Header.Del("Authorization")
		r.Header.Del("Cookie")
	}
	return proxy, nil
}

func (d *Daemon) handleWeb(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && d.setupAvailable(r.Context()) && wantsHTML(r) {
			http.Redirect(w, r, "/setup?token="+d.cfg.SetupToken, http.StatusFound)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (d *Daemon) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"ok":true}`))
}

func (d *Daemon) handleRPC(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read request body", http.StatusBadRequest)
		return
	}
	ctx := r.Context()
	if methodRequiresHTTPIdentity(rpcMethod(body), r.Header.Get("Authorization")) {
		identity, err := d.httpIdentity(r.Context(), r.Header.Get("Authorization"))
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		ctx = rpc.ContextWithIdentity(ctx, identity)
		ctx = caller.ContextWithCaller(ctx, caller.Caller{
			UserID:   identity.UserID,
			MemberID: member.ID(identity.MemberID),
			Role:     identity.Role,
		})
	}
	resp, err := d.rpc.Handle(ctx, body)
	if err != nil {
		http.Error(w, "handle rpc request", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(resp)
}

func (d *Daemon) handleEvents(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher, _ := w.(http.Flusher)
	_, _ = io.WriteString(w, ": connected\n\n")
	if flusher != nil {
		flusher.Flush()
	}
	<-r.Context().Done()
}

func (d *Daemon) registerBootstrapMCPToken() error {
	const token = "agen8-local"
	d.mcpTokens.Register(token, mcp.Session{
		Token:       token,
		Bootstrap:   false,
		UserID:      "local",
		HarnessKind: "codex",
		ContextRegistrar: projectMCPContextRegistrar{
			projects: d.app.ProjectSvc,
			users:    d.app.UserSvc,
			baseURL:  "http://" + d.cfg.HTTPAddr,
		},
		MemberDirectory: d.app.ProjectSvc,
		MemberRegistrar: d.app.ProjectSvc,
		TaskMembers:     d.app.ProjectSvc,
		DecisionService: d.app.DecisionSvc,
		GraphService:    d.app.GraphSvc,
		TaskService:     d.app.TaskSvc,
		MissionService:  d.app.MissionSvc,
		MissionKRs:      d.app.MissionSvc,
		MissionProgress: d.app.MissionSvc,
	})
	return nil
}

func (d *Daemon) resolveMCPSession(ctx context.Context, token string, header http.Header, body []byte) (mcp.Session, error) {
	session, err := d.mcpTokens.Resolve(token)
	if err != nil {
		return mcp.Session{}, err
	}
	sessionID, threadID := mcp.SessionRefsFromHTTPHeader(header)
	if sessionID == "" && threadID == "" {
		bodyRefs := mcp.SessionRequestContextFromJSONRPCBody(body)
		sessionID = bodyRefs.SessionID
		threadID = bodyRefs.ThreadID
	}
	if sessionID == "" && threadID == "" {
		return session, nil
	}
	userID := d.mcpUserID(ctx, token)
	rosterMember, err := d.app.ProjectSvc.ResolveMCPContext(ctx, projectapp.ResolveMCPContextInput{
		Token:       token,
		UserID:      userID,
		ProjectID:   session.ProjectID,
		HarnessKind: session.HarnessKind,
		SessionID:   sessionID,
		ThreadID:    threadID,
	})
	if err != nil {
		if errors.Is(err, member.ErrNotFound) {
			return session, nil
		}
		return mcp.Session{}, err
	}
	session.UserID = strings.TrimSpace(rosterMember.UserID)
	session.MemberID = strings.TrimSpace(string(rosterMember.ID))
	session.ProjectID = strings.TrimSpace(rosterMember.ProjectID)
	session.ChannelID = types.ChannelID(strings.TrimSpace(rosterMember.ChannelID))
	session.HarnessKind = strings.TrimSpace(rosterMember.HarnessKind)
	return session, nil
}

type projectMCPContextRegistrar struct {
	projects *projectapp.Service
	users    *userapp.Service
	baseURL  string
}

func (r projectMCPContextRegistrar) RegisterMCPContext(ctx context.Context, req projecttool.RegisterContextRequest) (projecttool.RegisterContextResult, error) {
	if r.projects == nil {
		return projecttool.RegisterContextResult{}, fmt.Errorf("project service is required")
	}
	result, err := r.projects.RegisterMCPContext(ctx, projectapp.RegisterMCPContextInput{
		Token:            req.Token,
		UserID:           mcpUserID(ctx, r.users, req.Token),
		ProjectID:        req.ProjectID,
		ProjectRoot:      req.ProjectRoot,
		LocationID:       req.LocationID,
		DisplayName:      req.DisplayName,
		HarnessKind:      req.HarnessKind,
		SessionID:        req.SessionID,
		ThreadID:         req.ThreadID,
		NativeSessionRef: req.NativeSessionRef,
		Model:            req.Model,
		Effort:           req.Effort,
		PermissionMode:   req.PermissionMode,
		ConfigRef:        req.ConfigRef,
	})
	if err != nil {
		return projecttool.RegisterContextResult{}, err
	}
	mcpURL := strings.TrimRight(r.baseURL, "/") + "/mcp?token=" + result.Token
	return projecttool.RegisterContextResult{
		ProjectID:        result.ProjectID,
		ProjectRoot:      result.ProjectRoot,
		LocationID:       result.LocationID,
		MemberID:         result.MemberID,
		DisplayName:      result.DisplayName,
		MemberType:       result.MemberType,
		ChannelID:        result.ChannelID,
		SessionID:        result.SessionID,
		ThreadID:         result.ThreadID,
		NativeSessionRef: result.NativeSessionRef,
		Token:            result.Token,
		URL:              mcpURL,
		MCPServers:       result.MCPServers,
	}, nil
}

func (d *Daemon) mcpUserID(ctx context.Context, token string) string {
	if d == nil || d.app == nil {
		return "local"
	}
	return mcpUserID(ctx, d.app.UserSvc, token)
}

func mcpUserID(ctx context.Context, users *userapp.Service, token string) string {
	if strings.TrimSpace(token) == "agen8-local" && users != nil {
		record, err := users.FirstActive(ctx)
		if err == nil && strings.TrimSpace(record.ID.String()) != "" {
			return strings.TrimSpace(record.ID.String())
		}
	}
	return "local"
}

type rpcEnvelope struct {
	Method string `json:"method"`
}

func rpcMethod(body []byte) string {
	var env rpcEnvelope
	_ = json.Unmarshal(body, &env)
	return strings.TrimSpace(env.Method)
}

func methodRequiresHTTPIdentity(method string, authorization string) bool {
	method = strings.TrimSpace(method)
	if method == "" {
		return false
	}
	switch method {
	case "auth.login", "auth.setupStatus", "user.setupStatus":
		return false
	default:
		return strings.TrimSpace(authorization) != ""
	}
}

func (d *Daemon) httpIdentity(ctx context.Context, authorization string) (rpc.Identity, error) {
	token := bearerToken(authorization)
	if token == "" {
		return rpc.Identity{}, fmt.Errorf("bearer token is required")
	}
	user, err := d.app.AuthSvc.ValidateSession(ctx, token)
	if err != nil {
		return rpc.Identity{}, err
	}
	role := string(userdomain.RoleUser)
	if user.Role != "" {
		role = string(user.Role)
	}
	return rpc.Identity{UserID: user.ID.String(), Role: role}, nil
}

func bearerToken(header string) string {
	header = strings.TrimSpace(header)
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(header, prefix))
}

func (d *Daemon) handleSetupPage(w http.ResponseWriter, r *http.Request) {
	if !d.setupAvailable(r.Context()) || !d.validSetupToken(r.URL.Query().Get("token")) {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = io.WriteString(w, strings.ReplaceAll(setupPageHTML, "{{TOKEN}}", html.EscapeString(d.cfg.SetupToken)))
}

type setupRequest struct {
	Token           string `json:"token"`
	Email           string `json:"email"`
	Name            string `json:"name"`
	Password        string `json:"password"`
	ConfirmPassword string `json:"confirmPassword"`
	KeyName         string `json:"keyName"`
}

func (d *Daemon) handleSetupCreate(w http.ResponseWriter, r *http.Request) {
	if !d.setupAvailable(r.Context()) {
		http.Error(w, "setup is closed", http.StatusConflict)
		return
	}
	req, err := decodeSetupRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if !d.validSetupToken(req.Token) {
		http.Error(w, "invalid setup token", http.StatusForbidden)
		return
	}
	if req.Password != req.ConfirmPassword {
		http.Error(w, "password confirmation does not match", http.StatusBadRequest)
		return
	}
	if err := d.app.AuthSvc.ValidatePassword(req.Password); err != nil {
		http.Error(w, "invalid password", http.StatusBadRequest)
		return
	}
	created, err := d.app.UserSvc.SetupFirstUser(r.Context(), userapp.SetupFirstUserParams{Email: req.Email, Name: req.Name})
	if err != nil {
		http.Error(w, "create setup user", http.StatusBadRequest)
		return
	}
	if err := d.app.AuthSvc.CreatePassword(r.Context(), authapp.CreatePasswordParams{UserID: created.User.ID, Password: req.Password}); err != nil {
		http.Error(w, "create setup credential", http.StatusInternalServerError)
		return
	}
	sessionResult, err := d.app.AuthSvc.CreateSession(r.Context(), authapp.CreateSessionParams{UserID: created.User.ID})
	if err != nil {
		http.Error(w, "create setup session", http.StatusInternalServerError)
		return
	}
	keyName := strings.TrimSpace(req.KeyName)
	if keyName == "" {
		keyName = "initial daemon key"
	}
	apiKeyResult, err := d.app.AuthSvc.CreateAPIKey(r.Context(), authapp.CreateAPIKeyParams{UserID: created.User.ID, Name: keyName})
	if err != nil {
		http.Error(w, "create setup api key", http.StatusInternalServerError)
		return
	}
	if !setupWantsJSON(r) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprintf(w, `<!doctype html><html><head><title>agen8 setup complete</title></head><body><script>localStorage.setItem("agen8.sessionToken", %q); location.href = "/";</script><p>Setup complete. Opening agen8...</p></body></html>`, sessionResult.Token)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"user":    created.User,
		"session": map[string]any{"token": sessionResult.Token, "expiresAt": sessionResult.Session.ExpiresAt},
		"apiKey":  map[string]any{"id": apiKeyResult.APIKey.ID.String(), "name": apiKeyResult.APIKey.Name, "prefix": apiKeyResult.APIKey.Prefix, "secret": apiKeyResult.Token},
	})
}

func setupWantsJSON(r *http.Request) bool {
	contentType := strings.ToLower(r.Header.Get("Content-Type"))
	accept := strings.ToLower(r.Header.Get("Accept"))
	return strings.Contains(contentType, "application/json") || strings.Contains(accept, "application/json")
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

func (d *Daemon) setupAvailable(ctx context.Context) bool {
	open, err := d.app.UserSvc.SetupOpen(ctx)
	return err == nil && open
}

func (d *Daemon) validSetupToken(token string) bool {
	return strings.TrimSpace(token) != "" && strings.TrimSpace(token) == strings.TrimSpace(d.cfg.SetupToken)
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
      <section class="panel" aria-label="Create account">
        <h2>Create your account</h2>
        <p class="hint">Use these details to sign in to this agen8 daemon.</p>
        <form method="post" action="/setup">
          <input type="hidden" name="token" value="{{TOKEN}}">
          <label>Email<input name="email" type="email" autocomplete="email" required></label>
          <label>Name<input name="name" autocomplete="name" required></label>
          <label>Password<input name="password" type="password" autocomplete="new-password" minlength="8" required></label>
          <label>Confirm password<input name="confirmPassword" type="password" autocomplete="new-password" minlength="8" required></label>
          <button type="submit">Enter agen8</button>
        </form>
      </section>
    </div>
  </main>
</body>
</html>`
