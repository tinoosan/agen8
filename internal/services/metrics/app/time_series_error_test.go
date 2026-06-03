package app

import (
	"context"
	"errors"
	"testing"

	"github.com/tinoosan/agen8-mcp-server/internal/services/metrics/domain"
)

// TestTimeSeries_PropagatesManifestLoadRunMembersError verifies that when
// LoadRunMembers returns an error, TimeSeries propagates it instead of silently
// producing an empty time-series.
func TestTimeSeries_PropagatesManifestLoadRunMembersError(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("manifest db failure")
	svc := NewService(nil, &mockTimeSeriesQuerier{}, nil, nil, nil, &mockManifestLoader{
		loadRunMembers: func(context.Context, string) ([]string, error) {
			return nil, wantErr
		},
	}, nil)

	_, err := svc.TimeSeries(context.Background(), TimeSeriesParams{
		Scope:   domain.ScopeSpace,
		ScopeID: "space-1",
		Metric:  domain.MetricCost,
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected manifest error to propagate, got %v — collectSpaceRunIDs swallows the error", err)
	}
}

// TestTimeSeries_PropagatesListRunIDsBySpaceError verifies that when
// ListRunIDsBySpace returns an error, TimeSeries propagates it instead of
// silently returning an empty time-series.
func TestTimeSeries_PropagatesListRunIDsBySpaceError(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("task db failure")
	svc := NewService(nil, &mockTimeSeriesQuerier{}, nil, nil, &mockTaskQuerier{
		listRunIDsBySpace: func(context.Context, string, int) ([]string, error) {
			return nil, wantErr
		},
	}, nil, nil)

	_, err := svc.TimeSeries(context.Background(), TimeSeriesParams{
		Scope:   domain.ScopeSpace,
		ScopeID: "space-1",
		Metric:  domain.MetricCost,
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected task error to propagate, got %v — collectSpaceRunIDs swallows the error", err)
	}
}

// TestTimeSeries_ProjectScope_PropagatesLoadRunMembersError verifies that when
// resolveProjectRunIDs calls collectSpaceRunIDs and LoadRunMembers fails, the
// error propagates rather than being silently dropped.
func TestTimeSeries_ProjectScope_PropagatesLoadRunMembersError(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("manifest db failure")
	svc := NewService(nil, &mockTimeSeriesQuerier{}, nil, nil, nil, &mockManifestLoader{
		loadRunMembers: func(context.Context, string) ([]string, error) {
			return nil, wantErr
		},
	}, &mockProjectSpaceLister{
		listSpaces: func(context.Context, string) ([]domain.ProjectSpaceInfo, error) {
			return []domain.ProjectSpaceInfo{{SpaceID: "space-1"}}, nil
		},
	})

	_, err := svc.TimeSeries(context.Background(), TimeSeriesParams{
		Scope:   domain.ScopeProject,
		ScopeID: "/project",
		Metric:  domain.MetricCost,
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected manifest error to propagate from project scope, got %v", err)
	}
}
