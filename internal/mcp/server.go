package mcp

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	spacedomain "github.com/tinoosan/agen8-mcp-server/internal/services/space/domain"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	decisiontool "github.com/tinoosan/agen8-mcp-server/internal/mcp/tools/decision"
	graphtool "github.com/tinoosan/agen8-mcp-server/internal/mcp/tools/graph"
	harnessapprovaltool "github.com/tinoosan/agen8-mcp-server/internal/mcp/tools/harnessapproval"
	httptool "github.com/tinoosan/agen8-mcp-server/internal/mcp/tools/http"
	messagetool "github.com/tinoosan/agen8-mcp-server/internal/mcp/tools/message"
	missiontool "github.com/tinoosan/agen8-mcp-server/internal/mcp/tools/mission"
	operatortool "github.com/tinoosan/agen8-mcp-server/internal/mcp/tools/operator"
	scheduletool "github.com/tinoosan/agen8-mcp-server/internal/mcp/tools/schedule"
	"github.com/tinoosan/agen8-mcp-server/internal/mcp/tools/space"
	tasktool "github.com/tinoosan/agen8-mcp-server/internal/mcp/tools/task"
	humaninput "github.com/tinoosan/agen8-mcp-server/internal/services/humaninput/domain"
	"github.com/tinoosan/agen8-mcp-server/pkg/types"
)

type HumanInputAwaiter interface {
	Await(context.Context, humaninput.PendingRequest) (json.RawMessage, error)
}

type Session struct {
	Token              string
	Bootstrap          bool
	UserID             string
	ChannelID          types.ChannelID
	SpaceID            spacedomain.SpaceID
	MemberID           string
	HarnessKind        string
	ContextRegistrar   space.ContextRegistrar
	SpaceSetup         space.SetupService
	SpaceReader        space.SpaceService
	MemberDirectory    space.MemberService
	MemberRegistrar    space.MemberRegistrar
	TaskMembers        tasktool.MemberDirectory
	MessagePublisher   messagetool.MessagePublisher
	DecisionService    decisiontool.Service
	GraphService       graphtool.Service
	HumanInputAwaiter  HumanInputAwaiter
	CredentialResolver httptool.CredentialResolver
	TaskService        tasktool.Service
	ScheduleService    scheduletool.Service
	OperatorService    operatortool.Service
	MissionService     missiontool.MissionLifecycleService
	MissionKRs         missiontool.KeyResultService
	MissionProgress    missiontool.ProgressService
	ProjectID          string
}

type SessionRequestContext struct {
	SessionID string
	ThreadID  string
	TurnID    string
}

type TokenStore struct {
	mu       sync.RWMutex
	sessions map[string]Session
}

func NewTokenStore() *TokenStore {
	return &TokenStore{sessions: map[string]Session{}}
}

func (s *TokenStore) Register(token string, session Session) {
	if s == nil {
		panic("mcp token store is required")
	}
	token = strings.TrimSpace(token)
	if token == "" {
		panic("mcp token must not be empty")
	}
	// Note: ChannelID, SpaceID, and MemberID are required when an
	// agent invokes a tool that needs member/session identity. They are
	// validated by native tool handlers rather than at token registration
	// so tool list and narrow dispatch tests can register a minimal session.
	s.mu.Lock()
	s.sessions[token] = session
	s.mu.Unlock()
}

func (s *TokenStore) Revoke(token string) {
	if s == nil {
		panic("mcp token store is required")
	}
	token = strings.TrimSpace(token)
	if token == "" {
		panic("mcp token must not be empty")
	}
	s.mu.Lock()
	delete(s.sessions, token)
	s.mu.Unlock()
}

func (s *TokenStore) Resolve(token string) (Session, error) {
	if s == nil {
		return Session{}, fmt.Errorf("mcp token store is required")
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return Session{}, fmt.Errorf("mcp token is required")
	}
	s.mu.RLock()
	session, ok := s.sessions[token]
	s.mu.RUnlock()
	if !ok {
		return Session{}, fmt.Errorf("mcp token %q is not registered", token)
	}
	session.Token = token
	return session, nil
}

type Server struct {
	tokenStore               *TokenStore
	registry                 *Registry
	handler                  http.Handler
	resolver                 SessionResolver
	protocolSessionBindings  map[string]sessionRefs
	protocolSessionBindingMu sync.RWMutex
}

type SessionResolver func(ctx context.Context, token string, header http.Header, body []byte) (Session, error)

func NewServer(tokenStore *TokenStore) (*Server, error) {
	if tokenStore == nil {
		return nil, fmt.Errorf("mcp token store is required")
	}
	registry, err := NewRegistry()
	if err != nil {
		return nil, err
	}
	out := &Server{
		tokenStore:              tokenStore,
		registry:                registry,
		protocolSessionBindings: map[string]sessionRefs{},
	}
	out.handler = mcp.NewStreamableHTTPHandler(
		func(r *http.Request) *mcp.Server {
			token := strings.TrimSpace(r.URL.Query().Get("token"))
			initialSession, ok := sessionFromContext(r.Context())
			if !ok {
				resolved, err := out.tokenStore.Resolve(token)
				if err != nil {
					return nil
				}
				initialSession = resolved
			}
			sessionID, threadID := sessionRefsFromContext(r.Context())
			return out.newMCPServerForConnection(newMCPConnectionState(token, sessionID, threadID), initialSession)
		},
		&mcp.StreamableHTTPOptions{JSONResponse: true, Stateless: true},
	)
	return out, nil
}

func (s *Server) SetSessionResolver(resolver SessionResolver) {
	if s == nil {
		return
	}
	s.resolver = resolver
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/mcp", s.handleMCP)
	return mux
}

func (s *Server) Run(ctx context.Context, addr string, wg *sync.WaitGroup) {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		panic("mcp addr is required")
	}
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		panic(fmt.Sprintf("mcp listen %q: %v", addr, err))
	}
	s.RunListener(ctx, listener, wg)
}

func (s *Server) RunListener(ctx context.Context, listener net.Listener, wg *sync.WaitGroup) {
	if listener == nil {
		panic("mcp listener is required")
	}
	srv := &http.Server{Handler: s.Handler()}
	if wg != nil {
		wg.Add(2)
	}
	go func() {
		if wg != nil {
			defer wg.Done()
		}
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			slog.Debug("mcp server shutdown error", "error", err)
		}
	}()
	go func() {
		if wg != nil {
			defer wg.Done()
		}
		if err := srv.Serve(listener); err != nil && err != http.ErrServerClosed {
			slog.Error("mcp server failed", "addr", listener.Addr().String(), "error", err)
		}
	}()
}

const (
	mcpRPCCodeInternalError = -32603
	mcpRPCCodeInvalidToken  = -32001
)

type mcpJSONRPCResponse struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      json.RawMessage  `json:"id,omitempty"`
	Result  any              `json:"result,omitempty"`
	Error   *mcpJSONRPCError `json:"error,omitempty"`
}

type mcpJSONRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type mcpJSONRPCRequestEnvelope struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	ID      any    `json:"id,omitempty"`
	Params  struct {
		Name string `json:"name,omitempty"`
	} `json:"params,omitempty"`
}

const (
	agen8NativeSessionIDHeader = "Agen8-Native-Session-Id"
	agen8NativeThreadIDHeader  = "Agen8-Native-Thread-Id"
)

func (s *Server) handleMCP(w http.ResponseWriter, r *http.Request) {
	if s == nil || s.tokenStore == nil || s.handler == nil {
		s.writeRPCError(w, nil, mcpRPCCodeInternalError, "mcp server is not configured")
		return
	}
	body, err := readAndRestoreBody(r)
	if err != nil {
		s.writeRPCError(w, extractRPCID(body), mcpRPCCodeInternalError, fmt.Sprintf("read request body: %v", err))
		return
	}
	method := mcpRPCMethod(body)
	slog.Info("mcp request received", "http_method", r.Method, "rpc_method", method, "has_body", len(bytes.TrimSpace(body)) > 0)
	prepareInitialNativeSessionHeader(r, method)
	resolverHeader := nativeSessionResolverHeader(r.Header, method)
	if mappedSessionID, mappedThreadID := s.nativeRefsForProtocolSession(r.URL.Query().Get("token"), protocolSessionIDFromHeader(r.Header)); mappedSessionID != "" || mappedThreadID != "" {
		if mappedSessionID != "" {
			resolverHeader.Set("Mcp-Session-Id", mappedSessionID)
		}
		if mappedThreadID != "" {
			resolverHeader.Set("Mcp-Thread-Id", mappedThreadID)
		}
	}
	requestSessionID, requestThreadID := SessionRefsFromHTTPHeader(resolverHeader)
	ctx := contextWithSessionRefs(r.Context(), requestSessionID, requestThreadID)
	session, err := s.resolveSession(ctx, r.URL.Query().Get("token"), resolverHeader, body)
	if err != nil {
		s.writeRPCError(w, extractRPCID(body), mcpRPCCodeInvalidToken, err.Error())
		return
	}
	if handled := s.handleUnknownToolAsMCPResult(w, body, session); handled {
		return
	}
	r = r.WithContext(context.WithValue(ctx, sessionContextKey{}, session))
	s.handler.ServeHTTP(w, r)
}

func mcpRPCMethod(body []byte) string {
	req := mcpJSONRPCRequestEnvelope{}
	if err := json.Unmarshal(bytes.TrimSpace(body), &req); err != nil {
		return ""
	}
	return strings.TrimSpace(req.Method)
}

func prepareInitialNativeSessionHeader(r *http.Request, method string) {
	if r == nil || !strings.EqualFold(strings.TrimSpace(method), "initialize") {
		return
	}
	nativeSessionID := strings.TrimSpace(r.Header.Get("Mcp-Session-Id"))
	if nativeSessionID == "" {
		nativeSessionID = strings.TrimSpace(r.Header.Get("MCP-Session-Id"))
	}
	if nativeSessionID == "" {
		return
	}
	if strings.TrimSpace(r.Header.Get(agen8NativeSessionIDHeader)) == "" {
		r.Header.Set(agen8NativeSessionIDHeader, nativeSessionID)
	}
	r.Header.Del("Mcp-Session-Id")
	r.Header.Del("MCP-Session-Id")
}

func nativeSessionResolverHeader(header http.Header, method string) http.Header {
	out := header.Clone()
	if out == nil {
		out = http.Header{}
	}
	return out
}

func (s *Server) resolveSession(ctx context.Context, token string, header http.Header, body []byte) (Session, error) {
	if s != nil && s.resolver != nil {
		return s.resolver(ctx, token, header, body)
	}
	return s.tokenStore.Resolve(token)
}

func (s *Server) handleUnknownToolAsMCPResult(w http.ResponseWriter, body []byte, session Session) bool {
	req := mcpJSONRPCRequestEnvelope{}
	if err := json.Unmarshal(bytes.TrimSpace(body), &req); err != nil {
		return false
	}
	if strings.TrimSpace(req.Method) != "tools/call" {
		return false
	}
	toolName := strings.TrimSpace(req.Params.Name)
	if toolName == "" {
		return false
	}
	if s.hasToolForSession(toolName, session) {
		return false
	}
	message := fmt.Sprintf("unknown tool %q", toolName)
	response := mcpJSONRPCResponse{
		JSONRPC: "2.0",
		ID:      normalizeRPCID(req.ID),
		Result: map[string]any{
			"content": []map[string]string{{
				"type": "text",
				"text": message,
			}},
			"isError": true,
		},
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		slog.Error("mcp unknown tool response encode failed", "error", err)
	}
	return true
}

type mcpConnectionState struct {
	token string
	mu    sync.RWMutex
	refs  sessionRefs
}

func newMCPConnectionState(token string, sessionID string, threadID string) *mcpConnectionState {
	state := &mcpConnectionState{token: strings.TrimSpace(token)}
	state.observe(sessionID, threadID)
	return state
}

func (s *mcpConnectionState) observe(sessionID string, threadID string) {
	if s == nil {
		return
	}
	sessionID = strings.TrimSpace(sessionID)
	threadID = strings.TrimSpace(threadID)
	if sessionID == "" && threadID == "" {
		return
	}
	s.mu.Lock()
	if sessionID != "" {
		s.refs.sessionID = sessionID
	}
	if threadID != "" {
		s.refs.threadID = threadID
	}
	s.mu.Unlock()
}

func (s *mcpConnectionState) nativeRefs() (sessionID, threadID string) {
	if s == nil {
		return "", ""
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return strings.TrimSpace(s.refs.sessionID), strings.TrimSpace(s.refs.threadID)
}

func (s *Server) newMCPServerForConnection(conn *mcpConnectionState, initialSession Session) *mcp.Server {
	server := mcp.NewServer(
		&mcp.Implementation{Name: "agen8-mcp", Version: "1.0.0"},
		nil,
	)
	for _, def := range s.registry.Defs() {
		if !toolVisibleForSession(def, initialSession) {
			continue
		}
		if strings.TrimSpace(def.native.name) != "" {
			native := def.native
			tool := &mcp.Tool{
				Name:        native.name,
				Description: strings.TrimSpace(native.description),
				InputSchema: append(json.RawMessage(nil), def.inputSchema...),
			}
			server.AddTool(tool, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				rpcBody := toolCallRPCBody(req)
				sessionID, threadID := mcpSessionRefs(req)
				header := nativeRefsHeaderFromRequest(req)
				headerSessionID, headerThreadID := explicitNativeSessionRefsFromHeader(header)
				if headerSessionID == "" && headerThreadID == "" {
					if mappedSessionID, mappedThreadID := s.nativeRefsForProtocolSession(conn.token, protocolSessionIDFromHeader(header)); mappedSessionID != "" || mappedThreadID != "" {
						headerSessionID, headerThreadID = mappedSessionID, mappedThreadID
					}
				}
				if headerSessionID == "" && headerThreadID == "" {
					headerSessionID, headerThreadID = SessionRefsFromHTTPHeader(nativeSessionResolverHeader(header, ""))
				}
				if sessionID == "" {
					sessionID = headerSessionID
				}
				if threadID == "" {
					threadID = headerThreadID
				}
				bodyRefs := SessionRequestContextFromJSONRPCBody(rpcBody)
				if sessionID == "" {
					sessionID = bodyRefs.SessionID
				}
				if threadID == "" {
					threadID = bodyRefs.ThreadID
				}
				if strings.EqualFold(strings.TrimSpace(native.name), space.Name) {
					argSessionID, argThreadID := explicitNativeSessionRefsFromSpaceRegisterArguments(req)
					if sessionID == "" {
						sessionID = argSessionID
					}
					if threadID == "" {
						threadID = argThreadID
					}
				}
				session, err := s.resolveSessionForMCPCall(ctx, conn, nativeRefsHeaderFromRequest(req), sessionID, threadID, rpcBody)
				if err != nil {
					return mcpToolCallErrorResult(err.Error()), nil
				}
				ctx = contextWithSessionRefs(ctx, sessionID, threadID)
				result, err := executeNativeMCPTool(ctx, native, session, req)
				if err == nil && strings.EqualFold(strings.TrimSpace(native.name), space.Name) && strings.EqualFold(mcpToolAction(argumentsFromToolRequest(req)), "register") {
					resultSessionID, resultThreadID := nativeSessionRefsFromToolResult(result)
					if resultSessionID == "" {
						resultSessionID = sessionID
					}
					if resultThreadID == "" {
						resultThreadID = threadID
					}
					s.bindProtocolSession(conn.token, protocolSessionIDFromHeader(header), resultSessionID, resultThreadID)
					conn.observe(resultSessionID, resultThreadID)
				}
				return result, err
			})
			continue
		}
	}
	return server
}

func nativeRefsHeaderFromRequest(req interface{ GetExtra() *mcp.RequestExtra }) http.Header {
	if req == nil || req.GetExtra() == nil {
		return nil
	}
	return req.GetExtra().Header
}

func protocolSessionIDFromHeader(header http.Header) string {
	if len(header) == 0 {
		return ""
	}
	sessionID := strings.TrimSpace(header.Get("Mcp-Session-Id"))
	if sessionID == "" {
		sessionID = strings.TrimSpace(header.Get("MCP-Session-Id"))
	}
	return sessionID
}

func explicitNativeSessionRefsFromHeader(header http.Header) (sessionID, threadID string) {
	if len(header) == 0 {
		return "", ""
	}
	return strings.TrimSpace(header.Get(agen8NativeSessionIDHeader)), strings.TrimSpace(header.Get(agen8NativeThreadIDHeader))
}

func (s *Server) bindProtocolSession(token string, protocolSessionID string, sessionID string, threadID string) {
	if s == nil {
		return
	}
	token = strings.TrimSpace(token)
	protocolSessionID = strings.TrimSpace(protocolSessionID)
	sessionID = strings.TrimSpace(sessionID)
	threadID = strings.TrimSpace(threadID)
	if token == "" || protocolSessionID == "" || (sessionID == "" && threadID == "") {
		return
	}
	s.protocolSessionBindingMu.Lock()
	if s.protocolSessionBindings == nil {
		s.protocolSessionBindings = map[string]sessionRefs{}
	}
	s.protocolSessionBindings[protocolSessionBindingKey(token, protocolSessionID)] = sessionRefs{sessionID: sessionID, threadID: threadID}
	s.protocolSessionBindingMu.Unlock()
	slog.Info("mcp protocol session bound to native refs", "protocol_session_id", protocolSessionID, "has_session_id", sessionID != "", "has_thread_id", threadID != "")
}

func (s *Server) nativeRefsForProtocolSession(token string, protocolSessionID string) (sessionID, threadID string) {
	if s == nil {
		return "", ""
	}
	token = strings.TrimSpace(token)
	protocolSessionID = strings.TrimSpace(protocolSessionID)
	if token == "" || protocolSessionID == "" {
		return "", ""
	}
	s.protocolSessionBindingMu.RLock()
	refs := s.protocolSessionBindings[protocolSessionBindingKey(token, protocolSessionID)]
	s.protocolSessionBindingMu.RUnlock()
	return strings.TrimSpace(refs.sessionID), strings.TrimSpace(refs.threadID)
}

func protocolSessionBindingKey(token string, protocolSessionID string) string {
	return strings.TrimSpace(token) + "\x00" + strings.TrimSpace(protocolSessionID)
}

func (s *Server) resolveSessionForMCPCall(ctx context.Context, conn *mcpConnectionState, header http.Header, sessionID string, threadID string, rpcBody []byte) (Session, error) {
	if conn == nil {
		return Session{}, fmt.Errorf("mcp connection state is required")
	}
	if sessionID == "" && threadID == "" {
		sessionID, threadID = s.nativeRefsForProtocolSession(conn.token, protocolSessionIDFromHeader(header))
	}
	conn.observe(sessionID, threadID)
	storedSessionID, storedThreadID := conn.nativeRefs()
	resolverHeader := nativeSessionResolverHeader(header, "")
	if storedSessionID != "" {
		resolverHeader.Set("Mcp-Session-Id", storedSessionID)
	}
	if storedThreadID != "" {
		resolverHeader.Set("Mcp-Thread-Id", storedThreadID)
	}
	callCtx := contextWithSessionRefs(ctx, storedSessionID, storedThreadID)
	return s.resolveSession(callCtx, conn.token, resolverHeader, rpcBody)
}

func argumentsFromToolRequest(req *mcp.CallToolRequest) json.RawMessage {
	if req == nil || req.Params == nil || len(req.Params.Arguments) == 0 {
		return json.RawMessage(`{}`)
	}
	return req.Params.Arguments
}

func toolCallRPCBody(req *mcp.CallToolRequest) []byte {
	body := syntheticJSONRPCMethodBody("tools/call")
	if req == nil || req.Params == nil {
		return body
	}
	arguments := req.Params.Arguments
	if arguments == nil {
		arguments = json.RawMessage(`{}`)
	}
	payload := map[string]any{
		"method": "tools/call",
		"params": map[string]any{
			"name":      strings.TrimSpace(req.Params.Name),
			"arguments": arguments,
		},
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return body
	}
	return encoded
}

func syntheticJSONRPCMethodBody(method string) []byte {
	method = strings.TrimSpace(method)
	if method == "" {
		return nil
	}
	body, err := json.Marshal(map[string]string{"method": method})
	if err != nil {
		return nil
	}
	return body
}

func toolVisibleForSession(def toolDef, session Session) bool {
	if !def.native.internal {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(session.HarnessKind)) {
	case "claude-cli", "claude-code":
		return strings.EqualFold(strings.TrimSpace(def.name()), harnessapprovaltool.Name)
	default:
		return false
	}
}

func executeNativeMCPTool(ctx context.Context, def nativeToolDef, session Session, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var arguments json.RawMessage
	if req != nil && req.Params != nil && len(req.Params.Arguments) > 0 {
		arguments = append(json.RawMessage(nil), req.Params.Arguments...)
	}
	if len(arguments) == 0 {
		arguments = json.RawMessage(`{}`)
	}
	if !json.Valid(arguments) {
		return mcpToolCallErrorResult(fmt.Sprintf("tool %q arguments must be valid JSON", def.name)), nil
	}
	if session.Bootstrap {
		if !strings.EqualFold(strings.TrimSpace(def.name), space.Name) {
			return mcpToolCallErrorResult("mcp session is not registered; call space.register first"), nil
		}
		if !space.BootstrapActionAllowed(mcpToolAction(arguments)) {
			return mcpToolCallErrorResult("mcp session is not registered; call space.register first"), nil
		}
	}
	switch strings.TrimSpace(def.name) {
	case decisiontool.Name:
		handler := decisiontool.NewHandler()
		call := decisiontool.CallContext{
			Decisions:     session.DecisionService,
			ProjectID:     strings.TrimSpace(session.ProjectID),
			SpaceID:       strings.TrimSpace(string(session.SpaceID)),
			ActorMemberID: strings.TrimSpace(session.MemberID),
		}
		declaration, pending, err := handler.DeclareHumanInput(ctx, arguments)
		if err != nil {
			return mcpToolCallErrorResult(err.Error()), nil
		}
		if pending {
			if session.HumanInputAwaiter == nil {
				return mcpToolCallErrorResult("decision: human input awaiter is not configured"), nil
			}
			toolCallID := stableHumanInputToolCallID(def.name, arguments)
			resolved, err := session.HumanInputAwaiter.Await(ctx, humaninput.PendingRequest{
				ToolCallID:     toolCallID,
				ToolName:       decisiontool.Name,
				IdempotencyKey: toolCallID,
				Declaration:    declaration,
				ProjectID:      strings.TrimSpace(session.ProjectID),
				SpaceID:        strings.TrimSpace(string(session.SpaceID)),
				MemberID:       strings.TrimSpace(session.MemberID),
				ChannelID:      strings.TrimSpace(string(session.ChannelID)),
			})
			if err != nil {
				return mcpToolCallErrorResult(err.Error()), nil
			}
			result, err := handler.ResolveHumanInput(ctx, arguments, resolved, call)
			if err != nil {
				return mcpToolCallErrorResult(err.Error()), nil
			}
			return &mcp.CallToolResult{
				Content:           []mcp.Content{&mcp.TextContent{Text: strings.TrimSpace(result.Text)}},
				StructuredContent: result.Structured,
			}, nil
		}
		result, err := handler.Handle(ctx, call, arguments)
		if err != nil {
			return mcpToolCallErrorResult(err.Error()), nil
		}
		return &mcp.CallToolResult{
			Content:           []mcp.Content{&mcp.TextContent{Text: strings.TrimSpace(result.Text)}},
			StructuredContent: result.Structured,
		}, nil
	case httptool.Name:
		result, err := httptool.NewHandler().Handle(ctx, httptool.CallContext{
			Credentials: session.CredentialResolver,
		}, arguments)
		if err != nil {
			return mcpToolCallErrorResult(err.Error()), nil
		}
		return &mcp.CallToolResult{
			Content:           []mcp.Content{&mcp.TextContent{Text: strings.TrimSpace(result.Text)}},
			StructuredContent: result.Structured,
		}, nil
	case harnessapprovaltool.Name:
		result, err := harnessapprovaltool.NewHandler().Handle(ctx, harnessapprovaltool.CallContext{
			HumanInputAwaiter: session.HumanInputAwaiter,
			ProjectID:         strings.TrimSpace(session.ProjectID),
			SpaceID:           strings.TrimSpace(string(session.SpaceID)),
			MemberID:          strings.TrimSpace(session.MemberID),
			ChannelID:         strings.TrimSpace(string(session.ChannelID)),
		}, arguments)
		if err != nil {
			return mcpToolCallErrorResult(err.Error()), nil
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: strings.TrimSpace(result.Text)}},
		}, nil
	case graphtool.Name:
		result, err := graphtool.NewHandler().Handle(ctx, graphtool.CallContext{
			Graph:         session.GraphService,
			Members:       session.MemberDirectory,
			ProjectID:     strings.TrimSpace(session.ProjectID),
			SpaceID:       strings.TrimSpace(string(session.SpaceID)),
			ActorMemberID: strings.TrimSpace(session.MemberID),
		}, arguments)
		if err != nil {
			return mcpToolCallErrorResult(err.Error()), nil
		}
		return &mcp.CallToolResult{
			Content:           []mcp.Content{&mcp.TextContent{Text: strings.TrimSpace(result.Text)}},
			StructuredContent: result.Structured,
		}, nil
	case messagetool.Name:
		result, err := messagetool.NewHandler().Handle(ctx, messagetool.CallContext{
			Members:       session.MemberDirectory,
			Messages:      session.MessagePublisher,
			ProjectID:     strings.TrimSpace(session.ProjectID),
			SpaceID:       strings.TrimSpace(string(session.SpaceID)),
			ActorMemberID: strings.TrimSpace(session.MemberID),
		}, arguments)
		if err != nil {
			return mcpToolCallErrorResult(err.Error()), nil
		}
		return &mcp.CallToolResult{
			Content:           []mcp.Content{&mcp.TextContent{Text: strings.TrimSpace(result.Text)}},
			StructuredContent: result.Structured,
		}, nil
	case space.Name:
		sessionID, threadID := explicitNativeSessionRefsFromToolRequest(req)
		result, err := space.NewHandler().Handle(ctx, space.CallContext{
			Spaces:           session.SpaceReader,
			Members:          session.MemberDirectory,
			Registrar:        session.MemberRegistrar,
			ContextRegistrar: session.ContextRegistrar,
			SpaceSetup:       session.SpaceSetup,
			MCPToken:         strings.TrimSpace(session.Token),
			UserID:           strings.TrimSpace(session.UserID),
			HarnessKind:      strings.TrimSpace(session.HarnessKind),
			ProjectID:        strings.TrimSpace(session.ProjectID),
			SpaceID:          strings.TrimSpace(string(session.SpaceID)),
			ActorMemberID:    strings.TrimSpace(session.MemberID),
			SessionID:        sessionID,
			ThreadID:         threadID,
		}, arguments)
		if err != nil {
			return mcpToolCallErrorResult(err.Error()), nil
		}
		out := &mcp.CallToolResult{
			Content:           []mcp.Content{&mcp.TextContent{Text: strings.TrimSpace(result.Text)}},
			StructuredContent: result.Structured,
		}
		return out, nil
	case missiontool.Name:
		result, err := missiontool.NewHandler().Handle(ctx, missiontool.CallContext{
			Missions:      session.MissionService,
			KeyResults:    session.MissionKRs,
			Progress:      session.MissionProgress,
			Members:       session.MemberDirectory,
			ProjectID:     strings.TrimSpace(session.ProjectID),
			SpaceID:       strings.TrimSpace(string(session.SpaceID)),
			ActorMemberID: strings.TrimSpace(session.MemberID),
		}, arguments)
		if err != nil {
			return mcpToolCallErrorResult(err.Error()), nil
		}
		return &mcp.CallToolResult{
			Content:           []mcp.Content{&mcp.TextContent{Text: strings.TrimSpace(result.Text)}},
			StructuredContent: result.Structured,
		}, nil
	case operatortool.Name:
		result, err := operatortool.NewHandler().Handle(ctx, operatortool.CallContext{
			Operator:      session.OperatorService,
			ProjectID:     strings.TrimSpace(session.ProjectID),
			SpaceID:       strings.TrimSpace(string(session.SpaceID)),
			ActorMemberID: strings.TrimSpace(session.MemberID),
		}, arguments)
		if err != nil {
			return mcpToolCallErrorResult(err.Error()), nil
		}
		return &mcp.CallToolResult{
			Content:           []mcp.Content{&mcp.TextContent{Text: strings.TrimSpace(result.Text)}},
			StructuredContent: result.Structured,
		}, nil
	case scheduletool.Name:
		result, err := scheduletool.NewHandler().Handle(ctx, scheduletool.CallContext{
			Schedules:     session.ScheduleService,
			SpaceID:       strings.TrimSpace(string(session.SpaceID)),
			ActorMemberID: strings.TrimSpace(session.MemberID),
		}, arguments)
		if err != nil {
			return mcpToolCallErrorResult(err.Error()), nil
		}
		return &mcp.CallToolResult{
			Content:           []mcp.Content{&mcp.TextContent{Text: strings.TrimSpace(result.Text)}},
			StructuredContent: result.Structured,
		}, nil
	case tasktool.Name:
		result, err := tasktool.NewHandler().Handle(ctx, tasktool.CallContext{
			Tasks:         session.TaskService,
			Members:       session.TaskMembers,
			ProjectID:     strings.TrimSpace(session.ProjectID),
			SpaceID:       strings.TrimSpace(string(session.SpaceID)),
			ActorMemberID: strings.TrimSpace(session.MemberID),
		}, arguments)
		if err != nil {
			return mcpToolCallErrorResult(err.Error()), nil
		}
		return &mcp.CallToolResult{
			Content:           []mcp.Content{&mcp.TextContent{Text: strings.TrimSpace(result.Text)}},
			StructuredContent: result.Structured,
		}, nil
	default:
		return mcpToolCallErrorResult(fmt.Sprintf("tool %q has no native handler", def.name)), nil
	}
}

func mcpToolAction(arguments json.RawMessage) string {
	var raw struct {
		Action string `json:"action"`
	}
	if err := json.Unmarshal(arguments, &raw); err != nil {
		return ""
	}
	return strings.TrimSpace(raw.Action)
}

func nativeSessionRefsFromToolResult(result *mcp.CallToolResult) (sessionID, threadID string) {
	if result == nil || result.StructuredContent == nil {
		return "", ""
	}
	raw, err := json.Marshal(result.StructuredContent)
	if err != nil {
		return "", ""
	}
	var structured struct {
		NativeSessionRef string `json:"nativeSessionRef"`
		SessionID        string `json:"sessionId"`
		ThreadID         string `json:"threadId"`
	}
	if err := json.Unmarshal(raw, &structured); err != nil {
		return "", ""
	}
	sessionID = strings.TrimSpace(structured.NativeSessionRef)
	if sessionID == "" {
		sessionID = strings.TrimSpace(structured.SessionID)
	}
	return sessionID, strings.TrimSpace(structured.ThreadID)
}

func explicitNativeSessionRefsFromToolRequest(req *mcp.CallToolRequest) (sessionID, threadID string) {
	sessionID, threadID = mcpSessionRefs(req)
	if sessionID != "" || threadID != "" {
		return sessionID, threadID
	}
	sessionID, threadID = explicitNativeSessionRefsFromSpaceRegisterArguments(req)
	if sessionID != "" || threadID != "" {
		return sessionID, threadID
	}
	header := nativeRefsHeaderFromRequest(req)
	if header == nil {
		return "", ""
	}
	return strings.TrimSpace(header.Get(agen8NativeSessionIDHeader)), strings.TrimSpace(header.Get(agen8NativeThreadIDHeader))
}

func explicitNativeSessionRefsFromSpaceRegisterArguments(req *mcp.CallToolRequest) (sessionID, threadID string) {
	if req == nil || req.Params == nil || len(req.Params.Arguments) == 0 {
		return "", ""
	}
	var raw struct {
		Action           string `json:"action"`
		SessionID        string `json:"session_id"`
		ThreadID         string `json:"thread_id"`
		NativeSessionRef string `json:"native_session_ref"`
	}
	if err := json.Unmarshal(req.Params.Arguments, &raw); err != nil {
		return "", ""
	}
	if strings.TrimSpace(raw.Action) != "register" {
		return "", ""
	}
	if nativeSessionRef := strings.TrimSpace(raw.NativeSessionRef); nativeSessionRef != "" {
		return nativeSessionRef, strings.TrimSpace(raw.ThreadID)
	}
	return strings.TrimSpace(raw.SessionID), strings.TrimSpace(raw.ThreadID)
}

func mcpSessionRefs(req *mcp.CallToolRequest) (sessionID, threadID string) {
	if req == nil || req.Params == nil {
		return "", ""
	}
	meta := req.Params.Meta
	threadID = firstMetaString(meta, "threadId", "thread_id")
	sessionID = firstMetaString(meta, "sessionId", "session_id")
	if turnMeta, ok := meta["x-codex-turn-metadata"].(map[string]any); ok {
		if threadID == "" {
			threadID = firstMetaString(turnMeta, "thread_id", "threadId")
		}
		if sessionID == "" {
			sessionID = firstMetaString(turnMeta, "session_id", "sessionId")
		}
	}
	return strings.TrimSpace(sessionID), strings.TrimSpace(threadID)
}

func SessionRefsFromJSONRPCBody(body []byte) (sessionID, threadID string) {
	refs := SessionRequestContextFromJSONRPCBody(body)
	return refs.SessionID, refs.ThreadID
}

func SessionRequestContextFromJSONRPCBody(body []byte) SessionRequestContext {
	var envelope struct {
		Method string `json:"method"`
		Params struct {
			Name      string         `json:"name,omitempty"`
			Meta      map[string]any `json:"_meta,omitempty"`
			Arguments struct {
				Action    string `json:"action"`
				SessionID string `json:"session_id"`
				ThreadID  string `json:"thread_id"`
			} `json:"arguments,omitempty"`
		} `json:"params,omitempty"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(body), &envelope); err != nil {
		return SessionRequestContext{}
	}
	meta := envelope.Params.Meta
	threadID := firstMetaString(meta, "threadId", "thread_id")
	sessionID := firstMetaString(meta, "sessionId", "session_id")
	turnID := firstMetaString(meta, "turnId", "turn_id")
	if turnMeta, ok := meta["x-codex-turn-metadata"].(map[string]any); ok {
		if threadID == "" {
			threadID = firstMetaString(turnMeta, "thread_id", "threadId")
		}
		if sessionID == "" {
			sessionID = firstMetaString(turnMeta, "session_id", "sessionId")
		}
		if turnID == "" {
			turnID = firstMetaString(turnMeta, "turn_id", "turnId")
		}
	}
	if strings.TrimSpace(envelope.Method) == "tools/call" &&
		strings.TrimSpace(envelope.Params.Name) == space.Name &&
		strings.TrimSpace(envelope.Params.Arguments.Action) == "register" {
		if sessionID == "" {
			sessionID = strings.TrimSpace(envelope.Params.Arguments.SessionID)
		}
		if threadID == "" {
			threadID = strings.TrimSpace(envelope.Params.Arguments.ThreadID)
		}
	}
	return SessionRequestContext{
		SessionID: strings.TrimSpace(sessionID),
		ThreadID:  strings.TrimSpace(threadID),
		TurnID:    strings.TrimSpace(turnID),
	}
}

func SessionRefsFromHTTPHeader(header http.Header) (sessionID, threadID string) {
	if len(header) == 0 {
		return "", ""
	}
	sessionID = strings.TrimSpace(header.Get(agen8NativeSessionIDHeader))
	if sessionID == "" {
		sessionID = strings.TrimSpace(header.Get("Mcp-Session-Id"))
	}
	if sessionID == "" {
		sessionID = strings.TrimSpace(header.Get("MCP-Session-Id"))
	}
	threadID = strings.TrimSpace(header.Get(agen8NativeThreadIDHeader))
	if threadID == "" {
		threadID = strings.TrimSpace(header.Get("Mcp-Thread-Id"))
	}
	if threadID == "" {
		threadID = strings.TrimSpace(header.Get("MCP-Thread-Id"))
	}
	return sessionID, threadID
}

type sessionRefsContextKey struct{}

type sessionRefs struct {
	sessionID string
	threadID  string
}

func contextWithSessionRefs(ctx context.Context, sessionID, threadID string) context.Context {
	sessionID = strings.TrimSpace(sessionID)
	threadID = strings.TrimSpace(threadID)
	if sessionID == "" && threadID == "" {
		return ctx
	}
	return context.WithValue(ctx, sessionRefsContextKey{}, sessionRefs{sessionID: sessionID, threadID: threadID})
}

func sessionRefsFromContext(ctx context.Context) (sessionID, threadID string) {
	refs, ok := ctx.Value(sessionRefsContextKey{}).(sessionRefs)
	if !ok {
		return "", ""
	}
	return strings.TrimSpace(refs.sessionID), strings.TrimSpace(refs.threadID)
}

func firstMetaString(meta map[string]any, keys ...string) string {
	for _, key := range keys {
		value := strings.TrimSpace(metaString(meta[key]))
		if value != "" {
			return value
		}
	}
	return ""
}

func metaString(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case fmt.Stringer:
		return v.String()
	default:
		return ""
	}
}

func stableHumanInputToolCallID(toolName string, arguments json.RawMessage) string {
	sum := sha256.Sum256(bytes.TrimSpace(arguments))
	return "mcp:" + strings.TrimSpace(toolName) + ":" + hex.EncodeToString(sum[:8])
}

func (s *Server) hasToolForSession(name string, session Session) bool {
	if s == nil || s.registry == nil {
		return false
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return false
	}
	for _, def := range s.registry.Defs() {
		if strings.TrimSpace(def.name()) == name && toolVisibleForSession(def, session) {
			return true
		}
	}
	return false
}

func readAndRestoreBody(r *http.Request) ([]byte, error) {
	if r == nil || r.Body == nil {
		return nil, nil
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return body, err
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	return body, nil
}

func extractRPCID(body []byte) json.RawMessage {
	body = bytes.TrimSpace(body)
	if len(body) == 0 {
		return nil
	}
	var envelope struct {
		ID json.RawMessage `json:"id,omitempty"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil
	}
	if len(envelope.ID) == 0 {
		return nil
	}
	return append(json.RawMessage(nil), envelope.ID...)
}

func normalizeRPCID(id any) json.RawMessage {
	if id == nil {
		return nil
	}
	raw, err := json.Marshal(id)
	if err != nil {
		return nil
	}
	return append(json.RawMessage(nil), raw...)
}

func mcpToolCallErrorResult(message string) *mcp.CallToolResult {
	message = strings.TrimSpace(message)
	if message == "" {
		message = "tool call failed"
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: message}},
		IsError: true,
	}
}

func (s *Server) writeRPCError(w http.ResponseWriter, id json.RawMessage, code int, message string) {
	response := mcpJSONRPCResponse{
		JSONRPC: "2.0",
		ID:      append(json.RawMessage(nil), id...),
		Error:   &mcpJSONRPCError{Code: code, Message: strings.TrimSpace(message)},
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		slog.Error("mcp response encode failed", "error", err)
	}
}
