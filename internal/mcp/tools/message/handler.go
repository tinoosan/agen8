package message

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/tinoosan/agen8-mcp-server/internal/caller"
	spacedomain "github.com/tinoosan/agen8-mcp-server/internal/services/space/domain"

	"github.com/google/uuid"
	messagedomain "github.com/tinoosan/agen8-mcp-server/internal/services/message/domain"
	messagechannel "github.com/tinoosan/agen8-mcp-server/internal/services/message/domain/channel"
	"github.com/tinoosan/agen8-mcp-server/internal/services/space/domain/member"
	"github.com/tinoosan/agen8-mcp-server/pkg/types"
)

type Handler struct{}

func NewHandler() Handler {
	return Handler{}
}

func (h Handler) Handle(ctx context.Context, call CallContext, args json.RawMessage) (Result, error) {
	input, err := decode(args)
	if err != nil {
		return Result{}, err
	}
	if call.Members == nil {
		return Result{}, fmt.Errorf("message: member service is not configured")
	}
	if call.Messages == nil {
		return Result{}, fmt.Errorf("message: message service is not configured")
	}
	ctx = contextWithSessionActor(ctx, call.ActorMemberID, call.SpaceID)
	actor, err := h.actor(ctx, call)
	if err != nil {
		return Result{}, err
	}
	switch input.Action {
	case "send":
		return h.send(ctx, call, actor, input)
	case "inbox":
		return h.inbox(ctx, call, actor, input)
	default:
		return Result{}, fmt.Errorf("message: unsupported action %q", input.Action)
	}
}

func (h Handler) send(ctx context.Context, call CallContext, actor memberRef, input requestInput) (Result, error) {
	destination, err := h.destination(ctx, call, actor, input.DestinationMemberID)
	if err != nil {
		return Result{}, err
	}
	correlationID := input.CorrelationID
	if correlationID == "" {
		correlationID = types.CorrelationID("corr-" + uuid.NewString())
	}
	channelID := messagechannel.MemberChannelID(destination.SpaceID, destination.ID)
	published, err := call.Messages.PublishAgentMessage(ctx, messagedomain.NewMessageInput{
		Route: messagedomain.MessageRoute{
			SpaceID:             destination.SpaceID,
			SourceMemberID:      actor.ID,
			DestinationMemberID: destination.ID,
			ChannelID:           channelID,
		},
		Content: messagedomain.MessageContent{
			Kind:    input.Kind,
			Subject: input.Subject,
			Body: map[string]any{
				"text": input.Body,
			},
		},
		Producer: messagedomain.MessageProducer{
			IntentID:      types.IntentID("mcp.message.send:" + uuid.NewString()),
			CorrelationID: correlationID,
			Producer:      "mcp.message",
		},
		Metadata: map[string]any{
			"sourceMemberLabel":      actor.Label,
			"destinationMemberLabel": destination.Label,
		},
	})
	if err != nil {
		return Result{}, fmt.Errorf("message: publish: %w", err)
	}
	structured := map[string]any{
		"ok":                     true,
		"tool":                   Name,
		"action":                 "send",
		"guidance":               "Message delivery is asynchronous. The message is queued for the destination member; do not assume they have seen or acted on it until a later response, acknowledgment, or task/message state confirms it.",
		"messageId":              string(published.ID),
		"channelId":              string(published.ChannelID),
		"sourceMemberId":         string(actor.ID),
		"sourceMemberLabel":      actor.Label,
		"destinationMemberId":    string(destination.ID),
		"destinationMemberLabel": destination.Label,
		"kind":                   string(published.Kind),
		"subject":                published.Subject,
		"correlationId":          string(published.CorrelationID),
		"status":                 string(published.Status),
	}
	text, err := encodeText(structured)
	if err != nil {
		return Result{}, err
	}
	return Result{Text: text, Structured: structured}, nil
}

func (h Handler) inbox(ctx context.Context, call CallContext, actor memberRef, input requestInput) (Result, error) {
	limit := input.Limit
	if limit <= 0 {
		limit = 10
	}
	if limit > 50 {
		limit = 50
	}
	filter := messagedomain.MessageFilter{
		SpaceID:             actor.SpaceID,
		DestinationMemberID: actor.ID,
		Limit:               limit,
	}
	if input.Status != "" {
		filter.Statuses = []types.MessageStatus{input.Status}
	}
	messages, err := call.Messages.ListMessages(ctx, filter)
	if err != nil {
		return Result{}, fmt.Errorf("message: list inbox: %w", err)
	}
	items := make([]map[string]any, 0, len(messages))
	for _, msg := range messages {
		items = append(items, map[string]any{
			"messageId":           string(msg.ID),
			"spaceId":             string(msg.SpaceID),
			"channelId":           string(msg.ChannelID),
			"sourceMemberId":      string(msg.SourceMemberID),
			"destinationMemberId": string(msg.DestinationMemberID),
			"kind":                string(msg.Kind),
			"subject":             msg.Subject,
			"body":                msg.Body,
			"producer":            strings.TrimSpace(msg.Producer),
			"correlationId":       string(msg.CorrelationID),
			"taskRef":             string(msg.TaskRef),
			"status":              string(msg.Status),
			"visibleAt":           msg.VisibleAt.Format(time.RFC3339Nano),
			"createdAt":           msg.CreatedAt.Format(time.RFC3339Nano),
		})
	}
	structured := map[string]any{
		"ok":            true,
		"tool":          Name,
		"action":        "inbox",
		"memberId":      string(actor.ID),
		"memberLabel":   actor.Label,
		"spaceId":       string(actor.SpaceID),
		"status":        string(input.Status),
		"limit":         limit,
		"count":         len(items),
		"messages":      items,
		"guidance":      "This is the durable Agen8 inbox for the current member. Treat queued system/task messages as work for this registered runtime identity.",
		"deliveryModel": "pull; native push remains optional when the harness exposes a live delivery channel",
	}
	text, err := encodeText(structured)
	if err != nil {
		return Result{}, err
	}
	return Result{Text: text, Structured: structured}, nil
}

func contextWithSessionActor(ctx context.Context, actorMemberID, spaceID string) context.Context {
	actorMemberID = strings.TrimSpace(actorMemberID)
	spaceID = strings.TrimSpace(spaceID)
	if actorMemberID == "" && spaceID == "" {
		return ctx
	}
	return caller.ContextWithCaller(ctx, caller.Caller{
		MemberID: member.ID(actorMemberID),
		SpaceID:  spacedomain.SpaceID(spaceID),
	})
}

func (h Handler) actor(ctx context.Context, call CallContext) (memberRef, error) {
	memberID := member.ID(strings.TrimSpace(call.ActorMemberID))
	if memberID == "" {
		return memberRef{}, fmt.Errorf("message: caller member is required")
	}
	member, err := h.loadActiveMember(ctx, call, memberID, "caller")
	if err != nil {
		return memberRef{}, err
	}
	spaceID := spacedomain.SpaceID(strings.TrimSpace(string(member.SpaceID)))
	if call.SpaceID != "" {
		spaceID = spacedomain.SpaceID(strings.TrimSpace(call.SpaceID))
	}
	if spaceID == "" {
		return memberRef{}, fmt.Errorf("message: caller space is required")
	}
	if strings.TrimSpace(string(member.SpaceID)) != "" && spacedomain.SpaceID(member.SpaceID) != spaceID {
		return memberRef{}, fmt.Errorf("message: caller member %q belongs to space %q, not %q", member.ID, member.SpaceID, spaceID)
	}
	return memberRef{ID: member.ID, SpaceID: spaceID, Label: memberLabel(member)}, nil
}

func (h Handler) destination(ctx context.Context, call CallContext, actor memberRef, memberID member.ID) (memberRef, error) {
	member, err := h.loadActiveMember(ctx, call, memberID, "destination")
	if err != nil {
		return memberRef{}, err
	}
	if spacedomain.SpaceID(member.SpaceID) != actor.SpaceID {
		return memberRef{}, fmt.Errorf("message: destination member %q belongs to space %q, not %q", member.ID, member.SpaceID, actor.SpaceID)
	}
	return memberRef{ID: member.ID, SpaceID: spacedomain.SpaceID(member.SpaceID), Label: memberLabel(member)}, nil
}

func (h Handler) loadActiveMember(ctx context.Context, call CallContext, memberID member.ID, label string) (member.Record, error) {
	rosterMember, err := call.Members.GetMember(ctx, memberID)
	if err != nil {
		return member.Record{}, fmt.Errorf("message: load %s member: %w", label, err)
	}
	if strings.TrimSpace(rosterMember.LifecycleState) != "" && !strings.EqualFold(rosterMember.LifecycleState, member.LifecycleActive) {
		return member.Record{}, fmt.Errorf("message: %s member %q is not active", label, memberID)
	}
	if strings.TrimSpace(string(rosterMember.ID)) == "" {
		return member.Record{}, fmt.Errorf("message: %s member id is empty", label)
	}
	if strings.TrimSpace(string(rosterMember.SpaceID)) == "" {
		return member.Record{}, fmt.Errorf("message: %s member %q has no space", label, rosterMember.ID)
	}
	return rosterMember, nil
}

func decode(args json.RawMessage) (requestInput, error) {
	if err := rejectNullFields(args); err != nil {
		return requestInput{}, err
	}
	var raw rawRequest
	dec := json.NewDecoder(strings.NewReader(string(args)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&raw); err != nil {
		return requestInput{}, fmt.Errorf("message: invalid arguments: %w", err)
	}
	action := strings.TrimSpace(strings.ToLower(raw.Action))
	if action == "" {
		return requestInput{}, fmt.Errorf("message: action is required")
	}
	if action != "send" && action != "inbox" {
		return requestInput{}, fmt.Errorf("message: unsupported action %q", action)
	}
	if action == "inbox" {
		status := types.MessageStatus(strings.TrimSpace(strings.ToLower(ptrString(raw.Status))))
		switch status {
		case "", types.MessageStatusQueuedTyped, types.MessageStatusConsumedTyped:
		default:
			return requestInput{}, fmt.Errorf("message: status must be queued or consumed")
		}
		limit := 10
		if raw.Limit != nil {
			limit = *raw.Limit
		}
		if limit < 0 {
			return requestInput{}, fmt.Errorf("message: limit must be non-negative")
		}
		return requestInput{Action: action, Status: status, Limit: limit}, nil
	}
	destinationMemberID := member.ID(strings.TrimSpace(ptrString(raw.DestinationMemberID)))
	if destinationMemberID == "" {
		return requestInput{}, fmt.Errorf("message: destination_member_id is required")
	}
	kind := types.AgentMessageKind(strings.TrimSpace(strings.ToLower(raw.Kind)))
	switch kind {
	case types.AgentMessageKindInform, types.AgentMessageKindQuery, types.AgentMessageKindAck, types.AgentMessageKindResponse:
	default:
		return requestInput{}, fmt.Errorf("message: kind must be inform, query, ack, or response")
	}
	subject := strings.TrimSpace(raw.Subject)
	if subject == "" {
		return requestInput{}, fmt.Errorf("message: subject must be a non-empty string")
	}
	body := strings.TrimSpace(raw.Body)
	if body == "" {
		return requestInput{}, fmt.Errorf("message: body must be a non-empty string")
	}
	correlationID := types.CorrelationID(strings.TrimSpace(ptrString(raw.CorrelationID)))
	if (kind == types.AgentMessageKindAck || kind == types.AgentMessageKindResponse) && correlationID == "" {
		return requestInput{}, fmt.Errorf("message: correlation_id is required for ack and response")
	}
	return requestInput{
		Action:              action,
		DestinationMemberID: destinationMemberID,
		Kind:                kind,
		Subject:             subject,
		Body:                body,
		CorrelationID:       correlationID,
	}, nil
}

func rejectNullFields(args json.RawMessage) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(args, &fields); err != nil {
		return fmt.Errorf("message: invalid arguments: %w", err)
	}
	for field, raw := range fields {
		if strings.TrimSpace(string(raw)) == "null" {
			return fmt.Errorf("message: field %q must be omitted instead of null", field)
		}
	}
	return nil
}

func ptrString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
