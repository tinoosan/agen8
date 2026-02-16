package agent

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/tinoosan/workbench-core/pkg/agent/state"
	"github.com/tinoosan/workbench-core/pkg/types"
)

// Manager implements ServiceForRPC.
type Manager struct {
	sessions   SessionProvider
	tasks      TaskLister
	taskCancel ActiveTaskCanceler
	controller RuntimeController
}

// NewManager creates a new agent service manager.
func NewManager(sessions SessionProvider, tasks TaskLister, taskCancel ActiveTaskCanceler) *Manager {
	return &Manager{
		sessions:   sessions,
		tasks:      tasks,
		taskCancel: taskCancel,
	}
}

// SetRuntimeController sets the runtime controller (e.g. supervisor). Call after construction to break circular dependency.
func (m *Manager) SetRuntimeController(c RuntimeController) {
	m.controller = c
}

// List returns agents (runs) for the given session, sorted by StartedAt desc then RunID.
func (m *Manager) List(ctx context.Context, sessionID string) ([]AgentInfo, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, &ServiceError{Code: CodeInvalidParams, Message: "sessionId is required"}
	}
	sess, err := m.sessions.LoadSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	out := make([]AgentInfo, 0, len(sess.Runs))
	for _, runID := range sess.Runs {
		runID = strings.TrimSpace(runID)
		if runID == "" {
			continue
		}
		run, err := m.sessions.LoadRun(ctx, runID)
		if err != nil {
			continue
		}
		item := AgentInfo{
			RunID:       runID,
			SessionID:   strings.TrimSpace(run.SessionID),
			Status:      strings.TrimSpace(run.Status),
			Goal:        strings.TrimSpace(run.Goal),
			ParentRunID: strings.TrimSpace(run.ParentRunID),
			SpawnIndex:  run.SpawnIndex,
		}
		if run.Runtime != nil {
			item.Profile = strings.TrimSpace(run.Runtime.Profile)
		}
		if role, teamID := m.InferRunRoleAndTeam(ctx, runID); role != "" || teamID != "" {
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
		a, b := out[i].StartedAt, out[j].StartedAt
		if a == b {
			return out[i].RunID > out[j].RunID
		}
		return a > b
	})
	return out, nil
}

// Start creates a new run and updates the session.
func (m *Manager) Start(ctx context.Context, opts StartOptions) (StartResult, error) {
	sessionID := strings.TrimSpace(opts.SessionID)
	if sessionID == "" {
		return StartResult{}, &ServiceError{Code: CodeInvalidParams, Message: "sessionId is required"}
	}
	sess, err := m.sessions.LoadSession(ctx, sessionID)
	if err != nil {
		return StartResult{}, err
	}
	maxContext := opts.MaxBytesForContext
	if maxContext <= 0 {
		maxContext = 8 * 1024
	}
	goal := strings.TrimSpace(opts.Goal)
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
	if profileRef := strings.TrimSpace(opts.Profile); profileRef != "" {
		run.Runtime.Profile = profileRef
		sess.Profile = profileRef
	}
	if err := m.sessions.SaveRun(ctx, run); err != nil {
		return StartResult{}, err
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
	model := strings.TrimSpace(opts.Model)
	if model == "" {
		model = strings.TrimSpace(sess.ActiveModel)
	}
	if model != "" {
		if run.Runtime == nil {
			run.Runtime = &types.RunRuntimeConfig{}
		}
		run.Runtime.Model = model
		_ = m.sessions.SaveRun(ctx, run)
		sess.ActiveModel = model
	}
	if err := m.sessions.SaveSession(ctx, sess); err != nil {
		return StartResult{}, err
	}
	return StartResult{
		RunID:     runID,
		SessionID: sessionID,
		Profile:   strings.TrimSpace(opts.Profile),
		Model:     model,
	}, nil
}

// Pause pauses the run: delegates to RuntimeController if set, else updates run status and cancels active tasks.
func (m *Manager) Pause(ctx context.Context, runID, sessionID string) error {
	if strings.TrimSpace(runID) == "" {
		return &ServiceError{Code: CodeInvalidParams, Message: "runId is required"}
	}
	if m.controller != nil {
		return m.controller.PauseRun(runID)
	}
	return m.setRunPausedState(ctx, runID, sessionID, true)
}

// Resume resumes the run: delegates to RuntimeController if set, else updates run status.
func (m *Manager) Resume(ctx context.Context, runID, sessionID string) error {
	if strings.TrimSpace(runID) == "" {
		return &ServiceError{Code: CodeInvalidParams, Message: "runId is required"}
	}
	if m.controller != nil {
		return m.controller.ResumeRun(ctx, runID)
	}
	return m.setRunPausedState(ctx, runID, sessionID, false)
}

// InferRunRoleAndTeam returns role and teamID from run RuntimeConfig or from task metadata.
func (m *Manager) InferRunRoleAndTeam(ctx context.Context, runID string) (role, teamID string) {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return "", ""
	}
	if run, err := m.sessions.LoadRun(ctx, runID); err == nil && run.Runtime != nil {
		role = strings.TrimSpace(run.Runtime.Role)
		teamID = strings.TrimSpace(run.Runtime.TeamID)
		if role != "" && teamID != "" {
			return role, teamID
		}
	}
	if m.tasks == nil {
		return strings.TrimSpace(role), strings.TrimSpace(teamID)
	}
	tasks, err := m.tasks.ListTasks(ctx, state.TaskFilter{
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

// setRunPausedState updates run status to paused/running and validates scope. If paused, cancels active tasks.
func (m *Manager) setRunPausedState(ctx context.Context, runID, sessionID string, paused bool) error {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return &ServiceError{Code: CodeInvalidParams, Message: "runId is required"}
	}
	run, err := m.sessions.LoadRun(ctx, runID)
	if err != nil {
		return &ServiceError{Code: CodeItemNotFound, Message: "run not found"}
	}
	if strings.TrimSpace(run.SessionID) != strings.TrimSpace(sessionID) {
		return &ServiceError{Code: CodeThreadNotFound, Message: "thread not found"}
	}
	status := strings.ToLower(strings.TrimSpace(run.Status))
	switch status {
	case strings.ToLower(types.RunStatusRunning), strings.ToLower(types.RunStatusPaused):
		// supported
	default:
		return &ServiceError{Code: CodeInvalidState, Message: "run is not pauseable"}
	}
	if paused {
		run.Status = types.RunStatusPaused
	} else {
		run.Status = types.RunStatusRunning
	}
	run.FinishedAt = nil
	run.Error = nil
	if err := m.sessions.SaveRun(ctx, run); err != nil {
		return err
	}
	if paused {
		_, err := m.taskCancel.CancelActiveTasksByRun(ctx, runID, "run paused")
		return err
	}
	return nil
}
