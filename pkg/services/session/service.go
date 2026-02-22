package session

import (
	"context"

	"github.com/tinoosan/agen8/pkg/store"
	"github.com/tinoosan/agen8/pkg/types"
)

// StartOptions defines configuration for starting a new session.
type StartOptions struct {
	// Goal is the initial goal/prompt for the session.
	Goal string
	// MaxBytesForContext is the context window size for the initial run.
	MaxBytesForContext int
	// TaskID is optional, used when starting a session for a task.
	TaskID string
	// ParentRunID is optional, used for sub-agent sessions.
	ParentRunID string
}

// Service defines the interface for managing agent sessions.
// All session and run data access goes through this interface; callers do not use stores directly.
type Service interface {
	// Session CRUD
	LoadSession(ctx context.Context, sessionID string) (types.Session, error)
	SaveSession(ctx context.Context, s types.Session) error
	Start(ctx context.Context, opts StartOptions) (types.Session, types.Run, error)
	Delete(ctx context.Context, sessionID string) error
	ListSessionsPaginated(ctx context.Context, filter store.SessionFilter) ([]types.Session, error)
	CountSessions(ctx context.Context, filter store.SessionFilter) (int, error)

	// Runs
	LoadRun(ctx context.Context, runID string) (types.Run, error)
	SaveRun(ctx context.Context, run types.Run) error
	StopRun(ctx context.Context, runID string, status string, errorMsg string) (types.Run, error)
	ListRunsBySession(ctx context.Context, sessionID string) ([]types.Run, error)
	ListRunsByStatus(ctx context.Context, statuses []string) ([]types.Run, error)
	ListChildRuns(ctx context.Context, parentRunID string) ([]types.Run, error)
	AddRunToSession(ctx context.Context, sessionID, runID string) (types.Session, error)

	// Activities
	ListActivities(ctx context.Context, runID string, limit, offset int) ([]types.Activity, error)
	CountActivities(ctx context.Context, runID string) (int, error)

	// Latest run helpers (for CLI/TUI when no run ID is specified)
	LatestRun(ctx context.Context) (types.Run, error)
	LatestRunningRun(ctx context.Context) (types.Run, error)
}
