package app

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tinoosan/agen8/internal/services/graph/contextlink"
	"github.com/tinoosan/agen8/internal/services/graph/domain"
)

const (
	defaultNeighbourLimit = 50
	maxWarningIDs         = 50
	defaultNodeDepth      = 1
	maxNodeDepth          = 3
	defaultGraphCreator   = "graph_query"
)

type Service struct {
	contextLinks  contextlink.Repository
	hydrators     map[string]domain.NodeHydrator
	searchTimeout time.Duration
	pinReader     PinReader
}

type Linker interface {
	Link(ctx context.Context, req domain.GraphLinkRequest) (domain.GraphEdge, []domain.GraphWarning, error)
}

// PinReader reports which node refs are pinned in a project so pinned nodes can
// be prioritized in search results. It is optional: a nil PinReader (or a
// failing lookup) simply disables the boost — search still returns results.
type PinReader interface {
	PinnedNodeRefs(ctx context.Context, projectID string) (map[string]struct{}, error)
}

type NodeLinkDeleter interface {
	DeleteLinksForNode(ctx context.Context, nodeType string, nodeID string) error
}

func NewService(contextLinks contextlink.Repository, hydrators []domain.NodeHydrator, searchTimeout time.Duration) (*Service, error) {
	if contextLinks == nil {
		return nil, fmt.Errorf("graph: context link repository is required")
	}
	if searchTimeout <= 0 {
		return nil, fmt.Errorf("graph: search timeout must be > 0")
	}
	byType := make(map[string]domain.NodeHydrator, len(hydrators))
	for _, hydrator := range hydrators {
		if hydrator == nil {
			return nil, fmt.Errorf("graph: hydrator is nil")
		}
		nodeType := normalizeNodeType(hydrator.NodeType())
		if nodeType == "" {
			return nil, fmt.Errorf("graph: hydrator node type is required")
		}
		if _, exists := byType[nodeType]; exists {
			return nil, fmt.Errorf("graph: duplicate hydrator for node type %q", nodeType)
		}
		byType[nodeType] = hydrator
	}
	return &Service{
		contextLinks:  contextLinks,
		hydrators:     byType,
		searchTimeout: searchTimeout,
	}, nil
}

// SetPinReader wires the (optional) pin lookup used to prioritize pinned nodes
// in search. Safe to leave unset — search degrades to unboosted ordering.
func (s *Service) SetPinReader(reader PinReader) {
	s.pinReader = reader
}

// pinnedRefs returns the set of pinned node refs for a project. Pins are only a
// ranking boost, so a missing reader or a lookup error degrades silently to an
// empty set rather than failing the search.
func (s *Service) pinnedRefs(ctx context.Context, projectID string) map[string]struct{} {
	if s.pinReader == nil {
		return nil
	}
	refs, err := s.pinReader.PinnedNodeRefs(ctx, projectID)
	if err != nil {
		return nil
	}
	return refs
}

func (s *Service) Node(ctx context.Context, projectID, nodeType, nodeID string, depth int) (domain.GraphNodeDetail, []domain.GraphWarning, error) {
	projectID = strings.TrimSpace(projectID)
	nodeType = normalizeNodeType(nodeType)
	nodeID = strings.TrimSpace(nodeID)
	depth = normalizeNodeDepth(depth)
	if projectID == "" {
		return domain.GraphNodeDetail{}, []domain.GraphWarning{}, fmt.Errorf("graph_query: project id is required")
	}
	if nodeID == "" {
		return domain.GraphNodeDetail{}, []domain.GraphWarning{}, fmt.Errorf("graph_query: node_id is required")
	}
	if nodeType == "" {
		inferred, err := inferNodeTypeFromID(nodeID)
		if err != nil {
			return domain.GraphNodeDetail{}, []domain.GraphWarning{}, err
		}
		nodeType = inferred
	}
	if nodeType == domain.NodeTypeAll {
		return domain.GraphNodeDetail{}, []domain.GraphWarning{}, fmt.Errorf("graph_query: node_type=all is only valid for action=search")
	}
	detail, warnings, err := s.nodeOneHop(ctx, projectID, nodeType, nodeID)
	if err != nil {
		return domain.GraphNodeDetail{}, []domain.GraphWarning{}, err
	}
	if depth <= 1 {
		return detail, warnings, nil
	}
	subgraph, subgraphWarnings, err := s.expandSubgraph(ctx, projectID, detail, depth)
	if err != nil {
		return domain.GraphNodeDetail{}, []domain.GraphWarning{}, err
	}
	detail.Subgraph = subgraph
	warnings = append(warnings, subgraphWarnings...)
	return detail, warningsOrEmpty(warnings), nil
}

func (s *Service) nodeOneHop(ctx context.Context, projectID, nodeType, nodeID string) (domain.GraphNodeDetail, []domain.GraphWarning, error) {
	hydrator, ok := s.hydrators[nodeType]
	if !ok {
		return domain.GraphNodeDetail{}, []domain.GraphWarning{}, fmt.Errorf("graph_query: unsupported node_type %q", nodeType)
	}

	core, err := hydrator.Fetch(ctx, projectID, nodeID)
	if err != nil {
		return domain.GraphNodeDetail{}, []domain.GraphWarning{}, err
	}
	core.Type = normalizeNodeType(core.Type)
	if core.Type == "" {
		core.Type = nodeType
	}
	if core.ID == "" {
		core.ID = nodeID
	}
	createdAt, err := parseRFC3339Strict(core.CreatedAt, fmt.Sprintf("%s/%s", core.Type, core.ID))
	if err != nil {
		return domain.GraphNodeDetail{}, []domain.GraphWarning{}, err
	}
	core.CreatedAt = createdAt.Format(time.RFC3339Nano)
	if core.Fields == nil {
		core.Fields = map[string]any{}
	}

	links, err := contextlink.LinkedEntities(ctx, s.contextLinks, contextlink.NodeRef{Type: contextlink.NodeType(core.Type), ID: core.ID})
	if err != nil {
		return domain.GraphNodeDetail{}, []domain.GraphWarning{}, fmt.Errorf("graph_query: linked entities for %s/%s: %w", core.Type, core.ID, err)
	}
	edges := make([]domain.GraphEdge, 0, len(links))
	for _, link := range links {
		edges = append(edges, graphEdgeFromLink(link, ""))
	}

	neighbourIDs := map[string]map[string]struct{}{}
	for _, edge := range edges {
		otherType, otherID, ok := otherEndpoint(edge, core.Type, core.ID)
		if !ok {
			continue
		}
		if neighbourIDs[otherType] == nil {
			neighbourIDs[otherType] = map[string]struct{}{}
		}
		neighbourIDs[otherType][otherID] = struct{}{}
	}

	warnings := make([]domain.GraphWarning, 0, 4)
	neighbours := make([]domain.GraphNodeSummary, 0, len(neighbourIDs))
	missingNeighbourCount := 0
	danglingNeighbourCount := 0
	typesSeen := sortedTypeKeys(neighbourIDs)
	for _, neighbourType := range typesSeen {
		idSet := neighbourIDs[neighbourType]
		ids := sortedIDKeys(idSet)
		h, exists := s.hydrators[neighbourType]
		if !exists {
			missingNeighbourCount += len(ids)
			warnings = append(warnings, warningWithCappedIDs(
				"missing_hydrator",
				fmt.Sprintf("No hydrator registered for neighbour type %q", neighbourType),
				neighbourType,
				ids,
			))
			continue
		}
		summaries, err := h.FetchMany(ctx, projectID, ids)
		if err != nil {
			return domain.GraphNodeDetail{}, []domain.GraphWarning{}, err
		}
		returned := make(map[string]struct{}, len(summaries))
		for idx := range summaries {
			summary := &summaries[idx]
			summary.Type = normalizeNodeType(summary.Type)
			if summary.Type == "" {
				summary.Type = neighbourType
			}
			returned[strings.TrimSpace(summary.ID)] = struct{}{}
			parsed, parseErr := parseRFC3339Strict(summary.CreatedAt, fmt.Sprintf("%s/%s", summary.Type, summary.ID))
			if parseErr != nil {
				return domain.GraphNodeDetail{}, []domain.GraphWarning{}, parseErr
			}
			summary.CreatedAt = parsed.Format(time.RFC3339Nano)
		}
		for _, id := range ids {
			if _, ok := returned[id]; ok {
				continue
			}
			danglingNeighbourCount++
		}
		if len(ids) > len(returned) {
			danglingIDs := make([]string, 0, len(ids)-len(returned))
			for _, id := range ids {
				if _, ok := returned[id]; ok {
					continue
				}
				danglingIDs = append(danglingIDs, id)
			}
			warnings = append(warnings, warningWithCappedIDs(
				"dangling_edge",
				fmt.Sprintf("Neighbour nodes for type %q were not found", neighbourType),
				neighbourType,
				danglingIDs,
			))
		}
		neighbours = append(neighbours, summaries...)
	}

	sortNodeSummaries(neighbours)
	totalNeighbours := len(neighbours)
	if len(neighbours) > defaultNeighbourLimit {
		truncatedIDs := make([]string, 0, len(neighbours)-defaultNeighbourLimit)
		for _, neighbour := range neighbours[defaultNeighbourLimit:] {
			truncatedIDs = append(truncatedIDs, strings.TrimSpace(neighbour.ID))
		}
		warnings = append(warnings, warningWithCappedIDs(
			"neighbours_truncated",
			fmt.Sprintf("Neighbour list truncated to %d entries", defaultNeighbourLimit),
			"",
			truncatedIDs,
		))
		neighbours = neighbours[:defaultNeighbourLimit]
	}
	edges, err = s.pruneRedundantMissionEdges(ctx, projectID, core.Type, core.ID, edges)
	if err != nil {
		return domain.GraphNodeDetail{}, []domain.GraphWarning{}, err
	}
	sortGraphEdgesForFocal(edges, core.Type, core.ID)
	detail := domain.GraphNodeDetail{
		ID:                     core.ID,
		Type:                   core.Type,
		Title:                  strings.TrimSpace(core.Title),
		Status:                 strings.TrimSpace(core.Status),
		ScopeID:                strings.TrimSpace(core.ScopeID),
		CreatedAt:              core.CreatedAt,
		Fields:                 core.Fields,
		Neighbours:             neighbours,
		Edges:                  edges,
		MissingNeighbourCount:  missingNeighbourCount,
		DanglingNeighbourCount: danglingNeighbourCount,
	}
	if totalNeighbours == 0 {
		detail.Neighbours = []domain.GraphNodeSummary{}
	}
	return detail, warnings, nil
}

func (s *Service) pruneRedundantMissionEdges(ctx context.Context, projectID, focalType, focalID string, edges []domain.GraphEdge) ([]domain.GraphEdge, error) {
	if focalType != domain.NodeTypeMission || len(edges) == 0 {
		return edges, nil
	}
	out := make([]domain.GraphEdge, 0, len(edges))
	for _, edge := range edges {
		if s.isRedundantDirectMissionEdge(ctx, projectID, focalID, edge) {
			continue
		}
		out = append(out, edge)
	}
	return out, nil
}

func (s *Service) isRedundantDirectMissionEdge(ctx context.Context, projectID, missionID string, edge domain.GraphEdge) bool {
	if edge.EdgeType != domain.EdgeTypeServes || edge.TargetType != domain.NodeTypeMission || edge.TargetID != missionID {
		return false
	}
	switch edge.SourceType {
	case domain.NodeTypeTask, domain.NodeTypeDecision:
	default:
		return false
	}
	sourceLinks, err := s.contextLinks.FindBySource(ctx, contextlink.NodeRef{
		Type: contextlink.NodeType(edge.SourceType),
		ID:   edge.SourceID,
	})
	if err != nil {
		return false
	}
	for _, sourceLink := range sourceLinks {
		if strings.TrimSpace(string(sourceLink.EdgeType)) != domain.EdgeTypeServes {
			continue
		}
		if strings.TrimSpace(string(sourceLink.Target.Type)) != domain.NodeTypeKeyResult {
			continue
		}
		if s.keyResultBelongsToMission(ctx, projectID, sourceLink.Target.ID, missionID) {
			return true
		}
	}
	return false
}

func (s *Service) keyResultBelongsToMission(ctx context.Context, projectID, keyResultID, missionID string) bool {
	hydrator, ok := s.hydrators[domain.NodeTypeKeyResult]
	if !ok {
		return false
	}
	core, err := hydrator.Fetch(ctx, projectID, strings.TrimSpace(keyResultID))
	if err != nil {
		return false
	}
	raw, ok := core.Fields["missionId"].(string)
	if !ok {
		return false
	}
	return strings.TrimSpace(raw) == strings.TrimSpace(missionID)
}

func (s *Service) Search(ctx context.Context, req domain.GraphSearchRequest) ([]domain.GraphNodeSummary, []domain.GraphWarning, error) {
	projectID := strings.TrimSpace(req.ProjectID)
	nodeType := normalizeNodeType(req.NodeType)
	query := strings.TrimSpace(req.Query)
	limit := normalizeSearchLimit(req.Limit)
	hasEdge := strings.TrimSpace(strings.ToLower(req.HasEdge))
	missingEdge := strings.TrimSpace(strings.ToLower(req.MissingEdge))
	outgoingEdge := strings.TrimSpace(strings.ToLower(req.OutgoingEdge))
	incomingEdge := strings.TrimSpace(strings.ToLower(req.IncomingEdge))
	if projectID == "" {
		return []domain.GraphNodeSummary{}, []domain.GraphWarning{}, fmt.Errorf("graph_query: project id is required")
	}
	if query == "" && hasEdge == "" && missingEdge == "" && outgoingEdge == "" && incomingEdge == "" && (nodeType == "" || nodeType == domain.NodeTypeAll) {
		return []domain.GraphNodeSummary{}, []domain.GraphWarning{}, fmt.Errorf("graph_query: query is required for node_type=all; use a concrete node_type to browse without a query")
	}
	if hasEdge != "" && !contextlink.ValidEdgeType(contextlink.EdgeType(hasEdge)) {
		return []domain.GraphNodeSummary{}, []domain.GraphWarning{}, fmt.Errorf("graph_query: unknown has_edge %q", hasEdge)
	}
	if missingEdge != "" && !contextlink.ValidEdgeType(contextlink.EdgeType(missingEdge)) {
		return []domain.GraphNodeSummary{}, []domain.GraphWarning{}, fmt.Errorf("graph_query: unknown missing_edge %q", missingEdge)
	}
	if outgoingEdge != "" && !contextlink.ValidEdgeType(contextlink.EdgeType(outgoingEdge)) {
		return []domain.GraphNodeSummary{}, []domain.GraphWarning{}, fmt.Errorf("graph_query: unknown outgoing_edge %q", outgoingEdge)
	}
	if incomingEdge != "" && !contextlink.ValidEdgeType(contextlink.EdgeType(incomingEdge)) {
		return []domain.GraphNodeSummary{}, []domain.GraphWarning{}, fmt.Errorf("graph_query: unknown incoming_edge %q", incomingEdge)
	}
	if hasEdge != "" && missingEdge != "" && hasEdge == missingEdge {
		return []domain.GraphNodeSummary{}, []domain.GraphWarning{}, fmt.Errorf("graph_query: has_edge and missing_edge cannot both be %q", hasEdge)
	}
	searchLimit := limit
	if hasEdge != "" || missingEdge != "" || outgoingEdge != "" || incomingEdge != "" {
		searchLimit = max(limit*5, 100)
	}
	pinnedRefs := s.pinnedRefs(ctx, projectID)
	var summaries []domain.GraphNodeSummary
	var warnings []domain.GraphWarning
	relevanceSorted := false
	if nodeType == domain.NodeTypeAll {
		hits, searchWarnings, err := s.searchAll(ctx, projectID, query, searchLimit, pinnedRefs)
		if err != nil {
			return []domain.GraphNodeSummary{}, warningsOrEmpty(searchWarnings), err
		}
		warnings = searchWarnings
		summaries = summariesFromHits(hits)
		relevanceSorted = true
	} else {
		hydrator, ok := s.hydrators[nodeType]
		if !ok {
			return []domain.GraphNodeSummary{}, []domain.GraphWarning{}, fmt.Errorf("graph_query: unsupported node_type %q", nodeType)
		}
		var err error
		summaries, err = hydrator.Search(ctx, projectID, query, searchLimit)
		if err != nil {
			return []domain.GraphNodeSummary{}, []domain.GraphWarning{}, err
		}
	}
	for idx := range summaries {
		summary := &summaries[idx]
		summary.Type = normalizeNodeType(summary.Type)
		if summary.Type == "" {
			summary.Type = nodeType
		}
		createdAt, parseErr := parseRFC3339Strict(summary.CreatedAt, fmt.Sprintf("%s/%s", summary.Type, summary.ID))
		if parseErr != nil {
			return []domain.GraphNodeSummary{}, []domain.GraphWarning{}, parseErr
		}
		summary.CreatedAt = createdAt.Format(time.RFC3339Nano)
	}
	if !relevanceSorted && query != "" {
		sortSummariesBySearchScore(summaries, query, pinnedRefs)
	} else if !relevanceSorted {
		sortNodeSummaries(summaries)
		promotePinned(summaries, pinnedRefs)
	}
	if hasEdge != "" || missingEdge != "" || outgoingEdge != "" || incomingEdge != "" {
		filtered, err := s.filterSummariesByEdges(ctx, summaries, edgeFilter{
			hasEdge:      hasEdge,
			missingEdge:  missingEdge,
			outgoingEdge: outgoingEdge,
			incomingEdge: incomingEdge,
		})
		if err != nil {
			return []domain.GraphNodeSummary{}, []domain.GraphWarning{}, err
		}
		summaries = filtered
	}
	if len(summaries) > limit {
		summaries = summaries[:limit]
	}
	if summaries == nil {
		summaries = []domain.GraphNodeSummary{}
	}
	return summaries, warningsOrEmpty(warnings), nil
}

func (s *Service) Link(ctx context.Context, req domain.GraphLinkRequest) (domain.GraphEdge, []domain.GraphWarning, error) {
	req.ProjectID = strings.TrimSpace(req.ProjectID)
	req.SourceType = normalizeNodeType(req.SourceType)
	req.SourceID = strings.TrimSpace(req.SourceID)
	req.TargetType = normalizeNodeType(req.TargetType)
	req.TargetID = strings.TrimSpace(req.TargetID)
	req.EdgeType = strings.TrimSpace(strings.ToLower(req.EdgeType))
	req.Rationale = strings.TrimSpace(req.Rationale)
	req.Origin = normalizeEdgeOrigin(req.Origin, "manual")
	req.CreatedBy = strings.TrimSpace(req.CreatedBy)
	if req.CreatedBy == "" {
		req.CreatedBy = defaultGraphCreator
	}
	if req.ProjectID == "" {
		return domain.GraphEdge{}, []domain.GraphWarning{}, fmt.Errorf("graph_query: project id is required")
	}
	if req.SourceID == "" || req.TargetID == "" {
		return domain.GraphEdge{}, []domain.GraphWarning{}, fmt.Errorf("graph_query: source and target are required")
	}
	if req.SourceType == "" {
		inferred, err := inferEndpointTypeFromID("source_type", req.SourceID)
		if err != nil {
			return domain.GraphEdge{}, []domain.GraphWarning{}, err
		}
		req.SourceType = inferred
	}
	if req.TargetType == "" {
		inferred, err := inferEndpointTypeFromID("target_type", req.TargetID)
		if err != nil {
			return domain.GraphEdge{}, []domain.GraphWarning{}, err
		}
		req.TargetType = inferred
	}
	if req.EdgeType == "" {
		return domain.GraphEdge{}, []domain.GraphWarning{}, fmt.Errorf("graph_query: edge_type is required")
	}
	if req.SourceType == req.TargetType && req.SourceID == req.TargetID {
		return domain.GraphEdge{}, []domain.GraphWarning{}, fmt.Errorf("graph_query: source and target must be different nodes")
	}
	confidence := 1.0
	if req.Confidence != nil {
		confidence = *req.Confidence
	}
	if confidence < 0 || confidence > 1 {
		return domain.GraphEdge{}, []domain.GraphWarning{}, fmt.Errorf("graph_query: confidence must be between 0.0 and 1.0")
	}
	sourceHydrator, ok := s.hydrators[req.SourceType]
	if !ok {
		return domain.GraphEdge{}, []domain.GraphWarning{}, fmt.Errorf("graph_query: unsupported source_type %q", req.SourceType)
	}
	targetHydrator, ok := s.hydrators[req.TargetType]
	if !ok {
		return domain.GraphEdge{}, []domain.GraphWarning{}, fmt.Errorf("graph_query: unsupported target_type %q", req.TargetType)
	}
	if _, err := sourceHydrator.Fetch(ctx, req.ProjectID, req.SourceID); err != nil {
		return domain.GraphEdge{}, []domain.GraphWarning{}, fmt.Errorf("graph_query: source node not found: %w", err)
	}
	if _, err := targetHydrator.Fetch(ctx, req.ProjectID, req.TargetID); err != nil {
		return domain.GraphEdge{}, []domain.GraphWarning{}, fmt.Errorf("graph_query: target node not found: %w", err)
	}

	existing, err := s.contextLinks.FindBetween(ctx, contextlink.NodeRef{Type: contextlink.NodeType(req.SourceType), ID: req.SourceID}, contextlink.NodeRef{Type: contextlink.NodeType(req.TargetType), ID: req.TargetID})
	if err != nil {
		return domain.GraphEdge{}, []domain.GraphWarning{}, fmt.Errorf("graph_query: find existing link: %w", err)
	}
	var matched *contextlink.Link
	for idx := range existing {
		link := existing[idx]
		if strings.EqualFold(strings.TrimSpace(string(link.EdgeType)), req.EdgeType) {
			matched = &link
			break
		}
	}

	now := time.Now().UTC()
	if matched != nil {
		oldConfidence := matched.Confidence
		oldRationale := strings.TrimSpace(matched.Metadata["rationale"])
		oldOrigin := edgeOriginFromLink(*matched)
		oldCreatedBy := strings.TrimSpace(matched.CreatedBy)
		operation := "noop"
		if oldConfidence != confidence || oldRationale != req.Rationale || oldOrigin != req.Origin || oldCreatedBy != req.CreatedBy {
			operation = "updated"
		}
		if operation == "noop" {
			return graphEdgeFromLink(*matched, operation), []domain.GraphWarning{}, nil
		}
		matched.Confidence = confidence
		if matched.Metadata == nil {
			matched.Metadata = map[string]string{}
		}
		if req.Rationale != "" {
			matched.Metadata["rationale"] = req.Rationale
		} else {
			delete(matched.Metadata, "rationale")
		}
		matched.Metadata["origin"] = req.Origin
		if matched.CreatedAt.IsZero() {
			matched.CreatedAt = now
		}
		matched.CreatedBy = req.CreatedBy
		if err := s.contextLinks.Replace(ctx, *matched); err != nil {
			return domain.GraphEdge{}, []domain.GraphWarning{}, fmt.Errorf("graph_query: replace link save: %w", err)
		}
		return graphEdgeFromLink(*matched, operation), []domain.GraphWarning{}, nil
	}

	metadata := map[string]string{}
	if req.Rationale != "" {
		metadata["rationale"] = req.Rationale
	}
	metadata["origin"] = req.Origin
	link := contextlink.Link{
		ID:         contextlink.ID("cl-graph-" + uuid.NewString()),
		Source:     contextlink.NodeRef{Type: contextlink.NodeType(req.SourceType), ID: req.SourceID},
		Target:     contextlink.NodeRef{Type: contextlink.NodeType(req.TargetType), ID: req.TargetID},
		EdgeType:   contextlink.EdgeType(req.EdgeType),
		Confidence: confidence,
		Metadata:   metadata,
		CreatedAt:  now,
		CreatedBy:  req.CreatedBy,
	}
	if err := s.contextLinks.Save(ctx, link); err != nil {
		return domain.GraphEdge{}, []domain.GraphWarning{}, fmt.Errorf("graph_query: create link: %w", err)
	}
	return graphEdgeFromLink(link, "created"), []domain.GraphWarning{}, nil
}

func (s *Service) Unlink(ctx context.Context, req domain.GraphLinkRequest) (domain.GraphEdge, []domain.GraphWarning, error) {
	req.ProjectID = strings.TrimSpace(req.ProjectID)
	req.EdgeID = strings.TrimSpace(req.EdgeID)
	req.SourceType = normalizeNodeType(req.SourceType)
	req.SourceID = strings.TrimSpace(req.SourceID)
	req.TargetType = normalizeNodeType(req.TargetType)
	req.TargetID = strings.TrimSpace(req.TargetID)
	req.EdgeType = strings.TrimSpace(strings.ToLower(req.EdgeType))
	if req.ProjectID == "" {
		return domain.GraphEdge{}, []domain.GraphWarning{}, fmt.Errorf("graph_query: project id is required")
	}
	if req.EdgeID != "" {
		if req.SourceType != "" || req.SourceID != "" || req.TargetType != "" || req.TargetID != "" || req.EdgeType != "" {
			return domain.GraphEdge{}, []domain.GraphWarning{}, fmt.Errorf("graph_query: edge_id unlink must omit source, target, and edge_type fields")
		}
		return s.unlinkByID(ctx, req.ProjectID, req.EdgeID)
	}
	if req.SourceID == "" || req.TargetID == "" {
		return domain.GraphEdge{}, []domain.GraphWarning{}, fmt.Errorf("graph_query: source and target are required")
	}
	if req.SourceType == "" {
		inferred, err := inferEndpointTypeFromID("source_type", req.SourceID)
		if err != nil {
			return domain.GraphEdge{}, []domain.GraphWarning{}, err
		}
		req.SourceType = inferred
	}
	if req.TargetType == "" {
		inferred, err := inferEndpointTypeFromID("target_type", req.TargetID)
		if err != nil {
			return domain.GraphEdge{}, []domain.GraphWarning{}, err
		}
		req.TargetType = inferred
	}
	if req.EdgeType == "" {
		return domain.GraphEdge{}, []domain.GraphWarning{}, fmt.Errorf("graph_query: edge_type is required")
	}
	existing, err := s.contextLinks.FindBetween(ctx, contextlink.NodeRef{Type: contextlink.NodeType(req.SourceType), ID: req.SourceID}, contextlink.NodeRef{Type: contextlink.NodeType(req.TargetType), ID: req.TargetID})
	if err != nil {
		return domain.GraphEdge{}, []domain.GraphWarning{}, fmt.Errorf("graph_query: find existing link: %w", err)
	}
	for _, link := range existing {
		if !strings.EqualFold(strings.TrimSpace(string(link.EdgeType)), req.EdgeType) {
			continue
		}
		if err := s.contextLinks.Delete(ctx, link.ID); err != nil {
			return domain.GraphEdge{}, []domain.GraphWarning{}, fmt.Errorf("graph_query: delete link: %w", err)
		}
		return graphEdgeFromLink(link, "deleted"), []domain.GraphWarning{}, nil
	}
	return domain.GraphEdge{}, []domain.GraphWarning{}, fmt.Errorf("graph_query: link not found for %s/%s --[%s]--> %s/%s", req.SourceType, req.SourceID, req.EdgeType, req.TargetType, req.TargetID)
}

func (s *Service) unlinkByID(ctx context.Context, projectID, edgeID string) (domain.GraphEdge, []domain.GraphWarning, error) {
	link, err := s.contextLinks.FindByID(ctx, contextlink.ID(edgeID))
	if err != nil {
		return domain.GraphEdge{}, []domain.GraphWarning{}, fmt.Errorf("graph_query: find link by edge_id: %w", err)
	}
	if err := s.validateLinkEndpointsInProject(ctx, projectID, link); err != nil {
		return domain.GraphEdge{}, []domain.GraphWarning{}, err
	}
	if err := s.contextLinks.Delete(ctx, link.ID); err != nil {
		return domain.GraphEdge{}, []domain.GraphWarning{}, fmt.Errorf("graph_query: delete link: %w", err)
	}
	return graphEdgeFromLink(link, "deleted"), []domain.GraphWarning{}, nil
}

func (s *Service) validateLinkEndpointsInProject(ctx context.Context, projectID string, link contextlink.Link) error {
	sourceType := strings.TrimSpace(string(link.Source.Type))
	sourceHydrator, ok := s.hydrators[sourceType]
	if !ok {
		return fmt.Errorf("graph_query: unsupported source_type %q for edge_id unlink", sourceType)
	}
	if _, err := sourceHydrator.Fetch(ctx, projectID, strings.TrimSpace(link.Source.ID)); err != nil {
		return fmt.Errorf("graph_query: edge source node not visible in project: %w", err)
	}
	targetType := strings.TrimSpace(string(link.Target.Type))
	targetHydrator, ok := s.hydrators[targetType]
	if !ok {
		return fmt.Errorf("graph_query: unsupported target_type %q for edge_id unlink", targetType)
	}
	if _, err := targetHydrator.Fetch(ctx, projectID, strings.TrimSpace(link.Target.ID)); err != nil {
		return fmt.Errorf("graph_query: edge target node not visible in project: %w", err)
	}
	return nil
}

func (s *Service) DeleteLinksForNode(ctx context.Context, nodeType string, nodeID string) error {
	nodeType = normalizeNodeType(nodeType)
	nodeID = strings.TrimSpace(nodeID)
	if nodeType == "" {
		return fmt.Errorf("graph_query: node_type is required")
	}
	if nodeID == "" {
		return fmt.Errorf("graph_query: node_id is required")
	}
	if err := s.contextLinks.DeleteLinksForEntity(ctx, contextlink.NodeRef{Type: contextlink.NodeType(nodeType), ID: nodeID}); err != nil {
		return fmt.Errorf("graph_query: delete links for %s/%s: %w", nodeType, nodeID, err)
	}
	return nil
}

func normalizeNodeType(nodeType string) string {
	return strings.ToLower(strings.TrimSpace(nodeType))
}

func normalizeNodeDepth(depth int) int {
	if depth <= 0 {
		return defaultNodeDepth
	}
	if depth > maxNodeDepth {
		return maxNodeDepth
	}
	return depth
}

func inferNodeTypeFromID(nodeID string) (string, error) {
	id := strings.TrimSpace(strings.ToLower(nodeID))
	prefixMap := []struct {
		prefix   string
		nodeType string
	}{
		{"task-", domain.NodeTypeTask},
		{"dec-", domain.NodeTypeDecision},
		{"kr-", domain.NodeTypeKeyResult},
		{"mis-", domain.NodeTypeMission},
	}
	for _, candidate := range prefixMap {
		if strings.HasPrefix(id, candidate.prefix) {
			return candidate.nodeType, nil
		}
	}
	return "", fmt.Errorf("graph_query: node_type is required when node_id prefix is not recognized: %q", strings.TrimSpace(nodeID))
}

func inferEndpointTypeFromID(fieldName, nodeID string) (string, error) {
	nodeType, err := inferNodeTypeFromID(nodeID)
	if err != nil {
		return "", fmt.Errorf("graph_query: %s is required when node id prefix is not recognized: %q", strings.TrimSpace(fieldName), strings.TrimSpace(nodeID))
	}
	return nodeType, nil
}

func graphEdgeFromLink(link contextlink.Link, operation string) domain.GraphEdge {
	createdAt := ""
	if !link.CreatedAt.IsZero() {
		createdAt = link.CreatedAt.UTC().Format(time.RFC3339Nano)
	}
	origin := edgeOriginFromLink(link)
	createdBy := edgeCreatedByFromLink(link, origin)
	manual := origin == "manual"
	return domain.GraphEdge{
		ID:         strings.TrimSpace(string(link.ID)),
		SourceType: strings.TrimSpace(string(link.Source.Type)),
		SourceID:   strings.TrimSpace(link.Source.ID),
		TargetType: strings.TrimSpace(string(link.Target.Type)),
		TargetID:   strings.TrimSpace(link.Target.ID),
		EdgeType:   strings.TrimSpace(string(link.EdgeType)),
		Confidence: link.Confidence,
		Rationale:  strings.TrimSpace(link.Metadata["rationale"]),
		CreatedAt:  createdAt,
		CreatedBy:  createdBy,
		Origin:     origin,
		Source:     origin,
		Manual:     manual,
		Operation:  strings.TrimSpace(operation),
	}
}

func normalizeEdgeOrigin(origin, defaultOrigin string) string {
	origin = strings.TrimSpace(strings.ToLower(origin))
	switch origin {
	case "":
		return strings.TrimSpace(strings.ToLower(defaultOrigin))
	case "manual", "reference", "system":
		return origin
	case "explicit":
		return "manual"
	default:
		return strings.TrimSpace(strings.ToLower(defaultOrigin))
	}
}

func normalizeLegacyEdgeSource(source string) string {
	source = strings.TrimSpace(strings.ToLower(source))
	switch source {
	case "manual", "explicit":
		return "manual"
	case "reference", "system":
		return source
	default:
		return ""
	}
}

func inferEdgeOrigin(createdBy string) string {
	createdBy = strings.TrimSpace(createdBy)
	if createdBy == defaultGraphCreator || strings.HasPrefix(createdBy, "member:") {
		return "manual"
	}
	if createdBy == "" || createdBy == "system" {
		return "system"
	}
	return "reference"
}

func edgeOriginFromLink(link contextlink.Link) string {
	if origin := normalizeEdgeOrigin(link.Metadata["origin"], ""); origin != "" {
		if origin == "manual" && rationaleImpliesReference(link.Metadata["rationale"]) {
			return "reference"
		}
		return origin
	}
	if source := normalizeLegacyEdgeSource(link.Metadata["source"]); source != "" {
		if source == "manual" && rationaleImpliesReference(link.Metadata["rationale"]) {
			return "reference"
		}
		return source
	}
	if rationaleImpliesReference(link.Metadata["rationale"]) {
		return "reference"
	}
	return inferEdgeOrigin(link.CreatedBy)
}

func edgeCreatedByFromLink(link contextlink.Link, origin string) string {
	createdBy := strings.TrimSpace(link.CreatedBy)
	if origin == "reference" && createdBy == defaultGraphCreator {
		switch strings.TrimSpace(string(link.Source.Type)) {
		case domain.NodeTypeTask:
			return "task_service"
		case domain.NodeTypeDecision:
			return "decision_service"
		}
	}
	return createdBy
}

func rationaleImpliesReference(rationale string) bool {
	rationale = strings.ToLower(strings.TrimSpace(rationale))
	return strings.Contains(rationale, "references this work item")
}

func normalizeSearchLimit(limit int) int {
	if limit <= 0 {
		return 10
	}
	if limit > 50 {
		return 50
	}
	return limit
}

func (s *Service) expandSubgraph(ctx context.Context, projectID string, root domain.GraphNodeDetail, depth int) ([]domain.GraphNodeSummary, []domain.GraphWarning, error) {
	type frontierNode struct {
		nodeType string
		nodeID   string
		level    int
	}
	seen := map[string]struct{}{
		graphKey(root.Type, root.ID): {},
	}
	outByKey := map[string]domain.GraphNodeSummary{}
	queue := make([]frontierNode, 0, len(root.Neighbours))
	for _, neighbour := range root.Neighbours {
		key := graphKey(neighbour.Type, neighbour.ID)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		outByKey[key] = neighbour
		queue = append(queue, frontierNode{nodeType: neighbour.Type, nodeID: neighbour.ID, level: 1})
	}

	var warnings []domain.GraphWarning
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if current.level >= depth {
			continue
		}
		detail, nodeWarnings, err := s.nodeOneHop(ctx, projectID, current.nodeType, current.nodeID)
		if err != nil {
			return nil, warningsOrEmpty(warnings), err
		}
		warnings = append(warnings, nodeWarnings...)
		for _, neighbour := range detail.Neighbours {
			key := graphKey(neighbour.Type, neighbour.ID)
			if key == "" {
				continue
			}
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			outByKey[key] = neighbour
			queue = append(queue, frontierNode{nodeType: neighbour.Type, nodeID: neighbour.ID, level: current.level + 1})
		}
	}

	out := make([]domain.GraphNodeSummary, 0, len(outByKey))
	for _, node := range outByKey {
		out = append(out, node)
	}
	sortNodeSummaries(out)
	if out == nil {
		out = []domain.GraphNodeSummary{}
	}
	return out, warningsOrEmpty(warnings), nil
}

func graphKey(nodeType, nodeID string) string {
	nodeType = strings.TrimSpace(nodeType)
	nodeID = strings.TrimSpace(nodeID)
	if nodeType == "" || nodeID == "" {
		return ""
	}
	return nodeType + "/" + nodeID
}

type edgeFilter struct {
	hasEdge      string
	missingEdge  string
	outgoingEdge string
	incomingEdge string
}

func (s *Service) filterSummariesByEdges(ctx context.Context, summaries []domain.GraphNodeSummary, filter edgeFilter) ([]domain.GraphNodeSummary, error) {
	out := make([]domain.GraphNodeSummary, 0, len(summaries))
	for _, summary := range summaries {
		ref := contextlink.NodeRef{Type: contextlink.NodeType(strings.TrimSpace(summary.Type)), ID: strings.TrimSpace(summary.ID)}
		sourceLinks, err := s.contextLinks.FindBySource(ctx, ref)
		if err != nil {
			return nil, fmt.Errorf("graph_query: outgoing edge filter links for %s/%s: %w", summary.Type, summary.ID, err)
		}
		targetLinks, err := s.contextLinks.FindByTarget(ctx, ref)
		if err != nil {
			return nil, fmt.Errorf("graph_query: incoming edge filter links for %s/%s: %w", summary.Type, summary.ID, err)
		}
		allLinks := mergeContextLinks(sourceLinks, targetLinks)
		if filter.hasEdge != "" && !edgePresent(allLinks, filter.hasEdge) {
			continue
		}
		if filter.missingEdge != "" && edgePresent(allLinks, filter.missingEdge) {
			continue
		}
		if filter.outgoingEdge != "" && !edgePresent(sourceLinks, filter.outgoingEdge) {
			continue
		}
		if filter.incomingEdge != "" && !edgePresent(targetLinks, filter.incomingEdge) {
			continue
		}
		summary.MatchedEdges = matchedEdgeSnippets(sourceLinks, targetLinks, filter)
		out = append(out, summary)
	}
	if out == nil {
		return []domain.GraphNodeSummary{}, nil
	}
	return out, nil
}

func matchedEdgeSnippets(sourceLinks []contextlink.Link, targetLinks []contextlink.Link, filter edgeFilter) []domain.GraphEdge {
	links := make([]contextlink.Link, 0)
	if filter.hasEdge != "" {
		links = appendMatchingEdgeType(links, mergeContextLinks(sourceLinks, targetLinks), filter.hasEdge)
	}
	if filter.outgoingEdge != "" {
		links = appendMatchingEdgeType(links, sourceLinks, filter.outgoingEdge)
	}
	if filter.incomingEdge != "" {
		links = appendMatchingEdgeType(links, targetLinks, filter.incomingEdge)
	}
	links = dedupeContextLinks(links)
	sortContextLinks(links)
	if len(links) > 5 {
		links = links[:5]
	}
	out := make([]domain.GraphEdge, 0, len(links))
	for _, link := range links {
		out = append(out, graphEdgeFromLink(link, ""))
	}
	return out
}

func appendMatchingEdgeType(out []contextlink.Link, links []contextlink.Link, edgeType string) []contextlink.Link {
	for _, link := range links {
		if strings.EqualFold(strings.TrimSpace(string(link.EdgeType)), strings.TrimSpace(edgeType)) {
			out = append(out, link)
		}
	}
	return out
}

func mergeContextLinks(left []contextlink.Link, right []contextlink.Link) []contextlink.Link {
	return dedupeContextLinks(append(left, right...))
}

func dedupeContextLinks(links []contextlink.Link) []contextlink.Link {
	seen := make(map[contextlink.ID]struct{}, len(links))
	out := make([]contextlink.Link, 0, len(links))
	for _, link := range links {
		if _, ok := seen[link.ID]; ok {
			continue
		}
		seen[link.ID] = struct{}{}
		out = append(out, link)
	}
	return out
}

func sortContextLinks(links []contextlink.Link) {
	sort.SliceStable(links, func(i, j int) bool {
		left := strings.TrimSpace(string(links[i].EdgeType)) + "/" + strings.TrimSpace(string(links[i].ID))
		right := strings.TrimSpace(string(links[j].EdgeType)) + "/" + strings.TrimSpace(string(links[j].ID))
		return left < right
	})
}

func edgePresent(links []contextlink.Link, edgeType string) bool {
	edgeType = strings.TrimSpace(edgeType)
	if edgeType == "" {
		return false
	}
	for _, link := range links {
		if strings.EqualFold(strings.TrimSpace(string(link.EdgeType)), edgeType) {
			return true
		}
	}
	return false
}

func parseRFC3339Strict(raw, label string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(raw))
	if err == nil {
		return parsed.UTC(), nil
	}
	parsed, err = time.Parse(time.RFC3339Nano, strings.TrimSpace(raw))
	if err == nil {
		return parsed.UTC(), nil
	}
	return time.Time{}, fmt.Errorf("graph_query: invalid createdAt for %s: %q", strings.TrimSpace(label), strings.TrimSpace(raw))
}

func warningWithCappedIDs(code, message, affectedType string, ids []string) domain.GraphWarning {
	ids = dedupeNonEmpty(ids)
	out := domain.GraphWarning{
		Code:          strings.TrimSpace(code),
		Message:       strings.TrimSpace(message),
		AffectedType:  strings.TrimSpace(affectedType),
		AffectedIDs:   []string{},
		AffectedCount: len(ids),
	}
	if len(ids) == 0 {
		return out
	}
	if len(ids) > maxWarningIDs {
		ids = ids[:maxWarningIDs]
	}
	out.AffectedIDs = append(out.AffectedIDs, ids...)
	return out
}

func warningsOrEmpty(warnings []domain.GraphWarning) []domain.GraphWarning {
	if warnings == nil {
		return []domain.GraphWarning{}
	}
	return warnings
}

func sortedTypeKeys(byType map[string]map[string]struct{}) []string {
	out := make([]string, 0, len(byType))
	for nodeType := range byType {
		out = append(out, nodeType)
	}
	sort.Strings(out)
	return out
}

func sortedIDKeys(ids map[string]struct{}) []string {
	out := make([]string, 0, len(ids))
	for id := range ids {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

func dedupeNonEmpty(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, raw := range values {
		value := strings.TrimSpace(raw)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func sortNodeSummaries(items []domain.GraphNodeSummary) {
	sort.Slice(items, func(i, j int) bool {
		leftStatusRank := graphNodeStatusRank(items[i].Status)
		rightStatusRank := graphNodeStatusRank(items[j].Status)
		if leftStatusRank != rightStatusRank {
			return leftStatusRank < rightStatusRank
		}
		leftTime, _ := time.Parse(time.RFC3339Nano, strings.TrimSpace(items[i].CreatedAt))
		rightTime, _ := time.Parse(time.RFC3339Nano, strings.TrimSpace(items[j].CreatedAt))
		if !leftTime.Equal(rightTime) {
			return leftTime.After(rightTime)
		}
		leftType := strings.TrimSpace(items[i].Type)
		rightType := strings.TrimSpace(items[j].Type)
		if leftType != rightType {
			return leftType < rightType
		}
		return strings.TrimSpace(items[i].ID) < strings.TrimSpace(items[j].ID)
	})
}

func graphNodeStatusRank(status string) int {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "active", "in_progress", "pending", "in_review":
		return 0
	case "blocked", "failed":
		return 1
	case "log", "recorded":
		return 2
	case "succeeded", "completed", "done":
		return 3
	case "canceled", "cancelled", "archived", "dropped":
		return 4
	default:
		return 2
	}
}

func sortGraphEdgesForFocal(edges []domain.GraphEdge, focalType, focalID string) {
	focalType = strings.TrimSpace(focalType)
	focalID = strings.TrimSpace(focalID)
	sort.Slice(edges, func(i, j int) bool {
		liType, liID, _ := otherEndpoint(edges[i], focalType, focalID)
		ljType, ljID, _ := otherEndpoint(edges[j], focalType, focalID)
		liEdgeType := strings.TrimSpace(edges[i].EdgeType)
		ljEdgeType := strings.TrimSpace(edges[j].EdgeType)
		if liEdgeType != ljEdgeType {
			return liEdgeType < ljEdgeType
		}
		if liType != ljType {
			return liType < ljType
		}
		if liID != ljID {
			return liID < ljID
		}
		if edges[i].SourceType != edges[j].SourceType {
			return edges[i].SourceType < edges[j].SourceType
		}
		if edges[i].SourceID != edges[j].SourceID {
			return edges[i].SourceID < edges[j].SourceID
		}
		if edges[i].TargetType != edges[j].TargetType {
			return edges[i].TargetType < edges[j].TargetType
		}
		return edges[i].TargetID < edges[j].TargetID
	})
}

func otherEndpoint(edge domain.GraphEdge, focalType, focalID string) (string, string, bool) {
	sourceType := strings.TrimSpace(edge.SourceType)
	sourceID := strings.TrimSpace(edge.SourceID)
	targetType := strings.TrimSpace(edge.TargetType)
	targetID := strings.TrimSpace(edge.TargetID)
	if sourceType == focalType && sourceID == focalID {
		if targetType == "" || targetID == "" {
			return "", "", false
		}
		return targetType, targetID, true
	}
	if targetType == focalType && targetID == focalID {
		if sourceType == "" || sourceID == "" {
			return "", "", false
		}
		return sourceType, sourceID, true
	}
	return "", "", false
}
