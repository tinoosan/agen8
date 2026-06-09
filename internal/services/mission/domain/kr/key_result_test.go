package kr

import (
	"testing"
	"time"

	missiondomain "github.com/tinoosan/agen8/internal/services/mission/domain/mission"
)

func TestKeyResultUpdateDetailsAndMeasurement(t *testing.T) {
	now := time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)
	updatedAt := now.Add(time.Hour)
	keyResult := newKeyResultForTest(t, now)
	baseline := 10.0

	got, err := keyResult.Update(UpdateInput{
		Title:           ptr("Updated KR"),
		Description:     ptr("Updated scope"),
		MeasurementType: measurementPtr(MeasurementNumber),
		Direction:       directionPtr(DirectionDecrease),
		Unit:            ptr("bugs"),
		Baseline:        &baseline,
		TargetValue:     floatPtr(1),
	}, updatedAt)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if got.Title != "Updated KR" {
		t.Fatalf("Title=%q", got.Title)
	}
	if got.Direction != DirectionDecrease {
		t.Fatalf("Direction=%q", got.Direction)
	}
	if got.Baseline == nil || *got.Baseline != baseline {
		t.Fatalf("Baseline=%v", got.Baseline)
	}
	if got.Version != keyResult.Version+1 {
		t.Fatalf("Version=%d want %d", got.Version, keyResult.Version+1)
	}
	if !got.UpdatedAt.Equal(updatedAt) {
		t.Fatalf("UpdatedAt=%v want %v", got.UpdatedAt, updatedAt)
	}
}

func TestKeyResultUpdateRejectsInvalidMeasurement(t *testing.T) {
	keyResult := newKeyResultForTest(t, time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC))

	_, err := keyResult.Update(UpdateInput{
		MeasurementType: measurementPtr(MeasurementBoolean),
		Direction:       directionPtr(DirectionIncrease),
	}, time.Now())
	if err == nil {
		t.Fatal("Update error is nil")
	}
}

func TestKeyResultDrop(t *testing.T) {
	now := time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)
	keyResult := newKeyResultForTest(t, now)

	got, err := keyResult.Drop("out of scope", now.Add(time.Hour))
	if err != nil {
		t.Fatalf("Drop: %v", err)
	}
	if got.Status != KeyResultStatusDropped {
		t.Fatalf("Status=%q want %q", got.Status, KeyResultStatusDropped)
	}
	if got.Version != keyResult.Version+1 {
		t.Fatalf("Version=%d want %d", got.Version, keyResult.Version+1)
	}
	if got.LastUpdateNote != "out of scope" {
		t.Fatalf("LastUpdateNote=%q want out of scope", got.LastUpdateNote)
	}
	again, err := got.Drop("again", now.Add(2*time.Hour))
	if err != nil {
		t.Fatalf("Drop dropped KR: %v", err)
	}
	if again.Version != got.Version {
		t.Fatalf("Version=%d want %d", again.Version, got.Version)
	}
}

func TestKeyResultReopenCompletedResetsProgress(t *testing.T) {
	now := time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)
	keyResult := newKeyResultForTest(t, now)
	keyResult.Status = KeyResultStatusCompleted
	keyResult.CurrentValue = 100
	keyResult.ProgressPercent = 100
	keyResult.LastMilestoneNotified = 100
	completedAt := now.Add(time.Hour)
	keyResult.CompletedAt = &completedAt

	got, err := keyResult.Reopen("needs another pass", now.Add(2*time.Hour))
	if err != nil {
		t.Fatalf("Reopen: %v", err)
	}
	if got.Status != KeyResultStatusOpen {
		t.Fatalf("Status=%q want %q", got.Status, KeyResultStatusOpen)
	}
	if got.CompletedAt != nil {
		t.Fatalf("CompletedAt=%v want nil", got.CompletedAt)
	}
	if got.CurrentValue != 0 || got.ProgressPercent != 0 || got.LastMilestoneNotified != 0 {
		t.Fatalf("progress fields = current %v percent %d milestone %d, want reset", got.CurrentValue, got.ProgressPercent, got.LastMilestoneNotified)
	}
	if got.LastUpdateNote != "needs another pass" {
		t.Fatalf("LastUpdateNote=%q want needs another pass", got.LastUpdateNote)
	}
	if got.Version != keyResult.Version+1 {
		t.Fatalf("Version=%d want %d", got.Version, keyResult.Version+1)
	}
	again, err := got.Reopen("again", now.Add(3*time.Hour))
	if err != nil {
		t.Fatalf("Reopen open KR: %v", err)
	}
	if again.Version != got.Version {
		t.Fatalf("Version=%d want %d", again.Version, got.Version)
	}
}

func TestKeyResultReopenDropped(t *testing.T) {
	now := time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)
	keyResult := newKeyResultForTest(t, now)
	keyResult.Status = KeyResultStatusDropped
	baseline := 10.0
	keyResult.Baseline = &baseline
	keyResult.CurrentValue = 80
	keyResult.ProgressPercent = 70

	got, err := keyResult.Reopen("back in scope", now.Add(time.Hour))
	if err != nil {
		t.Fatalf("Reopen dropped: %v", err)
	}
	if got.Status != KeyResultStatusOpen {
		t.Fatalf("Status=%q want %q", got.Status, KeyResultStatusOpen)
	}
	if got.CurrentValue != baseline || got.ProgressPercent != 0 {
		t.Fatalf("progress = current %v percent %d, want baseline reset", got.CurrentValue, got.ProgressPercent)
	}
}

func newKeyResultForTest(t *testing.T, now time.Time) KeyResult {
	t.Helper()
	keyResult, err := NewKeyResult(NewKeyResultInput{
		ID:              KeyResultID("kr-1"),
		MissionID:       missiondomain.MissionID("mission-1"),
		Title:           "KR",
		MeasurementType: MeasurementNumber,
		Direction:       DirectionIncrease,
		TargetValue:     100,
		Now:             now,
	})
	if err != nil {
		t.Fatalf("NewKeyResult: %v", err)
	}
	return keyResult
}

func ptr(value string) *string { return &value }

func floatPtr(value float64) *float64 { return &value }

func measurementPtr(value MeasurementType) *MeasurementType { return &value }

func directionPtr(value MeasurementDirection) *MeasurementDirection { return &value }
