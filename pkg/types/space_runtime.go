package types

import (
	"strings"
	"time"

	"github.com/google/uuid"
	spacedomain "github.com/tinoosan/agen8-mcp-server/internal/services/space/domain"
)

// Space runtime lifecycle states.
const (
	SpaceRuntimeStateActive  = "active"
	SpaceRuntimeStateStopped = "stopped"
)

type SpaceRuntime struct {
	SpaceID spacedomain.SpaceID `json:"spaceId"`

	// State is the binary lifecycle state of the space runtime: "active" or "stopped".
	// Set by the supervisor when starting/stopping space runtimes.
	State string `json:"state,omitempty"`

	Name  string `json:"name,omitempty"`
	Title string `json:"title,omitempty"`

	ActiveModel string `json:"activeModel,omitempty"`
	Mode        string `json:"mode,omitempty"`

	ProjectID ProjectID `json:"projectId,omitempty"`

	ReasoningEffort  string                                 `json:"reasoningEffort,omitempty"`
	ReasoningSummary string                                 `json:"reasoningSummary,omitempty"`
	ReasoningByModel map[string]SpaceRuntimeReasoningConfig `json:"reasoningByModel,omitempty"`

	ApprovalsMode string `json:"approvalsMode,omitempty"`
	PlanMode      string `json:"planMode,omitempty"`

	SupervisedBlockedTools []string `json:"supervisedBlockedTools,omitempty"`

	CreatedAt *time.Time `json:"createdAt,omitempty"`

	// CurrentRunID is supervisor-internal. It tracks which run is currently
	// executing for this space. External callers should not rely on this field.
	CurrentRunID RunID `json:"currentRunId,omitempty"`

	CurrentGoal string `json:"currentGoal,omitempty"`
	Plan        string `json:"plan,omitempty"`
	Summary     string `json:"summary,omitempty"`

	UpdatedAt *time.Time `json:"updatedAt,omitempty"`

	// Runs is supervisor-internal. It tracks the pool of run IDs managed by
	// the supervisor for this space. External callers should not rely on this field.
	Runs []RunID `json:"runs,omitempty"`

	HistoryCursor string `json:"historyCursor,omitempty"`

	TokenUsage

	System bool `json:"system,omitempty"`
}

type SpaceRuntimeReasoningConfig struct {
	Effort  string `json:"effort,omitempty"`
	Summary string `json:"summary,omitempty"`
}

func NewSpaceRuntime(title string) SpaceRuntime {
	now := time.Now()
	return SpaceRuntime{
		SpaceID:   spacedomain.SpaceID("space-" + uuid.NewString()),
		State:     SpaceRuntimeStateActive,
		Name:      title,
		Title:     title,
		CreatedAt: &now,
		Runs:      nil,
	}
}

func (t SpaceRuntime) DisplayName() string {
	if t.Name != "" {
		return t.Name
	}
	if t.Title != "" {
		return t.Title
	}
	return t.CurrentGoal
}

func (t SpaceRuntime) Lifecycle() Lifecycle {
	startedAt := t.CreatedAt
	if startedAt == nil && t.UpdatedAt != nil {
		startedAt = t.UpdatedAt
	}
	return Lifecycle{
		CreatedAt: t.CreatedAt,
		StartedAt: startedAt,
		UpdatedAt: t.UpdatedAt,
	}
}

// DefaultRunIDForSpaceRuntime returns the best available run ID for a space runtime,
// preferring CurrentRunID then falling back to the first non-empty entry in Runs.
func DefaultRunIDForSpaceRuntime(t SpaceRuntime) string {
	runID := strings.TrimSpace(string(t.CurrentRunID))
	if runID != "" {
		return runID
	}
	for _, candidate := range t.Runs {
		runID := strings.TrimSpace(string(candidate))
		if runID != "" {
			return runID
		}
	}
	return ""
}

func (t SpaceRuntime) LifecyclePhase() LifecyclePhase {
	if t.UpdatedAt != nil && !t.UpdatedAt.IsZero() {
		return LifecyclePhaseActive
	}
	if t.CreatedAt != nil && !t.CreatedAt.IsZero() {
		return LifecyclePhasePending
	}
	return LifecyclePhaseUnknown
}
