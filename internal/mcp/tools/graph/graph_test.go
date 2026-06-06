package graph

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/tinoosan/agen8-mcp-server/internal/caller"
	"github.com/tinoosan/agen8-mcp-server/internal/services/graph/domain"
	"github.com/tinoosan/agen8-mcp-server/internal/services/project/domain/member"
)

type stubGraphService struct {
	nodeProjectID  string
	nodeType       string
	nodeID         string
	nodeDepth      int
	nodeCalls      int
	searchReq      domain.GraphSearchRequest
	linkReq        domain.GraphLinkRequest
	searchNodes    []domain.GraphNodeSummary
	searchWarnings []domain.GraphWarning
	nodeWarnings   []domain.GraphWarning
}

func (s *stubGraphService) Node(_ context.Context, projectID, nodeType, nodeID string, depth int) (domain.GraphNodeDetail, []domain.GraphWarning, error) {
	s.nodeProjectID = projectID
	s.nodeType = nodeType
	s.nodeID = nodeID
	s.nodeDepth = depth
	s.nodeCalls++
	return domain.GraphNodeDetail{
		ID:    nodeID,
		Type:  nodeType,
		Title: "Launch",
		Neighbours: []domain.GraphNodeSummary{
			{ID: "kr-1", Type: "key_result", Title: "Stabilize"},
		},
		Edges: []domain.GraphEdge{
			{SourceType: nodeType, SourceID: nodeID, TargetType: "key_result", TargetID: "kr-1", EdgeType: "serves"},
		},
	}, s.nodeWarnings, nil
}

func (s *stubGraphService) Search(_ context.Context, req domain.GraphSearchRequest) ([]domain.GraphNodeSummary, []domain.GraphWarning, error) {
	s.searchReq = req
	nodes := s.searchNodes
	if nodes == nil {
		nodes = []domain.GraphNodeSummary{{ID: "task-1", Type: "task", Title: "Ship"}}
	}
	warnings := s.searchWarnings
	if warnings == nil {
		warnings = []domain.GraphWarning{}
	}
	return nodes, warnings, nil
}

func (s *stubGraphService) Link(_ context.Context, req domain.GraphLinkRequest) (domain.GraphEdge, []domain.GraphWarning, error) {
	s.linkReq = req
	return domain.GraphEdge{SourceType: req.SourceType, SourceID: req.SourceID, TargetType: req.TargetType, TargetID: req.TargetID, EdgeType: req.EdgeType}, []domain.GraphWarning{}, nil
}

func (s *stubGraphService) Unlink(_ context.Context, req domain.GraphLinkRequest) (domain.GraphEdge, []domain.GraphWarning, error) {
	s.linkReq = req
	return domain.GraphEdge{SourceType: req.SourceType, SourceID: req.SourceID, TargetType: req.TargetType, TargetID: req.TargetID, EdgeType: req.EdgeType}, []domain.GraphWarning{}, nil
}

type stubMembers struct{}

func (stubMembers) GetMember(_ context.Context, id member.ID) (member.Record, error) {
	return member.Record{
		ID:             id,
		UserID:         "user-1",
		ProjectID:      "proj-1",
		LifecycleState: member.LifecycleActive,
	}, nil
}

type callerRequiredMembers struct {
	seen caller.Caller
}

func (m *callerRequiredMembers) GetMember(ctx context.Context, id member.ID) (member.Record, error) {
	seen, err := (caller.ContextResolver{}).ResolveCaller(ctx)
	if err != nil {
		return member.Record{}, err
	}
	m.seen = seen
	return member.Record{
		ID:             id,
		UserID:         "user-1",
		ProjectID:      string(seen.ProjectID),
		LifecycleState: member.LifecycleActive,
	}, nil
}

// membersFunc adapts a function to the members lookup port. stubMembers always
// returns an active record and never errors, so it cannot drive the inactive or
// load-error branches of contextWithActor(); tests that need those shapes use
// this instead.
type membersFunc func(context.Context, member.ID) (member.Record, error)

func (f membersFunc) GetMember(ctx context.Context, id member.ID) (member.Record, error) {
	return f(ctx, id)
}

func graphCallContext(svc *stubGraphService) CallContext {
	return CallContext{
		Graph:         svc,
		Members:       stubMembers{},
		ProjectID:     "proj-1",
		ActorMemberID: "member-1",
	}
}

func TestHandleNodeReadsGraphNode(t *testing.T) {
	service := &stubGraphService{}
	result, err := NewHandler().Handle(context.Background(), graphCallContext(service), json.RawMessage(`{"action":"node","node_type":"task","node_id":"task-1","depth":1}`))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if service.nodeProjectID != "proj-1" || service.nodeType != "task" || service.nodeID != "task-1" {
		t.Fatalf("node call=%+v", service)
	}
	if !strings.Contains(result.Text, `"node"`) {
		t.Fatalf("result text=%s", result.Text)
	}
}

func TestHandleRequiresNodeID(t *testing.T) {
	_, err := NewHandler().Handle(context.Background(), graphCallContext(&stubGraphService{}), json.RawMessage(`{"action":"node","node_type":"task"}`))
	if err == nil || !strings.Contains(err.Error(), "node_id is required") {
		t.Fatalf("err=%v want node_id required", err)
	}
}

func TestHandleLinkAllowsOmittedEndpointTypes(t *testing.T) {
	service := &stubGraphService{}
	_, err := NewHandler().Handle(context.Background(), graphCallContext(service), json.RawMessage(`{"action":"link","source_id":"task-1","target_id":"kr-1","edge_type":"serves","rationale":"Task serves KR."}`))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if service.linkReq.SourceType != "" || service.linkReq.TargetType != "" {
		t.Fatalf("handler should leave omitted endpoint types for service inference: %+v", service.linkReq)
	}
	if service.linkReq.SourceID != "task-1" || service.linkReq.TargetID != "kr-1" || service.linkReq.EdgeType != "serves" {
		t.Fatalf("link request=%+v", service.linkReq)
	}
}

func TestHandleUnlinkAcceptsEdgeID(t *testing.T) {
	service := &stubGraphService{}
	_, err := NewHandler().Handle(context.Background(), graphCallContext(service), json.RawMessage(`{"action":"unlink","edge_id":"cl-graph-1"}`))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if service.linkReq.EdgeID != "cl-graph-1" {
		t.Fatalf("edge id=%q", service.linkReq.EdgeID)
	}
	if service.linkReq.SourceID != "" || service.linkReq.TargetID != "" || service.linkReq.EdgeType != "" {
		t.Fatalf("edge_id unlink should not require endpoint tuple: %+v", service.linkReq)
	}
}

func TestHandleUnlinkRejectsEdgeIDMixedWithEndpointTuple(t *testing.T) {
	_, err := NewHandler().Handle(context.Background(), graphCallContext(&stubGraphService{}), json.RawMessage(`{"action":"unlink","edge_id":"cl-graph-1","source_id":"task-1","target_id":"kr-1","edge_type":"serves"}`))
	if err == nil || !strings.Contains(err.Error(), "edge_id unlink must omit") {
		t.Fatalf("err=%v want mixed edge_id error", err)
	}
}

func TestSchemaExposesNodeAndEdgeEnums(t *testing.T) {
	var schema struct {
		Properties map[string]struct {
			Enum []string `json:"enum"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(NewHandler().Schema(), &schema); err != nil {
		t.Fatalf("unmarshal schema: %v", err)
	}
	assertEnumContains(t, schema.Properties["node_type"].Enum, "mission")
	assertEnumContains(t, schema.Properties["node_type"].Enum, "all")
	assertEnumContains(t, schema.Properties["source_type"].Enum, "mission")
	assertEnumMissing(t, schema.Properties["source_type"].Enum, "all")
	assertEnumContains(t, schema.Properties["edge_type"].Enum, "informed_by")
	assertEnumContains(t, schema.Properties["outgoing_edge"].Enum, "serves")
	assertEnumContains(t, schema.Properties["incoming_edge"].Enum, "completed_by")
}

func TestDecodeRejectsNullField(t *testing.T) {
	_, err := decode(json.RawMessage(`{"action":"search","node_type":"task","query":null}`))
	if err == nil || !strings.Contains(err.Error(), `field "query" must be omitted instead of null`) {
		t.Fatalf("err=%v want null field error", err)
	}
}

func TestDecodeRejectsFieldFromOtherAction(t *testing.T) {
	_, err := decode(json.RawMessage(`{"action":"node","node_id":"task-1","query":"ship"}`))
	if err == nil || !strings.Contains(err.Error(), `field "query" is not valid for action "node"`) {
		t.Fatalf("err=%v want action field error", err)
	}
}

func TestDecodeAllowsDepthForSearch(t *testing.T) {
	input, err := decode(json.RawMessage(`{"action":"search","node_type":"task","query":"ship","depth":1}`))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if input.Depth != 1 {
		t.Fatalf("depth=%d want 1", input.Depth)
	}
}

func TestDecodeRejectsMissingAction(t *testing.T) {
	_, err := decode(json.RawMessage(`{"node_id":"task-1"}`))
	if err == nil || !strings.Contains(err.Error(), "action is required") {
		t.Fatalf("err=%v want action required", err)
	}
}

func TestDecodeRejectsNonStringAction(t *testing.T) {
	_, err := decode(json.RawMessage(`{"action":123}`))
	if err == nil || !strings.Contains(err.Error(), "action must be a string") {
		t.Fatalf("err=%v want action must be a string", err)
	}
}

func TestDecodeRejectsUnsupportedAction(t *testing.T) {
	_, err := decode(json.RawMessage(`{"action":"approve"}`))
	if err == nil || !strings.Contains(err.Error(), `unsupported action "approve"`) {
		t.Fatalf("err=%v want unsupported action", err)
	}
}

// TestDecodeRejectsMalformedJSON pins graph_query's malformed-JSON wording.
// graph_query now matches the other action tools (mission/task/project/decision/http),
// all of which emit "invalid arguments". The prior "decode arguments" divergence was
// normalized in the MCP decode error-contract cleanup mission (see dec-a4181ffe).
func TestDecodeRejectsMalformedJSON(t *testing.T) {
	_, err := decode(json.RawMessage(`{"action":"search"`))
	if err == nil || !strings.Contains(err.Error(), "invalid arguments") {
		t.Fatalf("err=%v want invalid arguments", err)
	}
}

func TestHandleRejectsUnknownNodeTypeBeforeServiceCall(t *testing.T) {
	service := &stubGraphService{}
	_, err := NewHandler().Handle(context.Background(), graphCallContext(service), json.RawMessage(`{"action":"search","node_type":"goal","query":"ship","limit":10}`))
	if err == nil || !strings.Contains(err.Error(), "node_type must be one of") {
		t.Fatalf("err=%v want node_type enum error", err)
	}
	if service.searchReq.ProjectID != "" {
		t.Fatalf("service was called: %+v", service.searchReq)
	}
}

func TestHandleRejectsAllAsLinkSourceType(t *testing.T) {
	_, err := NewHandler().Handle(context.Background(), graphCallContext(&stubGraphService{}), json.RawMessage(`{"action":"link","source_type":"all","source_id":"a","target_type":"mission","target_id":"m","edge_type":"relates_to"}`))
	if err == nil || !strings.Contains(err.Error(), "source_type must be one of") {
		t.Fatalf("err=%v want source_type enum error", err)
	}
}

func TestHandleStampsCallerBeforeLoadingMember(t *testing.T) {
	service := &stubGraphService{}
	members := &callerRequiredMembers{}
	call := CallContext{
		Graph:         service,
		Members:       members,
		ProjectID:     "proj-1",
		ActorMemberID: "member-1",
	}
	_, err := NewHandler().Handle(context.Background(), call, json.RawMessage(`{"action":"search","node_type":"task","query":"ship","limit":10}`))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if members.seen.MemberID != member.ID("member-1") || string(members.seen.ProjectID) != "proj-1" {
		t.Fatalf("caller=%+v", members.seen)
	}
}

// TestHandleRejectsMemberlessCaller locks the loud-failure half of the
// pre-registration affordance: the daemon resolves an unregistered session to a
// member-LESS session (no member, no error). A read-only verb like graph_query must
// still reject that caller loudly rather than reading the graph for nobody. The
// member check runs first in contextWithActor, so the service is never reached.
func TestHandleRejectsMemberlessCaller(t *testing.T) {
	service := &stubGraphService{}
	call := graphCallContext(service)
	call.ActorMemberID = ""
	_, err := NewHandler().Handle(context.Background(), call, json.RawMessage(`{"action":"search","node_type":"task","query":"ship","limit":10}`))
	if err == nil || !strings.Contains(err.Error(), "graph_query: caller member is required") {
		t.Fatalf("err=%v want graph_query caller member required", err)
	}
	if service.searchReq.ProjectID != "" {
		t.Fatalf("graph service ran despite member-less caller: %+v", service.searchReq)
	}
}

// contextWithActor runs before any graph read. Beyond the member-less case, it
// must reject a caller it cannot load and a caller that is no longer active —
// graph_query is read-only, but reading the graph "for nobody" or for a removed
// member is still an authorization failure. Both tests assert the graph service
// is never reached (searchReq stays zero).

func TestHandleRejectsInactiveCaller(t *testing.T) {
	service := &stubGraphService{}
	call := graphCallContext(service)
	call.Members = membersFunc(func(_ context.Context, id member.ID) (member.Record, error) {
		return member.Record{ID: id, ProjectID: "proj-1", LifecycleState: member.LifecycleRemoved}, nil
	})
	_, err := NewHandler().Handle(context.Background(), call, json.RawMessage(`{"action":"search","node_type":"task","query":"ship","limit":10}`))
	if err == nil || !strings.Contains(err.Error(), `caller member "member-1" is not active`) {
		t.Fatalf("err=%v want inactive caller", err)
	}
	if service.searchReq.ProjectID != "" {
		t.Fatalf("graph service ran despite inactive caller: %+v", service.searchReq)
	}
}

func TestHandleRejectsCallerLoadError(t *testing.T) {
	service := &stubGraphService{}
	call := graphCallContext(service)
	call.Members = membersFunc(func(context.Context, member.ID) (member.Record, error) {
		return member.Record{}, member.ErrNotFound
	})
	_, err := NewHandler().Handle(context.Background(), call, json.RawMessage(`{"action":"search","node_type":"task","query":"ship","limit":10}`))
	if err == nil || !strings.Contains(err.Error(), "load caller member") {
		t.Fatalf("err=%v want load caller member", err)
	}
	if !errors.Is(err, member.ErrNotFound) {
		t.Fatalf("error does not wrap member.ErrNotFound: %v", err)
	}
	if service.searchReq.ProjectID != "" {
		t.Fatalf("graph service ran despite caller load error: %+v", service.searchReq)
	}
}

func TestHandleSearchReturnsNodesPayload(t *testing.T) {
	service := &stubGraphService{}
	result, err := NewHandler().Handle(context.Background(), graphCallContext(service), json.RawMessage(`{"action":"search","node_type":"all","query":"ship","limit":10}`))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	structured, ok := result.Structured.(map[string]any)
	if !ok {
		t.Fatalf("structured=%T", result.Structured)
	}
	if structured["results"] != nil {
		t.Fatalf("search payload must use backend nodes key, not results: %+v", structured)
	}
	nodes, ok := structured["nodes"].([]domain.GraphNodeSummary)
	if !ok {
		t.Fatalf("nodes=%T %+v", structured["nodes"], structured["nodes"])
	}
	if len(nodes) != 1 || nodes[0].ID != "task-1" {
		t.Fatalf("nodes=%+v", nodes)
	}
	if service.nodeCalls != 0 {
		t.Fatalf("search without depth should not fetch node context, calls=%d", service.nodeCalls)
	}
	if structured["contexts"] != nil {
		t.Fatalf("search without depth should stay compact: %+v", structured)
	}
	if !strings.Contains(result.Text, `"nodes"`) || strings.Contains(result.Text, `"results"`) {
		t.Fatalf("result text=%s", result.Text)
	}
}

func TestHandleSearchDepthReturnsBoundedContexts(t *testing.T) {
	service := &stubGraphService{}
	result, err := NewHandler().Handle(context.Background(), graphCallContext(service), json.RawMessage(`{"action":"search","node_type":"all","query":"ship","limit":10,"depth":1}`))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if service.nodeCalls != 1 || service.nodeDepth != 1 || service.nodeID != "task-1" {
		t.Fatalf("node context call count=%d depth=%d node=%q", service.nodeCalls, service.nodeDepth, service.nodeID)
	}
	structured, ok := result.Structured.(map[string]any)
	if !ok {
		t.Fatalf("structured=%T", result.Structured)
	}
	contexts, ok := structured["contexts"].([]domain.GraphNodeDetail)
	if !ok {
		t.Fatalf("contexts=%T %+v", structured["contexts"], structured["contexts"])
	}
	if len(contexts) != 1 || contexts[0].ID != "task-1" || len(contexts[0].Neighbours) != 1 || len(contexts[0].Edges) != 1 {
		t.Fatalf("contexts=%+v", contexts)
	}
	if structured["depth"] != 1 {
		t.Fatalf("depth=%v want 1", structured["depth"])
	}
	if !strings.Contains(result.Text, `"contexts"`) {
		t.Fatalf("result text=%s", result.Text)
	}
}

func TestHandleSearchDepthDeduplicatesRepeatedWarnings(t *testing.T) {
	service := &stubGraphService{
		searchNodes: []domain.GraphNodeSummary{
			{ID: "task-1", Type: "task", Title: "Ship"},
			{ID: "task-2", Type: "task", Title: "Review"},
		},
		searchWarnings: []domain.GraphWarning{
			{
				Code:          "hydrator_partial",
				Message:       "Task hydrator returned partial records.",
				AffectedType:  "task",
				AffectedIDs:   []string{"task-search"},
				AffectedCount: 1,
			},
		},
		nodeWarnings: []domain.GraphWarning{
			{
				Code:          "neighbours_truncated",
				Message:       "Neighbour list truncated to 50 entries",
				AffectedType:  "",
				AffectedIDs:   []string{"task-a", "task-b"},
				AffectedCount: 2,
			},
			{
				Code:          "neighbours_truncated",
				Message:       "Neighbour list truncated to 50 entries",
				AffectedType:  "",
				AffectedIDs:   []string{"task-b", "task-c"},
				AffectedCount: 2,
			},
		},
	}
	result, err := NewHandler().Handle(context.Background(), graphCallContext(service), json.RawMessage(`{"action":"search","node_type":"all","query":"ship","limit":10,"depth":1}`))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if service.nodeCalls != 2 {
		t.Fatalf("node calls=%d want 2", service.nodeCalls)
	}
	structured := result.Structured.(map[string]any)
	warnings, ok := structured["warnings"].([]domain.GraphWarning)
	if !ok {
		t.Fatalf("warnings=%T %+v", structured["warnings"], structured["warnings"])
	}
	if len(warnings) != 2 {
		t.Fatalf("warnings len=%d warnings=%+v", len(warnings), warnings)
	}
	truncated := warnings[1]
	if truncated.Code != "neighbours_truncated" {
		t.Fatalf("warning order/code=%+v", warnings)
	}
	if truncated.AffectedCount != 3 {
		t.Fatalf("affected count=%d warning=%+v", truncated.AffectedCount, truncated)
	}
	wantIDs := []string{"task-a", "task-b", "task-c"}
	if strings.Join(truncated.AffectedIDs, ",") != strings.Join(wantIDs, ",") {
		t.Fatalf("affected ids=%+v want %+v", truncated.AffectedIDs, wantIDs)
	}
}

func TestHandleSearchDepthIsCapped(t *testing.T) {
	service := &stubGraphService{}
	result, err := NewHandler().Handle(context.Background(), graphCallContext(service), json.RawMessage(`{"action":"search","node_type":"all","query":"ship","limit":10,"depth":99}`))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if service.nodeDepth != maxSearchContextDepth {
		t.Fatalf("node depth=%d want %d", service.nodeDepth, maxSearchContextDepth)
	}
	structured := result.Structured.(map[string]any)
	if structured["depth"] != maxSearchContextDepth {
		t.Fatalf("structured depth=%v want %d", structured["depth"], maxSearchContextDepth)
	}
}

func assertEnumContains(t *testing.T, values []string, value string) {
	t.Helper()
	if !enumHasValue(values, value) {
		t.Fatalf("enum missing %q in %+v", value, values)
	}
}

func assertEnumMissing(t *testing.T, values []string, value string) {
	t.Helper()
	if enumHasValue(values, value) {
		t.Fatalf("enum unexpectedly includes %q in %+v", value, values)
	}
}

func enumHasValue(values []string, value string) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}
