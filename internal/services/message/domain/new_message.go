package domain

import (
	"fmt"
	"strings"
	"time"

	spacedomain "github.com/tinoosan/agen8-mcp-server/internal/services/space/domain"

	"github.com/tinoosan/agen8-mcp-server/internal/services/space/domain/member"
	taskdomain "github.com/tinoosan/agen8-mcp-server/internal/services/task/domain"
	"github.com/tinoosan/agen8-mcp-server/pkg/types"
)

type NewMessageInput struct {
	ID       types.AgentMessageID
	Route    MessageRoute
	Content  MessageContent
	Producer MessageProducer
	Metadata map[string]any
}

type MessageRoute struct {
	SpaceID             spacedomain.SpaceID
	SourceMemberID      member.ID
	DestinationMemberID member.ID
	ChannelID           types.ChannelID
}

type MessageContent struct {
	Kind    types.AgentMessageKind
	Subject string
	Body    map[string]any
	TaskRef taskdomain.TaskID
}

type MessageProducer struct {
	IntentID      types.IntentID
	CorrelationID types.CorrelationID
	CausationID   types.CausationID
	Producer      string
}

func NewMessage(input NewMessageInput, now time.Time) (Message, error) {
	route, err := normalizeRoute(input.Route)
	if err != nil {
		return Message{}, fmt.Errorf("new message: %w", err)
	}
	content, err := normalizeContent(input.Content)
	if err != nil {
		return Message{}, fmt.Errorf("new message: %w", err)
	}
	producer, err := normalizeProducer(input.Producer, content.Kind)
	if err != nil {
		return Message{}, fmt.Errorf("new message: %w", err)
	}
	id := types.AgentMessageID(strings.TrimSpace(string(input.ID)))
	if id == "" {
		id = types.NewAgentMessageID()
	}
	stamped := now.UTC()
	if stamped.IsZero() {
		stamped = time.Now().UTC()
	}
	msg := types.AgentMessage{
		ID:                  id,
		IntentID:            producer.IntentID,
		CorrelationID:       producer.CorrelationID,
		CausationID:         producer.CausationID,
		Producer:            producer.Producer,
		SpaceID:             route.SpaceID,
		SourceMemberID:      route.SourceMemberID,
		DestinationMemberID: route.DestinationMemberID,
		ChannelID:           route.ChannelID,
		Kind:                content.Kind,
		Subject:             content.Subject,
		Body:                content.Body,
		TaskRef:             string(content.TaskRef),
		Status:              types.MessageStatusQueuedTyped,
		VisibleAt:           stamped,
		Metadata:            cloneMap(input.Metadata),
		CreatedAt:           stamped,
		UpdatedAt:           stamped,
	}
	return Message{inner: msg.Normalized()}, nil
}

func normalizeRoute(route MessageRoute) (MessageRoute, error) {
	route.SpaceID = spacedomain.SpaceID(strings.TrimSpace(string(route.SpaceID)))
	route.SourceMemberID = trimMemberID(route.SourceMemberID)
	route.DestinationMemberID = trimMemberID(route.DestinationMemberID)
	route.ChannelID = types.ChannelID(strings.TrimSpace(string(route.ChannelID)))
	switch {
	case route.SpaceID == "":
		return MessageRoute{}, fmt.Errorf("space id is required")
	case route.DestinationMemberID == "":
		return MessageRoute{}, fmt.Errorf("destination member id is required")
	}
	return route, nil
}

func normalizeContent(content MessageContent) (MessageContent, error) {
	content.Kind = types.AgentMessageKind(strings.TrimSpace(string(content.Kind)))
	content.Subject = strings.TrimSpace(content.Subject)
	content.TaskRef = taskdomain.TaskID(strings.TrimSpace(string(content.TaskRef)))
	if content.Body == nil {
		content.Body = map[string]any{}
	}
	switch content.Kind {
	case types.AgentMessageKindInform, types.AgentMessageKindQuery, types.AgentMessageKindAck,
		types.AgentMessageKindResponse, types.AgentMessageKindTask, types.AgentMessageKindSystem:
	default:
		return MessageContent{}, fmt.Errorf("unsupported kind %q", content.Kind)
	}
	if content.Kind == types.AgentMessageKindTask && content.TaskRef == "" {
		return MessageContent{}, fmt.Errorf("task ref is required for task messages")
	}
	if content.Kind != types.AgentMessageKindTask && content.Subject == "" {
		return MessageContent{}, fmt.Errorf("subject is required")
	}
	if len(content.Body) == 0 {
		return MessageContent{}, fmt.Errorf("body is required")
	}
	content.Body = cloneMap(content.Body)
	return content, nil
}

func normalizeProducer(producer MessageProducer, kind types.AgentMessageKind) (MessageProducer, error) {
	producer.IntentID = types.IntentID(strings.TrimSpace(string(producer.IntentID)))
	producer.CorrelationID = types.CorrelationID(strings.TrimSpace(string(producer.CorrelationID)))
	producer.CausationID = types.CausationID(strings.TrimSpace(string(producer.CausationID)))
	producer.Producer = strings.TrimSpace(producer.Producer)
	if producer.IntentID == "" {
		return MessageProducer{}, fmt.Errorf("intent id is required")
	}
	if producer.Producer == "" {
		return MessageProducer{}, fmt.Errorf("producer is required")
	}
	if (kind == types.AgentMessageKindAck || kind == types.AgentMessageKindResponse) && producer.CorrelationID == "" {
		return MessageProducer{}, fmt.Errorf("correlation id is required for %s messages", kind)
	}
	return producer, nil
}

func cloneMap(in map[string]any) map[string]any {
	if len(in) == 0 {
		return map[string]any{}
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		out[k] = v
	}
	return out
}
