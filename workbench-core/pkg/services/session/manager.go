package session

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/tinoosan/workbench-core/pkg/agent/session"
	"github.com/tinoosan/workbench-core/pkg/config"
	pkgstore "github.com/tinoosan/workbench-core/pkg/store"
	"github.com/tinoosan/workbench-core/pkg/types"
)

// RuntimeSupervisor defines the interface for managing agent runtimes.
// This matches the internal app.RuntimeSupervisor methods needed by the service.
type RuntimeSupervisor interface {
	ResumeRun(ctx context.Context, runID string) error
	StopRun(runID string) error
	// GetSessionRunIDs returns all run IDs associated with a session.
	// Note: We might need to expose this from the supervisor or rely on the store.
}

// Store defines the data access layer requirements for the session service.
type Store interface {
	pkgstore.SessionStore
	LoadRun(ctx context.Context, runID string) (types.Run, error)
	ListRunsBySession(ctx context.Context, sessionID string) ([]types.Run, error)
}

// Manager implements the Service interface.
type Manager struct {
	cfg        config.Config
	store      Store
	supervisor RuntimeSupervisor

	mu       sync.RWMutex
	sessions map[string]*session.Session
}

// NewManager creates a new session service manager.
func NewManager(cfg config.Config, store Store, supervisor RuntimeSupervisor) *Manager {
	return &Manager{
		cfg:        cfg,
		store:      store,
		supervisor: supervisor,
		sessions:   make(map[string]*session.Session),
	}
}

// Start creates and starts a new session.
func (m *Manager) Start(ctx context.Context, opts StartOptions) (*session.Session, error) {
	// This logic usually resides in app.RunChat or similar.
	// For now, we'll need to coordinate with the existing Daemon logic or
	// move the session creation logic here.
	// Since creating a session involves specific agent/profile configuration that might
	// be tied to the Daemon's state, we might need a "SessionFactory" or similar dependency.

	// Placeholder: In strict refactoring, we might wrap existing logic.
	// But `Start` implies creating the record AND starting the runtime.
	return nil, errors.New("not implemented: requires moving creation logic from daemon")
}

// Get retrieves an active session by ID.
func (m *Manager) Get(ctx context.Context, sessionID string) (*session.Session, error) {
	m.mu.RLock()
	sess, ok := m.sessions[sessionID]
	m.mu.RUnlock()
	if ok {
		return sess, nil
	}

	// If not in memory, we might just load the data, but *session.Session is the live object.
	// If it's not live, we return nil or error?
	// The interface implies getting the live session controller.
	return nil, fmt.Errorf("session %s is not active", sessionID)
}

// List returns all active sessions.
func (m *Manager) List(ctx context.Context) ([]*session.Session, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	list := make([]*session.Session, 0, len(m.sessions))
	for _, s := range m.sessions {
		list = append(list, s)
	}
	return list, nil
}

// Stop terminates an active session's execution logic.
func (m *Manager) Stop(ctx context.Context, sessionID string) error {
	runs, err := m.store.ListRunsBySession(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("list runs: %w", err)
	}

	var errs []error
	for _, run := range runs {
		if err := m.supervisor.StopRun(run.RunID); err != nil {
			errs = append(errs, fmt.Errorf("stop run %s: %w", run.RunID, err))
		}
	}

	m.mu.Lock()
	delete(m.sessions, sessionID)
	m.mu.Unlock()

	if len(errs) > 0 {
		return fmt.Errorf("failed to stop session runs: %v", errs)
	}
	return nil
}

// Delete stops the session and removes persistent data.
func (m *Manager) Delete(ctx context.Context, sessionID string) error {
	// 1. Stop the session if it's running
	if err := m.Stop(ctx, sessionID); err != nil {
		// Proceed anyway? Or fail?
		// Best effort stop.
		fmt.Printf("stop session %s failed during delete: %v\n", sessionID, err)
	}

	// 2. Delete from Store (DB + FS)
	if err := m.store.DeleteSession(ctx, sessionID); err != nil {
		return fmt.Errorf("delete session storage: %w", err)
	}

	return nil
}

// LoadSession delegates to the underlying store.
func (m *Manager) LoadSession(ctx context.Context, sessionID string) (types.Session, error) {
	return m.store.LoadSession(ctx, sessionID)
}

// SaveSession delegates to the underlying store.
func (m *Manager) SaveSession(ctx context.Context, s types.Session) error {
	return m.store.SaveSession(ctx, s)
}

// DeleteSession delegates to the underlying store.
func (m *Manager) DeleteSession(ctx context.Context, sessionID string) error {
	return m.store.DeleteSession(ctx, sessionID)
}
