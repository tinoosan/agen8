package message

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/tinoosan/agen8-mcp-server/internal/caller"
	messagedomain "github.com/tinoosan/agen8-mcp-server/internal/services/message/domain"
	"github.com/tinoosan/agen8-mcp-server/internal/services/space/domain/member"
	"github.com/tinoosan/agen8-mcp-server/pkg/types"
)

var fixedNow = time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)

type stubMemberDirectory struct {
	getFn func(context.Context, member.ID) (member.Record, error)
}

func (s stubMemberDirectory) GetMember(ctx context.Context, id member.ID) (member.Record, error) {
	if s.getFn != nil {
		return s.getFn(ctx, id)
	}
	return member.Record{
		ID:             id,
		SpaceID:        "space-1",
		DisplayName:    string(id),
		LifecycleState: member.LifecycleActive,
	}, nil
}

type stubPublisher struct {
	publishFn func(context.Context, messagedomain.NewMessageInput) (types.AgentMessage, error)
}

func (s stubPublisher) PublishAgentMessage(ctx context.Context, input messagedomain.NewMessageInput) (types.AgentMessage, error) {
	if s.publishFn != nil {
		return s.publishFn(ctx, input)
	}
	msg, err := messagedomain.NewMessage(input, fixedNow)
	if err != nil {
		return types.AgentMessage{}, err
	}
	return msg.Inner(), nil
}

func TestDecodeRejectsMissingAction(t *testing.T) {
	_, err := decode(json.RawMessage(`{"action":"","destination_member_id":"member-dest","kind":"inform","subject":"S","body":"B"}`))
	if err == nil || !strings.Contains(err.Error(), "action is required") {
		t.Fatalf("unexpected err=%v", err)
	}
}

func TestDecodeRejectsMissingDestinationMember(t *testing.T) {
	_, err := decode(json.RawMessage(`{"action":"send","kind":"inform","subject":"S","body":"B"}`))
	if err == nil || !strings.Contains(err.Error(), "destination_member_id is required") {
		t.Fatalf("unexpected err=%v", err)
	}
}

func TestDecodeRequiresCorrelationForAck(t *testing.T) {
	_, err := decode(json.RawMessage(`{"action":"send","destination_member_id":"member-dest","kind":"ack","subject":"S","body":"B"}`))
	if err == nil || !strings.Contains(err.Error(), "correlation_id is required") {
		t.Fatalf("unexpected err=%v", err)
	}
}

func TestDecodeRejectsNullField(t *testing.T) {
	_, err := decode(json.RawMessage(`{"action":"send","destination_member_id":"member-dest","kind":"inform","subject":"S","body":"B","correlation_id":null}`))
	if err == nil || !strings.Contains(err.Error(), `field "correlation_id" must be omitted instead of null`) {
		t.Fatalf("unexpected err=%v", err)
	}
}

func TestHandleSendPublishesAgentMessage(t *testing.T) {
	handler := NewHandler()
	result, err := handler.Handle(context.Background(), CallContext{
		ActorMemberID: "member-source",
		SpaceID:       "space-1",
		Members: stubMemberDirectory{
			getFn: func(ctx context.Context, id member.ID) (member.Record, error) {
				switch id {
				case "member-source":
					resolved, err := (caller.ContextResolver{}).ResolveCaller(ctx)
					if err != nil {
						t.Fatalf("source lookup missing caller: %v", err)
					}
					if resolved.MemberID != id || resolved.SpaceID != "space-1" {
						t.Fatalf("caller=%+v want member-source in space-1", resolved)
					}
					return member.Record{ID: id, SpaceID: "space-1", DisplayName: "Source", LifecycleState: member.LifecycleActive}, nil
				case "member-dest":
					return member.Record{ID: id, SpaceID: "space-1", DisplayName: "Destination", LifecycleState: member.LifecycleActive}, nil
				default:
					t.Fatalf("unexpected member id=%q", id)
					return member.Record{}, nil
				}
			},
		},
		Messages: stubPublisher{
			publishFn: func(_ context.Context, input messagedomain.NewMessageInput) (types.AgentMessage, error) {
				if input.Route.SourceMemberID != "member-source" {
					t.Fatalf("source member id=%q", input.Route.SourceMemberID)
				}
				if input.Route.DestinationMemberID != "member-dest" {
					t.Fatalf("destination member id=%q", input.Route.DestinationMemberID)
				}
				if input.Route.ChannelID != "channel:space-1:member:member-dest" {
					t.Fatalf("channel id=%q", input.Route.ChannelID)
				}
				if input.Content.Body["text"] != "Please check this" {
					t.Fatalf("body=%+v", input.Content.Body)
				}
				msg, err := messagedomain.NewMessage(input, fixedNow)
				if err != nil {
					return types.AgentMessage{}, err
				}
				return msg.Inner(), nil
			},
		},
	}, json.RawMessage(`{"action":"send","destination_member_id":"member-dest","kind":"inform","subject":"Review","body":"Please check this"}`))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	structured, ok := result.Structured.(map[string]any)
	if !ok {
		t.Fatalf("structured type %T", result.Structured)
	}
	if structured["tool"] != Name || structured["action"] != "send" {
		t.Fatalf("unexpected structured=%+v", structured)
	}
	if structured["destinationMemberId"] != "member-dest" || structured["destinationMemberLabel"] != "Destination" {
		t.Fatalf("destination fields=%+v", structured)
	}
	guidance, _ := structured["guidance"].(string)
	if !strings.Contains(guidance, "asynchronous") || !strings.Contains(guidance, "do not assume") {
		t.Fatalf("guidance=%q", guidance)
	}
	if !strings.Contains(result.Text, "asynchronous") {
		t.Fatalf("text result missing guidance: %s", result.Text)
	}
	if _, exists := structured["destinationRunId"]; exists {
		t.Fatalf("structured contains run id: %+v", structured)
	}
}

func TestContextWithSessionActorStampsMemberCaller(t *testing.T) {
	ctx := contextWithSessionActor(context.Background(), "member-source", "space-1")
	resolved, err := (caller.ContextResolver{}).ResolveCaller(ctx)
	if err != nil {
		t.Fatalf("ResolveCaller: %v", err)
	}
	if resolved.MemberID != "member-source" || resolved.SpaceID != "space-1" {
		t.Fatalf("caller=%+v want member-source in space-1", resolved)
	}
}

func TestHandleSendRejectsCrossSpaceDestination(t *testing.T) {
	handler := NewHandler()
	_, err := handler.Handle(context.Background(), CallContext{
		ActorMemberID: "member-source",
		SpaceID:       "space-1",
		Members: stubMemberDirectory{
			getFn: func(_ context.Context, id member.ID) (member.Record, error) {
				if id == "member-dest" {
					return member.Record{ID: id, SpaceID: "space-2", DisplayName: "Destination", LifecycleState: member.LifecycleActive}, nil
				}
				return member.Record{ID: id, SpaceID: "space-1", DisplayName: "Source", LifecycleState: member.LifecycleActive}, nil
			},
		},
		Messages: stubPublisher{},
	}, json.RawMessage(`{"action":"send","destination_member_id":"member-dest","kind":"inform","subject":"Review","body":"Please check this"}`))
	if err == nil || !strings.Contains(err.Error(), "belongs to space") {
		t.Fatalf("unexpected err=%v", err)
	}
}

func TestSchemaRequiresOnlySendFields(t *testing.T) {
	var schema struct {
		Properties map[string]json.RawMessage `json:"properties"`
		Required   []string                   `json:"required"`
	}
	if err := json.Unmarshal(NewHandler().Schema(), &schema); err != nil {
		t.Fatalf("schema: %v", err)
	}
	assertRequiredFields(t, schema.Required, []string{"action", "destination_member_id", "kind", "subject", "body"})
	correlation := map[string]any{}
	if err := json.Unmarshal(schema.Properties["correlation_id"], &correlation); err != nil {
		t.Fatalf("correlation schema: %v", err)
	}
	if _, ok := correlation["anyOf"]; ok {
		t.Fatalf("correlation_id should not be nullable: %+v", correlation)
	}
}

func assertRequiredFields(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("required=%v want %v", got, want)
	}
	seen := make(map[string]bool, len(got))
	for _, field := range got {
		seen[field] = true
	}
	for _, field := range want {
		if !seen[field] {
			t.Fatalf("required=%v missing %q", got, field)
		}
	}
}
