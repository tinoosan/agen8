package domain

import (
	"context"
	"time"

	spacedomain "github.com/tinoosan/agen8-mcp-server/internal/services/space/domain"
)

type Repository interface {
	Reader
	Writer
}

type Reader interface {
	Get(ctx context.Context, id EntryID) (Entry, error)
	List(ctx context.Context, filter Filter) ([]Entry, error)
	ListDue(ctx context.Context, now time.Time, limit int) ([]Entry, error)
	ListRuns(ctx context.Context, filter RunFilter) ([]Run, error)
}

type Writer interface {
	Create(ctx context.Context, entry Entry) error
	Update(ctx context.Context, entry Entry) error
	ClaimDue(ctx context.Context, run Run) (Run, bool, error)
	UpdateRun(ctx context.Context, run Run) error
}

type Filter struct {
	SpaceID spacedomain.SpaceID
	Status  EntryStatus
	Limit   int
}

type RunFilter struct {
	EntryID EntryID
	SpaceID spacedomain.SpaceID
	Limit   int
}
