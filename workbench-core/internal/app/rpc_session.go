package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tinoosan/workbench-core/pkg/agent/state"
	"github.com/tinoosan/workbench-core/pkg/fsutil"
	"github.com/tinoosan/workbench-core/pkg/profile"
	"github.com/tinoosan/workbench-core/pkg/protocol"
	pkgagent "github.com/tinoosan/workbench-core/pkg/services/agent"
	pkgsession "github.com/tinoosan/workbench-core/pkg/services/session"
	"github.com/tinoosan/workbench-core/pkg/services/team"
	pkgstore "github.com/tinoosan/workbench-core/pkg/store"
	"github.com/tinoosan/workbench-core/pkg/timeutil"
	"github.com/tinoosan/workbench-core/pkg/types"
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
			return addBoundHandler[protocol.SessionDeleteParams, protocol.SessionDeleteResult](reg, protocol.MethodSessionDelete, false, s.sessionDelete)
		},
		func() error {
			return addBoundHandler[protocol.SessionGetTotalsParams, protocol.SessionGetTotalsResult](reg, protocol.MethodSessionGetTotals, false, s.sessionGetTotals)
		},
		func() error {
			return addBoundHandler[protocol.ActivityListParams, protocol.ActivityListResult](reg, protocol.MethodActivityList, false, s.activityList)
		},
		func() error {
			return addBoundHandler[protocol.RunListChildrenParams, protocol.RunListChildrenResult](reg, protocol.MethodRunListChildren, false, s.runListChildren)
		},
	)
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
	sess, run, err := s.session.Start(ctx, pkgsession.StartOptions{Goal: goal, MaxBytesForContext: maxContext})
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
	if activeModel == "" && strings.TrimSpace(p.Profile) != "" {
		if prof, _, err := resolveProfileRef(s.cfg, strings.TrimSpace(p.Profile)); err == nil && prof != nil {
			if m := strings.TrimSpace(prof.Model); m != "" {
				activeModel = m
			}
		}
	}
	if activeModel != "" {
		sess.ActiveModel = activeModel
	}
	ensureSessionReasoningForModel(&sess, sess.ActiveModel, "", "")
	if err := s.session.SaveSession(ctx, sess); err != nil {
		return protocol.SessionStartResult{}, err
	}
	if strings.TrimSpace(p.Profile) != "" || activeModel != "" {
		if created, err := s.session.LoadRun(ctx, strings.TrimSpace(run.RunID)); err == nil {
			if created.Runtime == nil {
				created.Runtime = &types.RunRuntimeConfig{}
			}
			if profileRef := strings.TrimSpace(p.Profile); profileRef != "" {
				created.Runtime.Profile = profileRef
			}
			if activeModel != "" {
				created.Runtime.Model = activeModel
			}
			_ = s.session.SaveRun(ctx, created)
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
	_, coordinatorRole, err := team.ValidateTeamRoles(prof.Team.Roles)
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
	roles := make([]team.RoleRecord, 0, len(runtimes))
	for _, rt := range runtimes {
		roles = append(roles, team.RoleRecord{
			RoleName:  strings.TrimSpace(rt.role.Name),
			RunID:     strings.TrimSpace(rt.run.RunID),
			SessionID: strings.TrimSpace(rt.run.SessionID),
		})
	}
	manifest := team.BuildManifest(teamID, strings.TrimSpace(prof.ID), coordinatorRole, primaryRunID, teamModel, roles, time.Now().UTC().Format(time.RFC3339Nano))
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
	filter := pkgstore.SessionFilter{
		TitleContains: strings.TrimSpace(p.TitleContains),
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
	affected, err := s.setSessionPausedState(ctx, threadID, sessionID, true)
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
	return protocol.SessionDeleteResult{}, nil
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
	if err := s.taskService.CreateTask(ctx, task); err != nil {
		return protocol.TaskCreateResult{}, err
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
				out.TotalTokens = sess.TotalTokens
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
	tasks, err := s.taskService.ListTasks(ctx, state.TaskFilter{
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
		rs, err := s.taskService.GetRunStats(ctx, runID)
		if err != nil {
			continue
		}
		out.TotalTokens += rs.TotalTokens
		out.TotalCostUSD += rs.TotalCost
		if rs.TotalTokens > 0 && rs.TotalCost <= 0 && !pricingKnownForRun(ctx, s.session, runID) {
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
		runID := strings.TrimSpace(scope.runID)
		if !p.IncludeChildRuns || runID == "" {
			acts, err := s.session.ListActivities(ctx, runID, limit, offset)
			if err != nil {
				return protocol.ActivityListResult{}, err
			}
			total, _ := s.session.CountActivities(ctx, runID)
			next := 0
			if offset+len(acts) < total {
				next = offset + len(acts)
			}
			return protocol.ActivityListResult{Activities: acts, TotalCount: total, NextOffset: next}, nil
		}
		// Include activities from child runs (sub-agents) and prefix with "[Sub-agent N]".
		merged := make([]types.Activity, 0, 256)
		parentActs, err := s.session.ListActivities(ctx, runID, 500, 0)
		if err == nil {
			merged = append(merged, parentActs...)
		}
		children, err := s.session.ListChildRuns(ctx, runID)
		if err == nil {
			for _, child := range children {
				n := child.SpawnIndex
				if n <= 0 {
					n = 1
				}
				prefix := fmt.Sprintf("[Sub-agent %d] ", n)
				childActs, err := s.session.ListActivities(ctx, child.RunID, 300, 0)
				if err != nil {
					continue
				}
				for i := range childActs {
					childActs[i].Title = prefix + strings.TrimSpace(childActs[i].Title)
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

	runRole := map[string]string{}
	runSet := map[string]struct{}{}
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
