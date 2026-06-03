package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	spacedomain "github.com/tinoosan/agen8-mcp-server/internal/services/space/domain"

	"github.com/tinoosan/agen8-mcp-server/internal/services/message/domain"
	"github.com/tinoosan/agen8-mcp-server/internal/services/message/domain/channel"
	"github.com/tinoosan/agen8-mcp-server/internal/services/message/domain/conversation"
	"github.com/tinoosan/agen8-mcp-server/internal/services/space/domain/member"
	taskdomain "github.com/tinoosan/agen8-mcp-server/internal/services/task/domain"
	"github.com/tinoosan/agen8-mcp-server/pkg/types"
)

func TestServicePublishAgentMessageSavesAndWakes(t *testing.T) {
	now := time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC)
	repo := newMemoryRepo()
	svc := newServiceForTest(t, repo, now)
	wakes, cancel := svc.SubscribeMemberWake("member-dest")
	defer cancel()

	saved, err := svc.PublishAgentMessage(context.Background(), domain.NewMessageInput{
		ID: "msg-1",
		Route: domain.MessageRoute{
			SpaceID:             "space-1",
			DestinationMemberID: "member-dest",
			ChannelID:           "channel:space-1:member:member-dest",
		},
		Content: domain.MessageContent{
			Kind:    types.AgentMessageKindInform,
			Subject: "Status",
			Body:    map[string]any{"text": "ready"},
		},
		Producer: domain.MessageProducer{
			IntentID: "intent-1",
			Producer: "test",
		},
	})
	if err != nil {
		t.Fatalf("PublishAgentMessage: %v", err)
	}
	if saved.Status != types.MessageStatusQueuedTyped || !saved.CreatedAt.Equal(now) {
		t.Fatalf("saved message = %+v", saved)
	}
	ch, err := svc.LoadMemberChannel(context.Background(), "space-1", "member-dest")
	if err != nil {
		t.Fatalf("LoadMemberChannel: %v", err)
	}
	if ch.LastMessageAt == nil || !ch.LastMessageAt.Equal(now) {
		t.Fatalf("channel last activity=%v want %s", ch.LastMessageAt, now)
	}
	counts, err := svc.UnreadCountsByChannel(context.Background(), "user-1", []types.ChannelID{ch.ID})
	if err != nil {
		t.Fatalf("UnreadCountsByChannel: %v", err)
	}
	if counts[ch.ID] != 1 {
		t.Fatalf("unread count=%d want 1", counts[ch.ID])
	}
	select {
	case wake := <-wakes:
		if wake.MessageID != "msg-1" || wake.DestinationMemberID != "member-dest" || wake.ChannelID != "channel:space-1:member:member-dest" {
			t.Fatalf("wake = %+v", wake)
		}
	case <-time.After(time.Second):
		t.Fatal("expected wake")
	}
}

func TestServiceDeliveryReceiveThenRecordDelivered(t *testing.T) {
	now := time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC)
	repo := newMemoryRepo()
	svc := newServiceForTest(t, repo, now)
	msg, err := domain.NewMessage(domain.NewMessageInput{
		ID: "msg-1",
		Route: domain.MessageRoute{
			SpaceID:             "space-1",
			DestinationMemberID: "member-dest",
		},
		Content: domain.MessageContent{
			Kind:    types.AgentMessageKindSystem,
			Subject: "Notice",
			Body:    map[string]any{"text": "ready"},
		},
		Producer: domain.MessageProducer{IntentID: "intent-1", Producer: "test"},
	}, now)
	if err != nil {
		t.Fatalf("NewMessage: %v", err)
	}
	if _, err := repo.SaveQueued(context.Background(), msg.Inner()); err != nil {
		t.Fatalf("SaveQueued: %v", err)
	}
	received, err := svc.ReceiveNextForDelivery(context.Background(), "member-dest")
	if err != nil {
		t.Fatalf("ReceiveNextForDelivery: %v", err)
	}
	if received.Status != types.MessageStatusQueuedTyped {
		t.Fatalf("received status=%q want queued", received.Status)
	}
	consumed, err := svc.RecordDelivered(context.Background(), received.ID, "consumer-1")
	if err != nil {
		t.Fatalf("RecordDelivered: %v", err)
	}
	if consumed.Status != types.MessageStatusConsumedTyped || consumed.ConsumedBy != "consumer-1" {
		t.Fatalf("consumed = %+v", consumed)
	}
}

func TestServiceDeliverNextAgentMessageSendsAttributedEnvelopeAndActivity(t *testing.T) {
	now := time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC)
	repo := newMemoryRepo()
	conversations := newMemoryConversationRepo()
	harness := &fakeHarnessChatSender{}
	svc := newServiceWithHarnessForTest(t, repo, conversations, harness, now)
	_, err := svc.PublishAgentMessage(context.Background(), domain.NewMessageInput{
		ID: "msg-1",
		Route: domain.MessageRoute{
			SpaceID:             "space-1",
			SourceMemberID:      "member-source",
			DestinationMemberID: "member-dest",
			ChannelID:           "channel:space-1:member:member-dest",
		},
		Content: domain.MessageContent{
			Kind:    types.AgentMessageKindQuery,
			Subject: "Check resolver failures",
			Body:    map[string]any{"text": "inspect resolver logs"},
		},
		Producer: domain.MessageProducer{
			IntentID:      "intent-1",
			CorrelationID: "corr-1",
			Producer:      "mcp.message",
		},
		Metadata: map[string]any{"sourceMemberLabel": "Sarah"},
	})
	if err != nil {
		t.Fatalf("PublishAgentMessage: %v", err)
	}

	delivered, err := svc.DeliverNextAgentMessage(context.Background(), "member-dest")
	if err != nil {
		t.Fatalf("DeliverNextAgentMessage: %v", err)
	}
	if delivered.Status != types.MessageStatusConsumedTyped || delivered.ConsumedBy != "member-dest" {
		t.Fatalf("delivered=%+v", delivered)
	}
	if !strings.Contains(harness.input.Text, "Agen8 member message") ||
		!strings.Contains(harness.input.Text, "from: Sarah (member-source)") ||
		!strings.Contains(harness.input.Text, "producer: mcp.message") ||
		!strings.Contains(harness.input.Text, "correlationId: corr-1") {
		t.Fatalf("harness text:\n%s", harness.input.Text)
	}
	if !harness.input.AllowSteering || !harness.input.SteerOnly {
		t.Fatalf("agent inbox delivery must steer the active harness turn only, input=%+v", harness.input)
	}
	activities, err := conversations.ListActivitiesByChannel(context.Background(), "channel:space-1:member:member-dest", 10)
	if err != nil {
		t.Fatalf("ListActivitiesByChannel: %v", err)
	}
	if len(activities) != 1 {
		t.Fatalf("activities=%+v", activities)
	}
	activity := activities[0]
	if activity.Kind != "agent_message_received" || activity.Title != "Message from Sarah" || activity.Status != "completed" {
		t.Fatalf("activity=%+v", activity)
	}
	if activity.Data["sourceMemberId"] != "member-source" || activity.Data["producer"] != "mcp.message" || activity.Data["correlationId"] != "corr-1" {
		t.Fatalf("activity data=%+v", activity.Data)
	}
}

func TestServiceDeliverNextAgentMessageSuppressesStaleTaskNotification(t *testing.T) {
	now := time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC)
	later := now.Add(time.Minute)
	repo := newMemoryRepo()
	conversations := newMemoryConversationRepo()
	harness := &fakeHarnessChatSender{}
	svc := newServiceWithHarnessForTest(t, repo, conversations, harness, now)
	svc.SetTaskStateReader(fakeTaskStateReader{tasks: map[taskdomain.TaskID]taskdomain.Task{
		"task-1": {
			ID:        "task-1",
			Status:    taskdomain.TaskStatusSucceeded,
			UpdatedAt: &later,
		},
	}})

	_, err := svc.PublishAgentMessage(context.Background(), domain.NewMessageInput{
		ID: "msg-stale",
		Route: domain.MessageRoute{
			SpaceID:             "space-1",
			DestinationMemberID: "member-dest",
			ChannelID:           "channel:space-1:member:member-dest",
		},
		Content: domain.MessageContent{
			Kind:    types.AgentMessageKindSystem,
			Subject: "Task ready for review",
			Body: map[string]any{
				"event":         "review_requested",
				"taskId":        "task-1",
				"nextAction":    "review",
				"taskStatus":    "in_review",
				"taskUpdatedAt": now.Format(time.RFC3339Nano),
				"message":       "Task ready for review",
			},
			TaskRef: "task-1",
		},
		Producer: domain.MessageProducer{
			IntentID:      "task:task-1:review_requested",
			CorrelationID: "task:task-1",
			Producer:      "task-service",
		},
	})
	if err != nil {
		t.Fatalf("PublishAgentMessage: %v", err)
	}

	delivered, err := svc.DeliverNextAgentMessage(context.Background(), "member-dest")
	if err != nil {
		t.Fatalf("DeliverNextAgentMessage: %v", err)
	}
	if delivered.Status != types.MessageStatusConsumedTyped {
		t.Fatalf("delivered status=%q want consumed", delivered.Status)
	}
	if harness.input.Text != "" {
		t.Fatalf("harness received stale notification: %q", harness.input.Text)
	}
	activities, err := svc.ListConversationActivities(context.Background(), "channel:space-1:member:member-dest", 10)
	if err != nil {
		t.Fatalf("ListConversationActivities: %v", err)
	}
	if len(activities) != 1 {
		t.Fatalf("activities=%d want 1", len(activities))
	}
	if activities[0].Status != "stale" || activities[0].Data["stale"] != "true" {
		t.Fatalf("activity status/data=%q/%v want stale", activities[0].Status, activities[0].Data)
	}
}

func TestServiceDeliverNextAgentMessageCreatesPendingActivityBeforeHarnessHandoff(t *testing.T) {
	now := time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC)
	repo := newMemoryRepo()
	conversations := newMemoryConversationRepo()
	harness := &fakeHarnessChatSender{
		sendFn: func(ctx context.Context, input HarnessChatMessage) (HarnessChatResult, error) {
			activities, err := conversations.ListActivitiesByChannel(ctx, "channel:space-1:member:member-dest", 10)
			if err != nil {
				return HarnessChatResult{}, err
			}
			if len(activities) != 1 || activities[0].Status != "pending" || activities[0].CompletedAt != nil {
				return HarnessChatResult{}, fmt.Errorf("pending activities=%+v", activities)
			}
			return HarnessChatResult{SessionID: "session-1", TurnID: "turn-1", Delivery: string(conversation.DeliveryDelivered), Text: "ok"}, nil
		},
	}
	svc := newServiceWithHarnessForTest(t, repo, conversations, harness, now)
	_, err := svc.PublishAgentMessage(context.Background(), domain.NewMessageInput{
		ID: "msg-1",
		Route: domain.MessageRoute{
			SpaceID:             "space-1",
			SourceMemberID:      "member-source",
			DestinationMemberID: "member-dest",
			ChannelID:           "channel:space-1:member:member-dest",
		},
		Content: domain.MessageContent{
			Kind:    types.AgentMessageKindInform,
			Subject: "Heads up",
			Body:    map[string]any{"text": "check status"},
		},
		Producer: domain.MessageProducer{IntentID: "intent-1", CorrelationID: "corr-1", Producer: "mcp.message"},
	})
	if err != nil {
		t.Fatalf("PublishAgentMessage: %v", err)
	}

	if _, err := svc.DeliverNextAgentMessage(context.Background(), "member-dest"); err != nil {
		t.Fatalf("DeliverNextAgentMessage: %v", err)
	}
	activities, err := conversations.ListActivitiesByChannel(context.Background(), "channel:space-1:member:member-dest", 10)
	if err != nil {
		t.Fatalf("ListActivitiesByChannel: %v", err)
	}
	if len(activities) != 1 || activities[0].Status != "completed" || activities[0].CompletedAt == nil {
		t.Fatalf("completed activities=%+v", activities)
	}
	if activities[0].SessionID != "session-1" || activities[0].TurnID != "turn-1" {
		t.Fatalf("activity binding=%+v", activities[0])
	}
}

func TestServiceDeliverNextAgentMessageKeepsReceivedActivityCreatedAt(t *testing.T) {
	publishedAt := time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC)
	completedAt := publishedAt.Add(45 * time.Second)
	repo := newMemoryRepo()
	conversations := newMemoryConversationRepo()
	clock := &mutableClock{now: publishedAt}
	harness := &fakeHarnessChatSender{
		sendFn: func(_ context.Context, _ HarnessChatMessage) (HarnessChatResult, error) {
			clock.now = completedAt
			return HarnessChatResult{SessionID: "session-1", TurnID: "turn-1", Delivery: string(conversation.DeliveryDelivered)}, nil
		},
	}
	svc, err := NewService(NewServiceParams{Repository: repo, Conversations: conversations, HarnessChatSender: harness, Clock: clock})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	_, err = svc.PublishAgentMessage(context.Background(), domain.NewMessageInput{
		ID: "msg-1",
		Route: domain.MessageRoute{
			SpaceID:             "space-1",
			SourceMemberID:      "member-source",
			DestinationMemberID: "member-dest",
			ChannelID:           "channel:space-1:member:member-dest",
		},
		Content: domain.MessageContent{
			Kind:    types.AgentMessageKindAck,
			Subject: "Ack: next-phase KRs noted",
			Body:    map[string]any{"text": "noted"},
		},
		Producer: domain.MessageProducer{IntentID: "intent-1", CorrelationID: "corr-1", Producer: "mcp.message"},
	})
	if err != nil {
		t.Fatalf("PublishAgentMessage: %v", err)
	}

	if _, err := svc.DeliverNextAgentMessage(context.Background(), "member-dest"); err != nil {
		t.Fatalf("DeliverNextAgentMessage: %v", err)
	}
	activities, err := conversations.ListActivitiesByChannel(context.Background(), "channel:space-1:member:member-dest", 10)
	if err != nil {
		t.Fatalf("ListActivitiesByChannel: %v", err)
	}
	if len(activities) != 1 {
		t.Fatalf("activities=%+v", activities)
	}
	if !activities[0].CreatedAt.Equal(publishedAt) {
		t.Fatalf("createdAt=%s want %s", activities[0].CreatedAt, publishedAt)
	}
	if activities[0].CompletedAt == nil || !activities[0].CompletedAt.Equal(completedAt) {
		t.Fatalf("completedAt=%v want %s", activities[0].CompletedAt, completedAt)
	}
}

func TestServiceDeliverNextAgentMessageStreamsResponseAndNotifiesConversation(t *testing.T) {
	now := time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC)
	repo := newMemoryRepo()
	conversations := newMemoryConversationRepo()
	notifier := &spyConversationNotifier{}
	harness := &fakeHarnessChatSender{
		sendFn: func(_ context.Context, input HarnessChatMessage) (HarnessChatResult, error) {
			if input.Stream == nil {
				return HarnessChatResult{}, fmt.Errorf("stream is required")
			}
			if err := input.Stream.AppendAssistantDelta(context.Background(), HarnessAssistantDelta{
				SessionID: "session-1",
				TurnID:    "turn-stream",
				Sequence:  2,
				Text:      "received and checking",
			}); err != nil {
				return HarnessChatResult{}, err
			}
			if err := input.Stream.AppendActivity(context.Background(), HarnessActivity{
				SessionID:  "session-1",
				TurnID:     "turn-stream",
				ToolCallID: "call-1",
				ToolName:   "agen8/message",
				Sequence:   3,
				Status:     "completed",
				Text:       "queued response",
			}); err != nil {
				return HarnessChatResult{}, err
			}
			return HarnessChatResult{SessionID: "session-1", TurnID: "turn-stream", Delivery: string(conversation.DeliveryDelivered)}, nil
		},
	}
	svc := newServiceWithHarnessForTest(t, repo, conversations, harness, now)
	svc.SetConversationNotifier(notifier)
	_, err := svc.PublishAgentMessage(context.Background(), domain.NewMessageInput{
		ID: "msg-1",
		Route: domain.MessageRoute{
			SpaceID:             "space-1",
			SourceMemberID:      "member-source",
			DestinationMemberID: "member-dest",
			ChannelID:           "channel:space-1:member:member-dest",
		},
		Content: domain.MessageContent{
			Kind:    types.AgentMessageKindQuery,
			Subject: "Check delivery",
			Body:    map[string]any{"text": "did this arrive?"},
		},
		Producer: domain.MessageProducer{
			IntentID:      "intent-1",
			CorrelationID: "corr-1",
			Producer:      "mcp.message",
		},
		Metadata: map[string]any{"sourceMemberLabel": "Sarah"},
	})
	if err != nil {
		t.Fatalf("PublishAgentMessage: %v", err)
	}

	if _, err := svc.DeliverNextAgentMessage(context.Background(), "member-dest"); err != nil {
		t.Fatalf("DeliverNextAgentMessage: %v", err)
	}
	messages, err := conversations.ListByChannel(context.Background(), "channel:space-1:member:member-dest", 10)
	if err != nil {
		t.Fatalf("ListByChannel: %v", err)
	}
	if len(messages) != 1 || messages[0].Text != "received and checking" || messages[0].TurnID != "turn-stream" {
		t.Fatalf("streamed messages=%+v", messages)
	}
	activities, err := conversations.ListActivitiesByChannel(context.Background(), "channel:space-1:member:member-dest", 10)
	if err != nil {
		t.Fatalf("ListActivitiesByChannel: %v", err)
	}
	if len(activities) != 2 {
		t.Fatalf("activities=%+v", activities)
	}
	if activities[0].Kind != "agent_message_received" || activities[0].Status != "completed" {
		t.Fatalf("received activity=%+v", activities[0])
	}
	if activities[1].Kind != "tool_call" || activities[1].Title != "agen8/message" {
		t.Fatalf("tool activity=%+v", activities[1])
	}
	if len(notifier.messages) < 3 {
		t.Fatalf("notifications=%+v", notifier.messages)
	}
}

func TestServiceDeliverNextAgentMessagePersistsNonStreamingNarrationAsOutboundMessage(t *testing.T) {
	now := time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC)
	repo := newMemoryRepo()
	conversations := newMemoryConversationRepo()
	harness := &fakeHarnessChatSender{
		sendFn: func(_ context.Context, _ HarnessChatMessage) (HarnessChatResult, error) {
			return HarnessChatResult{
				SessionID: "session-1",
				TurnID:    "turn-1",
				Delivery:  string(conversation.DeliveryDelivered),
				Text:      "Sarah sent a follow-up query. I will respond directly.",
			}, nil
		},
	}
	svc := newServiceWithHarnessForTest(t, repo, conversations, harness, now)
	_, err := svc.PublishAgentMessage(context.Background(), domain.NewMessageInput{
		ID: "msg-1",
		Route: domain.MessageRoute{
			SpaceID:             "space-1",
			SourceMemberID:      "member-source",
			DestinationMemberID: "member-dest",
			ChannelID:           "channel:space-1:member:member-dest",
		},
		Content: domain.MessageContent{
			Kind:    types.AgentMessageKindQuery,
			Subject: "Check delivery",
			Body:    map[string]any{"text": "did this arrive?"},
		},
		Producer: domain.MessageProducer{
			IntentID:      "intent-1",
			CorrelationID: "corr-1",
			Producer:      "mcp.message",
		},
		Metadata: map[string]any{"sourceMemberLabel": "Sarah"},
	})
	if err != nil {
		t.Fatalf("PublishAgentMessage: %v", err)
	}

	if _, err := svc.DeliverNextAgentMessage(context.Background(), "member-dest"); err != nil {
		t.Fatalf("DeliverNextAgentMessage: %v", err)
	}
	messages, err := conversations.ListByChannel(context.Background(), "channel:space-1:member:member-dest", 10)
	if err != nil {
		t.Fatalf("ListByChannel: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("messages=%+v", messages)
	}
	if messages[0].Direction != conversation.DirectionOutbound || messages[0].Render != conversation.RenderVisible {
		t.Fatalf("message visibility=%+v", messages[0])
	}
	if messages[0].Text != "Sarah sent a follow-up query. I will respond directly." {
		t.Fatalf("message text=%q", messages[0].Text)
	}
	activities, err := conversations.ListActivitiesByChannel(context.Background(), "channel:space-1:member:member-dest", 10)
	if err != nil {
		t.Fatalf("ListActivitiesByChannel: %v", err)
	}
	if len(activities) != 1 || activities[0].Kind != "agent_message_received" {
		t.Fatalf("activities=%+v", activities)
	}
	if _, ok := activities[0].Data["error"]; ok {
		t.Fatalf("received activity must not store successful narration as error: %+v", activities[0].Data)
	}
}

func TestServiceStartAgentDeliveryDrainsPublishedMessages(t *testing.T) {
	now := time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC)
	repo := newMemoryRepo()
	conversations := newMemoryConversationRepo()
	delivered := make(chan HarnessChatMessage, 1)
	harness := &fakeHarnessChatSender{
		sendFn: func(ctx context.Context, input HarnessChatMessage) (HarnessChatResult, error) {
			delivered <- input
			return HarnessChatResult{SessionID: "session-1", TurnID: "turn-1", Delivery: string(conversation.DeliveryDelivered)}, nil
		},
	}
	svc := newServiceWithHarnessForTest(t, repo, conversations, harness, now)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := svc.StartAgentDelivery(ctx, "member-dest"); err != nil {
		t.Fatalf("StartAgentDelivery: %v", err)
	}
	defer svc.StopAgentDelivery("member-dest")

	if _, err := svc.PublishAgentMessage(context.Background(), domain.NewMessageInput{
		ID: "msg-1",
		Route: domain.MessageRoute{
			SpaceID:             "space-1",
			DestinationMemberID: "member-dest",
			ChannelID:           "channel:space-1:member:member-dest",
		},
		Content: domain.MessageContent{
			Kind:    types.AgentMessageKindInform,
			Subject: "Wake",
			Body:    map[string]any{"text": "wake up"},
		},
		Producer: domain.MessageProducer{IntentID: "intent-1", Producer: "test"},
	}); err != nil {
		t.Fatalf("PublishAgentMessage: %v", err)
	}

	select {
	case input := <-delivered:
		if input.MemberID != "member-dest" || !strings.Contains(input.Text, "wake up") {
			t.Fatalf("delivered input=%+v", input)
		}
		if !input.AllowSteering {
			t.Fatalf("agent inbox delivery must allow harness steering")
		}
		if !input.SteerOnly {
			t.Fatalf("agent inbox delivery must not start a background harness turn")
		}
	case <-time.After(time.Second):
		t.Fatal("expected agent delivery")
	}
	var msg types.AgentMessage
	var err error
	deadline := time.Now().Add(time.Second)
	for {
		msg, err = svc.GetMessage(context.Background(), "msg-1")
		if err != nil {
			t.Fatalf("GetMessage: %v", err)
		}
		if msg.Status == types.MessageStatusConsumedTyped && msg.ConsumedBy == "member-dest" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("message=%+v", msg)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestServiceStartAgentDeliveryDrainsFreshTaskNotification(t *testing.T) {
	now := time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC)
	taskUpdatedAt := now.Add(-time.Minute)
	repo := newMemoryRepo()
	conversations := newMemoryConversationRepo()
	delivered := make(chan HarnessChatMessage, 1)
	harness := &fakeHarnessChatSender{
		sendFn: func(ctx context.Context, input HarnessChatMessage) (HarnessChatResult, error) {
			delivered <- input
			return HarnessChatResult{SessionID: "session-1", TurnID: "turn-1", Delivery: string(conversation.DeliveryDelivered)}, nil
		},
	}
	svc := newServiceWithHarnessForTest(t, repo, conversations, harness, now)
	svc.SetTaskStateReader(fakeTaskStateReader{tasks: map[taskdomain.TaskID]taskdomain.Task{
		"task-fresh": {
			ID:         "task-fresh",
			SpaceID:    "space-1",
			AssignedTo: "member-dest",
			Status:     taskdomain.TaskStatusPending,
			UpdatedAt:  &taskUpdatedAt,
		},
	}})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := svc.StartAgentDelivery(ctx, "member-dest"); err != nil {
		t.Fatalf("StartAgentDelivery: %v", err)
	}
	defer svc.StopAgentDelivery("member-dest")

	if _, err := svc.PublishAgentMessage(context.Background(), domain.NewMessageInput{
		ID: "msg-task-fresh",
		Route: domain.MessageRoute{
			SpaceID:             "space-1",
			DestinationMemberID: "member-dest",
			ChannelID:           "channel:space-1:member:member-dest",
		},
		Content: domain.MessageContent{
			Kind:    types.AgentMessageKindSystem,
			Subject: "Task assigned",
			TaskRef: "task-fresh",
			Body: map[string]any{
				"event":         "assigned",
				"message":       "Task assigned",
				"taskId":        "task-fresh",
				"taskStatus":    "pending",
				"taskUpdatedAt": taskUpdatedAt.Format(time.RFC3339Nano),
			},
		},
		Producer: domain.MessageProducer{IntentID: "intent-task-fresh", Producer: "task-service"},
	}); err != nil {
		t.Fatalf("PublishAgentMessage: %v", err)
	}

	select {
	case input := <-delivered:
		if input.MemberID != "member-dest" || !strings.Contains(input.Text, "taskRef: task-fresh") {
			t.Fatalf("delivered input=%+v", input)
		}
		if !input.AllowSteering {
			t.Fatalf("task inbox delivery must allow harness steering")
		}
	case <-time.After(time.Second):
		t.Fatal("expected fresh task notification delivery")
	}
	msg, err := svc.GetMessage(context.Background(), "msg-task-fresh")
	if err != nil {
		t.Fatalf("GetMessage: %v", err)
	}
	if msg.Status != types.MessageStatusConsumedTyped || msg.ConsumedBy != "member-dest" {
		t.Fatalf("message=%+v", msg)
	}
	activities, err := conversations.ListActivitiesByChannel(context.Background(), "channel:space-1:member:member-dest", 10)
	if err != nil {
		t.Fatalf("ListActivitiesByChannel: %v", err)
	}
	if len(activities) != 1 || activities[0].Kind != "agent_message_received" || activities[0].Status != "completed" {
		t.Fatalf("activities=%+v", activities)
	}
	if activities[0].SessionID != "session-1" || activities[0].TurnID != "turn-1" {
		t.Fatalf("activity routing=%+v", activities[0])
	}
}

func TestServicePublishAgentMessageAutoStartsDeliveryWhenConfigured(t *testing.T) {
	now := time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC)
	repo := newMemoryRepo()
	conversations := newMemoryConversationRepo()
	delivered := make(chan HarnessChatMessage, 1)
	harness := &fakeHarnessChatSender{
		sendFn: func(ctx context.Context, input HarnessChatMessage) (HarnessChatResult, error) {
			delivered <- input
			return HarnessChatResult{SessionID: "session-1", TurnID: "turn-1", Delivery: string(conversation.DeliveryDelivered)}, nil
		},
	}
	svc, err := NewService(NewServiceParams{
		Repository:             repo,
		Conversations:          conversations,
		HarnessChatSender:      harness,
		AutoStartAgentDelivery: true,
		Clock:                  FixedClock{T: now},
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	defer svc.StopAgentDelivery("member-dest")

	if _, err := svc.PublishAgentMessage(context.Background(), domain.NewMessageInput{
		ID: "msg-auto-start",
		Route: domain.MessageRoute{
			SpaceID:             "space-1",
			DestinationMemberID: "member-dest",
			ChannelID:           "channel:space-1:member:member-dest",
		},
		Content: domain.MessageContent{
			Kind:    types.AgentMessageKindSystem,
			Subject: "System wake",
			Body:    map[string]any{"text": "wake from system message"},
		},
		Producer: domain.MessageProducer{IntentID: "intent-auto-start", Producer: "test"},
	}); err != nil {
		t.Fatalf("PublishAgentMessage: %v", err)
	}

	select {
	case input := <-delivered:
		if input.MemberID != "member-dest" || !strings.Contains(input.Text, "wake from system message") {
			t.Fatalf("delivered input=%+v", input)
		}
	case <-time.After(time.Second):
		t.Fatal("expected publish to start agent delivery")
	}
	msg, err := svc.GetMessage(context.Background(), "msg-auto-start")
	if err != nil {
		t.Fatalf("GetMessage: %v", err)
	}
	if msg.Status != types.MessageStatusConsumedTyped || msg.ConsumedBy != "member-dest" {
		t.Fatalf("message=%+v", msg)
	}
}

func TestServicePublishAgentMessageRestartsExitedDeliveryWorker(t *testing.T) {
	now := time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC)
	repo := newMemoryRepo()
	conversations := newMemoryConversationRepo()
	delivered := make(chan HarnessChatMessage, 1)
	harness := &fakeHarnessChatSender{
		sendFn: func(ctx context.Context, input HarnessChatMessage) (HarnessChatResult, error) {
			delivered <- input
			return HarnessChatResult{SessionID: "session-1", TurnID: "turn-1", Delivery: string(conversation.DeliveryDelivered)}, nil
		},
	}
	svc, err := NewService(NewServiceParams{
		Repository:             repo,
		Conversations:          conversations,
		HarnessChatSender:      harness,
		AutoStartAgentDelivery: true,
		Clock:                  FixedClock{T: now},
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	staleCtx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := svc.StartAgentDelivery(staleCtx, "member-dest"); err == nil {
		t.Fatal("StartAgentDelivery returned nil for canceled context")
	}
	deadline := time.Now().Add(time.Second)
	for {
		svc.agentDeliveriesMu.Lock()
		workerCount := len(svc.agentDeliveries)
		svc.agentDeliveriesMu.Unlock()
		if workerCount == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("delivery worker registry kept exited worker")
		}
		time.Sleep(10 * time.Millisecond)
	}
	defer svc.StopAgentDelivery("member-dest")

	if _, err := svc.PublishAgentMessage(context.Background(), domain.NewMessageInput{
		ID: "msg-restart-exited-worker",
		Route: domain.MessageRoute{
			SpaceID:             "space-1",
			DestinationMemberID: "member-dest",
			ChannelID:           "channel:space-1:member:member-dest",
		},
		Content: domain.MessageContent{
			Kind:    types.AgentMessageKindInform,
			Subject: "Restart worker",
			Body:    map[string]any{"text": "wake after stale worker"},
		},
		Producer: domain.MessageProducer{IntentID: "intent-restart-exited-worker", Producer: "test"},
	}); err != nil {
		t.Fatalf("PublishAgentMessage: %v", err)
	}

	select {
	case input := <-delivered:
		if input.MemberID != "member-dest" || !strings.Contains(input.Text, "wake after stale worker") {
			t.Fatalf("delivered input=%+v", input)
		}
	case <-time.After(time.Second):
		t.Fatal("expected publish to restart exited delivery worker")
	}
	msg, err := svc.GetMessage(context.Background(), "msg-restart-exited-worker")
	if err != nil {
		t.Fatalf("GetMessage: %v", err)
	}
	if msg.Status != types.MessageStatusConsumedTyped || msg.ConsumedBy != "member-dest" {
		t.Fatalf("message=%+v", msg)
	}
}

func TestServiceStartAgentDeliveryDrainsQueuedWakeBufferedBeforeWorkerStarts(t *testing.T) {
	now := time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC)
	repo := newMemoryRepo()
	conversations := newMemoryConversationRepo()
	delivered := make(chan HarnessChatMessage, 1)
	harness := &fakeHarnessChatSender{
		sendFn: func(ctx context.Context, input HarnessChatMessage) (HarnessChatResult, error) {
			delivered <- input
			return HarnessChatResult{SessionID: "session-1", TurnID: "turn-1", Delivery: string(conversation.DeliveryDelivered)}, nil
		},
	}
	svc := newServiceWithHarnessForTest(t, repo, conversations, harness, now)

	if _, err := svc.PublishAgentMessage(context.Background(), domain.NewMessageInput{
		ID: "msg-before-start",
		Route: domain.MessageRoute{
			SpaceID:             "space-1",
			DestinationMemberID: "member-dest",
			ChannelID:           "channel:space-1:member:member-dest",
		},
		Content: domain.MessageContent{
			Kind:    types.AgentMessageKindInform,
			Subject: "Fresh member",
			Body:    map[string]any{"text": "first message after spawn"},
		},
		Producer: domain.MessageProducer{IntentID: "intent-1", Producer: "test"},
	}); err != nil {
		t.Fatalf("PublishAgentMessage: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := svc.StartAgentDelivery(ctx, "member-dest"); err != nil {
		t.Fatalf("StartAgentDelivery: %v", err)
	}
	defer svc.StopAgentDelivery("member-dest")

	select {
	case input := <-delivered:
		if input.MemberID != "member-dest" || !strings.Contains(input.Text, "first message after spawn") {
			t.Fatalf("delivered input=%+v", input)
		}
	case <-time.After(time.Second):
		t.Fatal("expected buffered queued wake to drain when delivery worker starts")
	}
	msg, err := svc.GetMessage(context.Background(), "msg-before-start")
	if err != nil {
		t.Fatalf("GetMessage: %v", err)
	}
	if msg.Status != types.MessageStatusConsumedTyped || msg.ConsumedBy != "member-dest" {
		t.Fatalf("message=%+v", msg)
	}
}

func TestServiceStartAgentDeliveryRetriesQueuedMessageAfterHarnessBusy(t *testing.T) {
	previousDelay := agentDeliveryRetryDelay
	agentDeliveryRetryDelay = 10 * time.Millisecond
	t.Cleanup(func() { agentDeliveryRetryDelay = previousDelay })

	now := time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC)
	repo := newMemoryRepo()
	conversations := newMemoryConversationRepo()
	attempts := make(chan int, 2)
	attempt := 0
	harness := &fakeHarnessChatSender{
		sendFn: func(ctx context.Context, input HarnessChatMessage) (HarnessChatResult, error) {
			attempt++
			attempts <- attempt
			if attempt == 1 {
				return HarnessChatResult{}, fmt.Errorf("harness session %q already has active run %q", "session-1", "run-1")
			}
			return HarnessChatResult{SessionID: "session-1", TurnID: "turn-1", Delivery: string(conversation.DeliveryDelivered)}, nil
		},
	}
	svc := newServiceWithHarnessForTest(t, repo, conversations, harness, now)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := svc.StartAgentDelivery(ctx, "member-dest"); err != nil {
		t.Fatalf("StartAgentDelivery: %v", err)
	}
	defer svc.StopAgentDelivery("member-dest")

	if _, err := svc.PublishAgentMessage(context.Background(), domain.NewMessageInput{
		ID: "msg-busy",
		Route: domain.MessageRoute{
			SpaceID:             "space-1",
			DestinationMemberID: "member-dest",
			ChannelID:           "channel:space-1:member:member-dest",
		},
		Content: domain.MessageContent{
			Kind:    types.AgentMessageKindSystem,
			Subject: "Task canceled",
			Body:    map[string]any{"message": "Task canceled"},
		},
		Producer: domain.MessageProducer{IntentID: "intent-busy", Producer: "test"},
	}); err != nil {
		t.Fatalf("PublishAgentMessage: %v", err)
	}

	for want := 1; want <= 2; want++ {
		select {
		case got := <-attempts:
			if got != want {
				t.Fatalf("attempt=%d want %d", got, want)
			}
		case <-time.After(time.Second):
			t.Fatalf("expected delivery attempt %d", want)
		}
	}
	deadline := time.Now().Add(time.Second)
	for {
		msg, err := svc.GetMessage(context.Background(), "msg-busy")
		if err != nil {
			t.Fatalf("GetMessage: %v", err)
		}
		if msg.Status == types.MessageStatusConsumedTyped && msg.ConsumedBy == "member-dest" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("message=%+v", msg)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestServiceDeliverNextAgentMessageDefersUnreachableTurnAndAllowsFreshMessage(t *testing.T) {
	previousDelay := agentDeliveryRetryDelay
	agentDeliveryRetryDelay = 10 * time.Millisecond
	t.Cleanup(func() { agentDeliveryRetryDelay = previousDelay })

	now := time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC)
	repo := newMemoryRepo()
	conversations := newMemoryConversationRepo()
	attempts := make(chan string, 2)
	harness := &fakeHarnessChatSender{
		sendFn: func(ctx context.Context, input HarnessChatMessage) (HarnessChatResult, error) {
			if strings.Contains(input.Text, "old unreachable") {
				attempts <- "old"
				return HarnessChatResult{}, fmt.Errorf("steer active codex turn: codex app-server thread %q is not loaded by the reachable remote-control server", "thread-1")
			}
			attempts <- "fresh"
			return HarnessChatResult{SessionID: "session-1", TurnID: "turn-1", Delivery: string(conversation.DeliveryDelivered)}, nil
		},
	}
	svc := newServiceWithHarnessForTest(t, repo, conversations, harness, now)

	for _, input := range []domain.NewMessageInput{
		{
			ID: "msg-a-old",
			Route: domain.MessageRoute{
				SpaceID:             "space-1",
				DestinationMemberID: "member-dest",
				ChannelID:           "channel:space-1:member:member-dest",
			},
			Content: domain.MessageContent{
				Kind:    types.AgentMessageKindSystem,
				Subject: "Old",
				Body:    map[string]any{"message": "old unreachable"},
			},
			Producer: domain.MessageProducer{IntentID: "intent-old", Producer: "test"},
		},
		{
			ID: "msg-z-fresh",
			Route: domain.MessageRoute{
				SpaceID:             "space-1",
				DestinationMemberID: "member-dest",
				ChannelID:           "channel:space-1:member:member-dest",
			},
			Content: domain.MessageContent{
				Kind:    types.AgentMessageKindSystem,
				Subject: "Fresh",
				Body:    map[string]any{"message": "fresh reachable"},
			},
			Producer: domain.MessageProducer{IntentID: "intent-fresh", Producer: "test"},
		},
	} {
		if _, err := svc.PublishAgentMessage(context.Background(), input); err != nil {
			t.Fatalf("PublishAgentMessage(%s): %v", input.ID, err)
		}
	}

	if _, err := svc.DeliverNextAgentMessage(context.Background(), "member-dest"); err == nil {
		t.Fatal("expected first delivery to report unreachable turn")
	} else if !isUnavailableTurnDeliveryError(err) {
		t.Fatalf("expected unreachable-turn error, got %v", err)
	}
	if _, err := svc.DeliverNextAgentMessage(context.Background(), "member-dest"); err != nil {
		t.Fatalf("DeliverNextAgentMessage fresh: %v", err)
	}

	if got := <-attempts; got != "old" {
		t.Fatalf("first attempt=%q want old", got)
	}
	if got := <-attempts; got != "fresh" {
		t.Fatalf("second attempt=%q want fresh", got)
	}
	oldMsg, err := svc.GetMessage(context.Background(), "msg-a-old")
	if err != nil {
		t.Fatalf("GetMessage old: %v", err)
	}
	if oldMsg.Status != types.MessageStatusQueuedTyped || !oldMsg.VisibleAt.Equal(now.Add(agentDeliveryRetryDelay)) {
		t.Fatalf("old message should stay queued with deferred visibility: %+v", oldMsg)
	}
	freshMsg, err := svc.GetMessage(context.Background(), "msg-z-fresh")
	if err != nil {
		t.Fatalf("GetMessage fresh: %v", err)
	}
	if freshMsg.Status != types.MessageStatusConsumedTyped || freshMsg.ConsumedBy != "member-dest" {
		t.Fatalf("fresh message should be consumed: %+v", freshMsg)
	}
}

func TestAgentDeliveryRetryClassifiesTransientStorageErrors(t *testing.T) {
	for _, err := range []error{
		fmt.Errorf("load task task-1 for notification validation: database is locked"),
		fmt.Errorf("next queued message for member-1: database table is locked"),
		fmt.Errorf("mark message consumed msg-1: database is busy"),
	} {
		if !isRetryableAgentDeliveryError(err) {
			t.Fatalf("expected retryable error: %v", err)
		}
	}
	if isRetryableAgentDeliveryError(fmt.Errorf("parse task notification updatedAt: bad timestamp")) {
		t.Fatal("programmer/data-shape errors should stay non-retryable")
	}
}

func TestServiceChannelMethodsUseChannelDomain(t *testing.T) {
	now := time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC)
	repo := newMemoryRepo()
	svc := newServiceForTest(t, repo, now)

	ch, err := svc.EnsureMemberChannel(context.Background(), NewMemberChannelParams{
		SpaceID:   "space-1",
		ProjectID: "project-1",
		MemberID:  "member-1",
	})
	if err != nil {
		t.Fatalf("EnsureMemberChannel: %v", err)
	}
	if ch.ID != "channel:space-1:member:member-1" || ch.MemberLabel != "" || ch.Title != "" || ch.RunID != "" {
		t.Fatalf("channel = %+v", ch)
	}
	closed, err := svc.CloseChannel(context.Background(), ch.ID)
	if err != nil {
		t.Fatalf("CloseChannel: %v", err)
	}
	if closed.Status != types.ChannelStatusClosed {
		t.Fatalf("closed status = %q", closed.Status)
	}
	if err := svc.RecordChannelActivity(context.Background(), ch.ID); err != nil {
		t.Fatalf("RecordChannelActivity: %v", err)
	}
	counts, err := svc.UnreadCountsByChannel(context.Background(), "user-1", []types.ChannelID{ch.ID})
	if err != nil {
		t.Fatalf("UnreadCountsByChannel: %v", err)
	}
	if counts[ch.ID] != 0 {
		t.Fatalf("unread count=%d want 0 without messages", counts[ch.ID])
	}
	if err := svc.MarkChannelRead(context.Background(), "user-1", ch.ID); err != nil {
		t.Fatalf("MarkChannelRead: %v", err)
	}
}

func TestServiceSendConversationMessagePersistsInboundRow(t *testing.T) {
	now := time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC)
	repo := newMemoryRepo()
	conversations := newMemoryConversationRepo()
	harness := &fakeHarnessChatSender{}
	svc := newServiceWithHarnessForTest(t, repo, conversations, harness, now)
	ch, err := svc.EnsureMemberChannel(context.Background(), NewMemberChannelParams{
		SpaceID:  "space-1",
		MemberID: "member-1",
	})
	if err != nil {
		t.Fatalf("EnsureMemberChannel: %v", err)
	}

	msg, err := svc.SendConversationMessage(context.Background(), SendConversationMessageParams{
		ChannelID:  ch.ID,
		SenderType: "user",
		SenderID:   "user-1",
		Text:       "please help",
	})
	if err != nil {
		t.Fatalf("SendConversationMessage: %v", err)
	}
	if msg.ChannelID != string(ch.ID) || msg.MemberID != "member-1" || msg.SpaceID != "space-1" {
		t.Fatalf("message route = %+v", msg)
	}
	if msg.Direction != conversation.DirectionInbound || msg.Delivery != conversation.DeliveryDelivered || msg.Render != conversation.RenderVisible {
		t.Fatalf("message state = %+v", msg)
	}
	if msg.SessionID != "session-1" || msg.TurnID != "turn-1" {
		t.Fatalf("delivery binding = session %q turn %q", msg.SessionID, msg.TurnID)
	}
	if harness.input.SpaceID != "space-1" || harness.input.MemberID != "member-1" || harness.input.ChannelID != string(ch.ID) || harness.input.Text != "please help" {
		t.Fatalf("harness input = %+v", harness.input)
	}
	got, err := conversations.ListByChannel(context.Background(), string(ch.ID), 10)
	if err != nil {
		t.Fatalf("ListByChannel: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("stored messages = %+v", got)
	}
	if got[0].Text != "please help" || got[0].Delivery != conversation.DeliveryDelivered {
		t.Fatalf("stored messages = %+v", got)
	}
	if got[1].Direction != conversation.DirectionOutbound || got[1].Text != "assistant response" || got[1].SessionID != "session-1" || got[1].TurnID != "turn-1" {
		t.Fatalf("outbound message = %+v", got[1])
	}
	loaded, err := svc.LoadChannel(context.Background(), ch.ID)
	if err != nil {
		t.Fatalf("LoadChannel: %v", err)
	}
	wantLastMessageAt := now.Add(1)
	if loaded.LastMessageAt == nil || !loaded.LastMessageAt.Equal(wantLastMessageAt) {
		t.Fatalf("last message at = %v, want %s", loaded.LastMessageAt, wantLastMessageAt)
	}
}

func TestServiceSendConversationMessageLeavesUserMessageQueuedBehindActiveRun(t *testing.T) {
	now := time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC)
	repo := newMemoryRepo()
	conversations := newMemoryConversationRepo()
	harness := &fakeHarnessChatSender{err: fmt.Errorf(`harness session "session-1" already has active run "run-1"`)}
	svc := newServiceWithHarnessForTest(t, repo, conversations, harness, now)
	ch, err := svc.EnsureMemberChannel(context.Background(), NewMemberChannelParams{
		SpaceID:  "space-1",
		MemberID: "member-1",
	})
	if err != nil {
		t.Fatalf("EnsureMemberChannel: %v", err)
	}

	msg, err := svc.SendConversationMessage(context.Background(), SendConversationMessageParams{
		ChannelID:  ch.ID,
		SenderType: "user",
		SenderID:   "user-1",
		Text:       "please use this direction",
	})
	if err != nil {
		t.Fatalf("SendConversationMessage: %v", err)
	}
	if msg.Delivery != conversation.DeliveryQueued || msg.Error != "" {
		t.Fatalf("message state = %+v", msg)
	}
	if harness.input.AllowSteering {
		t.Fatalf("normal conversation send allowed steering")
	}
	got, err := conversations.ListByChannel(context.Background(), string(ch.ID), 10)
	if err != nil {
		t.Fatalf("ListByChannel: %v", err)
	}
	if len(got) != 1 || got[0].Delivery != conversation.DeliveryQueued {
		t.Fatalf("stored messages = %+v", got)
	}
}

func TestServiceSteerConversationMessageRequiresQueuedUserMessageAndAllowsSteering(t *testing.T) {
	now := time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC)
	repo := newMemoryRepo()
	conversations := newMemoryConversationRepo()
	harness := &fakeHarnessChatSender{err: fmt.Errorf(`harness session "session-1" already has active run "run-1"`)}
	svc := newServiceWithHarnessForTest(t, repo, conversations, harness, now)
	ch, err := svc.EnsureMemberChannel(context.Background(), NewMemberChannelParams{
		SpaceID:  "space-1",
		MemberID: "member-1",
	})
	if err != nil {
		t.Fatalf("EnsureMemberChannel: %v", err)
	}
	msg, err := svc.SendConversationMessage(context.Background(), SendConversationMessageParams{
		ChannelID:  ch.ID,
		SenderType: "user",
		SenderID:   "user-1",
		Text:       "steer with this",
	})
	if err != nil {
		t.Fatalf("SendConversationMessage: %v", err)
	}

	harness.err = nil
	harness.sendFn = func(_ context.Context, input HarnessChatMessage) (HarnessChatResult, error) {
		if !input.AllowSteering {
			return HarnessChatResult{}, fmt.Errorf("steering was not allowed")
		}
		if input.ConversationMessageID != msg.ID || input.Text != "steer with this" || input.SenderType != "user" {
			return HarnessChatResult{}, fmt.Errorf("unexpected input = %+v", input)
		}
		return HarnessChatResult{SessionID: "session-1", RunID: "run-1", TurnID: "turn-1", Delivery: string(conversation.DeliverySteered)}, nil
	}

	steered, err := svc.SteerConversationMessage(context.Background(), msg.ID)
	if err != nil {
		t.Fatalf("SteerConversationMessage: %v", err)
	}
	if steered.Delivery != conversation.DeliverySteered || steered.SessionID != "session-1" || steered.TurnID != "turn-1" {
		t.Fatalf("steered message = %+v", steered)
	}
}

func TestServiceSendConversationMessageStreamsAssistantDeltasToOneOutboundRow(t *testing.T) {
	now := time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC)
	repo := newMemoryRepo()
	conversations := newMemoryConversationRepo()
	harness := &fakeHarnessChatSender{
		sendFn: func(_ context.Context, input HarnessChatMessage) (HarnessChatResult, error) {
			if err := input.Stream.AppendAssistantDelta(context.Background(), HarnessAssistantDelta{
				TurnID: "turn-stream",
				Text:   "hello ",
			}); err != nil {
				return HarnessChatResult{}, err
			}
			if err := input.Stream.AppendAssistantDelta(context.Background(), HarnessAssistantDelta{
				TurnID: "turn-stream",
				Text:   "world",
			}); err != nil {
				return HarnessChatResult{}, err
			}
			return HarnessChatResult{SessionID: "session-1", TurnID: "turn-stream", Delivery: string(conversation.DeliveryDelivered)}, nil
		},
	}
	svc := newServiceWithHarnessForTest(t, repo, conversations, harness, now)
	ch, err := svc.EnsureMemberChannel(context.Background(), NewMemberChannelParams{
		SpaceID:  "space-1",
		MemberID: "member-1",
	})
	if err != nil {
		t.Fatalf("EnsureMemberChannel: %v", err)
	}

	_, err = svc.SendConversationMessage(context.Background(), SendConversationMessageParams{
		ChannelID:  ch.ID,
		SenderType: "user",
		SenderID:   "user-1",
		Text:       "stream please",
	})
	if err != nil {
		t.Fatalf("SendConversationMessage: %v", err)
	}

	got, err := conversations.ListByChannel(context.Background(), string(ch.ID), 10)
	if err != nil {
		t.Fatalf("ListByChannel: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("stored messages = %+v", got)
	}
	outbound := got[1]
	if outbound.Direction != conversation.DirectionOutbound || outbound.Text != "hello world" {
		t.Fatalf("streamed outbound = %+v", outbound)
	}
	if outbound.SessionID != "session-1" || outbound.TurnID != "turn-stream" {
		t.Fatalf("streamed outbound binding = session %q turn %q", outbound.SessionID, outbound.TurnID)
	}
}

func TestServiceSendConversationMessagePersistsThinkingDeltasAsConversationActivity(t *testing.T) {
	now := time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC)
	repo := newMemoryRepo()
	conversations := newMemoryConversationRepo()
	harness := &fakeHarnessChatSender{
		sendFn: func(_ context.Context, input HarnessChatMessage) (HarnessChatResult, error) {
			if err := input.Stream.AppendThinkingDelta(context.Background(), HarnessThinkingDelta{
				SessionID: "session-1",
				TurnID:    "turn-stream",
				Sequence:  1,
				Text:      "Checking the plan",
				Data:      map[string]string{"itemId": "reason-1", "kind": "reasoning"},
			}); err != nil {
				return HarnessChatResult{}, err
			}
			if err := input.Stream.AppendAssistantDelta(context.Background(), HarnessAssistantDelta{
				SessionID: "session-1",
				TurnID:    "turn-stream",
				Sequence:  2,
				Text:      "done",
			}); err != nil {
				return HarnessChatResult{}, err
			}
			return HarnessChatResult{SessionID: "session-1", TurnID: "turn-stream", Delivery: string(conversation.DeliveryDelivered)}, nil
		},
	}
	svc := newServiceWithHarnessForTest(t, repo, conversations, harness, now)
	ch, err := svc.EnsureMemberChannel(context.Background(), NewMemberChannelParams{
		SpaceID:  "space-1",
		MemberID: "member-1",
	})
	if err != nil {
		t.Fatalf("EnsureMemberChannel: %v", err)
	}

	_, err = svc.SendConversationMessage(context.Background(), SendConversationMessageParams{
		ChannelID:  ch.ID,
		SenderType: "user",
		SenderID:   "user-1",
		Text:       "stream please",
	})
	if err != nil {
		t.Fatalf("SendConversationMessage: %v", err)
	}

	messages, err := conversations.ListByChannel(context.Background(), string(ch.ID), 10)
	if err != nil {
		t.Fatalf("ListByChannel: %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("stored messages = %+v", messages)
	}
	if messages[1].Direction != conversation.DirectionOutbound || messages[1].Text != "done" {
		t.Fatalf("assistant message = %+v", messages[1])
	}
	activities, err := conversations.ListActivitiesByChannel(context.Background(), string(ch.ID), 10)
	if err != nil {
		t.Fatalf("ListActivitiesByChannel: %v", err)
	}
	if len(activities) != 1 {
		t.Fatalf("activities = %+v", activities)
	}
	got := activities[0]
	if got.Kind != "thinking" || got.Text != "Checking the plan" || got.Status != "completed" {
		t.Fatalf("thinking activity = %+v", got)
	}
	if got.ToolCallID != "thinking-reason-1" || got.Data["itemId"] != "reason-1" || got.Data["kind"] != "reasoning" {
		t.Fatalf("thinking identity/data = %+v data=%+v", got, got.Data)
	}
}

func TestServiceSendConversationMessageKeepsInboundDeliveredWhenHarnessClosesAfterResponse(t *testing.T) {
	now := time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC)
	repo := newMemoryRepo()
	conversations := newMemoryConversationRepo()
	harness := &fakeHarnessChatSender{
		sendFn: func(_ context.Context, input HarnessChatMessage) (HarnessChatResult, error) {
			if err := input.Stream.AppendAssistantDelta(context.Background(), HarnessAssistantDelta{
				SessionID: "session-1",
				TurnID:    "turn-stream",
				Text:      "I queued that message.",
			}); err != nil {
				return HarnessChatResult{}, err
			}
			return HarnessChatResult{}, fmt.Errorf("failed to read JSON message: failed to get reader: failed to read frame header: EOF")
		},
	}
	svc := newServiceWithHarnessForTest(t, repo, conversations, harness, now)
	ch, err := svc.EnsureMemberChannel(context.Background(), NewMemberChannelParams{
		SpaceID:  "space-1",
		MemberID: "member-1",
	})
	if err != nil {
		t.Fatalf("EnsureMemberChannel: %v", err)
	}

	msg, err := svc.SendConversationMessage(context.Background(), SendConversationMessageParams{
		ChannelID:  ch.ID,
		SenderType: "user",
		SenderID:   "user-1",
		Text:       "send Fred a note",
	})
	if err != nil {
		t.Fatalf("SendConversationMessage: %v", err)
	}
	if msg.Delivery != conversation.DeliveryDelivered || msg.Error != "" {
		t.Fatalf("message=%+v", msg)
	}
	got, err := conversations.ListByChannel(context.Background(), string(ch.ID), 10)
	if err != nil {
		t.Fatalf("ListByChannel: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("stored messages=%+v", got)
	}
	if got[0].Delivery != conversation.DeliveryDelivered || got[0].Error != "" {
		t.Fatalf("inbound=%+v", got[0])
	}
	if got[1].Direction != conversation.DirectionOutbound || got[1].Text != "I queued that message." {
		t.Fatalf("outbound=%+v", got[1])
	}
}

func TestServiceSendConversationMessagePersistsHarnessActivities(t *testing.T) {
	now := time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC)
	repo := newMemoryRepo()
	conversations := newMemoryConversationRepo()
	harness := &fakeHarnessChatSender{
		sendFn: func(_ context.Context, input HarnessChatMessage) (HarnessChatResult, error) {
			if err := input.Stream.AppendActivity(context.Background(), HarnessActivity{
				SessionID:  "session-1",
				TurnID:     "turn-stream",
				ToolCallID: "call-1",
				ToolName:   "agen8/space",
				Status:     "in_progress",
				Data:       map[string]string{"input": `{"action":"members"}`},
			}); err != nil {
				return HarnessChatResult{}, err
			}
			if err := input.Stream.AppendActivity(context.Background(), HarnessActivity{
				SessionID:  "session-1",
				TurnID:     "turn-stream",
				ToolCallID: "call-1",
				ToolName:   "agen8/space",
				Status:     "completed",
				Text:       "OK",
				Data:       map[string]string{"result": "OK"},
			}); err != nil {
				return HarnessChatResult{}, err
			}
			return HarnessChatResult{SessionID: "session-1", TurnID: "turn-stream", Delivery: string(conversation.DeliveryDelivered), Text: "done"}, nil
		},
	}
	svc := newServiceWithHarnessForTest(t, repo, conversations, harness, now)
	ch, err := svc.EnsureMemberChannel(context.Background(), NewMemberChannelParams{
		SpaceID:  "space-1",
		MemberID: "member-1",
	})
	if err != nil {
		t.Fatalf("EnsureMemberChannel: %v", err)
	}

	_, err = svc.SendConversationMessage(context.Background(), SendConversationMessageParams{
		ChannelID:  ch.ID,
		SenderType: "user",
		SenderID:   "user-1",
		Text:       "list members",
	})
	if err != nil {
		t.Fatalf("SendConversationMessage: %v", err)
	}

	activities, err := svc.ListConversationActivities(context.Background(), ch.ID, 10)
	if err != nil {
		t.Fatalf("ListConversationActivities: %v", err)
	}
	if len(activities) != 1 {
		t.Fatalf("activities = %+v", activities)
	}
	got := activities[0]
	if got.ChannelID != string(ch.ID) || got.SessionID != "session-1" || got.TurnID != "turn-stream" || got.ToolCallID != "call-1" {
		t.Fatalf("activity identity = %+v", got)
	}
	if got.Status != "completed" || got.Title != "agen8/space" || got.Text != "OK" {
		t.Fatalf("activity state = %+v", got)
	}
	if got.Data["sourceType"] != "mcp" || got.Data["server"] != "agen8" || got.Data["domain"] != "space" {
		t.Fatalf("activity data = %+v", got.Data)
	}
	if got.Data["input"] != `{"action":"members"}` {
		t.Fatalf("activity lost original input data = %+v", got.Data)
	}
	if got.Sequence != 1 || got.Data["seq"] != "1" {
		t.Fatalf("activity sequence = %d data=%+v", got.Sequence, got.Data)
	}
}

func TestServiceSendConversationMessageAppendsCommandOutputDeltas(t *testing.T) {
	now := time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC)
	repo := newMemoryRepo()
	conversations := newMemoryConversationRepo()
	harness := &fakeHarnessChatSender{
		sendFn: func(_ context.Context, input HarnessChatMessage) (HarnessChatResult, error) {
			if err := input.Stream.AppendActivity(context.Background(), HarnessActivity{
				SessionID:  "session-1",
				TurnID:     "turn-stream",
				ToolCallID: "call-1",
				ToolName:   "bash",
				Status:     "in_progress",
				Data: map[string]string{
					"command": "/usr/bin/zsh -lc 'printf hello'",
				},
			}); err != nil {
				return HarnessChatResult{}, err
			}
			if err := input.Stream.AppendActivity(context.Background(), HarnessActivity{
				SessionID:  "session-1",
				TurnID:     "turn-stream",
				ToolCallID: "call-1",
				ToolName:   "bash",
				Status:     "in_progress",
				Text:       "hello ",
				Data: map[string]string{
					"outputDelta":   "true",
					"stdout":        "hello ",
					"result":        "hello ",
					"outputFull":    "hello ",
					"outputPreview": "hello ",
				},
			}); err != nil {
				return HarnessChatResult{}, err
			}
			if err := input.Stream.AppendActivity(context.Background(), HarnessActivity{
				SessionID:  "session-1",
				TurnID:     "turn-stream",
				ToolCallID: "call-1",
				ToolName:   "bash",
				Status:     "in_progress",
				Text:       "world\n",
				Data: map[string]string{
					"outputDelta":   "true",
					"stdout":        "world\n",
					"result":        "world\n",
					"outputFull":    "world\n",
					"outputPreview": "world\n",
				},
			}); err != nil {
				return HarnessChatResult{}, err
			}
			if err := input.Stream.AppendActivity(context.Background(), HarnessActivity{
				SessionID:  "session-1",
				TurnID:     "turn-stream",
				ToolCallID: "call-1",
				ToolName:   "bash",
				Status:     "completed",
				Data: map[string]string{
					"exitCode": "0",
				},
			}); err != nil {
				return HarnessChatResult{}, err
			}
			return HarnessChatResult{SessionID: "session-1", TurnID: "turn-stream", Delivery: string(conversation.DeliveryDelivered), Text: "done"}, nil
		},
	}
	svc := newServiceWithHarnessForTest(t, repo, conversations, harness, now)
	ch, err := svc.EnsureMemberChannel(context.Background(), NewMemberChannelParams{
		SpaceID:  "space-1",
		MemberID: "member-1",
	})
	if err != nil {
		t.Fatalf("EnsureMemberChannel: %v", err)
	}

	_, err = svc.SendConversationMessage(context.Background(), SendConversationMessageParams{
		ChannelID:  ch.ID,
		SenderType: "user",
		SenderID:   "user-1",
		Text:       "run command",
	})
	if err != nil {
		t.Fatalf("SendConversationMessage: %v", err)
	}

	activities, err := svc.ListConversationActivities(context.Background(), ch.ID, 10)
	if err != nil {
		t.Fatalf("ListConversationActivities: %v", err)
	}
	if len(activities) != 1 {
		t.Fatalf("activities = %+v", activities)
	}
	got := activities[0]
	if got.Status != "completed" || got.Data["exitCode"] != "0" {
		t.Fatalf("activity state = %+v data=%+v", got, got.Data)
	}
	if got.Data["outputFull"] != "hello world\n" || got.Data["stdout"] != "hello world\n" || got.Data["result"] != "hello world\n" {
		t.Fatalf("activity output not accumulated: %+v", got.Data)
	}
	if got.Data["command"] != "/usr/bin/zsh -lc 'printf hello'" {
		t.Fatalf("activity lost command data: %+v", got.Data)
	}
}

func TestServiceSendConversationMessagePreservesCommandUpdateOutputOnCompletion(t *testing.T) {
	now := time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC)
	repo := newMemoryRepo()
	conversations := newMemoryConversationRepo()
	harness := &fakeHarnessChatSender{
		sendFn: func(_ context.Context, input HarnessChatMessage) (HarnessChatResult, error) {
			if err := input.Stream.AppendActivity(context.Background(), HarnessActivity{
				SessionID:  "session-1",
				TurnID:     "turn-stream",
				ToolCallID: "call-1",
				ToolName:   "bash",
				Status:     "in_progress",
				Data: map[string]string{
					"command": "/usr/bin/zsh -lc 'date && whoami'",
				},
			}); err != nil {
				return HarnessChatResult{}, err
			}
			if err := input.Stream.AppendActivity(context.Background(), HarnessActivity{
				SessionID:  "session-1",
				TurnID:     "turn-stream",
				ToolCallID: "call-1",
				ToolName:   "bash",
				Status:     "in_progress",
				Text:       "exec_command test\n",
				Data: map[string]string{
					"stdout":        "exec_command test\n",
					"result":        "exec_command test\n",
					"outputFull":    "exec_command test\n",
					"outputPreview": "exec_command test\n",
				},
			}); err != nil {
				return HarnessChatResult{}, err
			}
			if err := input.Stream.AppendActivity(context.Background(), HarnessActivity{
				SessionID:  "session-1",
				TurnID:     "turn-stream",
				ToolCallID: "call-1",
				ToolName:   "bash",
				Status:     "completed",
				Data: map[string]string{
					"exitCode": "0",
				},
			}); err != nil {
				return HarnessChatResult{}, err
			}
			return HarnessChatResult{SessionID: "session-1", TurnID: "turn-stream", Delivery: string(conversation.DeliveryDelivered), Text: "done"}, nil
		},
	}
	svc := newServiceWithHarnessForTest(t, repo, conversations, harness, now)
	ch, err := svc.EnsureMemberChannel(context.Background(), NewMemberChannelParams{
		SpaceID:  "space-1",
		MemberID: "member-1",
	})
	if err != nil {
		t.Fatalf("EnsureMemberChannel: %v", err)
	}

	_, err = svc.SendConversationMessage(context.Background(), SendConversationMessageParams{
		ChannelID:  ch.ID,
		SenderType: "user",
		SenderID:   "user-1",
		Text:       "run command",
	})
	if err != nil {
		t.Fatalf("SendConversationMessage: %v", err)
	}

	activities, err := svc.ListConversationActivities(context.Background(), ch.ID, 10)
	if err != nil {
		t.Fatalf("ListConversationActivities: %v", err)
	}
	if len(activities) != 1 {
		t.Fatalf("activities = %+v", activities)
	}
	got := activities[0]
	if got.Status != "completed" || got.Data["exitCode"] != "0" {
		t.Fatalf("activity state = %+v data=%+v", got, got.Data)
	}
	if got.Data["outputFull"] != "exec_command test\n" || got.Data["stdout"] != "exec_command test\n" {
		t.Fatalf("activity output was not preserved: %+v", got.Data)
	}
}

func TestServiceSendConversationMessageSegmentsAssistantTextAroundToolActivity(t *testing.T) {
	now := time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC)
	repo := newMemoryRepo()
	conversations := newMemoryConversationRepo()
	harness := &fakeHarnessChatSender{
		sendFn: func(_ context.Context, input HarnessChatMessage) (HarnessChatResult, error) {
			if err := input.Stream.AppendAssistantDelta(context.Background(), HarnessAssistantDelta{
				SessionID: "session-1",
				TurnID:    "turn-stream",
				Sequence:  1,
				Text:      "before",
			}); err != nil {
				return HarnessChatResult{}, err
			}
			if err := input.Stream.AppendActivity(context.Background(), HarnessActivity{
				SessionID:  "session-1",
				TurnID:     "turn-stream",
				ToolCallID: "call-1",
				ToolName:   "agen8/mission",
				Sequence:   2,
				Status:     "completed",
				Text:       "OK",
			}); err != nil {
				return HarnessChatResult{}, err
			}
			if err := input.Stream.AppendAssistantDelta(context.Background(), HarnessAssistantDelta{
				SessionID: "session-1",
				TurnID:    "turn-stream",
				Sequence:  3,
				Text:      "after",
			}); err != nil {
				return HarnessChatResult{}, err
			}
			return HarnessChatResult{SessionID: "session-1", TurnID: "turn-stream", Delivery: string(conversation.DeliveryDelivered)}, nil
		},
	}
	svc := newServiceWithHarnessForTest(t, repo, conversations, harness, now)
	ch, err := svc.EnsureMemberChannel(context.Background(), NewMemberChannelParams{
		SpaceID:  "space-1",
		MemberID: "member-1",
	})
	if err != nil {
		t.Fatalf("EnsureMemberChannel: %v", err)
	}

	_, err = svc.SendConversationMessage(context.Background(), SendConversationMessageParams{
		ChannelID:  ch.ID,
		SenderType: "user",
		SenderID:   "user-1",
		Text:       "work",
	})
	if err != nil {
		t.Fatalf("SendConversationMessage: %v", err)
	}

	messages, err := conversations.ListByChannel(context.Background(), string(ch.ID), 10)
	if err != nil {
		t.Fatalf("ListByChannel: %v", err)
	}
	if len(messages) != 3 {
		t.Fatalf("messages = %+v", messages)
	}
	if messages[1].Text != "before" || messages[2].Text != "after" {
		t.Fatalf("assistant segments = %+v", messages)
	}
	if !messages[1].CreatedAt.Before(messages[2].CreatedAt) {
		t.Fatalf("assistant segment order = %s then %s", messages[1].CreatedAt, messages[2].CreatedAt)
	}
	activities, err := conversations.ListActivitiesByChannel(context.Background(), string(ch.ID), 10)
	if err != nil {
		t.Fatalf("ListActivitiesByChannel: %v", err)
	}
	if len(activities) != 1 {
		t.Fatalf("activities = %+v", activities)
	}
	if activities[0].Sequence != 2 {
		t.Fatalf("activity sequence = %d", activities[0].Sequence)
	}
	if !(messages[1].CreatedAt.Before(activities[0].CreatedAt) && activities[0].CreatedAt.Before(messages[2].CreatedAt)) {
		t.Fatalf("timeline order: before=%s activity=%s after=%s", messages[1].CreatedAt, activities[0].CreatedAt, messages[2].CreatedAt)
	}
}

func TestServiceSendConversationMessageEnsuresDeterministicMemberChannel(t *testing.T) {
	now := time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC)
	repo := newMemoryRepo()
	conversations := newMemoryConversationRepo()
	harness := &fakeHarnessChatSender{}
	svc := newServiceWithHarnessForTest(t, repo, conversations, harness, now)

	msg, err := svc.SendConversationMessage(context.Background(), SendConversationMessageParams{
		ChannelID:  "channel:space-1:member:member-1",
		SenderType: "user",
		SenderID:   "user-1",
		Text:       "first message",
	})
	if err != nil {
		t.Fatalf("SendConversationMessage: %v", err)
	}
	if msg.ChannelID != "channel:space-1:member:member-1" || msg.SpaceID != "space-1" || msg.MemberID != "member-1" {
		t.Fatalf("message route = %+v", msg)
	}
	if harness.input.SpaceID != "space-1" || harness.input.MemberID != "member-1" || harness.input.Text != "first message" {
		t.Fatalf("harness input = %+v", harness.input)
	}
	ch, err := svc.LoadChannel(context.Background(), "channel:space-1:member:member-1")
	if err != nil {
		t.Fatalf("LoadChannel: %v", err)
	}
	wantLastMessageAt := now.Add(1)
	if ch.LastMessageAt == nil || !ch.LastMessageAt.Equal(wantLastMessageAt) {
		t.Fatalf("last message at = %v, want %s", ch.LastMessageAt, wantLastMessageAt)
	}
}

func TestServiceSendConversationMessageRequiresConversationRepo(t *testing.T) {
	now := time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC)
	repo := newMemoryRepo()
	svc := newServiceForTest(t, repo, now)

	_, err := svc.SendConversationMessage(context.Background(), SendConversationMessageParams{
		ChannelID:  "channel:space-1:member:member-1",
		SenderType: "user",
		Text:       "hello",
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestServiceUploadConversationAttachmentAndSendToHarness(t *testing.T) {
	now := time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC)
	repo := newMemoryRepo()
	conversations := newMemoryConversationRepo()
	harness := &fakeHarnessChatSender{}
	svc := newServiceWithHarnessForTest(t, repo, conversations, harness, now)
	projectRoot := t.TempDir()
	svc.SetProjectRootResolver(fakeProjectRootResolver{roots: map[types.ProjectID]string{"project-1": projectRoot}})
	_, err := svc.EnsureMemberChannel(context.Background(), NewMemberChannelParams{
		SpaceID:   "space-1",
		ProjectID: "project-1",
		MemberID:  "member-1",
	})
	if err != nil {
		t.Fatalf("EnsureMemberChannel: %v", err)
	}

	attachment, err := svc.UploadConversationAttachment(context.Background(), UploadConversationAttachmentParams{
		ChannelID: "channel:space-1:member:member-1",
		Name:      "../screenshot.png",
		MediaType: "image/png",
		Bytes:     []byte("png-bytes"),
	})
	if err != nil {
		t.Fatalf("UploadConversationAttachment: %v", err)
	}
	if attachment.Name != "screenshot.png" {
		t.Fatalf("attachment name=%q", attachment.Name)
	}
	if _, err := os.Stat(attachment.URI); err != nil {
		t.Fatalf("attachment file not written: %v", err)
	}
	if filepath.Dir(filepath.Dir(attachment.URI)) != filepath.Join(projectRoot, ".agen8", "conversation-attachments") {
		t.Fatalf("attachment URI=%q not under project attachment root", attachment.URI)
	}

	msg, err := svc.SendConversationMessage(context.Background(), SendConversationMessageParams{
		ChannelID:     "channel:space-1:member:member-1",
		SenderType:    "user",
		SenderID:      "user-1",
		AttachmentIDs: []string{attachment.ID},
	})
	if err != nil {
		t.Fatalf("SendConversationMessage: %v", err)
	}
	if len(msg.Attachments) != 1 || msg.Attachments[0].ID != attachment.ID {
		t.Fatalf("message attachments = %+v", msg.Attachments)
	}
	if len(harness.input.Attachments) != 1 || harness.input.Attachments[0].URI != attachment.URI {
		t.Fatalf("harness attachments = %+v", harness.input.Attachments)
	}
}

func TestServiceGetConversationAttachmentReturnsStoredBytes(t *testing.T) {
	now := time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC)
	repo := newMemoryRepo()
	conversations := newMemoryConversationRepo()
	svc := newServiceWithHarnessForTest(t, repo, conversations, &fakeHarnessChatSender{}, now)
	projectRoot := t.TempDir()
	svc.SetProjectRootResolver(fakeProjectRootResolver{roots: map[types.ProjectID]string{"project-1": projectRoot}})
	if _, err := svc.EnsureMemberChannel(context.Background(), NewMemberChannelParams{
		SpaceID:   "space-1",
		ProjectID: "project-1",
		MemberID:  "member-1",
	}); err != nil {
		t.Fatalf("EnsureMemberChannel: %v", err)
	}

	attachment, err := svc.UploadConversationAttachment(context.Background(), UploadConversationAttachmentParams{
		ChannelID: "channel:space-1:member:member-1",
		Name:      "screen.png",
		MediaType: "image/png",
		Bytes:     []byte("png-bytes"),
	})
	if err != nil {
		t.Fatalf("UploadConversationAttachment: %v", err)
	}

	got, err := svc.GetConversationAttachment(context.Background(), attachment.ID)
	if err != nil {
		t.Fatalf("GetConversationAttachment: %v", err)
	}
	if got.Attachment.ID != attachment.ID || string(got.Bytes) != "png-bytes" {
		t.Fatalf("got attachment blob = %+v bytes=%q", got.Attachment, string(got.Bytes))
	}
}

func TestServiceGetConversationAttachmentRejectsMissingID(t *testing.T) {
	now := time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC)
	repo := newMemoryRepo()
	conversations := newMemoryConversationRepo()
	svc := newServiceWithHarnessForTest(t, repo, conversations, &fakeHarnessChatSender{}, now)

	_, err := svc.GetConversationAttachment(context.Background(), "")
	if err == nil {
		t.Fatal("GetConversationAttachment returned nil error for missing id")
	}
}

func TestServiceUploadConversationAttachmentUsesRequestProjectForLegacyChannel(t *testing.T) {
	now := time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC)
	repo := newMemoryRepo()
	conversations := newMemoryConversationRepo()
	svc := newServiceWithHarnessForTest(t, repo, conversations, &fakeHarnessChatSender{}, now)
	projectRoot := t.TempDir()
	svc.SetProjectRootResolver(fakeProjectRootResolver{roots: map[types.ProjectID]string{"project-1": projectRoot}})
	if _, err := svc.EnsureMemberChannel(context.Background(), NewMemberChannelParams{
		SpaceID:  "space-1",
		MemberID: "member-1",
	}); err != nil {
		t.Fatalf("EnsureMemberChannel: %v", err)
	}

	attachment, err := svc.UploadConversationAttachment(context.Background(), UploadConversationAttachmentParams{
		ChannelID: "channel:space-1:member:member-1",
		ProjectID: "project-1",
		Name:      "screen.png",
		MediaType: "image/png",
		Bytes:     []byte("png-bytes"),
	})
	if err != nil {
		t.Fatalf("UploadConversationAttachment: %v", err)
	}
	if attachment.ProjectID != "project-1" {
		t.Fatalf("attachment project=%q want project-1", attachment.ProjectID)
	}
}

func newServiceForTest(t *testing.T, repo *memoryRepo, now time.Time) *Service {
	return newServiceWithConversationsForTest(t, repo, nil, now)
}

func newServiceWithConversationsForTest(t *testing.T, repo *memoryRepo, conversations conversation.Repository, now time.Time) *Service {
	return newServiceWithHarnessForTest(t, repo, conversations, nil, now)
}

func newServiceWithHarnessForTest(t *testing.T, repo *memoryRepo, conversations conversation.Repository, harness HarnessChatSender, now time.Time) *Service {
	t.Helper()
	svc, err := NewService(NewServiceParams{Repository: repo, Conversations: conversations, HarnessChatSender: harness, Clock: FixedClock{T: now}})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return svc
}

type fakeHarnessChatSender struct {
	input  HarnessChatMessage
	err    error
	sendFn func(context.Context, HarnessChatMessage) (HarnessChatResult, error)
}

type fakeProjectRootResolver struct {
	roots map[types.ProjectID]string
}

func (r fakeProjectRootResolver) ResolveProjectRoot(_ context.Context, projectID types.ProjectID) (string, error) {
	root := strings.TrimSpace(r.roots[projectID])
	if root == "" {
		return "", fmt.Errorf("project %s not found", projectID)
	}
	return root, nil
}

type fakeTaskStateReader struct {
	tasks map[taskdomain.TaskID]taskdomain.Task
}

func (r fakeTaskStateReader) Get(_ context.Context, taskID taskdomain.TaskID) (taskdomain.Task, error) {
	task, ok := r.tasks[taskID]
	if !ok {
		return taskdomain.Task{}, fmt.Errorf("task %s not found", taskID)
	}
	return task, nil
}

type mutableClock struct {
	now time.Time
}

func (c *mutableClock) Now() time.Time {
	return c.now
}

func (f *fakeHarnessChatSender) SendMessage(ctx context.Context, input HarnessChatMessage) (HarnessChatResult, error) {
	f.input = input
	if f.sendFn != nil {
		return f.sendFn(ctx, input)
	}
	if f.err != nil {
		return HarnessChatResult{}, f.err
	}
	return HarnessChatResult{SessionID: "session-1", TurnID: "turn-1", Delivery: string(conversation.DeliveryDelivered), Text: "assistant response"}, nil
}

type spyConversationNotifier struct {
	messages []conversation.Message
}

func (s *spyConversationNotifier) NotifyConversationChanged(_ context.Context, msg conversation.Message) error {
	s.messages = append(s.messages, msg)
	return nil
}

type memoryRepo struct {
	messages map[types.AgentMessageID]types.AgentMessage
	channels map[types.ChannelID]types.Channel
	reads    map[string]map[types.ChannelID]time.Time
}

func newMemoryRepo() *memoryRepo {
	return &memoryRepo{
		messages: map[types.AgentMessageID]types.AgentMessage{},
		channels: map[types.ChannelID]types.Channel{},
		reads:    map[string]map[types.ChannelID]time.Time{},
	}
}

func (r *memoryRepo) SaveQueued(_ context.Context, msg types.AgentMessage) (types.AgentMessage, error) {
	r.messages[msg.ID] = msg.Normalized()
	return r.messages[msg.ID], nil
}

func (r *memoryRepo) NextQueuedForMember(_ context.Context, memberID member.ID, now time.Time) (types.AgentMessage, error) {
	messages := make([]types.AgentMessage, 0, len(r.messages))
	for _, msg := range r.messages {
		if msg.DestinationMemberID == memberID && msg.Status == types.MessageStatusQueuedTyped && !msg.VisibleAt.After(now) {
			messages = append(messages, msg)
		}
	}
	sort.Slice(messages, func(i, j int) bool {
		if !messages[i].VisibleAt.Equal(messages[j].VisibleAt) {
			return messages[i].VisibleAt.Before(messages[j].VisibleAt)
		}
		if !messages[i].CreatedAt.Equal(messages[j].CreatedAt) {
			return messages[i].CreatedAt.Before(messages[j].CreatedAt)
		}
		return messages[i].ID < messages[j].ID
	})
	if len(messages) > 0 {
		return messages[0], nil
	}
	return types.AgentMessage{}, domain.ErrMessageNotFound
}

func (r *memoryRepo) DeferQueued(_ context.Context, messageID types.AgentMessageID, visibleAt time.Time, updatedAt time.Time) (types.AgentMessage, error) {
	msg, ok := r.messages[messageID]
	if !ok {
		return types.AgentMessage{}, domain.ErrMessageNotFound
	}
	if msg.Status != types.MessageStatusQueuedTyped {
		return types.AgentMessage{}, domain.ErrConsumed
	}
	msg.VisibleAt = visibleAt.UTC()
	msg.UpdatedAt = updatedAt.UTC()
	r.messages[messageID] = msg.Normalized()
	return r.messages[messageID], nil
}

func (r *memoryRepo) MarkConsumed(_ context.Context, msg types.AgentMessage) (types.AgentMessage, error) {
	r.messages[msg.ID] = msg.Normalized()
	return r.messages[msg.ID], nil
}

func (r *memoryRepo) Get(_ context.Context, id types.AgentMessageID) (types.AgentMessage, error) {
	msg, ok := r.messages[id]
	if !ok {
		return types.AgentMessage{}, domain.ErrMessageNotFound
	}
	return msg, nil
}

func (r *memoryRepo) List(context.Context, domain.MessageFilter) ([]types.AgentMessage, error) {
	out := make([]types.AgentMessage, 0, len(r.messages))
	for _, msg := range r.messages {
		out = append(out, msg)
	}
	return out, nil
}

func (r *memoryRepo) Count(context.Context, domain.MessageFilter) (int, error) {
	return len(r.messages), nil
}

func (r *memoryRepo) Save(_ context.Context, ch types.Channel) (types.Channel, error) {
	ch = channel.WrapChannel(ch).Inner()
	r.channels[ch.ID] = ch
	return ch, nil
}

func (r *memoryRepo) Load(_ context.Context, channelID types.ChannelID) (types.Channel, error) {
	ch, ok := r.channels[channelID]
	if !ok {
		return types.Channel{}, channel.ErrNotFound
	}
	return ch, nil
}

func (r *memoryRepo) LoadMemberChannel(ctx context.Context, spaceID spacedomain.SpaceID, memberID member.ID) (types.Channel, error) {
	return r.Load(ctx, channel.MemberChannelID(spaceID, memberID))
}

func (r *memoryRepo) ListBySpace(_ context.Context, spaceID spacedomain.SpaceID) ([]types.Channel, error) {
	out := []types.Channel{}
	for _, ch := range r.channels {
		if ch.SpaceID == spaceID {
			out = append(out, ch)
		}
	}
	return out, nil
}

func (r *memoryRepo) RecordActivity(_ context.Context, channelID types.ChannelID, at time.Time) error {
	ch, ok := r.channels[channelID]
	if !ok {
		return channel.ErrNotFound
	}
	next, err := channel.WrapChannel(ch).MarkActivity(at)
	if err != nil {
		return err
	}
	r.channels[channelID] = next.Inner()
	return nil
}

func (r *memoryRepo) MarkRead(_ context.Context, userID string, channelID types.ChannelID, at time.Time) error {
	if r.reads[userID] == nil {
		r.reads[userID] = map[types.ChannelID]time.Time{}
	}
	r.reads[userID][channelID] = at
	return nil
}

func (r *memoryRepo) DeleteForMember(_ context.Context, spaceID spacedomain.SpaceID, memberID member.ID) error {
	id := channel.MemberChannelID(spaceID, memberID)
	if _, ok := r.channels[id]; !ok {
		return channel.ErrNotFound
	}
	delete(r.channels, id)
	return nil
}

func (r *memoryRepo) UnreadCountsByChannel(_ context.Context, userID string, channelIDs []types.ChannelID) (map[types.ChannelID]int, error) {
	out := map[types.ChannelID]int{}
	for _, id := range channelIDs {
		seen := r.reads[userID][id]
		for _, msg := range r.messages {
			if msg.ChannelID != id {
				continue
			}
			if seen.IsZero() || seen.Before(msg.CreatedAt) {
				out[id]++
			}
		}
	}
	return out, nil
}

type memoryConversationRepo struct {
	messages    map[string]conversation.Message
	activities  map[string]conversation.Activity
	attachments map[string]conversation.Attachment
}

func newMemoryConversationRepo() *memoryConversationRepo {
	return &memoryConversationRepo{
		messages:    map[string]conversation.Message{},
		activities:  map[string]conversation.Activity{},
		attachments: map[string]conversation.Attachment{},
	}
}

func (r *memoryConversationRepo) Save(_ context.Context, msg conversation.Message) error {
	if err := conversation.ValidateMessage(msg); err != nil {
		return err
	}
	r.messages[msg.ID] = msg
	return nil
}

func (r *memoryConversationRepo) SaveActivity(_ context.Context, activity conversation.Activity) error {
	if err := conversation.ValidateActivity(activity); err != nil {
		return err
	}
	existing := r.activities[activity.ID]
	if !existing.CreatedAt.IsZero() {
		activity.CreatedAt = existing.CreatedAt
	}
	r.activities[activity.ID] = activity
	return nil
}

func (r *memoryConversationRepo) SaveAttachment(_ context.Context, attachment conversation.Attachment) error {
	if err := conversation.ValidateAttachment(attachment); err != nil {
		return err
	}
	r.attachments[attachment.ID] = attachment
	return nil
}

func (r *memoryConversationRepo) AppendText(_ context.Context, id string, delta string, updatedAt time.Time) (conversation.Message, error) {
	msg, ok := r.messages[id]
	if !ok {
		return conversation.Message{}, domain.ErrMessageNotFound
	}
	msg.Text += delta
	msg.UpdatedAt = updatedAt
	r.messages[id] = msg
	return msg, nil
}

func (r *memoryConversationRepo) UpdateDelivery(_ context.Context, id string, state conversation.DeliveryState, errText string, updatedAt time.Time) (conversation.Message, error) {
	return r.UpdateDeliveryBinding(context.Background(), id, state, "", "", errText, updatedAt)
}

func (r *memoryConversationRepo) UpdateDeliveryBinding(_ context.Context, id string, state conversation.DeliveryState, sessionID string, turnID string, errText string, updatedAt time.Time) (conversation.Message, error) {
	msg, ok := r.messages[id]
	if !ok {
		return conversation.Message{}, domain.ErrMessageNotFound
	}
	msg.Delivery = state
	msg.SessionID = sessionID
	msg.TurnID = turnID
	msg.Error = errText
	msg.UpdatedAt = updatedAt
	r.messages[id] = msg
	return msg, nil
}

func (r *memoryConversationRepo) UpdateRender(_ context.Context, id string, state conversation.RenderState, errText string, updatedAt time.Time) (conversation.Message, error) {
	msg, ok := r.messages[id]
	if !ok {
		return conversation.Message{}, domain.ErrMessageNotFound
	}
	msg.Render = state
	msg.Error = errText
	msg.UpdatedAt = updatedAt
	r.messages[id] = msg
	return msg, nil
}

func (r *memoryConversationRepo) Get(_ context.Context, id string) (*conversation.Message, error) {
	msg, ok := r.messages[strings.TrimSpace(id)]
	if !ok {
		return nil, nil
	}
	cp := msg
	return &cp, nil
}

func (r *memoryConversationRepo) ListByChannel(_ context.Context, channelID string, _ int) ([]conversation.Message, error) {
	var out []conversation.Message
	for _, msg := range r.messages {
		if msg.ChannelID == channelID {
			out = append(out, msg)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out, nil
}

func (r *memoryConversationRepo) ListActivitiesByChannel(_ context.Context, channelID string, _ int) ([]conversation.Activity, error) {
	var out []conversation.Activity
	for _, activity := range r.activities {
		if activity.ChannelID == channelID {
			out = append(out, activity)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Sequence != out[j].Sequence {
			return out[i].Sequence < out[j].Sequence
		}
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out, nil
}

func (r *memoryConversationRepo) NextQueuedForSession(_ context.Context, sessionID string) (*conversation.Message, error) {
	for _, msg := range r.messages {
		if msg.SessionID == sessionID && msg.Direction == conversation.DirectionInbound && msg.Delivery == conversation.DeliveryQueued {
			cp := msg
			return &cp, nil
		}
	}
	return nil, nil
}

func (r *memoryConversationRepo) GetAttachments(_ context.Context, ids []string) ([]conversation.Attachment, error) {
	out := make([]conversation.Attachment, 0, len(ids))
	for _, id := range ids {
		attachment, ok := r.attachments[strings.TrimSpace(id)]
		if !ok {
			return nil, fmt.Errorf("attachment %s not found", id)
		}
		out = append(out, attachment)
	}
	return out, nil
}
