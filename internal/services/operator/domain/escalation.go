// Package domain defines the Escalation aggregate root and its value objects.
//
// An escalation is a synchronous permission gate: the agent presents a
// situation, makes a recommendation, and the operator decides. The agent acts
// on the decision. The operator is the decider, not the doer.
package domain

import (
	"fmt"
	"strings"
	"time"
)

type EscalationID string

type Status string

const (
	StatusPending  Status = "pending"
	StatusResolved Status = "resolved"
	StatusExpired  Status = "expired"
	StatusCanceled Status = "canceled"
)

// Resolution is the operator's verdict on an escalation. Simplified to a
// binary approve/reject; "redirect", "defer", and "delegate" semantics are
// expressed in the freeform ResolutionNote (e.g. "approve with: use
// approach X" or "reject — ask Bob to handle"). Defer is now expressed by
// not resolving (the escalation simply stays Pending).
type Resolution string

const (
	ResolutionApprove Resolution = "approve"
	ResolutionReject  Resolution = "reject"
)

type Escalation struct {
	ID              EscalationID      `json:"id"`
	ProjectID       string            `json:"projectId"`
	SpaceID         string            `json:"spaceId,omitempty"`
	TaskRef         string            `json:"taskRef,omitempty"`
	KeyResultRef    string            `json:"keyResultRef,omitempty"`
	MissionRef      string            `json:"missionRef,omitempty"`
	Source          Source            `json:"source"`
	MemberID        string            `json:"memberId,omitempty"`
	Category        Category          `json:"category"`
	Urgency         Urgency           `json:"urgency"`
	Title           string            `json:"title"`
	Description     string            `json:"description"`
	Recommendation  string            `json:"recommendation,omitempty"`
	Confidence      float64           `json:"confidence,omitempty"`
	Status          Status            `json:"status"`
	Resolution      Resolution        `json:"resolution,omitempty"`
	ResolutionNote  string            `json:"resolutionNote,omitempty"`
	Deadline        *time.Time        `json:"deadline,omitempty"`
	EscalatedAt     *time.Time        `json:"escalatedAt,omitempty"`
	OriginalUrgency Urgency           `json:"originalUrgency,omitempty"`
	Metadata        map[string]string `json:"metadata,omitempty"`
	CreatedAt       time.Time         `json:"createdAt"`
	ResolvedAt      *time.Time        `json:"resolvedAt,omitempty"`
	ResolvedBy      string            `json:"resolvedBy,omitempty"`
}

func (e *Escalation) Validate() error {
	if strings.TrimSpace(e.Title) == "" {
		return fmt.Errorf("title is required")
	}
	if strings.TrimSpace(e.Description) == "" {
		return fmt.Errorf("description is required")
	}
	if strings.TrimSpace(e.ProjectID) == "" {
		return fmt.Errorf("projectId is required")
	}
	if err := ValidateCategory(e.Category); err != nil {
		return err
	}
	if err := ValidateUrgency(e.Urgency); err != nil {
		return err
	}
	switch strings.TrimSpace(string(e.Source)) {
	case string(SourceMember):
	default:
		return fmt.Errorf("invalid source: %q (must be 'member')", e.Source)
	}
	if e.Confidence < 0 || e.Confidence > 1 {
		return fmt.Errorf("confidence must be in range [0.0, 1.0], got %f", e.Confidence)
	}
	return nil
}

func (e *Escalation) IsBlocking() bool {
	return e.TaskRef != "" && e.Status == StatusPending
}

func (e *Escalation) IsOverdue(now time.Time) bool {
	return e.Deadline != nil && now.After(*e.Deadline) && e.Status == StatusPending
}

func (e *Escalation) CanTransitionTo(target Status) error {
	switch e.Status {
	case StatusResolved:
		return fmt.Errorf("escalation %s is already resolved, cannot transition to %s", e.ID, target)
	case StatusCanceled:
		return fmt.Errorf("escalation %s is already canceled, cannot transition to %s", e.ID, target)
	case StatusPending:
		switch target {
		case StatusResolved, StatusExpired, StatusCanceled:
			return nil
		default:
			return fmt.Errorf("invalid transition from %s to %s for escalation %s", e.Status, target, e.ID)
		}
	case StatusExpired:
		switch target {
		case StatusPending, StatusResolved, StatusCanceled:
			return nil
		default:
			return fmt.Errorf("invalid transition from %s to %s for escalation %s", e.Status, target, e.ID)
		}
	default:
		return fmt.Errorf("unknown status %q for escalation %s", e.Status, e.ID)
	}
}

func UrgencyRank(u Urgency) (int, error) {
	switch u {
	case UrgencyLow:
		return 0, nil
	case UrgencyMedium:
		return 1, nil
	case UrgencyHigh:
		return 2, nil
	case UrgencyCritical:
		return 3, nil
	default:
		return 0, fmt.Errorf("unknown urgency: %q", u)
	}
}

func UrgencyToSeverity(u Urgency) (string, error) {
	switch u {
	case UrgencyLow:
		return "info", nil
	case UrgencyMedium, UrgencyHigh:
		return "warning", nil
	case UrgencyCritical:
		return "critical", nil
	default:
		return "", fmt.Errorf("unknown urgency: %q", u)
	}
}

func EscalateUrgency(u Urgency) (Urgency, error) {
	switch u {
	case UrgencyLow:
		return UrgencyMedium, nil
	case UrgencyMedium:
		return UrgencyHigh, nil
	case UrgencyHigh:
		return UrgencyCritical, nil
	case UrgencyCritical:
		return UrgencyCritical, nil
	default:
		return "", fmt.Errorf("unknown urgency: %q", u)
	}
}

func ValidateResolution(resolution Resolution) error {
	switch resolution {
	case ResolutionApprove, ResolutionReject:
		return nil
	default:
		return fmt.Errorf("invalid resolution: %q (must be 'approve' or 'reject')", resolution)
	}
}

func ValidateStatus(status Status) error {
	switch status {
	case StatusPending, StatusResolved, StatusExpired, StatusCanceled:
		return nil
	default:
		return fmt.Errorf("invalid status: %q", status)
	}
}
