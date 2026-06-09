package domain

import "context"

const (
	NodeTypeTask      = "task"
	NodeTypeDecision  = "decision"
	NodeTypeKeyResult = "key_result"
	NodeTypeMission   = "mission"
	NodeTypePlan      = "plan"
	NodeTypeAll       = "all"
)

const (
	EdgeTypeBlockedBy   = "blocked_by"
	EdgeTypeResolvedBy  = "resolved_by"
	EdgeTypeCompletedBy = "completed_by"
	EdgeTypeServes      = "serves"
	EdgeTypeInformedBy  = "informed_by"
	EdgeTypeProduced    = "produced"
	EdgeTypeMadeDuring  = "made_during"
	EdgeTypeSpawned     = "spawned"
	EdgeTypeChildOf     = "child_of"
	EdgeTypeRelatesTo   = "relates_to"
	EdgeTypeSupersedes  = "supersedes"
)

type GraphNodeCore struct {
	ID        string
	Type      string
	Title     string
	Status    string
	ScopeID   string
	CreatedAt string
	Fields    map[string]any
}

type GraphNodeSummary struct {
	ID           string         `json:"id"`
	Type         string         `json:"type"`
	Title        string         `json:"title"`
	Status       string         `json:"status"`
	ScopeID      string         `json:"scopeId"`
	CreatedAt    string         `json:"createdAt"`
	Fields       map[string]any `json:"fields,omitempty"`
	MatchedEdges []GraphEdge    `json:"matchedEdges,omitempty"`
}

type GraphNodeDetail struct {
	ID                     string             `json:"id"`
	Type                   string             `json:"type"`
	Title                  string             `json:"title"`
	Status                 string             `json:"status"`
	ScopeID                string             `json:"scopeId"`
	CreatedAt              string             `json:"createdAt"`
	Fields                 map[string]any     `json:"fields"`
	Neighbours             []GraphNodeSummary `json:"neighbours"`
	Subgraph               []GraphNodeSummary `json:"subgraph,omitempty"`
	Edges                  []GraphEdge        `json:"edges"`
	MissingNeighbourCount  int                `json:"missingNeighbourCount"`
	DanglingNeighbourCount int                `json:"danglingNeighbourCount"`
}

type GraphEdge struct {
	ID         string  `json:"id,omitempty"`
	SourceType string  `json:"sourceType"`
	SourceID   string  `json:"sourceID"`
	TargetType string  `json:"targetType"`
	TargetID   string  `json:"targetID"`
	EdgeType   string  `json:"edgeType"`
	Confidence float64 `json:"confidence"`
	Rationale  string  `json:"rationale,omitempty"`
	CreatedAt  string  `json:"createdAt,omitempty"`
	CreatedBy  string  `json:"createdBy,omitempty"`
	Origin     string  `json:"origin,omitempty"`
	Source     string  `json:"source,omitempty"`
	Manual     bool    `json:"manual"`
	Operation  string  `json:"operation,omitempty"`
}

type GraphWarning struct {
	Code          string   `json:"code"`
	Message       string   `json:"message"`
	AffectedType  string   `json:"affectedType"`
	AffectedIDs   []string `json:"affectedIDs"`
	AffectedCount int      `json:"affectedCount"`
}

type GraphLinkRequest struct {
	ProjectID  string
	EdgeID     string
	SourceType string
	SourceID   string
	TargetType string
	TargetID   string
	EdgeType   string
	Confidence *float64
	Rationale  string
	Origin     string
	CreatedBy  string
}

type GraphSearchRequest struct {
	ProjectID    string
	NodeType     string
	Query        string
	Limit        int
	HasEdge      string
	MissingEdge  string
	OutgoingEdge string
	IncomingEdge string
}

type NodeHydrator interface {
	NodeType() string
	Fetch(ctx context.Context, projectID, nodeID string) (GraphNodeCore, error)
	FetchMany(ctx context.Context, projectID string, nodeIDs []string) ([]GraphNodeSummary, error)
	Search(ctx context.Context, projectID, query string, limit int) ([]GraphNodeSummary, error)
}
