package kr

import (
	"fmt"
	"strings"
	"time"

	missiondomain "github.com/tinoosan/agen8-mcp-server/internal/services/mission/domain/mission"
)

type KeyResult struct {
	ID                    KeyResultID
	MissionID             missiondomain.MissionID
	Title                 string
	Description           string
	MeasurementType       MeasurementType
	Direction             MeasurementDirection
	Unit                  string
	Baseline              *float64
	TargetValue           float64
	CurrentValue          float64
	ProgressPercent       int
	Status                KeyResultStatus
	ProjectID             string
	OwnerProjectName      string
	OwnerAssignedAt       *time.Time
	LastUpdatedBy         string
	LastUpdateNote        string
	LastMilestoneNotified int
	Version               int64
	CreatedAt             time.Time
	UpdatedAt             time.Time
	CompletedAt           *time.Time
}

type UpdateInput struct {
	Title           *string
	Description     *string
	MeasurementType *MeasurementType
	Direction       *MeasurementDirection
	Unit            *string
	Baseline        *float64
	TargetValue     *float64
}

func (k KeyResult) Update(input UpdateInput, now time.Time) (KeyResult, error) {
	if k.Status == KeyResultStatusDropped {
		return KeyResult{}, fmt.Errorf("update key result: key result %s is dropped", k.ID)
	}
	next := k
	if input.Title != nil {
		title := strings.TrimSpace(*input.Title)
		if title == "" {
			return KeyResult{}, fmt.Errorf("update key result: title is required")
		}
		next.Title = title
	}
	if input.Description != nil {
		next.Description = strings.TrimSpace(*input.Description)
	}
	if input.MeasurementType != nil {
		next.MeasurementType = *input.MeasurementType
	}
	if input.Direction != nil {
		next.Direction = *input.Direction
	}
	if input.Unit != nil {
		next.Unit = strings.TrimSpace(*input.Unit)
	}
	if input.Baseline != nil {
		next.Baseline = input.Baseline
	}
	if input.TargetValue != nil {
		next.TargetValue = *input.TargetValue
	}
	if err := ValidateMeasurement(next.MeasurementType, next.Direction); err != nil {
		return KeyResult{}, err
	}
	next.Version++
	stampUpdated(&next, now)
	return next, nil
}

func (k KeyResult) Drop(note string, now time.Time) (KeyResult, error) {
	next := k
	if k.Status == KeyResultStatusDropped {
		return next, nil
	}
	next.Status = KeyResultStatusDropped
	next.LastUpdateNote = strings.TrimSpace(note)
	next.Version++
	stampUpdated(&next, now)
	return next, nil
}

func (k KeyResult) Reopen(note string, now time.Time) (KeyResult, error) {
	switch k.Status {
	case KeyResultStatusCompleted, KeyResultStatusDropped:
	case KeyResultStatusOpen, KeyResultStatusInProgress:
		return k, nil
	default:
		return KeyResult{}, fmt.Errorf("reopen key result: key result %s can reopen only from completed or dropped; current status is %s", k.ID, k.Status)
	}
	next := k
	next.Status = KeyResultStatusOpen
	next.CurrentValue = k.startingValue()
	next.ProgressPercent = 0
	next.CompletedAt = nil
	next.LastMilestoneNotified = 0
	next.LastUpdateNote = strings.TrimSpace(note)
	next.Version++
	stampUpdated(&next, now)
	return next, nil
}

func (k KeyResult) startingValue() float64 {
	if k.Baseline != nil {
		return *k.Baseline
	}
	return 0
}

func stampUpdated(keyResult *KeyResult, now time.Time) {
	keyResult.UpdatedAt = utcOrNow(now)
}

func utcOrNow(now time.Time) time.Time {
	if now.IsZero() {
		return time.Now().UTC()
	}
	return now.UTC()
}
