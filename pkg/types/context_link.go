package types

import (
	"fmt"
	"time"
)

// ContextLinkID is the unique identifier for a context link.
type ContextLinkID string

// ── Edge type constants ──────────────────────────────
// Only these edge types are allowed. Unknown edge types must be rejected.
// See PRD decision D14 and D34 for the rationale.
const (
	// EdgeTypeBlockedBy indicates the source entity is blocked by the target.
	// Example: Task →[blocked_by]→ Escalation, Task →[blocked_by]→ OperatorAction
	EdgeTypeBlockedBy = "blocked_by"

	// EdgeTypeResolvedBy indicates the source entity was resolved by the target.
	// Example: Escalation →[resolved_by]→ Decision
	EdgeTypeResolvedBy = "resolved_by"

	// EdgeTypeCompletedBy indicates the source entity was completed by the target.
	// Example: OperatorAction →[completed_by]→ Decision
	EdgeTypeCompletedBy = "completed_by"

	// EdgeTypeServes indicates the source entity serves/contributes to the target.
	// Example: Task →[serves]→ KeyResult, Decision →[serves]→ Mission
	EdgeTypeServes = "serves"

	// EdgeTypeInformedBy indicates the source entity was informed by the target.
	// Example: Decision →[informed_by]→ Event, Decision →[informed_by]→ Decision
	EdgeTypeInformedBy = "informed_by"

	// EdgeTypeProduced indicates the source entity produced the target.
	// Example: Decision →[produced]→ Task
	EdgeTypeProduced = "produced"

	// EdgeTypeMadeDuring indicates the source entity was created during the target.
	// Example: Decision →[made_during]→ Task, Decision →[made_during]→ Correlation
	EdgeTypeMadeDuring = "made_during"

	// EdgeTypeSpawned indicates the source entity spawned the target.
	// Example: Escalation →[spawned]→ OperatorAction
	EdgeTypeSpawned = "spawned"

	// EdgeTypeChildOf indicates the source entity is a child of the target.
	// Used for parent-child task ancestry, letting agents walk up the task
	// tree via context links without needing a separate tree-walk API.
	// Example: Task(child) →[child_of]→ Task(parent)
	EdgeTypeChildOf = "child_of"

	// EdgeTypeRelatesTo is a generic bidirectional relationship for cross-cutting
	// edges identified by coordinators via the context_link tool.
	// Example: Any →[relates_to]→ Any
	EdgeTypeRelatesTo = "relates_to"
)

// validEdgeTypes is the set of all valid edge types. Used for validation.
var validEdgeTypes = map[string]bool{
	EdgeTypeBlockedBy:   true,
	EdgeTypeResolvedBy:  true,
	EdgeTypeCompletedBy: true,
	EdgeTypeServes:      true,
	EdgeTypeInformedBy:  true,
	EdgeTypeProduced:    true,
	EdgeTypeMadeDuring:  true,
	EdgeTypeSpawned:     true,
	EdgeTypeChildOf:     true,
	EdgeTypeRelatesTo:   true,
}

// ValidEdgeType reports whether the given edge type string is one of the
// known edge types. Unknown edge types must be rejected — no freeform strings.
func ValidEdgeType(et string) bool {
	return validEdgeTypes[et]
}

// ContextLink represents a typed edge in the knowledge graph connecting
// two entities. Used for cross-domain relationships (e.g., decision serves
// key result, operator action blocks task).
type ContextLink struct {
	ID         ContextLinkID     `json:"id"`
	SourceType string            `json:"sourceType"` // e.g. "task", "decision", "operator_action"
	SourceID   string            `json:"sourceId"`
	TargetType string            `json:"targetType"` // e.g. "key_result", "mission", "task"
	TargetID   string            `json:"targetId"`
	EdgeType   string            `json:"edgeType"`   // Must be one of the EdgeType* constants
	Confidence float64           `json:"confidence"` // 0.0-1.0 confidence weight
	Metadata   map[string]string `json:"metadata,omitempty"`
	CreatedAt  time.Time         `json:"createdAt"`
	CreatedBy  string            `json:"createdBy,omitempty"` // "member:<label>" or "operator" or "system"
}

// Validate checks that required fields are non-empty, that EdgeType is from
// the known set, and that Confidence is within the [0.0, 1.0] range. It
// returns an error describing the first problem found, or nil if the link
// is valid.
func (cl *ContextLink) Validate() error {
	if cl.SourceType == "" {
		return fmt.Errorf("context link: sourceType is required")
	}
	if cl.SourceID == "" {
		return fmt.Errorf("context link: sourceId is required")
	}
	if cl.TargetType == "" {
		return fmt.Errorf("context link: targetType is required")
	}
	if cl.TargetID == "" {
		return fmt.Errorf("context link: targetId is required")
	}
	if cl.EdgeType == "" {
		return fmt.Errorf("context link: edgeType is required")
	}
	if !ValidEdgeType(cl.EdgeType) {
		return fmt.Errorf("context link: unknown edgeType %q", cl.EdgeType)
	}
	if cl.Confidence < 0 || cl.Confidence > 1 {
		return fmt.Errorf("context link: confidence must be in range [0.0, 1.0], got %f", cl.Confidence)
	}
	return nil
}
