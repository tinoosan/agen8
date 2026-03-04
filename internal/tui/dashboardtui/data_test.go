package dashboardtui

import (
	"testing"

	"github.com/tinoosan/agen8/pkg/protocol"
)

func TestAccumulateTeamTaskCounts_ExcludesStagedCallbackChildren(t *testing.T) {
	tasks := []protocol.Task{
		{ID: "task-parent", RunID: protocol.RunID("run-coordinator"), Status: "succeeded"},
		{ID: "callback-child-1", RunID: protocol.RunID("run-reviewer"), Status: "review_pending", BatchMode: true, BatchIncludedIn: "callback-batch-1"},
		{ID: "callback-child-2", RunID: protocol.RunID("run-reviewer"), Status: "review_pending", BatchMode: true, BatchIncludedIn: "callback-batch-1"},
		{ID: "callback-child-3", RunID: protocol.RunID("run-reviewer"), Status: "review_pending", BatchMode: true, BatchIncludedIn: "callback-batch-1"},
		{ID: "callback-child-4", RunID: protocol.RunID("run-reviewer"), Status: "review_pending", BatchMode: true, BatchIncludedIn: "callback-batch-1"},
		{ID: "callback-child-5", RunID: protocol.RunID("run-reviewer"), Status: "review_pending", BatchMode: true, BatchIncludedIn: "callback-batch-1"},
		{ID: "callback-batch-1", RunID: protocol.RunID("run-reviewer"), Status: "review_pending", BatchMode: true, BatchSynthetic: true},
		{ID: "review-handoff-1", RunID: protocol.RunID("run-coordinator"), Status: "pending", Source: "review.handoff"},
	}

	counts := accumulateTeamTaskCounts(tasks)

	if got := counts.Assigned; got != 3 {
		t.Fatalf("expected 3 assigned top-level tasks, got %d", got)
	}
	if got := counts.Done; got != 1 {
		t.Fatalf("expected 1 done top-level task, got %d", got)
	}
	if got := counts.Active; got != 1 {
		t.Fatalf("expected 1 active top-level task, got %d", got)
	}
	if got := counts.Pending; got != 1 {
		t.Fatalf("expected 1 pending top-level task, got %d", got)
	}
	if got := counts.AssignedByRun["run-reviewer"]; got != 1 {
		t.Fatalf("expected reviewer run assigned count of 1, got %d", got)
	}
	if got := counts.AssignedByRun["run-coordinator"]; got != 2 {
		t.Fatalf("expected coordinator run assigned count of 2, got %d", got)
	}
}

func TestAccumulateTeamTaskCounts_UsesRoleBucketsWhenRunMissing(t *testing.T) {
	tasks := []protocol.Task{
		{ID: "team-role-task", AssignedRole: "reviewer", AssignedToType: "role", AssignedTo: "reviewer", Status: "pending"},
		{ID: "team-role-done", AssignedRole: "reviewer", AssignedToType: "role", AssignedTo: "reviewer", Status: "succeeded"},
	}

	counts := accumulateTeamTaskCounts(tasks)
	if got := counts.AssignedByRole["reviewer"]; got != 2 {
		t.Fatalf("expected reviewer role assigned count of 2, got %d", got)
	}
	if got := counts.CompletedByRole["reviewer"]; got != 1 {
		t.Fatalf("expected reviewer role completed count of 1, got %d", got)
	}
}
