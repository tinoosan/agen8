package rpc

import (
	"context"
	"testing"
	"time"

	"github.com/tinoosan/agen8-mcp-server/internal/services/events/domain"
	"github.com/tinoosan/agen8-mcp-server/pkg/protocol"
	"github.com/tinoosan/agen8-mcp-server/pkg/types"
)

type stubEventService struct {
	eventsByRun map[string][]types.EventRecord
	filters     []domain.EventFilter
}

func (s *stubEventService) Append(context.Context, types.EventRecord) error {
	return nil
}

func (s *stubEventService) ListPaginated(_ context.Context, filter domain.EventFilter) ([]types.EventRecord, int64, error) {
	s.filters = append(s.filters, filter)
	return append([]types.EventRecord(nil), s.eventsByRun[filter.RunID]...), 0, nil
}

func (s *stubEventService) Count(context.Context, domain.EventFilter) (int, error) {
	return 0, nil
}

func (s *stubEventService) LatestSeq(context.Context, string) (int64, error) {
	return 0, nil
}

type stubProjectSpaces struct {
	spaces []ProjectSpaceInfo
}

func (s stubProjectSpaces) ListSpaces(context.Context, string) ([]ProjectSpaceInfo, error) {
	return append([]ProjectSpaceInfo(nil), s.spaces...), nil
}

func TestListPaginatedProjectEventsIncludesRoleRuns(t *testing.T) {
	now := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	events := &stubEventService{eventsByRun: map[string][]types.EventRecord{
		"run-cto": {
			{EventID: "evt-1", RunID: "run-cto", Type: "run.start", Message: "Agent started", Timestamp: now.Add(-time.Minute)},
		},
		"run-backend": {
			{EventID: "evt-2", RunID: "run-backend", Type: "agent.tool.call", Message: "tool -> pending", Timestamp: now},
		},
	}}
	h := &Handler{
		Events: events,
		SpaceLister: stubProjectSpaces{spaces: []ProjectSpaceInfo{{
			SpaceID:          "space-1",
			ProjectID:        "playground",
			CoordinatorRunID: "run-cto",
			Members: []ProjectMemberInfo{
				{MemberLabel: "cto", RunID: "run-cto"},
				{MemberLabel: "backend-engineer", RunID: "run-backend"},
			},
		}}},
	}

	result, err := h.ListPaginated(context.Background(), protocol.EventsListPaginatedParams{
		ProjectRoot: "/tmp/project",
		SortDesc:    true,
	})
	if err != nil {
		t.Fatalf("ListPaginated error: %v", err)
	}
	if len(events.filters) != 2 {
		t.Fatalf("queried runs=%d want 2", len(events.filters))
	}
	if len(result.Events) != 2 {
		t.Fatalf("events=%d want 2", len(result.Events))
	}
	if got := result.Events[0].Data["role"]; got != "backend-engineer" {
		t.Fatalf("newest role=%q want backend-engineer", got)
	}
	if got := result.Events[0].Data["spaceId"]; got != "space-1" {
		t.Fatalf("spaceId=%q want space-1", got)
	}
	if got := result.Events[1].Data["role"]; got != "cto" {
		t.Fatalf("coordinator role=%q want cto", got)
	}
}

func TestListPaginatedProjectEventsFiltersBySpaceID(t *testing.T) {
	now := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	events := &stubEventService{eventsByRun: map[string][]types.EventRecord{
		"run-market-1": {
			{EventID: "evt-market-1", RunID: "run-market-1", Type: "run.start", Message: "Agent started", Timestamp: now},
		},
		"run-market-2": {
			{EventID: "evt-market-2", RunID: "run-market-2", Type: "run.start", Message: "Agent started", Timestamp: now.Add(time.Second)},
		},
		"run-eng": {
			{EventID: "evt-eng", RunID: "run-eng", Type: "run.start", Message: "Agent started", Timestamp: now.Add(2 * time.Second)},
		},
	}}
	h := &Handler{
		Events: events,
		SpaceLister: stubProjectSpaces{spaces: []ProjectSpaceInfo{
			{SpaceID: "space-market-a", ProjectID: "playground", Members: []ProjectMemberInfo{{MemberLabel: "analyst", RunID: "run-market-1"}}},
			{SpaceID: "space-market-b", ProjectID: "playground", Members: []ProjectMemberInfo{{MemberLabel: "analyst", RunID: "run-market-2"}}},
			{SpaceID: "space-eng", ProjectID: "playground", Members: []ProjectMemberInfo{{MemberLabel: "engineer", RunID: "run-eng"}}},
		}},
	}

	result, err := h.ListPaginated(context.Background(), protocol.EventsListPaginatedParams{
		ProjectRoot: "/tmp/project",
		SpaceID:     "space-market-a",
		SortDesc:    true,
	})
	if err != nil {
		t.Fatalf("ListPaginated error: %v", err)
	}
	if len(events.filters) != 1 {
		t.Fatalf("queried runs=%d want 1", len(events.filters))
	}
	if len(result.Events) != 1 {
		t.Fatalf("events=%d want 1", len(result.Events))
	}
	for _, event := range result.Events {
		if got := event.Data["spaceId"]; got != "space-market-a" {
			t.Fatalf("spaceId=%q want space-market-a", got)
		}
	}
}
