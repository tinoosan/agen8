package types

import (
	"strings"
	"time"

	"github.com/google/uuid"
	spacedomain "github.com/tinoosan/agen8-mcp-server/internal/services/space/domain"
	"github.com/tinoosan/agen8-mcp-server/internal/services/space/domain/member"
)

type AgentMessageKind string

const (
	AgentMessageKindInform   AgentMessageKind = "inform"
	AgentMessageKindQuery    AgentMessageKind = "query"
	AgentMessageKindAck      AgentMessageKind = "ack"
	AgentMessageKindResponse AgentMessageKind = "response"
	AgentMessageKindTask     AgentMessageKind = "task"
	AgentMessageKindSystem   AgentMessageKind = "system"
)

type MessageStatus string

const (
	MessageStatusQueuedTyped   MessageStatus = "queued"
	MessageStatusConsumedTyped MessageStatus = "consumed"
)

type AgentMessage struct {
	ID                  AgentMessageID      `json:"id"`
	IntentID            IntentID            `json:"intentId"`
	CorrelationID       CorrelationID       `json:"correlationId,omitempty"`
	CausationID         CausationID         `json:"causationId,omitempty"`
	Producer            string              `json:"producer,omitempty"`
	SpaceID             spacedomain.SpaceID `json:"spaceId"`
	SourceMemberID      member.ID           `json:"sourceMemberId,omitempty"`
	DestinationMemberID member.ID           `json:"destinationMemberId"`
	ChannelID           ChannelID           `json:"channelId,omitempty"`
	Kind                AgentMessageKind    `json:"kind"`
	Subject             string              `json:"subject,omitempty"`
	Body                map[string]any      `json:"body,omitempty"`
	TaskRef             string              `json:"taskRef,omitempty"`
	Status              MessageStatus       `json:"status"`
	VisibleAt           time.Time           `json:"visibleAt"`
	Metadata            map[string]any      `json:"metadata,omitempty"`
	CreatedAt           time.Time           `json:"createdAt"`
	UpdatedAt           time.Time           `json:"updatedAt"`
	ConsumedAt          *time.Time          `json:"consumedAt,omitempty"`
	ConsumedBy          member.ID           `json:"consumedBy,omitempty"`
}

func NewAgentMessageID() AgentMessageID {
	return AgentMessageID("msg-" + uuid.NewString())
}

func (m AgentMessage) Normalized() AgentMessage {
	m.ID = AgentMessageID(strings.TrimSpace(string(m.ID)))
	m.IntentID = IntentID(strings.TrimSpace(string(m.IntentID)))
	m.CorrelationID = CorrelationID(strings.TrimSpace(string(m.CorrelationID)))
	m.CausationID = CausationID(strings.TrimSpace(string(m.CausationID)))
	m.Producer = strings.TrimSpace(m.Producer)
	m.SpaceID = spacedomain.SpaceID(strings.TrimSpace(string(m.SpaceID)))
	m.SourceMemberID = member.ID(strings.TrimSpace(string(m.SourceMemberID)))
	m.DestinationMemberID = member.ID(strings.TrimSpace(string(m.DestinationMemberID)))
	m.ChannelID = ChannelID(strings.TrimSpace(string(m.ChannelID)))
	m.Kind = AgentMessageKind(strings.TrimSpace(string(m.Kind)))
	m.Subject = strings.TrimSpace(m.Subject)
	m.TaskRef = strings.TrimSpace(m.TaskRef)
	m.Status = MessageStatus(strings.TrimSpace(string(m.Status)))
	m.ConsumedBy = member.ID(strings.TrimSpace(string(m.ConsumedBy)))
	m.VisibleAt = m.VisibleAt.UTC()
	m.CreatedAt = m.CreatedAt.UTC()
	m.UpdatedAt = m.UpdatedAt.UTC()
	if m.ConsumedAt != nil {
		consumed := m.ConsumedAt.UTC()
		m.ConsumedAt = &consumed
	}
	if m.Body == nil {
		m.Body = map[string]any{}
	}
	if m.Metadata == nil {
		m.Metadata = map[string]any{}
	}
	return m
}
