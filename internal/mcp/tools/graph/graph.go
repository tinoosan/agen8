package graph

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/tinoosan/agen8-mcp-server/internal/caller"
	"github.com/tinoosan/agen8-mcp-server/internal/core/types"
	"github.com/tinoosan/agen8-mcp-server/internal/services/graph/domain"
	"github.com/tinoosan/agen8-mcp-server/internal/services/project/domain/member"
)

const Name = "graph_query"
const Description = "[GRAPH] Read, search, link, and unlink workspace graph nodes."
const maxSearchContextDepth = 3

var allActions = []string{"node", "search", "link", "unlink"}
var searchableNodeTypes = []string{
	domain.NodeTypeTask,
	domain.NodeTypeDecision,
	domain.NodeTypeKeyResult,
	domain.NodeTypeMission,
	domain.NodeTypeAll,
}
var linkableNodeTypes = []string{
	domain.NodeTypeTask,
	domain.NodeTypeDecision,
	domain.NodeTypeKeyResult,
	domain.NodeTypeMission,
}
var edgeTypes = []string{
	"blocked_by",
	"resolved_by",
	"completed_by",
	"serves",
	"informed_by",
	"produced",
	"made_during",
	"spawned",
	"child_of",
	"relates_to",
}

type Service interface {
	domain.GraphQueryService
}

type MemberDirectory interface {
	GetMember(ctx context.Context, id member.ID) (member.Record, error)
}

type CallContext struct {
	Graph         Service
	Members       MemberDirectory
	ProjectID     string
	ActorMemberID string
}

type Result struct {
	Text       string
	Structured any
}

type Handler struct{}

func NewHandler() Handler {
	return Handler{}
}

func (h Handler) Schema() json.RawMessage {
	body, err := json.Marshal(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action":        map[string]any{"type": "string", "enum": allActions},
			"node_type":     enumSchema(searchableNodeTypes, "Node type for node/search. Optional for node when node_id has a typed prefix. Use all only with action=search."),
			"node_id":       stringSchema("Node id for node."),
			"depth":         integerSchema("Node expansion depth for node and optional bounded context for search. Maximum 3."),
			"query":         stringSchema("Search query. Empty is valid only with a concrete node_type or an edge filter."),
			"limit":         integerSchema("Search limit. Maximum 50."),
			"has_edge":      enumSchema(edgeTypes, "Only return search hits with this edge type."),
			"missing_edge":  enumSchema(edgeTypes, "Only return search hits missing this edge type."),
			"outgoing_edge": enumSchema(edgeTypes, "Only return search hits where the node is the source of this edge type."),
			"incoming_edge": enumSchema(edgeTypes, "Only return search hits where the node is the target of this edge type."),
			"source_type":   enumSchema(linkableNodeTypes, "Source node type for link/unlink. Optional when source_id has a typed prefix."),
			"source_id":     stringSchema("Source node id for link/unlink."),
			"target_type":   enumSchema(linkableNodeTypes, "Target node type for link/unlink. Optional when target_id has a typed prefix."),
			"target_id":     stringSchema("Target node id for link/unlink."),
			"edge_id":       stringSchema("Stable edge id for unlink. Omit source, target, and edge_type when using edge_id."),
			"edge_type":     enumSchema(edgeTypes, "Edge type for link/unlink."),
			"confidence":    numberSchema("Link confidence from 0.0 to 1.0."),
			"rationale":     stringSchema("Human-readable link rationale."),
		},
		"required":             []string{"action"},
		"additionalProperties": false,
	})
	if err != nil {
		panic(fmt.Sprintf("graph schema encode: %v", err))
	}
	return body
}

func (h Handler) Handle(ctx context.Context, call CallContext, args json.RawMessage) (Result, error) {
	input, err := decode(args)
	if err != nil {
		return Result{}, err
	}
	if call.Graph == nil {
		return Result{}, fmt.Errorf("graph_query: graph service is not configured")
	}
	ctx, err = h.contextWithActor(ctx, call)
	if err != nil {
		return Result{}, err
	}
	projectID := strings.TrimSpace(call.ProjectID)
	if projectID == "" {
		return Result{}, fmt.Errorf("graph_query: project id is required")
	}
	switch input.Action {
	case "node":
		nodeID, err := requireValue(input.NodeID, "node_id")
		if err != nil {
			return Result{}, err
		}
		node, warnings, err := call.Graph.Node(ctx, projectID, input.NodeType, nodeID, input.Depth)
		return result("node", map[string]any{"node": node, "warnings": warnings}, err)
	case "search":
		nodes, warnings, err := call.Graph.Search(ctx, domain.GraphSearchRequest{
			ProjectID:    projectID,
			NodeType:     input.NodeType,
			Query:        input.Query,
			Limit:        input.Limit,
			HasEdge:      input.HasEdge,
			MissingEdge:  input.MissingEdge,
			OutgoingEdge: input.OutgoingEdge,
			IncomingEdge: input.IncomingEdge,
		})
		if err != nil {
			return Result{}, err
		}
		fields := map[string]any{"nodes": nodes, "warnings": warnings, "count": len(nodes)}
		if input.Depth > 0 {
			contexts, contextWarnings, err := searchContexts(ctx, call.Graph, projectID, nodes, input.Depth)
			if err != nil {
				return Result{}, err
			}
			fields["depth"] = normalizedSearchContextDepth(input.Depth)
			fields["contexts"] = contexts
			fields["warnings"] = append(warnings, contextWarnings...)
		}
		return result("search", fields, nil)
	case "link":
		req, err := linkRequest(projectID, input, true)
		if err != nil {
			return Result{}, err
		}
		edge, warnings, err := call.Graph.Link(ctx, req)
		return result("link", map[string]any{"edge": edge, "warnings": warnings}, err)
	case "unlink":
		req, err := linkRequest(projectID, input, false)
		if err != nil {
			return Result{}, err
		}
		edge, warnings, err := call.Graph.Unlink(ctx, req)
		return result("unlink", map[string]any{"edge": edge, "warnings": warnings}, err)
	default:
		return Result{}, fmt.Errorf("graph_query: unsupported action %q", input.Action)
	}
}

func searchContexts(ctx context.Context, graph Service, projectID string, nodes []domain.GraphNodeSummary, depth int) ([]domain.GraphNodeDetail, []domain.GraphWarning, error) {
	if len(nodes) == 0 {
		return []domain.GraphNodeDetail{}, []domain.GraphWarning{}, nil
	}
	depth = normalizedSearchContextDepth(depth)
	contexts := make([]domain.GraphNodeDetail, 0, len(nodes))
	var warnings []domain.GraphWarning
	for _, node := range nodes {
		detail, nodeWarnings, err := graph.Node(ctx, projectID, node.Type, node.ID, depth)
		if err != nil {
			return []domain.GraphNodeDetail{}, []domain.GraphWarning{}, err
		}
		contexts = append(contexts, detail)
		warnings = append(warnings, nodeWarnings...)
	}
	if warnings == nil {
		warnings = []domain.GraphWarning{}
	}
	return contexts, warnings, nil
}

func normalizedSearchContextDepth(depth int) int {
	if depth <= 0 {
		return 0
	}
	if depth > maxSearchContextDepth {
		return maxSearchContextDepth
	}
	return depth
}

func (h Handler) contextWithActor(ctx context.Context, call CallContext) (context.Context, error) {
	memberID := strings.TrimSpace(call.ActorMemberID)
	if memberID == "" {
		return nil, fmt.Errorf("graph_query: caller member is required")
	}
	projectID := strings.TrimSpace(call.ProjectID)
	if projectID == "" {
		return nil, fmt.Errorf("graph_query: project id is required")
	}
	if call.Members == nil {
		return nil, fmt.Errorf("graph_query: member service is not configured")
	}
	ctx = caller.ContextWithCaller(ctx, caller.Caller{
		MemberID:  member.ID(memberID),
		ProjectID: types.ProjectID(projectID),
	})
	actor, err := call.Members.GetMember(ctx, member.ID(memberID))
	if err != nil {
		return nil, fmt.Errorf("graph_query: load caller member: %w", err)
	}
	if strings.TrimSpace(actor.LifecycleState) != member.LifecycleActive {
		return nil, fmt.Errorf("graph_query: caller member %q is not active", memberID)
	}
	return caller.ContextWithCaller(ctx, caller.Caller{
		UserID:    strings.TrimSpace(actor.UserID),
		MemberID:  actor.ID,
		ProjectID: types.ProjectID(projectID),
	}), nil
}

type rawRequest struct {
	Action       *string  `json:"action"`
	NodeType     *string  `json:"node_type"`
	NodeID       *string  `json:"node_id"`
	Depth        *int     `json:"depth"`
	Query        *string  `json:"query"`
	Limit        *int     `json:"limit"`
	HasEdge      *string  `json:"has_edge"`
	MissingEdge  *string  `json:"missing_edge"`
	OutgoingEdge *string  `json:"outgoing_edge"`
	IncomingEdge *string  `json:"incoming_edge"`
	SourceType   *string  `json:"source_type"`
	SourceID     *string  `json:"source_id"`
	TargetType   *string  `json:"target_type"`
	TargetID     *string  `json:"target_id"`
	EdgeID       *string  `json:"edge_id"`
	EdgeType     *string  `json:"edge_type"`
	Confidence   *float64 `json:"confidence"`
	Rationale    *string  `json:"rationale"`
}

type requestInput struct {
	Action       string
	NodeType     string
	NodeID       string
	Depth        int
	Query        string
	Limit        int
	HasEdge      string
	MissingEdge  string
	OutgoingEdge string
	IncomingEdge string
	SourceType   string
	SourceID     string
	TargetType   string
	TargetID     string
	EdgeID       string
	EdgeType     string
	Confidence   *float64
	Rationale    string
}

func decode(args json.RawMessage) (requestInput, error) {
	if len(args) == 0 {
		return requestInput{}, fmt.Errorf("graph_query: arguments are required")
	}
	if err := validateActionFields(args); err != nil {
		return requestInput{}, err
	}
	var raw rawRequest
	if err := json.Unmarshal(args, &raw); err != nil {
		return requestInput{}, fmt.Errorf("graph_query: decode arguments: %w", err)
	}
	action, err := requireString(raw.Action, "action")
	if err != nil {
		return requestInput{}, err
	}
	if !containsAction(action) {
		return requestInput{}, fmt.Errorf("graph_query: unsupported action %q", action)
	}
	out := requestInput{Action: action, Confidence: raw.Confidence}
	out.NodeType = optionalString(raw.NodeType)
	if err := validateOptionalEnum("node_type", out.NodeType, searchableNodeTypes); err != nil {
		return requestInput{}, err
	}
	out.NodeID = optionalString(raw.NodeID)
	out.Depth = optionalInt(raw.Depth)
	out.Query = optionalString(raw.Query)
	out.Limit = optionalInt(raw.Limit)
	out.HasEdge = optionalString(raw.HasEdge)
	if err := validateOptionalEnum("has_edge", out.HasEdge, edgeTypes); err != nil {
		return requestInput{}, err
	}
	out.MissingEdge = optionalString(raw.MissingEdge)
	if err := validateOptionalEnum("missing_edge", out.MissingEdge, edgeTypes); err != nil {
		return requestInput{}, err
	}
	out.OutgoingEdge = optionalString(raw.OutgoingEdge)
	if err := validateOptionalEnum("outgoing_edge", out.OutgoingEdge, edgeTypes); err != nil {
		return requestInput{}, err
	}
	out.IncomingEdge = optionalString(raw.IncomingEdge)
	if err := validateOptionalEnum("incoming_edge", out.IncomingEdge, edgeTypes); err != nil {
		return requestInput{}, err
	}
	out.SourceType = optionalString(raw.SourceType)
	if err := validateOptionalEnum("source_type", out.SourceType, linkableNodeTypes); err != nil {
		return requestInput{}, err
	}
	out.SourceID = optionalString(raw.SourceID)
	out.TargetType = optionalString(raw.TargetType)
	if err := validateOptionalEnum("target_type", out.TargetType, linkableNodeTypes); err != nil {
		return requestInput{}, err
	}
	out.TargetID = optionalString(raw.TargetID)
	out.EdgeID = optionalString(raw.EdgeID)
	out.EdgeType = optionalString(raw.EdgeType)
	if err := validateOptionalEnum("edge_type", out.EdgeType, edgeTypes); err != nil {
		return requestInput{}, err
	}
	out.Rationale = optionalString(raw.Rationale)
	return out, nil
}

func validateActionFields(args json.RawMessage) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(args, &fields); err != nil {
		return fmt.Errorf("graph_query: decode arguments: %w", err)
	}
	actionRaw, ok := fields["action"]
	if !ok || isJSONNull(actionRaw) {
		return fmt.Errorf("graph_query: action is required")
	}
	var action string
	if err := json.Unmarshal(actionRaw, &action); err != nil {
		return fmt.Errorf("graph_query: action must be a string")
	}
	action = strings.TrimSpace(action)
	allowed, ok := fieldsByAction[action]
	if !ok {
		return fmt.Errorf("graph_query: unsupported action %q", action)
	}
	for field, raw := range fields {
		if _, ok := allowed[field]; !ok {
			return fmt.Errorf("graph_query: field %q is not valid for action %q", field, action)
		}
		if isJSONNull(raw) {
			return fmt.Errorf("graph_query: field %q must be omitted instead of null", field)
		}
	}
	return nil
}

var fieldsByAction = map[string]map[string]struct{}{
	"node": fieldSet(
		"action",
		"node_type",
		"node_id",
		"depth",
	),
	"search": fieldSet(
		"action",
		"node_type",
		"depth",
		"query",
		"limit",
		"has_edge",
		"missing_edge",
		"outgoing_edge",
		"incoming_edge",
	),
	"link": fieldSet(
		"action",
		"source_type",
		"source_id",
		"target_type",
		"target_id",
		"edge_type",
		"confidence",
		"rationale",
	),
	"unlink": fieldSet(
		"action",
		"edge_id",
		"source_type",
		"source_id",
		"target_type",
		"target_id",
		"edge_type",
	),
}

func fieldSet(fields ...string) map[string]struct{} {
	out := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		out[field] = struct{}{}
	}
	return out
}

func isJSONNull(raw json.RawMessage) bool {
	return strings.TrimSpace(string(raw)) == "null"
}

func linkRequest(projectID string, input requestInput, includeRationale bool) (domain.GraphLinkRequest, error) {
	if !includeRationale && input.EdgeID != "" {
		if input.SourceType != "" || input.SourceID != "" || input.TargetType != "" || input.TargetID != "" || input.EdgeType != "" {
			return domain.GraphLinkRequest{}, fmt.Errorf("graph_query: edge_id unlink must omit source_type, source_id, target_type, target_id, and edge_type")
		}
		return domain.GraphLinkRequest{ProjectID: projectID, EdgeID: input.EdgeID}, nil
	}
	sourceID, err := requireString(&input.SourceID, "source_id")
	if err != nil {
		return domain.GraphLinkRequest{}, err
	}
	targetID, err := requireString(&input.TargetID, "target_id")
	if err != nil {
		return domain.GraphLinkRequest{}, err
	}
	edgeType, err := requireString(&input.EdgeType, "edge_type")
	if err != nil {
		return domain.GraphLinkRequest{}, err
	}
	req := domain.GraphLinkRequest{
		ProjectID:  projectID,
		Origin:     "manual",
		CreatedBy:  "graph_query",
		SourceType: input.SourceType,
		SourceID:   sourceID,
		TargetType: input.TargetType,
		TargetID:   targetID,
		EdgeType:   edgeType,
		Confidence: input.Confidence,
	}
	if includeRationale {
		req.Rationale = input.Rationale
	}
	return req, nil
}

func result(action string, fields map[string]any, err error) (Result, error) {
	if err != nil {
		return Result{}, err
	}
	structured := map[string]any{"ok": true, "tool": Name, "action": action}
	for key, value := range fields {
		structured[key] = value
	}
	text, err := encodeText(structured)
	if err != nil {
		return Result{}, err
	}
	return Result{Text: text, Structured: structured}, nil
}

func encodeText(value any) (string, error) {
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return "", fmt.Errorf("graph_query: encode result: %w", err)
	}
	return string(body), nil
}

func requireString(value *string, name string) (string, error) {
	if value == nil {
		return "", fmt.Errorf("graph_query: %s is required", name)
	}
	out := strings.TrimSpace(*value)
	if out == "" {
		return "", fmt.Errorf("graph_query: %s is required", name)
	}
	return out, nil
}

func requireValue(value string, name string) (string, error) {
	out := strings.TrimSpace(value)
	if out == "" {
		return "", fmt.Errorf("graph_query: %s is required", name)
	}
	return out, nil
}

func optionalString(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func optionalInt(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}

func stringSchema(description string) map[string]any {
	return map[string]any{"type": "string", "description": description}
}

func enumSchema(values []string, description string) map[string]any {
	return map[string]any{
		"type":        "string",
		"enum":        append([]string(nil), values...),
		"description": description + " Allowed values: " + strings.Join(values, ", ") + ".",
	}
}

func integerSchema(description string) map[string]any {
	return map[string]any{"type": "integer", "description": description}
}

func numberSchema(description string) map[string]any {
	return map[string]any{"type": "number", "description": description}
}

func containsAction(action string) bool {
	for _, allowed := range allActions {
		if action == allowed {
			return true
		}
	}
	return false
}

func validateOptionalEnum(field string, value string, allowed []string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	for _, candidate := range allowed {
		if value == candidate {
			return nil
		}
	}
	return fmt.Errorf("graph_query: %s must be one of: %s", field, strings.Join(allowed, ", "))
}
