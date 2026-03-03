package session_test

import (
	"context"
	"errors"
	"testing"
	"time"

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

func (m *mockStore) ListRunsBySessionIDs(ctx context.Context, sessionIDs []string) (map[string][]types.Run, error) {
	out := make(map[string][]types.Run, len(sessionIDs))
	for _, sessionID := range sessionIDs {
		runs, _ := m.ListRunsBySession(ctx, sessionID)
		out[sessionID] = runs
	}
	return out, nil
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
func (m *mockStore) ListActivitiesByRunIDs(ctx context.Context, runIDs []string, limit, offset int, sortDesc bool) ([]types.Activity, error) {
	return nil, nil
}
func (m *mockStore) CountActivities(ctx context.Context, runID string) (int, error) {
	return 0, nil
}
func (m *mockStore) CountActivitiesByRunIDs(ctx context.Context, runIDs []string) (int, error) {
	return 0, nil
}
func (m *mockStore) ListSessionIDs(ctx context.Context) ([]string, error) {
	return nil, nil
}

func (m *mockStore) LatestRun(ctx context.Context) (types.Run, error) {
	return types.Run{}, nil
}

func (m *mockStore) LatestRunningRun(ctx context.Context) (types.Run, error) {
	return types.Run{}, nil
}

type mockSupervisor struct {
	stoppedRuns []string
	stopErr     error
}

func (s *mockSupervisor) StopRun(ctx context.Context, runID string) error {
	s.stoppedRuns = append(s.stoppedRuns, runID)
	return s.stopErr
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

func TestManager_Delete_ContinuesWhenStopFails(t *testing.T) {
	store := &mockStore{
		runs: map[string]types.Run{
			"run-1": {RunID: "run-1", SessionID: "sess-1"},
		},
	}
	supervisor := &mockSupervisor{stopErr: errors.New("stop failed")}

	mgr := session.NewManager(config.Config{}, store, supervisor)

	if err := mgr.Delete(context.Background(), "sess-1"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	if len(supervisor.stoppedRuns) != 1 || supervisor.stoppedRuns[0] != "run-1" {
		t.Fatalf("expected stop attempt for run-1, got %v", supervisor.stoppedRuns)
	}
	if len(store.deletedSessions) != 1 || store.deletedSessions[0] != "sess-1" {
		t.Fatalf("expected session delete to continue, got %v", store.deletedSessions)
	}
}

func expectWake(t *testing.T, ch <-chan struct{}, label string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(300 * time.Millisecond):
		t.Fatalf("expected wake for %s", label)
	}
}

func expectNoWake(t *testing.T, ch <-chan struct{}, label string) {
	t.Helper()
	select {
	case <-ch:
		t.Fatalf("unexpected wake for %s", label)
	case <-time.After(80 * time.Millisecond):
	}
}

func TestManager_SubscribeWake_FiltersAndCancel(t *testing.T) {
	store := &mockStore{}
	mgr := session.NewManager(config.Config{}, store, &mockSupervisor{})

	allCh, cancelAll := mgr.SubscribeWake("", "")
	defer cancelAll()
	sessCh, cancelSess := mgr.SubscribeWake("sess-1", "")
	defer cancelSess()
	runCh, cancelRun := mgr.SubscribeWake("sess-1", "run-1")
	defer cancelRun()
	otherRunCh, cancelOtherRun := mgr.SubscribeWake("sess-1", "run-2")
	defer cancelOtherRun()

	if err := mgr.SaveSession(context.Background(), types.Session{SessionID: "sess-1"}); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}
	expectWake(t, allCh, "all watcher on SaveSession")
	expectWake(t, sessCh, "session watcher on SaveSession")
	expectNoWake(t, runCh, "run watcher on SaveSession")
	expectNoWake(t, otherRunCh, "other run watcher on SaveSession")

	if err := mgr.SaveRun(context.Background(), types.Run{SessionID: "sess-1", RunID: "run-1"}); err != nil {
		t.Fatalf("SaveRun: %v", err)
	}
	expectWake(t, allCh, "all watcher on SaveRun")
	expectWake(t, sessCh, "session watcher on SaveRun")
	expectWake(t, runCh, "run watcher on SaveRun")
	expectNoWake(t, otherRunCh, "other run watcher on SaveRun")

	cancelRun()
	if err := mgr.SaveRun(context.Background(), types.Run{SessionID: "sess-1", RunID: "run-1"}); err != nil {
		t.Fatalf("SaveRun second: %v", err)
	}
	expectWake(t, allCh, "all watcher after cancel")
	expectWake(t, sessCh, "session watcher after cancel")
	expectNoWake(t, otherRunCh, "other run watcher after cancel")
}
