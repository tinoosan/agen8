package mission

import (
	"fmt"
	"strings"
	"time"
)

type NewMissionInput struct {
	ID          MissionID
	ProjectID   string
	Title       string
	Description string
	StartDate   *time.Time
	EndDate     *time.Time
	Now         time.Time
}

func NewMission(input NewMissionInput) (Mission, error) {
	id := MissionID(strings.TrimSpace(string(input.ID)))
	if id == "" {
		return Mission{}, fmt.Errorf("new mission: id is required")
	}
	projectID := strings.TrimSpace(input.ProjectID)
	if projectID == "" {
		return Mission{}, fmt.Errorf("new mission: project id is required")
	}
	title := strings.TrimSpace(input.Title)
	if title == "" {
		return Mission{}, fmt.Errorf("new mission: title is required")
	}
	now := input.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	return Mission{
		ID:          id,
		ProjectID:   projectID,
		Title:       title,
		Description: strings.TrimSpace(input.Description),
		Status:      MissionStatusDraft,
		StartDate:   input.StartDate,
		EndDate:     input.EndDate,
		CreatedAt:   now,
		UpdatedAt:   now,
	}, nil
}
