package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	implstore "github.com/tinoosan/agen8/internal/store"
	"github.com/tinoosan/agen8/pkg/agent/state"
	"github.com/tinoosan/agen8/pkg/protocol"
	pkgagent "github.com/tinoosan/agen8/pkg/services/agent"
	"github.com/tinoosan/agen8/pkg/timeutil"
	"github.com/tinoosan/agen8/pkg/types"
)

var warnLegacyTeamFallbackOnce sync.Once

type sessionRunBatchReader interface {
	ListRunsBySessionIDs(ctx context.Context, sessionIDs []string) (map[string][]types.Run, error)
}

type sessionActivityBatchReader interface {
	ListActivitiesByRunIDs(ctx context.Context, runIDs []string, limit, offset int, sortDesc bool) ([]types.Activity, error)
	CountActivitiesByRunIDs(ctx context.Context, runIDs []string) (int, error)
}

func registerSessionHandlers(s *RPCServer, reg methodRegistry) error {
	return registerHandlers(
		func() error {
			return addBoundHandler[protocol.ThreadGetParams, protocol.ThreadGetResult](reg, protocol.MethodThreadGet, false, s.threadGet)
		},
		func() error {
			return addBoundHandler[protocol.ThreadCreateParams, protocol.ThreadCreateResult](reg, protocol.MethodThreadCreate, true, s.threadCreate)
		},
		func() error {
			return addBoundHandler[protocol.TurnCreateParams, protocol.TurnCreateResult](reg, protocol.MethodTurnCreate, false, s.turnCreate)
		},
		func() error {
			return addBoundHandler[protocol.TurnCancelParams, protocol.TurnCancelResult](reg, protocol.MethodTurnCancel, false, s.turnCancel)
		},
		func() error {
			return addBoundHandler[protocol.ItemListParams, protocol.ItemListResult](reg, protocol.MethodItemList, false, s.itemList)
		},
		func() error {
			return addBoundHandler[protocol.MessageListParams, protocol.MessageListResult](reg, protocol.MethodMessageList, false, s.messageList)
		},
		func() error {
			return addBoundHandler[protocol.MessageGetParams, protocol.MessageGetResult](reg, protocol.MethodMessageGet, false, s.messageGet)
		},
		func() error {
			return addBoundHandler[protocol.TaskListParams, protocol.TaskListResult](reg, protocol.MethodTaskList, false, s.taskList)
		},
		func() error {
			return addBoundHandler[protocol.TaskCreateParams, protocol.TaskCreateResult](reg, protocol.MethodTaskCreate, false, s.taskCreate)
		},
		func() error {
			return addBoundHandler[protocol.TaskClaimParams, protocol.TaskClaimResult](reg, protocol.MethodTaskClaim, false, s.taskClaim)
		},
		func() error {
			return addBoundHandler[protocol.TaskCompleteParams, protocol.TaskCompleteResult](reg, protocol.MethodTaskComplete, false, s.taskComplete)
		},
		func() error {
			return addBoundHandler[protocol.SessionStartParams, protocol.SessionStartResult](reg, protocol.MethodSessionStart, false, s.sessionStart)
		},
		func() error {
			return addBoundHandler[protocol.SessionListParams, protocol.SessionListResult](reg, protocol.MethodSessionList, false, s.sessionList)
		},
		func() error {
			return addBoundHandler[protocol.SessionRenameParams, protocol.SessionRenameResult](reg, protocol.MethodSessionRename, false, s.sessionRename)
		},
		func() error {
			return addBoundHandler[protocol.AgentListParams, protocol.AgentListResult](reg, protocol.MethodAgentList, false, s.agentList)
		},
		func() error {
			return addBoundHandler[protocol.AgentStartParams, protocol.AgentStartResult](reg, protocol.MethodAgentStart, false, s.agentStart)
		},
		func() error {
			return addBoundHandler[protocol.AgentPauseParams, protocol.AgentPauseResult](reg, protocol.MethodAgentPause, false, s.agentPauseHandler)
		},
		func() error {
			return addBoundHandler[protocol.AgentResumeParams, protocol.AgentResumeResult](reg, protocol.MethodAgentResume, false, s.agentResumeHandler)
		},
		func() error {
			return addBoundHandler[protocol.SessionPauseParams, protocol.SessionPauseResult](reg, protocol.MethodSessionPause, false, s.sessionPauseHandler)
		},
		func() error {
			return addBoundHandler[protocol.SessionResumeParams, protocol.SessionResumeResult](reg, protocol.MethodSessionResume, false, s.sessionResumeHandler)
		},
		func() error {
			return addBoundHandler[protocol.SessionStopParams, protocol.SessionStopResult](reg, protocol.MethodSessionStop, false, s.sessionStopHandler)
		},
		func() error {
			return addBoundHandler[protocol.SessionClearHistoryParams, protocol.SessionClearHistoryResult](reg, protocol.MethodSessionClearHistory, false, s.sessionClearHistory)
		},
		func() error {
			return addBoundHandler[protocol.SessionDeleteParams, protocol.SessionDeleteResult](reg, protocol.MethodSessionDelete, false, s.sessionDelete)
		},
		func() error {
			return addBoundHandler[protocol.SessionGetTotalsParams, protocol.SessionGetTotalsResult](reg, protocol.MethodSessionGetTotals, false, s.sessionGetTotals)
		},
		func() error {
			return addBoundHandler[protocol.SessionResolveThreadParams, protocol.SessionResolveThreadResult](reg, protocol.MethodSessionResolveThread, false, s.sessionResolveThread)
		},
		func() error {
			return addBoundHandler[protocol.ActivityListParams, protocol.ActivityListResult](reg, protocol.MethodActivityList, false, s.activityList)
		},
		func() error {
			return addBoundHandler[protocol.RunListChildrenParams, protocol.RunListChildrenResult](reg, protocol.MethodRunListChildren, false, s.runListChildren)
		},
	)
}

func (s *RPCServer) sessionResolveThread(ctx context.Context, p protocol.SessionResolveThreadParams) (protocol.SessionResolveThreadResult, error) {
	sessionID := strings.TrimSpace(p.SessionID)
	if sessionID == "" {
		return protocol.SessionResolveThreadResult{}, &protocol.ProtocolError{Code: protocol.CodeInvalidParams, Message: "sessionId is required"}
	}
	sess, err := s.session.LoadSession(ctx, sessionID)
	if err != nil {
		return protocol.SessionResolveThreadResult{}, err
	}
	runID := strings.TrimSpace(p.RunID)
	if runID == "" {
		runID = defaultRunIDForSession(sess)
	}
	return protocol.SessionResolveThreadResult{
		SessionID: strings.TrimSpace(sess.SessionID),
		ThreadID:  strings.TrimSpace(sess.SessionID),
		RunID:     strings.TrimSpace(runID),
		TeamID:    strings.TrimSpace(sess.TeamID),
		Exists:    true,
	}, nil
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
	source := taskMetaString(t.Metadata, "source")
	batchMode := taskMetaBool(t.Metadata, "batchMode")
	batchSynthetic := taskMetaBool(t.Metadata, "batchSynthetic")
	batchDelivered := taskMetaBool(t.Metadata, "batchDelivered")
	batchParentTaskID := taskMetaString(t.Metadata, "batchParentTaskId")
	batchWaveID := taskMetaString(t.Metadata, "batchWaveId")
	batchIncludedIn := taskMetaString(t.Metadata, "batchIncludedIn")

	return protocol.Task{
		ID:                strings.TrimSpace(t.TaskID),
		ThreadID:          protocol.ThreadID(strings.TrimSpace(t.SessionID)),
		RunID:             protocol.RunID(strings.TrimSpace(t.RunID)),
		TeamID:            strings.TrimSpace(t.TeamID),
		Source:            source,
		BatchMode:         batchMode,
		BatchSynthetic:    batchSynthetic,
		BatchDelivered:    batchDelivered,
		BatchParentTaskID: batchParentTaskID,
		BatchWaveID:       batchWaveID,
		BatchIncludedIn:   batchIncludedIn,
		TaskKind:          strings.TrimSpace(t.TaskKind),
		AssignedToType:    strings.TrimSpace(t.AssignedToType),
		AssignedTo:        strings.TrimSpace(t.AssignedTo),
		AssignedRole:      strings.TrimSpace(t.AssignedRole),
		ClaimedByAgentID:  strings.TrimSpace(t.ClaimedByAgentID),
		RoleSnapshot:      strings.TrimSpace(t.RoleSnapshot),
		Goal:              strings.TrimSpace(t.Goal),
		Status:            strings.TrimSpace(string(t.Status)),
		Summary:           strings.TrimSpace(t.Summary),
		Error:             strings.TrimSpace(t.Error),
		Artifacts:         append([]string(nil), t.Artifacts...),
		InputTokens:       t.InputTokens,
		OutputTokens:      t.OutputTokens,
		TotalTokens:       t.TotalTokens,
		CostUSD:           t.CostUSD,
		CreatedAt:         timeutil.OrNow(t.CreatedAt),
		CompletedAt:       timeutil.OrNow(t.CompletedAt),
	}
}

func taskMetaString(metadata map[string]any, key string) string {
	if len(metadata) == 0 {
		return ""
	}
	raw, ok := metadata[key]
	if !ok || raw == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(raw))
}

func taskMetaBool(metadata map[string]any, key string) bool {
	if len(metadata) == 0 {
		return false
	}
	raw, ok := metadata[key]
	if !ok || raw == nil {
		return false
	}
	switch v := raw.(type) {
	case bool:
		return v
	case string:
		return strings.EqualFold(strings.TrimSpace(v), "true")
	case int:
		return v != 0
	case int32:
		return v != 0
	case int64:
		return v != 0
	case float32:
		return v != 0
	case float64:
		return v != 0
	default:
		return strings.EqualFold(strings.TrimSpace(fmt.Sprint(v)), "true")
	}
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
		View:     view,
		SortBy:   "created_at",
		SortDesc: true,
		Limit:    clampLimit(p.Limit, 200, 2000),
		Offset:   max(0, p.Offset),
	}
	scopeMode := strings.ToLower(strings.TrimSpace(p.Scope))
	if scopeMode != "" && scopeMode != "team" && scopeMode != "run" {
		return protocol.TaskListResult{}, &protocol.ProtocolError{Code: protocol.CodeInvalidParams, Message: "scope must be team or run"}
	}
	if scopeMode == "team" || (strings.TrimSpace(scope.teamID) != "" && strings.TrimSpace(p.RunID) == "") {
		filter.RunID = ""
	}
	if scopeMode == "run" && strings.TrimSpace(scope.runID) == "" {
		return protocol.TaskListResult{}, &protocol.ProtocolError{Code: protocol.CodeInvalidParams, Message: "run scope requires runId"}
	}
	switch view {
	case "inbox":
		filter.Status = []types.TaskStatus{types.TaskStatusPending, types.TaskStatusActive, types.TaskStatusReviewPending}
		filter.SortBy = "created_at"
		filter.SortDesc = false
	case "outbox":
		filter.Status = []types.TaskStatus{
			types.TaskStatusSucceeded,
			types.TaskStatusFailed,
			types.TaskStatusCanceled,
			types.TaskStatusReviewPending,
		}
		filter.SortBy = "finished_at"
		filter.SortDesc = true
	}
	if at, av := parseAssignee(p.Assignee); av != "" {
		filter.AssignedTo = av
		filter.AssignedToType = at
	}
	tasks, err := s.taskService.ListTasks(ctx, filter)
	if err != nil {
		return protocol.TaskListResult{}, err
	}
	total, err := s.taskService.CountTasks(ctx, filter)
	if err != nil {
		return protocol.TaskListResult{}, err
	}
	out := make([]protocol.Task, 0, len(tasks))
	for _, t := range tasks {
		out = append(out, protocolTaskFromTypesTask(t))
	}
	return protocol.TaskListResult{Tasks: out, TotalCount: total}, nil
}

func (s *RPCServer) sessionStart(ctx context.Context, p protocol.SessionStartParams) (protocol.SessionStartResult, error) {
	return newSessionStartService(s).sessionStart(ctx, p)
}

func (s *RPCServer) sessionList(ctx context.Context, p protocol.SessionListParams) (protocol.SessionListResult, error) {
	return newSessionQueryService(s).sessionList(ctx, p)
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
	if s.agentService == nil {
		return protocol.AgentListResult{}, &protocol.ProtocolError{Code: protocol.CodeInternalError, Message: "agent service not configured"}
	}
	threadID, err := s.resolveThreadID(p.ThreadID)
	if err != nil {
		return protocol.AgentListResult{}, err
	}
	sessionID := strings.TrimSpace(p.SessionID)
	if sessionID == "" {
		sessionID = threadID
	}
	list, err := s.agentService.List(ctx, sessionID)
	if err != nil {
		return protocol.AgentListResult{}, asProtocolError(err)
	}
	out := make([]protocol.AgentListItem, 0, len(list))
	for _, info := range list {
		out = append(out, protocol.AgentListItem{
			RunID:       info.RunID,
			SessionID:   info.SessionID,
			Status:      info.Status,
			Goal:        info.Goal,
			ParentRunID: info.ParentRunID,
			SpawnIndex:  info.SpawnIndex,
			Profile:     info.Profile,
			Role:        info.Role,
			TeamID:      info.TeamID,
			StartedAt:   info.StartedAt,
			FinishedAt:  info.FinishedAt,
		})
	}
	return protocol.AgentListResult{Agents: out}, nil
}

func asProtocolError(err error) *protocol.ProtocolError {
	if err == nil {
		return nil
	}
	var pe *protocol.ProtocolError
	if errors.As(err, &pe) {
		return pe
	}
	var se *pkgagent.ServiceError
	if errors.As(err, &se) {
		return &protocol.ProtocolError{Code: se.Code, Message: se.Message}
	}
	return &protocol.ProtocolError{Code: protocol.CodeInternalError, Message: err.Error()}
}

func (s *RPCServer) runListChildren(ctx context.Context, p protocol.RunListChildrenParams) (protocol.RunListChildrenResult, error) {
	parentRunID := strings.TrimSpace(p.ParentRunID)
	if parentRunID == "" {
		return protocol.RunListChildrenResult{Runs: nil}, nil
	}
	runs, err := s.session.ListChildRuns(ctx, parentRunID)
	if err != nil {
		return protocol.RunListChildrenResult{}, err
	}
	return protocol.RunListChildrenResult{Runs: runs}, nil
}

func (s *RPCServer) agentStart(ctx context.Context, p protocol.AgentStartParams) (protocol.AgentStartResult, error) {
	if s.agentService == nil {
		return protocol.AgentStartResult{}, &protocol.ProtocolError{Code: protocol.CodeInternalError, Message: "agent service not configured"}
	}
	threadID, err := s.resolveThreadID(p.ThreadID)
	if err != nil {
		return protocol.AgentStartResult{}, err
	}
	sessionID := strings.TrimSpace(p.SessionID)
	if sessionID == "" {
		sessionID = threadID
	}
	maxContext := s.run.MaxBytesForContext
	if maxContext <= 0 {
		maxContext = 8 * 1024
	}
	opts := pkgagent.StartOptions{
		SessionID:          sessionID,
		Goal:               strings.TrimSpace(p.Goal),
		Profile:            strings.TrimSpace(p.Profile),
		Model:              strings.TrimSpace(p.Model),
		MaxBytesForContext: maxContext,
	}
	res, err := s.agentService.Start(ctx, opts)
	if err != nil {
		return protocol.AgentStartResult{}, asProtocolError(err)
	}
	sess, err := s.session.LoadSession(ctx, res.SessionID)
	if err != nil {
		return protocol.AgentStartResult{}, err
	}
	ensureSessionReasoningForModel(&sess, sess.ActiveModel, "", "")
	if err := s.session.SaveSession(ctx, sess); err != nil {
		return protocol.AgentStartResult{}, err
	}
	return protocol.AgentStartResult{
		RunID:     res.RunID,
		SessionID: res.SessionID,
		Profile:   res.Profile,
		Model:     res.Model,
	}, nil
}

func (s *RPCServer) agentPauseHandler(ctx context.Context, p protocol.AgentPauseParams) (protocol.AgentPauseResult, error) {
	if s.agentService == nil {
		return protocol.AgentPauseResult{}, &protocol.ProtocolError{Code: protocol.CodeInternalError, Message: "agent service not configured"}
	}
	threadID, err := s.resolveThreadID(p.ThreadID)
	if err != nil {
		return protocol.AgentPauseResult{}, err
	}
	runID := strings.TrimSpace(p.RunID)
	if runID == "" {
		return protocol.AgentPauseResult{}, &protocol.ProtocolError{Code: protocol.CodeInvalidParams, Message: "runId is required"}
	}
	if err := s.agentService.Pause(ctx, runID, threadID); err != nil {
		return protocol.AgentPauseResult{}, asProtocolError(err)
	}
	return protocol.AgentPauseResult{RunID: runID, Status: types.RunStatusPaused}, nil
}

func (s *RPCServer) agentResumeHandler(ctx context.Context, p protocol.AgentResumeParams) (protocol.AgentResumeResult, error) {
	if s.agentService == nil {
		return protocol.AgentResumeResult{}, &protocol.ProtocolError{Code: protocol.CodeInternalError, Message: "agent service not configured"}
	}
	threadID, err := s.resolveThreadID(p.ThreadID)
	if err != nil {
		return protocol.AgentResumeResult{}, err
	}
	runID := strings.TrimSpace(p.RunID)
	if runID == "" {
		return protocol.AgentResumeResult{}, &protocol.ProtocolError{Code: protocol.CodeInvalidParams, Message: "runId is required"}
	}
	if err := s.agentService.Resume(ctx, runID, threadID); err != nil {
		return protocol.AgentResumeResult{}, asProtocolError(err)
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

func (s *RPCServer) sessionStopHandler(ctx context.Context, p protocol.SessionStopParams) (protocol.SessionStopResult, error) {
	threadID, err := s.resolveThreadID(p.ThreadID)
	if err != nil {
		return protocol.SessionStopResult{}, err
	}
	sessionID := strings.TrimSpace(p.SessionID)
	if sessionID == "" {
		sessionID = threadID
	}
	if sessionID == "" {
		return protocol.SessionStopResult{}, &protocol.ProtocolError{Code: protocol.CodeInvalidParams, Message: "sessionId is required"}
	}
	if s.sessionStop != nil {
		affected, err := s.sessionStop(ctx, threadID, sessionID)
		if err != nil {
			return protocol.SessionStopResult{}, err
		}
		return protocol.SessionStopResult{SessionID: sessionID, AffectedRunIDs: affected}, nil
	}
	affected, err := s.setSessionStoppedState(ctx, threadID, sessionID)
	if err != nil {
		return protocol.SessionStopResult{}, err
	}
	return protocol.SessionStopResult{SessionID: sessionID, AffectedRunIDs: affected}, nil
}

func (s *RPCServer) sessionDelete(ctx context.Context, p protocol.SessionDeleteParams) (protocol.SessionDeleteResult, error) {
	if s.session == nil {
		return protocol.SessionDeleteResult{}, &protocol.ProtocolError{Code: protocol.CodeInternalError, Message: "session service not initialized"}
	}
	sessionID := strings.TrimSpace(p.SessionID)
	if sessionID == "" {
		return protocol.SessionDeleteResult{}, &protocol.ProtocolError{Code: protocol.CodeInvalidParams, Message: "sessionId is required"}
	}
	sess, err := s.session.LoadSession(ctx, sessionID)
	if err != nil {
		return protocol.SessionDeleteResult{}, err
	}
	// We call Delete on the service, which handles stopping runs and cleaning up storage.
	if err := s.session.Delete(ctx, sessionID); err != nil {
		return protocol.SessionDeleteResult{}, err
	}
	projectRoot := strings.TrimSpace(sess.ProjectRoot)
	teamID := strings.TrimSpace(sess.TeamID)
	if projectRoot != "" && teamID != "" {
		if projectCtx, err := LoadProjectContext(projectRoot); err == nil && projectCtx.Exists && strings.TrimSpace(projectCtx.State.ActiveSessionID) == sessionID {
			nextState := ProjectState{
				ActiveTeamID: teamID,
				LastCommand:  "session.delete",
			}
			if s.projectTeamSvc == nil {
				nextState = ProjectState{LastCommand: "session.delete"}
			} else if teamSummary, err := s.projectTeamSvc.GetTeam(ctx, projectRoot, teamID); err != nil || strings.TrimSpace(teamSummary.TeamID) == "" {
				nextState = ProjectState{LastCommand: "session.delete"}
			}
			_, _ = SetActiveSession(projectRoot, nextState)
		}
	}
	return protocol.SessionDeleteResult{SessionID: sessionID}, nil
}

func (s *RPCServer) sessionClearHistory(ctx context.Context, p protocol.SessionClearHistoryParams) (protocol.SessionClearHistoryResult, error) {
	if s.session == nil {
		return protocol.SessionClearHistoryResult{}, &protocol.ProtocolError{Code: protocol.CodeInternalError, Message: "session service not initialized"}
	}
	threadID, err := s.resolveThreadID(p.ThreadID)
	if err != nil {
		return protocol.SessionClearHistoryResult{}, err
	}
	scope, err := s.resolveTeamOrRunScope(ctx, p.ThreadID, p.TeamID, "")
	if err != nil {
		return protocol.SessionClearHistoryResult{}, err
	}
	if teamID := strings.TrimSpace(scope.teamID); teamID != "" {
		runIDs, _ := s.loadTeamManifestRunRoles(ctx, teamID)
		if len(runIDs) == 0 && s.taskService != nil {
			tasks, _ := s.taskService.ListTasks(ctx, state.TaskFilter{
				TeamID:   teamID,
				Limit:    1000,
				SortBy:   "created_at",
				SortDesc: true,
			})
			seen := map[string]struct{}{}
			for _, t := range tasks {
				runID := strings.TrimSpace(t.RunID)
				if runID == "" {
					continue
				}
				if _, ok := seen[runID]; ok {
					continue
				}
				seen[runID] = struct{}{}
				runIDs = append(runIDs, runID)
			}
		}
		cleared, err := implstore.ClearHistoryForRunIDs(s.cfg, runIDs)
		if err != nil {
			return protocol.SessionClearHistoryResult{}, err
		}
		return protocol.SessionClearHistoryResult{
			TeamID:              teamID,
			SourceRuns:          append([]string(nil), cleared.SourceRuns...),
			EventsDeleted:       cleared.EventsDeleted,
			HistoryDeleted:      cleared.HistoryDeleted,
			ActivitiesDeleted:   cleared.ActivitiesDeleted,
			ConstructorState:    cleared.ConstructorState,
			ConstructorManifest: cleared.ConstructorManifest,
		}, nil
	}

	sessionID := strings.TrimSpace(p.SessionID)
	if sessionID == "" {
		sessionID = strings.TrimSpace(threadID)
	}
	if sessionID == "" {
		return protocol.SessionClearHistoryResult{}, &protocol.ProtocolError{Code: protocol.CodeInvalidParams, Message: "sessionId is required"}
	}
	sess, err := s.session.LoadSession(ctx, sessionID)
	if err != nil {
		return protocol.SessionClearHistoryResult{}, err
	}
	if strings.TrimSpace(sess.SessionID) != strings.TrimSpace(threadID) {
		return protocol.SessionClearHistoryResult{}, &protocol.ProtocolError{Code: protocol.CodeThreadNotFound, Message: "thread not found"}
	}
	cleared, err := implstore.ClearHistoryForSession(s.cfg, sessionID)
	if err != nil {
		return protocol.SessionClearHistoryResult{}, err
	}
	return protocol.SessionClearHistoryResult{
		SessionID:           sessionID,
		SourceRuns:          append([]string(nil), cleared.SourceRuns...),
		EventsDeleted:       cleared.EventsDeleted,
		HistoryDeleted:      cleared.HistoryDeleted,
		ActivitiesDeleted:   cleared.ActivitiesDeleted,
		ConstructorState:    cleared.ConstructorState,
		ConstructorManifest: cleared.ConstructorManifest,
	}, nil
}

func (s *RPCServer) setSessionPausedState(ctx context.Context, threadID, sessionID string, paused bool) ([]string, error) {
	if s.agentService == nil {
		return nil, &protocol.ProtocolError{Code: protocol.CodeInternalError, Message: "agent service not configured"}
	}
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
		runID = strings.TrimSpace(runID)
		if runID == "" {
			continue
		}
		if paused {
			if err := s.agentService.Pause(ctx, runID, sessionID); err != nil {
				return affected, asProtocolError(err)
			}
		} else {
			if err := s.agentService.Resume(ctx, runID, sessionID); err != nil {
				return affected, asProtocolError(err)
			}
		}
		affected = append(affected, runID)
	}
	return affected, nil
}

func (s *RPCServer) setSessionStoppedState(ctx context.Context, threadID, sessionID string) ([]string, error) {
	if s.agentService == nil {
		return nil, &protocol.ProtocolError{Code: protocol.CodeInternalError, Message: "agent service not configured"}
	}
	if strings.TrimSpace(threadID) == "" {
		return nil, &protocol.ProtocolError{Code: protocol.CodeInvalidParams, Message: "threadId is required"}
	}
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
		runID = strings.TrimSpace(runID)
		if runID == "" {
			continue
		}
		if err := s.agentService.Pause(ctx, runID, sessionID); err != nil {
			return affected, asProtocolError(err)
		}
		loaded, lerr := s.session.LoadRun(ctx, runID)
		if lerr != nil {
			return affected, fmt.Errorf("load run %s: %w", runID, lerr)
		}
		loaded.Status = types.RunStatusPaused
		loaded.FinishedAt = nil
		loaded.Error = nil
		if err := s.session.SaveRun(ctx, loaded); err != nil {
			return affected, fmt.Errorf("save run %s: %w", runID, err)
		}
		affected = append(affected, runID)
	}
	return affected, nil
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

func legacyTeamFallbackEnabled() bool {
	raw := strings.ToLower(strings.TrimSpace(os.Getenv("AGEN8_LEGACY_TEAM_FALLBACK")))
	enabled := raw == "1" || raw == "true" || raw == "yes" || raw == "on"
	if enabled {
		warnLegacyTeamFallbackOnce.Do(func() {
			log.Printf("rpc: AGEN8_LEGACY_TEAM_FALLBACK is enabled; implicit team fallback remains active and should be removed after compatibility window")
		})
	}
	return enabled
}

func (s *RPCServer) deriveDefaultTeamRole(ctx context.Context, scope artifactScope) string {
	runID := strings.TrimSpace(scope.runID)
	if runID != "" && s.session != nil {
		if run, err := s.session.LoadRun(ctx, runID); err == nil && run.Runtime != nil {
			if role := strings.TrimSpace(run.Runtime.Role); role != "" {
				return role
			}
		}
	}
	teamID := strings.TrimSpace(scope.teamID)
	if teamID == "" || runID == "" {
		return ""
	}
	_, roleByRun := s.loadTeamManifestRunRoles(ctx, teamID)
	if len(roleByRun) == 0 {
		return ""
	}
	return strings.TrimSpace(roleByRun[runID])
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
	rawAssignedToType := strings.TrimSpace(p.AssignedToType)
	assignedToType := normalizeAssignedToType(p.AssignedToType)
	if rawAssignedToType != "" && assignedToType == "" {
		return protocol.TaskCreateResult{}, &protocol.ProtocolError{Code: protocol.CodeInvalidParams, Message: "assignedToType must be one of: role, agent, team"}
	}
	assignedTo := strings.TrimSpace(p.AssignedTo)
	assignedRole := strings.TrimSpace(p.AssignedRole)
	teamScope := strings.TrimSpace(scope.teamID) != ""
	if assignedToType == "" {
		if teamScope {
			if assignedRole != "" {
				assignedToType = "role"
				assignedTo = assignedRole
			} else if derivedRole := s.deriveDefaultTeamRole(ctx, scope); derivedRole != "" {
				assignedRole = derivedRole
				assignedToType = "role"
				assignedTo = derivedRole
			} else if legacyTeamFallbackEnabled() {
				assignedToType = "team"
				assignedTo = scope.teamID
			} else {
				return protocol.TaskCreateResult{}, &protocol.ProtocolError{
					Code:    protocol.CodeInvalidParams,
					Message: "team task routing requires explicit assignee (assignedRole or assignedToType+assignedTo)",
				}
			}
		} else {
			assignedToType = "agent"
			assignedTo = scope.runID
		}
	}
	if assignedTo == "" {
		switch assignedToType {
		case "team":
			if !teamScope {
				return protocol.TaskCreateResult{}, &protocol.ProtocolError{Code: protocol.CodeInvalidParams, Message: "assignedToType=team requires team scope"}
			}
			assignedTo = scope.teamID
		case "role":
			if assignedRole == "" && teamScope {
				assignedRole = s.deriveDefaultTeamRole(ctx, scope)
			}
			assignedTo = assignedRole
			if strings.TrimSpace(assignedTo) == "" {
				return protocol.TaskCreateResult{}, &protocol.ProtocolError{Code: protocol.CodeInvalidParams, Message: "assignedToType=role requires assignedRole or assignedTo"}
			}
		case "agent":
			assignedTo = scope.runID
			if strings.TrimSpace(assignedTo) == "" {
				return protocol.TaskCreateResult{}, &protocol.ProtocolError{Code: protocol.CodeInvalidParams, Message: "assignedToType=agent requires assignedTo or run scope"}
			}
		}
	}
	if assignedToType == "role" && assignedRole == "" {
		assignedRole = strings.TrimSpace(assignedTo)
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
	if err := s.taskService.CreateTask(ctx, task); err != nil {
		return protocol.TaskCreateResult{}, err
	}

	if s.eventsService != nil && strings.TrimSpace(strings.ToLower(task.TaskKind)) == "user_message" {
		opID := "user-msg-" + uuid.NewString()
		goal := strings.TrimSpace(task.Goal)
		if goal == "" {
			goal = "user message"
		}

		_ = s.eventsService.Append(ctx, types.EventRecord{
			EventID:   uuid.NewString(),
			Type:      "agent.op.request",
			RunID:     task.RunID,
			Message:   goal,
			Timestamp: time.Now(),
			Data: map[string]string{
				"opId":          opID,
				"op":            "user_message",
				"path":          "coordinator",
				"textPreview":   goal,
				"textTruncated": "false",
				"textRedacted":  "false",
				"textIsJSON":    "false",
				"textBytes":     goal,
			},
		})

		_ = s.eventsService.Append(ctx, types.EventRecord{
			EventID:   uuid.NewString(),
			Type:      "agent.op.response",
			RunID:     task.RunID,
			Message:   goal,
			Timestamp: time.Now(),
			Data: map[string]string{
				"opId":          opID,
				"op":            "user_message",
				"path":          "coordinator",
				"ok":            "true",
				"outputPreview": goal,
				"responseOnly":  "true",
			},
		})
	}

	if s.wake != nil {
		s.wake()
	}
	got, err := s.taskService.GetTask(ctx, taskID)
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
	if err := s.taskService.ClaimTask(ctx, taskID, 2*time.Minute); err != nil {
		if errors.Is(err, state.ErrTaskClaimed) {
			return protocol.TaskClaimResult{}, &protocol.ProtocolError{Code: protocol.CodeInvalidState, Message: "task already claimed"}
		}
		if errors.Is(err, state.ErrTaskNotFound) {
			return protocol.TaskClaimResult{}, &protocol.ProtocolError{Code: protocol.CodeTurnNotFound, Message: "task not found"}
		}
		if errors.Is(err, state.ErrTaskMissingMessage) {
			return protocol.TaskClaimResult{}, &protocol.ProtocolError{Code: protocol.CodeInvalidState, Message: "task has no backing message envelope"}
		}
		if errors.Is(err, state.ErrMessageClaimed) {
			return protocol.TaskClaimResult{}, &protocol.ProtocolError{Code: protocol.CodeInvalidState, Message: "task message already claimed"}
		}
		if errors.Is(err, state.ErrMessageTerminal) || errors.Is(err, state.ErrMessageNotClaimable) {
			return protocol.TaskClaimResult{}, &protocol.ProtocolError{Code: protocol.CodeInvalidState, Message: "task message is not claimable"}
		}
		return protocol.TaskClaimResult{}, err
	}
	task, err := s.taskService.GetTask(ctx, taskID)
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
	if err := s.taskService.UpdateTask(ctx, task); err != nil {
		return protocol.TaskClaimResult{}, err
	}
	task, err = s.taskService.GetTask(ctx, taskID)
	if err != nil {
		return protocol.TaskClaimResult{}, err
	}
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
	task, err := s.taskService.GetTask(ctx, taskID)
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
	if err := s.taskService.CompleteTask(ctx, taskID, res); err != nil {
		if errors.Is(err, state.ErrTaskMissingMessage) {
			return protocol.TaskCompleteResult{}, &protocol.ProtocolError{Code: protocol.CodeInvalidState, Message: "task has no backing message envelope"}
		}
		return protocol.TaskCompleteResult{}, err
	}
	updated, err := s.taskService.GetTask(ctx, taskID)
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
		if scope.teamID == "" && s.taskService != nil {
			tasks, err := s.taskService.ListTasks(ctx, state.TaskFilter{
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

func (s *RPCServer) sessionGetTotals(ctx context.Context, p protocol.SessionGetTotalsParams) (protocol.SessionGetTotalsResult, error) {
	return newSessionQueryService(s).sessionGetTotals(ctx, p)
}

func (s *RPCServer) activityList(ctx context.Context, p protocol.ActivityListParams) (protocol.ActivityListResult, error) {
	return newSessionQueryService(s).activityList(ctx, p)
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
	if s.taskService == nil {
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
		Metadata: map[string]any{
			"source":        "rpc.turn.create",
			"messageKind":   types.MessageKindUserInput,
			"intentId":      "turn.create:" + taskID,
			"correlationId": taskID,
			"producer":      "rpc.turn.create",
		},
	}
	if err := s.taskService.CreateTask(ctx, task); err != nil {
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
	if s.taskService == nil {
		return protocol.TurnCancelResult{}, &protocol.ProtocolError{Code: protocol.CodeInternalError, Message: "task store not configured"}
	}
	task, err := s.taskService.GetTask(ctx, turnID)
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
		if err := s.taskService.CompleteTask(ctx, task.TaskID, tr); err != nil {
			if errors.Is(err, state.ErrTaskMissingMessage) {
				return protocol.TurnCancelResult{}, &protocol.ProtocolError{Code: protocol.CodeInvalidState, Message: "turn has no backing message envelope"}
			}
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
