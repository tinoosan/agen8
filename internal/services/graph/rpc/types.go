package rpc

import "time"

type LinkView struct {
	ID         string            `json:"id"`
	SourceType string            `json:"sourceType"`
	SourceID   string            `json:"sourceId"`
	TargetType string            `json:"targetType"`
	TargetID   string            `json:"targetId"`
	EdgeType   string            `json:"edgeType"`
	Confidence float64           `json:"confidence"`
	Metadata   map[string]string `json:"metadata,omitempty"`
	CreatedAt  time.Time         `json:"createdAt"`
	CreatedBy  string            `json:"createdBy,omitempty"`
	Origin     string            `json:"origin,omitempty"`
	Source     string            `json:"source,omitempty"`
	Manual     bool              `json:"manual"`
}

type LinksBySourceParams struct {
	SourceType string `json:"sourceType"`
	SourceID   string `json:"sourceId"`
}

type LinksBySourceResult struct {
	ContextLinks []LinkView `json:"contextLinks"`
}

type LinksByTargetParams struct {
	TargetType string `json:"targetType"`
	TargetID   string `json:"targetId"`
}

type LinksByTargetResult struct {
	ContextLinks []LinkView `json:"contextLinks"`
}

type NodeParams struct {
	ProjectID string `json:"projectId"`
	NodeType  string `json:"nodeType"`
	NodeID    string `json:"nodeId"`
	Depth     int    `json:"depth"`
}

type SearchParams struct {
	ProjectID   string `json:"projectId"`
	NodeType    string `json:"nodeType"`
	Query       string `json:"query"`
	Limit       int    `json:"limit"`
	HasEdge     string `json:"hasEdge"`
	MissingEdge string `json:"missingEdge"`
}

type LinkParams struct {
	ProjectID  string   `json:"projectId"`
	Origin     string   `json:"origin,omitempty"`
	EdgeID     string   `json:"edgeId,omitempty"`
	SourceType string   `json:"sourceType"`
	SourceID   string   `json:"sourceId"`
	TargetType string   `json:"targetType"`
	TargetID   string   `json:"targetId"`
	EdgeType   string   `json:"edgeType"`
	Confidence *float64 `json:"confidence,omitempty"`
	Rationale  string   `json:"rationale"`
	CreatedBy  string   `json:"createdBy,omitempty"`
}

type UnlinkParams LinkParams
