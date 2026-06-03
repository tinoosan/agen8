package types

import (
	"fmt"
	"math"
	"strings"
	"time"
)

// MissionID is the unique identifier for a mission.
type MissionID string

// KeyResultID is the unique identifier for a key result.
type KeyResultID string

// MissionStatus represents the lifecycle state of a mission.
type MissionStatus string

const (
	MissionStatusDraft     MissionStatus = "draft"     // Defined but not started
	MissionStatusActive    MissionStatus = "active"    // Currently being pursued
	MissionStatusPaused    MissionStatus = "paused"    // Temporarily suspended
	MissionStatusCompleted MissionStatus = "completed" // All key results achieved
	MissionStatusArchived  MissionStatus = "archived"  // No longer relevant
)

// ValidMissionStatuses is the set of all valid mission statuses.
var ValidMissionStatuses = []MissionStatus{
	MissionStatusDraft, MissionStatusActive, MissionStatusPaused,
	MissionStatusCompleted, MissionStatusArchived,
}

// validMissionTransitions defines allowed status transitions.
// Key is current status, value is set of statuses it can transition to.
var validMissionTransitions = map[MissionStatus]map[MissionStatus]bool{
	MissionStatusDraft: {
		MissionStatusActive:   true,
		MissionStatusArchived: true,
	},
	MissionStatusActive: {
		MissionStatusPaused:    true,
		MissionStatusCompleted: true,
		MissionStatusArchived:  true,
	},
	MissionStatusPaused: {
		MissionStatusActive:   true,
		MissionStatusArchived: true,
	},
	MissionStatusCompleted: {
		MissionStatusActive:   true, // Scope expansion: new KR added auto-reactivates (D83)
		MissionStatusArchived: true,
	},
	MissionStatusArchived: {},
}

// Mission represents a strategic objective for the project.
// Multiple missions can be active simultaneously.
type Mission struct {
	ID          MissionID     `json:"id"`
	ProjectID   string        `json:"projectId"`
	Title       string        `json:"title"`
	Description string        `json:"description,omitempty"`
	Status      MissionStatus `json:"status"`
	StartDate   *time.Time    `json:"startDate,omitempty"` // Optional — when this mission begins
	EndDate     *time.Time    `json:"endDate,omitempty"`   // Optional — target completion date
	CreatedAt   time.Time     `json:"createdAt"`
	UpdatedAt   time.Time     `json:"updatedAt"`
	PausedAt    *time.Time    `json:"pausedAt,omitempty"`
	CompletedAt *time.Time    `json:"completedAt,omitempty"`
}

// Validate checks that the mission has all required fields and valid values.
// Returns an error for any invalid input -- never clamps or defaults.
func (m *Mission) Validate() error {
	if strings.TrimSpace(m.Title) == "" {
		return fmt.Errorf("title is required")
	}
	if strings.TrimSpace(m.ProjectID) == "" {
		return fmt.Errorf("projectId is required")
	}
	switch m.Status {
	case MissionStatusDraft, MissionStatusActive, MissionStatusPaused,
		MissionStatusCompleted, MissionStatusArchived:
	default:
		return fmt.Errorf("invalid status: %q", m.Status)
	}
	if m.StartDate != nil && m.EndDate != nil && m.EndDate.Before(*m.StartDate) {
		return fmt.Errorf("endDate cannot be before startDate")
	}
	return nil
}

// ValidateTransition checks whether the mission can move from its current status
// to the given new status. Returns an error if the transition is not allowed.
func (m *Mission) ValidateTransition(newStatus MissionStatus) error {
	// Validate the new status is a known value first.
	switch newStatus {
	case MissionStatusDraft, MissionStatusActive, MissionStatusPaused,
		MissionStatusCompleted, MissionStatusArchived:
	default:
		return fmt.Errorf("invalid target status: %q", newStatus)
	}

	if m.Status == newStatus {
		return nil // No-op transition is always allowed.
	}

	allowed, ok := validMissionTransitions[m.Status]
	if !ok {
		return fmt.Errorf("no transitions defined for status %q", m.Status)
	}
	if !allowed[newStatus] {
		return fmt.Errorf("cannot transition from %q to %q", m.Status, newStatus)
	}
	return nil
}

// AllNonDroppedKRsComplete returns true if every KR in the given slice with a
// status other than "dropped" has a ProgressPercent of 100. Returns false if
// there are no non-dropped KRs (a mission with all KRs dropped does NOT
// auto-complete per D74).
func AllNonDroppedKRsComplete(krs []KeyResult) bool {
	nonDropped := 0
	for _, kr := range krs {
		if kr.Status == KeyResultStatusDropped {
			continue
		}
		nonDropped++
		if kr.ProgressPercent != 100 {
			return false
		}
	}
	return nonDropped > 0
}

// KeyResultStatus represents the lifecycle state of a key result.
type KeyResultStatus string

const (
	KeyResultStatusOpen      KeyResultStatus = "open"      // Not started
	KeyResultStatusOnTrack   KeyResultStatus = "on_track"  // Progress is healthy
	KeyResultStatusAtRisk    KeyResultStatus = "at_risk"   // Behind schedule
	KeyResultStatusCompleted KeyResultStatus = "completed" // Target achieved
	KeyResultStatusDropped   KeyResultStatus = "dropped"   // No longer pursued
)

// MeasurementType defines how a key result's value is measured.
type MeasurementType string

const (
	MeasurementBinary     MeasurementType = "binary"     // Done or not done
	MeasurementCount      MeasurementType = "count"      // Current count toward target count
	MeasurementPercentage MeasurementType = "percentage" // Current % toward target %
	MeasurementCurrency   MeasurementType = "currency"   // Current amount toward target amount
	MeasurementNumeric    MeasurementType = "numeric"    // Generic number (duration, score, etc.)
)

// ValidMeasurementTypes is the set of all valid measurement types.
var ValidMeasurementTypes = []MeasurementType{
	MeasurementBinary, MeasurementCount, MeasurementPercentage,
	MeasurementCurrency, MeasurementNumeric,
}

// Direction indicates whether the key result value should go up or down.
type Direction string

const (
	DirectionIncrease Direction = "increase" // Higher is better (revenue, users, score)
	DirectionDecrease Direction = "decrease" // Lower is better (latency, error rate, cost)
	DirectionMaintain Direction = "maintain" // Stay at target (zero bugs, SLA compliance)
)

// ValidDirections is the set of all valid directions.
var ValidDirections = []Direction{
	DirectionIncrease, DirectionDecrease, DirectionMaintain,
}

// KeyResult is a measurable objective that contributes to a Mission.
// Progress is reported by coordinators (via tool) and operators (via UI).
// No auto-calculation — progress comes from intentional measurement.
type KeyResult struct {
	ID          KeyResultID `json:"id"`
	MissionID   MissionID   `json:"missionId"`
	Title       string      `json:"title"`
	Description string      `json:"description,omitempty"`

	// Typed measurement model
	MeasurementType MeasurementType `json:"measurementType"`
	Direction       Direction       `json:"direction"`
	Unit            string          `json:"unit,omitempty"`
	Baseline        *float64        `json:"baseline,omitempty"`
	TargetValue     float64         `json:"targetValue"`
	CurrentValue    float64         `json:"currentValue"`

	// Computed (derived from baseline/target/current/direction)
	ProgressPercent int `json:"progressPercent"` // 0-100

	// Progress tracking
	LastUpdatedBy         string `json:"lastUpdatedBy,omitempty"`
	LastUpdateNote        string `json:"lastUpdateNote,omitempty"`
	LastMilestoneNotified int    `json:"lastMilestoneNotified"` // 0, 25, 50, 75, or 100

	// Space assignment — single owning space. The space must be in
	// `open` status when assigned (validated by the mission service).
	// OwnerSpaceName is a derived view field populated by the mission
	// repository's read-time join against the spaces table; it is
	// never stored on the key_results row.
	SpaceID         string     `json:"spaceId,omitempty"`
	OwnerSpaceName  string     `json:"ownerSpaceName,omitempty"`
	OwnerAssignedAt *time.Time `json:"ownerAssignedAt,omitempty"`

	Status      KeyResultStatus `json:"status"`
	CreatedAt   time.Time       `json:"createdAt"`
	UpdatedAt   time.Time       `json:"updatedAt"`
	CompletedAt *time.Time      `json:"completedAt,omitempty"`
}

// Validate checks that the key result has all required fields and valid values.
// Returns an error for any invalid input -- never clamps or defaults.
func (kr *KeyResult) Validate() error {
	if strings.TrimSpace(kr.Title) == "" {
		return fmt.Errorf("title is required")
	}
	if strings.TrimSpace(string(kr.MissionID)) == "" {
		return fmt.Errorf("missionId is required")
	}
	switch kr.Status {
	case KeyResultStatusOpen, KeyResultStatusOnTrack, KeyResultStatusAtRisk,
		KeyResultStatusCompleted, KeyResultStatusDropped:
	default:
		return fmt.Errorf("invalid status: %q", kr.Status)
	}
	switch kr.MeasurementType {
	case MeasurementBinary, MeasurementCount, MeasurementPercentage,
		MeasurementCurrency, MeasurementNumeric:
	default:
		return fmt.Errorf("invalid measurement type: %q", kr.MeasurementType)
	}
	switch kr.Direction {
	case DirectionIncrease, DirectionDecrease, DirectionMaintain:
	default:
		return fmt.Errorf("invalid direction: %q", kr.Direction)
	}
	// Direction=decrease REQUIRES baseline — can't decrease from 0.
	if kr.Direction == DirectionDecrease && kr.Baseline == nil {
		return fmt.Errorf("baseline is required for direction %q", kr.Direction)
	}
	if kr.ProgressPercent < 0 || kr.ProgressPercent > 100 {
		return fmt.Errorf("progressPercent must be in range [0, 100], got %d", kr.ProgressPercent)
	}
	return nil
}

// ComputeProgress calculates ProgressPercent from current/target/baseline/direction.
// Returns an error if the state is invalid for computation (e.g., decrease without baseline).
// The returned int is in range [0, 100].
func (kr *KeyResult) ComputeProgress() (int, error) {
	if kr.MeasurementType == MeasurementBinary {
		if kr.CurrentValue >= 1 {
			return 100, nil
		}
		return 0, nil
	}

	// Maintain: progress is 100% if current value meets the target, 0% if violated.
	if kr.Direction == DirectionMaintain {
		baseline := 0.0
		if kr.Baseline != nil {
			baseline = *kr.Baseline
		}
		if baseline >= kr.TargetValue || (kr.Baseline == nil && kr.TargetValue == 0) {
			if kr.CurrentValue <= kr.TargetValue {
				return 100, nil
			}
			return 0, nil
		}
		if kr.CurrentValue >= kr.TargetValue {
			return 100, nil
		}
		return 0, nil
	}

	baseline := 0.0
	if kr.Baseline != nil {
		baseline = *kr.Baseline
	}

	if kr.Direction == DirectionDecrease {
		if kr.Baseline == nil {
			return 0, fmt.Errorf("baseline is required for direction %q", kr.Direction)
		}
		span := baseline - kr.TargetValue
		if span == 0 {
			return 100, nil
		}
		progress := (baseline - kr.CurrentValue) / span * 100
		return clampProgress(progress), nil
	}

	// Direction = increase
	span := kr.TargetValue - baseline
	if span == 0 {
		if kr.CurrentValue >= kr.TargetValue {
			return 100, nil
		}
		return 0, nil
	}
	progress := (kr.CurrentValue - baseline) / span * 100
	return clampProgress(progress), nil
}

// clampProgress constrains a float progress value to the integer range [0, 100].
// Uses math.Round to avoid truncation (e.g. 99.7% → 100, not 99).
func clampProgress(progress float64) int {
	if progress < 0 {
		return 0
	}
	if progress > 100 {
		return 100
	}
	return int(math.Round(progress))
}

// ValidateTargetAdjustment checks whether the given set of field changes are
// allowed on this KR. The hasProgressEntries parameter indicates whether any
// ProgressEntry records exist for this KR.
//
// Rules (D79):
//   - Cannot change measurementType if progress entries exist
//     (would invalidate audit history).
//   - All other editable fields (targetValue, baseline, direction,
//     title, description, unit) are always allowed.
func (kr *KeyResult) ValidateTargetAdjustment(newMeasurementType *MeasurementType, hasProgressEntries bool) error {
	if newMeasurementType != nil && *newMeasurementType != kr.MeasurementType && hasProgressEntries {
		return fmt.Errorf("cannot change measurementType from %q to %q: progress entries exist", kr.MeasurementType, *newMeasurementType)
	}
	return nil
}

// ProgressEntry records a single progress update for the audit trail.
// Entries are append-only — they are never modified or deleted.
type ProgressEntry struct {
	ID          string      `json:"id"`
	KeyResultID KeyResultID `json:"keyResultId"`
	Value       float64     `json:"value"`          // The reported value
	Progress    int         `json:"progress"`       // Computed progress at time of update
	UpdatedBy   string      `json:"updatedBy"`      // "member:<label>" or "operator"
	Note        string      `json:"note,omitempty"` // Context for the update
	CreatedAt   time.Time   `json:"createdAt"`
}

// ValidateProgressEntry checks that a progress entry has all required fields.
func ValidateProgressEntry(pe ProgressEntry) error {
	if strings.TrimSpace(pe.ID) == "" {
		return fmt.Errorf("id is required")
	}
	if strings.TrimSpace(string(pe.KeyResultID)) == "" {
		return fmt.Errorf("keyResultId is required")
	}
	if strings.TrimSpace(pe.UpdatedBy) == "" {
		return fmt.Errorf("updatedBy is required")
	}
	if pe.Progress < 0 || pe.Progress > 100 {
		return fmt.Errorf("progress must be in range [0, 100], got %d", pe.Progress)
	}
	return nil
}

// MissionFilter controls optional filtering and pagination for FindByProject queries.
type MissionFilter struct {
	Statuses []MissionStatus
	Limit    int
	Offset   int
}
