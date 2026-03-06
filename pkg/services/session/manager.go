package session

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/tinoosan/agen8/pkg/config"
	pkgstore "github.com/tinoosan/agen8/pkg/store"
	"github.com/tinoosan/agen8/pkg/types"
	"github.com/tinoosan/agen8/pkg/wake"
)

// RuntimeSupervisor defines the interface for managing agent runtimes.
type RuntimeSupervisor interface {
	ResumeRun(ctx context.Context, runID string) error
	StopRun(ctx context.Context, runID string) error
}

var ErrRuntimeSupervisorNotConfigured = errors.New("runtime supervisor is not configured")

// Store defines the data access layer requirements for the session service.
type Store interface {
	pkgstore.SessionStore
	LoadRun(ctx context.Context, runID string) (types.Run, error)
	SaveRun(ctx context.Context, run types.Run) error
	StopRun(ctx context.Context, runID string, status string, errorMsg string) (types.Run, error)
	ListRunsBySession(ctx context.Context, sessionID string) ([]types.Run, error)
	ListRunsBySessionIDs(ctx context.Context, sessionIDs []string) (map[string][]types.Run, error)
	ListRunsByStatus(ctx context.Context, statuses []string) ([]types.Run, error)
	ListChildRuns(ctx context.Context, parentRunID string) ([]types.Run, error)
	AddRunToSession(ctx context.Context, sessionID, runID string) (types.Session, error)
	ListActivities(ctx context.Context, runID string, limit, offset int) ([]types.Activity, error)
	ListActivitiesByRunIDs(ctx context.Context, runIDs []string, limit, offset int, sortDesc bool) ([]types.Activity, error)
	CountActivities(ctx context.Context, runID string) (int, error)
	CountActivitiesByRunIDs(ctx context.Context, runIDs []string) (int, error)
	LatestRun(ctx context.Context) (types.Run, error)
	LatestRunningRun(ctx context.Context) (types.Run, error)
}

// Manager implements the Service interface.
type Manager struct {
	cfg        config.Config
	store      Store
	supervisor RuntimeSupervisor
	watchers   *wake.SignalHub[sessionWakeFilter]
}

type sessionWakeFilter struct {
	sessionID string
	runID     string
}

// NewManager creates a new session service manager.
func NewManager(cfg config.Config, store Store, supervisor RuntimeSupervisor) *Manager {
	return &Manager{
		cfg:        cfg,
		store:      store,
		supervisor: supervisor,
		watchers:   wake.NewSignalHub[sessionWakeFilter](),
	}
}

// SubscribeWake returns a channel receiving best-effort wake signals for matching
// session/run writes. Empty filters subscribe to all session mutations.
func (m *Manager) SubscribeWake(sessionID, runID string) (<-chan struct{}, func()) {
	if m == nil {
		ch := make(chan struct{})
		close(ch)
		return ch, func() {}
	}
	return m.watchers.Subscribe(sessionWakeFilter{
		sessionID: strings.TrimSpace(sessionID),
		runID:     strings.TrimSpace(runID),
	})
}

func (m *Manager) notifyWake(sessionID, runID string) {
	if m == nil {
		return
	}
	sessionID = strings.TrimSpace(sessionID)
	runID = strings.TrimSpace(runID)
	m.watchers.Notify(func(filter sessionWakeFilter) bool {
		if filter.sessionID != "" && filter.sessionID != sessionID {
			return false
		}
		if filter.runID != "" && filter.runID != runID {
			return false
		}
		return true
	})
}

// Start creates a new session and its first run, persists both, and links them.
func (m *Manager) Start(ctx context.Context, opts StartOptions) (types.Session, types.Run, error) {
	goal := strings.TrimSpace(opts.Goal)

	maxBytes := opts.MaxBytesForContext
	if maxBytes <= 0 {
		maxBytes = 8 * 1024
	}
	sess := types.NewSession(goal)
	if err := m.SaveSession(ctx, sess); err != nil {
		return types.Session{}, types.Run{}, fmt.Errorf("save session: %w", err)
	}
	run := types.NewRun(goal, maxBytes, sess.SessionID)
	if err := m.SaveRun(ctx, run); err != nil {
		return types.Session{}, types.Run{}, fmt.Errorf("save run: %w", err)
	}
	updated, err := m.AddRunToSession(ctx, sess.SessionID, run.RunID)
	if err != nil {
		return types.Session{}, types.Run{}, fmt.Errorf("add run to session: %w", err)
	}
	return updated, run, nil
}

// Stop terminates an active session's execution (stops all runs).
func (m *Manager) Stop(ctx context.Context, sessionID string) error {
	runs, err := m.store.ListRunsBySession(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("list runs: %w", err)
	}
	if m.supervisor == nil {
		return ErrRuntimeSupervisorNotConfigured
	}
	var errs []error
	for _, run := range runs {
		if err := m.supervisor.StopRun(ctx, run.RunID); err != nil {
			errs = append(errs, fmt.Errorf("stop run %s: %w", run.RunID, err))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("failed to stop session runs: %v", errs)
	}
	return nil
}

// Delete stops the session runs and removes persistent data.
func (m *Manager) Delete(ctx context.Context, sessionID string) error {
	if err := m.Stop(ctx, sessionID); err != nil {
		slog.Warn("session delete: stop runs best-effort", "component", "session", "session_id", strings.TrimSpace(sessionID), "error", err)
	}
	if err := m.store.DeleteSession(ctx, sessionID); err != nil {
		return fmt.Errorf("delete session storage: %w", err)
	}
	return nil
}

// LoadSession delegates to the store.
func (m *Manager) LoadSession(ctx context.Context, sessionID string) (types.Session, error) {
	return m.store.LoadSession(ctx, sessionID)
}

// SaveSession delegates to the store.
func (m *Manager) SaveSession(ctx context.Context, s types.Session) error {
	if err := m.store.SaveSession(ctx, s); err != nil {
		return err
	}
	m.notifyWake(strings.TrimSpace(s.SessionID), "")
	return nil
}

// ListSessionsPaginated delegates to the store.
func (m *Manager) ListSessionsPaginated(ctx context.Context, filter pkgstore.SessionFilter) ([]types.Session, error) {
	return m.store.ListSessionsPaginated(ctx, filter)
}

// CountSessions delegates to the store.
func (m *Manager) CountSessions(ctx context.Context, filter pkgstore.SessionFilter) (int, error) {
	return m.store.CountSessions(ctx, filter)
}

// LoadRun delegates to the store.
func (m *Manager) LoadRun(ctx context.Context, runID string) (types.Run, error) {
	return m.store.LoadRun(ctx, runID)
}

// SaveRun delegates to the store.
func (m *Manager) SaveRun(ctx context.Context, run types.Run) error {
	if err := m.store.SaveRun(ctx, run); err != nil {
		return err
	}
	m.notifyWake(strings.TrimSpace(run.SessionID), strings.TrimSpace(run.RunID))
	return nil
}

// StopRun delegates to the store.
func (m *Manager) StopRun(ctx context.Context, runID, status, errorMsg string) (types.Run, error) {
	run, err := m.store.StopRun(ctx, runID, status, errorMsg)
	if err != nil {
		return types.Run{}, err
	}
	m.notifyWake(strings.TrimSpace(run.SessionID), strings.TrimSpace(run.RunID))
	return run, nil
}

// ListRunsByStatus delegates to the store.
func (m *Manager) ListRunsByStatus(ctx context.Context, statuses []string) ([]types.Run, error) {
	return m.store.ListRunsByStatus(ctx, statuses)
}

// ListRunsBySession delegates to the store.
func (m *Manager) ListRunsBySession(ctx context.Context, sessionID string) ([]types.Run, error) {
	return m.store.ListRunsBySession(ctx, sessionID)
}

// ListRunsBySessionIDs delegates to the store.
func (m *Manager) ListRunsBySessionIDs(ctx context.Context, sessionIDs []string) (map[string][]types.Run, error) {
	return m.store.ListRunsBySessionIDs(ctx, sessionIDs)
}

// ListChildRuns delegates to the store.
func (m *Manager) ListChildRuns(ctx context.Context, parentRunID string) ([]types.Run, error) {
	return m.store.ListChildRuns(ctx, parentRunID)
}

// AddRunToSession delegates to the store.
func (m *Manager) AddRunToSession(ctx context.Context, sessionID, runID string) (types.Session, error) {
	sess, err := m.store.AddRunToSession(ctx, sessionID, runID)
	if err != nil {
		return types.Session{}, err
	}
	m.notifyWake(strings.TrimSpace(sessionID), strings.TrimSpace(runID))
	return sess, nil
}

// ListActivities delegates to the store.
func (m *Manager) ListActivities(ctx context.Context, runID string, limit, offset int) ([]types.Activity, error) {
	return m.store.ListActivities(ctx, runID, limit, offset)
}

// ListActivitiesByRunIDs delegates to the store.
func (m *Manager) ListActivitiesByRunIDs(ctx context.Context, runIDs []string, limit, offset int, sortDesc bool) ([]types.Activity, error) {
	return m.store.ListActivitiesByRunIDs(ctx, runIDs, limit, offset, sortDesc)
}

// CountActivities delegates to the store.
func (m *Manager) CountActivities(ctx context.Context, runID string) (int, error) {
	return m.store.CountActivities(ctx, runID)
}

// CountActivitiesByRunIDs delegates to the store.
func (m *Manager) CountActivitiesByRunIDs(ctx context.Context, runIDs []string) (int, error) {
	return m.store.CountActivitiesByRunIDs(ctx, runIDs)
}

// LatestRun delegates to the store.
func (m *Manager) LatestRun(ctx context.Context) (types.Run, error) {
	return m.store.LatestRun(ctx)
}

// LatestRunningRun delegates to the store.
func (m *Manager) LatestRunningRun(ctx context.Context) (types.Run, error) {
	return m.store.LatestRunningRun(ctx)
}
