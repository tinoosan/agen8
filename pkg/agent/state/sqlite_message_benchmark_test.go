package state

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/tinoosan/agen8/pkg/types"
)

func benchmarkMessageStore(b *testing.B) *SQLiteTaskStore {
	b.Helper()
	store, err := NewSQLiteTaskStore(filepath.Join(b.TempDir(), "tasks.sqlite"))
	if err != nil {
		b.Fatalf("NewSQLiteTaskStore: %v", err)
	}
	return store
}

func BenchmarkMessageStorePublish(b *testing.B) {
	ctx := context.Background()
	store := benchmarkMessageStore(b)
	now := time.Now().UTC()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		msg := types.AgentMessage{
			MessageID:     fmt.Sprintf("msg-%d", i),
			IntentID:      fmt.Sprintf("intent-%d", i),
			CorrelationID: fmt.Sprintf("corr-%d", i),
			ThreadID:      "thread-bench",
			RunID:         "run-bench",
			Channel:       types.MessageChannelInbox,
			Kind:          types.MessageKindTask,
			TaskRef:       fmt.Sprintf("task-%d", i),
			Status:        types.MessageStatusPending,
			VisibleAt:     now,
		}
		if _, err := store.PublishMessage(ctx, msg); err != nil {
			b.Fatalf("PublishMessage(%d): %v", i, err)
		}
	}
}

func BenchmarkMessageStoreClaimAck(b *testing.B) {
	ctx := context.Background()
	store := benchmarkMessageStore(b)
	now := time.Now().UTC()
	for i := 0; i < b.N; i++ {
		msg := types.AgentMessage{
			MessageID:     fmt.Sprintf("msg-claim-%d", i),
			IntentID:      fmt.Sprintf("intent-claim-%d", i),
			CorrelationID: fmt.Sprintf("corr-claim-%d", i),
			ThreadID:      "thread-claim",
			RunID:         "run-claim",
			Channel:       types.MessageChannelInbox,
			Kind:          types.MessageKindTask,
			TaskRef:       fmt.Sprintf("task-claim-%d", i),
			Status:        types.MessageStatusPending,
			VisibleAt:     now,
		}
		if _, err := store.PublishMessage(ctx, msg); err != nil {
			b.Fatalf("seed PublishMessage(%d): %v", i, err)
		}
	}

	filter := MessageClaimFilter{
		ThreadID: "thread-claim",
		RunID:    "run-claim",
		Channel:  types.MessageChannelInbox,
		Kinds:    []string{types.MessageKindTask},
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		msg, err := store.ClaimNextMessage(ctx, filter, time.Minute, "bench-consumer")
		if err != nil {
			b.Fatalf("ClaimNextMessage(%d): %v", i, err)
		}
		if err := store.AckMessage(ctx, msg.MessageID, MessageAckResult{Status: types.MessageStatusAcked}); err != nil {
			b.Fatalf("AckMessage(%d): %v", i, err)
		}
	}
}
