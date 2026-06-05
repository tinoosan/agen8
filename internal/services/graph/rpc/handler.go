package rpc

import (
	"context"
	"fmt"
	"strings"

	graphapp "github.com/tinoosan/agen8-mcp-server/internal/services/graph/app"
	"github.com/tinoosan/agen8-mcp-server/internal/services/graph/contextlink"
	"github.com/tinoosan/agen8-mcp-server/internal/services/graph/domain"
)

type Handler struct {
	svc   *graphapp.Service
	links contextlink.Reader
}

func NewHandler(svc *graphapp.Service, links contextlink.Reader) (*Handler, error) {
	if svc == nil {
		return nil, fmt.Errorf("graph service is required")
	}
	if links == nil {
		return nil, fmt.Errorf("contextlink reader is required")
	}
	return &Handler{svc: svc, links: links}, nil
}

func (h *Handler) Node(ctx context.Context, p NodeParams) (domain.GraphNodeDetail, error) {
	node, _, err := h.svc.Node(ctx, p.ProjectID, p.NodeType, p.NodeID, p.Depth)
	return node, err
}

func (h *Handler) Search(ctx context.Context, p SearchParams) ([]domain.GraphNodeSummary, error) {
	nodes, _, err := h.svc.Search(ctx, domain.GraphSearchRequest{
		ProjectID:   p.ProjectID,
		NodeType:    p.NodeType,
		Query:       p.Query,
		Limit:       p.Limit,
		HasEdge:     p.HasEdge,
		MissingEdge: p.MissingEdge,
	})
	return nodes, err
}

func (h *Handler) Link(ctx context.Context, p LinkParams) (domain.GraphEdge, error) {
	edge, _, err := h.svc.Link(ctx, domain.GraphLinkRequest{
		ProjectID:  p.ProjectID,
		Origin:     p.Origin,
		SourceType: p.SourceType,
		SourceID:   p.SourceID,
		TargetType: p.TargetType,
		TargetID:   p.TargetID,
		EdgeType:   p.EdgeType,
		Confidence: p.Confidence,
		Rationale:  p.Rationale,
		CreatedBy:  p.CreatedBy,
	})
	return edge, err
}

func (h *Handler) Unlink(ctx context.Context, p UnlinkParams) (domain.GraphEdge, error) {
	edge, _, err := h.svc.Unlink(ctx, domain.GraphLinkRequest{
		ProjectID:  p.ProjectID,
		EdgeID:     p.EdgeID,
		SourceType: p.SourceType,
		SourceID:   p.SourceID,
		TargetType: p.TargetType,
		TargetID:   p.TargetID,
		EdgeType:   p.EdgeType,
	})
	return edge, err
}

func (h *Handler) LinksBySource(ctx context.Context, p LinksBySourceParams) (LinksBySourceResult, error) {
	source := contextlink.NodeRef{Type: contextlink.NodeType(strings.TrimSpace(p.SourceType)), ID: strings.TrimSpace(p.SourceID)}
	if source.Type == "" {
		return LinksBySourceResult{}, fmt.Errorf("sourceType is required")
	}
	if source.ID == "" {
		return LinksBySourceResult{}, fmt.Errorf("sourceId is required")
	}
	links, err := h.links.FindBySource(ctx, source)
	if err != nil {
		return LinksBySourceResult{}, err
	}
	return LinksBySourceResult{ContextLinks: linksToView(links)}, nil
}

func (h *Handler) LinksByTarget(ctx context.Context, p LinksByTargetParams) (LinksByTargetResult, error) {
	target := contextlink.NodeRef{Type: contextlink.NodeType(strings.TrimSpace(p.TargetType)), ID: strings.TrimSpace(p.TargetID)}
	if target.Type == "" {
		return LinksByTargetResult{}, fmt.Errorf("targetType is required")
	}
	if target.ID == "" {
		return LinksByTargetResult{}, fmt.Errorf("targetId is required")
	}
	links, err := h.links.FindByTarget(ctx, target)
	if err != nil {
		return LinksByTargetResult{}, err
	}
	return LinksByTargetResult{ContextLinks: linksToView(links)}, nil
}

func linksToView(in []contextlink.Link) []LinkView {
	out := make([]LinkView, len(in))
	for i := range in {
		out[i] = LinkView{
			ID:         string(in[i].ID),
			SourceType: string(in[i].Source.Type),
			SourceID:   in[i].Source.ID,
			TargetType: string(in[i].Target.Type),
			TargetID:   in[i].Target.ID,
			EdgeType:   string(in[i].EdgeType),
			Confidence: in[i].Confidence,
			Metadata:   in[i].Metadata,
			CreatedAt:  in[i].CreatedAt,
			CreatedBy:  linkCreatedBy(in[i]),
			Origin:     linkOrigin(in[i]),
			Source:     linkSource(in[i]),
			Manual:     linkIsManual(in[i]),
		}
	}
	return out
}

func linkSource(link contextlink.Link) string {
	return linkOrigin(link)
}

func linkIsManual(link contextlink.Link) bool {
	return linkOrigin(link) == "manual"
}

func linkCreatedBy(link contextlink.Link) string {
	createdBy := strings.TrimSpace(link.CreatedBy)
	if linkOrigin(link) == "reference" && createdBy == "graph_query" {
		switch strings.TrimSpace(string(link.Source.Type)) {
		case domain.NodeTypeTask:
			return "task_service"
		case domain.NodeTypeDecision:
			return "decision_service"
		}
	}
	return createdBy
}

func linkOrigin(link contextlink.Link) string {
	if origin := normalizeLinkOrigin(link.Metadata["origin"]); origin != "" {
		if origin == "manual" && rationaleImpliesReference(link.Metadata["rationale"]) {
			return "reference"
		}
		return origin
	}
	if source := normalizeLinkOrigin(link.Metadata["source"]); source != "" {
		if source == "manual" && rationaleImpliesReference(link.Metadata["rationale"]) {
			return "reference"
		}
		return source
	}
	if rationaleImpliesReference(link.Metadata["rationale"]) {
		return "reference"
	}
	createdBy := strings.TrimSpace(link.CreatedBy)
	if createdBy == "graph_query" || strings.HasPrefix(createdBy, "member:") {
		return "manual"
	}
	if createdBy == "" || createdBy == "system" {
		return "system"
	}
	return "reference"
}

func normalizeLinkOrigin(raw string) string {
	switch strings.TrimSpace(strings.ToLower(raw)) {
	case "manual", "explicit":
		return "manual"
	case "reference", "system":
		return strings.TrimSpace(strings.ToLower(raw))
	default:
		return ""
	}
}

func rationaleImpliesReference(rationale string) bool {
	rationale = strings.ToLower(strings.TrimSpace(rationale))
	return strings.Contains(rationale, "references this work item")
}
