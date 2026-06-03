package protocol

import (
	"time"

	humaninput "github.com/tinoosan/agen8-mcp-server/internal/services/humaninput/domain"
)

// Decision RPC method names.
const (
	MethodDecisionCreate = "decision.create"
	MethodDecisionGet    = "decision.get"
	MethodDecisionList   = "decision.list"
	MethodDecisionCount  = "decision.count"
	MethodDecisionExport = "decision.export"
	MethodDecisionDelete = "decision.delete"
)

// DecisionView is the wire-format read model for a decision.
//
// MemberID is the asker's stable identity. MemberName is the resolved
// display name at read time — UI surfaces should prefer MemberName so
// the raw id never reaches a user-facing card or list. SourceIdentity
// is preserved for back-compat readers (some older clients still
// project on it) and mirrors MemberID.
type DecisionView struct {
	ID                     string                `json:"id"`
	ProjectID              string                `json:"projectId"`
	SpaceID                string                `json:"spaceId,omitempty"`
	Source                 string                `json:"source"`
	Kind                   string                `json:"kind"`
	MemberID               string                `json:"memberId,omitempty"`
	MemberName             string                `json:"memberName,omitempty"`
	SourceIdentity         string                `json:"sourceIdentity,omitempty"`
	RunID                  string                `json:"runId,omitempty"`
	Title                  string                `json:"title"`
	Rationale              string                `json:"rationale"`
	Context                string                `json:"context,omitempty"`
	Questions              []humaninput.Question `json:"questions,omitempty"`
	Answers                []humaninput.Answer   `json:"answers,omitempty"`
	Cancelled              bool                  `json:"cancelled,omitempty"`
	AlternativesRejected   string                `json:"alternativesRejected,omitempty"`
	InvalidationConditions []string              `json:"invalidationConditions,omitempty"`
	Confidence             float64               `json:"confidence"`
	Outcome                string                `json:"outcome,omitempty"`
	TaskRef                string                `json:"taskRef,omitempty"`
	KeyResultRef           string                `json:"keyResultRef,omitempty"`
	MissionRef             string                `json:"missionRef,omitempty"`
	PlanRef                string                `json:"planRef,omitempty"`
	OperatorActionRef      string                `json:"operatorActionRef,omitempty"`
	EscalationRef          string                `json:"escalationRef,omitempty"`
	CorrelationRef         string                `json:"correlationRef,omitempty"`
	InformedByRef          string                `json:"informedByRef,omitempty"`
	Tags                   []string              `json:"tags,omitempty"`
	Metadata               map[string]string     `json:"metadata,omitempty"`
	CreatedAt              time.Time             `json:"createdAt"`
}

// -- decision.create --

// DecisionCreateParams are the parameters for the decision.create RPC call.
type DecisionCreateParams struct {
	ProjectID              string                `json:"projectId"`
	SpaceID                string                `json:"spaceId,omitempty"`
	Source                 string                `json:"source"`
	Kind                   string                `json:"kind,omitempty"`
	SourceIdentity         string                `json:"sourceIdentity,omitempty"`
	RunID                  string                `json:"runId,omitempty"`
	Title                  string                `json:"title"`
	Rationale              string                `json:"rationale"`
	Context                string                `json:"context,omitempty"`
	Questions              []humaninput.Question `json:"questions,omitempty"`
	Answers                []humaninput.Answer   `json:"answers,omitempty"`
	Cancelled              bool                  `json:"cancelled,omitempty"`
	AlternativesRejected   string                `json:"alternativesRejected,omitempty"`
	InvalidationConditions []string              `json:"invalidationConditions,omitempty"`
	Confidence             float64               `json:"confidence"`
	Outcome                string                `json:"outcome,omitempty"`
	TaskRef                string                `json:"taskRef,omitempty"`
	KeyResultRef           string                `json:"keyResultRef,omitempty"`
	MissionRef             string                `json:"missionRef,omitempty"`
	PlanRef                string                `json:"planRef,omitempty"`
	OperatorActionRef      string                `json:"operatorActionRef,omitempty"`
	EscalationRef          string                `json:"escalationRef,omitempty"`
	CorrelationRef         string                `json:"correlationRef,omitempty"`
	InformedByRef          string                `json:"informedByRef,omitempty"`
	Tags                   []string              `json:"tags,omitempty"`
	Metadata               map[string]string     `json:"metadata,omitempty"`
}

// DecisionCreateResult is the result of the decision.create RPC call.
type DecisionCreateResult struct {
	Decision DecisionView `json:"decision"`
}

// -- decision.get --

// DecisionGetParams are the parameters for the decision.get RPC call.
type DecisionGetParams struct {
	DecisionID string `json:"decisionId"`
}

// DecisionGetResult is the result of the decision.get RPC call.
type DecisionGetResult struct {
	Decision DecisionView `json:"decision"`
}

// -- decision.list --

// DecisionListParams are the parameters for the decision.list RPC call.
type DecisionListParams struct {
	ProjectID string   `json:"projectId"`
	Source    string   `json:"source,omitempty"`
	SpaceID   string   `json:"spaceId,omitempty"`
	Tags      []string `json:"tags,omitempty"`
	Query     string   `json:"query,omitempty"`
	Since     string   `json:"since,omitempty"` // ISO 8601 datetime
	Until     string   `json:"until,omitempty"` // ISO 8601 datetime
	Sort      string   `json:"sort,omitempty"`  // newest | oldest
	Limit     int      `json:"limit,omitempty"`
	Offset    int      `json:"offset,omitempty"`
}

// DecisionListResult is the result of the decision.list RPC call.
type DecisionListResult struct {
	Decisions []DecisionView `json:"decisions"`
}

// -- decision.count --

type DecisionCountParams struct {
	ProjectID string   `json:"projectId"`
	Source    string   `json:"source,omitempty"`
	SpaceID   string   `json:"spaceId,omitempty"`
	Tags      []string `json:"tags,omitempty"`
	Query     string   `json:"query,omitempty"`
	Since     string   `json:"since,omitempty"`
	Until     string   `json:"until,omitempty"`
}

type DecisionCountResult struct {
	Count int `json:"count"`
}

// -- decision.export --

type DecisionExportParams struct {
	ProjectID string   `json:"projectId"`
	Source    string   `json:"source,omitempty"`
	SpaceID   string   `json:"spaceId,omitempty"`
	Tags      []string `json:"tags,omitempty"`
	Query     string   `json:"query,omitempty"`
	Since     string   `json:"since,omitempty"`
	Until     string   `json:"until,omitempty"`
	Sort      string   `json:"sort,omitempty"`
}

type DecisionExportResult struct {
	Decisions []DecisionView `json:"decisions"`
}

// -- decision.delete --

type DecisionDeleteParams struct {
	DecisionID string `json:"decisionId"`
}

type DecisionDeleteResult struct {
	Deleted bool `json:"deleted"`
}
