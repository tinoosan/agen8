package session

import (
	"context"

	"github.com/tinoosan/workbench-core/pkg/agent/session"
	pkgstore "github.com/tinoosan/workbench-core/pkg/store"
)

// StartOptions defines configuration for starting a new session.
type StartOptions struct {
	// TaskID is the ID of the task to start the session for.
	TaskID string
	// Goal is the initial goal/prompt for the session.
	Goal string
	// ParentRunID is optional, used for sub-agent sessions.
	ParentRunID string
}

// Service defines the interface for managing agent sessions.
type Service interface {
	pkgstore.SessionReaderWriter

	// Start creates and starts a new session.
	Start(ctx context.Context, opts StartOptions) (*session.Session, error)

	// Get retrieves an active session by ID.
	Get(ctx context.Context, sessionID string) (*session.Session, error)

	// List returns all active sessions.
	List(ctx context.Context) ([]*session.Session, error)

	// Stop terminates an active session's execution but keeps its state.
	Stop(ctx context.Context, sessionID string) error

	// Delete stops the session (if running) and removes its persistent data
	// and workspace artifacts.
	Delete(ctx context.Context, sessionID string) error
}
