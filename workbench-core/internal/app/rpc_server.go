package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
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
	cfg config.Config
	run types.Run

	taskStore state.TaskStore
	session   pkgstore.SessionReaderWriter
	initErr   error

	notifyCh <-chan protocol.Message
	index    *protocol.Index

	wake func()
}

type RPCServerConfig struct {
	Cfg       config.Config
	Run       types.Run
	TaskStore state.TaskStore
	Session   pkgstore.SessionReaderWriter
	NotifyCh  <-chan protocol.Message
	Index     *protocol.Index
	Wake      func()
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
		cfg:       cfg.Cfg,
		run:       cfg.Run,
		taskStore: cfg.TaskStore,
		session:   sess,
		initErr:   initErr,
		notifyCh:  cfg.NotifyCh,
		index:     cfg.Index,
		wake:      cfg.Wake,
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
		select {
		case <-ctx.Done():
			return
		case <-stopCh:
			return
		case <-writerDone:
			return
		case outCh <- msg:
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

		resp := s.handleRequest(ctx, msg)
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

	default:
		return protocol.NewErrorResponse(id, &protocol.RPCError{Code: protocol.CodeMethodNotFound, Message: "method not found"})
	}
}

func (s *RPCServer) threadGet(ctx context.Context, p protocol.ThreadGetParams) (protocol.ThreadGetResult, error) {
	want := protocol.ThreadID(strings.TrimSpace(s.run.SessionID))
	if strings.TrimSpace(string(p.ThreadID)) == "" {
		return protocol.ThreadGetResult{}, &protocol.ProtocolError{Code: protocol.CodeInvalidParams, Message: "threadId is required"}
	}
	if strings.TrimSpace(string(p.ThreadID)) != strings.TrimSpace(string(want)) {
		return protocol.ThreadGetResult{}, &protocol.ProtocolError{Code: protocol.CodeThreadNotFound, Message: "thread not found"}
	}
	sess, err := s.session.LoadSession(ctx, s.run.SessionID)
	if err != nil {
		return protocol.ThreadGetResult{}, err
	}
	return protocol.ThreadGetResult{Thread: threadFromSession(s.run, sess)}, nil
}

func (s *RPCServer) threadCreate(ctx context.Context, p protocol.ThreadCreateParams) (protocol.ThreadCreateResult, error) {
	sess, err := s.session.LoadSession(ctx, s.run.SessionID)
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
		if err := s.session.SaveSession(ctx, sess); err != nil {
			return protocol.ThreadCreateResult{}, err
		}
	}
	return protocol.ThreadCreateResult{Thread: threadFromSession(s.run, sess)}, nil
}

func (s *RPCServer) turnCreate(ctx context.Context, p protocol.TurnCreateParams) (protocol.TurnCreateResult, error) {
	threadID := strings.TrimSpace(string(p.ThreadID))
	if threadID == "" {
		return protocol.TurnCreateResult{}, &protocol.ProtocolError{Code: protocol.CodeInvalidParams, Message: "threadId is required"}
	}
	if threadID != strings.TrimSpace(s.run.SessionID) {
		return protocol.TurnCreateResult{}, &protocol.ProtocolError{Code: protocol.CodeThreadNotFound, Message: "thread not found"}
	}
	if p.Input == nil || strings.TrimSpace(p.Input.Text) == "" {
		return protocol.TurnCreateResult{}, &protocol.ProtocolError{Code: protocol.CodeInvalidParams, Message: "input.text is required"}
	}
	if s.taskStore == nil {
		return protocol.TurnCreateResult{}, &protocol.ProtocolError{Code: protocol.CodeInternalError, Message: "task store not configured"}
	}

	now := time.Now().UTC()
	taskID := "task-" + uuid.NewString()
	task := types.Task{
		TaskID:    taskID,
		SessionID: strings.TrimSpace(s.run.SessionID),
		RunID:     strings.TrimSpace(s.run.RunID),
		Goal:      strings.TrimSpace(p.Input.Text),
		Status:    types.TaskStatusPending,
		CreatedAt: &now,
	}
	if err := s.taskStore.CreateTask(ctx, task); err != nil {
		return protocol.TurnCreateResult{}, err
	}
	if s.wake != nil {
		s.wake()
	}

	turn := protocol.Turn{
		ID:        protocol.TurnID(taskID),
		ThreadID:  protocol.ThreadID(strings.TrimSpace(s.run.SessionID)),
		RunID:     protocol.RunID(strings.TrimSpace(s.run.RunID)),
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
	if strings.TrimSpace(task.RunID) != strings.TrimSpace(s.run.RunID) || strings.TrimSpace(task.SessionID) != strings.TrimSpace(s.run.SessionID) {
		return protocol.TurnCancelResult{}, &protocol.ProtocolError{Code: protocol.CodeTurnNotFound, Message: "turn not found"}
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

func toRPCError(err error) *protocol.RPCError {
	var pe *protocol.ProtocolError
	if errors.As(err, &pe) {
		return pe.RPC()
	}
	return &protocol.RPCError{Code: protocol.CodeInternalError, Message: "internal error"}
}

func threadFromSession(run types.Run, sess types.Session) protocol.Thread {
	createdAt := timeutil.OrNow(sess.CreatedAt)
	return protocol.Thread{
		ID:          protocol.ThreadID(strings.TrimSpace(sess.SessionID)),
		Title:       strings.TrimSpace(sess.Title),
		CreatedAt:   createdAt,
		ActiveModel: strings.TrimSpace(sess.ActiveModel),
		ActiveRunID: protocol.RunID(strings.TrimSpace(run.RunID)),

		InputTokens:  sess.InputTokens,
		OutputTokens: sess.OutputTokens,
		TotalTokens:  sess.TotalTokens,
		CostUSD:      sess.CostUSD,
	}
}
