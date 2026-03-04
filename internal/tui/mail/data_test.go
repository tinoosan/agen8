package mail

import (
	"testing"
	"time"

	"github.com/tinoosan/agen8/pkg/protocol"
	"github.com/tinoosan/agen8/pkg/types"
)

func TestFilterTasks_InboxIncludesReviewPending(t *testing.T) {
	now := time.Now().UTC()
	tasks := []protocol.Task{
		{ID: "task-pending", Status: string(types.TaskStatusPending), Goal: "pending", CreatedAt: now},
		{ID: "task-active", Status: string(types.TaskStatusActive), Goal: "active", CreatedAt: now},
		{ID: "task-review", Status: string(types.TaskStatusReviewPending), Goal: "review", CreatedAt: now},
		{ID: "task-done", Status: string(types.TaskStatusSucceeded), Goal: "done", CreatedAt: now},
	}

	inbox := filterTasks(tasks, true)
	if len(inbox) != 3 {
		t.Fatalf("expected 3 inbox tasks (pending/active/review_pending), got %d", len(inbox))
	}
	foundReviewPending := false
	for _, t := range inbox {
		if t.ID == "task-review" {
			foundReviewPending = true
			break
		}
	}
	if !foundReviewPending {
		t.Fatalf("expected review_pending callback task in inbox")
	}
}

func TestFilterTasks_OutboxIncludesReviewPendingAndCompletedOnly(t *testing.T) {
	now := time.Now().UTC()
	tasks := []protocol.Task{
		{ID: "task-pending", Status: string(types.TaskStatusPending), Goal: "pending", CreatedAt: now},
		{ID: "task-active", Status: string(types.TaskStatusActive), Goal: "active", CreatedAt: now},
		{ID: "task-review", Status: string(types.TaskStatusReviewPending), Goal: "review", CreatedAt: now},
		{ID: "task-done", Status: string(types.TaskStatusSucceeded), Goal: "done", CreatedAt: now},
		{ID: "task-failed", Status: string(types.TaskStatusFailed), Goal: "failed", CreatedAt: now},
	}

	outbox := filterTasks(tasks, false)
	if len(outbox) != 3 {
		t.Fatalf("expected 3 outbox tasks (review_pending + terminal), got %d", len(outbox))
	}
	seen := map[string]bool{}
	for _, task := range outbox {
		seen[task.ID] = true
	}
	if !seen["task-review"] || !seen["task-done"] || !seen["task-failed"] {
		t.Fatalf("unexpected outbox tasks: %+v", outbox)
	}
	if seen["task-pending"] || seen["task-active"] {
		t.Fatalf("outbox should exclude pending/active tasks: %+v", outbox)
	}
}
