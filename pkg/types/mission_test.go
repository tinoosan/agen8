package types

import (
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Mission.Validate
// ---------------------------------------------------------------------------

func TestMission_Validate(t *testing.T) {
	valid := Mission{
		ID:        "mis-1",
		ProjectID: "proj-1",
		Title:     "Launch MVP by Q2",
		Status:    MissionStatusDraft,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	t.Run("valid mission passes", func(t *testing.T) {
		m := valid
		if err := m.Validate(); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("missing title", func(t *testing.T) {
		m := valid
		m.Title = ""
		if err := m.Validate(); err == nil {
			t.Fatal("expected error for missing title")
		}
	})

	t.Run("whitespace-only title", func(t *testing.T) {
		m := valid
		m.Title = "   "
		if err := m.Validate(); err == nil {
			t.Fatal("expected error for whitespace-only title")
		}
	})

	t.Run("missing projectId", func(t *testing.T) {
		m := valid
		m.ProjectID = ""
		if err := m.Validate(); err == nil {
			t.Fatal("expected error for missing projectId")
		}
	})

	t.Run("whitespace-only projectId", func(t *testing.T) {
		m := valid
		m.ProjectID = "   "
		if err := m.Validate(); err == nil {
			t.Fatal("expected error for whitespace-only projectId")
		}
	})

	t.Run("invalid status", func(t *testing.T) {
		m := valid
		m.Status = "invalid"
		if err := m.Validate(); err == nil {
			t.Fatal("expected error for invalid status")
		}
	})

	t.Run("empty status", func(t *testing.T) {
		m := valid
		m.Status = ""
		if err := m.Validate(); err == nil {
			t.Fatal("expected error for empty status")
		}
	})

	t.Run("all valid statuses accepted", func(t *testing.T) {
		for _, s := range ValidMissionStatuses {
			m := valid
			m.Status = s
			if err := m.Validate(); err != nil {
				t.Errorf("status %q should be valid, got %v", s, err)
			}
		}
	})

	t.Run("valid with optional description", func(t *testing.T) {
		m := valid
		m.Description = "Strategic objective for Q2 launch"
		if err := m.Validate(); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("valid with optional timestamps", func(t *testing.T) {
		m := valid
		now := time.Now().UTC()
		m.PausedAt = &now
		m.CompletedAt = &now
		if err := m.Validate(); err != nil {
			t.Fatalf("expected no error with optional timestamps, got %v", err)
		}
	})

	t.Run("valid with startDate only", func(t *testing.T) {
		m := valid
		now := time.Now().UTC()
		m.StartDate = &now
		if err := m.Validate(); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("valid with endDate only", func(t *testing.T) {
		m := valid
		now := time.Now().UTC()
		m.EndDate = &now
		if err := m.Validate(); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("valid with startDate before endDate", func(t *testing.T) {
		m := valid
		start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		end := time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)
		m.StartDate = &start
		m.EndDate = &end
		if err := m.Validate(); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("endDate before startDate returns error", func(t *testing.T) {
		m := valid
		start := time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)
		end := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		m.StartDate = &start
		m.EndDate = &end
		if err := m.Validate(); err == nil {
			t.Fatal("expected error when endDate is before startDate")
		}
	})

	t.Run("startDate equals endDate is valid", func(t *testing.T) {
		m := valid
		same := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
		m.StartDate = &same
		m.EndDate = &same
		if err := m.Validate(); err != nil {
			t.Fatalf("expected no error when dates are equal, got %v", err)
		}
	})
}

// ---------------------------------------------------------------------------
// Mission.ValidateTransition
// ---------------------------------------------------------------------------

func TestMission_ValidateTransition(t *testing.T) {
	tests := []struct {
		name      string
		from      MissionStatus
		to        MissionStatus
		expectErr bool
	}{
		// Draft transitions
		{"draft to active", MissionStatusDraft, MissionStatusActive, false},
		{"draft to archived", MissionStatusDraft, MissionStatusArchived, false},
		{"draft to paused is invalid", MissionStatusDraft, MissionStatusPaused, true},
		{"draft to completed is invalid", MissionStatusDraft, MissionStatusCompleted, true},

		// Active transitions
		{"active to paused", MissionStatusActive, MissionStatusPaused, false},
		{"active to completed", MissionStatusActive, MissionStatusCompleted, false},
		{"active to archived", MissionStatusActive, MissionStatusArchived, false},
		{"active to draft is invalid", MissionStatusActive, MissionStatusDraft, true},

		// Paused transitions
		{"paused to active", MissionStatusPaused, MissionStatusActive, false},
		{"paused to archived", MissionStatusPaused, MissionStatusArchived, false},
		{"paused to completed is invalid", MissionStatusPaused, MissionStatusCompleted, true},
		{"paused to draft is invalid", MissionStatusPaused, MissionStatusDraft, true},

		// Completed transitions — completed → active is now allowed for scope expansion (D83)
		{"completed to archived", MissionStatusCompleted, MissionStatusArchived, false},
		{"completed to active (scope expansion)", MissionStatusCompleted, MissionStatusActive, false},
		{"completed to draft is invalid", MissionStatusCompleted, MissionStatusDraft, true},
		{"completed to paused is invalid", MissionStatusCompleted, MissionStatusPaused, true},

		// Archived transitions (terminal)
		{"archived to active is invalid", MissionStatusArchived, MissionStatusActive, true},
		{"archived to draft is invalid", MissionStatusArchived, MissionStatusDraft, true},
		{"archived to paused is invalid", MissionStatusArchived, MissionStatusPaused, true},
		{"archived to completed is invalid", MissionStatusArchived, MissionStatusCompleted, true},

		// Self transitions are always allowed
		{"draft to draft (no-op)", MissionStatusDraft, MissionStatusDraft, false},
		{"active to active (no-op)", MissionStatusActive, MissionStatusActive, false},
		{"paused to paused (no-op)", MissionStatusPaused, MissionStatusPaused, false},
		{"completed to completed (no-op)", MissionStatusCompleted, MissionStatusCompleted, false},
		{"archived to archived (no-op)", MissionStatusArchived, MissionStatusArchived, false},

		// Invalid target status
		{"active to bogus", MissionStatusActive, MissionStatus("bogus"), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := Mission{
				ID:        "mis-1",
				ProjectID: "proj-1",
				Title:     "Test Mission",
				Status:    tt.from,
				CreatedAt: time.Now().UTC(),
				UpdatedAt: time.Now().UTC(),
			}
			err := m.ValidateTransition(tt.to)
			if tt.expectErr && err == nil {
				t.Fatalf("expected error for transition %s -> %s", tt.from, tt.to)
			}
			if !tt.expectErr && err != nil {
				t.Fatalf("expected no error for transition %s -> %s, got %v", tt.from, tt.to, err)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// KeyResult.Validate
// ---------------------------------------------------------------------------

func TestKeyResult_Validate(t *testing.T) {
	valid := KeyResult{
		ID:              "kr-1",
		MissionID:       "mis-1",
		Title:           "Reach 100 beta users",
		MeasurementType: MeasurementCount,
		Direction:       DirectionIncrease,
		TargetValue:     100,
		CurrentValue:    47,
		Unit:            "users",
		ProgressPercent: 47,
		Status:          KeyResultStatusOpen,
		CreatedAt:       time.Now().UTC(),
		UpdatedAt:       time.Now().UTC(),
	}

	t.Run("valid key result passes", func(t *testing.T) {
		kr := valid
		if err := kr.Validate(); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("missing title", func(t *testing.T) {
		kr := valid
		kr.Title = ""
		if err := kr.Validate(); err == nil {
			t.Fatal("expected error for missing title")
		}
	})

	t.Run("whitespace-only title", func(t *testing.T) {
		kr := valid
		kr.Title = "   "
		if err := kr.Validate(); err == nil {
			t.Fatal("expected error for whitespace-only title")
		}
	})

	t.Run("missing missionId", func(t *testing.T) {
		kr := valid
		kr.MissionID = ""
		if err := kr.Validate(); err == nil {
			t.Fatal("expected error for missing missionId")
		}
	})

	t.Run("whitespace-only missionId", func(t *testing.T) {
		kr := valid
		kr.MissionID = "   "
		if err := kr.Validate(); err == nil {
			t.Fatal("expected error for whitespace-only missionId")
		}
	})

	t.Run("invalid status", func(t *testing.T) {
		kr := valid
		kr.Status = "bogus"
		if err := kr.Validate(); err == nil {
			t.Fatal("expected error for invalid status")
		}
	})

	t.Run("empty status", func(t *testing.T) {
		kr := valid
		kr.Status = ""
		if err := kr.Validate(); err == nil {
			t.Fatal("expected error for empty status")
		}
	})

	t.Run("all valid statuses accepted", func(t *testing.T) {
		statuses := []KeyResultStatus{
			KeyResultStatusOpen, KeyResultStatusOnTrack, KeyResultStatusAtRisk,
			KeyResultStatusCompleted, KeyResultStatusDropped,
		}
		for _, s := range statuses {
			kr := valid
			kr.Status = s
			if err := kr.Validate(); err != nil {
				t.Errorf("status %q should be valid, got %v", s, err)
			}
		}
	})

	t.Run("invalid measurement type", func(t *testing.T) {
		kr := valid
		kr.MeasurementType = "rainbow"
		if err := kr.Validate(); err == nil {
			t.Fatal("expected error for invalid measurement type")
		}
	})

	t.Run("empty measurement type", func(t *testing.T) {
		kr := valid
		kr.MeasurementType = ""
		if err := kr.Validate(); err == nil {
			t.Fatal("expected error for empty measurement type")
		}
	})

	t.Run("all valid measurement types accepted", func(t *testing.T) {
		for _, mt := range ValidMeasurementTypes {
			kr := valid
			kr.MeasurementType = mt
			if err := kr.Validate(); err != nil {
				t.Errorf("measurement type %q should be valid, got %v", mt, err)
			}
		}
	})

	t.Run("invalid direction", func(t *testing.T) {
		kr := valid
		kr.Direction = "sideways"
		if err := kr.Validate(); err == nil {
			t.Fatal("expected error for invalid direction")
		}
	})

	t.Run("empty direction", func(t *testing.T) {
		kr := valid
		kr.Direction = ""
		if err := kr.Validate(); err == nil {
			t.Fatal("expected error for empty direction")
		}
	})

	t.Run("all valid directions accepted", func(t *testing.T) {
		for _, d := range ValidDirections {
			kr := valid
			kr.Direction = d
			if d == DirectionDecrease {
				baseline := 100.0
				kr.Baseline = &baseline
			}
			if err := kr.Validate(); err != nil {
				t.Errorf("direction %q should be valid, got %v", d, err)
			}
		}
	})

	t.Run("decrease direction without baseline returns error", func(t *testing.T) {
		kr := valid
		kr.Direction = DirectionDecrease
		kr.Baseline = nil
		if err := kr.Validate(); err == nil {
			t.Fatal("expected error for decrease direction without baseline")
		}
	})

	t.Run("decrease direction with baseline passes", func(t *testing.T) {
		kr := valid
		kr.Direction = DirectionDecrease
		baseline := 200.0
		kr.Baseline = &baseline
		if err := kr.Validate(); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("progress below zero returns error", func(t *testing.T) {
		kr := valid
		kr.ProgressPercent = -1
		if err := kr.Validate(); err == nil {
			t.Fatal("expected error for negative progress")
		}
	})

	t.Run("progress above 100 returns error", func(t *testing.T) {
		kr := valid
		kr.ProgressPercent = 101
		if err := kr.Validate(); err == nil {
			t.Fatal("expected error for progress > 100")
		}
	})

	t.Run("progress at boundaries is valid", func(t *testing.T) {
		for _, p := range []int{0, 50, 100} {
			kr := valid
			kr.ProgressPercent = p
			if err := kr.Validate(); err != nil {
				t.Errorf("progress %d should be valid, got %v", p, err)
			}
		}
	})

	t.Run("valid with optional fields", func(t *testing.T) {
		kr := valid
		kr.Description = "Track beta user signups"
		kr.SpaceID = "space-1"
		kr.LastUpdatedBy = "operator"
		kr.LastUpdateNote = "Initial setup"
		now := time.Now().UTC()
		kr.CompletedAt = &now
		if err := kr.Validate(); err != nil {
			t.Fatalf("expected no error with optional fields, got %v", err)
		}
	})

	t.Run("valid with baseline", func(t *testing.T) {
		kr := valid
		baseline := 10.0
		kr.Baseline = &baseline
		if err := kr.Validate(); err != nil {
			t.Fatalf("expected no error with baseline, got %v", err)
		}
	})
}

// ---------------------------------------------------------------------------
// KeyResult.ComputeProgress
// ---------------------------------------------------------------------------

func TestKeyResult_ComputeProgress(t *testing.T) {
	// Helper to create a float64 pointer.
	fp := func(v float64) *float64 { return &v }

	// --- Binary ---
	t.Run("binary/done", func(t *testing.T) {
		kr := KeyResult{MeasurementType: MeasurementBinary, Direction: DirectionIncrease, CurrentValue: 1}
		got, err := kr.ComputeProgress()
		assertNoError(t, err)
		assertEqual(t, 100, got)
	})

	t.Run("binary/not done", func(t *testing.T) {
		kr := KeyResult{MeasurementType: MeasurementBinary, Direction: DirectionIncrease, CurrentValue: 0}
		got, err := kr.ComputeProgress()
		assertNoError(t, err)
		assertEqual(t, 0, got)
	})

	t.Run("binary/value greater than 1 is done", func(t *testing.T) {
		kr := KeyResult{MeasurementType: MeasurementBinary, Direction: DirectionIncrease, CurrentValue: 5}
		got, err := kr.ComputeProgress()
		assertNoError(t, err)
		assertEqual(t, 100, got)
	})

	// --- Increase direction ---
	t.Run("increase/count no baseline", func(t *testing.T) {
		// 47 out of 100 = 47%
		kr := KeyResult{MeasurementType: MeasurementCount, Direction: DirectionIncrease, TargetValue: 100, CurrentValue: 47}
		got, err := kr.ComputeProgress()
		assertNoError(t, err)
		assertEqual(t, 47, got)
	})

	t.Run("increase/count with baseline", func(t *testing.T) {
		// baseline=20, target=100, current=60 → (60-20)/(100-20) = 50%
		kr := KeyResult{MeasurementType: MeasurementCount, Direction: DirectionIncrease, Baseline: fp(20), TargetValue: 100, CurrentValue: 60}
		got, err := kr.ComputeProgress()
		assertNoError(t, err)
		assertEqual(t, 50, got)
	})

	t.Run("increase/percentage no baseline", func(t *testing.T) {
		// 75 out of 100 = 75%
		kr := KeyResult{MeasurementType: MeasurementPercentage, Direction: DirectionIncrease, TargetValue: 100, CurrentValue: 75}
		got, err := kr.ComputeProgress()
		assertNoError(t, err)
		assertEqual(t, 75, got)
	})

	t.Run("increase/currency with baseline", func(t *testing.T) {
		// baseline=1000, target=5000, current=3000 → (3000-1000)/(5000-1000) = 50%
		kr := KeyResult{MeasurementType: MeasurementCurrency, Direction: DirectionIncrease, Baseline: fp(1000), TargetValue: 5000, CurrentValue: 3000}
		got, err := kr.ComputeProgress()
		assertNoError(t, err)
		assertEqual(t, 50, got)
	})

	t.Run("increase/numeric no baseline", func(t *testing.T) {
		// NPS score 70 out of 80 = 87%
		kr := KeyResult{MeasurementType: MeasurementNumeric, Direction: DirectionIncrease, TargetValue: 80, CurrentValue: 70}
		got, err := kr.ComputeProgress()
		assertNoError(t, err)
		assertEqual(t, 88, got) // round(87.5) = 88
	})

	t.Run("increase/zero span with current at target", func(t *testing.T) {
		// baseline=50, target=50, current=50 → span is 0, current >= target → 100%
		kr := KeyResult{MeasurementType: MeasurementCount, Direction: DirectionIncrease, Baseline: fp(50), TargetValue: 50, CurrentValue: 50}
		got, err := kr.ComputeProgress()
		assertNoError(t, err)
		assertEqual(t, 100, got)
	})

	t.Run("increase/zero span with current below target", func(t *testing.T) {
		// baseline=50, target=50, current=40 → span is 0, current < target → 0%
		kr := KeyResult{MeasurementType: MeasurementCount, Direction: DirectionIncrease, Baseline: fp(50), TargetValue: 50, CurrentValue: 40}
		got, err := kr.ComputeProgress()
		assertNoError(t, err)
		assertEqual(t, 0, got)
	})

	t.Run("increase/current below baseline clamps to 0", func(t *testing.T) {
		// baseline=20, target=100, current=10 → (10-20)/(100-20) < 0 → 0%
		kr := KeyResult{MeasurementType: MeasurementCount, Direction: DirectionIncrease, Baseline: fp(20), TargetValue: 100, CurrentValue: 10}
		got, err := kr.ComputeProgress()
		assertNoError(t, err)
		assertEqual(t, 0, got)
	})

	t.Run("increase/current above target clamps to 100", func(t *testing.T) {
		// baseline=0, target=100, current=150 → 150% → clamped to 100%
		kr := KeyResult{MeasurementType: MeasurementCount, Direction: DirectionIncrease, TargetValue: 100, CurrentValue: 150}
		got, err := kr.ComputeProgress()
		assertNoError(t, err)
		assertEqual(t, 100, got)
	})

	t.Run("increase/exactly at target", func(t *testing.T) {
		kr := KeyResult{MeasurementType: MeasurementCount, Direction: DirectionIncrease, TargetValue: 100, CurrentValue: 100}
		got, err := kr.ComputeProgress()
		assertNoError(t, err)
		assertEqual(t, 100, got)
	})

	t.Run("increase/at baseline is 0 percent", func(t *testing.T) {
		kr := KeyResult{MeasurementType: MeasurementCount, Direction: DirectionIncrease, Baseline: fp(20), TargetValue: 100, CurrentValue: 20}
		got, err := kr.ComputeProgress()
		assertNoError(t, err)
		assertEqual(t, 0, got)
	})

	// --- Decrease direction ---
	t.Run("decrease/basic", func(t *testing.T) {
		// baseline=8, target=4, current=6 → (8-6)/(8-4) = 50%
		kr := KeyResult{MeasurementType: MeasurementNumeric, Direction: DirectionDecrease, Baseline: fp(8), TargetValue: 4, CurrentValue: 6}
		got, err := kr.ComputeProgress()
		assertNoError(t, err)
		assertEqual(t, 50, got)
	})

	t.Run("decrease/at target", func(t *testing.T) {
		// baseline=8, target=4, current=4 → (8-4)/(8-4) = 100%
		kr := KeyResult{MeasurementType: MeasurementNumeric, Direction: DirectionDecrease, Baseline: fp(8), TargetValue: 4, CurrentValue: 4}
		got, err := kr.ComputeProgress()
		assertNoError(t, err)
		assertEqual(t, 100, got)
	})

	t.Run("decrease/at baseline", func(t *testing.T) {
		// baseline=8, target=4, current=8 → (8-8)/(8-4) = 0%
		kr := KeyResult{MeasurementType: MeasurementNumeric, Direction: DirectionDecrease, Baseline: fp(8), TargetValue: 4, CurrentValue: 8}
		got, err := kr.ComputeProgress()
		assertNoError(t, err)
		assertEqual(t, 0, got)
	})

	t.Run("decrease/past target clamps to 100", func(t *testing.T) {
		// baseline=8, target=4, current=2 → (8-2)/(8-4) = 150% → clamped to 100%
		kr := KeyResult{MeasurementType: MeasurementNumeric, Direction: DirectionDecrease, Baseline: fp(8), TargetValue: 4, CurrentValue: 2}
		got, err := kr.ComputeProgress()
		assertNoError(t, err)
		assertEqual(t, 100, got)
	})

	t.Run("decrease/above baseline clamps to 0", func(t *testing.T) {
		// baseline=8, target=4, current=10 → (8-10)/(8-4) < 0 → 0%
		kr := KeyResult{MeasurementType: MeasurementNumeric, Direction: DirectionDecrease, Baseline: fp(8), TargetValue: 4, CurrentValue: 10}
		got, err := kr.ComputeProgress()
		assertNoError(t, err)
		assertEqual(t, 0, got)
	})

	t.Run("decrease/without baseline returns error", func(t *testing.T) {
		kr := KeyResult{MeasurementType: MeasurementNumeric, Direction: DirectionDecrease, TargetValue: 4, CurrentValue: 6}
		_, err := kr.ComputeProgress()
		if err == nil {
			t.Fatal("expected error for decrease direction without baseline")
		}
	})

	t.Run("decrease/zero span (baseline equals target)", func(t *testing.T) {
		// baseline=4, target=4 → span is 0 → 100% (already at target)
		kr := KeyResult{MeasurementType: MeasurementNumeric, Direction: DirectionDecrease, Baseline: fp(4), TargetValue: 4, CurrentValue: 4}
		got, err := kr.ComputeProgress()
		assertNoError(t, err)
		assertEqual(t, 100, got)
	})

	t.Run("decrease/error rate from 5% to 1%", func(t *testing.T) {
		// baseline=5, target=1, current=3 → (5-3)/(5-1) = 50%
		kr := KeyResult{MeasurementType: MeasurementPercentage, Direction: DirectionDecrease, Baseline: fp(5), TargetValue: 1, CurrentValue: 3}
		got, err := kr.ComputeProgress()
		assertNoError(t, err)
		assertEqual(t, 50, got)
	})

	// --- Maintain direction ---
	t.Run("maintain/zero bugs at target", func(t *testing.T) {
		// target=0, current=0 → 100% (meeting target)
		kr := KeyResult{MeasurementType: MeasurementCount, Direction: DirectionMaintain, TargetValue: 0, CurrentValue: 0}
		got, err := kr.ComputeProgress()
		assertNoError(t, err)
		assertEqual(t, 100, got)
	})

	t.Run("maintain/zero bugs violated", func(t *testing.T) {
		// target=0, current=2 → 0% (violated)
		kr := KeyResult{MeasurementType: MeasurementCount, Direction: DirectionMaintain, TargetValue: 0, CurrentValue: 2}
		got, err := kr.ComputeProgress()
		assertNoError(t, err)
		assertEqual(t, 0, got)
	})

	t.Run("maintain/uptime above threshold", func(t *testing.T) {
		// target=99.9, baseline=95 (< target → "keep at or above"), current=99.95 → 100%
		kr := KeyResult{MeasurementType: MeasurementPercentage, Direction: DirectionMaintain, Baseline: fp(95), TargetValue: 99.9, CurrentValue: 99.95}
		got, err := kr.ComputeProgress()
		assertNoError(t, err)
		assertEqual(t, 100, got)
	})

	t.Run("maintain/uptime below threshold", func(t *testing.T) {
		// target=99.9, baseline=95, current=99.5 → 0% (violated)
		kr := KeyResult{MeasurementType: MeasurementPercentage, Direction: DirectionMaintain, Baseline: fp(95), TargetValue: 99.9, CurrentValue: 99.5}
		got, err := kr.ComputeProgress()
		assertNoError(t, err)
		assertEqual(t, 0, got)
	})

	t.Run("maintain/keep at or below with baseline above target", func(t *testing.T) {
		// baseline=5 (> target=1) → "keep at or below 1", current=0.5 → 100%
		kr := KeyResult{MeasurementType: MeasurementPercentage, Direction: DirectionMaintain, Baseline: fp(5), TargetValue: 1, CurrentValue: 0.5}
		got, err := kr.ComputeProgress()
		assertNoError(t, err)
		assertEqual(t, 100, got)
	})

	t.Run("maintain/keep at or below violated", func(t *testing.T) {
		// baseline=5, target=1, current=2 → violated
		kr := KeyResult{MeasurementType: MeasurementPercentage, Direction: DirectionMaintain, Baseline: fp(5), TargetValue: 1, CurrentValue: 2}
		got, err := kr.ComputeProgress()
		assertNoError(t, err)
		assertEqual(t, 0, got)
	})

	t.Run("maintain/at exact target value is 100%", func(t *testing.T) {
		kr := KeyResult{MeasurementType: MeasurementCount, Direction: DirectionMaintain, TargetValue: 5, CurrentValue: 5, Baseline: fp(3)}
		got, err := kr.ComputeProgress()
		assertNoError(t, err)
		assertEqual(t, 100, got)
	})

	t.Run("maintain/no baseline with non-zero target and current at target", func(t *testing.T) {
		// No baseline, target=99.9 → default baseline 0 < 99.9 → "keep at or above"
		kr := KeyResult{MeasurementType: MeasurementPercentage, Direction: DirectionMaintain, TargetValue: 99.9, CurrentValue: 99.9}
		got, err := kr.ComputeProgress()
		assertNoError(t, err)
		assertEqual(t, 100, got)
	})

	t.Run("maintain/no baseline with non-zero target and current below", func(t *testing.T) {
		kr := KeyResult{MeasurementType: MeasurementPercentage, Direction: DirectionMaintain, TargetValue: 99.9, CurrentValue: 98.0}
		got, err := kr.ComputeProgress()
		assertNoError(t, err)
		assertEqual(t, 0, got)
	})

	// --- Edge cases ---
	t.Run("increase/zero target no baseline", func(t *testing.T) {
		// target=0, baseline=0 (default), span=0, current=0 → current >= target → 100%
		kr := KeyResult{MeasurementType: MeasurementCount, Direction: DirectionIncrease, TargetValue: 0, CurrentValue: 0}
		got, err := kr.ComputeProgress()
		assertNoError(t, err)
		assertEqual(t, 100, got)
	})

	t.Run("increase/negative current", func(t *testing.T) {
		// baseline=0, target=100, current=-10 → (-10-0)/(100-0) < 0 → 0%
		kr := KeyResult{MeasurementType: MeasurementNumeric, Direction: DirectionIncrease, TargetValue: 100, CurrentValue: -10}
		got, err := kr.ComputeProgress()
		assertNoError(t, err)
		assertEqual(t, 0, got)
	})

	t.Run("decrease/negative current", func(t *testing.T) {
		// baseline=10, target=0, current=-5 → (10-(-5))/(10-0) = 150% → clamped to 100%
		kr := KeyResult{MeasurementType: MeasurementNumeric, Direction: DirectionDecrease, Baseline: fp(10), TargetValue: 0, CurrentValue: -5}
		got, err := kr.ComputeProgress()
		assertNoError(t, err)
		assertEqual(t, 100, got)
	})
}

// ---------------------------------------------------------------------------
// KeyResult.ValidateTargetAdjustment
// ---------------------------------------------------------------------------

func TestKeyResult_ValidateTargetAdjustment(t *testing.T) {
	kr := KeyResult{
		ID:              "kr-1",
		MissionID:       "mis-1",
		Title:           "Test KR",
		MeasurementType: MeasurementCount,
		Direction:       DirectionIncrease,
		TargetValue:     100,
		Status:          KeyResultStatusOpen,
	}

	t.Run("change measurement type with no entries is allowed", func(t *testing.T) {
		newType := MeasurementPercentage
		if err := kr.ValidateTargetAdjustment(&newType, false); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("change measurement type with entries returns error", func(t *testing.T) {
		newType := MeasurementPercentage
		if err := kr.ValidateTargetAdjustment(&newType, true); err == nil {
			t.Fatal("expected error when changing measurement type with entries")
		}
	})

	t.Run("same measurement type with entries is allowed", func(t *testing.T) {
		sameType := MeasurementCount
		if err := kr.ValidateTargetAdjustment(&sameType, true); err != nil {
			t.Fatalf("expected no error for same type, got %v", err)
		}
	})

	t.Run("nil measurement type is allowed", func(t *testing.T) {
		if err := kr.ValidateTargetAdjustment(nil, true); err != nil {
			t.Fatalf("expected no error for nil type, got %v", err)
		}
	})
}

// ---------------------------------------------------------------------------
// AllNonDroppedKRsComplete
// ---------------------------------------------------------------------------

func TestAllNonDroppedKRsComplete(t *testing.T) {
	t.Run("all KRs at 100%", func(t *testing.T) {
		krs := []KeyResult{
			{Status: KeyResultStatusCompleted, ProgressPercent: 100},
			{Status: KeyResultStatusCompleted, ProgressPercent: 100},
		}
		if !AllNonDroppedKRsComplete(krs) {
			t.Fatal("expected true when all KRs are at 100%")
		}
	})

	t.Run("one KR not at 100%", func(t *testing.T) {
		krs := []KeyResult{
			{Status: KeyResultStatusCompleted, ProgressPercent: 100},
			{Status: KeyResultStatusOnTrack, ProgressPercent: 75},
		}
		if AllNonDroppedKRsComplete(krs) {
			t.Fatal("expected false when not all KRs are at 100%")
		}
	})

	t.Run("dropped KRs excluded", func(t *testing.T) {
		krs := []KeyResult{
			{Status: KeyResultStatusCompleted, ProgressPercent: 100},
			{Status: KeyResultStatusDropped, ProgressPercent: 30},
		}
		if !AllNonDroppedKRsComplete(krs) {
			t.Fatal("expected true when non-dropped KRs are at 100%")
		}
	})

	t.Run("all KRs dropped returns false", func(t *testing.T) {
		krs := []KeyResult{
			{Status: KeyResultStatusDropped, ProgressPercent: 30},
			{Status: KeyResultStatusDropped, ProgressPercent: 50},
		}
		if AllNonDroppedKRsComplete(krs) {
			t.Fatal("expected false when all KRs are dropped")
		}
	})

	t.Run("empty slice returns false", func(t *testing.T) {
		if AllNonDroppedKRsComplete(nil) {
			t.Fatal("expected false for nil slice")
		}
		if AllNonDroppedKRsComplete([]KeyResult{}) {
			t.Fatal("expected false for empty slice")
		}
	})

	t.Run("single KR at 100%", func(t *testing.T) {
		krs := []KeyResult{
			{Status: KeyResultStatusCompleted, ProgressPercent: 100},
		}
		if !AllNonDroppedKRsComplete(krs) {
			t.Fatal("expected true for single complete KR")
		}
	})

	t.Run("single KR at 0%", func(t *testing.T) {
		krs := []KeyResult{
			{Status: KeyResultStatusOpen, ProgressPercent: 0},
		}
		if AllNonDroppedKRsComplete(krs) {
			t.Fatal("expected false for single incomplete KR")
		}
	})
}

// ---------------------------------------------------------------------------
// ProgressEntry validation
// ---------------------------------------------------------------------------

func TestValidateProgressEntry(t *testing.T) {
	valid := ProgressEntry{
		ID:          "pe-1",
		KeyResultID: "kr-1",
		Value:       47,
		Progress:    47,
		UpdatedBy:   "operator",
		Note:        "Initial update",
		CreatedAt:   time.Now().UTC(),
	}

	t.Run("valid entry passes", func(t *testing.T) {
		if err := ValidateProgressEntry(valid); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("missing id", func(t *testing.T) {
		pe := valid
		pe.ID = ""
		if err := ValidateProgressEntry(pe); err == nil {
			t.Fatal("expected error for missing id")
		}
	})

	t.Run("missing keyResultId", func(t *testing.T) {
		pe := valid
		pe.KeyResultID = ""
		if err := ValidateProgressEntry(pe); err == nil {
			t.Fatal("expected error for missing keyResultId")
		}
	})

	t.Run("missing updatedBy", func(t *testing.T) {
		pe := valid
		pe.UpdatedBy = ""
		if err := ValidateProgressEntry(pe); err == nil {
			t.Fatal("expected error for missing updatedBy")
		}
	})

	t.Run("progress below zero", func(t *testing.T) {
		pe := valid
		pe.Progress = -1
		if err := ValidateProgressEntry(pe); err == nil {
			t.Fatal("expected error for negative progress")
		}
	})

	t.Run("progress above 100", func(t *testing.T) {
		pe := valid
		pe.Progress = 101
		if err := ValidateProgressEntry(pe); err == nil {
			t.Fatal("expected error for progress > 100")
		}
	})

	t.Run("empty note is allowed", func(t *testing.T) {
		pe := valid
		pe.Note = ""
		if err := ValidateProgressEntry(pe); err != nil {
			t.Fatalf("expected no error with empty note, got %v", err)
		}
	})

	t.Run("agent source format accepted", func(t *testing.T) {
		pe := valid
		pe.UpdatedBy = "member:coordinator"
		if err := ValidateProgressEntry(pe); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})
}

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

func assertNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func assertEqual(t *testing.T, expected, got int) {
	t.Helper()
	if expected != got {
		t.Fatalf("expected %d, got %d", expected, got)
	}
}
