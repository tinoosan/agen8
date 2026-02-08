package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	implstore "github.com/tinoosan/workbench-core/internal/store"
	"github.com/tinoosan/workbench-core/pkg/agent/state"
	"github.com/tinoosan/workbench-core/pkg/config"
	"github.com/tinoosan/workbench-core/pkg/fsutil"
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

const (
	artifactListDefaultLimit = 200
	artifactListMaxLimit     = 2000
	artifactSearchDefault    = 100
	artifactSearchMax        = 1000
	artifactGetDefaultBytes  = 256 * 1024
	artifactGetMaxBytes      = 1024 * 1024
)

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

type artifactScope struct {
	teamID string
	runID  string
}

func (s *RPCServer) resolveArtifactScope(ctx context.Context, threadID protocol.ThreadID, teamIDOverride string) (artifactScope, error) {
	thread := strings.TrimSpace(string(threadID))
	if thread == "" {
		return artifactScope{}, &protocol.ProtocolError{Code: protocol.CodeInvalidParams, Message: "threadId is required"}
	}
	if thread != strings.TrimSpace(s.run.SessionID) {
		return artifactScope{}, &protocol.ProtocolError{Code: protocol.CodeThreadNotFound, Message: "thread not found"}
	}

	teamID := strings.TrimSpace(teamIDOverride)
	if teamID == "" && s.taskStore != nil {
		tasks, err := s.taskStore.ListTasks(ctx, state.TaskFilter{
			RunID:    strings.TrimSpace(s.run.RunID),
			SortBy:   "created_at",
			SortDesc: true,
			Limit:    1,
		})
		if err == nil && len(tasks) != 0 {
			teamID = strings.TrimSpace(tasks[0].TeamID)
		}
	}
	if teamID != "" {
		return artifactScope{teamID: teamID}, nil
	}
	return artifactScope{runID: strings.TrimSpace(s.run.RunID)}, nil
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
