package types

import (
	"time"

	spacedomain "github.com/tinoosan/agen8-mcp-server/internal/services/space/domain"
)

// Message is the first-class domain entity for communication.
//
// Messages are communication. Tasks are work.
//
// A user talks to the coordinator via messages.
// A coordinator communicates with other coordinators via messages.
// A coordinator assigns work to roles via tasks.
//
// Messages carry explicit sender identity so the coordinator always knows
// who is speaking: a user, another coordinator, or the system.
//
// A Message can optionally reference a Task (humans discussing a work item),
// but a Task never wraps a Message. They are independent peers.
type Message struct {
	MessageID MessageID `json:"messageId"`

	// Sender identity — who sent this message.
	SenderType  string `json:"senderType"` // "user", "coordinator", "system"
	SenderName  string `json:"senderName"` // e.g. "User", "head-analyst", "system"
	SenderSpace string `json:"senderSpace,omitempty"`

	// Routing — where this message goes.
	SourceSpaceID       spacedomain.SpaceID `json:"sourceSpaceId,omitempty"`
	DestinationSpaceID  spacedomain.SpaceID `json:"destinationSpaceId,omitempty"`
	SourceMemberID      string              `json:"sourceMemberId,omitempty"`
	DestinationMemberID string              `json:"destinationMemberId,omitempty"`
	AssignedToType      string              `json:"assignedToType"` // "coordinator", "role", "member"
	AssignedTo          string              `json:"assignedTo"`     // role name or run ID
	AssignedRole        string              `json:"assignedRole,omitempty"`

	// Content.
	Kind    string `json:"kind"` // user_input, inform, query, ack, response, timeout
	Subject string `json:"subject,omitempty"`
	Body    string `json:"body"`

	// Optional task reference — discussing a task, not wrapping one.
	TaskRef string `json:"taskRef,omitempty"`

	// Spaceing.
	SpaceID       spacedomain.SpaceID `json:"spaceId"`
	CorrelationID string              `json:"correlationId,omitempty"`

	// Delivery state (no lifecycle — just delivery tracking).
	Status string `json:"status"` // pending, delivered, read, failed

	CreatedAt   *time.Time `json:"createdAt,omitempty"`
	ReadAt      *time.Time `json:"readAt,omitempty"`
	ProcessedAt *time.Time `json:"processedAt,omitempty"`

	Metadata map[string]any `json:"metadata,omitempty"`
}

// Sender type constants.
const (
	SenderTypeUser        = "user"
	SenderTypeCoordinator = "coordinator"
	SenderTypeSystem      = "system"
)

// MessageFromMemberMessage projects a MemberMessage bus envelope into the
// domain-facing Message type.
func MessageFromMemberMessage(msg MemberMessage) Message {
	// Extract body text: prefer msg.Message if set, fall back to Body map.
	body := ""
	subject := ""
	senderType := ""
	senderName := ""
	senderSpace := ""

	if msg.Message != nil {
		body = msg.Message.Body
		subject = msg.Message.Subject
		senderType = msg.Message.SenderType
		senderName = msg.Message.SenderName
		senderSpace = msg.Message.SenderSpace
	} else if msg.Body != nil {
		if v, ok := msg.Body["body"].(string); ok {
			body = v
		} else if v, ok := msg.Body["text"].(string); ok {
			body = v
		} else if v, ok := msg.Body["goal"].(string); ok {
			body = v
		}
		if v, ok := msg.Body["subject"].(string); ok {
			subject = v
		}
	}

	return Message{
		MessageID:           msg.MessageID,
		SenderType:          senderType,
		SenderName:          senderName,
		SenderSpace:         senderSpace,
		SourceSpaceID:       msg.SpaceID,
		DestinationSpaceID:  msg.SpaceID,
		SourceMemberID:      msg.ActorMemberID,
		DestinationMemberID: msg.TargetMemberID,
		AssignedToType:      msg.AssignedToType,
		AssignedTo:          msg.AssignedTo,
		AssignedRole:        msg.AssignedRole,
		Kind:                msg.Kind,
		Subject:             subject,
		Body:                body,
		TaskRef:             msg.TaskRef,
		SpaceID:             msg.SpaceID,
		CorrelationID:       msg.CorrelationID,
		Status:              msg.Status,
		CreatedAt:           msg.CreatedAt,
		ProcessedAt:         msg.ProcessedAt,
		Metadata:            cloneMessageMap(msg.Metadata),
	}
}

func cloneMessageMap(src map[string]any) map[string]any {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]any, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}
