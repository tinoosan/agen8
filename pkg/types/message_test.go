package types

import (
	"testing"
	"time"
)

func TestMessageFromMemberMessageDropsQueueFields(t *testing.T) {
	now := time.Date(2026, 3, 13, 11, 0, 0, 0, time.UTC)
	processed := now.Add(time.Minute)
	msg := MemberMessage{
		MessageID:      "msg-1",
		IntentID:       "intent-1",
		CorrelationID:  "corr-1",
		SpaceID:        "space-1",
		RunID:          "run-1",
		ActorMemberID:  "member-a",
		TargetMemberID: "member-b",
		Channel:        MessageChannelInbox,
		Kind:           MessageKindInform,
		Body:           map[string]any{"subject": "Heads up", "body": "details here"},
		TaskRef:        "task-1",
		Status:         MessageStatusQueued,
		LeaseOwner:     "worker-1",
		Attempts:       3,
		VisibleAt:      now,
		Priority:       7,
		Error:          "problem",
		Metadata:       map[string]any{"source": "rpc"},
		CreatedAt:      &now,
		UpdatedAt:      &now,
		ProcessedAt:    &processed,
	}

	projected := MessageFromMemberMessage(msg)

	if projected.MessageID != "msg-1" || projected.CorrelationID != "corr-1" {
		t.Fatalf("unexpected ids: %+v", projected)
	}
	if projected.SourceSpaceID != "space-1" || projected.DestinationSpaceID != "space-1" {
		t.Fatalf("unexpected space projection: %+v", projected)
	}
	if projected.SourceMemberID != "member-a" || projected.DestinationMemberID != "member-b" {
		t.Fatalf("unexpected member projection: %+v", projected)
	}
	if projected.Kind != MessageKindInform || projected.TaskRef != "task-1" {
		t.Fatalf("unexpected payload projection: %+v", projected)
	}
	if projected.Subject != "Heads up" || projected.Body != "details here" {
		t.Fatalf("unexpected body projection: subject=%q body=%q", projected.Subject, projected.Body)
	}
	if projected.CreatedAt != &now || projected.ProcessedAt != &processed {
		t.Fatalf("timestamps not preserved")
	}
	if _, ok := projected.Metadata["source"]; !ok {
		t.Fatalf("metadata not projected")
	}
}

func TestMessageFromMemberMessage_WithStructuredMessage(t *testing.T) {
	now := time.Date(2026, 3, 13, 11, 0, 0, 0, time.UTC)
	msg := MemberMessage{
		MessageID:      "msg-2",
		CorrelationID:  "corr-2",
		SpaceID:        "space-2",
		ActorMemberID:  "member-a",
		TargetMemberID: "member-b",
		Kind:           MessageKindInform,
		Message: &Message{
			MessageID:   "msg-2",
			SenderType:  SenderTypeCoordinator,
			SenderName:  "head-analyst",
			SenderSpace: "space-2",
			Subject:     "Analysis complete",
			Body:        "The report is ready",
		},
		Status:    MessageStatusQueued,
		CreatedAt: &now,
	}

	projected := MessageFromMemberMessage(msg)

	if projected.SenderType != SenderTypeCoordinator {
		t.Fatalf("expected coordinator sender, got %q", projected.SenderType)
	}
	if projected.SenderName != "head-analyst" {
		t.Fatalf("expected head-analyst sender, got %q", projected.SenderName)
	}
	if projected.SenderSpace != "space-2" {
		t.Fatalf("expected space-2 sender space, got %q", projected.SenderSpace)
	}
	if projected.Subject != "Analysis complete" || projected.Body != "The report is ready" {
		t.Fatalf("body not projected from Message: subject=%q body=%q", projected.Subject, projected.Body)
	}
}
