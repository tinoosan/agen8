package domain

import (
	"context"
	"time"
)

// CostAggregator queries aggregate cost/token data (space scope).
// Encapsulates the runs-first, tasks-fallback query strategy.
type CostAggregator interface {
	AggregateBySpace(ctx context.Context, spaceID, member string) (TokenCost, int, error)
}

// TimeSeriesQuerier queries time-bucketed metric data.
type TimeSeriesQuerier interface {
	Query(ctx context.Context, runIDs []string, metric Metric, granularity Granularity, tr TimeRange) (TimeSeries, error)
}

// ToolUsageQuerier queries tool/activity usage data.
type ToolUsageQuerier interface {
	QueryByRun(ctx context.Context, runID string, limit int) ([]ToolUsage, error)
}

// RunLoader loads run data for token/cost/member resolution.
type RunLoader interface {
	LoadRun(ctx context.Context, runID string) (RunInfo, error)
}

// RunInfo is the minimal run projection needed by metrics.
type RunInfo struct {
	RunID        string
	InputTokens  int
	OutputTokens int
	CostUSD      float64
	Member       string
	Goal         string
}

// TaskQuerier provides task-level data access for metrics.
type TaskQuerier interface {
	CountByStatus(ctx context.Context, spaceID string, statuses []string) (int, error)
	CountByRunAndStatus(ctx context.Context, spaceID, runID string, statuses []string) (int, error)
	ListCompleted(ctx context.Context, spaceID string, limit int) ([]TaskInfo, error)
	ListCompletedByRun(ctx context.Context, runID string, limit int) ([]TaskInfo, error)
	GetRunStats(ctx context.Context, runID string) (RunTaskStats, error)
	ListRunIDsBySpace(ctx context.Context, spaceID string, limit int) ([]string, error)
}

// TaskInfo is the minimal task projection needed by metrics.
type TaskInfo struct {
	TaskID      string
	RunID       string
	Goal        string
	Status      string
	CostUSD     float64
	CreatedAt   *time.Time
	CompletedAt *time.Time
}

// RunTaskStats is per-run task statistics.
type RunTaskStats struct {
	Succeeded int
	Failed    int
	TotalCost float64
}

// ManifestLoader resolves manifest data for run-member mapping.
type ManifestLoader interface {
	LoadRunMembers(ctx context.Context, spaceID string) (runIDs []string, err error)
	LoadManifest(ctx context.Context, spaceID string) (*ManifestInfo, error)
}

// ManifestInfo is the minimal manifest projection needed by metrics.
type ManifestInfo struct {
	Members []ManifestMember
}

// ManifestMember is a member entry in a manifest.
type ManifestMember struct {
	MemberLabel string
	RunID       string
}

// ProjectSpaceLister lists spaces within a project for project-scope metrics.
type ProjectSpaceLister interface {
	ListSpacesIncludingDeleted(ctx context.Context, projectRoot string) ([]ProjectSpaceInfo, error)
	ListSpaces(ctx context.Context, projectRoot string) ([]ProjectSpaceInfo, error)
}

// ProjectSpaceInfo is the minimal space-in-project projection.
type ProjectSpaceInfo struct {
	SpaceID string
	Status  string
}
