package rpc

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/tinoosan/agen8-mcp-server/internal/core/types"
	"github.com/tinoosan/agen8-mcp-server/internal/services/project/domain/member"
	taskdomain "github.com/tinoosan/agen8-mcp-server/internal/services/task/domain"
)

func TestNewTaskViewMapsRebuiltTaskFields(t *testing.T) {
	createdAt := time.Date(2026, time.May, 15, 10, 0, 0, 0, time.FixedZone("BST", 3600))
	startedAt := createdAt.Add(1 * time.Minute)
	completedAt := createdAt.Add(10 * time.Minute)
	updatedAt := createdAt.Add(11 * time.Minute)
	phaseID := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	todoID := uuid.MustParse("22222222-2222-4222-8222-222222222222")

	task := taskdomain.Task{
		ID:                taskdomain.TaskID("task-1"),
		ProjectID:         types.ProjectID("project-1"),
		AssignedTo:        member.ID("member-worker"),
		ClaimedByMemberID: member.ID("member-worker"),
		TaskKind:          "implementation",
		CreatedBy:         "member-coordinator",
		Title:             "Build RPC contract",
		Description:       "Create task CRUD RPC types",
		AcceptanceCriteria: []taskdomain.AcceptanceCriterion{
			{ID: "criterion-1", Text: "types are internal", Satisfied: true},
			{ID: "criterion-2", Text: "old usage fields are absent"},
		},
		Status:       taskdomain.TaskStatusInReview,
		Summary:      "Implemented the DTOs",
		Error:        "needs one more check",
		Artifacts:    []string{"internal/services/task/rpc/types.go"},
		KeyResultRef: "kr-1",
		PlanPhaseID:  &phaseID,
		PlanTodoID:   &todoID,
		Metadata: map[string]any{
			"history": []any{"created", "submitted"},
		},
		CreatedAt:   &createdAt,
		StartedAt:   &startedAt,
		CompletedAt: &completedAt,
		UpdatedAt:   &updatedAt,
	}

	view := NewTaskView(task)

	if view.ID != "task-1" {
		t.Fatalf("id=%q want task-1", view.ID)
	}
	if view.ProjectID != "project-1" {
		t.Fatalf("projectId=%q want project-1", view.ProjectID)
	}
	if view.AssignedTo != "member-worker" {
		t.Fatalf("assignedTo=%q want member-worker", view.AssignedTo)
	}
	if view.ClaimedByMemberID != "member-worker" {
		t.Fatalf("claimedByMemberId=%q want member-worker", view.ClaimedByMemberID)
	}
	if view.CreatedBy != "member-coordinator" {
		t.Fatalf("createdBy=%q want member-coordinator", view.CreatedBy)
	}
	if view.Description != "Create task CRUD RPC types" {
		t.Fatalf("description=%q", view.Description)
	}
	if len(view.AcceptanceCriteria) != 2 || view.AcceptanceCriteria[0].ID != "criterion-1" || !view.AcceptanceCriteria[0].Satisfied {
		t.Fatalf("acceptanceCriteria=%+v", view.AcceptanceCriteria)
	}
	if view.PlanPhaseID != phaseID.String() || view.PlanTodoID != todoID.String() {
		t.Fatalf("plan refs = %q/%q", view.PlanPhaseID, view.PlanTodoID)
	}
	if view.Metadata["history"] == nil {
		t.Fatalf("metadata history missing: %+v", view.Metadata)
	}
	if view.CreatedAt == nil || !view.CreatedAt.Equal(createdAt.UTC()) {
		t.Fatalf("createdAt=%v want %s", view.CreatedAt, createdAt.UTC())
	}
	if view.StartedAt == nil || !view.StartedAt.Equal(startedAt.UTC()) {
		t.Fatalf("startedAt=%v want %s", view.StartedAt, startedAt.UTC())
	}
	if view.CompletedAt == nil || !view.CompletedAt.Equal(completedAt.UTC()) {
		t.Fatalf("completedAt=%v want %s", view.CompletedAt, completedAt.UTC())
	}
	if view.UpdatedAt == nil || !view.UpdatedAt.Equal(updatedAt.UTC()) {
		t.Fatalf("updatedAt=%v want %s", view.UpdatedAt, updatedAt.UTC())
	}

	view.Artifacts[0] = "mutated"
	if task.Artifacts[0] == "mutated" {
		t.Fatal("NewTaskView reused task artifact slice")
	}
	view.AcceptanceCriteria[0].Text = "mutated"
	if task.AcceptanceCriteria[0].Text == "mutated" {
		t.Fatal("NewTaskView reused task acceptance criteria slice")
	}
	view.Metadata["new"] = true
	if task.Metadata["new"] == true {
		t.Fatal("NewTaskView reused task metadata map")
	}
}

func TestTaskViewDoesNotExposeRemovedTaskConcepts(t *testing.T) {
	taskViewType := reflect.TypeOf(TaskView{})
	for _, field := range []string{
		"RunID",
		"Goal",
		"Priority",
		"AssignedRole",
		"AssignedToType",
		"TokenUsage",
		"DurationSeconds",
		"ParentTaskID",
		"RootTaskID",
		"SourceSpaceID",
		"DestinationSpaceID",
	} {
		if _, ok := taskViewType.FieldByName(field); ok {
			t.Fatalf("TaskView exposes removed field %s", field)
		}
	}
}

func TestTaskViewJSONShape(t *testing.T) {
	createdAt := time.Date(2026, time.May, 15, 9, 30, 0, 0, time.UTC)
	phaseID := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	todoID := uuid.MustParse("22222222-2222-4222-8222-222222222222")

	view := NewTaskView(taskdomain.Task{
		ID:                taskdomain.TaskID("task-1"),
		ProjectID:         types.ProjectID("project-1"),
		AssignedTo:        member.ID("member-worker"),
		ClaimedByMemberID: member.ID("member-worker"),
		TaskKind:          "implementation",
		CreatedBy:         "member-coordinator",
		Title:             "Build RPC contract",
		Description:       "Create task CRUD RPC types",
		AcceptanceCriteria: []taskdomain.AcceptanceCriterion{
			{ID: "criterion-1", Text: "types are internal", Satisfied: true},
		},
		Status:       taskdomain.TaskStatusInReview,
		Summary:      "Implemented the DTOs",
		Artifacts:    []string{"internal/services/task/rpc/types.go"},
		KeyResultRef: "kr-1",
		PlanPhaseID:  &phaseID,
		PlanTodoID:   &todoID,
		Metadata:     map[string]any{"history": []any{"created"}, "missionRef": "mission-1"},
		CreatedAt:    &createdAt,
	})

	raw, err := json.Marshal(view)
	if err != nil {
		t.Fatalf("marshal task view: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("unmarshal task view json: %v", err)
	}

	for _, key := range []string{
		"id",
		"projectId",
		"assignedTo",
		"claimedByMemberId",
		"taskKind",
		"createdBy",
		"title",
		"description",
		"acceptanceCriteria",
		"status",
		"summary",
		"artifacts",
		"keyResultRef",
		"missionRef",
		"planPhaseId",
		"planTodoId",
		"metadata",
		"createdAt",
	} {
		if _, ok := payload[key]; !ok {
			t.Fatalf("json key %q missing from %s", key, raw)
		}
	}

	for _, key := range []string{
		"runId",
		"goal",
		"priority",
		"assignedRole",
		"assignedToType",
		"inputTokens",
		"outputTokens",
		"totalTokens",
		"costUSD",
		"durationSeconds",
		"parentTaskId",
		"rootTaskId",
		"sourceSpaceId",
		"destinationSpaceId",
	} {
		if _, ok := payload[key]; ok {
			t.Fatalf("json contains removed key %q in %s", key, raw)
		}
	}
}
