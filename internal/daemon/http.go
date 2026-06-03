package daemon

import (
	"context"
	"crypto/sha1"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tinoosan/agen8-mcp-server/internal/caller"
	mcpspace "github.com/tinoosan/agen8-mcp-server/internal/mcp/tools/space"
	"github.com/tinoosan/agen8-mcp-server/internal/rpc"
	authapp "github.com/tinoosan/agen8-mcp-server/internal/services/auth/app"
	harnessapp "github.com/tinoosan/agen8-mcp-server/internal/services/harness/app"
	humaninputdomain "github.com/tinoosan/agen8-mcp-server/internal/services/humaninput/domain"
	messageapp "github.com/tinoosan/agen8-mcp-server/internal/services/message/app"
	"github.com/tinoosan/agen8-mcp-server/internal/services/message/domain/conversation"
	projectapp "github.com/tinoosan/agen8-mcp-server/internal/services/project/app"
	projectdomain "github.com/tinoosan/agen8-mcp-server/internal/services/project/domain/project"
	spaceapp "github.com/tinoosan/agen8-mcp-server/internal/services/space/app"
	spacedomain "github.com/tinoosan/agen8-mcp-server/internal/services/space/domain"
	"github.com/tinoosan/agen8-mcp-server/internal/services/space/domain/member"
	userapp "github.com/tinoosan/agen8-mcp-server/internal/services/user/app"
	user "github.com/tinoosan/agen8-mcp-server/internal/services/user/domain"
	"github.com/tinoosan/agen8-mcp-server/internal/web"
	"github.com/tinoosan/agen8-mcp-server/pkg/protocol"
	"github.com/tinoosan/agen8-mcp-server/pkg/types"
)

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
	srv := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}
	if d.cfg.Out != nil {
		if d.setupAvailable(ctx) {
			fmt.Fprintf(d.cfg.Out, "agen8 setup: http://%s/ (setup token omitted from logs)\n", ln.Addr().String())
		}
		fmt.Fprintf(d.cfg.Out, "agen8 daemon listening on http://%s\n", ln.Addr().String())
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()
	err = srv.Serve(ln)
	if err == nil || err == http.ErrServerClosed || ctx.Err() != nil {
		return nil
	}
	return fmt.Errorf("serve http daemon: %w", err)
}

func (d *Daemon) httpHandler() (http.Handler, error) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", d.handleHealthz)
	mux.HandleFunc("POST /rpc", d.handleRPC)
	mux.HandleFunc("POST /mcp/register", d.handleMCPRegister)
	if d.mcp == nil {
		return nil, fmt.Errorf("mcp server is required")
	}
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
	method := rpcMethod(body)
	if methodRequiresHTTPIdentity(method, r.Header.Get("Authorization")) {
		identity, err := d.httpIdentity(r.Context(), r.Header.Get("Authorization"))
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		d.rememberLocalUser(identity.UserID)
		ctx = rpc.ContextWithIdentity(ctx, identity)
	}
	resp, err := d.rpc.Handle(ctx, body)
	if err != nil {
		http.Error(w, "handle rpc request", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(resp)
}

type mcpRegisterRequest struct {
	ProjectID      string `json:"projectId"`
	ProjectRoot    string `json:"projectRoot"`
	LocationID     string `json:"locationId"`
	SpaceID        string `json:"spaceId"`
	MemberID       string `json:"memberId"`
	DisplayName    string `json:"displayName"`
	MemberType     string `json:"memberType"`
	HarnessKind    string `json:"harnessKind"`
	SessionID      string `json:"sessionId"`
	ThreadID       string `json:"threadId"`
	Model          string `json:"model"`
	Effort         string `json:"effort"`
	PermissionMode string `json:"permissionMode"`
	ConfigRef      string `json:"configRef"`
}

type mcpRegisterResponse struct {
	ProjectID   string   `json:"projectId"`
	ProjectRoot string   `json:"projectRoot"`
	LocationID  string   `json:"locationId"`
	SpaceID     string   `json:"spaceId"`
	MemberID    string   `json:"memberId"`
	MemberType  string   `json:"memberType"`
	ChannelID   string   `json:"channelId"`
	Token       string   `json:"token"`
	URL         string   `json:"url"`
	MCPServers  []string `json:"mcpServers"`
}

func (d *Daemon) handleMCPRegister(w http.ResponseWriter, r *http.Request) {
	identity, err := d.httpIdentity(r.Context(), r.Header.Get("Authorization"))
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	d.rememberLocalUser(identity.UserID)
	var req mcpRegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	resp, err := d.registerExternalMCPHarness(r.Context(), identity, req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (d *Daemon) registerExternalMCPHarness(ctx context.Context, identity rpc.Identity, req mcpRegisterRequest) (mcpRegisterResponse, error) {
	userID := strings.TrimSpace(identity.UserID)
	if userID == "" {
		return mcpRegisterResponse{}, fmt.Errorf("user identity is required")
	}
	return d.registerExternalMCPHarnessForUser(ctx, userID, req, "")
}

func (d *Daemon) RegisterMCPContext(ctx context.Context, req mcpspace.RegisterContextRequest) (mcpspace.RegisterContextResult, error) {
	userID := d.currentLocalUser()
	if userID == "" && d != nil && d.app != nil && d.app.UserSvc != nil {
		user, err := d.app.UserSvc.FirstActive(ctx)
		if err == nil && !user.ID.IsZero() {
			userID = user.ID.String()
		}
	}
	if strings.TrimSpace(userID) == "" {
		userID = "local-user"
	}
	out, err := d.registerExternalMCPHarnessForUser(ctx, userID, mcpRegisterRequest{
		ProjectID:   req.ProjectID,
		ProjectRoot: req.ProjectRoot,
		LocationID:  req.LocationID,
		SpaceID:     req.SpaceID,
		DisplayName: req.DisplayName,
		HarnessKind: req.HarnessKind,
		SessionID:   req.SessionID,
		ThreadID:    req.ThreadID,
	}, req.Token)
	if err != nil {
		return mcpspace.RegisterContextResult{}, err
	}
	return mcpspace.RegisterContextResult{
		ProjectID:   out.ProjectID,
		ProjectRoot: out.ProjectRoot,
		LocationID:  out.LocationID,
		SpaceID:     out.SpaceID,
		MemberID:    out.MemberID,
		MemberType:  out.MemberType,
		ChannelID:   out.ChannelID,
		Token:       out.Token,
		URL:         out.URL,
	}, nil
}

func (d *Daemon) registerExternalMCPHarnessForUser(ctx context.Context, userID string, req mcpRegisterRequest, tokenOverride string) (mcpRegisterResponse, error) {
	if d == nil || d.app == nil {
		return mcpRegisterResponse{}, fmt.Errorf("daemon application is required")
	}
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return mcpRegisterResponse{}, fmt.Errorf("user identity is required")
	}
	harnessKind := strings.TrimSpace(req.HarnessKind)
	if harnessKind == "" {
		harnessKind = "codex"
	}
	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = d.app.HarnessSvc.DefaultModel(harnessKind)
	}
	effort := strings.TrimSpace(req.Effort)
	if effort == "" {
		effort = "medium"
	}
	project, err := d.resolveMCPRegisterProject(ctx, req)
	if err != nil {
		return mcpRegisterResponse{}, err
	}
	projectID := string(project.ID())
	callCtx := caller.ContextWithCaller(ctx, caller.Caller{UserID: userID})
	space, err := d.resolveMCPRegisterSpace(callCtx, userID, projectID, strings.TrimSpace(req.SpaceID))
	if err != nil {
		return mcpRegisterResponse{}, err
	}
	sessionKey := mcpRegisterSessionKey(req)
	memberID := strings.TrimSpace(req.MemberID)
	if memberID == "" && sessionKey == "" {
		memberID, err = d.resolveExistingMCPRegisterMemberID(callCtx, userID, projectID, string(space.ID), harnessKind)
		if err != nil {
			return mcpRegisterResponse{}, err
		}
	}
	if memberID == "" {
		identityKey := harnessKind
		if sessionKey != "" {
			identityKey = harnessKind + "\x00" + sessionKey
		}
		memberID = "member-" + shortHash(projectID+"\x00"+userID+"\x00"+identityKey)
	}
	memberType, err := d.resolveMCPRegisterMemberType(callCtx, string(space.ID), memberID, strings.TrimSpace(req.MemberType))
	if err != nil {
		return mcpRegisterResponse{}, err
	}
	channelID := "channel:" + string(space.ID) + ":member:" + memberID
	displayName := strings.TrimSpace(req.DisplayName)
	if displayName == "" {
		displayName = strings.TrimSpace(harnessKind)
	}
	rosterMember, err := d.app.SpaceSvc.UpsertExternalHarnessMember(callCtx, spaceapp.UpsertExternalHarnessMemberParams{
		ID:             member.ID(memberID),
		UserID:         userID,
		ProjectID:      projectID,
		SpaceID:        string(space.ID),
		ChannelID:      channelID,
		DisplayName:    displayName,
		MemberType:     memberType,
		HarnessKind:    harnessKind,
		Model:          model,
		Effort:         effort,
		PermissionMode: strings.TrimSpace(req.PermissionMode),
		ConfigRef:      strings.TrimSpace(req.ConfigRef),
	})
	if err != nil {
		return mcpRegisterResponse{}, err
	}
	active, err := d.app.HarnessSvc.GetActiveSession(ctx, string(rosterMember.ID))
	if err != nil {
		return mcpRegisterResponse{}, fmt.Errorf("load active harness session: %w", err)
	}
	activation := harnessapp.ActivateSessionParams{
		ProjectID:      projectID,
		MemberID:       string(rosterMember.ID),
		SpaceID:        rosterMember.SpaceID,
		ChannelID:      rosterMember.ChannelID,
		DisplayName:    rosterMember.DisplayName,
		MemberType:     rosterMember.MemberType,
		LifecycleState: rosterMember.LifecycleState,
		HarnessKind:    rosterMember.HarnessKind,
		Model:          rosterMember.Model,
		Effort:         rosterMember.Effort,
		PermissionMode: rosterMember.PermissionMode,
		ConfigRef:      rosterMember.ConfigRef,
		SessionRef:     sessionKey,
		MCPToken:       strings.TrimSpace(tokenOverride),
	}
	if active == nil {
		active, err = d.app.HarnessSvc.ActivateSession(ctx, activation)
	} else {
		active, err = d.app.HarnessSvc.UpdateSessionRuntimeContext(ctx, active.ID, activation)
	}
	if err != nil {
		return mcpRegisterResponse{}, fmt.Errorf("activate external harness session: %w", err)
	}
	if err := d.registerMCPTokenForSession(active); err != nil {
		return mcpRegisterResponse{}, err
	}
	if err := d.app.MessageSvc.StartAgentDelivery(ctx, member.ID(active.MemberID)); err != nil {
		return mcpRegisterResponse{}, fmt.Errorf("start message delivery for member %s: %w", active.MemberID, err)
	}
	return mcpRegisterResponse{
		ProjectID:   projectID,
		ProjectRoot: project.Root(),
		LocationID:  string(project.LocationID()),
		SpaceID:     active.SpaceID,
		MemberID:    active.MemberID,
		MemberType:  active.MemberType,
		ChannelID:   active.ChannelID,
		Token:       active.MCPToken,
		URL:         d.mcpURL(active.MCPToken),
		MCPServers:  append([]string(nil), active.MCPServers...),
	}, nil
}

func (d *Daemon) resolveExistingMCPRegisterMemberID(ctx context.Context, userID string, projectID string, spaceID string, harnessKind string) (string, error) {
	if d == nil || d.app == nil || d.app.SpaceSvc == nil {
		return "", fmt.Errorf("space service is required")
	}
	members, err := d.app.SpaceSvc.ListMembers(ctx, member.Filter{
		SpaceID:        strings.TrimSpace(spaceID),
		ProjectID:      strings.TrimSpace(projectID),
		UserID:         strings.TrimSpace(userID),
		LifecycleState: member.LifecycleActive,
		Limit:          50,
	})
	if err != nil {
		return "", fmt.Errorf("list existing mcp members: %w", err)
	}
	for _, candidate := range members {
		if strings.EqualFold(strings.TrimSpace(candidate.HarnessKind), strings.TrimSpace(harnessKind)) &&
			member.IsCoordinatorType(candidate.MemberType) &&
			strings.TrimSpace(candidate.ID.String()) != "" {
			return candidate.ID.String(), nil
		}
	}
	for _, candidate := range members {
		if strings.EqualFold(strings.TrimSpace(candidate.HarnessKind), strings.TrimSpace(harnessKind)) &&
			strings.TrimSpace(candidate.ID.String()) != "" {
			return candidate.ID.String(), nil
		}
	}
	return "", nil
}

func (d *Daemon) resolveMCPRegisterMemberType(ctx context.Context, spaceID string, memberID string, requested string) (string, error) {
	requested = strings.TrimSpace(requested)
	if requested != "" {
		return requested, nil
	}
	memberID = strings.TrimSpace(memberID)
	var existingMemberType string
	if memberID != "" {
		existing, err := d.app.SpaceSvc.GetMember(ctx, member.ID(memberID))
		if err == nil && strings.TrimSpace(existing.MemberType) != "" {
			existingMemberType = strings.TrimSpace(existing.MemberType)
			if member.IsCoordinatorType(existingMemberType) {
				return existingMemberType, nil
			}
		}
		if err != nil && !errors.Is(err, member.ErrNotFound) {
			return "", fmt.Errorf("load existing member for role assignment: %w", err)
		}
	}
	members, err := d.app.SpaceSvc.ListMembers(ctx, member.Filter{
		SpaceID:        strings.TrimSpace(spaceID),
		LifecycleState: member.LifecycleActive,
		Limit:          500,
	})
	if err != nil {
		return "", fmt.Errorf("list space members for role assignment: %w", err)
	}
	for _, candidate := range members {
		if member.IsCoordinatorType(candidate.MemberType) {
			if existingMemberType != "" {
				return existingMemberType, nil
			}
			return member.TypeWorker, nil
		}
	}
	return member.TypeCoordinator, nil
}

func mcpRegisterSessionKey(req mcpRegisterRequest) string {
	for _, value := range []string{req.ThreadID, req.SessionID} {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func (d *Daemon) resolveMCPRegisterProject(ctx context.Context, req mcpRegisterRequest) (projectdomain.Project, error) {
	locationID := types.LocationID(strings.TrimSpace(req.LocationID))
	if locationID == "" {
		locationID = "local"
	}
	projectID := types.ProjectID(strings.TrimSpace(req.ProjectID))
	root := strings.TrimSpace(req.ProjectRoot)
	if projectID == "" {
		if root == "" {
			return projectdomain.Project{}, fmt.Errorf("projectId or projectRoot is required")
		}
		projectID = projectapp.ProjectIDForLocationRoot(locationID, root)
	}
	project, err := d.app.ProjectSvc.GetProject(ctx, projectID)
	if err == nil {
		return project, nil
	}
	if !errors.Is(err, projectdomain.ErrNotFound) {
		return projectdomain.Project{}, fmt.Errorf("load project: %w", err)
	}
	if root == "" {
		return projectdomain.Project{}, fmt.Errorf("project %s not found", projectID)
	}
	return d.app.ProjectSvc.SaveProject(ctx, projectapp.SaveProjectInput{
		ID:         projectID,
		LocationID: locationID,
		Root:       root,
		Title:      filepath.Base(filepath.Clean(root)),
		Status:     projectdomain.StatusOpen,
	})
}

func (d *Daemon) resolveMCPRegisterSpace(ctx context.Context, userID string, projectID string, requestedSpaceID string) (spacedomain.SpaceRecord, error) {
	if requestedSpaceID != "" {
		space, err := d.app.SpaceSvc.Get(ctx, spacedomain.SpaceID(requestedSpaceID))
		if err != nil {
			return spacedomain.SpaceRecord{}, err
		}
		if strings.TrimSpace(space.ProjectID) != projectID {
			return spacedomain.SpaceRecord{}, fmt.Errorf("space %s is not in project %s", requestedSpaceID, projectID)
		}
		return space, nil
	}
	spaces, err := d.app.SpaceSvc.List(ctx, spacedomain.SpaceFilter{ProjectID: projectID, Status: spacedomain.SpaceStatusOpen, Limit: 1})
	if err != nil {
		return spacedomain.SpaceRecord{}, err
	}
	if len(spaces) > 0 {
		return spaces[0], nil
	}
	return d.app.SpaceSvc.Create(ctx, spacedomain.SpaceRecord{
		ID:        spacedomain.SpaceID("space-" + shortHash(projectID+"\x00mcp")),
		UserID:    userID,
		ProjectID: projectID,
		Title:     "MCP Work Context",
		Status:    spacedomain.SpaceStatusOpen,
	})
}

func shortHash(value string) string {
	sum := sha1.Sum([]byte(value))
	return fmt.Sprintf("%x", sum[:])[:12]
}

func rpcMethod(body []byte) string {
	var req struct {
		Method string `json:"method"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return ""
	}
	return strings.TrimSpace(req.Method)
}

func methodRequiresHTTPIdentity(method string, authorization string) bool {
	switch method {
	case rpc.MethodAuthLogin:
		return false
	case rpc.MethodAuthStatus:
		return bearerToken(authorization) != ""
	}
	return true
}

func (d *Daemon) handleEvents(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	ch, cancel := d.events.Subscribe("*")
	defer cancel()
	_, _ = w.Write([]byte(": connected\n\n"))
	flusher.Flush()
	for {
		select {
		case <-r.Context().Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			body, err := json.Marshal(msg)
			if err != nil {
				continue
			}
			if _, err := w.Write([]byte("data: ")); err != nil {
				return
			}
			if _, err := w.Write(body); err != nil {
				return
			}
			if _, err := w.Write([]byte("\n\n")); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

var _ messageapp.ConversationNotifier = (*Daemon)(nil)
var _ interface {
	NotifyHumanInputChanged(context.Context, humaninputdomain.Request) error
} = (*Daemon)(nil)

func (d *Daemon) NotifyConversationChanged(_ context.Context, message conversation.Message) error {
	if d == nil || d.events == nil {
		return fmt.Errorf("daemon event stream is required")
	}
	params := map[string]any{
		"messageId": message.ID,
		"channelId": message.ChannelID,
		"spaceId":   message.SpaceID,
		"memberId":  message.MemberID,
		"sessionId": message.SessionID,
		"turnId":    message.TurnID,
		"direction": string(message.Direction),
		"render":    string(message.Render),
		"updatedAt": message.UpdatedAt,
	}
	legacy, err := protocol.NewNotification(protocol.MethodNotifyEventAppend, params)
	if err != nil {
		return err
	}
	d.events.Notify(legacy, func(string) bool { return true })
	return nil
}

func (d *Daemon) NotifyHumanInputChanged(_ context.Context, req humaninputdomain.Request) error {
	if d == nil || d.events == nil {
		return fmt.Errorf("daemon event stream is required")
	}
	notification, err := protocol.NewNotification(protocol.NotifyChannelHumanInputChanged, map[string]any{
		"requestId":  string(req.ID),
		"channelId":  req.ChannelID,
		"spaceId":    req.SpaceID,
		"memberId":   req.AskerMemberID,
		"toolCallId": string(req.ToolCallID),
		"toolName":   req.ToolName,
		"status":     string(req.Status),
		"projectId":  req.ProjectID,
	})
	if err != nil {
		return err
	}
	d.events.Notify(notification, func(string) bool { return true })
	return nil
}

func (d *Daemon) httpIdentity(ctx context.Context, header string) (rpc.Identity, error) {
	token := bearerToken(header)
	if token == "" {
		return rpc.Identity{}, fmt.Errorf("bearer token is required")
	}
	account, err := d.validateBearer(ctx, token)
	if err != nil {
		return rpc.Identity{}, err
	}
	return rpc.Identity{
		UserID: account.ID.String(),
		Role:   string(account.Role),
	}, nil
}

func (d *Daemon) rememberLocalUser(userID string) {
	if d == nil {
		return
	}
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return
	}
	d.identity.mu.Lock()
	d.identity.userID = userID
	d.identity.mu.Unlock()
}

func (d *Daemon) currentLocalUser() string {
	if d == nil {
		return ""
	}
	d.identity.mu.RLock()
	userID := d.identity.userID
	d.identity.mu.RUnlock()
	return strings.TrimSpace(userID)
}

func (d *Daemon) validateBearer(ctx context.Context, token string) (user.User, error) {
	switch {
	case strings.HasPrefix(token, "ses_"):
		return d.app.AuthSvc.ValidateSession(ctx, token)
	case strings.HasPrefix(token, "ak_"):
		return d.app.AuthSvc.ValidateAPIKey(ctx, token)
	default:
		return user.User{}, fmt.Errorf("unsupported bearer token")
	}
}

func bearerToken(header string) string {
	header = strings.TrimSpace(header)
	if header == "" {
		return ""
	}
	prefix := "Bearer "
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
	created, err := d.app.UserSvc.SetupFirstUser(r.Context(), userapp.SetupFirstUserParams{
		Email: req.Email,
		Name:  req.Name,
	})
	if err != nil {
		http.Error(w, "create setup user", http.StatusBadRequest)
		return
	}
	if err := d.app.AuthSvc.CreatePassword(r.Context(), authapp.CreatePasswordParams{
		UserID:   created.User.ID,
		Password: req.Password,
	}); err != nil {
		http.Error(w, "create setup credential", http.StatusInternalServerError)
		return
	}
	sessionResult, err := d.app.AuthSvc.CreateSession(r.Context(), authapp.CreateSessionParams{
		UserID: created.User.ID,
	})
	if err != nil {
		http.Error(w, "create setup session", http.StatusInternalServerError)
		return
	}
	keyName := strings.TrimSpace(req.KeyName)
	if keyName == "" {
		keyName = "initial daemon key"
	}
	apiKeyResult, err := d.app.AuthSvc.CreateAPIKey(r.Context(), authapp.CreateAPIKeyParams{
		UserID: created.User.ID,
		Name:   keyName,
	})
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
		"user": map[string]any{
			"id":    created.User.ID.String(),
			"email": created.User.Email,
			"name":  created.User.Name,
			"role":  string(created.User.Role),
		},
		"session": map[string]any{
			"token":     sessionResult.Token,
			"expiresAt": sessionResult.Session.ExpiresAt,
		},
		"apiKey": map[string]any{
			"id":     apiKeyResult.APIKey.ID.String(),
			"name":   apiKeyResult.APIKey.Name,
			"prefix": apiKeyResult.APIKey.Prefix,
			"secret": apiKeyResult.Token,
		},
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
	contentType := strings.ToLower(r.Header.Get("Content-Type"))
	if strings.Contains(contentType, "application/json") {
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
    .note {
      margin-top: 18px;
      padding: 12px;
      border: 1px solid var(--border);
      border-radius: var(--r-md);
      background: var(--bg-surface);
      color: var(--text-3);
      font-size: 12px;
      line-height: 1.5;
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
          <h1 id="setup-title">Bring your agents into one place.</h1>
          <p class="lead">agen8 gives agents a shared workspace for missions, plans, tasks, decisions, and messages. Create your account to open the workspace and start coordinating work.</p>
        </div>
        <div class="steps" aria-label="Setup outcome">
          <div class="step"><span class="badge">1</span><span>Create your sign-in account.</span></div>
          <div class="step"><span class="badge">2</span><span>Open agen8 and create your first space.</span></div>
          <div class="step"><span class="badge">3</span><span>Start assigning work to agents.</span></div>
        </div>
      </section>
      <section class="panel" aria-label="Create account">
        <h2>Create your account</h2>
        <p class="hint">Use these details to sign in to this agen8 workspace.</p>
        <form method="post" action="/setup">
          <input type="hidden" name="token" value="{{TOKEN}}">
          <label>Email<input name="email" type="email" autocomplete="email" required></label>
          <label>Name<input name="name" autocomplete="name" required></label>
          <label>Password<input name="password" type="password" autocomplete="new-password" minlength="8" required></label>
          <button type="submit">Enter agen8</button>
        </form>
      </section>
    </div>
  </main>
</body>
</html>`
