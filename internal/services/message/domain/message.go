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

type Message struct {
	inner types.AgentMessage
}

func WrapMessage(inner types.AgentMessage) Message {
	return Message{inner: inner.Normalized()}
}

func (m Message) Inner() types.AgentMessage { return m.inner }

func (m Message) ID() types.AgentMessageID           { return m.inner.ID }
func (m Message) Status() types.MessageStatus        { return m.inner.Status }
func (m Message) SpaceID() spacedomain.SpaceID       { return m.inner.SpaceID }
func (m Message) SourceMemberID() member.ID          { return m.inner.SourceMemberID }
func (m Message) DestinationMemberID() member.ID     { return m.inner.DestinationMemberID }
func (m Message) ChannelID() types.ChannelID         { return m.inner.ChannelID }
func (m Message) Kind() types.AgentMessageKind       { return m.inner.Kind }
func (m Message) CorrelationID() types.CorrelationID { return m.inner.CorrelationID }
func (m Message) TaskRef() taskdomain.TaskID         { return taskdomain.TaskID(m.inner.TaskRef) }
func (m Message) IntentID() types.IntentID           { return m.inner.IntentID }
func (m Message) CausationID() types.CausationID     { return m.inner.CausationID }
func (m Message) Producer() string                   { return strings.TrimSpace(m.inner.Producer) }
func (m Message) IsConsumed() bool                   { return m.inner.Status == types.MessageStatusConsumedTyped }
func (m Message) IsQueued() bool                     { return m.inner.Status == types.MessageStatusQueuedTyped }

func (m Message) Consume(consumerID member.ID, now time.Time) (Message, error) {
	consumerID = trimMemberID(consumerID)
	if consumerID == "" {
		return Message{}, fmt.Errorf("consume: consumer member id is required")
	}
	if m.inner.Status != types.MessageStatusQueuedTyped {
		return Message{}, fmt.Errorf("consume: message %s cannot be consumed from status %s: must be %s",
			m.inner.ID, m.inner.Status, types.MessageStatusQueuedTyped)
	}
	stamped := now.UTC()
	next := m.inner
	next.Status = types.MessageStatusConsumedTyped
	next.ConsumedBy = consumerID
	next.ConsumedAt = &stamped
	next.UpdatedAt = stamped
	return Message{inner: next.Normalized()}, nil
}

func trimMemberID(id member.ID) member.ID {
	return member.ID(strings.TrimSpace(string(id)))
}
