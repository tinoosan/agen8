package mail

import (
	"testing"
	"time"

	"github.com/tinoosan/agen8/pkg/protocol"
	"github.com/tinoosan/agen8/pkg/types"
)

func TestFilterTasks_InboxIncludesReviewPending(t *testing.T) {
	now := time.Now().UTC()
	messages := []protocol.MailMessage{
		taskBackedMessage("task-pending", string(types.TaskStatusPending), "pending", now),
		taskBackedMessage("task-active", string(types.TaskStatusActive), "active", now),
		taskBackedMessage("task-review", string(types.TaskStatusReviewPending), "review", now),
		taskBackedMessage("task-done", string(types.TaskStatusSucceeded), "done", now),
	}

	inbox := filterMessages(messages, true)
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
	messages := []protocol.MailMessage{
		taskBackedMessage("task-pending", string(types.TaskStatusPending), "pending", now),
		taskBackedMessage("task-active", string(types.TaskStatusActive), "active", now),
		taskBackedMessage("task-review", string(types.TaskStatusReviewPending), "review", now),
		taskBackedMessage("task-done", string(types.TaskStatusSucceeded), "done", now),
		taskBackedMessage("task-failed", string(types.TaskStatusFailed), "failed", now),
	}

	outbox := filterMessages(messages, false)
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

func TestFilterTasks_CollapsesStagedCallbacksUnderBatchParent(t *testing.T) {
	now := time.Now().UTC()
	messages := []protocol.MailMessage{
		taskBackedMessageWithTask(protocol.Task{
			ID:             "callback-batch-1",
			Status:         string(types.TaskStatusReviewPending),
			Goal:           "batch callback",
			Source:         "team.batch.callback",
			BatchMode:      true,
			BatchSynthetic: true,
			CreatedAt:      now,
		}, now),
		taskBackedMessageWithTask(protocol.Task{
			ID:              "callback-child-1",
			Status:          string(types.TaskStatusReviewPending),
			Goal:            "child callback one",
			Source:          "team.callback",
			BatchMode:       true,
			BatchIncludedIn: "callback-batch-1",
			CreatedAt:       now,
		}, now),
		taskBackedMessageWithTask(protocol.Task{
			ID:              "callback-child-2",
			Status:          string(types.TaskStatusReviewPending),
			Goal:            "child callback two",
			Source:          "team.callback",
			BatchMode:       true,
			BatchIncludedIn: "callback-batch-1",
			CreatedAt:       now,
		}, now),
		taskBackedMessage("task-terminal", string(types.TaskStatusSucceeded), "normal completed task", now),
	}

	outbox := filterMessages(messages, false)
	if len(outbox) != 2 {
		t.Fatalf("expected 2 top-level outbox tasks, got %d: %+v", len(outbox), outbox)
	}
	var batch *taskEntry
	for i := range outbox {
		if outbox[i].ID == "callback-batch-1" {
			batch = &outbox[i]
			break
		}
	}
	if batch == nil {
		t.Fatalf("expected synthetic batch row in outbox: %+v", outbox)
	}
	if len(batch.Children) != 2 {
		t.Fatalf("expected two staged callbacks attached to batch, got %d", len(batch.Children))
	}
}

func TestFilterTasks_OrphanStagedCallbackRemainsVisible(t *testing.T) {
	now := time.Now().UTC()
	messages := []protocol.MailMessage{
		taskBackedMessageWithTask(protocol.Task{
			ID:              "callback-orphan",
			Status:          string(types.TaskStatusReviewPending),
			Goal:            "orphan callback",
			Source:          "team.callback",
			BatchMode:       true,
			BatchIncludedIn: "missing-batch-parent",
			CreatedAt:       now,
		}, now),
	}

	outbox := filterMessages(messages, false)
	if len(outbox) != 1 {
		t.Fatalf("expected orphan callback to remain visible, got %d", len(outbox))
	}
	if outbox[0].ID != "callback-orphan" {
		t.Fatalf("unexpected top-level orphan callback row: %+v", outbox[0])
	}
}

func TestFilterTasks_ChildDisplayStatusUsesBatchedForTerminalParent(t *testing.T) {
	now := time.Now().UTC()
	messages := []protocol.MailMessage{
		taskBackedMessageWithTask(protocol.Task{
			ID:             "callback-batch-done",
			Status:         string(types.TaskStatusSucceeded),
			Goal:           "completed batch",
			Source:         "team.batch.callback",
			BatchMode:      true,
			BatchSynthetic: true,
			CreatedAt:      now,
			CompletedAt:    now,
		}, now),
		taskBackedMessageWithTask(protocol.Task{
			ID:              "callback-child-review-pending",
			Status:          string(types.TaskStatusReviewPending),
			Goal:            "staged callback",
			Source:          "team.callback",
			BatchMode:       true,
			BatchIncludedIn: "callback-batch-done",
			CreatedAt:       now,
		}, now),
	}

	outbox := filterMessages(messages, false)
	if len(outbox) != 1 {
		t.Fatalf("expected one batch row, got %d", len(outbox))
	}
	if len(outbox[0].Children) != 1 {
		t.Fatalf("expected one child callback, got %d", len(outbox[0].Children))
	}
	if got := outbox[0].Children[0].DisplayStatus; got != "batched" {
		t.Fatalf("expected child display status to be batched, got %q", got)
	}
}

func taskBackedMessage(taskID, status, goal string, now time.Time) protocol.MailMessage {
	return taskBackedMessageWithTask(protocol.Task{
		ID:        taskID,
		Status:    status,
		Goal:      goal,
		CreatedAt: now,
	}, now)
}

func taskBackedMessageWithTask(task protocol.Task, now time.Time) protocol.MailMessage {
	task.TaskKind = task.TaskKind
	return protocol.MailMessage{
		MessageID:   "msg-" + task.ID,
		Kind:        types.MessageKindTask,
		Channel:     types.MessageChannelInbox,
		Status:      types.MessageStatusPending,
		TaskID:      task.ID,
		TaskStatus:  task.Status,
		ReadOnly:    false,
		CreatedAt:   now,
		UpdatedAt:   now,
		BodyPreview: task.Goal,
		Task:        &task,
	}
}
