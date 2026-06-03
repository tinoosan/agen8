package app

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/tinoosan/agen8-mcp-server/internal/services/metrics/domain"
)

// MemberDetail returns member-level metrics including tool usage.
func (s *Service) MemberDetail(ctx context.Context, runID string) (domain.MemberDetail, error) {
	// Load run data.
	var member string
	var tokens domain.TokenCost
	if s.runs != nil {
		info, err := s.runs.LoadRun(ctx, runID)
		if err != nil {
			return domain.MemberDetail{}, err
		}
		tokens.InputTokens = info.InputTokens
		tokens.OutputTokens = info.OutputTokens
		tokens.CostUSD = info.CostUSD
		member = strings.TrimSpace(info.Member)
		if member == "" {
			member = strings.TrimSpace(info.Goal)
		}
	}

	// Get run stats for success rate.
	var runStats domain.RunTaskStats
	if s.tasks != nil {
		var err error
		runStats, err = s.tasks.GetRunStats(ctx, runID)
		if err != nil {
			return domain.MemberDetail{}, err
		}
	}

	// Get tool usage.
	var toolUsage []domain.ToolUsage
	if s.tools != nil {
		var err error
		toolUsage, err = s.tools.QueryByRun(ctx, runID, 20)
		if err != nil {
			return domain.MemberDetail{}, err
		}
	}
	if toolUsage == nil {
		toolUsage = []domain.ToolUsage{}
	}
	sort.Slice(toolUsage, func(i, j int) bool { return toolUsage[i].Count > toolUsage[j].Count })

	// Get recent tasks.
	var doneTasks []domain.TaskInfo
	if s.tasks != nil {
		var err error
		doneTasks, err = s.tasks.ListCompletedByRun(ctx, runID, 20)
		if err != nil {
			return domain.MemberDetail{}, err
		}
	}

	recentTasks := make([]domain.RecentTask, 0, len(doneTasks))
	for _, t := range doneTasks {
		durationMs := int64(0)
		if t.CompletedAt != nil && t.CreatedAt != nil {
			durationMs = t.CompletedAt.Sub(*t.CreatedAt).Milliseconds()
		}
		completedAt := ""
		if t.CompletedAt != nil {
			completedAt = t.CompletedAt.Format(time.RFC3339)
		}
		recentTasks = append(recentTasks, domain.RecentTask{
			TaskID:      t.TaskID,
			Goal:        truncateGoal(strings.TrimSpace(t.Goal), 100),
			Status:      t.Status,
			CostUSD:     t.CostUSD,
			DurationMs:  durationMs,
			CompletedAt: completedAt,
		})
	}

	tasksDone := runStats.Succeeded
	tasksFailed := runStats.Failed
	successRate := 0.0
	total := tasksDone + tasksFailed
	if total > 0 {
		successRate = float64(tasksDone) / float64(total)
	}

	avgTaskDuration := int64(0)
	if len(doneTasks) > 0 {
		var totalDur int64
		durCount := 0
		for _, t := range doneTasks {
			if t.CompletedAt != nil && t.CreatedAt != nil {
				dur := t.CompletedAt.Sub(*t.CreatedAt).Milliseconds()
				if dur > 0 {
					totalDur += dur
					durCount++
				}
			}
		}
		if durCount > 0 {
			avgTaskDuration = totalDur / int64(durCount)
		}
	}

	return domain.MemberDetail{
		Member:            member,
		RunID:             runID,
		Tokens:            tokens,
		TasksDone:         tasksDone,
		TasksFailed:       tasksFailed,
		AvgTaskDurationMs: avgTaskDuration,
		SuccessRate:       successRate,
		ToolUsage:         toolUsage,
		RecentTasks:       recentTasks,
	}, nil
}
