package domain

import (
	"context"
	"time"

	spacedomain "github.com/tinoosan/agen8-mcp-server/internal/services/space/domain"

	"github.com/tinoosan/agen8-mcp-server/internal/services/space/domain/member"
	taskdomain "github.com/tinoosan/agen8-mcp-server/internal/services/task/domain"
	"github.com/tinoosan/agen8-mcp-server/pkg/types"
)

type Reader interface {
	Get(ctx context.Context, id types.AgentMessageID) (types.AgentMessage, error)
	List(ctx context.Context, filter MessageFilter) ([]types.AgentMessage, error)
	Count(ctx context.Context, filter MessageFilter) (int, error)
}

type Writer interface {
	SaveQueued(ctx context.Context, msg types.AgentMessage) (types.AgentMessage, error)
	NextQueuedForMember(ctx context.Context, memberID member.ID, now time.Time) (types.AgentMessage, error)
	DeferQueued(ctx context.Context, messageID types.AgentMessageID, visibleAt time.Time, updatedAt time.Time) (types.AgentMessage, error)
	MarkConsumed(ctx context.Context, msg types.AgentMessage) (types.AgentMessage, error)
}

type Repository interface {
	Reader
	Writer
}

type MessageFilter struct {
	SpaceID             spacedomain.SpaceID
	SourceMemberID      member.ID
	DestinationMemberID member.ID
	ChannelID           types.ChannelID
	CorrelationID       types.CorrelationID
	TaskRef             taskdomain.TaskID
	Kinds               []types.AgentMessageKind
	Statuses            []types.MessageStatus
	Since               *time.Time
	Until               *time.Time
	Limit               int
	Offset              int
}

type MessageWake struct {
	MessageID           types.AgentMessageID
	SpaceID             spacedomain.SpaceID
	DestinationMemberID member.ID
	ChannelID           types.ChannelID
	Kind                types.AgentMessageKind
}

type MemberWakePublisher interface {
	NotifyMember(memberID member.ID, wake MessageWake)
}
