package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// DecisionID is the unique identifier for a decision.
type DecisionID string

// DecisionSource identifies who or what created a decision. Source is
// orthogonal to Kind: it records the actor; Kind comes from which payload
// is set.
type DecisionSource string

// DecisionKind records the retained decision shape.
type DecisionKind string

const (
	// DecisionSourceAgent — an agent deliberately logged via the decision tool.
	DecisionSourceAgent DecisionSource = "agent"

	DecisionKindLog DecisionKind = "log"
)

// ValidDecisionSources returns all valid decision sources.
func ValidDecisionSources() []DecisionSource {
	return []DecisionSource{DecisionSourceAgent}
}

// IsValidDecisionSource returns true if the given source is valid.
func IsValidDecisionSource(s DecisionSource) bool {
	switch s {
	case DecisionSourceAgent:
		return true
	default:
		return false
	}
}

// Decision is the aggregate root for the decision audit trail.
type Decision struct {
	ID             DecisionID     `json:"id"`
	ProjectID      string         `json:"projectId"`
	Source         DecisionSource `json:"source"`
	SourceIdentity string         `json:"sourceIdentity,omitempty"`
	Title          string         `json:"title"`
	Confidence     float64        `json:"confidence,omitempty"`

	TaskRef        string `json:"taskRef,omitempty"`
	KeyResultRef   string `json:"keyResultRef,omitempty"`
	MissionRef     string `json:"missionRef,omitempty"`
	CorrelationRef string `json:"correlationRef,omitempty"`
	InformedByRef  string `json:"informedByRef,omitempty"`

	// Collections.
	Tags     []string          `json:"tags,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`

	CreatedAt time.Time `json:"createdAt"`

	Log *LogPayload `json:"log,omitempty"`
}

// LogPayload holds the kind-specific data for a "log" decision: the agent
// records what was decided and why.
type LogPayload struct {
	Rationale              string   `json:"rationale"`
	Context                string   `json:"context,omitempty"`
	AlternativesRejected   string   `json:"alternativesRejected,omitempty"`
	InvalidationConditions []string `json:"invalidationConditions,omitempty"`
	Outcome                string   `json:"outcome,omitempty"`
}

// Kind returns the retained decision kind.
func (d *Decision) Kind() DecisionKind {
	return DecisionKindLog
}

// Validate checks all required fields.
func (d *Decision) Validate() error {
	if strings.TrimSpace(d.Title) == "" {
		return errors.New("title is required")
	}
	if strings.TrimSpace(d.ProjectID) == "" {
		return errors.New("projectId is required")
	}
	if !IsValidDecisionSource(d.Source) {
		return fmt.Errorf("invalid source %q (must be agent)", d.Source)
	}
	if d.Confidence < 0 || d.Confidence > 1 {
		return fmt.Errorf("confidence must be in [0.0, 1.0], got %v", d.Confidence)
	}

	if d.Log == nil {
		return errors.New("decision: a log payload is required")
	}
	if strings.TrimSpace(d.Log.Rationale) == "" {
		return errors.New("log decision: rationale is required")
	}
	return nil
}
