package tui

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	agentstate "github.com/tinoosan/agen8/pkg/agent/state"
	"github.com/tinoosan/agen8/pkg/types"
)

func TestMergeTeamAndChildRunIDs_IncludesChildrenAndSubagentRole(t *testing.T) {
	roleByRun := map[string]string{
		"run-parent": "General Agent",
	}
	childRuns := []types.Run{
		{RunID: "run-child-1", SpawnIndex: 1},
		{RunID: "run-child-2", SpawnIndex: 2},
		{RunID: "run-child-1", SpawnIndex: 1}, // dedupe
	}

	merged := mergeTeamAndChildRunIDs([]string{"run-parent"}, childRuns, roleByRun)
	if len(merged) != 3 {
		t.Fatalf("expected merged run ids to include parent + 2 children, got %d (%v)", len(merged), merged)
	}
	if got := roleByRun["run-child-1"]; got != "Subagent-1" {
		t.Fatalf("expected run-child-1 role Subagent-1, got %q", got)
	}
	if got := roleByRun["run-child-2"]; got != "Subagent-2" {
		t.Fatalf("expected run-child-2 role Subagent-2, got %q", got)
	}
}

func TestListReviewPendingCallbacksBySourceRunID(t *testing.T) {
	store, err := agentstate.NewSQLiteTaskStore(filepath.Join(t.TempDir(), "agen8.db"))
	if err != nil {
		t.Fatalf("NewSQLiteTaskStore: %v", err)
	}
	now := time.Now().UTC()
	mk := func(task types.Task) {
		t.Helper()
		if err := store.CreateTask(context.Background(), task); err != nil {
			t.Fatalf("CreateTask(%s): %v", task.TaskID, err)
		}
	}
	mk(types.Task{
		TaskID:    "callback-1",
		SessionID: "sess-1",
		RunID:     "run-parent",
		Goal:      "callback 1",
		Status:    types.TaskStatusReviewPending,
		CreatedAt: &now,
		Metadata: map[string]any{
			"source":      "subagent.callback",
			"sourceRunId": "run-child-1",
		},
	})
	mk(types.Task{
		TaskID:    "callback-2",
		SessionID: "sess-1",
		RunID:     "run-parent",
		Goal:      "callback 2",
		Status:    types.TaskStatusReviewPending,
		CreatedAt: &now,
		Metadata: map[string]any{
			"source":      "subagent.callback",
			"sourceRunId": "run-child-1",
		},
	})
	mk(types.Task{
		TaskID:    "task-non-callback",
		SessionID: "sess-1",
		RunID:     "run-parent",
		Goal:      "non callback",
		Status:    types.TaskStatusReviewPending,
		CreatedAt: &now,
		Metadata: map[string]any{
			"source":      "task_create",
			"sourceRunId": "run-child-1",
		},
	})

	got := listReviewPendingCallbacksBySourceRunID(context.Background(), store, "", "sess-1")
	if got["run-child-1"] != 2 {
		t.Fatalf("expected 2 review_pending callbacks for run-child-1, got %d", got["run-child-1"])
	}
}
