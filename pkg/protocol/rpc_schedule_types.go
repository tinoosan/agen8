package protocol

import "time"

const (
	MethodScheduleCreate = "schedule.create"
	MethodScheduleGet    = "schedule.get"
	MethodScheduleList   = "schedule.list"
	MethodScheduleCancel = "schedule.cancel"
)

type ScheduleView struct {
	ID           string     `json:"id"`
	EntryID      string     `json:"entryId"`
	SpaceID      string     `json:"spaceId"`
	MemberID     string     `json:"memberId,omitempty"`
	CreatedBy    string     `json:"createdBy"`
	Name         string     `json:"name"`
	Goal         string     `json:"goal"`
	Status       string     `json:"status"`
	TargetKind   string     `json:"targetKind"`
	ScheduleType string     `json:"scheduleType"`
	ScheduleExpr string     `json:"scheduleExpr"`
	NextRunAt    *time.Time `json:"nextRunAt,omitempty"`
	ExpiresAt    *time.Time `json:"expiresAt,omitempty"`
	DedupeKey    string     `json:"dedupeKey,omitempty"`
	CreatedAt    time.Time  `json:"createdAt"`
	UpdatedAt    time.Time  `json:"updatedAt"`
	Runs         []RunView  `json:"runs,omitempty"`
}

type RunView struct {
	ID             string     `json:"id"`
	DueAt          time.Time  `json:"dueAt"`
	StartedAt      time.Time  `json:"startedAt"`
	FinishedAt     *time.Time `json:"finishedAt,omitempty"`
	Status         string     `json:"status"`
	TargetKind     string     `json:"targetKind"`
	TargetObjectID string     `json:"targetObjectId,omitempty"`
	Error          string     `json:"error,omitempty"`
}

type ScheduleCreateParams struct {
	SpaceID            string     `json:"spaceId"`
	Title              string     `json:"title"`
	Description        string     `json:"description,omitempty"`
	Mode               string     `json:"mode"`
	RunAt              *time.Time `json:"runAt,omitempty"`
	IntervalSeconds    int        `json:"intervalSeconds,omitempty"`
	Cron               string     `json:"cron,omitempty"`
	Timezone           string     `json:"timezone,omitempty"`
	TargetMemberID     string     `json:"targetMemberId"`
	TaskTitle          string     `json:"taskTitle"`
	TaskDescription    string     `json:"taskDescription"`
	AcceptanceCriteria []string   `json:"acceptanceCriteria,omitempty"`
	MissionRef         string     `json:"missionRef,omitempty"`
	KeyResultRef       string     `json:"keyResultRef,omitempty"`
	PlanTodoID         string     `json:"planTodoId,omitempty"`
	ExpiresAt          *time.Time `json:"expiresAt,omitempty"`
	DedupeKey          string     `json:"dedupeKey,omitempty"`
}

type ScheduleCreateResult struct {
	Schedule ScheduleView `json:"schedule"`
}

type ScheduleGetParams struct {
	ScheduleID string `json:"scheduleId"`
}

type ScheduleGetResult struct {
	Schedule ScheduleView `json:"schedule"`
}

type ScheduleListParams struct {
	SpaceID string `json:"spaceId"`
	Status  string `json:"status,omitempty"`
	Limit   int    `json:"limit,omitempty"`
}

type ScheduleListResult struct {
	Schedules []ScheduleView `json:"schedules"`
	Entries   []ScheduleView `json:"entries"`
}

type ScheduleCancelParams struct {
	ScheduleID string `json:"scheduleId"`
}

type ScheduleCancelResult struct {
	Schedule ScheduleView `json:"schedule"`
}
