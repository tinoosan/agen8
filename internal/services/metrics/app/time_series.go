package app

import (
	"context"
	"strings"
	"time"

	"github.com/tinoosan/agen8-mcp-server/internal/services/metrics/domain"
)

// TimeSeriesParams are the parameters for a time-series query.
type TimeSeriesParams struct {
	Scope       domain.Scope
	ScopeID     string
	Metric      domain.Metric
	Granularity domain.Granularity
	FromTime    string
	ToTime      string
}

// TimeSeries returns time-bucketed metric data.
func (s *Service) TimeSeries(ctx context.Context, p TimeSeriesParams) (domain.TimeSeries, error) {
	empty := domain.TimeSeries{Points: []domain.TimeSeriesPoint{}}

	granularity := p.Granularity
	if granularity == "" {
		granularity = domain.GranularityHour
	}

	// Parse time range.
	toTime := time.Now().UTC()
	fromTime := toTime.Add(-24 * time.Hour)
	if p.FromTime != "" {
		if t, err := time.Parse(time.RFC3339, p.FromTime); err == nil {
			fromTime = t
		}
	}
	if p.ToTime != "" {
		if t, err := time.Parse(time.RFC3339, p.ToTime); err == nil {
			toTime = t
		}
	}

	// Resolve run IDs for the scope.
	runIDs, err := s.resolveRunIDs(ctx, p.Scope, p.ScopeID)
	if err != nil {
		return empty, err
	}
	if len(runIDs) == 0 {
		return empty, nil
	}

	if s.timeseries == nil {
		return empty, nil
	}

	return s.timeseries.Query(ctx, runIDs, p.Metric, granularity, domain.TimeRange{From: fromTime, To: toTime})
}

// resolveRunIDs returns the run IDs for a given metrics scope.
func (s *Service) resolveRunIDs(ctx context.Context, scope domain.Scope, scopeID string) ([]string, error) {
	switch scope {
	case domain.ScopeProject:
		return s.resolveProjectRunIDs(ctx, scopeID)
	case domain.ScopeSpace:
		return s.resolveSpaceRunIDs(ctx, scopeID)
	case domain.ScopeMember:
		return []string{scopeID}, nil
	default:
		return nil, &domain.ErrInvalidInput{Field: "scope", Message: "unsupported scope: " + string(scope)}
	}
}

func (s *Service) resolveProjectRunIDs(ctx context.Context, projectRoot string) ([]string, error) {
	if s.projects == nil {
		return nil, nil
	}
	spaces, err := s.projects.ListSpaces(ctx, projectRoot)
	if err != nil {
		return nil, err
	}

	var runIDs []string
	seen := map[string]struct{}{}
	for _, space := range spaces {
		spaceID := strings.TrimSpace(space.SpaceID)
		if spaceID == "" {
			continue
		}
		ids, err := s.collectSpaceRunIDs(ctx, spaceID)
		if err != nil {
			return nil, err
		}
		for _, id := range ids {
			if _, ok := seen[id]; !ok {
				runIDs = append(runIDs, id)
				seen[id] = struct{}{}
			}
		}
	}
	return runIDs, nil
}

func (s *Service) resolveSpaceRunIDs(ctx context.Context, spaceID string) ([]string, error) {
	ids, err := s.collectSpaceRunIDs(ctx, spaceID)
	if err != nil {
		return nil, err
	}
	// Deduplicate.
	seen := map[string]struct{}{}
	var runIDs []string
	for _, id := range ids {
		if _, ok := seen[id]; !ok {
			runIDs = append(runIDs, id)
			seen[id] = struct{}{}
		}
	}
	return runIDs, nil
}

func (s *Service) collectSpaceRunIDs(ctx context.Context, spaceID string) ([]string, error) {
	var runIDs []string

	if s.manifests != nil {
		manifestRunIDs, err := s.manifests.LoadRunMembers(ctx, spaceID)
		if err != nil {
			return nil, err
		}
		for _, runID := range manifestRunIDs {
			runID = strings.TrimSpace(runID)
			if runID != "" {
				runIDs = append(runIDs, runID)
			}
		}
	}

	if s.tasks != nil {
		taskRunIDs, err := s.tasks.ListRunIDsBySpace(ctx, spaceID, 500)
		if err != nil {
			return nil, err
		}
		for _, runID := range taskRunIDs {
			runID = strings.TrimSpace(runID)
			if runID != "" {
				runIDs = append(runIDs, runID)
			}
		}
	}

	return runIDs, nil
}
