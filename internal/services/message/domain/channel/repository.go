package channel

import (
	"context"
	"time"

	spacedomain "github.com/tinoosan/agen8-mcp-server/internal/services/space/domain"

	"github.com/tinoosan/agen8-mcp-server/internal/services/space/domain/member"
	"github.com/tinoosan/agen8-mcp-server/pkg/types"
)

type Reader interface {
	Load(ctx context.Context, channelID types.ChannelID) (types.Channel, error)
	LoadMemberChannel(ctx context.Context, spaceID spacedomain.SpaceID, memberID member.ID) (types.Channel, error)
	ListBySpace(ctx context.Context, spaceID spacedomain.SpaceID) ([]types.Channel, error)
	UnreadCountsByChannel(ctx context.Context, userID string, channelIDs []types.ChannelID) (map[types.ChannelID]int, error)
}

type Writer interface {
	Save(ctx context.Context, channel types.Channel) (types.Channel, error)
	RecordActivity(ctx context.Context, channelID types.ChannelID, at time.Time) error
	MarkRead(ctx context.Context, userID string, channelID types.ChannelID, at time.Time) error
	DeleteForMember(ctx context.Context, spaceID spacedomain.SpaceID, memberID member.ID) error
}

type Repository interface {
	Reader
	Writer
}
