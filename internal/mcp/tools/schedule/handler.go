package schedule

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	scheduleapp "github.com/tinoosan/agen8-mcp-server/internal/services/schedule/app"
	scheduledomain "github.com/tinoosan/agen8-mcp-server/internal/services/schedule/domain"
	spacedomain "github.com/tinoosan/agen8-mcp-server/internal/services/space/domain"
	"github.com/tinoosan/agen8-mcp-server/internal/services/space/domain/member"
)

type Handler struct{}

func NewHandler() Handler { return Handler{} }

func (h Handler) Handle(ctx context.Context, call CallContext, args json.RawMessage) (Result, error) {
	input, err := decode(args)
	if err != nil {
		return Result{}, err
	}
	if call.Schedules == nil {
		return Result{}, fmt.Errorf("schedule: schedule service is not configured")
	}
	switch input.Action {
	case "create":
		timing, err := timingFromInput(input)
		if err != nil {
			return Result{}, err
		}
		target, err := targetFromInput(input)
		if err != nil {
			return Result{}, err
		}
		expiresAt, err := optionalTime(input.ExpiresAt)
		if err != nil {
			return Result{}, fmt.Errorf("schedule: expires_at: %w", err)
		}
		entry, err := call.Schedules.Create(ctx, scheduleapp.CreateParams{
			SpaceID:     spacedomain.SpaceID(strings.TrimSpace(call.SpaceID)),
			CreatedBy:   scheduledomain.ActorRef(strings.TrimSpace(call.ActorMemberID)),
			Title:       input.Title,
			Description: input.Description,
			Timing:      timing,
			Target:      target,
			ExpiresAt:   expiresAt,
			DedupeKey:   input.DedupeKey,
		})
		return h.entryResult("create", entry, nil, err)
	case "get":
		id, err := requireScheduleID(input.ScheduleID)
		if err != nil {
			return Result{}, err
		}
		entry, runs, err := call.Schedules.Get(ctx, id)
		return h.entryResult("get", entry, runs, err)
	case "list":
		limit := input.Limit
		if limit == 0 {
			limit = 20
		}
		entries, err := call.Schedules.List(ctx, scheduledomain.Filter{
			SpaceID: spacedomain.SpaceID(strings.TrimSpace(call.SpaceID)),
			Status:  scheduledomain.EntryStatus(input.Status),
			Limit:   limit,
		})
		return h.listResult(entries, err)
	case "update":
		id, err := requireScheduleID(input.ScheduleID)
		if err != nil {
			return Result{}, err
		}
		params := scheduleapp.UpdateParams{EntryID: id}
		if input.Title != "" {
			params.Title = &input.Title
		}
		if input.Description != "" {
			params.Description = &input.Description
		}
		if input.Mode != "" {
			timing, err := timingFromInput(input)
			if err != nil {
				return Result{}, err
			}
			params.Timing = &timing
		}
		if input.TargetKind != "" {
			target, err := targetFromInput(input)
			if err != nil {
				return Result{}, err
			}
			params.Target = &target
		}
		if input.ExpiresAt != "" {
			expiresAt, err := optionalTime(input.ExpiresAt)
			if err != nil {
				return Result{}, fmt.Errorf("schedule: expires_at: %w", err)
			}
			params.ExpiresAt = &expiresAt
		}
		if input.DedupeKey != "" {
			params.DedupeKey = &input.DedupeKey
		}
		entry, err := call.Schedules.Update(ctx, params)
		return h.entryResult("update", entry, nil, err)
	case "cancel":
		id, err := requireScheduleID(input.ScheduleID)
		if err != nil {
			return Result{}, err
		}
		entry, err := call.Schedules.Cancel(ctx, id)
		return h.entryResult("cancel", entry, nil, err)
	default:
		return Result{}, fmt.Errorf("schedule: unsupported action %q", input.Action)
	}
}

func timingFromInput(input requestInput) (scheduledomain.TimingExpression, error) {
	switch input.Mode {
	case "once":
		runAt, err := requiredTime("run_at", input.RunAt)
		if err != nil {
			return scheduledomain.TimingExpression{}, err
		}
		return scheduledomain.TimingExpression{Mode: scheduledomain.TimingModeOnce, RunAt: &runAt}, nil
	case "interval":
		if input.IntervalSeconds <= 0 {
			return scheduledomain.TimingExpression{}, fmt.Errorf("schedule: interval_seconds must be positive")
		}
		return scheduledomain.TimingExpression{Mode: scheduledomain.TimingModeInterval, Interval: time.Duration(input.IntervalSeconds) * time.Second}, nil
	case "cron":
		return scheduledomain.TimingExpression{Mode: scheduledomain.TimingModeCron, Cron: input.Cron, Timezone: input.Timezone}, nil
	default:
		return scheduledomain.TimingExpression{}, fmt.Errorf("schedule: mode is required")
	}
}

func targetFromInput(input requestInput) (scheduledomain.Target, error) {
	if input.TargetKind != string(scheduledomain.TargetKindTaskCreate) {
		return scheduledomain.Target{}, fmt.Errorf("schedule: target_kind must be %q", scheduledomain.TargetKindTaskCreate)
	}
	return scheduledomain.Target{
		Kind: scheduledomain.TargetKindTaskCreate,
		TaskCreate: scheduledomain.TaskCreatePayload{
			TargetMemberID:     member.ID(input.TargetMemberID),
			Title:              input.TaskTitle,
			Description:        input.TaskDescription,
			AcceptanceCriteria: input.AcceptanceCriteria,
			MissionID:          input.MissionRef,
			KeyResultID:        input.KeyResultRef,
		},
	}, nil
}

func requireScheduleID(raw string) (scheduledomain.EntryID, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("schedule: schedule_id is required")
	}
	return scheduledomain.EntryID(raw), nil
}

func requiredTime(field string, raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, fmt.Errorf("schedule: %s is required", field)
	}
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, fmt.Errorf("schedule: parse %s: %w", field, err)
	}
	return parsed.UTC(), nil
}

func optionalTime(raw string) (*time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return nil, err
	}
	parsed = parsed.UTC()
	return &parsed, nil
}
