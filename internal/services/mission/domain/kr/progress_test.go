package kr

import (
	"testing"
	"time"
)

func TestValidateMeasurement(t *testing.T) {
	tests := []struct {
		name            string
		measurementType MeasurementType
		direction       MeasurementDirection
		wantErr         bool
	}{
		{name: "number increase", measurementType: MeasurementNumber, direction: DirectionIncrease},
		{name: "number missing direction", measurementType: MeasurementNumber, wantErr: true},
		{name: "percentage increase", measurementType: MeasurementPercentage, direction: DirectionIncrease},
		{name: "percentage decrease rejected", measurementType: MeasurementPercentage, direction: DirectionDecrease, wantErr: true},
		{name: "boolean no direction", measurementType: MeasurementBoolean},
		{name: "boolean direction rejected", measurementType: MeasurementBoolean, direction: DirectionIncrease, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateMeasurement(tt.measurementType, tt.direction)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidateMeasurement error=%v wantErr=%v", err, tt.wantErr)
			}
		})
	}
}

func TestKeyResultUpdateProgressNumberIncrease(t *testing.T) {
	now := time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)
	keyResult := newKeyResultForTest(t, now)

	got, entry, err := keyResult.UpdateProgress(50, "halfway", keyResult.Version, "progress-1", "member-1", now.Add(time.Hour))
	if err != nil {
		t.Fatalf("UpdateProgress: %v", err)
	}
	if got.CurrentValue != 50 || got.ProgressPercent != 50 {
		t.Fatalf("value=%v progress=%d", got.CurrentValue, got.ProgressPercent)
	}
	if got.Status != KeyResultStatusInProgress {
		t.Fatalf("Status=%q want %q", got.Status, KeyResultStatusInProgress)
	}
	if got.Version != keyResult.Version+1 {
		t.Fatalf("Version=%d want %d", got.Version, keyResult.Version+1)
	}
	if entry.PreviousValue != 0 || entry.NewValue != 50 || entry.ProgressPercent != 50 {
		t.Fatalf("entry=%+v", entry)
	}
}

func TestKeyResultUpdateProgressNumberDecrease(t *testing.T) {
	now := time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)
	baseline := 100.0
	keyResult, err := NewKeyResult(NewKeyResultInput{
		ID:              KeyResultID("kr-1"),
		MissionID:       "mission-1",
		Title:           "Reduce bugs",
		MeasurementType: MeasurementNumber,
		Direction:       DirectionDecrease,
		Baseline:        &baseline,
		TargetValue:     20,
		Now:             now,
	})
	if err != nil {
		t.Fatalf("NewKeyResult: %v", err)
	}

	got, _, err := keyResult.UpdateProgress(60, "halfway", 0, "progress-1", "member-1", now.Add(time.Hour))
	if err != nil {
		t.Fatalf("UpdateProgress: %v", err)
	}
	if got.ProgressPercent != 50 {
		t.Fatalf("ProgressPercent=%d want 50", got.ProgressPercent)
	}
}

func TestKeyResultUpdateProgressPercentageAndBoolean(t *testing.T) {
	now := time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)
	percentage, err := NewKeyResult(NewKeyResultInput{
		ID:              KeyResultID("kr-percentage"),
		MissionID:       "mission-1",
		Title:           "Percent",
		MeasurementType: MeasurementPercentage,
		Direction:       DirectionIncrease,
		TargetValue:     100,
		Now:             now,
	})
	if err != nil {
		t.Fatalf("NewKeyResult percentage: %v", err)
	}
	got, _, err := percentage.UpdateProgress(125, "done", 0, "progress-1", "member-1", now)
	if err != nil {
		t.Fatalf("UpdateProgress percentage: %v", err)
	}
	if got.ProgressPercent != 100 || got.Status != KeyResultStatusCompleted {
		t.Fatalf("percentage result=%+v", got)
	}

	boolean, err := NewKeyResult(NewKeyResultInput{
		ID:              KeyResultID("kr-boolean"),
		MissionID:       "mission-1",
		Title:           "Boolean",
		MeasurementType: MeasurementBoolean,
		TargetValue:     1,
		Now:             now,
	})
	if err != nil {
		t.Fatalf("NewKeyResult boolean: %v", err)
	}
	got, _, err = boolean.UpdateProgress(1, "true", 0, "progress-2", "member-1", now)
	if err != nil {
		t.Fatalf("UpdateProgress boolean: %v", err)
	}
	if got.ProgressPercent != 100 || got.Status != KeyResultStatusCompleted {
		t.Fatalf("boolean result=%+v", got)
	}
}

func TestKeyResultUpdateProgressRejectsVersionMismatchAndTerminalStatuses(t *testing.T) {
	now := time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)
	keyResult := newKeyResultForTest(t, now)
	if _, _, err := keyResult.UpdateProgress(50, "", keyResult.Version+1, "progress-1", "member-1", now); err == nil {
		t.Fatal("version mismatch error is nil")
	}

	keyResult.Status = KeyResultStatusCompleted
	if _, _, err := keyResult.UpdateProgress(50, "", 0, "progress-1", "member-1", now); err == nil {
		t.Fatal("completed update error is nil")
	}
	keyResult.Status = KeyResultStatusDropped
	if _, _, err := keyResult.UpdateProgress(50, "", 0, "progress-1", "member-1", now); err == nil {
		t.Fatal("dropped update error is nil")
	}
}
