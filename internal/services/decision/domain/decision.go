package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"

	humaninput "github.com/tinoosan/agen8-mcp-server/internal/services/humaninput/domain"
)

// DecisionID is the unique identifier for a decision.
type DecisionID string

// DecisionSource identifies who or what created a decision. Source is
// orthogonal to Kind: it records the actor; Kind comes from which payload
// is set.
type DecisionSource string

// DecisionKind discriminates the two decision shapes.
type DecisionKind string

const (
	// DecisionSourceAgent — an agent deliberately logged via the decision tool.
	DecisionSourceAgent DecisionSource = "agent"
	// DecisionSourceOperator — auto-created when an operator resolves an
	// escalation.
	DecisionSourceOperator DecisionSource = "operator"

	DecisionKindLog     DecisionKind = "log"
	DecisionKindAskUser DecisionKind = "ask_user"
)

// ValidDecisionSources returns all valid decision sources.
func ValidDecisionSources() []DecisionSource {
	return []DecisionSource{DecisionSourceAgent, DecisionSourceOperator}
}

// IsValidDecisionSource returns true if the given source is valid.
func IsValidDecisionSource(s DecisionSource) bool {
	switch s {
	case DecisionSourceAgent, DecisionSourceOperator:
		return true
	default:
		return false
	}
}

// Decision is the aggregate root for the decision audit trail.
//
// Exactly one of Log or AskUser is set; that pointer determines the
// decision's kind. The unset pointer is omitted from JSON, so the wire
// format naturally reflects the variant.
type Decision struct {
	ID             DecisionID     `json:"id"`
	ProjectID      string         `json:"projectId"`
	SpaceID        string         `json:"spaceId,omitempty"`
	Source         DecisionSource `json:"source"`
	SourceIdentity string         `json:"sourceIdentity,omitempty"`
	RunID          string         `json:"runId,omitempty"`
	Title          string         `json:"title"`
	Confidence     float64        `json:"confidence,omitempty"`

	// Typed FK fields — primary relationships.
	TaskRef           string `json:"taskRef,omitempty"`
	KeyResultRef      string `json:"keyResultRef,omitempty"`
	MissionRef        string `json:"missionRef,omitempty"`
	PlanRef           string `json:"planRef,omitempty"`
	OperatorActionRef string `json:"operatorActionRef,omitempty"`
	EscalationRef     string `json:"escalationRef,omitempty"`
	CorrelationRef    string `json:"correlationRef,omitempty"`
	InformedByRef     string `json:"informedByRef,omitempty"`

	// Collections.
	Tags     []string          `json:"tags,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`

	CreatedAt time.Time `json:"createdAt"`

	// Exactly one of these must be non-nil. Kind() reads them.
	Log     *LogPayload     `json:"log,omitempty"`
	AskUser *AskUserPayload `json:"askUser,omitempty"`
}

// LogPayload holds the kind-specific data for a "log" decision: the agent
// (or operator on resolve) records what was decided and why.
type LogPayload struct {
	Rationale              string   `json:"rationale"`
	Context                string   `json:"context,omitempty"`
	AlternativesRejected   string   `json:"alternativesRejected,omitempty"`
	InvalidationConditions []string `json:"invalidationConditions,omitempty"`
	Outcome                string   `json:"outcome,omitempty"`
}

// AskUserPayload holds the kind-specific data for an "ask_user" decision:
// a question posed by an agent, awaiting human answers.
type AskUserPayload struct {
	Context   string                `json:"context,omitempty"`
	Questions []humaninput.Question `json:"questions,omitempty"`
	Answers   []humaninput.Answer   `json:"answers,omitempty"`
	Cancelled bool                  `json:"cancelled,omitempty"`
}

// Kind returns the decision's kind based on which payload pointer is set.
// Returns the empty kind if neither is set; Validate guards against that.
func (d *Decision) Kind() DecisionKind {
	switch {
	case d.Log != nil:
		return DecisionKindLog
	case d.AskUser != nil:
		return DecisionKindAskUser
	}
	return ""
}

// Validate checks all required fields and the variant invariants:
// exactly one payload must be set, and that payload's required fields
// must be present.
func (d *Decision) Validate() error {
	if strings.TrimSpace(d.Title) == "" {
		return errors.New("title is required")
	}
	if strings.TrimSpace(d.ProjectID) == "" {
		return errors.New("projectId is required")
	}
	if !IsValidDecisionSource(d.Source) {
		return fmt.Errorf("invalid source %q (must be agent or operator)", d.Source)
	}
	if d.Confidence < 0 || d.Confidence > 1 {
		return fmt.Errorf("confidence must be in [0.0, 1.0], got %v", d.Confidence)
	}

	switch {
	case d.Log != nil && d.AskUser != nil:
		return errors.New("decision: exactly one payload may be set, got both Log and AskUser")
	case d.Log != nil:
		if strings.TrimSpace(d.Log.Rationale) == "" {
			return errors.New("log decision: rationale is required")
		}
	case d.AskUser != nil:
		if len(d.AskUser.Questions) == 0 {
			return errors.New("ask_user decision: questions are required")
		}
	default:
		return errors.New("decision: a payload (Log or AskUser) is required")
	}
	return nil
}
