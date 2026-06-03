package schedule

import (
	"context"
	"encoding/json"

	scheduleapp "github.com/tinoosan/agen8-mcp-server/internal/services/schedule/app"
	scheduledomain "github.com/tinoosan/agen8-mcp-server/internal/services/schedule/domain"
)

type Service interface {
	Create(context.Context, scheduleapp.CreateParams) (scheduledomain.Entry, error)
	Get(context.Context, scheduledomain.EntryID) (scheduledomain.Entry, []scheduledomain.Run, error)
	List(context.Context, scheduledomain.Filter) ([]scheduledomain.Entry, error)
	Update(context.Context, scheduleapp.UpdateParams) (scheduledomain.Entry, error)
	Cancel(context.Context, scheduledomain.EntryID) (scheduledomain.Entry, error)
}

type CallContext struct {
	Schedules     Service
	SpaceID       string
	ActorMemberID string
}

type Result struct {
	Text       string
	Structured any
}

type rawRequest struct {
	Action             string   `json:"action"`
	ScheduleID         *string  `json:"schedule_id"`
	Title              *string  `json:"title"`
	Description        *string  `json:"description"`
	Mode               *string  `json:"mode"`
	RunAt              *string  `json:"run_at"`
	IntervalSeconds    *int     `json:"interval_seconds"`
	Cron               *string  `json:"cron"`
	Timezone           *string  `json:"timezone"`
	TargetKind         *string  `json:"target_kind"`
	TargetMemberID     *string  `json:"target_member_id"`
	TaskTitle          *string  `json:"task_title"`
	TaskDescription    *string  `json:"task_description"`
	AcceptanceCriteria []string `json:"acceptance_criteria"`
	MissionRef         *string  `json:"mission_ref"`
	KeyResultRef       *string  `json:"key_result_ref"`
	Status             *string  `json:"status"`
	Limit              *int     `json:"limit"`
	ExpiresAt          *string  `json:"expires_at"`
	DedupeKey          *string  `json:"dedupe_key"`
}

type requestInput struct {
	Action             string
	ScheduleID         string
	Title              string
	Description        string
	Mode               string
	RunAt              string
	IntervalSeconds    int
	Cron               string
	Timezone           string
	TargetKind         string
	TargetMemberID     string
	TaskTitle          string
	TaskDescription    string
	AcceptanceCriteria []string
	MissionRef         string
	KeyResultRef       string
	Status             string
	Limit              int
	ExpiresAt          string
	DedupeKey          string
}

func rawMap(args json.RawMessage) (map[string]json.RawMessage, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(args, &raw); err != nil {
		return nil, err
	}
	return raw, nil
}
