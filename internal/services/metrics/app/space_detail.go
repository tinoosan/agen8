package app

import (
	"context"
	"strings"

	"github.com/tinoosan/agen8-mcp-server/internal/services/metrics/domain"
)

// SpaceDetail returns per-member metrics for a space.
func (s *Service) SpaceDetail(ctx context.Context, spaceID string) (domain.SpaceDetail, error) {
	var members []domain.MemberSummary

	// Load manifest for member data.
	if s.manifests != nil {
		manifest, err := s.manifests.LoadManifest(ctx, spaceID)
		if err != nil {
			return domain.SpaceDetail{}, err
		}
		if manifest != nil {
			for _, member := range manifest.Members {
				memberName := strings.TrimSpace(member.MemberLabel)
				runID := strings.TrimSpace(member.RunID)
				if runID == "" {
					continue
				}

				tokens := s.aggregateCostForRun(ctx, runID)

				var memberDone, memberFailed int
				if s.tasks != nil {
					memberDone, err = s.tasks.CountByRunAndStatus(ctx, spaceID, runID, []string{"succeeded"})
					if err != nil {
						return domain.SpaceDetail{}, err
					}
					memberFailed, err = s.tasks.CountByRunAndStatus(ctx, spaceID, runID, []string{"failed"})
					if err != nil {
						return domain.SpaceDetail{}, err
					}
				}

				avgCost := 0.0
				if memberDone > 0 {
					avgCost = tokens.CostUSD / float64(memberDone)
				}

				members = append(members, domain.MemberSummary{
					Member:         memberName,
					RunID:          runID,
					Tokens:         tokens,
					TasksDone:      memberDone,
					TasksFailed:    memberFailed,
					AvgCostPerTask: avgCost,
				})
			}
		}
	}

	// Aggregate space totals.
	var totalTokens domain.TokenCost
	if s.costs != nil {
		var err error
		totalTokens, _, err = s.costs.AggregateBySpace(ctx, spaceID, "")
		if err != nil {
			return domain.SpaceDetail{}, err
		}
	}

	var done, failed, pending int
	if s.tasks != nil {
		var err error
		done, err = s.tasks.CountByStatus(ctx, spaceID, []string{"succeeded"})
		if err != nil {
			return domain.SpaceDetail{}, err
		}
		failed, err = s.tasks.CountByStatus(ctx, spaceID, []string{"failed"})
		if err != nil {
			return domain.SpaceDetail{}, err
		}
		pending, err = s.tasks.CountByStatus(ctx, spaceID, []string{"pending", "active"})
		if err != nil {
			return domain.SpaceDetail{}, err
		}
	}

	avgCost := 0.0
	if done > 0 {
		avgCost = totalTokens.CostUSD / float64(done)
	}

	// Calculate average task duration from completed tasks.
	avgDuration := int64(0)
	if s.tasks != nil {
		completedTasks, err := s.tasks.ListCompleted(ctx, spaceID, 200)
		if err != nil {
			return domain.SpaceDetail{}, err
		}
		if len(completedTasks) > 0 {
			var totalDuration int64
			durCount := 0
			for _, t := range completedTasks {
				if t.CompletedAt != nil && t.CreatedAt != nil {
					dur := t.CompletedAt.Sub(*t.CreatedAt).Milliseconds()
					if dur > 0 {
						totalDuration += dur
						durCount++
					}
				}
			}
			if durCount > 0 {
				avgDuration = totalDuration / int64(durCount)
			}
		}
	}

	if members == nil {
		members = []domain.MemberSummary{}
	}

	return domain.SpaceDetail{
		SpaceID:        spaceID,
		Tokens:         totalTokens,
		TasksDone:      done,
		TasksFailed:    failed,
		TasksPending:   pending,
		AvgCostPerTask: avgCost,
		AvgDurationMs:  avgDuration,
		Members:        members,
	}, nil
}
