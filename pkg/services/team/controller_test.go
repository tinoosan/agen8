package team

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/tinoosan/agen8/pkg/agent/state"
	"github.com/tinoosan/agen8/pkg/types"
)

func TestController_SetModel_ThreadNotFound(t *testing.T) {
	ctrl := NewController(ControllerConfig{
		SessionService: &mockSessionService{},
		Runtimes:       []RoleRunController{},
	})
	_, err := ctrl.SetModel(context.Background(), "nonexistent", "", "gpt-5")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrThreadNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestController_SetReasoning_ThreadNotFound(t *testing.T) {
	ctrl := NewController(ControllerConfig{Runtimes: []RoleRunController{}})
	_, err := ctrl.SetReasoning(context.Background(), "nonexistent", "", "high", "auto")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrThreadNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestController_PauseRuns_ThreadNotFound(t *testing.T) {
	ctrl := NewController(ControllerConfig{Runtimes: []RoleRunController{}})
	_, err := ctrl.PauseRuns(context.Background(), "nonexistent", "")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrThreadNotFound) {
		t.Fatalf("err = %v", err)
	}
}

type mockRoleRunController struct {
	runID     string
	sessionID string
	paused    bool
	model     string
	effort    string
	summary   string
}

func (m *mockRoleRunController) RunID() string     { return m.runID }
func (m *mockRoleRunController) SessionID() string { return m.sessionID }
func (m *mockRoleRunController) SetPaused(p bool)  { m.paused = p }
func (m *mockRoleRunController) SetModel(ctx context.Context, model string) error {
	m.model = model
	return nil
}
func (m *mockRoleRunController) SetReasoning(ctx context.Context, effort, summary string) error {
	m.effort = effort
	m.summary = summary
	return nil
}

type mockActiveTaskCanceler struct {
	cancelFn func(ctx context.Context, runID, reason string) (int, error)
}

func (m *mockActiveTaskCanceler) CancelActiveTasksByRun(ctx context.Context, runID, reason string) (int, error) {
	if m.cancelFn != nil {
		return m.cancelFn(ctx, runID, reason)
	}
	return 0, nil
}

func TestController_SetModel_Integration(t *testing.T) {
	var savedSession types.Session
	sessionSvc := &mockSessionService{
		loadSession: func(ctx context.Context, sessionID string) (types.Session, error) {
			return types.Session{SessionID: "sess-1", ActiveModel: "gpt-4"}, nil
		},
		saveSession: func(ctx context.Context, s types.Session) error {
			savedSession = s
			return nil
		},
	}
	taskStore := &mockTaskStore{countTasks: func(ctx context.Context, filter state.TaskFilter) (int, error) {
		return 0, nil
	}}
	store := &mockManifestStore{save: func(ctx context.Context, m Manifest) error { return nil }}
	stateMgr := NewStateManager(store, Manifest{TeamID: "team-1"})
	rt := &mockRoleRunController{runID: "run-1", sessionID: "sess-1"}
	applier := &mockModelApplier{apply: func(ctx context.Context, model, target string) ([]string, error) {
		return []string{"run-1"}, nil
	}}
	ctrl := NewController(ControllerConfig{
		SessionService:          sessionSvc,
		TaskStore:               taskStore,
		StateMgr:                stateMgr,
		Runtimes:                []RoleRunController{rt},
		Applier:                 applier,
		DefaultReasoningEffort:  "medium",
		DefaultReasoningSummary: "auto",
	})
	applied, err := ctrl.SetModel(context.Background(), "sess-1", "", "gpt-5")
	if err != nil {
		t.Fatalf("SetModel: %v", err)
	}
	if len(applied) != 1 || applied[0] != "run-1" {
		t.Fatalf("applied = %v", applied)
	}
	if savedSession.ActiveModel != "gpt-5" {
		t.Fatalf("savedSession.ActiveModel = %q", savedSession.ActiveModel)
	}
	// Controller applies reasoning to runtimes for applied run IDs; applier is responsible for SetModel.
	if rt.effort != "medium" || rt.summary != "auto" {
		t.Fatalf("runtime effort=%q summary=%q", rt.effort, rt.summary)
	}
}

func TestController_SetReasoning_Integration(t *testing.T) {
	sessionSvc := &mockSessionService{
		loadSession: func(ctx context.Context, sessionID string) (types.Session, error) {
			return types.Session{SessionID: "sess-1"}, nil
		},
		saveSession: func(ctx context.Context, s types.Session) error { return nil },
	}
	rt := &mockRoleRunController{runID: "run-1", sessionID: "sess-1"}
	ctrl := NewController(ControllerConfig{
		SessionService: sessionSvc,
		Runtimes:       []RoleRunController{rt},
	})
	applied, err := ctrl.SetReasoning(context.Background(), "sess-1", "", "high", "detailed")
	if err != nil {
		t.Fatalf("SetReasoning: %v", err)
	}
	if len(applied) != 1 || applied[0] != "run-1" {
		t.Fatalf("applied = %v", applied)
	}
	if rt.effort != "high" || rt.summary != "detailed" {
		t.Fatalf("runtime effort=%q summary=%q", rt.effort, rt.summary)
	}
}

func TestController_PauseRuns_DoesNotCancelActiveTasks(t *testing.T) {
	sessionSvc := &mockSessionService{}
	rt := &mockRoleRunController{runID: "run-1", sessionID: "sess-1"}
	cancelCalled := false
	taskCanceler := &mockActiveTaskCanceler{
		cancelFn: func(ctx context.Context, runID, reason string) (int, error) {
			cancelCalled = true
			return 0, fmt.Errorf("cancel should not be called on pause")
		},
	}
	ctrl := NewController(ControllerConfig{
		SessionService: sessionSvc,
		TaskCanceler:   taskCanceler,
		Runtimes:       []RoleRunController{rt},
	})

	affected, err := ctrl.PauseRuns(context.Background(), "sess-1", "")
	if err != nil {
		t.Fatalf("PauseRuns: %v", err)
	}
	if len(affected) != 1 || affected[0] != "run-1" {
		t.Fatalf("affected = %v", affected)
	}
	if cancelCalled {
		t.Fatalf("expected no task cancellation on pause")
	}
}

func TestController_StopRuns_ReturnsStopAndCancelErrors(t *testing.T) {
	sessionSvc := &mockSessionService{}
	rt := &mockRoleRunController{runID: "run-1", sessionID: "sess-1"}
	ctrl := NewController(ControllerConfig{
		SessionService: sessionSvc,
		TaskCanceler: &mockActiveTaskCanceler{
			cancelFn: func(ctx context.Context, runID, reason string) (int, error) {
				return 0, fmt.Errorf("cancel failed")
			},
		},
		RunStopper: &mockRunStopper{
			stopRun: func(ctx context.Context, runID string) error {
				return fmt.Errorf("stop failed")
			},
		},
		Runtimes: []RoleRunController{rt},
	})

	affected, err := ctrl.StopRuns(context.Background(), "sess-1", "")
	if err == nil {
		t.Fatalf("expected stop error")
	}
	if len(affected) != 0 {
		t.Fatalf("expected no successful runs on stop failure, got %v", affected)
	}
	if !errors.Is(err, ErrStopRunFailed) {
		t.Fatalf("expected stop-run error, got: %v", err)
	}
	if !errors.Is(err, ErrCancelActive) {
		t.Fatalf("expected cancel error, got: %v", err)
	}
	if !rt.paused {
		t.Fatalf("expected runtime paused flag to be set")
	}
}
