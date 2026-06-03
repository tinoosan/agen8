package domain

import (
	"testing"
	"time"

	"github.com/tinoosan/agen8-mcp-server/pkg/types"
)

func TestNewMessageCreatesQueuedAgentMessage(t *testing.T) {
	now := time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC)
	msg, err := NewMessage(NewMessageInput{
		ID: "msg-1",
		Route: MessageRoute{
			SpaceID:             "space-1",
			SourceMemberID:      "member-source",
			DestinationMemberID: "member-dest",
			ChannelID:           "channel:space-1:member:member-dest",
		},
		Content: MessageContent{
			Kind:    types.AgentMessageKindInform,
			Subject: "Status",
			Body:    map[string]any{"text": "ready"},
		},
		Producer: MessageProducer{
			IntentID: "intent-1",
			Producer: "test",
		},
	}, now)
	if err != nil {
		t.Fatalf("NewMessage: %v", err)
	}
	inner := msg.Inner()
	if inner.ID != "msg-1" || inner.Status != types.MessageStatusQueuedTyped {
		t.Fatalf("message identity/status = %q/%q", inner.ID, inner.Status)
	}
	if inner.SpaceID != "space-1" || inner.DestinationMemberID != "member-dest" || inner.ChannelID != "channel:space-1:member:member-dest" {
		t.Fatalf("message route = %+v", inner)
	}
	if !inner.CreatedAt.Equal(now) || !inner.UpdatedAt.Equal(now) || !inner.VisibleAt.Equal(now) {
		t.Fatalf("timestamps not stamped from now: %+v", inner)
	}
}

func TestNewMessageValidatesRequiredFields(t *testing.T) {
	base := NewMessageInput{
		Route: MessageRoute{
			SpaceID:             "space-1",
			DestinationMemberID: "member-dest",
		},
		Content: MessageContent{
			Kind:    types.AgentMessageKindInform,
			Subject: "Status",
			Body:    map[string]any{"text": "ready"},
		},
		Producer: MessageProducer{
			IntentID: "intent-1",
			Producer: "test",
		},
	}
	tests := []struct {
		name string
		edit func(*NewMessageInput)
	}{
		{"space", func(in *NewMessageInput) { in.Route.SpaceID = " " }},
		{"destination", func(in *NewMessageInput) { in.Route.DestinationMemberID = " " }},
		{"kind", func(in *NewMessageInput) { in.Content.Kind = "" }},
		{"subject", func(in *NewMessageInput) { in.Content.Subject = " " }},
		{"body", func(in *NewMessageInput) { in.Content.Body = nil }},
		{"intent", func(in *NewMessageInput) { in.Producer.IntentID = " " }},
		{"producer", func(in *NewMessageInput) { in.Producer.Producer = " " }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := base
			input.Content.Body = map[string]any{"text": "ready"}
			tt.edit(&input)
			if _, err := NewMessage(input, time.Now()); err == nil {
				t.Fatalf("expected validation error")
			}
		})
	}
}

func TestNewMessageValidatesReplyCorrelationAndTaskRef(t *testing.T) {
	base := NewMessageInput{
		Route: MessageRoute{
			SpaceID:             "space-1",
			DestinationMemberID: "member-dest",
		},
		Content: MessageContent{
			Kind:    types.AgentMessageKindAck,
			Subject: "Ack",
			Body:    map[string]any{"ok": true},
		},
		Producer: MessageProducer{
			IntentID: "intent-1",
			Producer: "test",
		},
	}
	if _, err := NewMessage(base, time.Now()); err == nil {
		t.Fatalf("expected ack without correlation to fail")
	}
	base.Content.Kind = types.AgentMessageKindResponse
	if _, err := NewMessage(base, time.Now()); err == nil {
		t.Fatalf("expected response without correlation to fail")
	}
	base.Content.Kind = types.AgentMessageKindTask
	base.Content.Subject = ""
	base.Producer.CorrelationID = "corr-1"
	if _, err := NewMessage(base, time.Now()); err == nil {
		t.Fatalf("expected task without task ref to fail")
	}
}

func TestMessageConsume(t *testing.T) {
	now := time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC)
	msg, err := NewMessage(NewMessageInput{
		Route: MessageRoute{SpaceID: "space-1", DestinationMemberID: "member-dest"},
		Content: MessageContent{
			Kind:    types.AgentMessageKindInform,
			Subject: "Status",
			Body:    map[string]any{"text": "ready"},
		},
		Producer: MessageProducer{IntentID: "intent-1", Producer: "test"},
	}, now)
	if err != nil {
		t.Fatalf("NewMessage: %v", err)
	}
	consumedAt := now.Add(time.Minute)
	consumed, err := msg.Consume("consumer-1", consumedAt)
	if err != nil {
		t.Fatalf("Consume: %v", err)
	}
	inner := consumed.Inner()
	if inner.Status != types.MessageStatusConsumedTyped || inner.ConsumedBy != "consumer-1" {
		t.Fatalf("consume state = %q/%q", inner.Status, inner.ConsumedBy)
	}
	if inner.ConsumedAt == nil || !inner.ConsumedAt.Equal(consumedAt) || !inner.UpdatedAt.Equal(consumedAt) {
		t.Fatalf("consume timestamps = %+v", inner)
	}
	if _, err := consumed.Consume("consumer-2", consumedAt); err == nil {
		t.Fatalf("expected consumed message to reject second consume")
	}
	if _, err := msg.Consume(" ", consumedAt); err == nil {
		t.Fatalf("expected empty consumer to fail")
	}
}
