package message

import (
	"context"

	spacedomain "github.com/tinoosan/agen8-mcp-server/internal/services/space/domain"

	messagedomain "github.com/tinoosan/agen8-mcp-server/internal/services/message/domain"
	"github.com/tinoosan/agen8-mcp-server/internal/services/space/domain/member"
	"github.com/tinoosan/agen8-mcp-server/pkg/types"
)

type MemberDirectory interface {
	GetMember(ctx context.Context, id member.ID) (member.Record, error)
}

type MessagePublisher interface {
	PublishAgentMessage(ctx context.Context, input messagedomain.NewMessageInput) (types.AgentMessage, error)
	ListMessages(ctx context.Context, filter messagedomain.MessageFilter) ([]types.AgentMessage, error)
}

type CallContext struct {
	Members       MemberDirectory
	Messages      MessagePublisher
	ProjectID     string
	SpaceID       string
	ActorMemberID string
}

type Result struct {
	Text       string
	Structured any
}

type rawRequest struct {
	Action              string  `json:"action"`
	DestinationMemberID *string `json:"destination_member_id"`
	Kind                string  `json:"kind"`
	Subject             string  `json:"subject"`
	Body                string  `json:"body"`
	CorrelationID       *string `json:"correlation_id"`
	Status              *string `json:"status"`
	Limit               *int    `json:"limit"`
}

type requestInput struct {
	Action              string
	DestinationMemberID member.ID
	Kind                types.AgentMessageKind
	Subject             string
	Body                string
	CorrelationID       types.CorrelationID
	Status              types.MessageStatus
	Limit               int
}

type memberRef struct {
	ID      member.ID           `json:"memberId"`
	SpaceID spacedomain.SpaceID `json:"spaceId"`
	Label   string              `json:"label"`
}
