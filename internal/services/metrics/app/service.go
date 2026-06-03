package app

import (
	"context"
	"strings"

	"github.com/tinoosan/agen8-mcp-server/internal/services/metrics/domain"
)

// Service is the metrics application service. It coordinates between
// repositories and the domain to implement metrics use cases.
type Service struct {
	costs      domain.CostAggregator
	timeseries domain.TimeSeriesQuerier
	tools      domain.ToolUsageQuerier
	runs       domain.RunLoader
	tasks      domain.TaskQuerier
	manifests  domain.ManifestLoader
	projects   domain.ProjectSpaceLister
}

// NewService constructs the application service with its dependencies.
func NewService(
	costs domain.CostAggregator,
	timeseries domain.TimeSeriesQuerier,
	tools domain.ToolUsageQuerier,
	runs domain.RunLoader,
	tasks domain.TaskQuerier,
	manifests domain.ManifestLoader,
	projects domain.ProjectSpaceLister,
) *Service {
	return &Service{
		costs:      costs,
		timeseries: timeseries,
		tools:      tools,
		runs:       runs,
		tasks:      tasks,
		manifests:  manifests,
		projects:   projects,
	}
}

// aggregateCostForRun loads a Run for tokens/cost, falling back to task stats
// when cost is zero. This is app-layer orchestration (not SQL).
func (s *Service) aggregateCostForRun(ctx context.Context, runID string) domain.TokenCost {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return domain.TokenCost{}
	}

	var tc domain.TokenCost
	if s.runs != nil {
		if info, err := s.runs.LoadRun(ctx, runID); err == nil {
			tc.InputTokens = info.InputTokens
			tc.OutputTokens = info.OutputTokens
			tc.CostUSD = info.CostUSD
		}
	}

	if tc.CostUSD == 0 && s.tasks != nil {
		if stats, err := s.tasks.GetRunStats(ctx, runID); err == nil {
			tc.CostUSD = stats.TotalCost
		}
	}

	return tc
}

func truncateGoal(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}
