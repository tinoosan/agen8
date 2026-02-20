package session_test

import (
	"context"
	"testing"

	"github.com/tinoosan/agen8/pkg/config"
	"github.com/tinoosan/agen8/pkg/services/session"
	pkgstore "github.com/tinoosan/agen8/pkg/store"
	"github.com/tinoosan/agen8/pkg/types"
)

type mockStore struct {
	runs            map[string]types.Run
	deletedSessions []string
}

func (m *mockStore) ListRunsBySession(ctx context.Context, sessionID string) ([]types.Run, error) {
	var result []types.Run
	for _, r := range m.runs {
		if r.SessionID == sessionID {
			result = append(result, r)
		}
	}
	return result, nil
}

func (m *mockStore) DeleteSession(ctx context.Context, sessionID string) error {
	m.deletedSessions = append(m.deletedSessions, sessionID)
	return nil
}

func (m *mockStore) LoadRun(ctx context.Context, runID string) (types.Run, error) {
	return types.Run{}, nil
}
func (m *mockStore) LoadSession(ctx context.Context, sessionID string) (types.Session, error) {
	return types.Session{}, nil
}
func (m *mockStore) SaveSession(ctx context.Context, session types.Session) error {
	return nil
}
func (m *mockStore) ListSessions(ctx context.Context) ([]types.Session, error) {
	return nil, nil
}
func (m *mockStore) ListSessionsPaginated(ctx context.Context, filter pkgstore.SessionFilter) ([]types.Session, error) {
	return nil, nil
}
func (m *mockStore) CountSessions(ctx context.Context, filter pkgstore.SessionFilter) (int, error) {
	return 0, nil
}
func (m *mockStore) SaveRun(ctx context.Context, run types.Run) error {
	if m.runs == nil {
		m.runs = make(map[string]types.Run)
	}
	m.runs[run.RunID] = run
	return nil
}
func (m *mockStore) StopRun(ctx context.Context, runID, status, errorMsg string) (types.Run, error) {
	return types.Run{}, nil
}
func (m *mockStore) ListRunsByStatus(ctx context.Context, statuses []string) ([]types.Run, error) {
	return nil, nil
}
func (m *mockStore) ListChildRuns(ctx context.Context, parentRunID string) ([]types.Run, error) {
	return nil, nil
}
func (m *mockStore) AddRunToSession(ctx context.Context, sessionID, runID string) (types.Session, error) {
	return types.Session{SessionID: sessionID, Runs: []string{runID}}, nil
}
func (m *mockStore) ListActivities(ctx context.Context, runID string, limit, offset int) ([]types.Activity, error) {
	return nil, nil
}
func (m *mockStore) CountActivities(ctx context.Context, runID string) (int, error) {
	return 0, nil
}
func (m *mockStore) ListSessionIDs(ctx context.Context) ([]string, error) {
	return nil, nil
}

type mockSupervisor struct {
	stoppedRuns []string
}

func (s *mockSupervisor) StopRun(runID string) error {
	s.stoppedRuns = append(s.stoppedRuns, runID)
	return nil
}

func (s *mockSupervisor) ResumeRun(ctx context.Context, runID string) error {
	return nil
}

func TestManager_Delete(t *testing.T) {
	store := &mockStore{
		runs: map[string]types.Run{
			"run-1": {RunID: "run-1", SessionID: "sess-1"},
			"run-2": {RunID: "run-2", SessionID: "sess-1"},
			"run-3": {RunID: "run-3", SessionID: "sess-2"},
		},
	}
	supervisor := &mockSupervisor{}

	mgr := session.NewManager(config.Config{}, store, supervisor)

	err := mgr.Delete(context.Background(), "sess-1")
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Check if runs were stopped
	if len(supervisor.stoppedRuns) != 2 {
		t.Errorf("expected 2 stopped runs, got %d", len(supervisor.stoppedRuns))
	}
	// Verify exact runs stopped (order might vary, so use map or contains check)
	stoppedMap := make(map[string]bool)
	for _, id := range supervisor.stoppedRuns {
		stoppedMap[id] = true
	}
	if !stoppedMap["run-1"] || !stoppedMap["run-2"] {
		t.Errorf("expected run-1 and run-2 to be stopped, got %v", supervisor.stoppedRuns)
	}

	// Check if session was deleted from store
	if len(store.deletedSessions) != 1 || store.deletedSessions[0] != "sess-1" {
		t.Errorf("expected session sess-1 to be deleted, got %v", store.deletedSessions)
	}
}
