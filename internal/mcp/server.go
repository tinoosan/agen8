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
	ChannelID          types.ChannelID
	SpaceID            spacedomain.SpaceID
	MemberID           string
	HarnessKind        string
	ContextRegistrar   space.ContextRegistrar
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
	tokenStore *TokenStore
	registry   *Registry
	handler    http.Handler
}

func NewServer(tokenStore *TokenStore) (*Server, error) {
	if tokenStore == nil {
		return nil, fmt.Errorf("mcp token store is required")
	}
	registry, err := NewRegistry()
	if err != nil {
		return nil, err
	}
	out := &Server{
		tokenStore: tokenStore,
		registry:   registry,
	}
	out.handler = mcp.NewStreamableHTTPHandler(
		func(r *http.Request) *mcp.Server {
			session, ok := sessionFromContext(r.Context())
			if !ok {
				resolved, err := out.tokenStore.Resolve(r.URL.Query().Get("token"))
				if err != nil {
					return nil
				}
				session = resolved
			}
			return out.newMCPServerForSession(session)
		},
		&mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true},
	)
	return out, nil
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
	session, err := s.tokenStore.Resolve(r.URL.Query().Get("token"))
	if err != nil {
		s.writeRPCError(w, extractRPCID(body), mcpRPCCodeInvalidToken, err.Error())
		return
	}
	if handled := s.handleUnknownToolAsMCPResult(w, body, session); handled {
		return
	}
	r = r.WithContext(context.WithValue(r.Context(), sessionContextKey{}, session))
	s.handler.ServeHTTP(w, r)
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

func (s *Server) newMCPServerForSession(session Session) *mcp.Server {
	server := mcp.NewServer(
		&mcp.Implementation{Name: "agen8-mcp", Version: "1.0.0"},
		nil,
	)
	for _, def := range s.registry.Defs() {
		if !toolVisibleForSession(def, session) {
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
				return executeNativeMCPTool(ctx, native, session, req)
			})
			continue
		}
	}
	return server
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
		sessionID, threadID := mcpSessionRefs(req)
		result, err := space.NewHandler().Handle(ctx, space.CallContext{
			Spaces:           session.SpaceReader,
			Members:          session.MemberDirectory,
			Registrar:        session.MemberRegistrar,
			ContextRegistrar: session.ContextRegistrar,
			MCPToken:         strings.TrimSpace(session.Token),
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
