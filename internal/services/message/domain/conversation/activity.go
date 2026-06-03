package conversation

import (
	"fmt"
	"strings"
	"time"
)

type Activity struct {
	ID          string
	ChannelID   string
	SpaceID     string
	MemberID    string
	SessionID   string
	TurnID      string
	ToolCallID  string
	Sequence    int
	Kind        string
	Title       string
	Status      string
	Text        string
	CreatedAt   time.Time
	CompletedAt *time.Time
	Data        map[string]string
}

func ValidateActivity(activity Activity) error {
	if strings.TrimSpace(activity.ID) == "" {
		return fmt.Errorf("id is required")
	}
	if strings.TrimSpace(activity.ChannelID) == "" {
		return fmt.Errorf("channelID is required")
	}
	if strings.TrimSpace(activity.SpaceID) == "" {
		return fmt.Errorf("spaceID is required")
	}
	if strings.TrimSpace(activity.MemberID) == "" {
		return fmt.Errorf("memberID is required")
	}
	if strings.TrimSpace(activity.SessionID) == "" {
		return fmt.Errorf("sessionID is required")
	}
	if strings.TrimSpace(activity.TurnID) == "" {
		return fmt.Errorf("turnID is required")
	}
	if strings.TrimSpace(activity.ToolCallID) == "" {
		return fmt.Errorf("toolCallID is required")
	}
	if activity.Sequence <= 0 {
		return fmt.Errorf("sequence must be greater than zero")
	}
	if strings.TrimSpace(activity.Kind) == "" {
		return fmt.Errorf("kind is required")
	}
	if strings.TrimSpace(activity.Title) == "" {
		return fmt.Errorf("title is required")
	}
	if strings.TrimSpace(activity.Status) == "" {
		return fmt.Errorf("status is required")
	}
	if activity.CreatedAt.IsZero() {
		return fmt.Errorf("createdAt is required")
	}
	return nil
}
