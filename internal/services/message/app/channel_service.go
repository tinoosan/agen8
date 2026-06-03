package app

import (
	"context"
	"fmt"
	"strings"

	spacedomain "github.com/tinoosan/agen8-mcp-server/internal/services/space/domain"

	"github.com/tinoosan/agen8-mcp-server/internal/services/message/domain/channel"
	"github.com/tinoosan/agen8-mcp-server/internal/services/space/domain/member"
	"github.com/tinoosan/agen8-mcp-server/pkg/types"
)

type NewMemberChannelParams struct {
	SpaceID   spacedomain.SpaceID
	ProjectID types.ProjectID
	MemberID  member.ID
	Status    types.ChannelStatus
}

// EnsureMemberChannel creates or updates the deterministic channel for a space member.
func (s *Service) EnsureMemberChannel(ctx context.Context, params NewMemberChannelParams) (types.Channel, error) {
	ch, err := channel.NewMemberChannel(channel.NewMemberInput{
		SpaceID:   params.SpaceID,
		ProjectID: params.ProjectID,
		MemberID:  params.MemberID,
		Status:    params.Status,
	}, s.clock.Now())
	if err != nil {
		return types.Channel{}, err
	}
	return s.repo.Save(ctx, ch.Inner())
}

// LoadChannel loads a channel by its durable channel id.
func (s *Service) LoadChannel(ctx context.Context, channelID types.ChannelID) (types.Channel, error) {
	channelID = types.ChannelID(strings.TrimSpace(string(channelID)))
	if channelID == "" {
		return types.Channel{}, fmt.Errorf("channel id is required")
	}
	return s.repo.Load(ctx, channelID)
}

// LoadMemberChannel loads the deterministic channel for a space/member pair.
func (s *Service) LoadMemberChannel(ctx context.Context, spaceID spacedomain.SpaceID, memberID member.ID) (types.Channel, error) {
	spaceID = trimSpaceID(spaceID)
	memberID = trimMemberID(memberID)
	if spaceID == "" {
		return types.Channel{}, fmt.Errorf("space id is required")
	}
	if memberID == "" {
		return types.Channel{}, fmt.Errorf("member id is required")
	}
	return s.repo.LoadMemberChannel(ctx, spaceID, memberID)
}

// ListChannelsBySpace returns every channel address in a space.
func (s *Service) ListChannelsBySpace(ctx context.Context, spaceID spacedomain.SpaceID) ([]types.Channel, error) {
	spaceID = trimSpaceID(spaceID)
	if spaceID == "" {
		return nil, fmt.Errorf("space id is required")
	}
	return s.repo.ListBySpace(ctx, spaceID)
}

// MarkChannelRead records that a user has seen a channel at the service clock time.
func (s *Service) MarkChannelRead(ctx context.Context, userID string, channelID types.ChannelID) error {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return fmt.Errorf("user id is required")
	}
	channelID = types.ChannelID(strings.TrimSpace(string(channelID)))
	if channelID == "" {
		return fmt.Errorf("channel id is required")
	}
	return s.repo.MarkRead(ctx, userID, channelID, s.clock.Now())
}

// RecordChannelActivity bumps a channel's last-activity timestamp to the service clock time.
func (s *Service) RecordChannelActivity(ctx context.Context, channelID types.ChannelID) error {
	channelID = types.ChannelID(strings.TrimSpace(string(channelID)))
	if channelID == "" {
		return fmt.Errorf("channel id is required")
	}
	return s.repo.RecordActivity(ctx, channelID, s.clock.Now())
}

// CloseChannel marks a channel closed through the channel domain transition.
func (s *Service) CloseChannel(ctx context.Context, channelID types.ChannelID) (types.Channel, error) {
	loaded, err := s.LoadChannel(ctx, channelID)
	if err != nil {
		return types.Channel{}, err
	}
	closed, err := channel.WrapChannel(loaded).Close(s.clock.Now())
	if err != nil {
		return types.Channel{}, err
	}
	return s.repo.Save(ctx, closed.Inner())
}

// ReopenChannel marks a channel open through the channel domain transition.
func (s *Service) ReopenChannel(ctx context.Context, channelID types.ChannelID) (types.Channel, error) {
	loaded, err := s.LoadChannel(ctx, channelID)
	if err != nil {
		return types.Channel{}, err
	}
	reopened, err := channel.WrapChannel(loaded).Reopen(s.clock.Now())
	if err != nil {
		return types.Channel{}, err
	}
	return s.repo.Save(ctx, reopened.Inner())
}

// DeleteMemberChannel removes the deterministic channel for a retired space member.
func (s *Service) DeleteMemberChannel(ctx context.Context, spaceID spacedomain.SpaceID, memberID member.ID) error {
	spaceID = trimSpaceID(spaceID)
	memberID = trimMemberID(memberID)
	if spaceID == "" {
		return fmt.Errorf("space id is required")
	}
	if memberID == "" {
		return fmt.Errorf("member id is required")
	}
	return s.repo.DeleteForMember(ctx, spaceID, memberID)
}

// UnreadCountsByChannel computes per-user unread message counts for channel ids.
func (s *Service) UnreadCountsByChannel(ctx context.Context, userID string, channelIDs []types.ChannelID) (map[types.ChannelID]int, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, fmt.Errorf("user id is required")
	}
	return s.repo.UnreadCountsByChannel(ctx, userID, channelIDs)
}
