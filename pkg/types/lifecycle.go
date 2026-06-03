package types

import (
	"strings"
	"time"
)

type Lifecycle struct {
	CreatedAt   *time.Time `json:"createdAt,omitempty"`
	StartedAt   *time.Time `json:"startedAt,omitempty"`
	CompletedAt *time.Time `json:"completedAt,omitempty"`
	UpdatedAt   *time.Time `json:"updatedAt,omitempty"`
}

type LifecyclePhase string

const (
	LifecyclePhasePending   LifecyclePhase = "pending"
	LifecyclePhaseActive    LifecyclePhase = "active"
	LifecyclePhaseCompleted LifecyclePhase = "completed"
	LifecyclePhaseFailed    LifecyclePhase = "failed"
	LifecyclePhaseCanceled  LifecyclePhase = "canceled"
	LifecyclePhaseUnknown   LifecyclePhase = "unknown"
)

func LifecyclePhaseForStatus(status string) LifecyclePhase {
	switch strings.TrimSpace(strings.ToLower(status)) {
	case "", "pending":
		return LifecyclePhasePending
	case "running", "active", "in_progress", "in_review", "paused", "ok":
		return LifecyclePhaseActive
	case "completed", "succeeded", "done":
		return LifecyclePhaseCompleted
	case "failed", "error":
		return LifecyclePhaseFailed
	case "canceled":
		return LifecyclePhaseCanceled
	default:
		return LifecyclePhaseUnknown
	}
}
