package channel

import (
	"fmt"
	"strings"
	"time"

	spacedomain "github.com/tinoosan/agen8-mcp-server/internal/services/space/domain"

	"github.com/tinoosan/agen8-mcp-server/internal/services/space/domain/member"
	"github.com/tinoosan/agen8-mcp-server/pkg/types"
)

type NewMemberInput struct {
	SpaceID   spacedomain.SpaceID
	ProjectID types.ProjectID
	MemberID  member.ID
	Status    types.ChannelStatus
}

func NewMemberChannel(input NewMemberInput, now time.Time) (Channel, error) {
	spaceID := spacedomain.SpaceID(strings.TrimSpace(string(input.SpaceID)))
	if spaceID == "" {
		return Channel{}, fmt.Errorf("new member channel: space id is required")
	}
	memberID := member.ID(strings.TrimSpace(string(input.MemberID)))
	if memberID == "" {
		return Channel{}, fmt.Errorf("new member channel: member id is required")
	}
	status := types.ChannelStatus(strings.TrimSpace(string(input.Status)))
	if status == "" {
		status = types.ChannelStatus(types.ChannelStatusOpen)
	}
	if status != types.ChannelStatus(types.ChannelStatusOpen) && status != types.ChannelStatus(types.ChannelStatusClosed) {
		return Channel{}, fmt.Errorf("new member channel: unsupported status %q", status)
	}
	stamped := now.UTC()
	if stamped.IsZero() {
		stamped = time.Now().UTC()
	}
	ch := types.Channel{
		ID:        MemberChannelID(spaceID, memberID),
		SpaceID:   spaceID,
		ProjectID: types.ProjectID(strings.TrimSpace(string(input.ProjectID))),
		MemberID:  string(memberID),
		Status:    string(status),
		CreatedAt: stamped,
		UpdatedAt: stamped,
	}
	return Channel{inner: normalizeChannel(ch)}, nil
}

func MemberChannelID(spaceID spacedomain.SpaceID, memberID member.ID) types.ChannelID {
	return types.ChannelID("channel:" + strings.TrimSpace(string(spaceID)) + ":member:" + strings.TrimSpace(string(memberID)))
}
