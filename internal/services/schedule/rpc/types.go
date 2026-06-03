package rpc

import (
	"fmt"
	"time"

	schedule "github.com/tinoosan/agen8-mcp-server/internal/services/schedule/domain"
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

func NewScheduleView(entry schedule.Entry, runs []schedule.Run) ScheduleView {
	viewRuns := make([]RunView, 0, len(runs))
	for _, run := range runs {
		viewRuns = append(viewRuns, RunView{
			ID:             string(run.ID),
			DueAt:          run.DueAt,
			StartedAt:      run.StartedAt,
			FinishedAt:     cloneTime(run.FinishedAt),
			Status:         string(run.Status),
			TargetKind:     string(run.TargetKind),
			TargetObjectID: run.TargetObjectID,
			Error:          run.Error,
		})
	}
	return ScheduleView{
		ID:           string(entry.ID),
		EntryID:      string(entry.ID),
		SpaceID:      string(entry.SpaceID),
		MemberID:     string(entry.Target.TaskCreate.TargetMemberID),
		CreatedBy:    string(entry.CreatedBy),
		Name:         entry.Title,
		Goal:         entry.Target.TaskCreate.Description,
		Status:       string(entry.Status),
		TargetKind:   string(entry.Target.Kind),
		ScheduleType: scheduleType(entry.Timing),
		ScheduleExpr: scheduleExpr(entry.Timing),
		NextRunAt:    cloneTime(entry.NextRunAt),
		ExpiresAt:    cloneTime(entry.ExpiresAt),
		DedupeKey:    entry.DedupeKey,
		CreatedAt:    entry.CreatedAt,
		UpdatedAt:    entry.UpdatedAt,
		Runs:         viewRuns,
	}
}

func scheduleType(timing schedule.TimingExpression) string {
	switch timing.Mode {
	case schedule.TimingModeOnce:
		return "one_off"
	case schedule.TimingModeCron:
		return "cron"
	case schedule.TimingModeInterval:
		return "recurring"
	default:
		return string(timing.Mode)
	}
}

func scheduleExpr(timing schedule.TimingExpression) string {
	switch timing.Mode {
	case schedule.TimingModeOnce:
		if timing.RunAt == nil {
			return ""
		}
		return timing.RunAt.UTC().Format(time.RFC3339)
	case schedule.TimingModeCron:
		return timing.Cron
	case schedule.TimingModeInterval:
		return durationExpr(timing.Interval)
	default:
		return ""
	}
}

func durationExpr(d time.Duration) string {
	if d%time.Hour == 0 {
		return fmt.Sprintf("%dh", int(d/time.Hour))
	}
	if d%time.Minute == 0 {
		return fmt.Sprintf("%dm", int(d/time.Minute))
	}
	if d%time.Second == 0 {
		return fmt.Sprintf("%ds", int(d/time.Second))
	}
	return d.String()
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := value.UTC()
	return &cloned
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
