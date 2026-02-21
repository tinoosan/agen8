package store

import (
	"context"

	"github.com/tinoosan/agen8/pkg/types"
)

// SessionFilter specifies filtering and pagination for session queries.
type SessionFilter struct {
	// Filtering
	TitleContains string // case-insensitive substring match against title/current goal
	IncludeSystem bool   // include daemon/system sessions in results

	// Pagination
	Limit  int // max results (default: 50, 0 = use default)
	Offset int // skip N sessions

	// Sorting
	SortBy   string // "updated_at" (default), "created_at", "title"
	SortDesc bool   // true = DESC (default), false = ASC
}

// SessionReader loads sessions.
type SessionReader interface {
	LoadSession(ctx context.Context, sessionID string) (types.Session, error)
}

// SessionWriter persists sessions.
type SessionWriter interface {
	SaveSession(ctx context.Context, s types.Session) error
	DeleteSession(ctx context.Context, sessionID string) error
}

// SessionPager supports paginated session listing and counting.
type SessionPager interface {
	ListSessionsPaginated(ctx context.Context, filter SessionFilter) ([]types.Session, error)
	CountSessions(ctx context.Context, filter SessionFilter) (int, error)
}

// SessionLister provides non-paginated listing utilities.
type SessionLister interface {
	ListSessionIDs(ctx context.Context) ([]string, error)
	ListSessions(ctx context.Context) ([]types.Session, error)
}

// SessionReaderWriter is the minimal interface for components that only load/save sessions.
type SessionReaderWriter interface {
	SessionReader
	SessionWriter
}

// SessionQuery is the minimal interface for components that need to query sessions and load details.
type SessionQuery interface {
	SessionReader
	SessionPager
}

// SessionStore is the convenience "full" interface combining all session operations.
type SessionStore interface {
	SessionReaderWriter
	SessionPager
	SessionLister
}
