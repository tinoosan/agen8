package team

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/tinoosan/agen8/pkg/agent/hosttools"
	"github.com/tinoosan/agen8/pkg/agent/state"
	"github.com/tinoosan/agen8/pkg/profile"
	"github.com/tinoosan/agen8/pkg/services/session"
	"github.com/tinoosan/agen8/pkg/store"
	"github.com/tinoosan/agen8/pkg/types"
)

func TestValidateTeamRoles(t *testing.T) {
	t.Run("valid roles", func(t *testing.T) {
		roles := []profile.RoleConfig{
			{Name: "ceo", Coordinator: true},
			{Name: "writer", Coordinator: false},
		}
		names, coord, err := ValidateTeamRoles(roles)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(names) != 2 || names[0] != "ceo" || names[1] != "writer" {
			t.Fatalf("names = %v", names)
		}
		if coord != "ceo" {
			t.Fatalf("coordinatorRole = %q", coord)
		}
	})

	t.Run("missing coordinator", func(t *testing.T) {
		roles := []profile.RoleConfig{
			{Name: "a", Coordinator: false},
			{Name: "b", Coordinator: false},
		}
		_, _, err := ValidateTeamRoles(roles)
		if err == nil {
			t.Fatal("expected error")
		}
		if !errors.Is(err, ErrMissingCoordinator) {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("duplicate role name", func(t *testing.T) {
		roles := []profile.RoleConfig{
			{Name: "ceo", Coordinator: true},
			{Name: "ceo", Coordinator: false},
		}
		_, _, err := ValidateTeamRoles(roles)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("empty role name", func(t *testing.T) {
		roles := []profile.RoleConfig{
			{Name: "ceo", Coordinator: true},
			{Name: " ", Coordinator: false},
		}
		_, _, err := ValidateTeamRoles(roles)
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestResolveReviewerRole(t *testing.T) {
	p := &profile.Profile{
		Team: &profile.TeamConfig{
			Reviewer: &profile.ReviewerConfig{
				Enabled: true,
				Name:    "quality-reviewer",
			},
		},
	}
	if got := ResolveReviewerRole(p, "ceo"); got != "quality-reviewer" {
		t.Fatalf("ResolveReviewerRole() = %q", got)
	}
	if got := ResolveReviewerRole(&profile.Profile{}, "ceo"); got != "ceo" {
		t.Fatalf("fallback reviewer = %q", got)
	}
}

func TestBuildManifest(t *testing.T) {
	roles := []RoleRecord{
		{RoleName: "ceo", RunID: "run-1", SessionID: "sess-1"},
		{RoleName: "writer", RunID: "run-2", SessionID: "sess-2"},
	}
	m := BuildManifest("team-1", "profile-1", "ceo", "run-1", "gpt-5", roles, "2025-01-01T00:00:00Z")
	if m.TeamID != "team-1" || m.ProfileID != "profile-1" || m.CoordinatorRole != "ceo" || m.CoordinatorRun != "run-1" || m.TeamModel != "gpt-5" {
		t.Fatalf("manifest = %+v", m)
	}
	if len(m.Roles) != 2 || m.Roles[0].RoleName != "ceo" || m.Roles[1].RoleName != "writer" {
		t.Fatalf("roles = %+v", m.Roles)
	}
}

func TestSeedCoordinatorTask(t *testing.T) {
	var created types.Task
	store := &mockTaskStore{
		createTask: func(ctx context.Context, task types.Task) error {
			created = task
			return nil
		},
	}
	err := SeedCoordinatorTask(context.Background(), store, "sess-1", "run-1", "team-1", "ceo", "ship the feature")
	if err != nil {
		t.Fatalf("SeedCoordinatorTask: %v", err)
	}
	if created.SessionID != "sess-1" || created.RunID != "run-1" || created.TeamID != "team-1" || created.AssignedRole != "ceo" {
		t.Fatalf("task = %+v", created)
	}
	if created.Goal != "ship the feature" {
		t.Fatalf("goal = %q", created.Goal)
	}
	if created.Status != types.TaskStatusPending {
		t.Fatalf("status = %q", created.Status)
	}
	if created.Metadata == nil || created.Metadata["source"] != "team.goal" {
		t.Fatalf("metadata = %+v", created.Metadata)
	}
}

func TestEscalateToCoordinator(t *testing.T) {
	var createCalled bool
	var stopRunCalled, sessionStopCalled bool
	taskCreator := &mockRetryEscalationCreator{
		createEscalation: func(ctx context.Context, callbackTaskID string, data hosttools.EscalationData) error {
			createCalled = true
			return nil
		},
	}
	sessionSvc := &mockSessionService{
		stopRun: func(ctx context.Context, runID, status, msg string) (types.Run, error) {
			if runID == "child-run-1" && status == types.RunStatusFailed && msg == "escalated" {
				sessionStopCalled = true
			}
			return types.Run{}, nil
		},
	}
	runStopper := &mockRunStopper{
		stopRun: func(ctx context.Context, runID string) error {
			if runID == "child-run-1" {
				stopRunCalled = true
			}
			return nil
		},
	}
	err := EscalateToCoordinator(
		context.Background(),
		taskCreator,
		sessionSvc,
		runStopper,
		"callback-task-1",
		hosttools.EscalationData{SourceRunID: "child-run-1"},
	)
	if err != nil {
		t.Fatalf("EscalateToCoordinator: %v", err)
	}
	if !createCalled {
		t.Fatal("CreateEscalationTask was not called")
	}
	if !stopRunCalled {
		t.Fatal("StopRun was not called")
	}
	if !sessionStopCalled {
		t.Fatal("Session StopRun was not called")
	}
}

func TestEscalateToCoordinator_ReturnsStopFailures(t *testing.T) {
	stopLoopErr := errors.New("loop stop failed")
	stopPersistErr := errors.New("persist stop failed")

	taskCreator := &mockRetryEscalationCreator{
		createEscalation: func(ctx context.Context, callbackTaskID string, data hosttools.EscalationData) error {
			return nil
		},
	}
	runStopper := &mockRunStopper{
		stopRun: func(ctx context.Context, runID string) error {
			return stopLoopErr
		},
	}
	sessionSvc := &mockSessionService{
		stopRun: func(ctx context.Context, runID, status, msg string) (types.Run, error) {
			return types.Run{}, stopPersistErr
		},
	}

	err := EscalateToCoordinator(
		context.Background(),
		taskCreator,
		sessionSvc,
		runStopper,
		"callback-task-1",
		hosttools.EscalationData{SourceRunID: "child-run-1"},
	)
	if err == nil {
		t.Fatal("expected escalation stop error")
	}
	if !errors.Is(err, stopLoopErr) {
		t.Fatalf("expected run stopper error, got: %v", err)
	}
	if !errors.Is(err, stopPersistErr) {
		t.Fatalf("expected session stop error, got: %v", err)
	}
}

func TestIsTeamIdle(t *testing.T) {
	store := &mockTaskStore{
		countTasks: func(ctx context.Context, filter state.TaskFilter) (int, error) {
			if filter.TaskKind == state.TaskKindHeartbeat {
				return 0, nil
			}
			return 0, nil
		},
	}
	ok := IsTeamIdle(context.Background(), store, "team-1")
	if !ok {
		t.Fatal("expected idle")
	}

	store.countTasks = func(ctx context.Context, filter state.TaskFilter) (int, error) {
		if filter.TaskKind == state.TaskKindHeartbeat {
			return 0, nil
		}
		return 2, nil
	}
	ok = IsTeamIdle(context.Background(), store, "team-1")
	if ok {
		t.Fatal("expected not idle")
	}
}

func TestRequestModelChange(t *testing.T) {
	var saved Manifest
	store := &mockManifestStore{
		save: func(ctx context.Context, m Manifest) error {
			saved = m
			return nil
		},
		load: func(ctx context.Context, teamID string) (*Manifest, error) {
			return &Manifest{TeamID: teamID}, nil
		},
	}
	stateMgr := NewStateManager(store, Manifest{TeamID: "team-1"})
	taskStore := &mockTaskStore{countTasks: func(ctx context.Context, filter state.TaskFilter) (int, error) {
		return 0, nil
	}}
	applier := &mockModelApplier{
		apply: func(ctx context.Context, model, target string) ([]string, error) {
			return []string{"run-1"}, nil
		},
	}
	applied, err := RequestModelChange(context.Background(), taskStore, stateMgr, applier, "gpt-5", "", "rpc")
	if err != nil {
		t.Fatalf("RequestModelChange: %v", err)
	}
	if len(applied) != 1 || applied[0] != "run-1" {
		t.Fatalf("applied = %v", applied)
	}
	if saved.TeamModel != "gpt-5" {
		t.Fatalf("saved.TeamModel = %q", saved.TeamModel)
	}
}

// Mocks

type mockTaskStore struct {
	createTask   func(ctx context.Context, task types.Task) error
	countTasks   func(ctx context.Context, filter state.TaskFilter) (int, error)
	listTasks    func(ctx context.Context, filter state.TaskFilter) ([]types.Task, error)
	getTask      func(ctx context.Context, taskID string) (types.Task, error)
	completeTask func(ctx context.Context, taskID string, result types.TaskResult) error
}

func (m *mockTaskStore) CreateTask(ctx context.Context, task types.Task) error {
	if m.createTask != nil {
		return m.createTask(ctx, task)
	}
	return nil
}
func (m *mockTaskStore) CountTasks(ctx context.Context, filter state.TaskFilter) (int, error) {
	if m.countTasks != nil {
		return m.countTasks(ctx, filter)
	}
	return 0, nil
}
func (m *mockTaskStore) GetTask(ctx context.Context, taskID string) (types.Task, error) {
	if m.getTask != nil {
		return m.getTask(ctx, taskID)
	}
	return types.Task{}, state.ErrTaskNotFound
}
func (m *mockTaskStore) ListTasks(ctx context.Context, filter state.TaskFilter) ([]types.Task, error) {
	if m.listTasks != nil {
		return m.listTasks(ctx, filter)
	}
	return nil, nil
}
func (m *mockTaskStore) GetRunStats(ctx context.Context, runID string) (state.RunStats, error) {
	return state.RunStats{}, nil
}
func (m *mockTaskStore) DeleteTask(ctx context.Context, taskID string) error   { return nil }
func (m *mockTaskStore) UpdateTask(ctx context.Context, task types.Task) error { return nil }
func (m *mockTaskStore) CompleteTask(ctx context.Context, taskID string, result types.TaskResult) error {
	if m.completeTask != nil {
		return m.completeTask(ctx, taskID, result)
	}
	return nil
}
func (m *mockTaskStore) ClaimTask(ctx context.Context, taskID string, ttl time.Duration) error {
	return nil
}
func (m *mockTaskStore) ExtendLease(ctx context.Context, taskID string, ttl time.Duration) error {
	return nil
}
func (m *mockTaskStore) ReleaseLease(ctx context.Context, taskID string) error { return nil }
func (m *mockTaskStore) DelegateTask(ctx context.Context, taskID string) error { return nil }
func (m *mockTaskStore) ResumeTask(ctx context.Context, taskID string) error   { return nil }
func (m *mockTaskStore) RecoverExpiredLeases(ctx context.Context) error        { return nil }

type mockRetryEscalationCreator struct {
	createEscalation func(ctx context.Context, callbackTaskID string, data hosttools.EscalationData) error
}

func (m *mockRetryEscalationCreator) CreateRetryTask(ctx context.Context, childRunID, feedback string) error {
	return nil
}
func (m *mockRetryEscalationCreator) CreateEscalationTask(ctx context.Context, callbackTaskID string, data hosttools.EscalationData) error {
	if m.createEscalation != nil {
		return m.createEscalation(ctx, callbackTaskID, data)
	}
	return nil
}

type mockSessionService struct {
	stopRun     func(ctx context.Context, runID, status, msg string) (types.Run, error)
	loadSession func(ctx context.Context, sessionID string) (types.Session, error)
	saveSession func(ctx context.Context, s types.Session) error
}

func (m *mockSessionService) StopRun(ctx context.Context, runID, status, msg string) (types.Run, error) {
	if m.stopRun != nil {
		return m.stopRun(ctx, runID, status, msg)
	}
	return types.Run{}, nil
}

func (m *mockSessionService) LoadSession(ctx context.Context, sessionID string) (types.Session, error) {
	if m.loadSession != nil {
		return m.loadSession(ctx, sessionID)
	}
	return types.Session{}, nil
}
func (m *mockSessionService) SaveSession(ctx context.Context, s types.Session) error {
	if m.saveSession != nil {
		return m.saveSession(ctx, s)
	}
	return nil
}
func (m *mockSessionService) Start(ctx context.Context, opts session.StartOptions) (types.Session, types.Run, error) {
	return types.Session{}, types.Run{}, nil
}
func (m *mockSessionService) Delete(ctx context.Context, sessionID string) error { return nil }
func (m *mockSessionService) ListSessionsPaginated(ctx context.Context, filter store.SessionFilter) ([]types.Session, error) {
	return nil, nil
}
func (m *mockSessionService) CountSessions(ctx context.Context, filter store.SessionFilter) (int, error) {
	return 0, nil
}
func (m *mockSessionService) LoadRun(ctx context.Context, runID string) (types.Run, error) {
	return types.Run{}, nil
}
func (m *mockSessionService) SaveRun(ctx context.Context, run types.Run) error { return nil }
func (m *mockSessionService) ListRunsBySession(ctx context.Context, sessionID string) ([]types.Run, error) {
	return nil, nil
}
func (m *mockSessionService) ListRunsByStatus(ctx context.Context, statuses []string) ([]types.Run, error) {
	return nil, nil
}
func (m *mockSessionService) ListChildRuns(ctx context.Context, parentRunID string) ([]types.Run, error) {
	return nil, nil
}
func (m *mockSessionService) AddRunToSession(ctx context.Context, sessionID, runID string) (types.Session, error) {
	return types.Session{}, nil
}
func (m *mockSessionService) ListActivities(ctx context.Context, runID string, limit, offset int) ([]types.Activity, error) {
	return nil, nil
}
func (m *mockSessionService) CountActivities(ctx context.Context, runID string) (int, error) {
	return 0, nil
}

func (m *mockSessionService) LatestRun(ctx context.Context) (types.Run, error) {
	return types.Run{}, nil
}

func (m *mockSessionService) LatestRunningRun(ctx context.Context) (types.Run, error) {
	return types.Run{}, nil
}

type mockRunStopper struct {
	stopRun func(ctx context.Context, runID string) error
}

func (m *mockRunStopper) StopRun(ctx context.Context, runID string) error {
	if m.stopRun != nil {
		return m.stopRun(ctx, runID)
	}
	return nil
}

type mockManifestStore struct {
	load func(ctx context.Context, teamID string) (*Manifest, error)
	save func(ctx context.Context, manifest Manifest) error
}

func (m *mockManifestStore) Load(ctx context.Context, teamID string) (*Manifest, error) {
	if m.load != nil {
		return m.load(ctx, teamID)
	}
	return nil, nil
}
func (m *mockManifestStore) Save(ctx context.Context, manifest Manifest) error {
	if m.save != nil {
		return m.save(ctx, manifest)
	}
	return nil
}

type mockModelApplier struct {
	apply func(ctx context.Context, model, target string) ([]string, error)
}

func (m *mockModelApplier) ApplyModel(ctx context.Context, model, target string) ([]string, error) {
	if m.apply != nil {
		return m.apply(ctx, model, target)
	}
	return nil, nil
}
