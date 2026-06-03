package channel

import (
	"fmt"
	"strings"
	"time"

	spacedomain "github.com/tinoosan/agen8-mcp-server/internal/services/space/domain"

	"github.com/tinoosan/agen8-mcp-server/internal/services/space/domain/member"
	"github.com/tinoosan/agen8-mcp-server/pkg/types"
)

type Channel struct {
	inner types.Channel
}

func WrapChannel(inner types.Channel) Channel {
	return Channel{inner: normalizeChannel(inner)}
}

func (c Channel) Inner() types.Channel { return c.inner }

func (c Channel) ID() types.ChannelID          { return c.inner.ID }
func (c Channel) SpaceID() spacedomain.SpaceID { return c.inner.SpaceID }
func (c Channel) ProjectID() types.ProjectID   { return c.inner.ProjectID }
func (c Channel) MemberID() member.ID {
	return member.ID(strings.TrimSpace(c.inner.MemberID))
}
func (c Channel) Status() types.ChannelStatus {
	return types.ChannelStatus(strings.TrimSpace(c.inner.Status))
}
func (c Channel) LastMessageAt() *time.Time { return cloneTime(c.inner.LastMessageAt) }
func (c Channel) IsOpen() bool              { return c.inner.Status == types.ChannelStatusOpen }

func (c Channel) MarkActivity(at time.Time) (Channel, error) {
	if at.IsZero() {
		return Channel{}, fmt.Errorf("mark activity: timestamp is required")
	}
	stamped := at.UTC()
	next := c.inner
	if next.LastMessageAt == nil || stamped.After(next.LastMessageAt.UTC()) {
		next.LastMessageAt = &stamped
	}
	next.UpdatedAt = stamped
	return Channel{inner: normalizeChannel(next)}, nil
}

func (c Channel) Close(now time.Time) (Channel, error) {
	if c.inner.Status == types.ChannelStatusClosed {
		return c, nil
	}
	stamped := now.UTC()
	next := c.inner
	next.Status = types.ChannelStatusClosed
	next.UpdatedAt = stamped
	return Channel{inner: normalizeChannel(next)}, nil
}

func (c Channel) Reopen(now time.Time) (Channel, error) {
	if c.inner.Status == types.ChannelStatusOpen {
		return c, nil
	}
	stamped := now.UTC()
	next := c.inner
	next.Status = types.ChannelStatusOpen
	next.UpdatedAt = stamped
	return Channel{inner: normalizeChannel(next)}, nil
}

func normalizeChannel(ch types.Channel) types.Channel {
	ch = ch.Normalized()
	ch.RunID = ""
	ch.MemberLabel = ""
	ch.Title = ""
	ch.Unread = false
	if ch.Status == "" {
		ch.Status = types.ChannelStatusOpen
	}
	if ch.LastMessageAt != nil {
		at := ch.LastMessageAt.UTC()
		ch.LastMessageAt = &at
	}
	ch.CreatedAt = ch.CreatedAt.UTC()
	ch.UpdatedAt = ch.UpdatedAt.UTC()
	return ch
}

func cloneTime(in *time.Time) *time.Time {
	if in == nil {
		return nil
	}
	out := in.UTC()
	return &out
}
