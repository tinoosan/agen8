package app

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	implstore "github.com/tinoosan/agen8/internal/store"
	"github.com/tinoosan/agen8/pkg/agent/state"
	"github.com/tinoosan/agen8/pkg/profile"
	"github.com/tinoosan/agen8/pkg/protocol"
	pkgagent "github.com/tinoosan/agen8/pkg/services/agent"
	"github.com/tinoosan/agen8/pkg/services/team"
	pkgstore "github.com/tinoosan/agen8/pkg/store"
	"github.com/tinoosan/agen8/pkg/timeutil"
	"github.com/tinoosan/agen8/pkg/types"
)

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

func metadataValueString(metadata map[string]any, key string) string {
	if len(metadata) == 0 {
		return ""
	}
	raw, ok := metadata[key]
	if !ok || raw == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(raw))
}

func protocolTaskFromTypesTask(t types.Task) protocol.Task {
	return protocol.Task{
		ID:               strings.TrimSpace(t.TaskID),
		ThreadID:         protocol.ThreadID(strings.TrimSpace(t.SessionID)),
		RunID:            protocol.RunID(strings.TrimSpace(t.RunID)),
		TeamID:           strings.TrimSpace(t.TeamID),
		TaskKind:         strings.TrimSpace(t.TaskKind),
		HarnessID:        metadataValueString(t.Metadata, "harnessId"),
		HarnessRunRef:    metadataValueString(t.Metadata, "harnessRunRef"),
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
		filter.Status = []types.TaskStatus{types.TaskStatusSucceeded, types.TaskStatusFailed, types.TaskStatusCanceled}
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
	if _, err := s.resolveThreadID(p.ThreadID); err != nil {
		return protocol.SessionStartResult{}, err
	}
	requestedMode := strings.ToLower(strings.TrimSpace(p.Mode))
	if requestedMode != "" && requestedMode != "single-agent" && requestedMode != "multi-agent" {
		return protocol.SessionStartResult{}, &protocol.ProtocolError{
			Code:    protocol.CodeInvalidParams,
			Message: "mode must be single-agent or multi-agent",
		}
	}
	profileRef := strings.TrimSpace(p.Profile)
	if profileRef == "" {
		profileRef = "general"
	}
	prof, _, err := resolveProfileRef(s.cfg, profileRef)
	if err != nil {
		return protocol.SessionStartResult{}, &protocol.ProtocolError{Code: protocol.CodeInvalidParams, Message: "load profile: " + err.Error()}
	}
	if prof == nil {
		return protocol.SessionStartResult{}, &protocol.ProtocolError{Code: protocol.CodeInvalidParams, Message: "profile not found"}
	}
	roles, err := prof.RolesForSession()
	if err != nil {
		return protocol.SessionStartResult{}, &protocol.ProtocolError{Code: protocol.CodeInvalidParams, Message: err.Error()}
	}
	_, coordinatorRole, err := team.ValidateTeamRoles(roles)
	if err != nil {
		return protocol.SessionStartResult{}, &protocol.ProtocolError{Code: protocol.CodeInvalidParams, Message: err.Error()}
	}
	teamRoles := append([]profile.RoleConfig(nil), roles...)
	reviewerCfg, reviewerEnabled := prof.ReviewerForSession()
	mode := "single-agent"
	if len(teamRoles) > 1 || reviewerEnabled {
		mode = "multi-agent"
	}
	if requestedMode != "" && requestedMode != mode {
		return protocol.SessionStartResult{}, &protocol.ProtocolError{
			Code:    protocol.CodeInvalidParams,
			Message: fmt.Sprintf("mode %q does not match profile role count (%s)", requestedMode, mode),
		}
	}

	goal := strings.TrimSpace(p.Goal)
	maxContext := s.run.MaxBytesForContext
	if maxContext <= 0 {
		maxContext = 8 * 1024
	}
	sess := types.NewSession(goal)
	sess.CurrentGoal = goal
	sess.Profile = strings.TrimSpace(prof.ID)
	sess.ProjectRoot = strings.TrimSpace(p.ProjectRoot)
	teamID := "team-" + uuid.NewString()
	sess.TeamID = teamID
	sess.Mode = mode

	teamModel := strings.TrimSpace(p.Model)
	if teamModel == "" {
		teamModel = prof.TeamModelForSession()
	}
	if teamModel == "" {
		teamModel = strings.TrimSpace(prof.Model)
	}
	if teamModel != "" {
		sess.ActiveModel = teamModel
	}
	ensureSessionReasoningForModel(&sess, sess.ActiveModel, "", "")
	if err := s.session.SaveSession(ctx, sess); err != nil {
		return protocol.SessionStartResult{}, err
	}

	runIDs := make([]string, 0, len(teamRoles))
	manifestRoles := make([]team.RoleRecord, 0, len(teamRoles)+1)
	primaryRunID := ""
	for _, role := range teamRoles {
		roleName := strings.TrimSpace(role.Name)
		if roleName == "" {
			continue
		}
		roleGoal := strings.TrimSpace(role.Description)
		if roleGoal == "" {
			roleGoal = goal
		}
		run := types.NewRun(roleGoal, maxContext, strings.TrimSpace(sess.SessionID))
		roleModel := resolveRoleModel(role, teamModel)
		if roleModel == "" {
			return protocol.SessionStartResult{}, &protocol.ProtocolError{Code: protocol.CodeInvalidParams, Message: "no model resolved for role " + roleName}
		}
		run.Runtime = &types.RunRuntimeConfig{
			Profile: strings.TrimSpace(prof.ID),
			Model:   roleModel,
			TeamID:  strings.TrimSpace(teamID),
			Role:    roleName,
		}
		if err := s.session.SaveRun(ctx, run); err != nil {
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
		runIDs = append(runIDs, runID)
		manifestRoles = append(manifestRoles, team.RoleRecord{
			RoleName:  roleName,
			RunID:     runID,
			SessionID: strings.TrimSpace(sess.SessionID),
		})
		if strings.EqualFold(roleName, coordinatorRole) && primaryRunID == "" {
			primaryRunID = runID
		}
	}
	if reviewerEnabled && reviewerCfg != nil {
		reviewerGoal := strings.TrimSpace(reviewerCfg.Description)
		if reviewerGoal == "" {
			reviewerGoal = goal
		}
		reviewerRun := types.NewRun(reviewerGoal, maxContext, strings.TrimSpace(sess.SessionID))
		reviewerModel := strings.TrimSpace(reviewerCfg.Model)
		if reviewerModel == "" {
			reviewerModel = teamModel
		}
		if reviewerModel == "" {
			return protocol.SessionStartResult{}, &protocol.ProtocolError{Code: protocol.CodeInvalidParams, Message: "no model resolved for reviewer"}
		}
		reviewerName := strings.TrimSpace(reviewerCfg.EffectiveName())
		reviewerRun.Runtime = &types.RunRuntimeConfig{
			Profile: strings.TrimSpace(prof.ID),
			Model:   reviewerModel,
			TeamID:  strings.TrimSpace(teamID),
			Role:    reviewerName,
		}
		if err := s.session.SaveRun(ctx, reviewerRun); err != nil {
			return protocol.SessionStartResult{}, err
		}
		reviewerRunID := strings.TrimSpace(reviewerRun.RunID)
		sess.Runs = append(sess.Runs, reviewerRunID)
		runIDs = append(runIDs, reviewerRunID)
		manifestRoles = append(manifestRoles, team.RoleRecord{
			RoleName:  reviewerName,
			RunID:     reviewerRunID,
			SessionID: strings.TrimSpace(sess.SessionID),
		})
	}
	if primaryRunID == "" && len(runIDs) > 0 {
		primaryRunID = runIDs[0]
	}
	if primaryRunID == "" {
		return protocol.SessionStartResult{}, &protocol.ProtocolError{Code: protocol.CodeInvalidState, Message: "profile produced no runs"}
	}
	sess.CurrentRunID = primaryRunID
	if err := s.session.SaveSession(ctx, sess); err != nil {
		return protocol.SessionStartResult{}, err
	}

	if err := s.workspacePreparer.PrepareTeamWorkspace(ctx, teamID); err != nil {
		return protocol.SessionStartResult{}, err
	}
	manifest := team.BuildManifest(teamID, strings.TrimSpace(prof.ID), coordinatorRole, primaryRunID, teamModel, manifestRoles, time.Now().UTC().Format(time.RFC3339Nano))
	if err := s.manifestStore.Save(ctx, manifest); err != nil {
		return protocol.SessionStartResult{}, err
	}
	if goal != "" {
		if err := team.SeedCoordinatorTask(ctx, s.taskService, strings.TrimSpace(sess.SessionID), primaryRunID, teamID, coordinatorRole, goal); err != nil {
			return protocol.SessionStartResult{}, err
		}
	}
	return protocol.SessionStartResult{
		SessionID:    strings.TrimSpace(sess.SessionID),
		PrimaryRunID: primaryRunID,
		Mode:         sess.Mode,
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
	filter := pkgstore.SessionFilter{
		TitleContains: strings.TrimSpace(p.TitleContains),
		ProjectRoot:   strings.TrimSpace(p.ProjectRoot),
		Limit:         clampLimit(p.Limit, 50, 500),
		Offset:        max(0, p.Offset),
		SortBy:        "updated_at",
		SortDesc:      true,
	}
	total, err := s.session.CountSessions(ctx, filter)
	if err != nil {
		return protocol.SessionListResult{}, err
	}
	sessions, err := s.session.ListSessionsPaginated(ctx, filter)
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
			if run, rerr := s.session.LoadRun(ctx, runID); rerr == nil && run.Runtime != nil {
				if profileID == "" {
					profileID = strings.TrimSpace(run.Runtime.Profile)
				}
				if teamID == "" {
					teamID = strings.TrimSpace(run.Runtime.TeamID)
				}
			}
			if s.agentService != nil {
				if _, inferredTeamID := s.agentService.InferRunRoleAndTeam(ctx, runID); teamID == "" && inferredTeamID != "" {
					teamID = strings.TrimSpace(inferredTeamID)
				}
			}
		}
		if mode == "" {
			if teamID != "" {
				mode = "multi-agent"
			} else {
				mode = "single-agent"
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
			run, rerr := s.session.LoadRun(ctx, listedRunID)
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
			ProjectRoot:   strings.TrimSpace(sess.ProjectRoot),
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
	// We call Delete on the service, which handles stopping runs and cleaning up storage.
	if err := s.session.Delete(ctx, sessionID); err != nil {
		return protocol.SessionDeleteResult{}, err
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
		if lerr == nil {
			loaded.Status = types.RunStatusCanceled
			now := time.Now().UTC()
			loaded.FinishedAt = &now
			loaded.Error = nil
			_ = s.session.SaveRun(ctx, loaded)
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
	if harnessID := strings.ToLower(strings.TrimSpace(p.HarnessID)); harnessID != "" {
		task.Metadata["harnessId"] = harnessID
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
	_ = s.taskService.UpdateTask(ctx, task)
	task, _ = s.taskService.GetTask(ctx, taskID)
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
	scope, err := s.resolveTeamOrRunScope(ctx, p.ThreadID, p.TeamID, p.RunID)
	if err != nil {
		return protocol.SessionGetTotalsResult{}, err
	}
	if s.taskService == nil {
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
				out.TotalTokens = out.TotalTokensIn + out.TotalTokensOut
				if out.TotalTokens == 0 {
					out.TotalTokens = sess.TotalTokens
				}
				out.TotalCostUSD = sess.CostUSD
				out.PricingKnown = sess.TotalTokens == 0 || sess.CostUSD > 0 || pricingKnownForRun(ctx, s.session, strings.TrimSpace(scope.runID))
			}
		}
		stats, err := s.taskService.GetRunStats(ctx, strings.TrimSpace(scope.runID))
		if err == nil {
			out.TasksDone = stats.Succeeded + stats.Failed
		}
		return out, nil
	}

	runIDSet := map[string]struct{}{}
	manifestRunIDs, _ := s.loadTeamManifestRunRoles(ctx, strings.TrimSpace(scope.teamID))
	for _, runID := range manifestRunIDs {
		runID = strings.TrimSpace(runID)
		if runID == "" {
			continue
		}
		runIDSet[runID] = struct{}{}
	}
	if len(runIDSet) == 0 {
		tasks, err := s.taskService.ListTasks(ctx, state.TaskFilter{
			TeamID:   strings.TrimSpace(scope.teamID),
			Limit:    1000,
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
		}
	}
	tasks, err := s.taskService.ListTasks(ctx, state.TaskFilter{
		TeamID:   strings.TrimSpace(scope.teamID),
		Limit:    2000,
		SortBy:   "created_at",
		SortDesc: true,
	})
	if err != nil {
		return protocol.SessionGetTotalsResult{}, err
	}
	for _, t := range tasks {
		if t.Status == types.TaskStatusSucceeded || t.Status == types.TaskStatusFailed || t.Status == types.TaskStatusCanceled {
			if len(runIDSet) == 0 {
				out.TasksDone++
				continue
			}
			if _, ok := runIDSet[strings.TrimSpace(t.RunID)]; ok {
				out.TasksDone++
			}
		}
	}
	statsTotalTokens := 0
	for runID := range runIDSet {
		if s.session != nil {
			if run, err := s.session.LoadRun(ctx, runID); err == nil {
				if sessionID := strings.TrimSpace(run.SessionID); sessionID != "" {
					if sess, serr := s.session.LoadSession(ctx, sessionID); serr == nil {
						out.TotalTokensIn += sess.InputTokens
						out.TotalTokensOut += sess.OutputTokens
					}
				}
			}
		}
		rs, err := s.taskService.GetRunStats(ctx, runID)
		if err != nil {
			continue
		}
		statsTotalTokens += rs.TotalTokens
		out.TotalCostUSD += rs.TotalCost
		if rs.TotalTokens > 0 && rs.TotalCost <= 0 && !pricingKnownForRun(ctx, s.session, runID) {
			out.PricingKnown = false
		}
	}
	out.TotalTokens = out.TotalTokensIn + out.TotalTokensOut
	if out.TotalTokens == 0 {
		out.TotalTokens = statsTotalTokens
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
		runID := strings.TrimSpace(scope.runID)
		if !p.IncludeChildRuns || runID == "" {
			acts, err := s.session.ListActivities(ctx, runID, limit, offset)
			if err != nil {
				return protocol.ActivityListResult{}, err
			}
			// Populate Data["role"] with the profile name for standalone parent runs.
			parentRole := "agent"
			if s.run.Runtime != nil && strings.TrimSpace(s.run.Runtime.Profile) != "" {
				parentRole = strings.TrimSpace(s.run.Runtime.Profile)
			}
			for i := range acts {
				if acts[i].Data == nil {
					acts[i].Data = map[string]string{}
				}
				if strings.TrimSpace(acts[i].Data["role"]) == "" {
					acts[i].Data["role"] = parentRole
				}
			}
			total, _ := s.session.CountActivities(ctx, runID)
			next := 0
			if offset+len(acts) < total {
				next = offset + len(acts)
			}
			return protocol.ActivityListResult{Activities: acts, TotalCount: total, NextOffset: next}, nil
		}
		// Include activities from child runs (sub-agents) with Data["role"] set.
		merged := make([]types.Activity, 0, 256)
		parentActs, err := s.session.ListActivities(ctx, runID, 500, 0)
		if err == nil {
			// Populate Data["role"] with the profile name for standalone parent runs.
			parentRole := "agent"
			if s.run.Runtime != nil && strings.TrimSpace(s.run.Runtime.Profile) != "" {
				parentRole = strings.TrimSpace(s.run.Runtime.Profile)
			}
			for i := range parentActs {
				if parentActs[i].Data == nil {
					parentActs[i].Data = map[string]string{}
				}
				if strings.TrimSpace(parentActs[i].Data["role"]) == "" {
					parentActs[i].Data["role"] = parentRole
				}
			}
			merged = append(merged, parentActs...)
		}
		children, err := s.session.ListChildRuns(ctx, runID)
		if err == nil {
			for _, child := range children {
				n := child.SpawnIndex
				if n <= 0 {
					n = 1
				}
				childRole := fmt.Sprintf("Sub-agent %d", n)
				childActs, err := s.session.ListActivities(ctx, child.RunID, 300, 0)
				if err != nil {
					continue
				}
				for i := range childActs {
					if childActs[i].Data == nil {
						childActs[i].Data = map[string]string{}
					}
					childActs[i].Data["role"] = childRole
					merged = append(merged, childActs[i])
				}
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

	manifestRunIDs, runRole := s.loadTeamManifestRunRoles(ctx, strings.TrimSpace(scope.teamID))
	runSet := map[string]struct{}{}
	for _, runID := range manifestRunIDs {
		runID = strings.TrimSpace(runID)
		if runID == "" {
			continue
		}
		runSet[runID] = struct{}{}
	}
	if len(runSet) == 0 {
		tasks, err := s.taskService.ListTasks(ctx, state.TaskFilter{
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
	}
	if targetRunID := strings.TrimSpace(p.RunID); targetRunID != "" {
		if _, ok := runSet[targetRunID]; ok {
			runSet = map[string]struct{}{targetRunID: {}}
		} else {
			runSet = map[string]struct{}{}
		}
	}
	merged := make([]types.Activity, 0, 512)
	for runID := range runSet {
		acts, err := s.session.ListActivities(ctx, runID, 300, 0)
		if err != nil {
			continue
		}
		role := strings.TrimSpace(runRole[runID])
		if roleFilter := strings.TrimSpace(p.Role); roleFilter != "" && !strings.EqualFold(roleFilter, role) {
			continue
		}
		for i := range acts {
			if role != "" {
				if acts[i].Data == nil {
					acts[i].Data = map[string]string{}
				}
				acts[i].Data["role"] = role
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
