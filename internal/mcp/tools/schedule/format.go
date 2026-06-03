package schedule

import (
	"fmt"
	"time"

	scheduledomain "github.com/tinoosan/agen8-mcp-server/internal/services/schedule/domain"
)

type entryView struct {
	ID          string     `json:"id"`
	SpaceID     string     `json:"spaceId"`
	CreatedBy   string     `json:"createdBy"`
	Status      string     `json:"status"`
	Title       string     `json:"title"`
	Description string     `json:"description,omitempty"`
	Timing      any        `json:"timing"`
	Target      any        `json:"target"`
	NextRunAt   *time.Time `json:"nextRunAt,omitempty"`
	ExpiresAt   *time.Time `json:"expiresAt,omitempty"`
	DedupeKey   string     `json:"dedupeKey,omitempty"`
	Runs        []runView  `json:"runs,omitempty"`
}

type runView struct {
	ID             string     `json:"id"`
	DueAt          time.Time  `json:"dueAt"`
	StartedAt      time.Time  `json:"startedAt"`
	FinishedAt     *time.Time `json:"finishedAt,omitempty"`
	Status         string     `json:"status"`
	TargetKind     string     `json:"targetKind"`
	TargetObjectID string     `json:"targetObjectId,omitempty"`
	Error          string     `json:"error,omitempty"`
}

func (h Handler) entryResult(action string, entry scheduledomain.Entry, runs []scheduledomain.Run, err error) (Result, error) {
	if err != nil {
		return Result{}, err
	}
	view := toEntryView(entry, runs)
	return Result{
		Text: fmt.Sprintf("schedule %s: %s (%s)", action, view.ID, view.Status),
		Structured: map[string]any{
			"tool":     Name,
			"action":   action,
			"schedule": view,
		},
	}, nil
}

func (h Handler) listResult(entries []scheduledomain.Entry, err error) (Result, error) {
	if err != nil {
		return Result{}, err
	}
	rows := make([]entryView, 0, len(entries))
	for _, entry := range entries {
		rows = append(rows, toEntryView(entry, nil))
	}
	return Result{
		Text: fmt.Sprintf("schedule list: %d entries", len(rows)),
		Structured: map[string]any{
			"tool":      Name,
			"action":    "list",
			"schedules": rows,
			"count":     len(rows),
		},
	}, nil
}

func toEntryView(entry scheduledomain.Entry, runs []scheduledomain.Run) entryView {
	viewRuns := make([]runView, 0, len(runs))
	for _, run := range runs {
		viewRuns = append(viewRuns, runView{
			ID:             string(run.ID),
			DueAt:          run.DueAt,
			StartedAt:      run.StartedAt,
			FinishedAt:     run.FinishedAt,
			Status:         string(run.Status),
			TargetKind:     string(run.TargetKind),
			TargetObjectID: run.TargetObjectID,
			Error:          run.Error,
		})
	}
	return entryView{
		ID:          string(entry.ID),
		SpaceID:     string(entry.SpaceID),
		CreatedBy:   string(entry.CreatedBy),
		Status:      string(entry.Status),
		Title:       entry.Title,
		Description: entry.Description,
		Timing:      entry.Timing,
		Target:      entry.Target,
		NextRunAt:   entry.NextRunAt,
		ExpiresAt:   entry.ExpiresAt,
		DedupeKey:   entry.DedupeKey,
		Runs:        viewRuns,
	}
}
