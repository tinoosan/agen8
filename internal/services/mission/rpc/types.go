package rpc

type MissionView struct {
	ID          string `json:"id"`
	ProjectID   string `json:"projectId"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Status      string `json:"status"`
	StartDate   string `json:"startDate,omitempty"`
	EndDate     string `json:"endDate,omitempty"`
	CreatedAt   string `json:"createdAt"`
	UpdatedAt   string `json:"updatedAt"`
	PausedAt    string `json:"pausedAt,omitempty"`
	CompletedAt string `json:"completedAt,omitempty"`
}

type CreateMissionParams struct {
	ProjectID   string `json:"projectId"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	StartDate   string `json:"startDate,omitempty"`
	EndDate     string `json:"endDate,omitempty"`
}

type CreateMissionResult struct {
	Mission MissionView `json:"mission"`
}

type GetMissionParams struct {
	MissionID string `json:"missionId"`
}

type GetMissionResult struct {
	Mission MissionView `json:"mission"`
}

type ListMissionsParams struct {
	ProjectID string   `json:"projectId"`
	Status    []string `json:"status,omitempty"`
	Limit     int      `json:"limit,omitempty"`
	Offset    int      `json:"offset,omitempty"`
}

type ListMissionsResult struct {
	Missions []MissionView `json:"missions"`
}

type UpdateMissionParams struct {
	MissionID   string  `json:"missionId"`
	Title       *string `json:"title,omitempty"`
	Description *string `json:"description,omitempty"`
	Status      *string `json:"status,omitempty"`
	StartDate   *string `json:"startDate,omitempty"`
	EndDate     *string `json:"endDate,omitempty"`
	Note        string  `json:"note,omitempty"`
}

type UpdateMissionResult struct {
	Mission MissionView `json:"mission"`
}

type DeleteMissionParams struct {
	MissionID string `json:"missionId"`
	Note      string `json:"note,omitempty"`
}

type DeleteMissionResult struct {
	Mission MissionView `json:"mission"`
}

type PurgeMissionParams struct {
	MissionID string `json:"missionId"`
	Note      string `json:"note,omitempty"`
}

type PurgeMissionResult struct {
	Mission MissionView `json:"mission"`
}

type CreateKeyResultParams struct {
	MissionID       string   `json:"missionId"`
	Title           string   `json:"title"`
	Description     string   `json:"description,omitempty"`
	MeasurementType string   `json:"measurementType"`
	Direction       string   `json:"direction,omitempty"`
	Unit            string   `json:"unit,omitempty"`
	Baseline        *float64 `json:"baseline,omitempty"`
	TargetValue     float64  `json:"targetValue"`
}

type KeyResultView struct {
	ID                    string   `json:"id"`
	MissionID             string   `json:"missionId"`
	Title                 string   `json:"title"`
	Description           string   `json:"description,omitempty"`
	MeasurementType       string   `json:"measurementType"`
	Direction             string   `json:"direction,omitempty"`
	Unit                  string   `json:"unit,omitempty"`
	Baseline              *float64 `json:"baseline,omitempty"`
	TargetValue           float64  `json:"targetValue"`
	CurrentValue          float64  `json:"currentValue"`
	ProgressPercent       int      `json:"progressPercent"`
	Status                string   `json:"status"`
	LastMilestoneNotified int      `json:"lastMilestoneNotified"`
	Version               int64    `json:"version"`
}

type CreateKeyResultResult struct {
	KeyResult KeyResultView `json:"keyResult"`
}

type GetKeyResultParams struct {
	KeyResultID string `json:"keyResultId"`
}

type GetKeyResultResult struct {
	KeyResult KeyResultView `json:"keyResult"`
}

type ListKeyResultsParams struct {
	MissionID string `json:"missionId"`
	Limit     int    `json:"limit,omitempty"`
	Offset    int    `json:"offset,omitempty"`
}

type ListKeyResultsResult struct {
	KeyResults []KeyResultView `json:"keyResults"`
}

type UpdateKeyResultParams struct {
	KeyResultID     string   `json:"keyResultId"`
	Title           *string  `json:"title,omitempty"`
	Description     *string  `json:"description,omitempty"`
	MeasurementType *string  `json:"measurementType,omitempty"`
	Direction       *string  `json:"direction,omitempty"`
	Unit            *string  `json:"unit,omitempty"`
	Baseline        *float64 `json:"baseline,omitempty"`
	TargetValue     *float64 `json:"targetValue,omitempty"`
}

type UpdateKeyResultResult struct {
	KeyResult KeyResultView `json:"keyResult"`
}

type DeleteKeyResultParams struct {
	KeyResultID string `json:"keyResultId"`
	Note        string `json:"note,omitempty"`
}

type DeleteKeyResultResult struct {
	KeyResult KeyResultView `json:"keyResult"`
}

type ReopenKeyResultParams struct {
	KeyResultID string `json:"keyResultId"`
	Note        string `json:"note,omitempty"`
}

type ReopenKeyResultResult struct {
	KeyResult KeyResultView `json:"keyResult"`
}

type UpdateProgressParams struct {
	KeyResultID     string  `json:"keyResultId"`
	Value           float64 `json:"value"`
	Note            string  `json:"note,omitempty"`
	ExpectedVersion int64   `json:"expectedVersion,omitempty"`
}

type UpdateProgressResult struct {
	KeyResult KeyResultView `json:"keyResult"`
}

type ProgressEntryView struct {
	ID              string  `json:"id"`
	KeyResultID     string  `json:"keyResultId"`
	PreviousValue   float64 `json:"previousValue"`
	NewValue        float64 `json:"newValue"`
	ProgressPercent int     `json:"progressPercent"`
	UpdatedBy       string  `json:"updatedBy"`
	Note            string  `json:"note,omitempty"`
	CreatedAt       string  `json:"createdAt"`
}

type ProgressHistoryParams struct {
	KeyResultID string `json:"keyResultId"`
}

type ProgressHistoryResult struct {
	Entries []ProgressEntryView `json:"entries"`
}

type MissionProgressParams struct {
	MissionID string `json:"missionId"`
}

type MissionProgressResult struct {
	MissionID          string                     `json:"missionId"`
	ProgressPercent    int                        `json:"progressPercent"`
	KeyResultCount     int                        `json:"keyResultCount"`
	StatusCounts       map[string]int             `json:"statusCounts"`
	BlockingKeyResults []MissionProgressKRSummary `json:"blockingKeyResults"`
}

type MissionProgressKRSummary struct {
	ID              string `json:"id"`
	Title           string `json:"title,omitempty"`
	Status          string `json:"status"`
	ProgressPercent int    `json:"progressPercent"`
}

type LifecycleHistoryParams struct {
	MissionID string `json:"missionId"`
	Limit     int    `json:"limit,omitempty"`
	Offset    int    `json:"offset,omitempty"`
}

type LifecycleHistoryResult struct {
	MissionID string                      `json:"missionId"`
	Entries   []LifecycleHistoryEntryView `json:"entries"`
	Count     int                         `json:"count"`
	Limit     int                         `json:"limit"`
	Offset    int                         `json:"offset"`
}

type LifecycleHistoryEntryView struct {
	EventID         string `json:"eventId,omitempty"`
	MissionID       string `json:"missionId"`
	KeyResultID     string `json:"keyResultId,omitempty"`
	Type            string `json:"type"`
	Action          string `json:"action"`
	Status          string `json:"status,omitempty"`
	Note            string `json:"note,omitempty"`
	Actor           string `json:"actor,omitempty"`
	Origin          string `json:"origin,omitempty"`
	Message         string `json:"message,omitempty"`
	ProgressPercent string `json:"progressPercent,omitempty"`
	Timestamp       string `json:"timestamp"`
}
