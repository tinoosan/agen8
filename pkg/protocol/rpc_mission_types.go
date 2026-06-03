package protocol

import (
	"time"
)

// Mission RPC method names.
const (
	MethodMissionCreate           = "mission.create"
	MethodMissionGet              = "mission.get"
	MethodMissionList             = "mission.list"
	MethodMissionUpdate           = "mission.update"
	MethodMissionDelete           = "mission.delete"
	MethodKeyResultCreate         = "keyResult.create"
	MethodKeyResultGet            = "keyResult.get"
	MethodKeyResultList           = "keyResult.list"
	MethodKeyResultUpdate         = "keyResult.update"
	MethodKeyResultDelete         = "keyResult.delete"
	MethodKeyResultSetSpace       = "keyResult.setSpace"
	MethodKeyResultUpdateProgress = "keyResult.updateProgress"
	MethodMissionGetProgress      = "mission.getProgress"
)

// MissionView is the wire-format read model for a mission.
type MissionView struct {
	ID          string     `json:"id"`
	ProjectID   string     `json:"projectId"`
	Title       string     `json:"title"`
	Description string     `json:"description,omitempty"`
	Status      string     `json:"status"`
	StartDate   *time.Time `json:"startDate,omitempty"`
	EndDate     *time.Time `json:"endDate,omitempty"`
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
	PausedAt    *time.Time `json:"pausedAt,omitempty"`
	CompletedAt *time.Time `json:"completedAt,omitempty"`
}

// KeyResultView is the wire-format read model for a key result.
type KeyResultView struct {
	ID                    string     `json:"id"`
	MissionID             string     `json:"missionId"`
	Title                 string     `json:"title"`
	Description           string     `json:"description,omitempty"`
	MeasurementType       string     `json:"measurementType"`
	Direction             string     `json:"direction"`
	Unit                  string     `json:"unit,omitempty"`
	Baseline              *float64   `json:"baseline,omitempty"`
	TargetValue           float64    `json:"targetValue"`
	CurrentValue          float64    `json:"currentValue"`
	ProgressPercent       int        `json:"progressPercent"`
	LastUpdatedBy         string     `json:"lastUpdatedBy,omitempty"`
	LastUpdateNote        string     `json:"lastUpdateNote,omitempty"`
	LastMilestoneNotified int        `json:"lastMilestoneNotified"`
	SpaceID               string     `json:"spaceId,omitempty"`
	OwnerSpaceName        string     `json:"ownerSpaceName,omitempty"`
	OwnerAssignedAt       *time.Time `json:"ownerAssignedAt,omitempty"`
	Status                string     `json:"status"`
	CreatedAt             time.Time  `json:"createdAt"`
	UpdatedAt             time.Time  `json:"updatedAt"`
	CompletedAt           *time.Time `json:"completedAt,omitempty"`
}

// -- mission.create --

type MissionCreateParams struct {
	ProjectID   string     `json:"projectId"`
	Title       string     `json:"title"`
	Description string     `json:"description,omitempty"`
	StartDate   *time.Time `json:"startDate,omitempty"`
	EndDate     *time.Time `json:"endDate,omitempty"`
}

type MissionCreateResult struct {
	Mission MissionView `json:"mission"`
}

// -- mission.get --

type MissionGetParams struct {
	MissionID string `json:"missionId"`
}

type MissionGetResult struct {
	Mission MissionView `json:"mission"`
}

// -- mission.list --

type MissionListParams struct {
	ProjectID string   `json:"projectId"`
	Statuses  []string `json:"statuses,omitempty"`
	Limit     int      `json:"limit,omitempty"`
	Offset    int      `json:"offset,omitempty"`
}

type MissionListResult struct {
	Missions []MissionView `json:"missions"`
}

// -- mission.update --

type MissionUpdateParams struct {
	MissionID   string     `json:"missionId"`
	Title       *string    `json:"title,omitempty"`
	Description *string    `json:"description,omitempty"`
	Status      *string    `json:"status,omitempty"`
	StartDate   *time.Time `json:"startDate,omitempty"`
	EndDate     *time.Time `json:"endDate,omitempty"`
}

type MissionUpdateResult struct {
	Mission MissionView `json:"mission"`
}

// -- mission.delete --

type MissionDeleteParams struct {
	MissionID string `json:"missionId"`
}

type MissionDeleteResult struct {
	Deleted bool `json:"deleted"`
}

// -- mission.getProgress --

// MissionGetProgressParams are the params for mission.getProgress.
type MissionGetProgressParams struct {
	MissionID string `json:"missionId"`
}

// KeyResultProgressView is the wire-format read model for a single key result's
// computed progress snapshot.
type KeyResultProgressView struct {
	ID              string   `json:"id"`
	Title           string   `json:"title"`
	Status          string   `json:"status"`
	MeasurementType string   `json:"measurementType"`
	Direction       string   `json:"direction"`
	ProgressPercent int      `json:"progressPercent"`
	TargetValue     float64  `json:"targetValue"`
	CurrentValue    float64  `json:"currentValue"`
	Unit            string   `json:"unit,omitempty"`
	Baseline        *float64 `json:"baseline,omitempty"`
	LinkedDecisions int      `json:"linkedDecisions"`
}

// MissionGetProgressResult is the result for mission.getProgress.
type MissionGetProgressResult struct {
	MissionID      string                  `json:"missionId"`
	Title          string                  `json:"title"`
	Status         string                  `json:"status"`
	OverallPercent float64                 `json:"overallPercent"`
	KeyResults     []KeyResultProgressView `json:"keyResults"`
}

// -- keyResult.create --

type KeyResultCreateParams struct {
	MissionID       string   `json:"missionId"`
	Title           string   `json:"title"`
	Description     string   `json:"description,omitempty"`
	MeasurementType string   `json:"measurementType"`
	Direction       string   `json:"direction"`
	Unit            string   `json:"unit,omitempty"`
	Baseline        *float64 `json:"baseline,omitempty"`
	TargetValue     float64  `json:"targetValue"`
}

type KeyResultCreateResult struct {
	KeyResult KeyResultView `json:"keyResult"`
}

// -- keyResult.get --

type KeyResultGetParams struct {
	KeyResultID string `json:"keyResultId"`
}

type KeyResultGetResult struct {
	KeyResult KeyResultView `json:"keyResult"`
}

// -- keyResult.list --

type KeyResultListParams struct {
	MissionID string `json:"missionId"`
}

type KeyResultListResult struct {
	KeyResults []KeyResultView `json:"keyResults"`
}

// -- keyResult.update --

type KeyResultUpdateParams struct {
	KeyResultID     string   `json:"keyResultId"`
	Title           *string  `json:"title,omitempty"`
	Description     *string  `json:"description,omitempty"`
	MeasurementType *string  `json:"measurementType,omitempty"`
	Direction       *string  `json:"direction,omitempty"`
	Unit            *string  `json:"unit,omitempty"`
	Baseline        *float64 `json:"baseline,omitempty"`
	TargetValue     *float64 `json:"targetValue,omitempty"`
	CurrentValue    *float64 `json:"currentValue,omitempty"`
	Status          *string  `json:"status,omitempty"`
}

type KeyResultUpdateResult struct {
	KeyResult KeyResultView `json:"keyResult"`
}

// -- keyResult.delete --

type KeyResultDeleteParams struct {
	KeyResultID string `json:"keyResultId"`
}

type KeyResultDeleteResult struct {
	Deleted bool `json:"deleted"`
}

// -- keyResult.setSpace --

type KeyResultSetSpaceParams struct {
	KeyResultID string `json:"keyResultId"`
	SpaceID     string `json:"spaceId"`
}

// -- keyResult.updateProgress --

type KeyResultUpdateProgressParams struct {
	KeyResultID string  `json:"keyResultId"`
	Value       float64 `json:"value"`
	Note        string  `json:"note"` // Required — explains what was measured
}

type KeyResultUpdateProgressResult struct {
	KeyResult KeyResultView `json:"keyResult"`
}

// -- keyResult.progressHistory --

const MethodKeyResultProgressHistory = "keyResult.progressHistory"

type KeyResultProgressHistoryParams struct {
	KeyResultID string `json:"keyResultId"`
	Limit       int    `json:"limit,omitempty"` // Default: 50
}

type ProgressEntryView struct {
	ID          string  `json:"id"`
	KeyResultID string  `json:"keyResultId"`
	Value       float64 `json:"value"`
	Progress    int     `json:"progress"`
	UpdatedBy   string  `json:"updatedBy"`
	Note        string  `json:"note,omitempty"`
	CreatedAt   string  `json:"createdAt"`
}

type KeyResultProgressHistoryResult struct {
	Entries []ProgressEntryView `json:"entries"`
}
