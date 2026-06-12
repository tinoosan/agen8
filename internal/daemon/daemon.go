package daemon

import (
	"bytes"
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

	"github.com/tinoosan/agen8/internal/app"
	"github.com/tinoosan/agen8/internal/caller"
	"github.com/tinoosan/agen8/internal/core/types"
	"github.com/tinoosan/agen8/internal/logging"
	"github.com/tinoosan/agen8/internal/mcp"
	projecttool "github.com/tinoosan/agen8/internal/mcp/tools/project"
	"github.com/tinoosan/agen8/internal/rpc"
	"github.com/tinoosan/agen8/internal/services/attention"
	authapp "github.com/tinoosan/agen8/internal/services/auth/app"
	projectapp "github.com/tinoosan/agen8/internal/services/project/app"
	"github.com/tinoosan/agen8/internal/services/project/domain/member"
	userapp "github.com/tinoosan/agen8/internal/services/user/app"
	"github.com/tinoosan/agen8/internal/web"
	"github.com/tinoosan/agen8/pkg/buildinfo"
)

type loopbackIPLookup func(string) ([]net.IP, error)

type Daemon struct {
	cfg       Config
	app       *app.Application
	rpc       *rpc.Server
	mcpTokens *mcp.TokenStore
	mcp       *mcp.Server
	events    *eventsHub
	attention *attention.Service
	logger    *slog.Logger
}

const (
	maxRPCRequestBodyBytes   = 1024 * 1024
	maxSetupRequestBodyBytes = 64 * 1024
)

var errRPCRequestBodyTooLarge = errors.New("rpc request body is too large")

var lookupLoopbackHostIPs loopbackIPLookup = net.LookupIP

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
	logger, err := logging.NewLogger(cfg.Logging)
	if err != nil {
		return nil, fmt.Errorf("build daemon logger: %w", err)
	}
	attentionSvc, err := attention.NewService(
		application.ProjectSvc,
		application.EventBus,
		nil,
		attention.DefaultTTL,
		logger.With("service", "attention"),
	)
	if err != nil {
		return nil, fmt.Errorf("build attention service: %w", err)
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
		func() error { return rpc.RegisterPin(reg, application.PinSvc) },
		func() error { return rpc.RegisterNotification(reg, application.NotificationSvc) },
		func() error { return rpc.RegisterAttention(reg, attentionSvc) },
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
	d := &Daemon{
		cfg:       cfg,
		app:       application,
		rpc:       rpcServer,
		mcpTokens: tokenStore,
		mcp:       mcpServer,
		events:    newEventsHub(application.EventBus, logger.With("service", "events")),
		attention: attentionSvc,
		logger:    logger.With("service", "daemon"),
	}
	mcpServer.SetSessionResolver(d.resolveMCPSession)
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
	go func() {
		if err := d.events.Run(ctx); err != nil && !errors.Is(err, context.Canceled) && ctx.Err() == nil {
			d.logger.Error("events hub stopped", "error", err)
		}
	}()
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
	mux.HandleFunc("POST /hooks/attention", d.handleAttentionHook)
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
	target, err := parseLoopbackDevWebURL(devWebURL)
	if err != nil {
		return nil, fmt.Errorf("%s is invalid: %w", EnvDevWebURL, err)
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.Transport = &http.Transport{
		DialContext: loopbackAwareDialContext(),
	}
	director := proxy.Director
	proxy.Director = func(r *http.Request) {
		director(r)
		r.Header.Del("Authorization")
		r.Header.Del("Cookie")
	}
	return proxy, nil
}

func loopbackAwareDialContext() func(context.Context, string, string) (net.Conn, error) {
	dialer := &net.Dialer{}
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, _, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, fmt.Errorf("proxy target host is invalid: %w", err)
		}
		// Dev web proxying is intentionally loopback-only to keep local web bridge
		// traffic on trusted local endpoints even when URL parsing succeeds.
		if err := validateLoopbackDialTarget(host); err != nil {
			return nil, err
		}
		return dialer.DialContext(ctx, network, addr)
	}
}

func validateLoopbackDialTarget(host string) error {
	return ensureLoopbackHostname(host)
}

func parseLoopbackDevWebURL(raw string) (*url.URL, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("empty URL")
	}
	target, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("parse URL: %w", err)
	}
	if target.Scheme == "" || target.Host == "" {
		return nil, fmt.Errorf("must be an absolute URL")
	}
	if target.User != nil {
		return nil, fmt.Errorf("userinfo is not allowed in %s", EnvDevWebURL)
	}
	if !strings.EqualFold(target.Scheme, "http") && !strings.EqualFold(target.Scheme, "https") {
		return nil, fmt.Errorf("unsupported scheme %q: only http/https allowed", target.Scheme)
	}
	host := strings.ToLower(strings.TrimSpace(target.Hostname()))
	if host == "" {
		return nil, fmt.Errorf("missing hostname")
	}
	if strings.Contains(host, "%") {
		return nil, fmt.Errorf("hostname contains percent-encoding, which is not supported")
	}
	if !isDNSLabelSafe(host) {
		return nil, fmt.Errorf("hostname %q has unsafe characters", host)
	}
	if isLoopbackHostname(host) {
		return target, ensureLoopbackHostname(host)
	}
	parsedIP := net.ParseIP(host)
	if parsedIP != nil && parsedIP.IsLoopback() {
		return target, nil
	}
	return nil, fmt.Errorf("host %q is not loopback-safe", host)
}

func ensureLoopbackHostname(host string) error {
	ips, err := lookupLoopbackHostIPs(host)
	if err != nil {
		return fmt.Errorf("host %q resolve failure: %w", host, err)
	}
	if len(ips) == 0 {
		return fmt.Errorf("host %q has no resolved IP addresses", host)
	}
	for _, ip := range ips {
		if !ip.IsLoopback() {
			return fmt.Errorf("host %q resolves to non-loopback IP %q", host, ip.String())
		}
	}
	return nil
}

func isLoopbackHostname(host string) bool {
	switch strings.TrimSpace(strings.ToLower(host)) {
	case "localhost", "127.0.0.1", "::1", "0:0:0:0:0:0:0:1":
		return true
	default:
		return false
	}
}

func isDNSLabelSafe(host string) bool {
	if host == "" {
		return false
	}
	for _, r := range host {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '-' || r == '.' || r == ':':
		default:
			return false
		}
	}
	return true
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
	_ = json.NewEncoder(w).Encode(struct {
		OK bool `json:"ok"`
		buildinfo.Info
	}{
		OK:   true,
		Info: buildinfo.Current(),
	})
}

func (d *Daemon) handleRPC(w http.ResponseWriter, r *http.Request) {
	body, err := readRequestBody(r, maxRPCRequestBodyBytes)
	if err != nil {
		if errors.Is(err, errRPCRequestBodyTooLarge) {
			http.Error(w, "request body is too large", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "read request body", http.StatusBadRequest)
		return
	}
	if setupStatusRPCMethod(rpcMethod(body)) {
		d.handleSetupStatusRPC(w, r, body)
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
			MemberID: identity.MemberID,
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

func setupStatusRPCMethod(method string) bool {
	switch strings.TrimSpace(method) {
	case "auth.setupStatus", "user.setupStatus":
		return true
	default:
		return false
	}
}

func (d *Daemon) handleSetupStatusRPC(w http.ResponseWriter, r *http.Request, body []byte) {
	var req rpc.Request
	if err := json.Unmarshal(body, &req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(rpc.Response{
			JSONRPC: "2.0",
			Error:   &rpc.Error{Code: rpc.CodeParseError, Message: "parse error"},
		})
		return
	}
	setupOpen := d.setupAvailable(r.Context())
	result := map[string]any{"setupOpen": setupOpen}
	if setupOpen {
		result["setupUrl"] = "/setup?token=" + url.QueryEscape(d.cfg.SetupToken)
	}
	raw, err := json.Marshal(result)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(rpc.Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &rpc.Error{Code: rpc.CodeInternalError, Message: "internal error"},
		})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(rpc.Response{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  raw,
	})
}

func readRequestBody(r *http.Request, maxBytes int64) ([]byte, error) {
	if r == nil || r.Body == nil {
		return nil, nil
	}
	if r.ContentLength > maxBytes {
		return nil, fmt.Errorf("%w: content-length %d exceeds %d", errRPCRequestBodyTooLarge, r.ContentLength, maxBytes)
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBytes+1))
	if err != nil {
		return body, err
	}
	if int64(len(body)) > maxBytes {
		return body, fmt.Errorf("%w: body exceeded %d bytes", errRPCRequestBodyTooLarge, maxBytes)
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	return body, nil
}

func (d *Daemon) handleEvents(w http.ResponseWriter, r *http.Request) {
	if d.events == nil {
		http.Error(w, "events unavailable", http.StatusServiceUnavailable)
		return
	}
	if !checkSameOriginRequest(r) {
		http.Error(w, "cross-origin request blocked", http.StatusForbidden)
		return
	}
	identity, err := d.httpIdentityFromSessionCookie(r.Context(), r)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	projectID := eventProjectID(r)
	if projectID == "" {
		http.Error(w, "projectId is required", http.StatusBadRequest)
		return
	}
	project, err := d.app.ProjectSvc.GetProject(caller.ContextWithCaller(r.Context(), caller.Caller{
		UserID: strings.TrimSpace(identity.UserID),
		Role:   strings.TrimSpace(identity.Role),
	}), types.ProjectID(projectID))
	if err != nil {
		http.Error(w, "project access denied", http.StatusForbidden)
		return
	}
	if strings.TrimSpace(project.UserID()) != strings.TrimSpace(identity.UserID) {
		http.Error(w, "project access denied", http.StatusForbidden)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	events, unregister := d.events.Register(projectID)
	defer unregister()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	_, _ = io.WriteString(w, ": connected\n\n")
	flusher.Flush()

	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-heartbeat.C:
			_, _ = io.WriteString(w, ": heartbeat\n\n")
			flusher.Flush()
		case payload, ok := <-events:
			if !ok {
				return
			}
			_, _ = w.Write([]byte("data: "))
			_, _ = w.Write(payload)
			_, _ = w.Write([]byte("\n\n"))
			flusher.Flush()
		}
	}
}

func checkSameOriginRequest(r *http.Request) bool {
	if r == nil {
		return false
	}
	if r.Host == "" {
		return false
	}
	scheme := "http"
	if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		scheme = "https"
	}
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		origin = deriveOriginFromReferer(r)
	}
	if origin == "" {
		return false
	}
	return strings.EqualFold(origin, scheme+"://"+r.Host)
}

func deriveOriginFromReferer(r *http.Request) string {
	if r == nil {
		return ""
	}
	referer := strings.TrimSpace(r.Header.Get("Referer"))
	if referer == "" {
		return ""
	}
	refererURL, err := url.Parse(referer)
	if err != nil || refererURL.Host == "" {
		return ""
	}
	return refererURL.Scheme + "://" + refererURL.Host
}

func (d *Daemon) mcpSession(token, userID, harnessKind string) mcp.Session {
	return mcp.Session{
		Token:       strings.TrimSpace(token),
		Bootstrap:   false,
		UserID:      strings.TrimSpace(userID),
		HarnessKind: strings.TrimSpace(harnessKind),
		ContextRegistrar: projectMCPContextRegistrar{
			projects: d.app.ProjectSvc,
			users:    d.app.UserSvc,
			auth:     d.app.AuthSvc,
			baseURL:  "http://" + d.cfg.HTTPAddr,
		},
		MemberDirectory: d.app.ProjectSvc,
		MemberRegistrar: d.app.ProjectSvc,
		TaskMembers:     d.app.ProjectSvc,
		DecisionService: d.app.DecisionSvc,
		GraphService:    d.app.GraphSvc,
		CredentialResolver: httpCredentialResolver{
			credentials: d.app.CredentialSvc,
		},
		TaskService:     d.app.TaskSvc,
		TaskFiles:       d.app.FileSvc,
		MissionService:  d.app.MissionSvc,
		MissionKRs:      d.app.MissionSvc,
		MissionProgress: d.app.MissionSvc,
	}
}

func (d *Daemon) resolveMCPSession(ctx context.Context, token string, header http.Header, body []byte) (mcp.Session, error) {
	session, err := d.mcpTokens.Resolve(token)
	if err != nil {
		// Token resolution priority: bootstrap/web-registered tokens (above) →
		// wlt_ link tokens → ak_ API keys. A wlt_ token is recognised first and
		// binds the session to a project server-side; an invalid one fails loudly
		// rather than falling through to the API-key path.
		if strings.HasPrefix(strings.TrimSpace(token), "wlt_") {
			bind, bindErr := d.app.AuthSvc.ValidateLinkToken(ctx, token)
			if bindErr != nil {
				return mcp.Session{}, err
			}
			session = d.mcpSession(token, bind.User.ID.String(), "")
			session.ProjectID = strings.TrimSpace(bind.ProjectID)
		} else {
			account, authErr := d.app.AuthSvc.ValidateAPIKey(ctx, token)
			if authErr != nil {
				return mcp.Session{}, err
			}
			session = d.mcpSession(token, account.ID.String(), "")
		}
	}
	// Prefer the in-band session refs carried in the JSON-RPC body over the
	// transport header. The body is the authoritative per-call identity: Codex
	// self-identifies via params._meta and Claude Code's PreToolUse hook stamps
	// arguments.session_id - both land in the body. The Mcp-Session-Id transport
	// header is only a connection-level memo and can be stale when several
	// concurrent conversations share one token over one connection, so it must not
	// outrank a fresh in-band id. Header remains the fallback for callers that send
	// no in-band refs. This is Codex-safe: Codex sends no Agen8-Native-Session-Id
	// header, so flipping precedence cannot demote a value it relies on.
	bodyRefs := mcp.SessionRequestContextFromJSONRPCBody(body)
	sessionID, threadID := bodyRefs.SessionID, bodyRefs.ThreadID
	if sessionID == "" && threadID == "" {
		sessionID, threadID = mcp.SessionRefsFromHTTPHeader(header)
	}
	// Auto-detect the calling harness from the registration fingerprint when the
	// session carries none yet (a fresh, not-yet-resolved registration). The agent
	// never enters this; it is read from how the client self-identifies in the body.
	// An existing member's persisted HarnessKind is authoritative and is restored
	// onto the session below, overriding this best-effort detection.
	if strings.TrimSpace(session.HarnessKind) == "" {
		nativeRef := sessionID
		if nativeRef == "" {
			nativeRef = threadID
		}
		session.HarnessKind = mcp.HarnessFromJSONRPCBody(body, nativeRef)
	}
	if sessionID == "" && threadID == "" {
		return session, nil
	}
	userID := d.mcpUserID(ctx, token)
	rosterMember, err := d.app.ProjectSvc.ResolveMCPContext(ctx, projectapp.ResolveMCPContextInput{
		Token:     token,
		UserID:    userID,
		ProjectID: session.ProjectID,
		// Resolve harness-agnostically. A shared user-scoped API key or link token can
		// serve more than one harness, so the token's own HarnessKind must not filter
		// the lookup. The (user, native_session_ref) pair identifies the member
		// uniquely, and ResolveMCPContext fails loudly if it ever matches more than
		// one. The resolved member's real HarnessKind is restored onto the session
		// below.
		HarnessKind: "",
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
	auth     *authapp.Service
	baseURL  string
}

func (r projectMCPContextRegistrar) RegisterMCPContext(ctx context.Context, req projecttool.RegisterContextRequest) (projecttool.RegisterContextResult, error) {
	if r.projects == nil {
		return projecttool.RegisterContextResult{}, fmt.Errorf("project service is required")
	}
	result, err := r.projects.RegisterMCPContext(ctx, projectapp.RegisterMCPContextInput{
		Token:            req.Token,
		BoundProjectID:   req.BoundProjectID,
		UserID:           mcpUserID(ctx, r.users, r.auth, req.Token),
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
		ProjectID:         result.ProjectID,
		ProjectRoot:       result.ProjectRoot,
		LocationID:        result.LocationID,
		MemberID:          result.MemberID,
		DisplayName:       result.DisplayName,
		MemberType:        result.MemberType,
		ChannelID:         result.ChannelID,
		SessionID:         result.SessionID,
		ThreadID:          result.ThreadID,
		NativeSessionRef:  result.NativeSessionRef,
		Token:             result.Token,
		URL:               mcpURL,
		MCPServers:        result.MCPServers,
		AlreadyRegistered: result.AlreadyRegistered,
	}, nil
}

func (d *Daemon) mcpUserID(ctx context.Context, token string) string {
	if d == nil || d.app == nil {
		return "local"
	}
	return mcpUserID(ctx, d.app.UserSvc, d.app.AuthSvc, token)
}

func mcpUserID(ctx context.Context, users *userapp.Service, auth *authapp.Service, token string) string {
	if auth != nil {
		if strings.HasPrefix(strings.TrimSpace(token), "wlt_") {
			binding, err := auth.ValidateLinkToken(ctx, token)
			if err == nil && strings.TrimSpace(binding.User.ID.String()) != "" {
				return strings.TrimSpace(binding.User.ID.String())
			}
		}
		record, err := auth.ValidateAPIKey(ctx, token)
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

func eventProjectID(r *http.Request) string {
	projectID := strings.TrimSpace(r.URL.Query().Get("projectId"))
	if projectID != "" {
		return projectID
	}
	ref := strings.TrimSpace(r.Header.Get("Referer"))
	if ref == "" {
		return ""
	}
	u, err := url.Parse(ref)
	if err != nil {
		return ""
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	for i := 0; i+1 < len(parts); i++ {
		if parts[i] == "project" {
			return strings.TrimSpace(parts[i+1])
		}
	}
	return ""
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
	mcpArtifacts, err := setupMCPArtifacts(r, apiKeyResult.Token)
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
	Config             string `json:"config"`
	CodexCommand       string `json:"codexCommand"`
	ClaudeCommand      string `json:"claudeCommand"`
	CodexSkillCommand  string `json:"codexSkillCommand"`
	ClaudeSkillCommand string `json:"claudeSkillCommand"`
}

func setupMCPArtifacts(r *http.Request, token string) (setupMCPResult, error) {
	mcpURL := setupMCPURL(r, token)
	config, err := setupMCPConfig(mcpURL)
	if err != nil {
		return setupMCPResult{}, err
	}
	return setupMCPResult{
		URL:                mcpURL,
		Config:             config,
		CodexCommand:       "codex mcp add agen8 --url " + shellQuote(mcpURL),
		ClaudeCommand:      "claude mcp add --transport http --scope user agen8 " + shellQuote(mcpURL),
		CodexSkillCommand:  "agen8 skill install --harness codex",
		ClaudeSkillCommand: "agen8 skill install --harness claude-cli",
	}, nil
}

func setupMCPURL(r *http.Request, token string) string {
	return setupRequestOrigin(r) + "/mcp?token=" + url.QueryEscape(token)
}

func setupRequestOrigin(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	host := strings.TrimSpace(r.Host)
	if host == "" {
		host = strings.TrimSpace(r.URL.Host)
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
			"agen8": map[string]string{
				"type": "http",
				"url":  mcpURL,
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
                      <p class="snippet-hint">Use this if a client asks for a server URL.</p>
                    </div>
                    <button class="secondary" type="button" data-copy-target="mcp-url">Copy</button>
                  </div>
                  <code id="mcp-url"></code>
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
