package channel

import (
	"testing"
	"time"

	"github.com/tinoosan/agen8-mcp-server/pkg/types"
)

func TestMemberChannelID(t *testing.T) {
	got := MemberChannelID(" space-1 ", " member-1 ")
	if got != "channel:space-1:member:member-1" {
		t.Fatalf("MemberChannelID=%q", got)
	}
}

func TestNewMemberChannel(t *testing.T) {
	now := time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC)
	ch, err := NewMemberChannel(NewMemberInput{
		SpaceID:   "space-1",
		ProjectID: "project-1",
		MemberID:  "member-1",
	}, now)
	if err != nil {
		t.Fatalf("NewMemberChannel: %v", err)
	}
	inner := ch.Inner()
	if inner.ID != "channel:space-1:member:member-1" || inner.RunID != "" {
		t.Fatalf("channel id/run = %q/%q", inner.ID, inner.RunID)
	}
	if inner.Status != types.ChannelStatusOpen || inner.MemberID != "member-1" || inner.MemberLabel != "" || inner.Title != "" {
		t.Fatalf("channel fields = %+v", inner)
	}
	if !inner.CreatedAt.Equal(now) || !inner.UpdatedAt.Equal(now) {
		t.Fatalf("channel timestamps = %+v", inner)
	}
}

func TestNewMemberChannelValidatesRequiredFields(t *testing.T) {
	base := NewMemberInput{SpaceID: "space-1", MemberID: "member-1"}
	for _, tt := range []struct {
		name string
		edit func(*NewMemberInput)
	}{
		{"space", func(in *NewMemberInput) { in.SpaceID = " " }},
		{"member", func(in *NewMemberInput) { in.MemberID = " " }},
		{"status", func(in *NewMemberInput) { in.Status = "paused" }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			input := base
			tt.edit(&input)
			if _, err := NewMemberChannel(input, time.Now()); err == nil {
				t.Fatalf("expected validation error")
			}
		})
	}
}

func TestChannelMarkActivityIsMonotonic(t *testing.T) {
	now := time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC)
	ch, err := NewMemberChannel(NewMemberInput{SpaceID: "space-1", MemberID: "member-1"}, now)
	if err != nil {
		t.Fatalf("NewMemberChannel: %v", err)
	}
	later := now.Add(time.Minute)
	ch, err = ch.MarkActivity(later)
	if err != nil {
		t.Fatalf("MarkActivity: %v", err)
	}
	ch, err = ch.MarkActivity(now)
	if err != nil {
		t.Fatalf("MarkActivity stale: %v", err)
	}
	if ch.Inner().LastMessageAt == nil || !ch.Inner().LastMessageAt.Equal(later) {
		t.Fatalf("last message at = %+v want %s", ch.Inner().LastMessageAt, later)
	}
}

func TestChannelCloseReopen(t *testing.T) {
	now := time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC)
	ch, err := NewMemberChannel(NewMemberInput{SpaceID: "space-1", MemberID: "member-1"}, now)
	if err != nil {
		t.Fatalf("NewMemberChannel: %v", err)
	}
	closed, err := ch.Close(now.Add(time.Minute))
	if err != nil {
		t.Fatalf("Close: %v", err)
	}
	if closed.Status() != types.ChannelStatus(types.ChannelStatusClosed) {
		t.Fatalf("status=%q", closed.Status())
	}
	reopened, err := closed.Reopen(now.Add(2 * time.Minute))
	if err != nil {
		t.Fatalf("Reopen: %v", err)
	}
	if reopened.Status() != types.ChannelStatus(types.ChannelStatusOpen) {
		t.Fatalf("status=%q", reopened.Status())
	}
}
