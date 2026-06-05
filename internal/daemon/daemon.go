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
	Token    string `json:"token"`
	Email    string `json:"email"`
	Name     string `json:"name"`
	Password string `json:"password"`
	KeyName  string `json:"keyName"`
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
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>agen8 setup</title>
  <style>
    body { margin: 0; min-height: 100vh; display: grid; place-items: center; font: 14px -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; background: #0f1115; color: #f4f5f7; }
    main { width: min(420px, calc(100vw - 32px)); }
    h1 { margin: 0 0 8px; font-size: 22px; }
    p { margin: 0 0 20px; color: #a7adb8; line-height: 1.5; }
    form { display: grid; gap: 12px; }
    label { display: grid; gap: 6px; color: #c5c9d1; }
    input { border: 1px solid #303641; border-radius: 8px; background: #171a21; color: #fff; padding: 11px 12px; font: inherit; }
    button { border: 0; border-radius: 8px; background: #3b82f6; color: white; padding: 11px 12px; font: inherit; font-weight: 650; cursor: pointer; }
  </style>
</head>
<body>
  <main>
    <h1>Set up agen8</h1>
    <p>Create the first local account for this daemon.</p>
    <form method="post" action="/setup">
      <input type="hidden" name="token" value="{{TOKEN}}" />
      <label>Email <input name="email" type="email" required autocomplete="email" /></label>
      <label>Name <input name="name" required autocomplete="name" /></label>
      <label>Password <input name="password" type="password" required autocomplete="new-password" /></label>
      <button type="submit">Create account</button>
    </form>
  </main>
</body>
</html>`
