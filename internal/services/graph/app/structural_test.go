package app

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/tinoosan/agen8/internal/services/graph/domain"
)

type stubDecisionLister struct {
	decisions []DecisionHydrationRow
}

func (s stubDecisionLister) GetDecision(_ context.Context, decisionID string) (DecisionHydrationRow, error) {
	for _, d := range s.decisions {
		if d.ID == decisionID {
			return d, nil
		}
	}
	return DecisionHydrationRow{}, fmt.Errorf("decision not found")
}

func (s stubDecisionLister) ListDecisions(_ context.Context, projectID string, _ int) ([]DecisionHydrationRow, error) {
	if projectID == "" {
		return append([]DecisionHydrationRow(nil), s.decisions...), nil
	}
	out := make([]DecisionHydrationRow, 0, len(s.decisions))
	for _, d := range s.decisions {
		if d.ProjectID == projectID {
			out = append(out, d)
		}
	}
	return out, nil
}

type stubKRLister struct {
	byMission map[string][]KeyResultHydrationRow
}

func (s stubKRLister) GetMission(context.Context, string) (MissionHydrationRow, error) {
	return MissionHydrationRow{}, fmt.Errorf("mission not found")
}

func (s stubKRLister) ListMissions(context.Context, string, int) ([]MissionHydrationRow, error) {
	return nil, nil
}

func (s stubKRLister) GetKeyResult(context.Context, string) (KeyResultHydrationRow, error) {
	return KeyResultHydrationRow{}, fmt.Errorf("key result not found")
}

func (s stubKRLister) ListKeyResults(_ context.Context, missionID string) ([]KeyResultHydrationRow, error) {
	return s.byMission[missionID], nil
}

func hasEdge(edges []domain.GraphEdge, sourceType, sourceID, targetType, targetID, edgeType string) bool {
	for _, e := range edges {
		if e.SourceType == sourceType && e.SourceID == sourceID &&
			e.TargetType == targetType && e.TargetID == targetID &&
			e.EdgeType == edgeType {
			return true
		}
	}
	return false
}

// A task linked directly to a mission with no key result must produce a
// task->mission structural edge. This is the reported bug: the mission was
// invisible because nothing materialized this ref as an edge.
func TestStructuralResolver_TaskWithMissionRefNoKR(t *testing.T) {
	resolver := NewStructuralResolver(stubTaskReader{}, stubDecisionLister{}, stubKRLister{})
	core := domain.GraphNodeCore{
		Type: domain.NodeTypeTask,
		ID:   "task-1",
		Fields: map[string]any{
			"missionRef": "mission-1",
		},
	}
	edges, err := resolver.Edges(context.Background(), "project-a", core)
	if err != nil {
		t.Fatalf("Edges: %v", err)
	}
	if !hasEdge(edges, domain.NodeTypeTask, "task-1", domain.NodeTypeMission, "mission-1", domain.EdgeTypeServes) {
		t.Fatalf("expected task->mission serves edge; got %+v", edges)
	}
}

// When a task has a key result, the structural parent is the key result, not the
// mission — the mission is reached through the KR, so no task->mission edge.
func TestStructuralResolver_TaskPrefersKeyResultOverMission(t *testing.T) {
	resolver := NewStructuralResolver(stubTaskReader{}, stubDecisionLister{}, stubKRLister{})
	core := domain.GraphNodeCore{
		Type: domain.NodeTypeTask,
		ID:   "task-1",
		Fields: map[string]any{
			"keyResultRef": "kr-1",
			"missionRef":   "mission-1",
		},
	}
	edges, err := resolver.Edges(context.Background(), "project-a", core)
	if err != nil {
		t.Fatalf("Edges: %v", err)
	}
	if !hasEdge(edges, domain.NodeTypeTask, "task-1", domain.NodeTypeKeyResult, "kr-1", domain.EdgeTypeServes) {
		t.Fatalf("expected task->kr serves edge; got %+v", edges)
	}
	if hasEdge(edges, domain.NodeTypeTask, "task-1", domain.NodeTypeMission, "mission-1", domain.EdgeTypeServes) {
		t.Fatalf("did not expect a redundant task->mission edge; got %+v", edges)
	}
}

// A decision attaches to its most-specific parent: the task when it has a
// taskRef, even if it also carries a missionRef.
func TestStructuralResolver_DecisionPrefersTask(t *testing.T) {
	resolver := NewStructuralResolver(stubTaskReader{}, stubDecisionLister{}, stubKRLister{})
	core := domain.GraphNodeCore{
		Type: domain.NodeTypeDecision,
		ID:   "dec-1",
		Fields: map[string]any{
			"taskRef":    "task-1",
			"missionRef": "mission-1",
		},
	}
	edges, err := resolver.Edges(context.Background(), "project-a", core)
	if err != nil {
		t.Fatalf("Edges: %v", err)
	}
	if !hasEdge(edges, domain.NodeTypeDecision, "dec-1", domain.NodeTypeTask, "task-1", domain.EdgeTypeMadeDuring) {
		t.Fatalf("expected decision->task made_during edge; got %+v", edges)
	}
	if hasEdge(edges, domain.NodeTypeDecision, "dec-1", domain.NodeTypeMission, "mission-1", domain.EdgeTypeServes) {
		t.Fatalf("did not expect decision->mission edge when task is present; got %+v", edges)
	}
}

// Opening a mission must surface its direct children: its key results, any
// KR-less tasks linked straight to it, and any orphan decisions — but NOT a
// decision that belongs to one of the mission's tasks (that decision attaches to
// the task, one hop deeper).
func TestStructuralResolver_MissionDownwardChildren(t *testing.T) {
	now := time.Now().UTC()
	krLister := stubKRLister{byMission: map[string][]KeyResultHydrationRow{
		"mission-1": {{ID: "kr-1", MissionID: "mission-1"}},
	}}
	taskReader := stubTaskReader{
		tasks: []TaskHydrationRow{
			{ID: "task-direct", ProjectID: "project-a", Metadata: map[string]any{"missionRef": "mission-1"}, CreatedAt: &now},
			{ID: "task-under-kr", ProjectID: "project-a", KeyResultRef: "kr-1", CreatedAt: &now},
		},
	}
	decLister := stubDecisionLister{decisions: []DecisionHydrationRow{
		{ID: "dec-orphan", ProjectID: "project-a", MissionRef: "mission-1"},
		{ID: "dec-on-task", ProjectID: "project-a", TaskRef: "task-direct", MissionRef: "mission-1"},
	}}
	resolver := NewStructuralResolver(taskReader, decLister, krLister)

	core := domain.GraphNodeCore{Type: domain.NodeTypeMission, ID: "mission-1"}
	edges, err := resolver.Edges(context.Background(), "project-a", core)
	if err != nil {
		t.Fatalf("Edges: %v", err)
	}
	if !hasEdge(edges, domain.NodeTypeKeyResult, "kr-1", domain.NodeTypeMission, "mission-1", domain.EdgeTypeServes) {
		t.Fatalf("expected kr->mission edge; got %+v", edges)
	}
	if !hasEdge(edges, domain.NodeTypeTask, "task-direct", domain.NodeTypeMission, "mission-1", domain.EdgeTypeServes) {
		t.Fatalf("expected KR-less task->mission edge; got %+v", edges)
	}
	if hasEdge(edges, domain.NodeTypeTask, "task-under-kr", domain.NodeTypeMission, "mission-1", domain.EdgeTypeServes) {
		t.Fatalf("did not expect a task with a KR to attach to the mission; got %+v", edges)
	}
	if !hasEdge(edges, domain.NodeTypeDecision, "dec-orphan", domain.NodeTypeMission, "mission-1", domain.EdgeTypeServes) {
		t.Fatalf("expected orphan decision->mission edge; got %+v", edges)
	}
	if hasEdge(edges, domain.NodeTypeDecision, "dec-on-task", domain.NodeTypeMission, "mission-1", domain.EdgeTypeServes) {
		t.Fatalf("did not expect a task's decision to attach to the mission; got %+v", edges)
	}
}
