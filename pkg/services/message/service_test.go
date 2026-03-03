package message

import (
	"context"
	"testing"
	"time"

	"github.com/tinoosan/agen8/pkg/agent/state"
	"github.com/tinoosan/agen8/pkg/types"
)

type mockStore struct {
	messages map[string]types.AgentMessage
}

func newMockStore() *mockStore {
	return &mockStore{messages: map[string]types.AgentMessage{}}
}

func (m *mockStore) PublishMessage(_ context.Context, msg types.AgentMessage) (types.AgentMessage, error) {
	m.messages[msg.MessageID] = msg
	return msg, nil
}

func (m *mockStore) ClaimNextMessage(_ context.Context, _ state.MessageClaimFilter, _ time.Duration, _ string) (types.AgentMessage, error) {
	return types.AgentMessage{}, state.ErrMessageNotFound
}

func (m *mockStore) AckMessage(_ context.Context, messageID string, _ state.MessageAckResult) error {
	msg := m.messages[messageID]
	msg.Status = types.MessageStatusAcked
	m.messages[messageID] = msg
	return nil
}

func (m *mockStore) NackMessage(_ context.Context, messageID string, reason string, _ *time.Time) error {
	msg := m.messages[messageID]
	msg.Status = types.MessageStatusNacked
	msg.Error = reason
	m.messages[messageID] = msg
	return nil
}

func (m *mockStore) RequeueExpiredClaims(_ context.Context) error { return nil }

func (m *mockStore) GetMessage(_ context.Context, messageID string) (types.AgentMessage, error) {
	if msg, ok := m.messages[messageID]; ok {
		return msg, nil
	}
	return types.AgentMessage{}, state.ErrMessageNotFound
}

func (m *mockStore) ListMessages(_ context.Context, _ state.MessageFilter) ([]types.AgentMessage, error) {
	out := make([]types.AgentMessage, 0, len(m.messages))
	for _, msg := range m.messages {
		out = append(out, msg)
	}
	return out, nil
}

func (m *mockStore) CountMessages(_ context.Context, _ state.MessageFilter) (int, error) {
	return len(m.messages), nil
}

func TestManager_SubscribeWake_Filtering(t *testing.T) {
	svc := NewManager(newMockStore())
	allCh, cancelAll := svc.SubscribeWake("", "")
	defer cancelAll()
	threadCh, cancelThread := svc.SubscribeWake("thread-1", "")
	defer cancelThread()
	runCh, cancelRun := svc.SubscribeWake("thread-1", "run-1")
	defer cancelRun()

	msg := types.AgentMessage{
		MessageID:     "msg-1",
		IntentID:      "intent-1",
		CorrelationID: "corr-1",
		ThreadID:      "thread-1",
		RunID:         "run-1",
		Channel:       types.MessageChannelInbox,
		Kind:          types.MessageKindTask,
		Status:        types.MessageStatusPending,
		VisibleAt:     time.Now().UTC(),
	}
	if _, err := svc.PublishMessage(context.Background(), msg); err != nil {
		t.Fatalf("PublishMessage: %v", err)
	}

	assertWake(t, allCh, "all")
	assertWake(t, threadCh, "thread")
	assertWake(t, runCh, "run")
}

func assertWake(t *testing.T, ch <-chan struct{}, label string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("expected wake for %s", label)
	}
}
