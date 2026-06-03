package conversation

import (
	"fmt"
	"strings"
	"time"
)

type Attachment struct {
	ID        string
	ProjectID string
	SpaceID   string
	ChannelID string
	Name      string
	MediaType string
	SizeBytes int64
	URI       string
	CreatedAt time.Time
}

func ValidateAttachment(attachment Attachment) error {
	if strings.TrimSpace(attachment.ID) == "" {
		return fmt.Errorf("attachment id is required")
	}
	if strings.TrimSpace(attachment.ProjectID) == "" {
		return fmt.Errorf("attachment projectID is required")
	}
	if strings.TrimSpace(attachment.SpaceID) == "" {
		return fmt.Errorf("attachment spaceID is required")
	}
	if strings.TrimSpace(attachment.ChannelID) == "" {
		return fmt.Errorf("attachment channelID is required")
	}
	if strings.TrimSpace(attachment.Name) == "" {
		return fmt.Errorf("attachment name is required")
	}
	if strings.TrimSpace(attachment.MediaType) == "" {
		return fmt.Errorf("attachment mediaType is required")
	}
	if attachment.SizeBytes <= 0 {
		return fmt.Errorf("attachment sizeBytes must be greater than zero")
	}
	if strings.TrimSpace(attachment.URI) == "" {
		return fmt.Errorf("attachment uri is required")
	}
	if attachment.CreatedAt.IsZero() {
		return fmt.Errorf("attachment createdAt is required")
	}
	return nil
}
