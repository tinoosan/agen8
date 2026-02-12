package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"strings"
	"sync"

	implstore "github.com/tinoosan/workbench-core/internal/store"
	"github.com/tinoosan/workbench-core/pkg/agent/state"
	"github.com/tinoosan/workbench-core/pkg/config"
	"github.com/tinoosan/workbench-core/pkg/protocol"
	pkgstore "github.com/tinoosan/workbench-core/pkg/store"
	"github.com/tinoosan/workbench-core/pkg/timeutil"
	"github.com/tinoosan/workbench-core/pkg/types"
)

// RPCServer serves the Workbench protocol over JSON-RPC 2.0.
//
// Phase 3 transport is STDIO. Notifications are streamed from notifyCh.
type RPCServer struct {
	cfg            config.Config
	run            types.Run
	allowAnyThread bool

	taskStore state.TaskStore
	session   pkgstore.SessionReaderWriter
	initErr   error

	notifyCh <-chan protocol.Message
	index    *protocol.Index

	wake func()

	methodHandlers map[string]MethodHandler

	controlSetModel     func(ctx context.Context, threadID, target, model string) ([]string, error)
	controlSetReasoning func(ctx context.Context, threadID, target, effort, summary string) ([]string, error)
	controlSetProfile   func(ctx context.Context, threadID, target, profile string) ([]string, error)
	agentPause          func(ctx context.Context, threadID, runID string) error
	agentResume         func(ctx context.Context, threadID, runID string) error
	sessionPause        func(ctx context.Context, threadID, sessionID string) ([]string, error)
	sessionResume       func(ctx context.Context, threadID, sessionID string) ([]string, error)
	sessionStop         func(ctx context.Context, threadID, sessionID string) ([]string, error)
}

type RPCServerConfig struct {
	Cfg                 config.Config
	Run                 types.Run
	AllowAnyThread      bool
	TaskStore           state.TaskStore
	Session             pkgstore.SessionReaderWriter
	NotifyCh            <-chan protocol.Message
	Index               *protocol.Index
	Wake                func()
	ControlSetModel     func(ctx context.Context, threadID, target, model string) ([]string, error)
	ControlSetReasoning func(ctx context.Context, threadID, target, effort, summary string) ([]string, error)
	ControlSetProfile   func(ctx context.Context, threadID, target, profile string) ([]string, error)
	AgentPause          func(ctx context.Context, threadID, runID string) error
	AgentResume         func(ctx context.Context, threadID, runID string) error
	SessionPause        func(ctx context.Context, threadID, sessionID string) ([]string, error)
	SessionResume       func(ctx context.Context, threadID, sessionID string) ([]string, error)
	SessionStop         func(ctx context.Context, threadID, sessionID string) ([]string, error)
}

type MethodHandler interface {
	Handle(ctx context.Context, params json.RawMessage) (any, error)
}

type methodHandlerFunc func(ctx context.Context, params json.RawMessage) (any, error)

func (f methodHandlerFunc) Handle(ctx context.Context, params json.RawMessage) (any, error) {
	return f(ctx, params)
}

type methodRegistry map[string]MethodHandler

func (r methodRegistry) add(method string, handler MethodHandler) error {
	method = strings.TrimSpace(method)
	if method == "" {
		return fmt.Errorf("rpc method is required")
	}
	if handler == nil {
		return fmt.Errorf("rpc handler for method %q is nil", method)
	}
	if _, exists := r[method]; exists {
		return fmt.Errorf("duplicate rpc handler registration for method %q", method)
	}
	r[method] = handler
	return nil
}

type methodRegistrar func(*RPCServer, methodRegistry) error

func buildMethodRegistry(s *RPCServer, registrars ...methodRegistrar) (methodRegistry, error) {
	reg := methodRegistry{}
	for _, registrar := range registrars {
		if registrar == nil {
			continue
		}
		if err := registrar(s, reg); err != nil {
			return nil, err
		}
	}
	return reg, nil
}

func bindHandler[Params any, Result any](allowEmptyParams bool, fn func(context.Context, Params) (Result, error)) MethodHandler {
	return methodHandlerFunc(func(ctx context.Context, params json.RawMessage) (any, error) {
		var p Params
		if allowEmptyParams && len(params) == 0 {
			return fn(ctx, p)
		}
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, &protocol.ProtocolError{Code: protocol.CodeInvalidParams, Message: "invalid params"}
		}
		return fn(ctx, p)
	})
}

func addBoundHandler[Params any, Result any](reg methodRegistry, method string, allowEmptyParams bool, fn func(context.Context, Params) (Result, error)) error {
	return reg.add(method, bindHandler[Params, Result](allowEmptyParams, fn))
}

func NewRPCServer(cfg RPCServerConfig) *RPCServer {
	var sess pkgstore.SessionReaderWriter
	var initErr error
	if cfg.Session != nil {
		sess = cfg.Session
	} else {
		st, err := implstore.NewSQLiteSessionStore(cfg.Cfg)
		if err != nil {
			initErr = err
		} else {
			sess = st
		}
	}

	srv := &RPCServer{
		cfg:                 cfg.Cfg,
		run:                 cfg.Run,
		allowAnyThread:      cfg.AllowAnyThread,
		taskStore:           cfg.TaskStore,
		session:             sess,
		initErr:             initErr,
		notifyCh:            cfg.NotifyCh,
		index:               cfg.Index,
		wake:                cfg.Wake,
		controlSetModel:     cfg.ControlSetModel,
		controlSetReasoning: cfg.ControlSetReasoning,
		controlSetProfile:   cfg.ControlSetProfile,
		agentPause:          cfg.AgentPause,
		agentResume:         cfg.AgentResume,
		sessionPause:        cfg.SessionPause,
		sessionResume:       cfg.SessionResume,
		sessionStop:         cfg.SessionStop,
	}

	handlers, err := buildMethodRegistry(
		srv,
		registerSessionHandlers,
		registerControlHandlers,
		registerTeamHandlers,
		registerArtifactHandlers,
	)
	if err != nil {
		srv.initErr = errors.Join(srv.initErr, err)
	} else {
		srv.methodHandlers = handlers
	}

	return srv
}

// Serve reads JSON-RPC requests from in and writes responses/notifications to out.
func (s *RPCServer) Serve(ctx context.Context, in io.Reader, out io.Writer) error {
	if s == nil {
		return fmt.Errorf("rpc server is nil")
	}
	if s.initErr != nil {
		return s.initErr
	}
	if s.session == nil {
		return fmt.Errorf("session store not configured")
	}

	enc := json.NewEncoder(out)
	outCh := make(chan protocol.Message, 1024)
	stopCh := make(chan struct{})
	var closeOnce sync.Once
	writerDone := make(chan struct{})
	var writerErr error
	var writerMu sync.Mutex

	writeErr := func() error {
		writerMu.Lock()
		defer writerMu.Unlock()
		return writerErr
	}
	setWriteErr := func(err error) {
		writerMu.Lock()
		defer writerMu.Unlock()
		if writerErr == nil {
			writerErr = err
		}
	}

	closeOut := func() {
		closeOnce.Do(func() {
			close(stopCh)
			close(outCh)
		})
	}

	go func() {
		defer close(writerDone)
		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-outCh:
				if !ok {
					return
				}
				if err := enc.Encode(msg); err != nil {
					setWriteErr(err)
					return
				}
			}
		}
	}()

	sendResponse := func(msg protocol.Message) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-stopCh:
			return io.ErrClosedPipe
		case <-writerDone:
			if err := writeErr(); err != nil {
				return err
			}
			return io.ErrClosedPipe
		case outCh <- msg:
			return nil
		}
	}

	sendNotification := func(msg protocol.Message) {
		defer func() {
			if recover() != nil {
				// outCh may close concurrently during shutdown; notifications are best-effort.
			}
		}()
		select {
		case <-ctx.Done():
			return
		case <-stopCh:
			return
		case <-writerDone:
			return
		case outCh <- msg:
			s.logNotificationSummary(msg)
			return
		default:
			// Best-effort: drop if the writer is backpressured.
			return
		}
	}

	// Forward notifications in a best-effort, non-blocking way.
	if s.notifyCh != nil {
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case <-stopCh:
					return
				case msg, ok := <-s.notifyCh:
					if !ok {
						return
					}
					sendNotification(msg)
				}
			}
		}()
	}

	dec := json.NewDecoder(in)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		var msg protocol.Message
		if err := dec.Decode(&msg); err != nil {
			if errors.Is(err, io.EOF) {
				// Best-effort: flush any already-queued notifications before shutdown.
				if s.notifyCh != nil {
					for {
						select {
						case n := <-s.notifyCh:
							sendNotification(n)
						default:
							goto drained
						}
					}
				}
			drained:
				closeOut()
				<-writerDone
				return nil
			}
			_ = sendResponse(protocol.NewErrorResponse(nil, &protocol.RPCError{Code: protocol.CodeParseError, Message: "parse error"}))
			closeOut()
			<-writerDone
			return err
		}

		// Only handle requests (notifications from the client are ignored for now).
		if msg.Method == "" || msg.ID == nil || strings.TrimSpace(*msg.ID) == "" {
			continue
		}
		s.logRequestSummary(msg)

		resp := s.handleRequest(ctx, msg)
		s.logResponseSummary(resp)
		if err := sendResponse(resp); err != nil {
			closeOut()
			<-writerDone
			return err
		}
	}
}

func (s *RPCServer) handleRequest(ctx context.Context, msg protocol.Message) protocol.Message {
	id := msg.ID
	if id == nil || strings.TrimSpace(*id) == "" {
		return protocol.NewErrorResponse(id, &protocol.RPCError{Code: protocol.CodeInvalidRequest, Message: "invalid request"})
	}
	if strings.TrimSpace(msg.JSONRPC) != "" && strings.TrimSpace(msg.JSONRPC) != "2.0" {
		return protocol.NewErrorResponse(id, &protocol.RPCError{Code: protocol.CodeInvalidRequest, Message: "invalid jsonrpc version"})
	}

	method := strings.TrimSpace(msg.Method)
	handler, ok := s.methodHandlers[method]
	if !ok {
		return protocol.NewErrorResponse(id, &protocol.RPCError{Code: protocol.CodeMethodNotFound, Message: "method not found"})
	}

	res, err := handler.Handle(ctx, msg.Params)
	if err != nil {
		return protocol.NewErrorResponse(id, toRPCError(err))
	}
	m, err := protocol.NewResponse(*id, res)
	if err != nil {
		return protocol.NewErrorResponse(id, &protocol.RPCError{Code: protocol.CodeInternalError, Message: "internal error"})
	}
	return m
}

func (s *RPCServer) logRequestSummary(msg protocol.Message) {
	method := strings.TrimSpace(msg.Method)
	if method == "" {
		return
	}
	id := ""
	if msg.ID != nil {
		id = strings.TrimSpace(*msg.ID)
	}
	log.Printf("rpc.request id=%q jsonrpc=%q method=%q params_bytes=%d", id, strings.TrimSpace(msg.JSONRPC), method, len(msg.Params))
}

func (s *RPCServer) logResponseSummary(msg protocol.Message) {
	id := ""
	if msg.ID != nil {
		id = strings.TrimSpace(*msg.ID)
	}
	if msg.Error != nil {
		log.Printf("rpc.response id=%q error_code=%d error_message=%q", id, msg.Error.Code, strings.TrimSpace(msg.Error.Message))
		return
	}
	log.Printf("rpc.response id=%q ok result_bytes=%d", id, len(msg.Result))
}

func (s *RPCServer) logNotificationSummary(msg protocol.Message) {
	method := strings.TrimSpace(msg.Method)
	if method == "" {
		return
	}
	log.Printf("rpc.notify method=%q params_bytes=%d", method, len(msg.Params))
}

func (s *RPCServer) resolveThreadID(threadID protocol.ThreadID) (string, error) {
	thread := strings.TrimSpace(string(threadID))
	if thread == "" {
		return "", &protocol.ProtocolError{Code: protocol.CodeInvalidParams, Message: "threadId is required"}
	}
	if s.allowAnyThread {
		return thread, nil
	}
	if thread != strings.TrimSpace(s.run.SessionID) {
		return "", &protocol.ProtocolError{Code: protocol.CodeThreadNotFound, Message: "thread not found"}
	}
	return thread, nil
}

func (s *RPCServer) loadSessionForID(ctx context.Context, sessionID string) (types.Session, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return types.Session{}, &protocol.ProtocolError{Code: protocol.CodeInvalidParams, Message: "sessionId is required"}
	}
	sess, err := s.session.LoadSession(ctx, sessionID)
	if err != nil {
		return types.Session{}, &protocol.ProtocolError{Code: protocol.CodeThreadNotFound, Message: "thread not found"}
	}
	if strings.TrimSpace(sess.SessionID) != sessionID {
		return types.Session{}, &protocol.ProtocolError{Code: protocol.CodeThreadNotFound, Message: "thread not found"}
	}
	return sess, nil
}

func toRPCError(err error) *protocol.RPCError {
	var pe *protocol.ProtocolError
	if errors.As(err, &pe) {
		return pe.RPC()
	}
	return &protocol.RPCError{Code: protocol.CodeInternalError, Message: "internal error"}
}

func threadFromSession(activeRunID string, sess types.Session) protocol.Thread {
	createdAt := timeutil.OrNow(sess.CreatedAt)
	return protocol.Thread{
		ID:          protocol.ThreadID(strings.TrimSpace(sess.SessionID)),
		Title:       strings.TrimSpace(sess.Title),
		CreatedAt:   createdAt,
		ActiveModel: strings.TrimSpace(sess.ActiveModel),
		ActiveReasoningEffort: func() string {
			effort, _ := sessionReasoningForModel(sess, strings.TrimSpace(sess.ActiveModel), "", "")
			return effort
		}(),
		ActiveReasoningSummary: func() string {
			_, summary := sessionReasoningForModel(sess, strings.TrimSpace(sess.ActiveModel), "", "")
			return summary
		}(),
		ActiveRunID: protocol.RunID(strings.TrimSpace(activeRunID)),

		InputTokens:  sess.InputTokens,
		OutputTokens: sess.OutputTokens,
		TotalTokens:  sess.TotalTokens,
		CostUSD:      sess.CostUSD,
	}
}
