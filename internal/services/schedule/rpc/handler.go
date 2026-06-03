package rpc

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/tinoosan/agen8-mcp-server/internal/caller"
	scheduleapp "github.com/tinoosan/agen8-mcp-server/internal/services/schedule/app"
	schedule "github.com/tinoosan/agen8-mcp-server/internal/services/schedule/domain"
	spacedomain "github.com/tinoosan/agen8-mcp-server/internal/services/space/domain"
	"github.com/tinoosan/agen8-mcp-server/internal/services/space/domain/member"
)

type Handler struct {
	svc *scheduleapp.Service
}

func NewHandler(svc *scheduleapp.Service) *Handler {
	if svc == nil {
		panic("schedule RPC handler requires schedule service")
	}
	return &Handler{svc: svc}
}

func (h *Handler) Create(ctx context.Context, p ScheduleCreateParams) (ScheduleCreateResult, error) {
	actor, err := caller.ContextResolver{}.ResolveCaller(ctx)
	if err != nil {
		return ScheduleCreateResult{}, internalError("resolve schedule caller", err)
	}
	spaceID := strings.TrimSpace(p.SpaceID)
	if spaceID == "" {
		return ScheduleCreateResult{}, invalidParams("spaceId is required")
	}
	title := strings.TrimSpace(p.Title)
	if title == "" {
		return ScheduleCreateResult{}, invalidParams("title is required")
	}
	timing, err := timingFromParams(p)
	if err != nil {
		return ScheduleCreateResult{}, err
	}
	targetMemberID := strings.TrimSpace(p.TargetMemberID)
	if targetMemberID == "" {
		return ScheduleCreateResult{}, invalidParams("targetMemberId is required")
	}
	taskTitle := strings.TrimSpace(p.TaskTitle)
	if taskTitle == "" {
		taskTitle = title
	}
	taskDescription := strings.TrimSpace(p.TaskDescription)
	if taskDescription == "" {
		return ScheduleCreateResult{}, invalidParams("taskDescription is required")
	}
	entry, err := h.svc.Create(ctx, scheduleapp.CreateParams{
		SpaceID:     spacedomain.SpaceID(spaceID),
		CreatedBy:   schedule.ActorRef(actor.ActorID()),
		Title:       title,
		Description: strings.TrimSpace(p.Description),
		Timing:      timing,
		Target: schedule.Target{
			Kind: schedule.TargetKindTaskCreate,
			TaskCreate: schedule.TaskCreatePayload{
				TargetMemberID:     member.ID(targetMemberID),
				Title:              taskTitle,
				Description:        taskDescription,
				AcceptanceCriteria: append([]string(nil), p.AcceptanceCriteria...),
				MissionID:          strings.TrimSpace(p.MissionRef),
				KeyResultID:        strings.TrimSpace(p.KeyResultRef),
				PlanTodoID:         strings.TrimSpace(p.PlanTodoID),
			},
		},
		ExpiresAt: p.ExpiresAt,
		DedupeKey: p.DedupeKey,
	})
	if err != nil {
		return ScheduleCreateResult{}, internalError("create schedule", err)
	}
	return ScheduleCreateResult{Schedule: NewScheduleView(entry, nil)}, nil
}

func (h *Handler) Get(ctx context.Context, p ScheduleGetParams) (ScheduleGetResult, error) {
	id := strings.TrimSpace(p.ScheduleID)
	if id == "" {
		return ScheduleGetResult{}, invalidParams("scheduleId is required")
	}
	entry, runs, err := h.svc.Get(ctx, schedule.EntryID(id))
	if err != nil {
		return ScheduleGetResult{}, internalError("get schedule", err)
	}
	return ScheduleGetResult{Schedule: NewScheduleView(entry, runs)}, nil
}

func (h *Handler) List(ctx context.Context, p ScheduleListParams) (ScheduleListResult, error) {
	spaceID := strings.TrimSpace(p.SpaceID)
	if spaceID == "" {
		return ScheduleListResult{}, invalidParams("spaceId is required")
	}
	if p.Limit < 0 {
		return ScheduleListResult{}, invalidParams("limit must be non-negative")
	}
	limit := p.Limit
	if limit == 0 {
		limit = 100
	}
	entries, err := h.svc.List(ctx, schedule.Filter{
		SpaceID: spacedomain.SpaceID(spaceID),
		Status:  schedule.EntryStatus(strings.TrimSpace(p.Status)),
		Limit:   limit,
	})
	if err != nil {
		return ScheduleListResult{}, internalError("list schedules", err)
	}
	views := make([]ScheduleView, 0, len(entries))
	for _, entry := range entries {
		loaded, runs, err := h.svc.Get(ctx, entry.ID)
		if err != nil {
			return ScheduleListResult{}, internalError("list schedule runs", err)
		}
		views = append(views, NewScheduleView(loaded, runs))
	}
	return ScheduleListResult{Schedules: views, Entries: views}, nil
}

func (h *Handler) Cancel(ctx context.Context, p ScheduleCancelParams) (ScheduleCancelResult, error) {
	id := strings.TrimSpace(p.ScheduleID)
	if id == "" {
		return ScheduleCancelResult{}, invalidParams("scheduleId is required")
	}
	entry, err := h.svc.Cancel(ctx, schedule.EntryID(id))
	if err != nil {
		return ScheduleCancelResult{}, internalError("cancel schedule", err)
	}
	return ScheduleCancelResult{Schedule: NewScheduleView(entry, nil)}, nil
}

func timingFromParams(p ScheduleCreateParams) (schedule.TimingExpression, error) {
	switch strings.TrimSpace(p.Mode) {
	case "once":
		if p.RunAt == nil {
			return schedule.TimingExpression{}, invalidParams("runAt is required")
		}
		runAt := p.RunAt.UTC()
		return schedule.TimingExpression{Mode: schedule.TimingModeOnce, RunAt: &runAt}, nil
	case "interval":
		if p.IntervalSeconds <= 0 {
			return schedule.TimingExpression{}, invalidParams("intervalSeconds must be positive")
		}
		return schedule.TimingExpression{Mode: schedule.TimingModeInterval, Interval: time.Duration(p.IntervalSeconds) * time.Second}, nil
	case "cron":
		return schedule.TimingExpression{Mode: schedule.TimingModeCron, Cron: strings.TrimSpace(p.Cron), Timezone: strings.TrimSpace(p.Timezone)}, nil
	default:
		return schedule.TimingExpression{}, invalidParams("mode must be once, interval, or cron")
	}
}

func invalidParams(message string) error {
	return rpcError{code: -32602, message: strings.TrimSpace(message)}
}

func internalError(action string, err error) error {
	return fmt.Errorf("%s: %w", action, err)
}

type rpcError struct {
	code    int
	message string
}

func (e rpcError) Error() string {
	return e.message
}

func (e rpcError) RPCCode() int {
	return e.code
}
