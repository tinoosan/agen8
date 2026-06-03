package types

import "time"

type HeartbeatScheduleEntryContext struct {
	RelatedTaskID   string `json:"relatedTaskId,omitempty"`
	CheckCondition  string `json:"checkCondition,omitempty"`
	SuccessCriteria string `json:"successCriteria,omitempty"`
}

type HeartbeatScheduleEntry struct {
	EntryID         string
	SpaceID         string
	RoleID          string
	CreatedBy       string
	Name            string
	Goal            string
	Context         *HeartbeatScheduleEntryContext
	Priority        int
	ScheduleType    string
	ScheduleExpr    string
	Status          string
	ExpiresAt       *time.Time
	NextRunAt       *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
	GuardrailReason string

	// Mission linkage (optional — not all heartbeats relate to missions).
	MissionID   string `json:"missionId,omitempty"`
	KeyResultID string `json:"keyResultId,omitempty"`
	DedupeKey   string `json:"dedupeKey,omitempty"`
}

type HeartbeatManageCreateParams struct {
	TargetRole  string                         `json:"targetRole"`
	Name        string                         `json:"name"`
	Goal        string                         `json:"goal"`
	Context     *HeartbeatScheduleEntryContext `json:"context"`
	Priority    int                            `json:"priority"`
	Interval    string                         `json:"interval"`
	Schedule    string                         `json:"schedule"`
	OneOff      bool                           `json:"oneOff"`
	MissionID   string                         `json:"missionId"`
	KeyResultID string                         `json:"keyResultId"`
	DedupeKey   string                         `json:"dedupeKey"`
}

type HeartbeatManageUpdateParams struct {
	EntryID     string                         `json:"entryId"`
	TargetRole  string                         `json:"targetRole"`
	Name        string                         `json:"name"`
	Goal        string                         `json:"goal"`
	Context     *HeartbeatScheduleEntryContext `json:"context"`
	Priority    *int                           `json:"priority"`
	Interval    string                         `json:"interval"`
	Schedule    string                         `json:"schedule"`
	OneOff      *bool                          `json:"oneOff"`
	Renew       bool                           `json:"renew"`
	MissionID   string                         `json:"missionId"`
	KeyResultID string                         `json:"keyResultId"`
	DedupeKey   string                         `json:"dedupeKey"`
}
