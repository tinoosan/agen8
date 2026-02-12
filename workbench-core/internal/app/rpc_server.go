package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	implstore "github.com/tinoosan/workbench-core/internal/store"
	"github.com/tinoosan/workbench-core/pkg/agent/state"
	"github.com/tinoosan/workbench-core/pkg/config"
	"github.com/tinoosan/workbench-core/pkg/cost"
	"github.com/tinoosan/workbench-core/pkg/fsutil"
	"github.com/tinoosan/workbench-core/pkg/profile"
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

	controlSetModel     func(ctx context.Context, threadID, target, model string) ([]string, error)
	controlSetReasoning func(ctx context.Context, threadID, target, effort, summary string) ([]string, error)
	controlSetProfile   func(ctx context.Context, threadID, target, profile string) ([]string, error)
	agentPause          func(ctx context.Context, threadID, runID string) error
	agentResume         func(ctx context.Context, threadID, runID string) error
	sessionPause        func(ctx context.Context, threadID, sessionID string) ([]string, error)
	sessionResume       func(ctx context.Context, threadID, sessionID string) ([]string, error)
}

const (
	artifactListDefaultLimit = 200
	artifactListMaxLimit     = 2000
	artifactSearchDefault    = 100
	artifactSearchMax        = 1000
	artifactGetDefaultBytes  = 256 * 1024
	artifactGetMaxBytes      = 1024 * 1024
)

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
	return &RPCServer{
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
	}
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

	switch strings.TrimSpace(msg.Method) {
	case protocol.MethodThreadGet:
		var p protocol.ThreadGetParams
		if err := json.Unmarshal(msg.Params, &p); err != nil {
			return protocol.NewErrorResponse(id, &protocol.RPCError{Code: protocol.CodeInvalidParams, Message: "invalid params"})
		}
		res, err := s.threadGet(ctx, p)
		if err != nil {
			return protocol.NewErrorResponse(id, toRPCError(err))
		}
		m, err := protocol.NewResponse(*id, res)
		if err != nil {
			return protocol.NewErrorResponse(id, &protocol.RPCError{Code: protocol.CodeInternalError, Message: "internal error"})
		}
		return m

	case protocol.MethodThreadCreate:
		var p protocol.ThreadCreateParams
		if len(msg.Params) != 0 {
			if err := json.Unmarshal(msg.Params, &p); err != nil {
				return protocol.NewErrorResponse(id, &protocol.RPCError{Code: protocol.CodeInvalidParams, Message: "invalid params"})
			}
		}
		res, err := s.threadCreate(ctx, p)
		if err != nil {
			return protocol.NewErrorResponse(id, toRPCError(err))
		}
		m, err := protocol.NewResponse(*id, res)
		if err != nil {
			return protocol.NewErrorResponse(id, &protocol.RPCError{Code: protocol.CodeInternalError, Message: "internal error"})
		}
		return m

	case protocol.MethodTurnCreate:
		var p protocol.TurnCreateParams
		if err := json.Unmarshal(msg.Params, &p); err != nil {
			return protocol.NewErrorResponse(id, &protocol.RPCError{Code: protocol.CodeInvalidParams, Message: "invalid params"})
		}
		res, err := s.turnCreate(ctx, p)
		if err != nil {
			return protocol.NewErrorResponse(id, toRPCError(err))
		}
		m, err := protocol.NewResponse(*id, res)
		if err != nil {
			return protocol.NewErrorResponse(id, &protocol.RPCError{Code: protocol.CodeInternalError, Message: "internal error"})
		}
		return m

	case protocol.MethodTurnCancel:
		var p protocol.TurnCancelParams
		if err := json.Unmarshal(msg.Params, &p); err != nil {
			return protocol.NewErrorResponse(id, &protocol.RPCError{Code: protocol.CodeInvalidParams, Message: "invalid params"})
		}
		res, err := s.turnCancel(ctx, p)
		if err != nil {
			return protocol.NewErrorResponse(id, toRPCError(err))
		}
		m, err := protocol.NewResponse(*id, res)
		if err != nil {
			return protocol.NewErrorResponse(id, &protocol.RPCError{Code: protocol.CodeInternalError, Message: "internal error"})
		}
		return m

	case protocol.MethodItemList:
		var p protocol.ItemListParams
		if err := json.Unmarshal(msg.Params, &p); err != nil {
			return protocol.NewErrorResponse(id, &protocol.RPCError{Code: protocol.CodeInvalidParams, Message: "invalid params"})
		}
		res, err := s.itemList(ctx, p)
		if err != nil {
			return protocol.NewErrorResponse(id, toRPCError(err))
		}
		m, err := protocol.NewResponse(*id, res)
		if err != nil {
			return protocol.NewErrorResponse(id, &protocol.RPCError{Code: protocol.CodeInternalError, Message: "internal error"})
		}
		return m
	case protocol.MethodTaskList:
		var p protocol.TaskListParams
		if err := json.Unmarshal(msg.Params, &p); err != nil {
			return protocol.NewErrorResponse(id, &protocol.RPCError{Code: protocol.CodeInvalidParams, Message: "invalid params"})
		}
		res, err := s.taskList(ctx, p)
		if err != nil {
			return protocol.NewErrorResponse(id, toRPCError(err))
		}
		m, err := protocol.NewResponse(*id, res)
		if err != nil {
			return protocol.NewErrorResponse(id, &protocol.RPCError{Code: protocol.CodeInternalError, Message: "internal error"})
		}
		return m
	case protocol.MethodTaskCreate:
		var p protocol.TaskCreateParams
		if err := json.Unmarshal(msg.Params, &p); err != nil {
			return protocol.NewErrorResponse(id, &protocol.RPCError{Code: protocol.CodeInvalidParams, Message: "invalid params"})
		}
		res, err := s.taskCreate(ctx, p)
		if err != nil {
			return protocol.NewErrorResponse(id, toRPCError(err))
		}
		m, err := protocol.NewResponse(*id, res)
		if err != nil {
			return protocol.NewErrorResponse(id, &protocol.RPCError{Code: protocol.CodeInternalError, Message: "internal error"})
		}
		return m
	case protocol.MethodTaskClaim:
		var p protocol.TaskClaimParams
		if err := json.Unmarshal(msg.Params, &p); err != nil {
			return protocol.NewErrorResponse(id, &protocol.RPCError{Code: protocol.CodeInvalidParams, Message: "invalid params"})
		}
		res, err := s.taskClaim(ctx, p)
		if err != nil {
			return protocol.NewErrorResponse(id, toRPCError(err))
		}
		m, err := protocol.NewResponse(*id, res)
		if err != nil {
			return protocol.NewErrorResponse(id, &protocol.RPCError{Code: protocol.CodeInternalError, Message: "internal error"})
		}
		return m
	case protocol.MethodTaskComplete:
		var p protocol.TaskCompleteParams
		if err := json.Unmarshal(msg.Params, &p); err != nil {
			return protocol.NewErrorResponse(id, &protocol.RPCError{Code: protocol.CodeInvalidParams, Message: "invalid params"})
		}
		res, err := s.taskComplete(ctx, p)
		if err != nil {
			return protocol.NewErrorResponse(id, toRPCError(err))
		}
		m, err := protocol.NewResponse(*id, res)
		if err != nil {
			return protocol.NewErrorResponse(id, &protocol.RPCError{Code: protocol.CodeInternalError, Message: "internal error"})
		}
		return m
	case protocol.MethodSessionStart:
		var p protocol.SessionStartParams
		if err := json.Unmarshal(msg.Params, &p); err != nil {
			return protocol.NewErrorResponse(id, &protocol.RPCError{Code: protocol.CodeInvalidParams, Message: "invalid params"})
		}
		res, err := s.sessionStart(ctx, p)
		if err != nil {
			return protocol.NewErrorResponse(id, toRPCError(err))
		}
		m, err := protocol.NewResponse(*id, res)
		if err != nil {
			return protocol.NewErrorResponse(id, &protocol.RPCError{Code: protocol.CodeInternalError, Message: "internal error"})
		}
		return m
	case protocol.MethodSessionList:
		var p protocol.SessionListParams
		if err := json.Unmarshal(msg.Params, &p); err != nil {
			return protocol.NewErrorResponse(id, &protocol.RPCError{Code: protocol.CodeInvalidParams, Message: "invalid params"})
		}
		res, err := s.sessionList(ctx, p)
		if err != nil {
			return protocol.NewErrorResponse(id, toRPCError(err))
		}
		m, err := protocol.NewResponse(*id, res)
		if err != nil {
			return protocol.NewErrorResponse(id, &protocol.RPCError{Code: protocol.CodeInternalError, Message: "internal error"})
		}
		return m
	case protocol.MethodSessionRename:
		var p protocol.SessionRenameParams
		if err := json.Unmarshal(msg.Params, &p); err != nil {
			return protocol.NewErrorResponse(id, &protocol.RPCError{Code: protocol.CodeInvalidParams, Message: "invalid params"})
		}
		res, err := s.sessionRename(ctx, p)
		if err != nil {
			return protocol.NewErrorResponse(id, toRPCError(err))
		}
		m, err := protocol.NewResponse(*id, res)
		if err != nil {
			return protocol.NewErrorResponse(id, &protocol.RPCError{Code: protocol.CodeInternalError, Message: "internal error"})
		}
		return m
	case protocol.MethodAgentList:
		var p protocol.AgentListParams
		if err := json.Unmarshal(msg.Params, &p); err != nil {
			return protocol.NewErrorResponse(id, &protocol.RPCError{Code: protocol.CodeInvalidParams, Message: "invalid params"})
		}
		res, err := s.agentList(ctx, p)
		if err != nil {
			return protocol.NewErrorResponse(id, toRPCError(err))
		}
		m, err := protocol.NewResponse(*id, res)
		if err != nil {
			return protocol.NewErrorResponse(id, &protocol.RPCError{Code: protocol.CodeInternalError, Message: "internal error"})
		}
		return m
	case protocol.MethodAgentStart:
		var p protocol.AgentStartParams
		if err := json.Unmarshal(msg.Params, &p); err != nil {
			return protocol.NewErrorResponse(id, &protocol.RPCError{Code: protocol.CodeInvalidParams, Message: "invalid params"})
		}
		res, err := s.agentStart(ctx, p)
		if err != nil {
			return protocol.NewErrorResponse(id, toRPCError(err))
		}
		m, err := protocol.NewResponse(*id, res)
		if err != nil {
			return protocol.NewErrorResponse(id, &protocol.RPCError{Code: protocol.CodeInternalError, Message: "internal error"})
		}
		return m
	case protocol.MethodAgentPause:
		var p protocol.AgentPauseParams
		if err := json.Unmarshal(msg.Params, &p); err != nil {
			return protocol.NewErrorResponse(id, &protocol.RPCError{Code: protocol.CodeInvalidParams, Message: "invalid params"})
		}
		res, err := s.agentPauseHandler(ctx, p)
		if err != nil {
			return protocol.NewErrorResponse(id, toRPCError(err))
		}
		m, err := protocol.NewResponse(*id, res)
		if err != nil {
			return protocol.NewErrorResponse(id, &protocol.RPCError{Code: protocol.CodeInternalError, Message: "internal error"})
		}
		return m
	case protocol.MethodAgentResume:
		var p protocol.AgentResumeParams
		if err := json.Unmarshal(msg.Params, &p); err != nil {
			return protocol.NewErrorResponse(id, &protocol.RPCError{Code: protocol.CodeInvalidParams, Message: "invalid params"})
		}
		res, err := s.agentResumeHandler(ctx, p)
		if err != nil {
			return protocol.NewErrorResponse(id, toRPCError(err))
		}
		m, err := protocol.NewResponse(*id, res)
		if err != nil {
			return protocol.NewErrorResponse(id, &protocol.RPCError{Code: protocol.CodeInternalError, Message: "internal error"})
		}
		return m
	case protocol.MethodSessionPause:
		var p protocol.SessionPauseParams
		if err := json.Unmarshal(msg.Params, &p); err != nil {
			return protocol.NewErrorResponse(id, &protocol.RPCError{Code: protocol.CodeInvalidParams, Message: "invalid params"})
		}
		res, err := s.sessionPauseHandler(ctx, p)
		if err != nil {
			return protocol.NewErrorResponse(id, toRPCError(err))
		}
		m, err := protocol.NewResponse(*id, res)
		if err != nil {
			return protocol.NewErrorResponse(id, &protocol.RPCError{Code: protocol.CodeInternalError, Message: "internal error"})
		}
		return m
	case protocol.MethodSessionResume:
		var p protocol.SessionResumeParams
		if err := json.Unmarshal(msg.Params, &p); err != nil {
			return protocol.NewErrorResponse(id, &protocol.RPCError{Code: protocol.CodeInvalidParams, Message: "invalid params"})
		}
		res, err := s.sessionResumeHandler(ctx, p)
		if err != nil {
			return protocol.NewErrorResponse(id, toRPCError(err))
		}
		m, err := protocol.NewResponse(*id, res)
		if err != nil {
			return protocol.NewErrorResponse(id, &protocol.RPCError{Code: protocol.CodeInternalError, Message: "internal error"})
		}
		return m
	case protocol.MethodSessionGetTotals:
		var p protocol.SessionGetTotalsParams
		if err := json.Unmarshal(msg.Params, &p); err != nil {
			return protocol.NewErrorResponse(id, &protocol.RPCError{Code: protocol.CodeInvalidParams, Message: "invalid params"})
		}
		res, err := s.sessionGetTotals(ctx, p)
		if err != nil {
			return protocol.NewErrorResponse(id, toRPCError(err))
		}
		m, err := protocol.NewResponse(*id, res)
		if err != nil {
			return protocol.NewErrorResponse(id, &protocol.RPCError{Code: protocol.CodeInternalError, Message: "internal error"})
		}
		return m
	case protocol.MethodActivityList:
		var p protocol.ActivityListParams
		if err := json.Unmarshal(msg.Params, &p); err != nil {
			return protocol.NewErrorResponse(id, &protocol.RPCError{Code: protocol.CodeInvalidParams, Message: "invalid params"})
		}
		res, err := s.activityList(ctx, p)
		if err != nil {
			return protocol.NewErrorResponse(id, toRPCError(err))
		}
		m, err := protocol.NewResponse(*id, res)
		if err != nil {
			return protocol.NewErrorResponse(id, &protocol.RPCError{Code: protocol.CodeInternalError, Message: "internal error"})
		}
		return m
	case protocol.MethodTeamGetStatus:
		var p protocol.TeamGetStatusParams
		if err := json.Unmarshal(msg.Params, &p); err != nil {
			return protocol.NewErrorResponse(id, &protocol.RPCError{Code: protocol.CodeInvalidParams, Message: "invalid params"})
		}
		res, err := s.teamGetStatus(ctx, p)
		if err != nil {
			return protocol.NewErrorResponse(id, toRPCError(err))
		}
		m, err := protocol.NewResponse(*id, res)
		if err != nil {
			return protocol.NewErrorResponse(id, &protocol.RPCError{Code: protocol.CodeInternalError, Message: "internal error"})
		}
		return m
	case protocol.MethodTeamGetManifest:
		var p protocol.TeamGetManifestParams
		if err := json.Unmarshal(msg.Params, &p); err != nil {
			return protocol.NewErrorResponse(id, &protocol.RPCError{Code: protocol.CodeInvalidParams, Message: "invalid params"})
		}
		res, err := s.teamGetManifest(ctx, p)
		if err != nil {
			return protocol.NewErrorResponse(id, toRPCError(err))
		}
		m, err := protocol.NewResponse(*id, res)
		if err != nil {
			return protocol.NewErrorResponse(id, &protocol.RPCError{Code: protocol.CodeInternalError, Message: "internal error"})
		}
		return m
	case protocol.MethodPlanGet:
		var p protocol.PlanGetParams
		if err := json.Unmarshal(msg.Params, &p); err != nil {
			return protocol.NewErrorResponse(id, &protocol.RPCError{Code: protocol.CodeInvalidParams, Message: "invalid params"})
		}
		res, err := s.planGet(ctx, p)
		if err != nil {
			return protocol.NewErrorResponse(id, toRPCError(err))
		}
		m, err := protocol.NewResponse(*id, res)
		if err != nil {
			return protocol.NewErrorResponse(id, &protocol.RPCError{Code: protocol.CodeInternalError, Message: "internal error"})
		}
		return m
	case protocol.MethodModelList:
		var p protocol.ModelListParams
		if err := json.Unmarshal(msg.Params, &p); err != nil {
			return protocol.NewErrorResponse(id, &protocol.RPCError{Code: protocol.CodeInvalidParams, Message: "invalid params"})
		}
		res, err := s.modelList(ctx, p)
		if err != nil {
			return protocol.NewErrorResponse(id, toRPCError(err))
		}
		m, err := protocol.NewResponse(*id, res)
		if err != nil {
			return protocol.NewErrorResponse(id, &protocol.RPCError{Code: protocol.CodeInternalError, Message: "internal error"})
		}
		return m
	case protocol.MethodControlSetModel:
		var p protocol.ControlSetModelParams
		if err := json.Unmarshal(msg.Params, &p); err != nil {
			return protocol.NewErrorResponse(id, &protocol.RPCError{Code: protocol.CodeInvalidParams, Message: "invalid params"})
		}
		res, err := s.controlSetModelHandler(ctx, p)
		if err != nil {
			return protocol.NewErrorResponse(id, toRPCError(err))
		}
		m, err := protocol.NewResponse(*id, res)
		if err != nil {
			return protocol.NewErrorResponse(id, &protocol.RPCError{Code: protocol.CodeInternalError, Message: "internal error"})
		}
		return m
	case protocol.MethodControlSetReasoning:
		var p protocol.ControlSetReasoningParams
		if err := json.Unmarshal(msg.Params, &p); err != nil {
			return protocol.NewErrorResponse(id, &protocol.RPCError{Code: protocol.CodeInvalidParams, Message: "invalid params"})
		}
		res, err := s.controlSetReasoningHandler(ctx, p)
		if err != nil {
			return protocol.NewErrorResponse(id, toRPCError(err))
		}
		m, err := protocol.NewResponse(*id, res)
		if err != nil {
			return protocol.NewErrorResponse(id, &protocol.RPCError{Code: protocol.CodeInternalError, Message: "internal error"})
		}
		return m
	case protocol.MethodControlSetProfile:
		var p protocol.ControlSetProfileParams
		if err := json.Unmarshal(msg.Params, &p); err != nil {
			return protocol.NewErrorResponse(id, &protocol.RPCError{Code: protocol.CodeInvalidParams, Message: "invalid params"})
		}
		res, err := s.controlSetProfileHandler(ctx, p)
		if err != nil {
			return protocol.NewErrorResponse(id, toRPCError(err))
		}
		m, err := protocol.NewResponse(*id, res)
		if err != nil {
			return protocol.NewErrorResponse(id, &protocol.RPCError{Code: protocol.CodeInternalError, Message: "internal error"})
		}
		return m
	case protocol.MethodArtifactList:
		var p protocol.ArtifactListParams
		if err := json.Unmarshal(msg.Params, &p); err != nil {
			return protocol.NewErrorResponse(id, &protocol.RPCError{Code: protocol.CodeInvalidParams, Message: "invalid params"})
		}
		res, err := s.artifactList(ctx, p)
		if err != nil {
			return protocol.NewErrorResponse(id, toRPCError(err))
		}
		m, err := protocol.NewResponse(*id, res)
		if err != nil {
			return protocol.NewErrorResponse(id, &protocol.RPCError{Code: protocol.CodeInternalError, Message: "internal error"})
		}
		return m
	case protocol.MethodArtifactSearch:
		var p protocol.ArtifactSearchParams
		if err := json.Unmarshal(msg.Params, &p); err != nil {
			return protocol.NewErrorResponse(id, &protocol.RPCError{Code: protocol.CodeInvalidParams, Message: "invalid params"})
		}
		res, err := s.artifactSearch(ctx, p)
		if err != nil {
			return protocol.NewErrorResponse(id, toRPCError(err))
		}
		m, err := protocol.NewResponse(*id, res)
		if err != nil {
			return protocol.NewErrorResponse(id, &protocol.RPCError{Code: protocol.CodeInternalError, Message: "internal error"})
		}
		return m
	case protocol.MethodArtifactGet:
		var p protocol.ArtifactGetParams
		if err := json.Unmarshal(msg.Params, &p); err != nil {
			return protocol.NewErrorResponse(id, &protocol.RPCError{Code: protocol.CodeInvalidParams, Message: "invalid params"})
		}
		res, err := s.artifactGet(ctx, p)
		if err != nil {
			return protocol.NewErrorResponse(id, toRPCError(err))
		}
		m, err := protocol.NewResponse(*id, res)
		if err != nil {
			return protocol.NewErrorResponse(id, &protocol.RPCError{Code: protocol.CodeInternalError, Message: "internal error"})
		}
		return m

	default:
		return protocol.NewErrorResponse(id, &protocol.RPCError{Code: protocol.CodeMethodNotFound, Message: "method not found"})
	}
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

func defaultRunIDForSession(sess types.Session) string {
	runID := strings.TrimSpace(sess.CurrentRunID)
	if runID != "" {
		return runID
	}
	for _, candidate := range sess.Runs {
		candidate = strings.TrimSpace(candidate)
		if candidate != "" {
			return candidate
		}
	}
	return ""
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

func parseAssignee(assignee string) (string, string) {
	assignee = strings.TrimSpace(assignee)
	if assignee == "" {
		return "", ""
	}
	for _, pfx := range []string{"team:", "role:", "agent:"} {
		if strings.HasPrefix(assignee, pfx) {
			return strings.TrimSuffix(pfx, ":"), strings.TrimSpace(strings.TrimPrefix(assignee, pfx))
		}
	}
	return "", assignee
}

func protocolTaskFromTypesTask(t types.Task) protocol.Task {
	return protocol.Task{
		ID:               strings.TrimSpace(t.TaskID),
		ThreadID:         protocol.ThreadID(strings.TrimSpace(t.SessionID)),
		RunID:            protocol.RunID(strings.TrimSpace(t.RunID)),
		TeamID:           strings.TrimSpace(t.TeamID),
		TaskKind:         strings.TrimSpace(t.TaskKind),
		AssignedToType:   strings.TrimSpace(t.AssignedToType),
		AssignedTo:       strings.TrimSpace(t.AssignedTo),
		AssignedRole:     strings.TrimSpace(t.AssignedRole),
		ClaimedByAgentID: strings.TrimSpace(t.ClaimedByAgentID),
		RoleSnapshot:     strings.TrimSpace(t.RoleSnapshot),
		Goal:             strings.TrimSpace(t.Goal),
		Status:           strings.TrimSpace(string(t.Status)),
		Summary:          strings.TrimSpace(t.Summary),
		Error:            strings.TrimSpace(t.Error),
		Artifacts:        append([]string(nil), t.Artifacts...),
		InputTokens:      t.InputTokens,
		OutputTokens:     t.OutputTokens,
		TotalTokens:      t.TotalTokens,
		CostUSD:          t.CostUSD,
		CreatedAt:        timeutil.OrNow(t.CreatedAt),
		CompletedAt:      timeutil.OrNow(t.CompletedAt),
	}
}

func (s *RPCServer) taskList(ctx context.Context, p protocol.TaskListParams) (protocol.TaskListResult, error) {
	scope, err := s.resolveTeamOrRunScope(ctx, p.ThreadID, p.TeamID, p.RunID)
	if err != nil {
		return protocol.TaskListResult{}, err
	}
	view := strings.ToLower(strings.TrimSpace(p.View))
	if view == "" {
		view = "inbox"
	}
	if view != "inbox" && view != "outbox" {
		return protocol.TaskListResult{}, &protocol.ProtocolError{Code: protocol.CodeInvalidParams, Message: "view must be inbox or outbox"}
	}
	filter := state.TaskFilter{
		TeamID:   scope.teamID,
		RunID:    scope.runID,
		SortBy:   "created_at",
		SortDesc: true,
		Limit:    clampLimit(p.Limit, 200, 2000),
		Offset:   max(0, p.Offset),
	}
	switch view {
	case "inbox":
		filter.Status = []types.TaskStatus{types.TaskStatusPending, types.TaskStatusActive}
		filter.SortBy = "created_at"
		filter.SortDesc = false
	case "outbox":
		filter.Status = []types.TaskStatus{types.TaskStatusSucceeded, types.TaskStatusFailed, types.TaskStatusCanceled}
		filter.SortBy = "finished_at"
		filter.SortDesc = true
	}
	if at, av := parseAssignee(p.Assignee); av != "" {
		filter.AssignedTo = av
		filter.AssignedToType = at
	}
	tasks, err := s.taskStore.ListTasks(ctx, filter)
	if err != nil {
		return protocol.TaskListResult{}, err
	}
	total, err := s.taskStore.CountTasks(ctx, filter)
	if err != nil {
		return protocol.TaskListResult{}, err
	}
	out := make([]protocol.Task, 0, len(tasks))
	for _, t := range tasks {
		out = append(out, protocolTaskFromTypesTask(t))
	}
	return protocol.TaskListResult{Tasks: out, TotalCount: total}, nil
}

func (s *RPCServer) controlSetModelHandler(ctx context.Context, p protocol.ControlSetModelParams) (protocol.ControlSetModelResult, error) {
	threadID, err := s.resolveThreadID(p.ThreadID)
	if err != nil {
		return protocol.ControlSetModelResult{}, err
	}
	model := strings.TrimSpace(p.Model)
	if model == "" {
		return protocol.ControlSetModelResult{}, &protocol.ProtocolError{Code: protocol.CodeInvalidParams, Message: "model is required"}
	}
	if s.controlSetModel == nil {
		return protocol.ControlSetModelResult{}, &protocol.ProtocolError{Code: protocol.CodeInvalidState, Message: "control.setModel is unavailable"}
	}
	appliedTo, err := s.controlSetModel(ctx, threadID, strings.TrimSpace(p.Target), model)
	if err != nil {
		return protocol.ControlSetModelResult{}, err
	}
	return protocol.ControlSetModelResult{
		Accepted:  true,
		AppliedTo: append([]string(nil), appliedTo...),
	}, nil
}

func normalizeReasoningEffort(v string) (string, error) {
	v = strings.ToLower(strings.TrimSpace(v))
	switch v {
	case "":
		return "", nil
	case "none", "minimal", "low", "medium", "high", "xhigh":
		return v, nil
	default:
		return "", &protocol.ProtocolError{Code: protocol.CodeInvalidParams, Message: "effort must be one of none|minimal|low|medium|high|xhigh"}
	}
}

func normalizeReasoningSummary(v string) (string, error) {
	v = strings.ToLower(strings.TrimSpace(v))
	if v == "none" {
		v = "off"
	}
	switch v {
	case "":
		return "", nil
	case "off", "auto", "concise", "detailed":
		return v, nil
	default:
		return "", &protocol.ProtocolError{Code: protocol.CodeInvalidParams, Message: "summary must be one of off|auto|concise|detailed"}
	}
}

func (s *RPCServer) controlSetReasoningHandler(ctx context.Context, p protocol.ControlSetReasoningParams) (protocol.ControlSetReasoningResult, error) {
	threadID, err := s.resolveThreadID(p.ThreadID)
	if err != nil {
		return protocol.ControlSetReasoningResult{}, err
	}
	effort, err := normalizeReasoningEffort(p.Effort)
	if err != nil {
		return protocol.ControlSetReasoningResult{}, err
	}
	summary, err := normalizeReasoningSummary(p.Summary)
	if err != nil {
		return protocol.ControlSetReasoningResult{}, err
	}
	if effort == "" && summary == "" {
		return protocol.ControlSetReasoningResult{}, &protocol.ProtocolError{Code: protocol.CodeInvalidParams, Message: "effort or summary is required"}
	}
	if s.controlSetReasoning == nil {
		return protocol.ControlSetReasoningResult{}, &protocol.ProtocolError{Code: protocol.CodeInvalidState, Message: "control.setReasoning is unavailable"}
	}
	appliedTo, err := s.controlSetReasoning(ctx, threadID, strings.TrimSpace(p.Target), effort, summary)
	if err != nil {
		return protocol.ControlSetReasoningResult{}, err
	}
	return protocol.ControlSetReasoningResult{
		Accepted:  true,
		AppliedTo: append([]string(nil), appliedTo...),
		Effort:    effort,
		Summary:   summary,
	}, nil
}

func (s *RPCServer) controlSetProfileHandler(ctx context.Context, p protocol.ControlSetProfileParams) (protocol.ControlSetProfileResult, error) {
	threadID, err := s.resolveThreadID(p.ThreadID)
	if err != nil {
		return protocol.ControlSetProfileResult{}, err
	}
	profile := strings.TrimSpace(p.Profile)
	if profile == "" {
		return protocol.ControlSetProfileResult{}, &protocol.ProtocolError{Code: protocol.CodeInvalidParams, Message: "profile is required"}
	}
	if s.controlSetProfile == nil {
		return protocol.ControlSetProfileResult{}, &protocol.ProtocolError{Code: protocol.CodeInvalidState, Message: "control.setProfile is unavailable"}
	}
	appliedTo, err := s.controlSetProfile(ctx, threadID, strings.TrimSpace(p.Target), profile)
	if err != nil {
		return protocol.ControlSetProfileResult{}, err
	}
	return protocol.ControlSetProfileResult{
		Accepted:                true,
		AppliedTo:               append([]string(nil), appliedTo...),
		PreservesSessionContext: true,
	}, nil
}

func (s *RPCServer) sessionStart(ctx context.Context, p protocol.SessionStartParams) (protocol.SessionStartResult, error) {
	if _, err := s.resolveThreadID(p.ThreadID); err != nil {
		return protocol.SessionStartResult{}, err
	}
	mode := strings.ToLower(strings.TrimSpace(p.Mode))
	if mode == "" {
		mode = "standalone"
	}
	if mode != "standalone" && mode != "team" {
		return protocol.SessionStartResult{}, &protocol.ProtocolError{
			Code:    protocol.CodeInvalidParams,
			Message: "mode must be standalone or team",
		}
	}
	if mode == "team" {
		return s.sessionStartTeam(ctx, p)
	}

	goal := strings.TrimSpace(p.Goal)
	if goal == "" {
		goal = "autonomous agent"
	}
	maxContext := s.run.MaxBytesForContext
	if maxContext <= 0 {
		maxContext = 8 * 1024
	}
	sess, run, err := implstore.CreateSession(s.cfg, goal, maxContext)
	if err != nil {
		return protocol.SessionStartResult{}, err
	}
	sess.Mode = "standalone"
	sess.TeamID = ""
	sess.Profile = strings.TrimSpace(p.Profile)

	activeModel := strings.TrimSpace(p.Model)
	if activeModel == "" && run.Runtime != nil {
		activeModel = strings.TrimSpace(run.Runtime.Model)
	}
	if activeModel != "" {
		sess.ActiveModel = activeModel
	}
	ensureSessionReasoningForModel(&sess, sess.ActiveModel, "", "")
	if err := s.session.SaveSession(ctx, sess); err != nil {
		return protocol.SessionStartResult{}, err
	}
	if strings.TrimSpace(p.Profile) != "" || activeModel != "" {
		if created, err := implstore.LoadRun(s.cfg, strings.TrimSpace(run.RunID)); err == nil {
			if created.Runtime == nil {
				created.Runtime = &types.RunRuntimeConfig{}
			}
			if profileRef := strings.TrimSpace(p.Profile); profileRef != "" {
				created.Runtime.Profile = profileRef
			}
			if activeModel != "" {
				created.Runtime.Model = activeModel
			}
			_ = implstore.SaveRun(s.cfg, created)
		}
	}

	return protocol.SessionStartResult{
		SessionID:    strings.TrimSpace(sess.SessionID),
		PrimaryRunID: strings.TrimSpace(run.RunID),
		Mode:         "standalone",
		Profile:      strings.TrimSpace(p.Profile),
		Model:        activeModel,
		RunIDs:       []string{strings.TrimSpace(run.RunID)},
	}, nil
}

func (s *RPCServer) sessionStartTeam(ctx context.Context, p protocol.SessionStartParams) (protocol.SessionStartResult, error) {
	profileRef := strings.TrimSpace(p.Profile)
	if profileRef == "" {
		return protocol.SessionStartResult{}, &protocol.ProtocolError{Code: protocol.CodeInvalidParams, Message: "team profile is required"}
	}
	prof, _, err := resolveProfileRef(s.cfg, profileRef)
	if err != nil {
		return protocol.SessionStartResult{}, &protocol.ProtocolError{Code: protocol.CodeInvalidParams, Message: "load profile: " + err.Error()}
	}
	if prof == nil || prof.Team == nil {
		return protocol.SessionStartResult{}, &protocol.ProtocolError{Code: protocol.CodeInvalidParams, Message: "profile is not a team profile"}
	}
	_, coordinatorRole, err := collectTeamRoles(prof.Team.Roles)
	if err != nil {
		return protocol.SessionStartResult{}, &protocol.ProtocolError{Code: protocol.CodeInvalidParams, Message: err.Error()}
	}

	goal := strings.TrimSpace(p.Goal)
	if goal == "" {
		goal = "team session (" + strings.TrimSpace(prof.ID) + ")"
	}
	maxContext := s.run.MaxBytesForContext
	if maxContext <= 0 {
		maxContext = 8 * 1024
	}
	sess := types.NewSession(goal)
	sess.CurrentGoal = goal
	sess.Mode = "team"
	sess.Profile = strings.TrimSpace(prof.ID)
	teamID := "team-" + uuid.NewString()
	sess.TeamID = teamID

	teamModel := strings.TrimSpace(p.Model)
	if teamModel == "" && prof.Team != nil {
		teamModel = strings.TrimSpace(prof.Team.Model)
	}
	if teamModel != "" {
		sess.ActiveModel = teamModel
	}
	ensureSessionReasoningForModel(&sess, sess.ActiveModel, "", "")
	if err := s.session.SaveSession(ctx, sess); err != nil {
		return protocol.SessionStartResult{}, err
	}

	runtimes := make([]teamRoleRuntime, 0, len(prof.Team.Roles))
	runIDs := make([]string, 0, len(prof.Team.Roles))
	primaryRunID := ""
	for _, role := range prof.Team.Roles {
		roleName := strings.TrimSpace(role.Name)
		if roleName == "" {
			continue
		}
		roleGoal := strings.TrimSpace(role.Description)
		if roleGoal == "" {
			roleGoal = goal
		}
		run := types.NewRun(roleGoal, maxContext, strings.TrimSpace(sess.SessionID))
		run.Runtime = &types.RunRuntimeConfig{
			Profile: strings.TrimSpace(prof.ID),
			Model:   strings.TrimSpace(teamModel),
			TeamID:  strings.TrimSpace(teamID),
			Role:    roleName,
		}
		if err := implstore.SaveRun(s.cfg, run); err != nil {
			return protocol.SessionStartResult{}, err
		}
		runID := strings.TrimSpace(run.RunID)
		exists := false
		for _, id := range sess.Runs {
			if strings.TrimSpace(id) == runID {
				exists = true
				break
			}
		}
		if !exists {
			sess.Runs = append(sess.Runs, runID)
		}
		runtimes = append(runtimes, teamRoleRuntime{
			role: profile.RoleConfig{
				Name:        roleName,
				Description: strings.TrimSpace(role.Description),
			},
			run: run,
		})
		runIDs = append(runIDs, runID)
		if strings.EqualFold(roleName, coordinatorRole) && primaryRunID == "" {
			primaryRunID = runID
		}
	}
	if primaryRunID == "" && len(runIDs) > 0 {
		primaryRunID = runIDs[0]
	}
	if primaryRunID == "" {
		return protocol.SessionStartResult{}, &protocol.ProtocolError{Code: protocol.CodeInvalidState, Message: "team profile produced no runs"}
	}
	sess.CurrentRunID = primaryRunID
	if err := s.session.SaveSession(ctx, sess); err != nil {
		return protocol.SessionStartResult{}, err
	}

	if err := os.MkdirAll(fsutil.GetTeamWorkspaceDir(s.cfg.DataDir, teamID), 0o755); err != nil {
		return protocol.SessionStartResult{}, err
	}
	manifest := buildTeamManifest(teamID, strings.TrimSpace(prof.ID), coordinatorRole, primaryRunID, teamModel, runtimes)
	if err := writeTeamManifestFile(s.cfg, manifest); err != nil {
		return protocol.SessionStartResult{}, err
	}

	return protocol.SessionStartResult{
		SessionID:    strings.TrimSpace(sess.SessionID),
		PrimaryRunID: primaryRunID,
		Mode:         "team",
		Profile:      strings.TrimSpace(prof.ID),
		Model:        teamModel,
		TeamID:       teamID,
		RunIDs:       runIDs,
	}, nil
}

func (s *RPCServer) sessionList(ctx context.Context, p protocol.SessionListParams) (protocol.SessionListResult, error) {
	if _, err := s.resolveThreadID(p.ThreadID); err != nil {
		return protocol.SessionListResult{}, err
	}
	query, ok := s.session.(pkgstore.SessionQuery)
	if !ok {
		return protocol.SessionListResult{}, &protocol.ProtocolError{Code: protocol.CodeInvalidState, Message: "session.list is unavailable"}
	}
	filter := pkgstore.SessionFilter{
		TitleContains: strings.TrimSpace(p.TitleContains),
		Limit:         clampLimit(p.Limit, 50, 500),
		Offset:        max(0, p.Offset),
		SortBy:        "updated_at",
		SortDesc:      true,
	}
	total, err := query.CountSessions(ctx, filter)
	if err != nil {
		return protocol.SessionListResult{}, err
	}
	sessions, err := query.ListSessionsPaginated(ctx, filter)
	if err != nil {
		return protocol.SessionListResult{}, err
	}
	out := make([]protocol.SessionListItem, 0, len(sessions))
	for _, sess := range sessions {
		mode := strings.TrimSpace(sess.Mode)
		teamID := strings.TrimSpace(sess.TeamID)
		profileID := strings.TrimSpace(sess.Profile)
		runID := strings.TrimSpace(sess.CurrentRunID)
		if runID == "" && len(sess.Runs) > 0 {
			runID = strings.TrimSpace(sess.Runs[0])
		}
		if runID != "" {
			if run, rerr := implstore.LoadRun(s.cfg, runID); rerr == nil && run.Runtime != nil {
				if profileID == "" {
					profileID = strings.TrimSpace(run.Runtime.Profile)
				}
				if teamID == "" {
					teamID = strings.TrimSpace(run.Runtime.TeamID)
				}
			}
			if _, inferredTeamID := s.inferRunRoleAndTeam(ctx, runID); teamID == "" && inferredTeamID != "" {
				teamID = strings.TrimSpace(inferredTeamID)
			}
		}
		if mode == "" {
			if teamID != "" {
				mode = "team"
			} else {
				mode = "standalone"
			}
		}
		totalAgents := 0
		runningAgents := 0
		pausedAgents := 0
		for _, listedRunID := range collectSessionRunIDs(sess) {
			listedRunID = strings.TrimSpace(listedRunID)
			if listedRunID == "" {
				continue
			}
			totalAgents++
			run, rerr := implstore.LoadRun(s.cfg, listedRunID)
			if rerr != nil {
				continue
			}
			switch strings.ToLower(strings.TrimSpace(run.Status)) {
			case strings.ToLower(types.RunStatusRunning):
				runningAgents++
			case strings.ToLower(types.RunStatusPaused):
				pausedAgents++
			}
		}
		item := protocol.SessionListItem{
			SessionID:     strings.TrimSpace(sess.SessionID),
			Title:         strings.TrimSpace(sess.Title),
			CurrentRunID:  strings.TrimSpace(sess.CurrentRunID),
			ActiveModel:   strings.TrimSpace(sess.ActiveModel),
			Mode:          mode,
			TeamID:        teamID,
			Profile:       profileID,
			RunningAgents: runningAgents,
			PausedAgents:  pausedAgents,
			TotalAgents:   totalAgents,
		}
		if sess.CreatedAt != nil && !sess.CreatedAt.IsZero() {
			item.CreatedAt = sess.CreatedAt.UTC().Format(time.RFC3339Nano)
		}
		if sess.UpdatedAt != nil && !sess.UpdatedAt.IsZero() {
			item.UpdatedAt = sess.UpdatedAt.UTC().Format(time.RFC3339Nano)
		}
		out = append(out, item)
	}
	return protocol.SessionListResult{Sessions: out, TotalCount: total}, nil
}

func (s *RPCServer) sessionRename(ctx context.Context, p protocol.SessionRenameParams) (protocol.SessionRenameResult, error) {
	threadID, err := s.resolveThreadID(p.ThreadID)
	if err != nil {
		return protocol.SessionRenameResult{}, err
	}
	sessionID := strings.TrimSpace(p.SessionID)
	if sessionID == "" {
		sessionID = threadID
	}
	title := strings.TrimSpace(p.Title)
	if title == "" {
		return protocol.SessionRenameResult{}, &protocol.ProtocolError{Code: protocol.CodeInvalidParams, Message: "title is required"}
	}
	sess, err := s.session.LoadSession(ctx, sessionID)
	if err != nil {
		return protocol.SessionRenameResult{}, err
	}
	sess.Title = title
	if err := s.session.SaveSession(ctx, sess); err != nil {
		return protocol.SessionRenameResult{}, err
	}
	return protocol.SessionRenameResult{SessionID: sessionID, Title: title}, nil
}

func (s *RPCServer) agentList(ctx context.Context, p protocol.AgentListParams) (protocol.AgentListResult, error) {
	threadID, err := s.resolveThreadID(p.ThreadID)
	if err != nil {
		return protocol.AgentListResult{}, err
	}
	sessionID := strings.TrimSpace(p.SessionID)
	if sessionID == "" {
		sessionID = threadID
	}
	sess, err := s.session.LoadSession(ctx, sessionID)
	if err != nil {
		return protocol.AgentListResult{}, err
	}
	out := make([]protocol.AgentListItem, 0, len(sess.Runs))
	for _, runID := range sess.Runs {
		runID = strings.TrimSpace(runID)
		if runID == "" {
			continue
		}
		run, err := implstore.LoadRun(s.cfg, runID)
		if err != nil {
			continue
		}
		item := protocol.AgentListItem{
			RunID:     runID,
			SessionID: strings.TrimSpace(run.SessionID),
			Status:    strings.TrimSpace(run.Status),
			Goal:      strings.TrimSpace(run.Goal),
		}
		if run.Runtime != nil {
			item.Profile = strings.TrimSpace(run.Runtime.Profile)
		}
		if role, teamID := s.inferRunRoleAndTeam(ctx, runID); role != "" || teamID != "" {
			item.Role = role
			item.TeamID = teamID
		}
		if run.StartedAt != nil && !run.StartedAt.IsZero() {
			item.StartedAt = run.StartedAt.UTC().Format(time.RFC3339Nano)
		}
		if run.FinishedAt != nil && !run.FinishedAt.IsZero() {
			item.FinishedAt = run.FinishedAt.UTC().Format(time.RFC3339Nano)
		}
		out = append(out, item)
	}
	sort.SliceStable(out, func(i, j int) bool {
		a := out[i].StartedAt
		b := out[j].StartedAt
		if a == b {
			return out[i].RunID > out[j].RunID
		}
		return a > b
	})
	return protocol.AgentListResult{Agents: out}, nil
}

func (s *RPCServer) inferRunRoleAndTeam(ctx context.Context, runID string) (role string, teamID string) {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return "", ""
	}
	if run, err := implstore.LoadRun(s.cfg, runID); err == nil && run.Runtime != nil {
		role = strings.TrimSpace(run.Runtime.Role)
		teamID = strings.TrimSpace(run.Runtime.TeamID)
		if role != "" && teamID != "" {
			return role, teamID
		}
	}
	if s.taskStore == nil {
		return strings.TrimSpace(role), strings.TrimSpace(teamID)
	}
	tasks, err := s.taskStore.ListTasks(ctx, state.TaskFilter{
		RunID:    runID,
		SortBy:   "created_at",
		SortDesc: true,
		Limit:    50,
	})
	if err != nil || len(tasks) == 0 {
		return strings.TrimSpace(role), strings.TrimSpace(teamID)
	}
	for _, t := range tasks {
		if strings.TrimSpace(teamID) == "" {
			teamID = strings.TrimSpace(t.TeamID)
		}
		if strings.TrimSpace(role) == "" {
			role = strings.TrimSpace(t.RoleSnapshot)
		}
		if strings.TrimSpace(role) == "" {
			role = strings.TrimSpace(t.AssignedRole)
		}
		if strings.TrimSpace(role) == "" && strings.EqualFold(strings.TrimSpace(t.AssignedToType), "role") {
			role = strings.TrimSpace(t.AssignedTo)
		}
		if role != "" && teamID != "" {
			break
		}
	}
	return strings.TrimSpace(role), strings.TrimSpace(teamID)
}

func (s *RPCServer) agentStart(ctx context.Context, p protocol.AgentStartParams) (protocol.AgentStartResult, error) {
	threadID, err := s.resolveThreadID(p.ThreadID)
	if err != nil {
		return protocol.AgentStartResult{}, err
	}
	sessionID := strings.TrimSpace(p.SessionID)
	if sessionID == "" {
		sessionID = threadID
	}
	sess, err := s.session.LoadSession(ctx, sessionID)
	if err != nil {
		return protocol.AgentStartResult{}, err
	}
	maxContext := s.run.MaxBytesForContext
	if maxContext <= 0 {
		maxContext = 8 * 1024
	}
	goal := strings.TrimSpace(p.Goal)
	if goal == "" {
		goal = strings.TrimSpace(sess.CurrentGoal)
	}
	if goal == "" {
		goal = "autonomous agent"
	}
	run := types.NewRun(goal, maxContext, sessionID)
	if run.Runtime == nil {
		run.Runtime = &types.RunRuntimeConfig{}
	}
	run.Runtime.TeamID = strings.TrimSpace(sess.TeamID)
	run.Runtime.Role = ""
	if profileRef := strings.TrimSpace(p.Profile); profileRef != "" {
		run.Runtime.Profile = profileRef
		sess.Profile = profileRef
	}
	if err := implstore.SaveRun(s.cfg, run); err != nil {
		return protocol.AgentStartResult{}, err
	}
	runID := strings.TrimSpace(run.RunID)
	exists := false
	for _, id := range sess.Runs {
		if strings.TrimSpace(id) == runID {
			exists = true
			break
		}
	}
	if !exists {
		sess.Runs = append(sess.Runs, runID)
	}
	sess.CurrentRunID = runID
	if strings.TrimSpace(sess.Mode) == "" {
		if strings.TrimSpace(sess.TeamID) != "" {
			sess.Mode = "team"
		} else {
			sess.Mode = "standalone"
		}
	}

	model := strings.TrimSpace(p.Model)
	if model == "" {
		model = strings.TrimSpace(sess.ActiveModel)
	}
	if model != "" {
		if run.Runtime == nil {
			run.Runtime = &types.RunRuntimeConfig{}
		}
		run.Runtime.Model = model
		_ = implstore.SaveRun(s.cfg, run)
		sess.ActiveModel = model
	}
	ensureSessionReasoningForModel(&sess, sess.ActiveModel, "", "")
	if err := s.session.SaveSession(ctx, sess); err != nil {
		return protocol.AgentStartResult{}, err
	}
	return protocol.AgentStartResult{
		RunID:     runID,
		SessionID: sessionID,
		Profile:   strings.TrimSpace(p.Profile),
		Model:     model,
	}, nil
}

func (s *RPCServer) agentPauseHandler(ctx context.Context, p protocol.AgentPauseParams) (protocol.AgentPauseResult, error) {
	threadID, err := s.resolveThreadID(p.ThreadID)
	if err != nil {
		return protocol.AgentPauseResult{}, err
	}
	runID := strings.TrimSpace(p.RunID)
	if runID == "" {
		return protocol.AgentPauseResult{}, &protocol.ProtocolError{Code: protocol.CodeInvalidParams, Message: "runId is required"}
	}
	if s.agentPause != nil {
		if err := s.agentPause(ctx, threadID, runID); err != nil {
			return protocol.AgentPauseResult{}, err
		}
		return protocol.AgentPauseResult{RunID: runID, Status: types.RunStatusPaused}, nil
	}
	if err := s.setRunPausedState(threadID, runID, true); err != nil {
		return protocol.AgentPauseResult{}, err
	}
	return protocol.AgentPauseResult{RunID: runID, Status: types.RunStatusPaused}, nil
}

func (s *RPCServer) agentResumeHandler(ctx context.Context, p protocol.AgentResumeParams) (protocol.AgentResumeResult, error) {
	threadID, err := s.resolveThreadID(p.ThreadID)
	if err != nil {
		return protocol.AgentResumeResult{}, err
	}
	runID := strings.TrimSpace(p.RunID)
	if runID == "" {
		return protocol.AgentResumeResult{}, &protocol.ProtocolError{Code: protocol.CodeInvalidParams, Message: "runId is required"}
	}
	if s.agentResume != nil {
		if err := s.agentResume(ctx, threadID, runID); err != nil {
			return protocol.AgentResumeResult{}, err
		}
		return protocol.AgentResumeResult{RunID: runID, Status: types.RunStatusRunning}, nil
	}
	if err := s.setRunPausedState(threadID, runID, false); err != nil {
		return protocol.AgentResumeResult{}, err
	}
	return protocol.AgentResumeResult{RunID: runID, Status: types.RunStatusRunning}, nil
}

func (s *RPCServer) sessionPauseHandler(ctx context.Context, p protocol.SessionPauseParams) (protocol.SessionPauseResult, error) {
	threadID, err := s.resolveThreadID(p.ThreadID)
	if err != nil {
		return protocol.SessionPauseResult{}, err
	}
	sessionID := strings.TrimSpace(p.SessionID)
	if sessionID == "" {
		sessionID = threadID
	}
	if sessionID == "" {
		return protocol.SessionPauseResult{}, &protocol.ProtocolError{Code: protocol.CodeInvalidParams, Message: "sessionId is required"}
	}
	if s.sessionPause != nil {
		affected, err := s.sessionPause(ctx, threadID, sessionID)
		if err != nil {
			return protocol.SessionPauseResult{}, err
		}
		return protocol.SessionPauseResult{SessionID: sessionID, AffectedRunIDs: affected}, nil
	}
	affected, err := s.setSessionPausedState(ctx, threadID, sessionID, true)
	if err != nil {
		return protocol.SessionPauseResult{}, err
	}
	return protocol.SessionPauseResult{SessionID: sessionID, AffectedRunIDs: affected}, nil
}

func (s *RPCServer) sessionResumeHandler(ctx context.Context, p protocol.SessionResumeParams) (protocol.SessionResumeResult, error) {
	threadID, err := s.resolveThreadID(p.ThreadID)
	if err != nil {
		return protocol.SessionResumeResult{}, err
	}
	sessionID := strings.TrimSpace(p.SessionID)
	if sessionID == "" {
		sessionID = threadID
	}
	if sessionID == "" {
		return protocol.SessionResumeResult{}, &protocol.ProtocolError{Code: protocol.CodeInvalidParams, Message: "sessionId is required"}
	}
	if s.sessionResume != nil {
		affected, err := s.sessionResume(ctx, threadID, sessionID)
		if err != nil {
			return protocol.SessionResumeResult{}, err
		}
		return protocol.SessionResumeResult{SessionID: sessionID, AffectedRunIDs: affected}, nil
	}
	affected, err := s.setSessionPausedState(ctx, threadID, sessionID, false)
	if err != nil {
		return protocol.SessionResumeResult{}, err
	}
	return protocol.SessionResumeResult{SessionID: sessionID, AffectedRunIDs: affected}, nil
}

func (s *RPCServer) setSessionPausedState(ctx context.Context, threadID, sessionID string, paused bool) ([]string, error) {
	if strings.TrimSpace(sessionID) == "" {
		return nil, &protocol.ProtocolError{Code: protocol.CodeInvalidParams, Message: "sessionId is required"}
	}
	sess, err := s.session.LoadSession(ctx, strings.TrimSpace(sessionID))
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(sess.SessionID) != strings.TrimSpace(threadID) {
		return nil, &protocol.ProtocolError{Code: protocol.CodeThreadNotFound, Message: "thread not found"}
	}
	runIDs := collectSessionRunIDs(sess)
	affected := make([]string, 0, len(runIDs))
	for _, runID := range runIDs {
		if err := s.setRunPausedState(threadID, runID, paused); err != nil {
			return affected, err
		}
		affected = append(affected, runID)
	}
	return affected, nil
}

func (s *RPCServer) setRunPausedState(threadID, runID string, paused bool) error {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return &protocol.ProtocolError{Code: protocol.CodeInvalidParams, Message: "runId is required"}
	}
	run, err := implstore.LoadRun(s.cfg, runID)
	if err != nil {
		return &protocol.ProtocolError{Code: protocol.CodeItemNotFound, Message: "run not found"}
	}
	if strings.TrimSpace(run.SessionID) != strings.TrimSpace(threadID) {
		return &protocol.ProtocolError{Code: protocol.CodeThreadNotFound, Message: "thread not found"}
	}
	status := strings.ToLower(strings.TrimSpace(run.Status))
	switch status {
	case strings.ToLower(types.RunStatusRunning), strings.ToLower(types.RunStatusPaused):
		// supported
	default:
		return &protocol.ProtocolError{Code: protocol.CodeInvalidState, Message: "run is not pauseable"}
	}
	if paused {
		run.Status = types.RunStatusPaused
	} else {
		run.Status = types.RunStatusRunning
	}
	run.FinishedAt = nil
	run.Error = nil
	return implstore.SaveRun(s.cfg, run)
}

func normalizeAssignedToType(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "team":
		return "team"
	case "role":
		return "role"
	case "agent":
		return "agent"
	default:
		return ""
	}
}

func (s *RPCServer) taskCreate(ctx context.Context, p protocol.TaskCreateParams) (protocol.TaskCreateResult, error) {
	scope, err := s.resolveTeamOrRunScope(ctx, p.ThreadID, p.TeamID, p.RunID)
	if err != nil {
		return protocol.TaskCreateResult{}, err
	}
	goal := strings.TrimSpace(p.Goal)
	if goal == "" {
		return protocol.TaskCreateResult{}, &protocol.ProtocolError{Code: protocol.CodeInvalidParams, Message: "goal is required"}
	}
	now := time.Now().UTC()
	taskID := "task-" + uuid.NewString()
	assignedToType := normalizeAssignedToType(p.AssignedToType)
	assignedTo := strings.TrimSpace(p.AssignedTo)
	assignedRole := strings.TrimSpace(p.AssignedRole)
	if assignedToType == "" {
		if scope.teamID != "" {
			if assignedRole != "" {
				assignedToType = "role"
				assignedTo = assignedRole
			} else {
				assignedToType = "team"
				assignedTo = scope.teamID
			}
		} else {
			assignedToType = "agent"
			assignedTo = scope.runID
		}
	}
	if assignedTo == "" {
		switch assignedToType {
		case "team":
			assignedTo = scope.teamID
		case "role":
			assignedTo = assignedRole
		case "agent":
			assignedTo = scope.runID
		}
	}
	taskRunID := strings.TrimSpace(scope.runID)
	if taskRunID == "" && strings.EqualFold(assignedToType, "agent") {
		taskRunID = strings.TrimSpace(assignedTo)
	}
	if taskRunID == "" {
		return protocol.TaskCreateResult{}, &protocol.ProtocolError{Code: protocol.CodeInvalidParams, Message: "run scope is required"}
	}
	sessionID := strings.TrimSpace(scope.sessionID)
	if sessionID == "" {
		return protocol.TaskCreateResult{}, &protocol.ProtocolError{Code: protocol.CodeInvalidParams, Message: "threadId is required"}
	}
	task := types.Task{
		TaskID:         taskID,
		SessionID:      sessionID,
		RunID:          taskRunID,
		TeamID:         strings.TrimSpace(scope.teamID),
		AssignedRole:   assignedRole,
		AssignedToType: assignedToType,
		AssignedTo:     assignedTo,
		TaskKind:       strings.TrimSpace(p.TaskKind),
		Goal:           goal,
		Priority:       p.Priority,
		Status:         types.TaskStatusPending,
		CreatedAt:      &now,
		Inputs:         map[string]any{},
		Metadata:       map[string]any{"source": "rpc.task.create"},
		CreatedBy:      "monitor",
	}
	if task.Priority == 0 {
		task.Priority = 5
	}
	if err := s.taskStore.CreateTask(ctx, task); err != nil {
		return protocol.TaskCreateResult{}, err
	}
	if s.wake != nil {
		s.wake()
	}
	got, err := s.taskStore.GetTask(ctx, taskID)
	if err != nil {
		return protocol.TaskCreateResult{}, err
	}
	return protocol.TaskCreateResult{Task: protocolTaskFromTypesTask(got)}, nil
}

func (s *RPCServer) taskClaim(ctx context.Context, p protocol.TaskClaimParams) (protocol.TaskClaimResult, error) {
	scope, err := s.resolveArtifactScope(ctx, p.ThreadID, "")
	if err != nil {
		return protocol.TaskClaimResult{}, err
	}
	taskID := strings.TrimSpace(p.TaskID)
	if taskID == "" {
		return protocol.TaskClaimResult{}, &protocol.ProtocolError{Code: protocol.CodeInvalidParams, Message: "taskId is required"}
	}
	if err := s.taskStore.ClaimTask(ctx, taskID, 2*time.Minute); err != nil {
		if errors.Is(err, state.ErrTaskClaimed) {
			return protocol.TaskClaimResult{}, &protocol.ProtocolError{Code: protocol.CodeInvalidState, Message: "task already claimed"}
		}
		if errors.Is(err, state.ErrTaskNotFound) {
			return protocol.TaskClaimResult{}, &protocol.ProtocolError{Code: protocol.CodeTurnNotFound, Message: "task not found"}
		}
		return protocol.TaskClaimResult{}, err
	}
	task, err := s.taskStore.GetTask(ctx, taskID)
	if err != nil {
		return protocol.TaskClaimResult{}, err
	}
	if scope.teamID != "" {
		if strings.TrimSpace(task.TeamID) != scope.teamID {
			return protocol.TaskClaimResult{}, &protocol.ProtocolError{Code: protocol.CodeTurnNotFound, Message: "task not found"}
		}
	} else if strings.TrimSpace(task.RunID) != scope.runID {
		return protocol.TaskClaimResult{}, &protocol.ProtocolError{Code: protocol.CodeTurnNotFound, Message: "task not found"}
	}
	claimer := strings.TrimSpace(p.AgentID)
	if claimer == "" {
		if strings.TrimSpace(scope.runID) != "" {
			claimer = strings.TrimSpace(scope.runID)
		} else {
			claimer = strings.TrimSpace(task.RunID)
		}
	}
	task.ClaimedByAgentID = claimer
	if strings.TrimSpace(task.RoleSnapshot) == "" {
		task.RoleSnapshot = strings.TrimSpace(task.AssignedRole)
	}
	_ = s.taskStore.UpdateTask(ctx, task)
	task, _ = s.taskStore.GetTask(ctx, taskID)
	return protocol.TaskClaimResult{Task: protocolTaskFromTypesTask(task)}, nil
}

func (s *RPCServer) taskComplete(ctx context.Context, p protocol.TaskCompleteParams) (protocol.TaskCompleteResult, error) {
	scope, err := s.resolveArtifactScope(ctx, p.ThreadID, p.TeamID)
	if err != nil {
		return protocol.TaskCompleteResult{}, err
	}
	taskID := strings.TrimSpace(p.TaskID)
	if taskID == "" {
		return protocol.TaskCompleteResult{}, &protocol.ProtocolError{Code: protocol.CodeInvalidParams, Message: "taskId is required"}
	}
	task, err := s.taskStore.GetTask(ctx, taskID)
	if err != nil {
		if errors.Is(err, state.ErrTaskNotFound) {
			return protocol.TaskCompleteResult{}, &protocol.ProtocolError{Code: protocol.CodeTurnNotFound, Message: "task not found"}
		}
		return protocol.TaskCompleteResult{}, err
	}
	if scope.teamID != "" {
		if strings.TrimSpace(task.TeamID) != scope.teamID {
			return protocol.TaskCompleteResult{}, &protocol.ProtocolError{Code: protocol.CodeTurnNotFound, Message: "task not found"}
		}
	} else if strings.TrimSpace(task.RunID) != scope.runID {
		return protocol.TaskCompleteResult{}, &protocol.ProtocolError{Code: protocol.CodeTurnNotFound, Message: "task not found"}
	}
	status := strings.ToLower(strings.TrimSpace(p.Status))
	if status == "" {
		if strings.TrimSpace(p.Error) != "" {
			status = string(types.TaskStatusFailed)
		} else {
			status = string(types.TaskStatusSucceeded)
		}
	}
	done := time.Now().UTC()
	res := types.TaskResult{
		TaskID:      taskID,
		Status:      types.TaskStatus(status),
		Summary:     strings.TrimSpace(p.Summary),
		Artifacts:   append([]string(nil), p.Artifacts...),
		Error:       strings.TrimSpace(p.Error),
		CompletedAt: &done,
	}
	if err := s.taskStore.CompleteTask(ctx, taskID, res); err != nil {
		return protocol.TaskCompleteResult{}, err
	}
	updated, err := s.taskStore.GetTask(ctx, taskID)
	if err != nil {
		return protocol.TaskCompleteResult{}, err
	}
	return protocol.TaskCompleteResult{Task: protocolTaskFromTypesTask(updated)}, nil
}

func (s *RPCServer) resolveTeamOrRunScope(ctx context.Context, threadID protocol.ThreadID, teamIDOverride string, runIDOverride string) (artifactScope, error) {
	if strings.TrimSpace(runIDOverride) != "" {
		resolvedThread, err := s.resolveThreadID(threadID)
		if err != nil {
			return artifactScope{}, err
		}
		scope := artifactScope{
			sessionID: resolvedThread,
			teamID:    strings.TrimSpace(teamIDOverride),
			runID:     strings.TrimSpace(runIDOverride),
		}
		if scope.teamID == "" && s.taskStore != nil {
			tasks, err := s.taskStore.ListTasks(ctx, state.TaskFilter{
				RunID:    scope.runID,
				SortBy:   "created_at",
				SortDesc: true,
				Limit:    1,
			})
			if err == nil && len(tasks) > 0 {
				scope.teamID = strings.TrimSpace(tasks[0].TeamID)
			}
		}
		return scope, nil
	}
	scope, err := s.resolveArtifactScope(ctx, threadID, teamIDOverride)
	if err != nil {
		return artifactScope{}, err
	}
	return scope, nil
}

func pricingKnownForRun(cfg config.Config, runID string) bool {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return false
	}
	run, err := implstore.LoadRun(cfg, runID)
	if err != nil || run.Runtime == nil {
		return false
	}
	if run.Runtime.PriceInPerMTokensUSD != 0 || run.Runtime.PriceOutPerMTokensUSD != 0 {
		return true
	}
	modelID := strings.TrimSpace(run.Runtime.Model)
	if modelID == "" {
		return false
	}
	_, _, ok := cost.DefaultPricing().Lookup(modelID)
	return ok
}

func (s *RPCServer) sessionGetTotals(ctx context.Context, p protocol.SessionGetTotalsParams) (protocol.SessionGetTotalsResult, error) {
	scope, err := s.resolveTeamOrRunScope(ctx, p.ThreadID, p.TeamID, p.RunID)
	if err != nil {
		return protocol.SessionGetTotalsResult{}, err
	}
	if s.taskStore == nil {
		return protocol.SessionGetTotalsResult{}, &protocol.ProtocolError{Code: protocol.CodeInvalidState, Message: "task store not configured"}
	}

	out := protocol.SessionGetTotalsResult{
		PricingKnown: true,
	}
	if strings.TrimSpace(scope.teamID) == "" {
		if s.session != nil && strings.TrimSpace(scope.sessionID) != "" {
			if sess, err := s.session.LoadSession(ctx, strings.TrimSpace(scope.sessionID)); err == nil {
				out.TotalTokensIn = sess.InputTokens
				out.TotalTokensOut = sess.OutputTokens
				out.TotalTokens = sess.TotalTokens
				out.TotalCostUSD = sess.CostUSD
				out.PricingKnown = sess.TotalTokens == 0 || sess.CostUSD > 0 || pricingKnownForRun(s.cfg, strings.TrimSpace(scope.runID))
			}
		}
		stats, err := s.taskStore.GetRunStats(ctx, strings.TrimSpace(scope.runID))
		if err == nil {
			out.TasksDone = stats.Succeeded + stats.Failed
		}
		return out, nil
	}

	runIDSet := map[string]struct{}{}
	tasks, err := s.taskStore.ListTasks(ctx, state.TaskFilter{
		TeamID:   strings.TrimSpace(scope.teamID),
		Limit:    500,
		SortBy:   "created_at",
		SortDesc: true,
	})
	if err != nil {
		return protocol.SessionGetTotalsResult{}, err
	}
	for _, t := range tasks {
		if r := strings.TrimSpace(t.RunID); r != "" {
			runIDSet[r] = struct{}{}
		}
		if t.Status == types.TaskStatusSucceeded || t.Status == types.TaskStatusFailed || t.Status == types.TaskStatusCanceled {
			out.TasksDone++
		}
	}
	for runID := range runIDSet {
		rs, err := s.taskStore.GetRunStats(ctx, runID)
		if err != nil {
			continue
		}
		out.TotalTokens += rs.TotalTokens
		out.TotalCostUSD += rs.TotalCost
		if rs.TotalTokens > 0 && rs.TotalCost <= 0 && !pricingKnownForRun(s.cfg, runID) {
			out.PricingKnown = false
		}
	}
	if out.TotalTokens == 0 {
		out.PricingKnown = true
	}
	return out, nil
}

func (s *RPCServer) activityList(ctx context.Context, p protocol.ActivityListParams) (protocol.ActivityListResult, error) {
	scope, err := s.resolveTeamOrRunScope(ctx, p.ThreadID, p.TeamID, p.RunID)
	if err != nil {
		return protocol.ActivityListResult{}, err
	}
	limit := clampLimit(p.Limit, 200, 2000)
	offset := p.Offset
	if offset < 0 {
		offset = 0
	}

	if strings.TrimSpace(scope.teamID) == "" {
		acts, err := implstore.ListActivities(ctx, s.cfg, strings.TrimSpace(scope.runID), limit, offset)
		if err != nil {
			return protocol.ActivityListResult{}, err
		}
		total, _ := implstore.CountActivities(ctx, s.cfg, strings.TrimSpace(scope.runID))
		next := 0
		if offset+len(acts) < total {
			next = offset + len(acts)
		}
		return protocol.ActivityListResult{Activities: acts, TotalCount: total, NextOffset: next}, nil
	}

	runRole := map[string]string{}
	runSet := map[string]struct{}{}
	tasks, err := s.taskStore.ListTasks(ctx, state.TaskFilter{
		TeamID:   strings.TrimSpace(scope.teamID),
		Limit:    1000,
		SortBy:   "created_at",
		SortDesc: true,
	})
	if err != nil {
		return protocol.ActivityListResult{}, err
	}
	for _, t := range tasks {
		runID := strings.TrimSpace(t.RunID)
		if runID == "" {
			continue
		}
		runSet[runID] = struct{}{}
		if _, ok := runRole[runID]; !ok {
			runRole[runID] = strings.TrimSpace(t.AssignedRole)
		}
	}
	merged := make([]types.Activity, 0, 512)
	for runID := range runSet {
		acts, err := implstore.ListActivities(ctx, s.cfg, runID, 300, 0)
		if err != nil {
			continue
		}
		role := strings.TrimSpace(runRole[runID])
		if roleFilter := strings.TrimSpace(p.Role); roleFilter != "" && !strings.EqualFold(roleFilter, role) {
			continue
		}
		for i := range acts {
			if role != "" {
				acts[i].Title = "[" + role + "] " + strings.TrimSpace(acts[i].Title)
			}
			acts[i].ID = runID + ":" + acts[i].ID
			merged = append(merged, acts[i])
		}
	}
	sort.SliceStable(merged, func(i, j int) bool {
		if p.SortDesc {
			return merged[i].StartedAt.After(merged[j].StartedAt)
		}
		return merged[i].StartedAt.Before(merged[j].StartedAt)
	})
	total := len(merged)
	if offset > total {
		offset = total
	}
	end := offset + limit
	if end > total {
		end = total
	}
	out := []types.Activity{}
	if offset < end {
		out = append(out, merged[offset:end]...)
	}
	next := 0
	if end < total {
		next = end
	}
	return protocol.ActivityListResult{Activities: out, TotalCount: total, NextOffset: next}, nil
}

func (s *RPCServer) teamGetStatus(ctx context.Context, p protocol.TeamGetStatusParams) (protocol.TeamGetStatusResult, error) {
	scope, err := s.resolveTeamOrRunScope(ctx, p.ThreadID, p.TeamID, "")
	if err != nil {
		return protocol.TeamGetStatusResult{}, err
	}
	if strings.TrimSpace(scope.teamID) == "" {
		return protocol.TeamGetStatusResult{}, &protocol.ProtocolError{Code: protocol.CodeInvalidParams, Message: "team scope is required"}
	}
	teamID := strings.TrimSpace(scope.teamID)
	pending, _ := s.taskStore.CountTasks(ctx, state.TaskFilter{TeamID: teamID, Status: []types.TaskStatus{types.TaskStatusPending}})
	active, _ := s.taskStore.CountTasks(ctx, state.TaskFilter{TeamID: teamID, Status: []types.TaskStatus{types.TaskStatusActive}})
	done, _ := s.taskStore.CountTasks(ctx, state.TaskFilter{TeamID: teamID, Status: []types.TaskStatus{types.TaskStatusSucceeded, types.TaskStatusFailed, types.TaskStatusCanceled}})

	roleInfo := map[string]string{}
	roleByRunID := map[string]string{}
	runIDSet := map[string]struct{}{}
	pendingTasks, _ := s.taskStore.ListTasks(ctx, state.TaskFilter{TeamID: teamID, Status: []types.TaskStatus{types.TaskStatusPending}, SortBy: "created_at", SortDesc: false, Limit: 200})
	activeTasks, _ := s.taskStore.ListTasks(ctx, state.TaskFilter{TeamID: teamID, Status: []types.TaskStatus{types.TaskStatusActive}, SortBy: "updated_at", SortDesc: true, Limit: 200})
	completedTasks, _ := s.taskStore.ListTasks(ctx, state.TaskFilter{TeamID: teamID, Status: []types.TaskStatus{types.TaskStatusSucceeded, types.TaskStatusFailed, types.TaskStatusCanceled}, SortBy: "finished_at", SortDesc: true, Limit: 500})

	for _, task := range pendingTasks {
		role := strings.TrimSpace(task.AssignedRole)
		if role == "" {
			role = "(coordinator)"
		}
		if runID := strings.TrimSpace(task.RunID); runID != "" {
			if _, ok := roleByRunID[runID]; !ok {
				roleByRunID[runID] = role
			}
			runIDSet[runID] = struct{}{}
		}
		if _, exists := roleInfo[role]; !exists {
			roleInfo[role] = "pending: " + truncateText(strings.TrimSpace(task.Goal), 52)
		}
	}
	for _, task := range activeTasks {
		role := strings.TrimSpace(task.AssignedRole)
		if role == "" {
			role = "(coordinator)"
		}
		if runID := strings.TrimSpace(task.RunID); runID != "" {
			if _, ok := roleByRunID[runID]; !ok {
				roleByRunID[runID] = role
			}
			runIDSet[runID] = struct{}{}
		}
		roleInfo[role] = "active: " + truncateText(strings.TrimSpace(task.Goal), 52)
	}
	for _, task := range completedTasks {
		role := strings.TrimSpace(task.AssignedRole)
		if role == "" {
			role = "(coordinator)"
		}
		if runID := strings.TrimSpace(task.RunID); runID != "" {
			if _, ok := roleByRunID[runID]; !ok {
				roleByRunID[runID] = role
			}
			runIDSet[runID] = struct{}{}
		}
	}
	roleKeys := make([]string, 0, len(roleInfo))
	for role := range roleInfo {
		roleKeys = append(roleKeys, role)
	}
	sort.Strings(roleKeys)
	roles := make([]protocol.TeamRoleStatus, 0, len(roleKeys))
	for _, role := range roleKeys {
		roles = append(roles, protocol.TeamRoleStatus{Role: role, Info: roleInfo[role]})
	}
	runIDs := make([]string, 0, len(runIDSet))
	for runID := range runIDSet {
		runIDs = append(runIDs, runID)
	}
	sort.Strings(runIDs)
	totalTokens := 0
	totalCostUSD := 0.0
	pricingKnown := true
	for _, runID := range runIDs {
		stats, err := s.taskStore.GetRunStats(ctx, runID)
		if err != nil {
			continue
		}
		totalTokens += stats.TotalTokens
		totalCostUSD += stats.TotalCost
		if stats.TotalTokens > 0 && stats.TotalCost <= 0 && !pricingKnownForRun(s.cfg, runID) {
			pricingKnown = false
		}
	}
	if totalTokens == 0 {
		pricingKnown = true
	}
	return protocol.TeamGetStatusResult{
		Pending:      pending,
		Active:       active,
		Done:         done,
		Roles:        roles,
		RunIDs:       runIDs,
		RoleByRunID:  roleByRunID,
		TotalTokens:  totalTokens,
		TotalCostUSD: totalCostUSD,
		PricingKnown: pricingKnown,
	}, nil
}

func (s *RPCServer) teamGetManifest(ctx context.Context, p protocol.TeamGetManifestParams) (protocol.TeamGetManifestResult, error) {
	scope, err := s.resolveTeamOrRunScope(ctx, p.ThreadID, p.TeamID, "")
	if err != nil {
		return protocol.TeamGetManifestResult{}, err
	}
	teamID := strings.TrimSpace(scope.teamID)
	if teamID == "" {
		return protocol.TeamGetManifestResult{}, &protocol.ProtocolError{Code: protocol.CodeInvalidParams, Message: "team scope is required"}
	}
	path := filepath.Join(fsutil.GetTeamDir(s.cfg.DataDir, teamID), "team.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		return protocol.TeamGetManifestResult{}, err
	}
	var mf struct {
		TeamID          string                            `json:"teamId"`
		ProfileID       string                            `json:"profileId"`
		TeamModel       string                            `json:"teamModel,omitempty"`
		ModelChange     *protocol.TeamManifestModelChange `json:"modelChange,omitempty"`
		CoordinatorRole string                            `json:"coordinatorRole"`
		CoordinatorRun  string                            `json:"coordinatorRunId"`
		Roles           []protocol.TeamManifestRole       `json:"roles"`
		CreatedAt       string                            `json:"createdAt"`
	}
	if err := json.Unmarshal(raw, &mf); err != nil {
		return protocol.TeamGetManifestResult{}, err
	}
	return protocol.TeamGetManifestResult{
		TeamID:          strings.TrimSpace(mf.TeamID),
		ProfileID:       strings.TrimSpace(mf.ProfileID),
		TeamModel:       strings.TrimSpace(mf.TeamModel),
		ModelChange:     mf.ModelChange,
		CoordinatorRole: strings.TrimSpace(mf.CoordinatorRole),
		CoordinatorRun:  strings.TrimSpace(mf.CoordinatorRun),
		Roles:           mf.Roles,
		CreatedAt:       strings.TrimSpace(mf.CreatedAt),
	}, nil
}

func readPlanFilesForRun(dataDir, runID string) (checklist string, checklistErr string, details string, detailsErr string) {
	runDir := fsutil.GetAgentDir(dataDir, runID)
	planDir := filepath.Join(runDir, "plan")
	load := func(name string) (string, string) {
		b, err := os.ReadFile(filepath.Join(planDir, name))
		if err != nil {
			if os.IsNotExist(err) {
				return "", ""
			}
			return "", err.Error()
		}
		return string(b), ""
	}
	details, detailsErr = load("HEAD.md")
	checklist, checklistErr = load("CHECKLIST.md")
	return checklist, checklistErr, details, detailsErr
}

func (s *RPCServer) planGet(ctx context.Context, p protocol.PlanGetParams) (protocol.PlanGetResult, error) {
	scope, err := s.resolveTeamOrRunScope(ctx, p.ThreadID, p.TeamID, p.RunID)
	if err != nil {
		return protocol.PlanGetResult{}, err
	}
	if strings.TrimSpace(scope.teamID) == "" {
		checklist, checklistErr, details, detailsErr := readPlanFilesForRun(s.cfg.DataDir, strings.TrimSpace(scope.runID))
		return protocol.PlanGetResult{
			Checklist: checklist, ChecklistErr: checklistErr, Details: details, DetailsErr: detailsErr, SourceRuns: []string{strings.TrimSpace(scope.runID)},
		}, nil
	}
	if strings.TrimSpace(scope.runID) != "" {
		checklist, checklistErr, details, detailsErr := readPlanFilesForRun(s.cfg.DataDir, strings.TrimSpace(scope.runID))
		return protocol.PlanGetResult{
			Checklist: checklist, ChecklistErr: checklistErr, Details: details, DetailsErr: detailsErr, SourceRuns: []string{strings.TrimSpace(scope.runID)},
		}, nil
	}
	aggregate := p.AggregateTeam
	if !aggregate {
		return protocol.PlanGetResult{}, &protocol.ProtocolError{Code: protocol.CodeInvalidParams, Message: "aggregateTeam must be true when runId is omitted in team mode"}
	}
	roleByRun := map[string]string{}
	runSet := map[string]struct{}{}
	tasks, _ := s.taskStore.ListTasks(ctx, state.TaskFilter{TeamID: strings.TrimSpace(scope.teamID), Limit: 1000, SortBy: "created_at", SortDesc: true})
	for _, t := range tasks {
		runID := strings.TrimSpace(t.RunID)
		if runID == "" {
			continue
		}
		runSet[runID] = struct{}{}
		if _, ok := roleByRun[runID]; !ok {
			roleByRun[runID] = strings.TrimSpace(t.AssignedRole)
		}
	}
	runIDs := make([]string, 0, len(runSet))
	for r := range runSet {
		runIDs = append(runIDs, r)
	}
	sort.Strings(runIDs)
	if len(runIDs) == 0 {
		return protocol.PlanGetResult{
			Checklist: "No team plan files found yet.",
			Details:   "Waiting for team runs to publish plan files.",
		}, nil
	}
	checkParts := make([]string, 0, len(runIDs))
	detailParts := make([]string, 0, len(runIDs))
	errParts := []string{}
	for _, runID := range runIDs {
		role := strings.TrimSpace(roleByRun[runID])
		if role == "" {
			role = runID
		}
		check, checkErr, det, detErr := readPlanFilesForRun(s.cfg.DataDir, runID)
		if checkErr != "" {
			errParts = append(errParts, "["+role+"] checklist: "+checkErr)
		}
		if detErr != "" {
			errParts = append(errParts, "["+role+"] details: "+detErr)
		}
		if strings.TrimSpace(check) != "" {
			checkParts = append(checkParts, "## "+role+"\n\n"+strings.TrimSpace(check))
		}
		if strings.TrimSpace(det) != "" {
			detailParts = append(detailParts, "## "+role+"\n\n"+strings.TrimSpace(det))
		}
	}
	checklist := "No team checklist files found yet."
	if len(checkParts) > 0 {
		checklist = strings.Join(checkParts, "\n\n---\n\n")
	}
	details := "No team plan detail files found yet."
	if len(detailParts) > 0 {
		details = strings.Join(detailParts, "\n\n---\n\n")
	}
	joinedErr := strings.Join(errParts, " | ")
	return protocol.PlanGetResult{
		Checklist: checklist, ChecklistErr: joinedErr, Details: details, DetailsErr: joinedErr, SourceRuns: runIDs,
	}, nil
}

func (s *RPCServer) modelList(ctx context.Context, p protocol.ModelListParams) (protocol.ModelListResult, error) {
	if _, err := s.resolveThreadID(p.ThreadID); err != nil {
		return protocol.ModelListResult{}, err
	}
	_ = ctx
	providerFilter := strings.ToLower(strings.TrimSpace(p.Provider))
	query := strings.ToLower(strings.TrimSpace(p.Query))
	infos := cost.SupportedModelInfos()
	models := make([]protocol.ModelEntry, 0, len(infos))
	counts := map[string]int{}
	for _, info := range infos {
		provider := strings.TrimSpace(info.Provider)
		id := strings.TrimSpace(info.ID)
		if providerFilter != "" && strings.ToLower(provider) != providerFilter {
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(id), query) && !strings.Contains(strings.ToLower(provider), query) {
			continue
		}
		counts[provider]++
		models = append(models, protocol.ModelEntry{
			ID:          id,
			Provider:    provider,
			InputPerM:   info.InputPerM,
			OutputPerM:  info.OutputPerM,
			IsReasoning: info.IsReasoning,
		})
	}
	providers := make([]protocol.ModelProvider, 0, len(counts))
	for name, count := range counts {
		providers = append(providers, protocol.ModelProvider{Name: name, Count: count})
	}
	sort.Slice(providers, func(i, j int) bool { return providers[i].Name < providers[j].Name })
	sort.Slice(models, func(i, j int) bool {
		if models[i].Provider != models[j].Provider {
			return models[i].Provider < models[j].Provider
		}
		return models[i].ID < models[j].ID
	})
	return protocol.ModelListResult{Providers: providers, Models: models}, nil
}

func (s *RPCServer) threadGet(ctx context.Context, p protocol.ThreadGetParams) (protocol.ThreadGetResult, error) {
	threadID, err := s.resolveThreadID(p.ThreadID)
	if err != nil {
		return protocol.ThreadGetResult{}, err
	}
	sess, err := s.loadSessionForID(ctx, threadID)
	if err != nil {
		return protocol.ThreadGetResult{}, err
	}
	return protocol.ThreadGetResult{Thread: threadFromSession(defaultRunIDForSession(sess), sess)}, nil
}

func (s *RPCServer) threadCreate(ctx context.Context, p protocol.ThreadCreateParams) (protocol.ThreadCreateResult, error) {
	threadID := strings.TrimSpace(string(p.ThreadID))
	if threadID == "" {
		if s.allowAnyThread {
			return protocol.ThreadCreateResult{}, &protocol.ProtocolError{Code: protocol.CodeInvalidParams, Message: "threadId is required"}
		}
		threadID = strings.TrimSpace(s.run.SessionID)
	}
	if _, err := s.resolveThreadID(protocol.ThreadID(threadID)); err != nil {
		return protocol.ThreadCreateResult{}, err
	}
	sess, err := s.loadSessionForID(ctx, threadID)
	if err != nil {
		return protocol.ThreadCreateResult{}, err
	}
	changed := false
	if title := strings.TrimSpace(p.Title); title != "" && strings.TrimSpace(sess.Title) != title {
		sess.Title = title
		changed = true
	}
	if model := strings.TrimSpace(p.ActiveModel); model != "" && strings.TrimSpace(sess.ActiveModel) != model {
		sess.ActiveModel = model
		changed = true
	}
	if changed {
		ensureSessionReasoningForModel(&sess, sess.ActiveModel, "", "")
	}
	if changed {
		if err := s.session.SaveSession(ctx, sess); err != nil {
			return protocol.ThreadCreateResult{}, err
		}
	}
	return protocol.ThreadCreateResult{Thread: threadFromSession(defaultRunIDForSession(sess), sess)}, nil
}

func (s *RPCServer) turnCreate(ctx context.Context, p protocol.TurnCreateParams) (protocol.TurnCreateResult, error) {
	if _, err := s.resolveThreadID(p.ThreadID); err != nil {
		return protocol.TurnCreateResult{}, err
	}
	if p.Input == nil || strings.TrimSpace(p.Input.Text) == "" {
		return protocol.TurnCreateResult{}, &protocol.ProtocolError{Code: protocol.CodeInvalidParams, Message: "input.text is required"}
	}
	if s.taskStore == nil {
		return protocol.TurnCreateResult{}, &protocol.ProtocolError{Code: protocol.CodeInternalError, Message: "task store not configured"}
	}
	scope, err := s.resolveTeamOrRunScope(ctx, p.ThreadID, "", "")
	if err != nil {
		return protocol.TurnCreateResult{}, err
	}
	if strings.TrimSpace(scope.runID) == "" || strings.TrimSpace(scope.sessionID) == "" {
		return protocol.TurnCreateResult{}, &protocol.ProtocolError{Code: protocol.CodeInvalidState, Message: "run scope is unavailable"}
	}

	now := time.Now().UTC()
	taskID := "task-" + uuid.NewString()
	task := types.Task{
		TaskID:         taskID,
		SessionID:      strings.TrimSpace(scope.sessionID),
		RunID:          strings.TrimSpace(scope.runID),
		TaskKind:       state.TaskKindTask,
		AssignedToType: "agent",
		AssignedTo:     strings.TrimSpace(scope.runID),
		Goal:           strings.TrimSpace(p.Input.Text),
		Status:         types.TaskStatusPending,
		CreatedAt:      &now,
	}
	if err := s.taskStore.CreateTask(ctx, task); err != nil {
		return protocol.TurnCreateResult{}, err
	}
	if s.wake != nil {
		s.wake()
	}

	turn := protocol.Turn{
		ID:        protocol.TurnID(taskID),
		ThreadID:  protocol.ThreadID(strings.TrimSpace(scope.sessionID)),
		RunID:     protocol.RunID(strings.TrimSpace(scope.runID)),
		Status:    protocol.TurnStatusPending,
		CreatedAt: now,
	}
	return protocol.TurnCreateResult{Turn: turn}, nil
}

func (s *RPCServer) turnCancel(ctx context.Context, p protocol.TurnCancelParams) (protocol.TurnCancelResult, error) {
	turnID := strings.TrimSpace(string(p.TurnID))
	if turnID == "" {
		return protocol.TurnCancelResult{}, &protocol.ProtocolError{Code: protocol.CodeInvalidParams, Message: "turnId is required"}
	}
	if s.taskStore == nil {
		return protocol.TurnCancelResult{}, &protocol.ProtocolError{Code: protocol.CodeInternalError, Message: "task store not configured"}
	}
	task, err := s.taskStore.GetTask(ctx, turnID)
	if err != nil {
		if errors.Is(err, state.ErrTaskNotFound) {
			return protocol.TurnCancelResult{}, &protocol.ProtocolError{Code: protocol.CodeTurnNotFound, Message: "turn not found"}
		}
		return protocol.TurnCancelResult{}, err
	}
	switch strings.ToLower(strings.TrimSpace(string(task.Status))) {
	case string(types.TaskStatusPending):
		doneAt := time.Now().UTC()
		tr := types.TaskResult{
			TaskID:      task.TaskID,
			Status:      types.TaskStatusCanceled,
			Error:       "canceled",
			CompletedAt: &doneAt,
		}
		if err := s.taskStore.CompleteTask(ctx, task.TaskID, tr); err != nil {
			return protocol.TurnCancelResult{}, err
		}
		return protocol.TurnCancelResult{Turn: protocol.Turn{
			ID:        protocol.TurnID(task.TaskID),
			ThreadID:  protocol.ThreadID(strings.TrimSpace(task.SessionID)),
			RunID:     protocol.RunID(strings.TrimSpace(task.RunID)),
			Status:    protocol.TurnStatusCanceled,
			CreatedAt: timeutil.OrNow(task.CreatedAt),
		}}, nil

	case string(types.TaskStatusActive):
		return protocol.TurnCancelResult{}, &protocol.ProtocolError{Code: protocol.CodeTurnNotCancelable, Message: "turn is in progress"}

	case string(types.TaskStatusSucceeded):
		return protocol.TurnCancelResult{Turn: protocol.Turn{
			ID:        protocol.TurnID(task.TaskID),
			ThreadID:  protocol.ThreadID(strings.TrimSpace(task.SessionID)),
			RunID:     protocol.RunID(strings.TrimSpace(task.RunID)),
			Status:    protocol.TurnStatusCompleted,
			CreatedAt: timeutil.OrNow(task.CreatedAt),
		}}, nil

	case string(types.TaskStatusFailed):
		pe := &protocol.Error{Message: strings.TrimSpace(task.Error)}
		if pe.Message == "" {
			pe.Message = "task failed"
		}
		return protocol.TurnCancelResult{Turn: protocol.Turn{
			ID:        protocol.TurnID(task.TaskID),
			ThreadID:  protocol.ThreadID(strings.TrimSpace(task.SessionID)),
			RunID:     protocol.RunID(strings.TrimSpace(task.RunID)),
			Status:    protocol.TurnStatusFailed,
			CreatedAt: timeutil.OrNow(task.CreatedAt),
			Error:     pe,
		}}, nil

	case string(types.TaskStatusCanceled):
		return protocol.TurnCancelResult{Turn: protocol.Turn{
			ID:        protocol.TurnID(task.TaskID),
			ThreadID:  protocol.ThreadID(strings.TrimSpace(task.SessionID)),
			RunID:     protocol.RunID(strings.TrimSpace(task.RunID)),
			Status:    protocol.TurnStatusCanceled,
			CreatedAt: timeutil.OrNow(task.CreatedAt),
		}}, nil

	default:
		return protocol.TurnCancelResult{}, &protocol.ProtocolError{Code: protocol.CodeInvalidState, Message: "unknown turn state"}
	}
}

func (s *RPCServer) itemList(ctx context.Context, p protocol.ItemListParams) (protocol.ItemListResult, error) {
	_ = ctx
	if strings.TrimSpace(string(p.TurnID)) == "" {
		return protocol.ItemListResult{}, &protocol.ProtocolError{Code: protocol.CodeInvalidParams, Message: "turnId is required"}
	}
	if s.index == nil {
		return protocol.ItemListResult{Items: nil}, nil
	}
	items, next := s.index.ListByTurn(p.TurnID, strings.TrimSpace(p.Cursor), p.Limit)
	return protocol.ItemListResult{Items: items, NextCursor: next}, nil
}

type artifactScope struct {
	sessionID string
	teamID    string
	runID     string
}

func (s *RPCServer) resolveArtifactScope(ctx context.Context, threadID protocol.ThreadID, teamIDOverride string) (artifactScope, error) {
	resolvedThread, err := s.resolveThreadID(threadID)
	if err != nil {
		return artifactScope{}, err
	}

	teamID := strings.TrimSpace(teamIDOverride)
	scope := artifactScope{sessionID: resolvedThread}
	if sess, serr := s.loadSessionForID(ctx, resolvedThread); serr == nil {
		scope.sessionID = strings.TrimSpace(sess.SessionID)
		scope.runID = defaultRunIDForSession(sess)
		if teamID == "" {
			teamID = strings.TrimSpace(sess.TeamID)
		}
	}
	if teamID == "" && s.taskStore != nil && strings.TrimSpace(scope.runID) != "" {
		tasks, err := s.taskStore.ListTasks(ctx, state.TaskFilter{
			RunID:    strings.TrimSpace(scope.runID),
			SortBy:   "created_at",
			SortDesc: true,
			Limit:    1,
		})
		if err == nil && len(tasks) != 0 {
			teamID = strings.TrimSpace(tasks[0].TeamID)
		}
	}
	if teamID != "" {
		scope.teamID = teamID
		return scope, nil
	}
	if strings.TrimSpace(scope.runID) == "" {
		return artifactScope{}, &protocol.ProtocolError{Code: protocol.CodeInvalidParams, Message: "run scope is unavailable for thread"}
	}
	return scope, nil
}

func (s *RPCServer) artifactIndexer() (state.ArtifactIndexer, error) {
	if s == nil || s.taskStore == nil {
		return nil, &protocol.ProtocolError{Code: protocol.CodeInvalidState, Message: "task store not configured"}
	}
	indexer, ok := s.taskStore.(state.ArtifactIndexer)
	if !ok {
		return nil, &protocol.ProtocolError{Code: protocol.CodeInvalidState, Message: "artifact index is unavailable"}
	}
	return indexer, nil
}

func clampLimit(v, dflt, maxV int) int {
	if v <= 0 {
		return dflt
	}
	if v > maxV {
		return maxV
	}
	return v
}

func (s *RPCServer) artifactList(ctx context.Context, p protocol.ArtifactListParams) (protocol.ArtifactListResult, error) {
	scope, err := s.resolveArtifactScope(ctx, p.ThreadID, p.TeamID)
	if err != nil {
		return protocol.ArtifactListResult{}, err
	}
	indexer, err := s.artifactIndexer()
	if err != nil {
		return protocol.ArtifactListResult{}, err
	}
	groups, err := indexer.ListArtifactGroups(ctx, state.ArtifactFilter{
		TeamID:    scope.teamID,
		RunID:     scope.runID,
		DayBucket: strings.TrimSpace(p.DayBucket),
		Role:      strings.TrimSpace(p.Role),
		TaskKind:  strings.TrimSpace(p.TaskKind),
		TaskID:    strings.TrimSpace(p.TaskID),
		Limit:     clampLimit(p.Limit, artifactListDefaultLimit, artifactListMaxLimit),
	})
	if err != nil {
		return protocol.ArtifactListResult{}, err
	}
	return protocol.ArtifactListResult{Nodes: artifactGroupsToNodes(groups)}, nil
}

func applyScopeKey(filter *state.ArtifactFilter, scopeKey string) {
	if filter == nil {
		return
	}
	scopeKey = strings.TrimSpace(scopeKey)
	if scopeKey == "" {
		return
	}
	switch {
	case strings.HasPrefix(scopeKey, "day:"):
		if filter.DayBucket == "" {
			filter.DayBucket = strings.TrimPrefix(scopeKey, "day:")
		}
	case strings.HasPrefix(scopeKey, "role:"):
		parts := strings.Split(scopeKey, ":")
		if len(parts) >= 3 {
			if filter.DayBucket == "" {
				filter.DayBucket = strings.TrimSpace(parts[1])
			}
			if filter.Role == "" {
				filter.Role = strings.TrimSpace(parts[2])
			}
		}
	case strings.HasPrefix(scopeKey, "kind:"):
		parts := strings.Split(scopeKey, ":")
		if len(parts) >= 4 {
			if filter.DayBucket == "" {
				filter.DayBucket = strings.TrimSpace(parts[1])
			}
			if filter.Role == "" {
				filter.Role = strings.TrimSpace(parts[2])
			}
			if filter.TaskKind == "" {
				filter.TaskKind = strings.TrimSpace(parts[3])
			}
		}
	case strings.HasPrefix(scopeKey, "task:"):
		if filter.TaskID == "" {
			filter.TaskID = strings.TrimPrefix(scopeKey, "task:")
		}
	}
}

func (s *RPCServer) artifactSearch(ctx context.Context, p protocol.ArtifactSearchParams) (protocol.ArtifactSearchResult, error) {
	query := strings.TrimSpace(p.Query)
	if query == "" {
		return protocol.ArtifactSearchResult{}, &protocol.ProtocolError{Code: protocol.CodeInvalidParams, Message: "query is required"}
	}
	scope, err := s.resolveArtifactScope(ctx, p.ThreadID, p.TeamID)
	if err != nil {
		return protocol.ArtifactSearchResult{}, err
	}
	indexer, err := s.artifactIndexer()
	if err != nil {
		return protocol.ArtifactSearchResult{}, err
	}
	filter := state.ArtifactFilter{
		TeamID:    scope.teamID,
		RunID:     scope.runID,
		DayBucket: strings.TrimSpace(p.DayBucket),
		Role:      strings.TrimSpace(p.Role),
		TaskKind:  strings.TrimSpace(p.TaskKind),
		TaskID:    strings.TrimSpace(p.TaskID),
		Limit:     clampLimit(p.Limit, artifactSearchDefault, artifactSearchMax),
	}
	applyScopeKey(&filter, p.ScopeKey)
	matches, err := indexer.SearchArtifacts(ctx, state.ArtifactSearchFilter{
		ArtifactFilter: filter,
		Query:          query,
	})
	if err != nil {
		return protocol.ArtifactSearchResult{}, err
	}
	if len(matches) == 0 {
		return protocol.ArtifactSearchResult{Nodes: nil, MatchCount: 0}, nil
	}
	groups, err := indexer.ListArtifactGroups(ctx, state.ArtifactFilter{
		TeamID:    filter.TeamID,
		RunID:     filter.RunID,
		DayBucket: filter.DayBucket,
		Role:      filter.Role,
		TaskKind:  filter.TaskKind,
		TaskID:    filter.TaskID,
		Limit:     artifactListMaxLimit,
	})
	if err != nil {
		return protocol.ArtifactSearchResult{}, err
	}
	byTask := make(map[string]state.ArtifactGroup, len(groups))
	for _, g := range groups {
		byTask[strings.TrimSpace(g.TaskID)] = g
	}
	nodes := searchMatchesToNodes(matches, byTask)
	return protocol.ArtifactSearchResult{
		Nodes:      nodes,
		MatchCount: len(matches),
	}, nil
}

func resolveArtifactDiskPath(dataDir, teamID, runID, vpath string) string {
	vpath = strings.TrimSpace(vpath)
	if !strings.HasPrefix(vpath, "/workspace/") {
		return ""
	}
	rel := strings.TrimPrefix(vpath, "/workspace/")
	if strings.TrimSpace(teamID) != "" {
		return filepath.Join(fsutil.GetTeamWorkspaceDir(dataDir, teamID), rel)
	}
	return filepath.Join(fsutil.GetWorkspaceDir(dataDir, runID), rel)
}

func (s *RPCServer) artifactGet(ctx context.Context, p protocol.ArtifactGetParams) (protocol.ArtifactGetResult, error) {
	scope, err := s.resolveArtifactScope(ctx, p.ThreadID, p.TeamID)
	if err != nil {
		return protocol.ArtifactGetResult{}, err
	}
	artifactID := strings.TrimSpace(p.ArtifactID)
	vpath := strings.TrimSpace(p.VPath)
	if artifactID == "" && vpath == "" {
		return protocol.ArtifactGetResult{}, &protocol.ProtocolError{Code: protocol.CodeInvalidParams, Message: "artifactId or vpath is required"}
	}

	indexer, err := s.artifactIndexer()
	if err != nil {
		return protocol.ArtifactGetResult{}, err
	}
	groups, err := indexer.ListArtifactGroups(ctx, state.ArtifactFilter{
		TeamID: scope.teamID,
		RunID:  scope.runID,
		Limit:  artifactListMaxLimit,
	})
	if err != nil {
		return protocol.ArtifactGetResult{}, err
	}

	var sel state.ArtifactRecord
	var parent state.ArtifactGroup
	found := false
	for _, g := range groups {
		for _, f := range g.Files {
			if artifactID != "" && strings.TrimSpace(f.ArtifactID) == artifactID {
				sel, parent, found = f, g, true
				break
			}
			if vpath != "" && strings.TrimSpace(f.VPath) == vpath {
				sel, parent, found = f, g, true
				break
			}
		}
		if found {
			break
		}
	}
	if !found {
		return protocol.ArtifactGetResult{}, &protocol.ProtocolError{Code: protocol.CodeItemNotFound, Message: "artifact not found"}
	}

	maxBytes := clampLimit(p.MaxBytes, artifactGetDefaultBytes, artifactGetMaxBytes)
	diskPath := strings.TrimSpace(sel.DiskPath)
	if diskPath == "" {
		diskPath = resolveArtifactDiskPath(s.cfg.DataDir, scope.teamID, scope.runID, sel.VPath)
	}
	if strings.TrimSpace(diskPath) == "" {
		return protocol.ArtifactGetResult{}, &protocol.ProtocolError{Code: protocol.CodeItemNotFound, Message: "artifact file path unavailable"}
	}
	f, err := os.Open(diskPath)
	if err != nil {
		return protocol.ArtifactGetResult{}, &protocol.ProtocolError{Code: protocol.CodeItemNotFound, Message: "artifact file not found"}
	}
	defer f.Close()
	buf, err := io.ReadAll(io.LimitReader(f, int64(maxBytes)+1))
	if err != nil {
		return protocol.ArtifactGetResult{}, err
	}
	truncated := len(buf) > maxBytes
	if truncated {
		buf = buf[:maxBytes]
	}
	node := protocol.ArtifactNode{
		NodeKey:     "file:" + strings.TrimSpace(sel.VPath),
		ParentKey:   "task:" + strings.TrimSpace(parent.TaskID),
		Kind:        "file",
		Label:       strings.TrimSpace(sel.DisplayName),
		DayBucket:   strings.TrimSpace(parent.DayBucket),
		Role:        strings.TrimSpace(parent.Role),
		TaskKind:    strings.TrimSpace(parent.TaskKind),
		TaskID:      strings.TrimSpace(parent.TaskID),
		Status:      strings.TrimSpace(parent.Status),
		ArtifactID:  strings.TrimSpace(sel.ArtifactID),
		DisplayName: strings.TrimSpace(sel.DisplayName),
		VPath:       strings.TrimSpace(sel.VPath),
		DiskPath:    diskPath,
		IsSummary:   sel.IsSummary,
		ProducedAt:  sel.ProducedAt,
	}
	if node.Label == "" {
		node.Label = filepath.Base(node.VPath)
	}
	return protocol.ArtifactGetResult{
		Artifact:  node,
		Content:   string(buf),
		Truncated: truncated,
		BytesRead: len(buf),
	}, nil
}

func taskKindLabel(kind string) string {
	switch strings.TrimSpace(kind) {
	case state.TaskKindCallback:
		return "Callback Tasks"
	case state.TaskKindHeartbeat:
		return "Heartbeat Tasks"
	case state.TaskKindCoordinator:
		return "Coordinator Tasks"
	case state.TaskKindTask:
		return "Tasks"
	default:
		return "Other Tasks"
	}
}

func artifactGroupsToNodes(groups []state.ArtifactGroup) []protocol.ArtifactNode {
	out := make([]protocol.ArtifactNode, 0, len(groups)*6)
	lastDay := ""
	lastRole := ""
	lastKind := ""
	for _, g := range groups {
		day := strings.TrimSpace(g.DayBucket)
		role := strings.TrimSpace(g.Role)
		kind := strings.TrimSpace(g.TaskKind)
		taskID := strings.TrimSpace(g.TaskID)
		if taskID == "" {
			continue
		}
		dayKey := "day:" + day
		roleKey := "role:" + day + ":" + role
		kindKey := "kind:" + day + ":" + role + ":" + kind
		taskKey := "task:" + taskID

		if day != lastDay {
			lastDay = day
			lastRole = ""
			lastKind = ""
			out = append(out, protocol.ArtifactNode{
				NodeKey:   dayKey,
				Kind:      "day",
				Label:     day,
				DayBucket: day,
			})
		}
		if role != lastRole {
			lastRole = role
			lastKind = ""
			out = append(out, protocol.ArtifactNode{
				NodeKey:   roleKey,
				ParentKey: dayKey,
				Kind:      "role",
				Label:     role,
				DayBucket: day,
				Role:      role,
			})
		}
		if kind != lastKind {
			lastKind = kind
			out = append(out, protocol.ArtifactNode{
				NodeKey:   kindKey,
				ParentKey: roleKey,
				Kind:      "stream",
				Label:     taskKindLabel(kind),
				DayBucket: day,
				Role:      role,
				TaskKind:  kind,
			})
		}
		label := strings.TrimSpace(g.Goal)
		if label == "" {
			label = taskID
		}
		out = append(out, protocol.ArtifactNode{
			NodeKey:   taskKey,
			ParentKey: kindKey,
			Kind:      "task",
			Label:     label,
			DayBucket: day,
			Role:      role,
			TaskKind:  kind,
			TaskID:    taskID,
			Status:    strings.TrimSpace(g.Status),
		})
		for _, f := range g.Files {
			fileLabel := strings.TrimSpace(f.DisplayName)
			if fileLabel == "" {
				fileLabel = filepath.Base(strings.TrimSpace(f.VPath))
			}
			out = append(out, protocol.ArtifactNode{
				NodeKey:     "file:" + strings.TrimSpace(f.VPath),
				ParentKey:   taskKey,
				Kind:        "file",
				Label:       fileLabel,
				DayBucket:   day,
				Role:        role,
				TaskKind:    kind,
				TaskID:      taskID,
				Status:      strings.TrimSpace(g.Status),
				ArtifactID:  strings.TrimSpace(f.ArtifactID),
				DisplayName: strings.TrimSpace(f.DisplayName),
				VPath:       strings.TrimSpace(f.VPath),
				DiskPath:    strings.TrimSpace(f.DiskPath),
				IsSummary:   f.IsSummary,
				ProducedAt:  f.ProducedAt,
			})
		}
	}
	return out
}

func searchMatchesToNodes(matches []state.ArtifactRecord, byTask map[string]state.ArtifactGroup) []protocol.ArtifactNode {
	out := make([]protocol.ArtifactNode, 0, len(matches)*5)
	seen := map[string]struct{}{}
	add := func(n protocol.ArtifactNode) {
		if strings.TrimSpace(n.NodeKey) == "" {
			return
		}
		if _, ok := seen[n.NodeKey]; ok {
			return
		}
		seen[n.NodeKey] = struct{}{}
		out = append(out, n)
	}
	for _, f := range matches {
		taskID := strings.TrimSpace(f.TaskID)
		g := byTask[taskID]
		day := strings.TrimSpace(g.DayBucket)
		role := strings.TrimSpace(g.Role)
		kind := strings.TrimSpace(g.TaskKind)
		if day == "" {
			day = strings.TrimSpace(f.DayBucket)
		}
		if role == "" {
			role = strings.TrimSpace(f.Role)
		}
		if kind == "" {
			kind = strings.TrimSpace(f.TaskKind)
		}
		dayKey := "day:" + day
		roleKey := "role:" + day + ":" + role
		kindKey := "kind:" + day + ":" + role + ":" + kind
		taskKey := "task:" + taskID
		add(protocol.ArtifactNode{NodeKey: dayKey, Kind: "day", Label: day, DayBucket: day})
		add(protocol.ArtifactNode{NodeKey: roleKey, ParentKey: dayKey, Kind: "role", Label: role, DayBucket: day, Role: role})
		add(protocol.ArtifactNode{NodeKey: kindKey, ParentKey: roleKey, Kind: "stream", Label: taskKindLabel(kind), DayBucket: day, Role: role, TaskKind: kind})
		taskLabel := strings.TrimSpace(g.Goal)
		if taskLabel == "" {
			taskLabel = taskID
		}
		add(protocol.ArtifactNode{
			NodeKey: taskKey, ParentKey: kindKey, Kind: "task", Label: taskLabel,
			DayBucket: day, Role: role, TaskKind: kind, TaskID: taskID, Status: strings.TrimSpace(g.Status),
		})
		fileLabel := strings.TrimSpace(f.DisplayName)
		if fileLabel == "" {
			fileLabel = filepath.Base(strings.TrimSpace(f.VPath))
		}
		add(protocol.ArtifactNode{
			NodeKey: "file:" + strings.TrimSpace(f.VPath), ParentKey: taskKey, Kind: "file", Label: fileLabel,
			DayBucket: day, Role: role, TaskKind: kind, TaskID: taskID, Status: strings.TrimSpace(g.Status),
			ArtifactID: strings.TrimSpace(f.ArtifactID), DisplayName: strings.TrimSpace(f.DisplayName),
			VPath: strings.TrimSpace(f.VPath), DiskPath: strings.TrimSpace(f.DiskPath), IsSummary: f.IsSummary, ProducedAt: f.ProducedAt,
		})
	}
	return out
}

func truncateText(s string, max int) string {
	s = strings.TrimSpace(s)
	if max <= 0 || len(s) <= max {
		return s
	}
	if max <= 3 {
		return s[:max]
	}
	return s[:max-3] + "..."
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
