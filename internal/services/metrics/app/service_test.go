package app

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/tinoosan/agen8-mcp-server/internal/services/metrics/domain"
)

// ── mocks ───────────────────────────────────────────

type mockCostAggregator struct {
	aggregateBySpace func(ctx context.Context, spaceID, member string) (domain.TokenCost, int, error)
}

func (m *mockCostAggregator) AggregateBySpace(ctx context.Context, spaceID, member string) (domain.TokenCost, int, error) {
	if m.aggregateBySpace != nil {
		return m.aggregateBySpace(ctx, spaceID, member)
	}
	return domain.TokenCost{}, 0, nil
}

type mockRunLoader struct {
	loadRun func(ctx context.Context, runID string) (domain.RunInfo, error)
}

func (m *mockRunLoader) LoadRun(ctx context.Context, runID string) (domain.RunInfo, error) {
	if m.loadRun != nil {
		return m.loadRun(ctx, runID)
	}
	return domain.RunInfo{}, nil
}

type mockTaskQuerier struct {
	countByStatus       func(ctx context.Context, spaceID string, statuses []string) (int, error)
	countByRunAndStatus func(ctx context.Context, spaceID, runID string, statuses []string) (int, error)
	listCompleted       func(ctx context.Context, spaceID string, limit int) ([]domain.TaskInfo, error)
	listCompletedByRun  func(ctx context.Context, runID string, limit int) ([]domain.TaskInfo, error)
	getRunStats         func(ctx context.Context, runID string) (domain.RunTaskStats, error)
	listRunIDsBySpace   func(ctx context.Context, spaceID string, limit int) ([]string, error)
}

func (m *mockTaskQuerier) CountByStatus(ctx context.Context, spaceID string, statuses []string) (int, error) {
	if m.countByStatus != nil {
		return m.countByStatus(ctx, spaceID, statuses)
	}
	return 0, nil
}
func (m *mockTaskQuerier) CountByRunAndStatus(ctx context.Context, spaceID, runID string, statuses []string) (int, error) {
	if m.countByRunAndStatus != nil {
		return m.countByRunAndStatus(ctx, spaceID, runID, statuses)
	}
	return 0, nil
}
func (m *mockTaskQuerier) ListCompleted(ctx context.Context, spaceID string, limit int) ([]domain.TaskInfo, error) {
	if m.listCompleted != nil {
		return m.listCompleted(ctx, spaceID, limit)
	}
	return nil, nil
}
func (m *mockTaskQuerier) ListCompletedByRun(ctx context.Context, runID string, limit int) ([]domain.TaskInfo, error) {
	if m.listCompletedByRun != nil {
		return m.listCompletedByRun(ctx, runID, limit)
	}
	return nil, nil
}
func (m *mockTaskQuerier) GetRunStats(ctx context.Context, runID string) (domain.RunTaskStats, error) {
	if m.getRunStats != nil {
		return m.getRunStats(ctx, runID)
	}
	return domain.RunTaskStats{}, nil
}
func (m *mockTaskQuerier) ListRunIDsBySpace(ctx context.Context, spaceID string, limit int) ([]string, error) {
	if m.listRunIDsBySpace != nil {
		return m.listRunIDsBySpace(ctx, spaceID, limit)
	}
	return nil, nil
}

type mockManifestLoader struct {
	loadRunMembers func(ctx context.Context, spaceID string) ([]string, error)
	loadManifest   func(ctx context.Context, spaceID string) (*domain.ManifestInfo, error)
}

func (m *mockManifestLoader) LoadRunMembers(ctx context.Context, spaceID string) ([]string, error) {
	if m.loadRunMembers != nil {
		return m.loadRunMembers(ctx, spaceID)
	}
	return nil, nil
}
func (m *mockManifestLoader) LoadManifest(ctx context.Context, spaceID string) (*domain.ManifestInfo, error) {
	if m.loadManifest != nil {
		return m.loadManifest(ctx, spaceID)
	}
	return nil, nil
}

type mockProjectSpaceLister struct {
	listSpacesIncludingDeleted func(ctx context.Context, projectRoot string) ([]domain.ProjectSpaceInfo, error)
	listSpaces                 func(ctx context.Context, projectRoot string) ([]domain.ProjectSpaceInfo, error)
}

func (m *mockProjectSpaceLister) ListSpacesIncludingDeleted(ctx context.Context, projectRoot string) ([]domain.ProjectSpaceInfo, error) {
	if m.listSpacesIncludingDeleted != nil {
		return m.listSpacesIncludingDeleted(ctx, projectRoot)
	}
	return nil, nil
}
func (m *mockProjectSpaceLister) ListSpaces(ctx context.Context, projectRoot string) ([]domain.ProjectSpaceInfo, error) {
	if m.listSpaces != nil {
		return m.listSpaces(ctx, projectRoot)
	}
	return nil, nil
}

type mockTimeSeriesQuerier struct {
	query func(ctx context.Context, runIDs []string, metric domain.Metric, granularity domain.Granularity, tr domain.TimeRange) (domain.TimeSeries, error)
}

func (m *mockTimeSeriesQuerier) Query(ctx context.Context, runIDs []string, metric domain.Metric, granularity domain.Granularity, tr domain.TimeRange) (domain.TimeSeries, error) {
	if m.query != nil {
		return m.query(ctx, runIDs, metric, granularity, tr)
	}
	return domain.TimeSeries{Points: []domain.TimeSeriesPoint{}}, nil
}

type mockToolUsageQuerier struct {
	queryByRun func(ctx context.Context, runID string, limit int) ([]domain.ToolUsage, error)
}

func (m *mockToolUsageQuerier) QueryByRun(ctx context.Context, runID string, limit int) ([]domain.ToolUsage, error) {
	if m.queryByRun != nil {
		return m.queryByRun(ctx, runID, limit)
	}
	return nil, nil
}

// ── tests ───────────────────────────────────────────

func TestProjectSummary_AggregatesSpaces(t *testing.T) {
	ctx := context.Background()

	projects := &mockProjectSpaceLister{
		listSpacesIncludingDeleted: func(_ context.Context, _ string) ([]domain.ProjectSpaceInfo, error) {
			return []domain.ProjectSpaceInfo{
				{SpaceID: "t1", Status: "active"},
				{SpaceID: "t2", Status: "deleted"},
			}, nil
		},
	}
	costs := &mockCostAggregator{
		aggregateBySpace: func(_ context.Context, spaceID, _ string) (domain.TokenCost, int, error) {
			if spaceID == "t1" {
				return domain.TokenCost{InputTokens: 100, OutputTokens: 50, CostUSD: 1.0}, 2, nil
			}
			return domain.TokenCost{InputTokens: 200, OutputTokens: 100, CostUSD: 2.0}, 1, nil
		},
	}
	tasks := &mockTaskQuerier{
		countByStatus: func(_ context.Context, spaceID string, statuses []string) (int, error) {
			if spaceID == "t1" && statuses[0] == "succeeded" {
				return 3, nil
			}
			return 0, nil
		},
	}

	svc := NewService(costs, nil, nil, nil, tasks, nil, projects)
	result, err := svc.ProjectSummary(ctx, "/project")
	if err != nil {
		t.Fatalf("ProjectSummary: %v", err)
	}

	if len(result.Spaces) != 2 {
		t.Fatalf("expected 2 spaces, got %d", len(result.Spaces))
	}
	if result.Tokens.CostUSD != 3.0 {
		t.Fatalf("expected total cost 3.0, got %f", result.Tokens.CostUSD)
	}
	if result.Tokens.InputTokens != 300 {
		t.Fatalf("expected 300 input tokens, got %d", result.Tokens.InputTokens)
	}
	if result.TotalTasks != 3 {
		t.Fatalf("expected 3 total tasks, got %d", result.TotalTasks)
	}
}

func TestProjectSummary_NoSpaces(t *testing.T) {
	ctx := context.Background()
	projects := &mockProjectSpaceLister{
		listSpacesIncludingDeleted: func(_ context.Context, _ string) ([]domain.ProjectSpaceInfo, error) {
			return nil, nil
		},
	}
	svc := NewService(nil, nil, nil, nil, nil, nil, projects)
	result, err := svc.ProjectSummary(ctx, "/project")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Spaces) != 0 {
		t.Fatalf("expected empty spaces, got %d", len(result.Spaces))
	}
}

func TestAggregateCostForRun_FallsBackToTaskStats(t *testing.T) {
	ctx := context.Background()

	runs := &mockRunLoader{
		loadRun: func(_ context.Context, _ string) (domain.RunInfo, error) {
			return domain.RunInfo{InputTokens: 100, OutputTokens: 50, CostUSD: 0}, nil
		},
	}
	tasks := &mockTaskQuerier{
		getRunStats: func(_ context.Context, _ string) (domain.RunTaskStats, error) {
			return domain.RunTaskStats{TotalCost: 1.5}, nil
		},
	}

	svc := NewService(nil, nil, nil, runs, tasks, nil, nil)
	tc := svc.aggregateCostForRun(ctx, "run-1")

	if tc.InputTokens != 100 || tc.OutputTokens != 50 {
		t.Fatalf("expected tokens 100/50, got %d/%d", tc.InputTokens, tc.OutputTokens)
	}
	if tc.CostUSD != 1.5 {
		t.Fatalf("expected cost 1.5 from task stats fallback, got %f", tc.CostUSD)
	}
}

func TestAggregateCostForRun_UsesRunCostWhenNonZero(t *testing.T) {
	ctx := context.Background()

	runs := &mockRunLoader{
		loadRun: func(_ context.Context, _ string) (domain.RunInfo, error) {
			return domain.RunInfo{InputTokens: 100, OutputTokens: 50, CostUSD: 2.0}, nil
		},
	}
	tasks := &mockTaskQuerier{
		getRunStats: func(_ context.Context, _ string) (domain.RunTaskStats, error) {
			t.Fatal("GetRunStats should not be called when run cost is non-zero")
			return domain.RunTaskStats{}, nil
		},
	}

	svc := NewService(nil, nil, nil, runs, tasks, nil, nil)
	tc := svc.aggregateCostForRun(ctx, "run-1")

	if tc.CostUSD != 2.0 {
		t.Fatalf("expected cost 2.0 from run, got %f", tc.CostUSD)
	}
}

func TestMemberDetail_SuccessRate(t *testing.T) {
	ctx := context.Background()

	runs := &mockRunLoader{
		loadRun: func(_ context.Context, _ string) (domain.RunInfo, error) {
			return domain.RunInfo{RunID: "r1", Member: "worker", InputTokens: 10, OutputTokens: 5, CostUSD: 0.5}, nil
		},
	}
	tasks := &mockTaskQuerier{
		getRunStats: func(_ context.Context, _ string) (domain.RunTaskStats, error) {
			return domain.RunTaskStats{Succeeded: 8, Failed: 2}, nil
		},
	}

	svc := NewService(nil, nil, nil, runs, tasks, nil, nil)
	result, err := svc.MemberDetail(ctx, "r1")
	if err != nil {
		t.Fatalf("MemberDetail: %v", err)
	}

	if result.SuccessRate != 0.8 {
		t.Fatalf("expected success rate 0.8, got %f", result.SuccessRate)
	}
	if result.Member != "worker" {
		t.Fatalf("expected member=worker, got %q", result.Member)
	}
}

func TestMemberDetail_GoalTruncation(t *testing.T) {
	ctx := context.Background()

	longGoal := ""
	for i := 0; i < 120; i++ {
		longGoal += "x"
	}

	now := time.Now()
	tasks := &mockTaskQuerier{
		listCompletedByRun: func(_ context.Context, _ string, _ int) ([]domain.TaskInfo, error) {
			return []domain.TaskInfo{
				{TaskID: "t1", Goal: longGoal, Status: "succeeded", CreatedAt: &now, CompletedAt: &now},
			}, nil
		},
	}

	svc := NewService(nil, nil, nil, nil, tasks, nil, nil)
	result, err := svc.MemberDetail(ctx, "r1")
	if err != nil {
		t.Fatalf("MemberDetail: %v", err)
	}

	if len(result.RecentTasks) != 1 {
		t.Fatalf("expected 1 recent task, got %d", len(result.RecentTasks))
	}
	if len(result.RecentTasks[0].Goal) != 100 {
		t.Fatalf("expected goal truncated to 100, got %d", len(result.RecentTasks[0].Goal))
	}
}

func TestTimeSeries_ResolveRunIDs_ByScope(t *testing.T) {
	ctx := context.Background()

	manifests := &mockManifestLoader{
		loadRunMembers: func(_ context.Context, spaceID string) ([]string, error) {
			return []string{"run-" + spaceID}, nil
		},
	}
	tasks := &mockTaskQuerier{
		listRunIDsBySpace: func(_ context.Context, spaceID string, _ int) ([]string, error) {
			return []string{"task-run-" + spaceID}, nil
		},
	}

	t.Run("member scope returns scopeID directly", func(t *testing.T) {
		svc := NewService(nil, &mockTimeSeriesQuerier{}, nil, nil, tasks, manifests, nil)
		var receivedIDs []string
		svc.timeseries = &mockTimeSeriesQuerier{
			query: func(_ context.Context, runIDs []string, _ domain.Metric, _ domain.Granularity, _ domain.TimeRange) (domain.TimeSeries, error) {
				receivedIDs = runIDs
				return domain.TimeSeries{Points: []domain.TimeSeriesPoint{}}, nil
			},
		}
		_, err := svc.TimeSeries(ctx, TimeSeriesParams{
			Scope:   domain.ScopeMember,
			ScopeID: "my-run",
			Metric:  domain.MetricCost,
		})
		if err != nil {
			t.Fatalf("TimeSeries: %v", err)
		}
		if len(receivedIDs) != 1 || receivedIDs[0] != "my-run" {
			t.Fatalf("expected [my-run], got %v", receivedIDs)
		}
	})

	t.Run("unsupported scope returns error", func(t *testing.T) {
		svc := NewService(nil, &mockTimeSeriesQuerier{}, nil, nil, tasks, manifests, nil)
		_, err := svc.TimeSeries(ctx, TimeSeriesParams{
			Scope:   "unknown",
			ScopeID: "x",
			Metric:  domain.MetricCost,
		})
		if err == nil {
			t.Fatal("expected error for unsupported scope")
		}
		var invalidInput *domain.ErrInvalidInput
		if !isInvalidInput(err, &invalidInput) {
			t.Fatalf("expected ErrInvalidInput, got %T: %v", err, err)
		}
	})
}

func TestSpaceDetail_RoleBreakdown(t *testing.T) {
	ctx := context.Background()

	manifests := &mockManifestLoader{
		loadManifest: func(_ context.Context, _ string) (*domain.ManifestInfo, error) {
			return &domain.ManifestInfo{
				Members: []domain.ManifestMember{
					{MemberLabel: "coordinator", RunID: "run-1"},
					{MemberLabel: "worker", RunID: "run-2"},
				},
			}, nil
		},
	}
	runs := &mockRunLoader{
		loadRun: func(_ context.Context, runID string) (domain.RunInfo, error) {
			if runID == "run-1" {
				return domain.RunInfo{InputTokens: 50, OutputTokens: 20, CostUSD: 0.5}, nil
			}
			return domain.RunInfo{InputTokens: 100, OutputTokens: 40, CostUSD: 1.0}, nil
		},
	}
	tasks := &mockTaskQuerier{
		countByRunAndStatus: func(_ context.Context, _, runID string, statuses []string) (int, error) {
			if runID == "run-1" && statuses[0] == "succeeded" {
				return 2, nil
			}
			if runID == "run-2" && statuses[0] == "succeeded" {
				return 5, nil
			}
			return 0, nil
		},
		countByStatus: func(_ context.Context, _ string, statuses []string) (int, error) {
			if statuses[0] == "succeeded" {
				return 7, nil
			}
			return 0, nil
		},
	}
	costs := &mockCostAggregator{
		aggregateBySpace: func(_ context.Context, _, _ string) (domain.TokenCost, int, error) {
			return domain.TokenCost{InputTokens: 150, OutputTokens: 60, CostUSD: 1.5}, 2, nil
		},
	}

	svc := NewService(costs, nil, nil, runs, tasks, manifests, nil)
	result, err := svc.SpaceDetail(ctx, "space-1")
	if err != nil {
		t.Fatalf("SpaceDetail: %v", err)
	}

	if result.SpaceID != "space-1" {
		t.Fatalf("expected spaceID=space-1, got %q", result.SpaceID)
	}
	if len(result.Members) != 2 {
		t.Fatalf("expected 2 members, got %d", len(result.Members))
	}
	if result.Members[0].Member != "coordinator" || result.Members[0].Tokens.CostUSD != 0.5 {
		t.Fatalf("member[0] = %+v", result.Members[0])
	}
	if result.Members[1].Member != "worker" || result.Members[1].TasksDone != 5 {
		t.Fatalf("member[1] = %+v", result.Members[1])
	}
	if result.TasksDone != 7 {
		t.Fatalf("expected total tasks done=7, got %d", result.TasksDone)
	}
}

func TestProjectSummary_MemberCount_MaxOfRunIDsAndDistinctRuns(t *testing.T) {
	ctx := context.Background()

	projects := &mockProjectSpaceLister{
		listSpacesIncludingDeleted: func(_ context.Context, _ string) ([]domain.ProjectSpaceInfo, error) {
			return []domain.ProjectSpaceInfo{{SpaceID: "t1"}}, nil
		},
	}
	manifests := &mockManifestLoader{
		loadRunMembers: func(_ context.Context, _ string) ([]string, error) {
			return []string{"r1", "r2"}, nil // 2 from manifests
		},
	}
	tasks := &mockTaskQuerier{
		listRunIDsBySpace: func(_ context.Context, _ string, _ int) ([]string, error) {
			return []string{"r1", "r3"}, nil // r3 is new, r1 is duplicate
		},
	}
	costs := &mockCostAggregator{
		aggregateBySpace: func(_ context.Context, _, _ string) (domain.TokenCost, int, error) {
			return domain.TokenCost{}, 5, nil // 5 distinct runs from cost query
		},
	}

	svc := NewService(costs, nil, nil, nil, tasks, manifests, projects)
	result, err := svc.ProjectSummary(ctx, "/project")
	if err != nil {
		t.Fatalf("ProjectSummary: %v", err)
	}

	// runIDSet = {r1, r2, r3} = 3, taskRunCount = 5 → max = 5
	if result.Spaces[0].MemberCount != 5 {
		t.Fatalf("expected MemberCount=5 (max of 3 runIDs, 5 distinct runs), got %d", result.Spaces[0].MemberCount)
	}
}

func TestProjectSummary_KeepsSpacesSeparateBySpaceID(t *testing.T) {
	ctx := context.Background()

	projects := &mockProjectSpaceLister{
		listSpacesIncludingDeleted: func(_ context.Context, _ string) ([]domain.ProjectSpaceInfo, error) {
			return []domain.ProjectSpaceInfo{
				{SpaceID: "old-id", Status: "deleted"},
				{SpaceID: "new-id", Status: "active"},
			}, nil
		},
	}
	costs := &mockCostAggregator{
		aggregateBySpace: func(_ context.Context, spaceID, _ string) (domain.TokenCost, int, error) {
			if spaceID == "old-id" {
				return domain.TokenCost{InputTokens: 100, OutputTokens: 50, CostUSD: 1.0}, 1, nil
			}
			return domain.TokenCost{InputTokens: 200, OutputTokens: 100, CostUSD: 2.0}, 2, nil
		},
	}
	tasks := &mockTaskQuerier{
		countByStatus: func(_ context.Context, spaceID string, statuses []string) (int, error) {
			if spaceID == "old-id" && statuses[0] == "succeeded" {
				return 3, nil
			}
			if spaceID == "new-id" && statuses[0] == "succeeded" {
				return 5, nil
			}
			return 0, nil
		},
	}

	svc := NewService(costs, nil, nil, nil, tasks, nil, projects)
	result, err := svc.ProjectSummary(ctx, "/project")
	if err != nil {
		t.Fatalf("ProjectSummary: %v", err)
	}

	if len(result.Spaces) != 2 {
		t.Fatalf("expected 2 spaces, got %d: %+v", len(result.Spaces), result.Spaces)
	}
	byID := map[string]domain.SpaceSummary{}
	for _, space := range result.Spaces {
		byID[space.SpaceID] = space
	}
	oldSpace, ok := byID["old-id"]
	if !ok {
		t.Fatalf("expected old-id space in summary: %+v", result.Spaces)
	}
	if oldSpace.Tokens.CostUSD != 1.0 || oldSpace.TasksDone != 3 || oldSpace.Status != "deleted" {
		t.Fatalf("old-id summary = %+v", oldSpace)
	}
	newSpace, ok := byID["new-id"]
	if !ok {
		t.Fatalf("expected new-id space in summary: %+v", result.Spaces)
	}
	if newSpace.Tokens.CostUSD != 2.0 || newSpace.TasksDone != 5 || newSpace.Status != "active" {
		t.Fatalf("new-id summary = %+v", newSpace)
	}
}

func TestSpaceDetail_PropagatesAggregateBySpaceError(t *testing.T) {
	ctx := context.Background()
	wantErr := errors.New("aggregate failed")
	svc := NewService(&mockCostAggregator{
		aggregateBySpace: func(context.Context, string, string) (domain.TokenCost, int, error) {
			return domain.TokenCost{}, 0, wantErr
		},
	}, nil, nil, nil, nil, nil, nil)

	_, err := svc.SpaceDetail(ctx, "space-1")
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected aggregate error, got %v", err)
	}
}

func TestSpaceDetail_PropagatesCountByStatusError(t *testing.T) {
	ctx := context.Background()
	wantErr := errors.New("count failed")
	svc := NewService(nil, nil, nil, nil, &mockTaskQuerier{
		countByStatus: func(context.Context, string, []string) (int, error) {
			return 0, wantErr
		},
	}, nil, nil)

	_, err := svc.SpaceDetail(ctx, "space-1")
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected count error, got %v", err)
	}
}

func TestSpaceDetail_PropagatesListCompletedError(t *testing.T) {
	ctx := context.Background()
	wantErr := errors.New("list completed failed")
	svc := NewService(nil, nil, nil, nil, &mockTaskQuerier{
		countByStatus: func(context.Context, string, []string) (int, error) { return 0, nil },
		listCompleted: func(context.Context, string, int) ([]domain.TaskInfo, error) {
			return nil, wantErr
		},
	}, nil, nil)

	_, err := svc.SpaceDetail(ctx, "space-1")
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected list completed error, got %v", err)
	}
}

func TestSpaceDetail_PropagatesCountByRunAndStatusError(t *testing.T) {
	ctx := context.Background()
	wantErr := errors.New("member count failed")
	svc := NewService(nil, nil, nil, nil, &mockTaskQuerier{
		countByRunAndStatus: func(context.Context, string, string, []string) (int, error) {
			return 0, wantErr
		},
	}, &mockManifestLoader{
		loadManifest: func(context.Context, string) (*domain.ManifestInfo, error) {
			return &domain.ManifestInfo{
				Members: []domain.ManifestMember{{MemberLabel: "worker", RunID: "run-1"}},
			}, nil
		},
	}, nil)

	_, err := svc.SpaceDetail(ctx, "space-1")
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected member count error, got %v", err)
	}
}

func TestMemberDetail_PropagatesGetRunStatsError(t *testing.T) {
	ctx := context.Background()
	wantErr := errors.New("run stats failed")
	svc := NewService(nil, nil, nil, nil, &mockTaskQuerier{
		getRunStats: func(context.Context, string) (domain.RunTaskStats, error) {
			return domain.RunTaskStats{}, wantErr
		},
	}, nil, nil)

	_, err := svc.MemberDetail(ctx, "run-1")
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected run stats error, got %v", err)
	}
}

func TestMemberDetail_PropagatesToolQueryError(t *testing.T) {
	ctx := context.Background()
	wantErr := errors.New("tool query failed")
	svc := NewService(nil, nil, &mockToolUsageQuerier{
		queryByRun: func(context.Context, string, int) ([]domain.ToolUsage, error) {
			return nil, wantErr
		},
	}, nil, nil, nil, nil)

	_, err := svc.MemberDetail(ctx, "run-1")
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected tool query error, got %v", err)
	}
}

func TestMemberDetail_PropagatesListCompletedByRunError(t *testing.T) {
	ctx := context.Background()
	wantErr := errors.New("completed by run failed")
	svc := NewService(nil, nil, nil, nil, &mockTaskQuerier{
		getRunStats: func(context.Context, string) (domain.RunTaskStats, error) { return domain.RunTaskStats{}, nil },
		listCompletedByRun: func(context.Context, string, int) ([]domain.TaskInfo, error) {
			return nil, wantErr
		},
	}, nil, nil)

	_, err := svc.MemberDetail(ctx, "run-1")
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected list completed by run error, got %v", err)
	}
}

func TestProjectSummary_PropagatesLoadRunMembersError(t *testing.T) {
	ctx := context.Background()
	wantErr := errors.New("load run members failed")
	svc := NewService(nil, nil, nil, nil, nil, &mockManifestLoader{
		loadRunMembers: func(context.Context, string) ([]string, error) { return nil, wantErr },
	}, &mockProjectSpaceLister{
		listSpacesIncludingDeleted: func(context.Context, string) ([]domain.ProjectSpaceInfo, error) {
			return []domain.ProjectSpaceInfo{{SpaceID: "space-1"}}, nil
		},
	})

	_, err := svc.ProjectSummary(ctx, "/project")
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected load run members error, got %v", err)
	}
}

func TestProjectSummary_PropagatesListRunIDsBySpaceError(t *testing.T) {
	ctx := context.Background()
	wantErr := errors.New("list run ids failed")
	svc := NewService(nil, nil, nil, nil, &mockTaskQuerier{
		listRunIDsBySpace: func(context.Context, string, int) ([]string, error) { return nil, wantErr },
	}, nil, &mockProjectSpaceLister{
		listSpacesIncludingDeleted: func(context.Context, string) ([]domain.ProjectSpaceInfo, error) {
			return []domain.ProjectSpaceInfo{{SpaceID: "space-1"}}, nil
		},
	})

	_, err := svc.ProjectSummary(ctx, "/project")
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected list run ids error, got %v", err)
	}
}

func TestProjectSummary_PropagatesAggregateBySpaceError(t *testing.T) {
	ctx := context.Background()
	wantErr := errors.New("aggregate failed")
	svc := NewService(&mockCostAggregator{
		aggregateBySpace: func(context.Context, string, string) (domain.TokenCost, int, error) {
			return domain.TokenCost{}, 0, wantErr
		},
	}, nil, nil, nil, nil, nil, &mockProjectSpaceLister{
		listSpacesIncludingDeleted: func(context.Context, string) ([]domain.ProjectSpaceInfo, error) {
			return []domain.ProjectSpaceInfo{{SpaceID: "space-1"}}, nil
		},
	})

	_, err := svc.ProjectSummary(ctx, "/project")
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected aggregate error, got %v", err)
	}
}

func TestProjectSummary_PropagatesCountByStatusError(t *testing.T) {
	ctx := context.Background()
	wantErr := errors.New("count failed")
	svc := NewService(nil, nil, nil, nil, &mockTaskQuerier{
		countByStatus: func(context.Context, string, []string) (int, error) { return 0, wantErr },
	}, nil, &mockProjectSpaceLister{
		listSpacesIncludingDeleted: func(context.Context, string) ([]domain.ProjectSpaceInfo, error) {
			return []domain.ProjectSpaceInfo{{SpaceID: "space-1"}}, nil
		},
	})

	_, err := svc.ProjectSummary(ctx, "/project")
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected count error, got %v", err)
	}
}

func isInvalidInput(err error, target **domain.ErrInvalidInput) bool {
	e, ok := err.(*domain.ErrInvalidInput)
	if ok {
		*target = e
		return true
	}
	// Check wrapped
	return fmt.Errorf("%w", err) != nil && ok
}
