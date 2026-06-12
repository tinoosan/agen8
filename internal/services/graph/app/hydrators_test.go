package app

import (
	"context"
	"fmt"
	"testing"
	"time"
)

type stubTaskReader struct {
	byID  map[string]TaskHydrationRow
	tasks []TaskHydrationRow
}

func (s stubTaskReader) GetTask(_ context.Context, taskID string) (TaskHydrationRow, error) {
	if task, ok := s.byID[taskID]; ok {
		return task, nil
	}
	return TaskHydrationRow{}, fmt.Errorf("task not found")
}

func (s stubTaskReader) ListTasks(_ context.Context, projectID string, _ int) ([]TaskHydrationRow, error) {
	if projectID == "" {
		return append([]TaskHydrationRow(nil), s.tasks...), nil
	}
	out := make([]TaskHydrationRow, 0, len(s.tasks))
	for _, task := range s.tasks {
		if task.ProjectID == projectID {
			out = append(out, task)
		}
	}
	return out, nil
}

func TestTaskHydrator_FetchCoordinatorScopeCanReadCrossProjectTask(t *testing.T) {
	now := time.Now().UTC()
	other := TaskHydrationRow{
		ID:          "task-project-b",
		ProjectID:   "project-b",
		Description: "Investigate prior work",
		Title:       "Investigate prior work",
		Status:      "pending",
		CreatedAt:   &now,
	}
	reader := stubTaskReader{
		byID: map[string]TaskHydrationRow{
			"task-project-b": other,
		},
		tasks: []TaskHydrationRow{other},
	}

	coordinatorHydrator := taskHydrator{tasks: reader}
	if _, err := coordinatorHydrator.Fetch(context.Background(), "", "task-project-b"); err != nil {
		t.Fatalf("Fetch with empty project scope: %v", err)
	}

	if _, err := coordinatorHydrator.Fetch(context.Background(), "project-a", "task-project-b"); err == nil {
		t.Fatal("expected project-scoped fetch to reject cross-project task")
	}
}

func TestTaskHydrator_FetchIncludesReadableActorLabels(t *testing.T) {
	now := time.Now().UTC()
	task := TaskHydrationRow{
		ID:                   "task-readable-actors",
		ProjectID:            "project-a",
		Description:          "Make task actors readable",
		Title:                "Make task actors readable",
		Status:               "active",
		AssignedTo:           "member-worker",
		AssignedToLabel:      "Backend engineer",
		ClaimedByMemberID:    "member-reviewer",
		ClaimedByMemberLabel: "Reviewer",
		CreatedBy:            "member-coordinator",
		CreatedByLabel:       "Coordinator",
		CreatedAt:            &now,
	}
	reader := stubTaskReader{
		byID: map[string]TaskHydrationRow{
			"task-readable-actors": task,
		},
		tasks: []TaskHydrationRow{task},
	}

	hydrator := taskHydrator{tasks: reader}
	node, err := hydrator.Fetch(context.Background(), "project-a", "task-readable-actors")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	for key, want := range map[string]string{
		"assigneeRef":          "member-worker",
		"assignedTo":           "member-worker",
		"assignedToLabel":      "Backend engineer",
		"claimedByMemberId":    "member-reviewer",
		"claimedByMemberLabel": "Reviewer",
		"createdBy":            "member-coordinator",
		"createdByLabel":       "Coordinator",
	} {
		if got, _ := node.Fields[key].(string); got != want {
			t.Fatalf("field %s=%q want %q; fields=%+v", key, got, want, node.Fields)
		}
	}
}

func TestTaskHydrator_SearchCoordinatorScopeListsTasksAcrossProjects(t *testing.T) {
	now := time.Now().UTC()
	taskA := TaskHydrationRow{
		ID:          "task-project-a",
		ProjectID:   "project-a",
		Description: "Cross-project search A",
		Title:       "Cross-project search A",
		Status:      "pending",
		CreatedAt:   &now,
	}
	taskB := TaskHydrationRow{
		ID:          "task-project-b",
		ProjectID:   "project-b",
		Description: "Cross-project search B",
		Title:       "Cross-project search B",
		Status:      "pending",
		CreatedAt:   &now,
	}
	reader := stubTaskReader{
		byID: map[string]TaskHydrationRow{
			"task-project-a": taskA,
			"task-project-b": taskB,
		},
		tasks: []TaskHydrationRow{taskA, taskB},
	}

	coordinatorHydrator := taskHydrator{tasks: reader}
	results, err := coordinatorHydrator.Search(context.Background(), "", "cross-project", 10)
	if err != nil {
		t.Fatalf("Search with empty project scope: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("results=%d want 2", len(results))
	}
}

type stubDecisionReader struct {
	byID map[string]DecisionHydrationRow
}

func (s stubDecisionReader) GetDecision(_ context.Context, id string) (DecisionHydrationRow, error) {
	if decision, ok := s.byID[id]; ok {
		return decision, nil
	}
	return DecisionHydrationRow{}, fmt.Errorf("decision not found")
}

func (s stubDecisionReader) ListDecisions(context.Context, string, int) ([]DecisionHydrationRow, error) {
	return nil, nil
}

func TestDecisionHydrator_FetchIncludesInvalidationConditions(t *testing.T) {
	reader := stubDecisionReader{byID: map[string]DecisionHydrationRow{
		"dec-1": {
			ID:         "dec-1",
			ProjectID:  "proj-1",
			Source:     "agent",
			Title:      "Prioritize metered pricing",
			Confidence: 0.8,
			CreatedAt:  time.Now().UTC(),
			Kind:       "log",
			Rationale:  "It tests willingness to pay.",
			Context:    "Customers asked for usage-based billing during onboarding.",
			InvalidationConditions: []string{
				"Conversion drops below baseline",
				"Metering error rate exceeds 1%",
			},
		},
	}}

	node, err := (decisionHydrator{decisions: reader}).Fetch(context.Background(), "proj-1", "dec-1")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	got, ok := node.Fields["invalidationConditions"].([]string)
	if !ok {
		t.Fatalf("invalidationConditions type=%T value=%v", node.Fields["invalidationConditions"], node.Fields["invalidationConditions"])
	}
	if len(got) != 2 || got[0] != "Conversion drops below baseline" {
		t.Fatalf("invalidationConditions=%v", got)
	}
	if node.Fields["context"] != "Customers asked for usage-based billing during onboarding." {
		t.Fatalf("context=%q", node.Fields["context"])
	}
}
