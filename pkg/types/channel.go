package types

import (
	"strings"
	"time"

	spacedomain "github.com/tinoosan/agen8-mcp-server/internal/services/space/domain"
)

const (
	ChannelStatusOpen   = "open"
	ChannelStatusClosed = "closed"
)

type ChannelStatus string

// Channel is a conversation lane scoped to a space and member address.
type Channel struct {
	ID          ChannelID           `json:"id"`
	SpaceID     spacedomain.SpaceID `json:"spaceId"`
	ProjectID   ProjectID           `json:"projectId,omitempty"`
	RunID       RunID               `json:"runId,omitempty"`
	MemberID    string              `json:"memberId,omitempty"`
	MemberLabel string              `json:"memberLabel,omitempty"`
	Title       string              `json:"title,omitempty"`
	Status      string              `json:"status,omitempty"`
	CreatedAt   time.Time           `json:"createdAt"`
	UpdatedAt   time.Time           `json:"updatedAt,omitempty"`

	// LastMessageAt is the timestamp of the most recent message
	// published into this member address. Updated by the message
	// publish path; combined with the per-user channel_reads.last_seen_at
	// row to compute Unread. Nullable for member addresses that have
	// never received a message.
	LastMessageAt *time.Time `json:"lastMessageAt,omitempty"`

	// Unread is computed per-user at list time, not persisted on the
	// channel record. True iff the user has either never read the
	// channel (no channel_reads row) or has activity newer than their
	// last seen marker.
	Unread bool `json:"unread,omitempty"`
}

func (c Channel) Normalized() Channel {
	c.ID = ChannelID(strings.TrimSpace(string(c.ID)))
	c.SpaceID = spacedomain.SpaceID(strings.TrimSpace(string(c.SpaceID)))
	c.ProjectID = ProjectID(strings.TrimSpace(string(c.ProjectID)))
	c.RunID = RunID(strings.TrimSpace(string(c.RunID)))
	c.MemberID = strings.TrimSpace(c.MemberID)
	c.MemberLabel = strings.TrimSpace(c.MemberLabel)
	c.Title = strings.TrimSpace(c.Title)
	c.Status = strings.ToLower(strings.TrimSpace(c.Status))
	return c
}
