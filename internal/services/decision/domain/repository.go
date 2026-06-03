package domain

import (
	"context"
	"time"
)

type Repository interface {
	Reader
	Writer
}

type Reader interface {
	GetDecision(ctx context.Context, id DecisionID) (Decision, error)
	ListDecisions(ctx context.Context, filter DecisionFilter) ([]Decision, error)
	ListDecisionsByKeyResult(ctx context.Context, keyResultRef string) ([]Decision, error)
	CountDecisions(ctx context.Context, filter DecisionFilter) (int, error)
	ExportDecisions(ctx context.Context, filter DecisionFilter) ([]Decision, error)
	DecisionExistsByFingerprint(ctx context.Context, projectID, sourceIdentity, title, taskRef string, since time.Time) (bool, error)
}

type Writer interface {
	CreateDecision(ctx context.Context, decision Decision) error
	DeleteDecision(ctx context.Context, id DecisionID) error
}

// DecisionFilter controls optional filtering and pagination for queries.
type DecisionFilter struct {
	ProjectID string
	Sources   []DecisionSource // Filter by source(s) — AND with other filters
	SpaceID   string           // Filter by owning space
	Tags      []string         // Filter by tags (AND semantics — all tags must match)
	Query     string           // Free-text query across decision content
	Since     *time.Time       // Filter by created_at >= since
	Until     *time.Time       // Filter by created_at <= until
	SortDesc  bool             // Sort newest first when true, oldest first when false
	Limit     int
	Offset    int
}
