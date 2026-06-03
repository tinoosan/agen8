package mission

import (
	"fmt"
	"strings"
	"time"
)

type Mission struct {
	ID          MissionID
	ProjectID   string
	Title       string
	Description string
	Status      MissionStatus
	StartDate   *time.Time
	EndDate     *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
	PausedAt    *time.Time
	CompletedAt *time.Time
}

func (m Mission) UpdateDetails(title string, description string, now time.Time) (Mission, error) {
	if m.Status == MissionStatusArchived {
		return Mission{}, fmt.Errorf("update mission: mission %s is archived", m.ID)
	}
	title = strings.TrimSpace(title)
	if title == "" {
		return Mission{}, fmt.Errorf("update mission: title is required")
	}
	next := m
	next.Title = title
	next.Description = strings.TrimSpace(description)
	stampUpdated(&next, now)
	return next, nil
}

func (m Mission) UpdateSchedule(startDate *time.Time, endDate *time.Time, now time.Time) (Mission, error) {
	if m.Status == MissionStatusArchived {
		return Mission{}, fmt.Errorf("update mission schedule: mission %s is archived", m.ID)
	}
	next := m
	next.StartDate = normalizeTimePtr(startDate)
	next.EndDate = normalizeTimePtr(endDate)
	stampUpdated(&next, now)
	return next, nil
}

func (m Mission) Activate(now time.Time) (Mission, error) {
	switch m.Status {
	case MissionStatusDraft, MissionStatusPaused, MissionStatusCompleted:
	default:
		return Mission{}, fmt.Errorf("activate mission: mission %s cannot activate from status %s", m.ID, m.Status)
	}
	next := m
	next.Status = MissionStatusActive
	next.PausedAt = nil
	next.CompletedAt = nil
	stampUpdated(&next, now)
	return next, nil
}

func (m Mission) Pause(now time.Time) (Mission, error) {
	if m.Status != MissionStatusActive {
		return Mission{}, fmt.Errorf("pause mission: mission %s cannot pause from status %s", m.ID, m.Status)
	}
	next := m
	next.Status = MissionStatusPaused
	paused := utcOrNow(now)
	next.PausedAt = &paused
	next.CompletedAt = nil
	next.UpdatedAt = paused
	return next, nil
}

func (m Mission) Complete(now time.Time) (Mission, error) {
	if m.Status != MissionStatusActive {
		return Mission{}, fmt.Errorf("complete mission: mission %s cannot complete from status %s", m.ID, m.Status)
	}
	next := m
	next.Status = MissionStatusCompleted
	next.PausedAt = nil
	completed := utcOrNow(now)
	next.CompletedAt = &completed
	next.UpdatedAt = completed
	return next, nil
}

func (m Mission) Archive(now time.Time) (Mission, error) {
	switch m.Status {
	case MissionStatusDraft, MissionStatusActive, MissionStatusPaused, MissionStatusCompleted:
	default:
		return Mission{}, fmt.Errorf("archive mission: mission %s cannot archive from status %s", m.ID, m.Status)
	}
	next := m
	next.Status = MissionStatusArchived
	stampUpdated(&next, now)
	return next, nil
}

func stampUpdated(mission *Mission, now time.Time) {
	mission.UpdatedAt = utcOrNow(now)
}

func utcOrNow(now time.Time) time.Time {
	if now.IsZero() {
		return time.Now().UTC()
	}
	return now.UTC()
}

func normalizeTimePtr(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	normalized := value.UTC()
	return &normalized
}
