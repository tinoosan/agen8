package run

import (
	"fmt"
	"strings"
	"time"
)

type Status string

const (
	StatusRunning       Status = "running"
	StatusStopRequested Status = "stop_requested"
	StatusCanceled      Status = "canceled"
	StatusCompleted     Status = "completed"
	StatusFailed        Status = "failed"
)

type Run struct {
	ID               string
	ProjectID        string
	SpaceID          string
	ChannelID        string
	MemberID         string
	SessionID        string
	HarnessKind      string
	NativeSessionRef string
	TurnID           string
	NativeTurnID     string
	Status           Status
	StopRequestedBy  string
	StopRequestedAt  *time.Time
	StartedAt        time.Time
	CompletedAt      *time.Time
	Error            string
}

type StartParams struct {
	ID               string
	ProjectID        string
	SpaceID          string
	ChannelID        string
	MemberID         string
	SessionID        string
	HarnessKind      string
	NativeSessionRef string
	TurnID           string
	StartedAt        time.Time
}

func Start(params StartParams) (Run, error) {
	r := Run{
		ID:               strings.TrimSpace(params.ID),
		ProjectID:        strings.TrimSpace(params.ProjectID),
		SpaceID:          strings.TrimSpace(params.SpaceID),
		ChannelID:        strings.TrimSpace(params.ChannelID),
		MemberID:         strings.TrimSpace(params.MemberID),
		SessionID:        strings.TrimSpace(params.SessionID),
		HarnessKind:      strings.TrimSpace(params.HarnessKind),
		NativeSessionRef: strings.TrimSpace(params.NativeSessionRef),
		TurnID:           strings.TrimSpace(params.TurnID),
		Status:           StatusRunning,
		StartedAt:        params.StartedAt.UTC(),
	}
	if err := r.Validate(); err != nil {
		return Run{}, err
	}
	return r, nil
}

func (r Run) Validate() error {
	if strings.TrimSpace(r.ID) == "" {
		return fmt.Errorf("run id is required")
	}
	if strings.TrimSpace(r.ProjectID) == "" {
		return fmt.Errorf("project id is required")
	}
	if strings.TrimSpace(r.SpaceID) == "" {
		return fmt.Errorf("space id is required")
	}
	if strings.TrimSpace(r.ChannelID) == "" {
		return fmt.Errorf("channel id is required")
	}
	if strings.TrimSpace(r.MemberID) == "" {
		return fmt.Errorf("member id is required")
	}
	if strings.TrimSpace(r.SessionID) == "" {
		return fmt.Errorf("session id is required")
	}
	if strings.TrimSpace(r.HarnessKind) == "" {
		return fmt.Errorf("harness kind is required")
	}
	if strings.TrimSpace(r.TurnID) == "" {
		return fmt.Errorf("turn id is required")
	}
	if r.StartedAt.IsZero() {
		return fmt.Errorf("started at is required")
	}
	if !ValidStatus(r.Status) {
		return fmt.Errorf("invalid run status %q", r.Status)
	}
	if isTerminal(r.Status) && r.CompletedAt == nil {
		return fmt.Errorf("completed at is required for terminal run")
	}
	if !isTerminal(r.Status) && r.CompletedAt != nil {
		return fmt.Errorf("completed at must be empty for non-terminal run")
	}
	if r.Status == StatusFailed && strings.TrimSpace(r.Error) == "" {
		return fmt.Errorf("error is required for failed run")
	}
	if r.Status != StatusFailed && strings.TrimSpace(r.Error) != "" {
		return fmt.Errorf("error is only valid for failed run")
	}
	if r.Status == StatusStopRequested && r.StopRequestedAt == nil {
		return fmt.Errorf("stop requested at is required")
	}
	return nil
}

func ValidStatus(status Status) bool {
	switch status {
	case StatusRunning, StatusStopRequested, StatusCanceled, StatusCompleted, StatusFailed:
		return true
	default:
		return false
	}
}

func (r Run) IsTerminal() bool {
	return isTerminal(r.Status)
}

func isTerminal(status Status) bool {
	switch status {
	case StatusCanceled, StatusCompleted, StatusFailed:
		return true
	default:
		return false
	}
}

func (r *Run) RequestStop(requestedBy string, at time.Time) error {
	if r == nil {
		return fmt.Errorf("run is nil")
	}
	if r.Status == StatusStopRequested {
		return nil
	}
	if r.Status != StatusRunning {
		return fmt.Errorf("cannot request stop for run %q: status is %q", r.ID, r.Status)
	}
	now := at.UTC()
	r.Status = StatusStopRequested
	r.StopRequestedBy = strings.TrimSpace(requestedBy)
	r.StopRequestedAt = &now
	return r.Validate()
}

func (r *Run) SetNativeTurnID(nativeTurnID string) error {
	if r == nil {
		return fmt.Errorf("run is nil")
	}
	if r.IsTerminal() {
		return nil
	}
	r.NativeTurnID = strings.TrimSpace(nativeTurnID)
	return r.Validate()
}

func (r *Run) SetNativeSessionRef(nativeSessionRef string) error {
	if r == nil {
		return fmt.Errorf("run is nil")
	}
	if r.IsTerminal() {
		return nil
	}
	r.NativeSessionRef = strings.TrimSpace(nativeSessionRef)
	return r.Validate()
}

func (r *Run) MarkCanceled(at time.Time) error {
	return r.markTerminal(StatusCanceled, "", at)
}

func (r *Run) MarkCompleted(at time.Time) error {
	return r.markTerminal(StatusCompleted, "", at)
}

func (r *Run) MarkFailed(errText string, at time.Time) error {
	errText = strings.TrimSpace(errText)
	if errText == "" {
		return fmt.Errorf("error is required")
	}
	return r.markTerminal(StatusFailed, errText, at)
}

func (r *Run) markTerminal(status Status, errText string, at time.Time) error {
	if r == nil {
		return fmt.Errorf("run is nil")
	}
	if r.IsTerminal() {
		return nil
	}
	if r.Status != StatusRunning && r.Status != StatusStopRequested {
		return fmt.Errorf("cannot mark run %q terminal from status %q", r.ID, r.Status)
	}
	completedAt := at.UTC()
	r.Status = status
	r.CompletedAt = &completedAt
	r.Error = strings.TrimSpace(errText)
	return r.Validate()
}
