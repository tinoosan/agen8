package agent

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/tinoosan/workbench-core/pkg/agent/state"
	"github.com/tinoosan/workbench-core/pkg/types"
)

type mockSessionProvider struct {
	sessions map[string]types.Session
	runs     map[string]types.Run
	loadErr  error
	saveErr  error
}

func (m *mockSessionProvider) LoadSession(ctx context.Context, sessionID string) (types.Session, error) {
	if m.loadErr != nil {
		return types.Session{}, m.loadErr
	}
	if s, ok := m.sessions[sessionID]; ok {
		return s, nil
	}
	return types.Session{}, errors.New("session not found")
}

func (m *mockSessionProvider) SaveSession(ctx context.Context, s types.Session) error {
	if m.saveErr != nil {
		return m.saveErr
	}
	if m.sessions == nil {
		m.sessions = make(map[string]types.Session)
	}
	m.sessions[s.SessionID] = s
	return nil
}

func (m *mockSessionProvider) LoadRun(ctx context.Context, runID string) (types.Run, error) {
	if m.loadErr != nil {
		return types.Run{}, m.loadErr
	}
	if r, ok := m.runs[runID]; ok {
		return r, nil
	}
	return types.Run{}, errors.New("run not found")
}

func (m *mockSessionProvider) SaveRun(ctx context.Context, run types.Run) error {
	if m.saveErr != nil {
		return m.saveErr
	}
	if m.runs == nil {
		m.runs = make(map[string]types.Run)
	}
	m.runs[run.RunID] = run
	return nil
}

type mockTaskLister struct {
	tasks []types.Task
	err   error
}

func (m *mockTaskLister) ListTasks(ctx context.Context, filter state.TaskFilter) ([]types.Task, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.tasks, nil
}

type mockActiveTaskCanceler struct {
	canceled []struct{ runID, reason string }
}

func (m *mockActiveTaskCanceler) CancelActiveTasksByRun(ctx context.Context, runID, reason string) (int, error) {
	if m.canceled == nil {
		m.canceled = make([]struct{ runID, reason string }, 0)
	}
	m.canceled = append(m.canceled, struct{ runID, reason string }{runID, reason})
	return 0, nil
}

type mockRuntimeController struct {
	paused    []string
	resumed   []string
	stopped   []string
	pauseErr  error
	resumeErr error
}

func (m *mockRuntimeController) PauseRun(runID string) error {
	if m.paused == nil {
		m.paused = make([]string, 0)
	}
	m.paused = append(m.paused, runID)
	return m.pauseErr
}

func (m *mockRuntimeController) ResumeRun(ctx context.Context, runID string) error {
	if m.resumed == nil {
		m.resumed = make([]string, 0)
	}
	m.resumed = append(m.resumed, runID)
	return m.resumeErr
}

func (m *mockRuntimeController) StopRun(runID string) error {
	if m.stopped == nil {
		m.stopped = make([]string, 0)
	}
	m.stopped = append(m.stopped, runID)
	return nil
}

func TestList_Success(t *testing.T) {
	now := time.Now().UTC()
	earlier := now.Add(-time.Hour)
	sessions := map[string]types.Session{
		"sess-1": {
			SessionID: "sess-1",
			Runs:      []string{"run-1", "run-2"},
		},
	}
	runs := map[string]types.Run{
		"run-1": {
			RunID: "run-1", SessionID: "sess-1", Goal: "goal1", Status: types.RunStatusRunning,
			StartedAt: &earlier,
		},
		"run-2": {
			RunID: "run-2", SessionID: "sess-1", Goal: "goal2", Status: types.RunStatusPaused,
			StartedAt: &now,
		},
	}
	prov := &mockSessionProvider{sessions: sessions, runs: runs}
	mgr := NewManager(prov, nil, nil)
	ctx := context.Background()
	list, err := mgr.List(ctx, "sess-1")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 agents, got %d", len(list))
	}
	// Sorted by StartedAt desc, so run-2 (now) before run-1 (earlier)
	if list[0].RunID != "run-2" || list[1].RunID != "run-1" {
		t.Errorf("expected order run-2, run-1; got %s, %s", list[0].RunID, list[1].RunID)
	}
	if list[0].Goal != "goal2" || list[0].Status != types.RunStatusPaused {
		t.Errorf("first item: goal=%q status=%q", list[0].Goal, list[0].Status)
	}
}

func TestList_SessionNotFound(t *testing.T) {
	prov := &mockSessionProvider{
		sessions: map[string]types.Session{},
		runs:     map[string]types.Run{},
		loadErr:  errors.New("session not found"),
	}
	mgr := NewManager(prov, nil, nil)
	ctx := context.Background()
	_, err := mgr.List(ctx, "sess-missing")
	if err == nil {
		t.Fatal("expected error when session not found")
	}
}

func TestStart_Success(t *testing.T) {
	sessions := map[string]types.Session{
		"sess-1": {
			SessionID: "sess-1",
			Runs:      []string{},
			TeamID:    "team-1",
		},
	}
	runs := map[string]types.Run{}
	prov := &mockSessionProvider{sessions: sessions, runs: runs}
	mgr := NewManager(prov, nil, nil)
	ctx := context.Background()
	res, err := mgr.Start(ctx, StartOptions{
		SessionID:          "sess-1",
		Goal:               "my goal",
		Profile:            "default",
		Model:              "gpt-4",
		MaxBytesForContext: 4096,
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if res.SessionID != "sess-1" || res.Profile != "default" || res.Model != "gpt-4" {
		t.Errorf("StartResult: session=%q profile=%q model=%q", res.SessionID, res.Profile, res.Model)
	}
	if res.RunID == "" {
		t.Error("RunID should be set")
	}
	sess, _ := prov.LoadSession(ctx, "sess-1")
	if len(sess.Runs) != 1 || sess.Runs[0] != res.RunID {
		t.Errorf("session Runs: %v", sess.Runs)
	}
	if sess.CurrentRunID != res.RunID {
		t.Errorf("CurrentRunID: %q", sess.CurrentRunID)
	}
	if sess.Mode != "team" {
		t.Errorf("Mode: %q", sess.Mode)
	}
	run, _ := prov.LoadRun(ctx, res.RunID)
	if run.Goal != "my goal" || run.Runtime == nil || run.Runtime.TeamID != "team-1" {
		t.Errorf("run: goal=%q runtime=%+v", run.Goal, run.Runtime)
	}
}

func TestPause_WithController(t *testing.T) {
	runs := map[string]types.Run{
		"run-1": {RunID: "run-1", SessionID: "sess-1", Status: types.RunStatusRunning},
	}
	sessions := map[string]types.Session{"sess-1": {SessionID: "sess-1", Runs: []string{"run-1"}}}
	prov := &mockSessionProvider{sessions: sessions, runs: runs}
	ctrl := &mockRuntimeController{}
	mgr := NewManager(prov, nil, nil)
	mgr.SetRuntimeController(ctrl)
	ctx := context.Background()
	err := mgr.Pause(ctx, "run-1", "sess-1")
	if err != nil {
		t.Fatalf("Pause: %v", err)
	}
	if len(ctrl.paused) != 1 || ctrl.paused[0] != "run-1" {
		t.Errorf("expected PauseRun(run-1), got %v", ctrl.paused)
	}
	// Run should not be updated in store when using controller
	run, _ := prov.LoadRun(ctx, "run-1")
	if run.Status != types.RunStatusRunning {
		t.Errorf("run status should be unchanged when using controller, got %q", run.Status)
	}
}

func TestPause_WithoutController_Fallback(t *testing.T) {
	runs := map[string]types.Run{
		"run-1": {RunID: "run-1", SessionID: "sess-1", Status: types.RunStatusRunning},
	}
	sessions := map[string]types.Session{"sess-1": {SessionID: "sess-1", Runs: []string{"run-1"}}}
	prov := &mockSessionProvider{sessions: sessions, runs: runs}
	cancel := &mockActiveTaskCanceler{}
	mgr := NewManager(prov, nil, cancel)
	ctx := context.Background()
	err := mgr.Pause(ctx, "run-1", "sess-1")
	if err != nil {
		t.Fatalf("Pause: %v", err)
	}
	run, _ := prov.LoadRun(ctx, "run-1")
	if run.Status != types.RunStatusPaused {
		t.Errorf("run status: got %q", run.Status)
	}
	if len(cancel.canceled) != 1 || cancel.canceled[0].runID != "run-1" || cancel.canceled[0].reason != "run paused" {
		t.Errorf("expected CancelActiveTasksByRun(run-1, \"run paused\"), got %v", cancel.canceled)
	}
}

func TestPause_InvalidStatus(t *testing.T) {
	runs := map[string]types.Run{
		"run-1": {RunID: "run-1", SessionID: "sess-1", Status: types.RunStatusSucceeded},
	}
	sessions := map[string]types.Session{"sess-1": {SessionID: "sess-1", Runs: []string{"run-1"}}}
	prov := &mockSessionProvider{sessions: sessions, runs: runs}
	mgr := NewManager(prov, nil, nil)
	ctx := context.Background()
	err := mgr.Pause(ctx, "run-1", "sess-1")
	if err == nil {
		t.Fatal("expected error for completed run")
	}
	var se *ServiceError
	if !errors.As(err, &se) || se.Code != CodeInvalidState {
		t.Errorf("expected ServiceError CodeInvalidState, got %v", err)
	}
}

func TestPause_WrongSession(t *testing.T) {
	runs := map[string]types.Run{
		"run-1": {RunID: "run-1", SessionID: "sess-other", Status: types.RunStatusRunning},
	}
	sessions := map[string]types.Session{"sess-1": {SessionID: "sess-1", Runs: []string{"run-1"}}}
	prov := &mockSessionProvider{sessions: sessions, runs: runs}
	mgr := NewManager(prov, nil, nil)
	ctx := context.Background()
	err := mgr.Pause(ctx, "run-1", "sess-1")
	if err == nil {
		t.Fatal("expected error when run belongs to different session")
	}
	var se *ServiceError
	if !errors.As(err, &se) || se.Code != CodeThreadNotFound {
		t.Errorf("expected ServiceError CodeThreadNotFound, got %v", err)
	}
}

func TestResume_WithController(t *testing.T) {
	runs := map[string]types.Run{
		"run-1": {RunID: "run-1", SessionID: "sess-1", Status: types.RunStatusPaused},
	}
	sessions := map[string]types.Session{"sess-1": {SessionID: "sess-1", Runs: []string{"run-1"}}}
	prov := &mockSessionProvider{sessions: sessions, runs: runs}
	ctrl := &mockRuntimeController{}
	mgr := NewManager(prov, nil, nil)
	mgr.SetRuntimeController(ctrl)
	ctx := context.Background()
	err := mgr.Resume(ctx, "run-1", "sess-1")
	if err != nil {
		t.Fatalf("Resume: %v", err)
	}
	if len(ctrl.resumed) != 1 || ctrl.resumed[0] != "run-1" {
		t.Errorf("expected ResumeRun(run-1), got %v", ctrl.resumed)
	}
}

func TestResume_WithoutController_Fallback(t *testing.T) {
	runs := map[string]types.Run{
		"run-1": {RunID: "run-1", SessionID: "sess-1", Status: types.RunStatusPaused},
	}
	sessions := map[string]types.Session{"sess-1": {SessionID: "sess-1", Runs: []string{"run-1"}}}
	prov := &mockSessionProvider{sessions: sessions, runs: runs}
	mgr := NewManager(prov, nil, nil)
	ctx := context.Background()
	err := mgr.Resume(ctx, "run-1", "sess-1")
	if err != nil {
		t.Fatalf("Resume: %v", err)
	}
	run, _ := prov.LoadRun(ctx, "run-1")
	if run.Status != types.RunStatusRunning {
		t.Errorf("run status: got %q", run.Status)
	}
}

func TestInferRunRoleAndTeam_FromRuntime(t *testing.T) {
	runs := map[string]types.Run{
		"run-1": {
			RunID: "run-1", SessionID: "sess-1",
			Runtime: &types.RunRuntimeConfig{Role: "researcher", TeamID: "team-1"},
		},
	}
	prov := &mockSessionProvider{runs: runs}
	mgr := NewManager(prov, nil, nil)
	ctx := context.Background()
	role, teamID := mgr.InferRunRoleAndTeam(ctx, "run-1")
	if role != "researcher" || teamID != "team-1" {
		t.Errorf("got role=%q teamID=%q", role, teamID)
	}
}

func TestInferRunRoleAndTeam_FromTasks(t *testing.T) {
	runs := map[string]types.Run{
		"run-1": {RunID: "run-1", SessionID: "sess-1", Runtime: &types.RunRuntimeConfig{}},
	}
	prov := &mockSessionProvider{runs: runs}
	tasks := &mockTaskLister{
		tasks: []types.Task{
			{TaskID: "t1", RunID: "run-1", TeamID: "team-2", AssignedRole: "ceo", RoleSnapshot: "ceo"},
		},
	}
	mgr := NewManager(prov, tasks, nil)
	ctx := context.Background()
	role, teamID := mgr.InferRunRoleAndTeam(ctx, "run-1")
	if role != "ceo" || teamID != "team-2" {
		t.Errorf("got role=%q teamID=%q", role, teamID)
	}
}
