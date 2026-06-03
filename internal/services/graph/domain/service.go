package domain

import "context"

type GraphQueryService interface {
	Node(ctx context.Context, projectID, nodeType, nodeID string, depth int) (GraphNodeDetail, []GraphWarning, error)
	Search(ctx context.Context, req GraphSearchRequest) ([]GraphNodeSummary, []GraphWarning, error)
	Link(ctx context.Context, req GraphLinkRequest) (GraphEdge, []GraphWarning, error)
	Unlink(ctx context.Context, req GraphLinkRequest) (GraphEdge, []GraphWarning, error)
}
