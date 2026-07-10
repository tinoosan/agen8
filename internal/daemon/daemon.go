package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/tinoosan/agen8/internal/app"
	"github.com/tinoosan/agen8/internal/caller"
	"github.com/tinoosan/agen8/internal/core/types"
	"github.com/tinoosan/agen8/internal/logging"
	"github.com/tinoosan/agen8/internal/mcp"
	"github.com/tinoosan/agen8/internal/rpc"
	"github.com/tinoosan/agen8/internal/services/attention"
	projectrpc "github.com/tinoosan/agen8/internal/services/project/rpc"
	"github.com/tinoosan/agen8/internal/web"
	"github.com/tinoosan/agen8/pkg/buildinfo"
)

type loopbackIPLookup func(string) ([]net.IP, error)

type Daemon struct {
	cfg          Config
	app          *app.Application
	rpc          *rpc.Server
	mcpTokens    *mcp.TokenStore
	mcp          *mcp.Server
	mcpResolver  *mcpSessionResolver
	events       *eventsHub
	attention    *attention.Service
	loginLimiter *loginAttemptLimiter
	logger       *slog.Logger
}

const maxRPCRequestBodyBytes = 1024 * 1024

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
		projectDirLookup{projects: application.ProjectSvc},
		application.EventBus,
		nil,
		attention.DefaultTTL,
		logger.With("service", "attention"),
	)
	if err != nil {
		return nil, fmt.Errorf("build attention service: %w", err)
	}
	var projectProvisioner *projectHooksProvisioner
	if !cfg.DisableLocalHookProvisioning {
		projectProvisioner = newProjectHooksProvisionerWithBaseURL(application.AuthSvc, cfg.externalBaseURL(), logger.With("service", "hooks-provision"))
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
		func() error {
			var postCreate rpc.PostProjectCreate
			var configureClaudeMCP rpc.ConfigureProjectClaudeMCP
			if projectProvisioner != nil {
				postCreate = projectProvisioner.ProvisionHooks
				configureClaudeMCP = func(ctx context.Context, userID, projectTitle, root string) (projectrpc.ProjectClaudeMCPConfigureResult, error) {
					result, err := projectProvisioner.ProvisionClaudeMCP(ctx, userID, projectTitle, root)
					if err != nil {
						return projectrpc.ProjectClaudeMCPConfigureResult{}, err
					}
					return projectrpc.ProjectClaudeMCPConfigureResult{
						Installed:  result.Installed,
						Path:       result.Path,
						ServerName: result.ServerName,
						URL:        result.URL,
					}, nil
				}
			}
			return rpc.RegisterProject(reg, application.ProjectSvc, postCreate, configureClaudeMCP)
		},
		func() error { return rpc.RegisterFile(reg, application.FileSvc) },
		func() error { return rpc.RegisterLocation(reg, application.LocationSvc) },
		func() error { return rpc.RegisterPin(reg, application.PinSvc) },
		func() error { return rpc.RegisterNotification(reg, application.NotificationSvc) },
		func() error { return rpc.RegisterAttention(reg, attentionSvc) },
		func() error { return rpc.RegisterLastSeen(reg, application.LastSeenStore) },
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
		mcpResolver: newMCPSessionResolver(mcpSessionResolverConfig{
			tokenStore:         tokenStore,
			auth:               application.AuthSvc,
			users:              application.UserSvc,
			projects:           application.ProjectSvc,
			decisions:          application.DecisionSvc,
			graph:              application.GraphSvc,
			credentials:        application.CredentialSvc,
			tasks:              application.TaskSvc,
			files:              application.FileSvc,
			missions:           application.MissionSvc,
			projectProvisioner: projectProvisioner,
		}),
		events:       newEventsHub(application.EventBus, logger.With("service", "events")),
		attention:    attentionSvc,
		loginLimiter: newLoginAttemptLimiter(),
		logger:       logger.With("service", "daemon"),
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
			fmt.Fprintf(d.cfg.Out, "agen8 setup: %s/setup?token=%s\n", d.cfg.externalBaseURL(), d.cfg.SetupToken)
		}
		fmt.Fprintf(d.cfg.Out, "agen8 daemon listening on http://%s\n", ln.Addr().String())
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
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
	mux.HandleFunc("POST /uploads/files", d.handleFileUpload)
	mux.Handle("/mcp", d.mcp.Handler())
	mux.HandleFunc("GET /events", d.handleEvents)
	mux.HandleFunc("POST /hooks/attention", d.handleAttentionHook)
	setup := d.setupHandler()
	mux.HandleFunc("GET /setup", setup.handlePage)
	mux.HandleFunc("POST /setup", setup.handleCreate)
	webHandler, err := d.webHandler()
	if err != nil {
		return nil, fmt.Errorf("mount web ui: %w", err)
	}
	mux.Handle("/", webHandler)
	return securityHeaders(mux), nil
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
	// #nosec G704 -- target was produced by parseLoopbackDevWebURL, which enforces
	// absolute http(s) URL, disallows userinfo/unsafe host encoding, and requires
	// loopback-only host/IP targets; transport dial-time checks re-validate each host.
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
	method := rpcMethod(body)
	if setupStatusRPCMethod(method) {
		d.handleSetupStatusRPC(w, r, body)
		return
	}
	usesSessionCookie := strings.TrimSpace(r.Header.Get("Authorization")) == "" && requestHasSessionCookie(r)
	if usesSessionCookie && !checkSameOriginRequest(r) {
		http.Error(w, "cross-origin request blocked", http.StatusForbidden)
		return
	}
	if method == rpc.MethodAuthLogout && strings.TrimSpace(r.Header.Get("Authorization")) == "" {
		d.handleCookieLogoutRPC(w, r, body)
		return
	}
	loginKey := ""
	if method == rpc.MethodAuthLogin {
		loginKey = loginAttemptKey(body)
		if retryAfter, allowed := d.loginLimiter.Allow(loginKey); !allowed {
			w.Header().Set("Retry-After", strconv.Itoa(int(retryAfter.Round(time.Second)/time.Second)))
			http.Error(w, "too many login attempts", http.StatusTooManyRequests)
			return
		}
	}
	ctx := r.Context()
	if methodRequiresHTTPIdentity(method, r.Header.Get("Authorization"), requestHasSessionCookie(r)) {
		identity, err := d.httpIdentityFromRequest(r.Context(), r)
		if err != nil {
			if usesSessionCookie {
				d.clearSessionCookie(w, r)
				if method == rpc.MethodAuthStatus {
					identity = rpc.Identity{}
				} else {
					http.Error(w, "unauthorized", http.StatusUnauthorized)
					return
				}
			} else {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
		}
		if strings.TrimSpace(identity.UserID) != "" {
			ctx = rpc.ContextWithIdentity(ctx, identity)
			ctx = caller.ContextWithCaller(ctx, caller.Caller{
				UserID:   identity.UserID,
				MemberID: identity.MemberID,
				Role:     identity.Role,
			})
		}
	}
	resp, err := d.rpc.Handle(ctx, body)
	if err != nil {
		http.Error(w, "handle rpc request", http.StatusInternalServerError)
		return
	}
	if method == rpc.MethodAuthLogin {
		if rpcResponseSucceeded(resp) {
			sanitized, token, expiresAt, sanitizedOK := sanitizeLoginResponse(resp)
			if !sanitizedOK {
				http.Error(w, "handle login response", http.StatusInternalServerError)
				return
			}
			d.loginLimiter.Reset(loginKey)
			d.setSessionCookie(w, r, token, expiresAt)
			resp = sanitized
		} else {
			d.loginLimiter.RecordFailure(loginKey)
		}
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

func (c Config) externalBaseURL() string {
	publicURL := strings.TrimRight(strings.TrimSpace(c.PublicURL), "/")
	if publicURL != "" {
		return publicURL
	}
	return daemonBaseURL(c.HTTPAddr)
}

func (d *Daemon) resolveMCPSession(ctx context.Context, token string, header http.Header, body []byte) (mcp.Session, error) {
	if d == nil || d.mcpResolver == nil {
		return mcp.Session{}, fmt.Errorf("mcp session resolver is not configured")
	}
	return d.mcpResolver.Resolve(ctx, token, header, body)
}

type rpcEnvelope struct {
	Method string `json:"method"`
}

func rpcMethod(body []byte) string {
	var env rpcEnvelope
	_ = json.Unmarshal(body, &env)
	return strings.TrimSpace(env.Method)
}

func methodRequiresHTTPIdentity(method string, authorization string, hasSessionCookie bool) bool {
	method = strings.TrimSpace(method)
	switch method {
	case rpc.MethodAuthLogin:
		return false
	case rpc.MethodAuthStatus:
		return strings.TrimSpace(authorization) != "" || hasSessionCookie
	default:
		return true
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
