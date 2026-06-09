package kr

import (
	"fmt"
	"strings"
	"time"

	missiondomain "github.com/tinoosan/agen8/internal/services/mission/domain/mission"
)

type NewKeyResultInput struct {
	ID              KeyResultID
	MissionID       missiondomain.MissionID
	Title           string
	Description     string
	MeasurementType MeasurementType
	Direction       MeasurementDirection
	Unit            string
	Baseline        *float64
	TargetValue     float64
	Now             time.Time
}

func NewKeyResult(input NewKeyResultInput) (KeyResult, error) {
	id := KeyResultID(strings.TrimSpace(string(input.ID)))
	if id == "" {
		return KeyResult{}, fmt.Errorf("new key result: id is required")
	}
	missionID := missiondomain.MissionID(strings.TrimSpace(string(input.MissionID)))
	if missionID == "" {
		return KeyResult{}, fmt.Errorf("new key result: mission id is required")
	}
	title := strings.TrimSpace(input.Title)
	if title == "" {
		return KeyResult{}, fmt.Errorf("new key result: title is required")
	}
	if err := ValidateMeasurement(input.MeasurementType, input.Direction); err != nil {
		return KeyResult{}, err
	}
	now := input.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	return KeyResult{
		ID:              id,
		MissionID:       missionID,
		Title:           title,
		Description:     strings.TrimSpace(input.Description),
		MeasurementType: input.MeasurementType,
		Direction:       input.Direction,
		Unit:            strings.TrimSpace(input.Unit),
		Baseline:        input.Baseline,
		TargetValue:     input.TargetValue,
		Status:          KeyResultStatusDraft,
		Version:         1,
		CreatedAt:       now,
		UpdatedAt:       now,
	}, nil
}
