package domain

import (
	"fmt"
	"strings"
	"time"

	spacedomain "github.com/tinoosan/agen8-mcp-server/internal/services/space/domain"
)

type RunStatus string

const (
	RunStatusStarted   RunStatus = "started"
	RunStatusSucceeded RunStatus = "succeeded"
	RunStatusFailed    RunStatus = "failed"
	RunStatusSkipped   RunStatus = "skipped"
)

type Run struct {
	ID             RunID               `json:"id"`
	EntryID        EntryID             `json:"entryId"`
	SpaceID        spacedomain.SpaceID `json:"spaceId"`
	DueAt          time.Time           `json:"dueAt"`
	StartedAt      time.Time           `json:"startedAt"`
	FinishedAt     *time.Time          `json:"finishedAt,omitempty"`
	Status         RunStatus           `json:"status"`
	TargetKind     TargetKind          `json:"targetKind"`
	TargetObjectID string              `json:"targetObjectId,omitempty"`
	Error          string              `json:"error,omitempty"`
}

func NewStartedRun(entry Entry, dueAt, now time.Time) (Run, error) {
	id, err := NewRunID()
	if err != nil {
		return Run{}, err
	}
	run := Run{
		ID:         id,
		EntryID:    entry.ID,
		SpaceID:    entry.SpaceID,
		DueAt:      dueAt.UTC(),
		StartedAt:  now.UTC(),
		Status:     RunStatusStarted,
		TargetKind: entry.Target.Kind,
	}
	if err := run.Validate(); err != nil {
		return Run{}, err
	}
	return run, nil
}

func (r Run) Validate() error {
	if strings.TrimSpace(string(r.ID)) == "" {
		return fmt.Errorf("id is required")
	}
	if strings.TrimSpace(string(r.EntryID)) == "" {
		return fmt.Errorf("entryId is required")
	}
	if strings.TrimSpace(string(r.SpaceID)) == "" {
		return fmt.Errorf("spaceId is required")
	}
	if r.DueAt.IsZero() {
		return fmt.Errorf("dueAt is required")
	}
	if r.StartedAt.IsZero() {
		return fmt.Errorf("startedAt is required")
	}
	if err := validateRunStatus(r.Status); err != nil {
		return err
	}
	if r.TargetKind == "" {
		return fmt.Errorf("targetKind is required")
	}
	return nil
}

func (r Run) Succeed(targetObjectID string, now time.Time) (Run, error) {
	if r.Status != RunStatusStarted {
		return Run{}, fmt.Errorf("cannot succeed schedule run with status %q", r.Status)
	}
	targetObjectID = strings.TrimSpace(targetObjectID)
	if targetObjectID == "" {
		return Run{}, fmt.Errorf("targetObjectId is required")
	}
	next := r
	finished := now.UTC()
	next.Status = RunStatusSucceeded
	next.TargetObjectID = targetObjectID
	next.Error = ""
	next.FinishedAt = &finished
	return next, nil
}

func (r Run) Fail(message string, now time.Time) (Run, error) {
	if r.Status != RunStatusStarted {
		return Run{}, fmt.Errorf("cannot fail schedule run with status %q", r.Status)
	}
	message = strings.TrimSpace(message)
	if message == "" {
		return Run{}, fmt.Errorf("error message is required")
	}
	next := r
	finished := now.UTC()
	next.Status = RunStatusFailed
	next.Error = message
	next.FinishedAt = &finished
	return next, nil
}

func (r Run) Skip(reason string, now time.Time) (Run, error) {
	if r.Status != RunStatusStarted {
		return Run{}, fmt.Errorf("cannot skip schedule run with status %q", r.Status)
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return Run{}, fmt.Errorf("skip reason is required")
	}
	next := r
	finished := now.UTC()
	next.Status = RunStatusSkipped
	next.Error = reason
	next.FinishedAt = &finished
	return next, nil
}

func validateRunStatus(status RunStatus) error {
	switch status {
	case RunStatusStarted, RunStatusSucceeded, RunStatusFailed, RunStatusSkipped:
		return nil
	default:
		return fmt.Errorf("invalid schedule run status %q", status)
	}
}
