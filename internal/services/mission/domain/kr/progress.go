package kr

import (
	"fmt"
	"math"
	"strings"
	"time"
)

func ValidateMeasurement(measurementType MeasurementType, direction MeasurementDirection) error {
	switch measurementType {
	case MeasurementNumber:
		if direction != DirectionIncrease && direction != DirectionDecrease {
			return fmt.Errorf("measurement number requires increase or decrease direction")
		}
	case MeasurementPercentage:
		if direction != "" && direction != DirectionIncrease {
			return fmt.Errorf("measurement percentage only supports increase direction")
		}
	case MeasurementBoolean:
		if direction != "" {
			return fmt.Errorf("measurement boolean forbids direction")
		}
	default:
		return fmt.Errorf("unknown measurement type %q", measurementType)
	}
	return nil
}

func ClampProgress(progress int) int {
	switch {
	case progress < 0:
		return 0
	case progress > 100:
		return 100
	default:
		return progress
	}
}

func (k KeyResult) UpdateProgress(value float64, note string, expectedVersion int64, entryID string, updatedBy string, now time.Time) (KeyResult, ProgressEntry, error) {
	entryID = strings.TrimSpace(entryID)
	if entryID == "" {
		return KeyResult{}, ProgressEntry{}, fmt.Errorf("update progress: progress entry id is required")
	}
	if expectedVersion != 0 && expectedVersion != k.Version {
		return KeyResult{}, ProgressEntry{}, fmt.Errorf("update progress: version mismatch: expected %d got %d", expectedVersion, k.Version)
	}
	switch k.Status {
	case KeyResultStatusDraft, KeyResultStatusOpen, KeyResultStatusInProgress:
	default:
		return KeyResult{}, ProgressEntry{}, fmt.Errorf("update progress: key result %s cannot update from status %s", k.ID, k.Status)
	}
	progress, err := k.progressForValue(value)
	if err != nil {
		return KeyResult{}, ProgressEntry{}, err
	}
	timestamp := utcOrNow(now)
	next := k
	next.CurrentValue = value
	next.ProgressPercent = progress
	next.LastUpdatedBy = strings.TrimSpace(updatedBy)
	next.LastUpdateNote = strings.TrimSpace(note)
	next.Version++
	next.UpdatedAt = timestamp
	if progress >= 100 {
		next.Status = KeyResultStatusCompleted
		completedAt := timestamp
		next.CompletedAt = &completedAt
	} else {
		next.Status = KeyResultStatusInProgress
		next.CompletedAt = nil
	}
	entry := ProgressEntry{
		ID:              entryID,
		KeyResultID:     k.ID,
		PreviousValue:   k.CurrentValue,
		NewValue:        value,
		ProgressPercent: progress,
		UpdatedBy:       next.LastUpdatedBy,
		Note:            next.LastUpdateNote,
		CreatedAt:       timestamp,
	}
	return next, entry, nil
}

func (k KeyResult) progressForValue(value float64) (int, error) {
	switch k.MeasurementType {
	case MeasurementNumber:
		return k.numberProgress(value)
	case MeasurementPercentage:
		return ClampProgress(int(math.Round(value))), nil
	case MeasurementBoolean:
		if value >= 1 {
			return 100, nil
		}
		return 0, nil
	default:
		return 0, fmt.Errorf("update progress: unknown measurement type %q", k.MeasurementType)
	}
}

func (k KeyResult) numberProgress(value float64) (int, error) {
	baseline := 0.0
	if k.Baseline != nil {
		baseline = *k.Baseline
	}
	var raw float64
	switch k.Direction {
	case DirectionIncrease:
		denominator := k.TargetValue - baseline
		if denominator == 0 {
			return 0, fmt.Errorf("update progress: target must differ from baseline")
		}
		raw = (value - baseline) / denominator
	case DirectionDecrease:
		denominator := baseline - k.TargetValue
		if denominator == 0 {
			return 0, fmt.Errorf("update progress: target must differ from baseline")
		}
		raw = (baseline - value) / denominator
	default:
		return 0, fmt.Errorf("update progress: number measurement requires increase or decrease direction")
	}
	return ClampProgress(int(math.Round(raw * 100))), nil
}
