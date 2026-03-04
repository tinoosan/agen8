package state

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/tinoosan/agen8/pkg/types"
)

func newSQLiteTaskStoreForMessageTest(t *testing.T) *SQLiteTaskStore {
	t.Helper()
	store, err := NewSQLiteTaskStore(filepath.Join(t.TempDir(), "tasks.sqlite"))
	if err != nil {
		t.Fatalf("NewSQLiteTaskStore: %v", err)
	}
	return store
}

func baseMessage() types.AgentMessage {
	now := time.Now().UTC()
	return types.AgentMessage{
		MessageID:     "msg-test-1",
		IntentID:      "intent-1",
		CorrelationID: "corr-1",
		ThreadID:      "thread-1",
		RunID:         "run-1",
		Channel:       types.MessageChannelInbox,
		Kind:          types.MessageKindTask,
		TaskRef:       "task-1",
		Status:        types.MessageStatusPending,
		VisibleAt:     now,
	}
}

func TestSQLiteTaskStore_PublishMessage_IdempotentByThreadIntent(t *testing.T) {
	ctx := context.Background()
	store := newSQLiteTaskStoreForMessageTest(t)

	first := baseMessage()
	published, err := store.PublishMessage(ctx, first)
	if err != nil {
		t.Fatalf("PublishMessage(first): %v", err)
	}

	second := first
	second.MessageID = "msg-test-2"
	second.Body = map[string]any{"retry": "true"}
	published2, err := store.PublishMessage(ctx, second)
	if err != nil {
		t.Fatalf("PublishMessage(second): %v", err)
	}

	if published2.MessageID != published.MessageID {
		t.Fatalf("expected idempotent publish to return %s, got %s", published.MessageID, published2.MessageID)
	}
	count, err := store.CountMessages(ctx, MessageFilter{ThreadID: "thread-1"})
	if err != nil {
		t.Fatalf("CountMessages: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 message, got %d", count)
	}
}

func TestSQLiteTaskStore_ClaimAckNackAndRequeue(t *testing.T) {
	ctx := context.Background()
	store := newSQLiteTaskStoreForMessageTest(t)

	msg := baseMessage()
	msg.MessageID = "msg-flow-1"
	msg.IntentID = "intent-flow-1"
	if _, err := store.PublishMessage(ctx, msg); err != nil {
		t.Fatalf("PublishMessage: %v", err)
	}

	claimed, err := store.ClaimNextMessage(ctx, MessageClaimFilter{
		ThreadID: "thread-1",
		RunID:    "run-1",
		Channel:  types.MessageChannelInbox,
		Kinds:    []string{types.MessageKindTask},
	}, time.Minute, "consumer-1")
	if err != nil {
		t.Fatalf("ClaimNextMessage: %v", err)
	}
	if claimed.Status != types.MessageStatusClaimed {
		t.Fatalf("expected claimed status, got %s", claimed.Status)
	}
	if claimed.Attempts != 1 {
		t.Fatalf("expected attempts=1 after claim, got %d", claimed.Attempts)
	}

	retryAt := time.Now().UTC().Add(10 * time.Millisecond)
	if err := store.NackMessage(ctx, claimed.MessageID, "retry", &retryAt); err != nil {
		t.Fatalf("NackMessage: %v", err)
	}
	nacked, err := store.GetMessage(ctx, claimed.MessageID)
	if err != nil {
		t.Fatalf("GetMessage after nack: %v", err)
	}
	if nacked.Status != types.MessageStatusPending {
		t.Fatalf("expected pending after retry nack, got %s", nacked.Status)
	}

	time.Sleep(15 * time.Millisecond)
	claimed2, err := store.ClaimNextMessage(ctx, MessageClaimFilter{
		ThreadID: "thread-1",
		RunID:    "run-1",
		Channel:  types.MessageChannelInbox,
		Kinds:    []string{types.MessageKindTask},
	}, time.Minute, "consumer-1")
	if err != nil {
		t.Fatalf("ClaimNextMessage(2): %v", err)
	}
	if err := store.AckMessage(ctx, claimed2.MessageID, MessageAckResult{Status: types.MessageStatusAcked}); err != nil {
		t.Fatalf("AckMessage: %v", err)
	}
	acked, err := store.GetMessage(ctx, claimed2.MessageID)
	if err != nil {
		t.Fatalf("GetMessage after ack: %v", err)
	}
	if acked.Status != types.MessageStatusAcked {
		t.Fatalf("expected acked status, got %s", acked.Status)
	}

	msg2 := baseMessage()
	msg2.MessageID = "msg-expired-1"
	msg2.IntentID = "intent-expired-1"
	if _, err := store.PublishMessage(ctx, msg2); err != nil {
		t.Fatalf("PublishMessage(msg2): %v", err)
	}
	claimed3, err := store.ClaimNextMessage(ctx, MessageClaimFilter{
		ThreadID: "thread-1",
		Channel:  types.MessageChannelInbox,
	}, 1*time.Millisecond, "consumer-2")
	if err != nil {
		t.Fatalf("ClaimNextMessage(3): %v", err)
	}
	time.Sleep(2 * time.Millisecond)
	if err := store.RequeueExpiredClaims(ctx); err != nil {
		t.Fatalf("RequeueExpiredClaims: %v", err)
	}
	requeued, err := store.GetMessage(ctx, claimed3.MessageID)
	if err != nil {
		t.Fatalf("GetMessage after requeue: %v", err)
	}
	if requeued.Status != types.MessageStatusPending {
		t.Fatalf("expected pending after requeue, got %s", requeued.Status)
	}
}

func TestSQLiteTaskStore_ClaimNextMessage_AssignedToFilter(t *testing.T) {
	ctx := context.Background()
	store := newSQLiteTaskStoreForMessageTest(t)
	now := time.Now().UTC()

	opsTask := types.Task{
		TaskID:         "task-ops-1",
		SessionID:      "thread-1",
		RunID:          "run-ceo",
		TeamID:         "team-1",
		AssignedRole:   "operations-lead",
		AssignedToType: "role",
		AssignedTo:     "operations-lead",
		Goal:           "ops task",
		Status:         types.TaskStatusPending,
		CreatedAt:      &now,
	}
	if err := store.CreateTask(ctx, opsTask); err != nil {
		t.Fatalf("CreateTask(ops): %v", err)
	}
	ctoTask := opsTask
	ctoTask.TaskID = "task-cto-1"
	ctoTask.AssignedRole = "cto"
	ctoTask.AssignedTo = "cto"
	ctoTask.Goal = "cto task"
	if err := store.CreateTask(ctx, ctoTask); err != nil {
		t.Fatalf("CreateTask(cto): %v", err)
	}

	msgOps := baseMessage()
	msgOps.MessageID = "msg-ops-1"
	msgOps.IntentID = "intent-ops-1"
	msgOps.TeamID = "team-1"
	msgOps.RunID = "run-ceo"
	msgOps.TaskRef = opsTask.TaskID
	msgOps.VisibleAt = now
	if _, err := store.PublishMessage(ctx, msgOps); err != nil {
		t.Fatalf("PublishMessage(ops): %v", err)
	}
	msgCTO := baseMessage()
	msgCTO.MessageID = "msg-cto-1"
	msgCTO.IntentID = "intent-cto-1"
	msgCTO.TeamID = "team-1"
	msgCTO.RunID = "run-ceo"
	msgCTO.TaskRef = ctoTask.TaskID
	msgCTO.VisibleAt = now
	if _, err := store.PublishMessage(ctx, msgCTO); err != nil {
		t.Fatalf("PublishMessage(cto): %v", err)
	}

	claimed, err := store.ClaimNextMessage(ctx, MessageClaimFilter{
		ThreadID:       "thread-1",
		TeamID:         "team-1",
		Channel:        types.MessageChannelInbox,
		Kinds:          []string{types.MessageKindTask},
		AssignedToType: "role",
		AssignedTo:     "operations-lead",
	}, time.Minute, "run-ops")
	if err != nil {
		t.Fatalf("ClaimNextMessage(role-filter): %v", err)
	}
	if claimed.TaskRef != opsTask.TaskID {
		t.Fatalf("claimed taskRef=%q want %q", claimed.TaskRef, opsTask.TaskID)
	}
}
