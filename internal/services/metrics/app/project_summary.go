package app

import (
	"context"
	"strings"

	"github.com/tinoosan/agen8-mcp-server/internal/services/metrics/domain"
)

// ProjectSummary aggregates cost, token, and task data across all spaces in a project.
func (s *Service) ProjectSummary(ctx context.Context, projectRoot string) (domain.ProjectSummary, error) {
	if s.projects == nil {
		return domain.ProjectSummary{}, &domain.ErrServiceUnavailable{Service: "project space service", Reason: "not configured"}
	}

	spaces, err := s.projects.ListSpacesIncludingDeleted(ctx, projectRoot)
	if err != nil {
		return domain.ProjectSummary{}, err
	}

	var result domain.ProjectSummary
	totalRunIDSet := map[string]struct{}{}

	for _, space := range spaces {
		spaceID := strings.TrimSpace(space.SpaceID)
		if spaceID == "" {
			continue
		}

		// Collect run IDs from manifests.
		runIDSet := map[string]struct{}{}
		if s.manifests != nil {
			manifestRunIDs, err := s.manifests.LoadRunMembers(ctx, spaceID)
			if err != nil {
				return domain.ProjectSummary{}, err
			}
			for _, runID := range manifestRunIDs {
				runID = strings.TrimSpace(runID)
				if runID != "" {
					runIDSet[runID] = struct{}{}
				}
			}
		}

		// Collect run IDs from tasks.
		if s.tasks != nil {
			taskRunIDs, err := s.tasks.ListRunIDsBySpace(ctx, spaceID, 500)
			if err != nil {
				return domain.ProjectSummary{}, err
			}
			for _, runID := range taskRunIDs {
				runID = strings.TrimSpace(runID)
				if runID != "" {
					runIDSet[runID] = struct{}{}
				}
			}
		}

		// Aggregate cost.
		var tokens domain.TokenCost
		var taskRunCount int
		if s.costs != nil {
			tokens, taskRunCount, err = s.costs.AggregateBySpace(ctx, spaceID, "")
			if err != nil {
				return domain.ProjectSummary{}, err
			}
		}

		// Count tasks.
		var done, failed, pending int
		if s.tasks != nil {
			done, err = s.tasks.CountByStatus(ctx, spaceID, []string{"succeeded"})
			if err != nil {
				return domain.ProjectSummary{}, err
			}
			failed, err = s.tasks.CountByStatus(ctx, spaceID, []string{"failed"})
			if err != nil {
				return domain.ProjectSummary{}, err
			}
			pending, err = s.tasks.CountByStatus(ctx, spaceID, []string{"pending", "active"})
			if err != nil {
				return domain.ProjectSummary{}, err
			}
		}

		memberCount := len(runIDSet)
		if taskRunCount > memberCount {
			memberCount = taskRunCount
		}

		spaceSummary := domain.SpaceSummary{
			SpaceID:      spaceID,
			Status:       strings.TrimSpace(space.Status),
			Tokens:       tokens,
			TasksDone:    done,
			TasksFailed:  failed,
			TasksPending: pending,
			MemberCount:  memberCount,
		}

		result.Spaces = append(result.Spaces, spaceSummary)
		result.Tokens = result.Tokens.Add(tokens)
		result.TotalTasks += done + failed + pending

		// Check if space is active.
		if s.tasks != nil {
			active, err := s.tasks.CountByStatus(ctx, spaceID, []string{"active"})
			if err != nil {
				return domain.ProjectSummary{}, err
			}
			if active > 0 {
				result.ActiveSpaces++
			}
		}

		for runID := range runIDSet {
			totalRunIDSet[runID] = struct{}{}
		}
	}

	result.TotalRuns = len(totalRunIDSet)

	if result.Spaces == nil {
		result.Spaces = []domain.SpaceSummary{}
	}
	return result, nil
}
