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
	StatsDecisions(ctx context.Context, filter DecisionFilter) (DecisionStats, error)
	ExportDecisions(ctx context.Context, filter DecisionFilter) ([]Decision, error)
	DecisionExistsByFingerprint(ctx context.Context, projectID, sourceIdentity, title, taskRef string, since time.Time) (bool, error)
}

// LowConfidenceThreshold is the confidence cutoff below which a logged decision
// is considered to warrant review. Single-sourced here so the SQLite aggregate
// and any caller reporting "needs review" agree on the boundary.
const LowConfidenceThreshold = 0.5

// DecisionStats summarizes a filtered set of decisions into the few signals that
// make agent decision-making easy to reason about at a glance.
type DecisionStats struct {
	Total                      int // Total decisions matching the filter
	LowConfidence              int // Decisions with confidence < LowConfidenceThreshold
	Unlinked                   int // Decisions with no task/key-result/mission ref
	WithInvalidationConditions int // Decisions that recorded revisit conditions
}

type Writer interface {
	CreateDecision(ctx context.Context, decision Decision) error
	DeleteDecision(ctx context.Context, id DecisionID) error
}

// DecisionFilter controls optional filtering and pagination for queries.
type DecisionFilter struct {
	ProjectID string
	Sources   []DecisionSource // Filter by source(s) - AND with other filters
	Tags      []string         // Filter by tags (AND semantics - all tags must match)
	Query     string           // Free-text query across decision content
	Since     *time.Time       // Filter by created_at >= since
	Until     *time.Time       // Filter by created_at <= until
	SortDesc  bool             // Sort newest first when true, oldest first when false
	Limit     int
	Offset    int
}
