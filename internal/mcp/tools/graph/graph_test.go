package graph

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/tinoosan/agen8-mcp-server/internal/caller"
	"github.com/tinoosan/agen8-mcp-server/internal/services/graph/domain"
	spacedomain "github.com/tinoosan/agen8-mcp-server/internal/services/space/domain"
	"github.com/tinoosan/agen8-mcp-server/internal/services/space/domain/member"
)

type stubGraphService struct {
	nodeProjectID string
	nodeType      string
	nodeID        string
	searchReq     domain.GraphSearchRequest
	linkReq       domain.GraphLinkRequest
}

func (s *stubGraphService) Node(_ context.Context, projectID, nodeType, nodeID string, _ int) (domain.GraphNodeDetail, []domain.GraphWarning, error) {
	s.nodeProjectID = projectID
	s.nodeType = nodeType
	s.nodeID = nodeID
	return domain.GraphNodeDetail{ID: nodeID, Type: nodeType, Title: "Launch"}, []domain.GraphWarning{}, nil
}

func (s *stubGraphService) Search(_ context.Context, req domain.GraphSearchRequest) ([]domain.GraphNodeSummary, []domain.GraphWarning, error) {
	s.searchReq = req
	return []domain.GraphNodeSummary{{ID: "task-1", Type: "task", Title: "Ship"}}, []domain.GraphWarning{}, nil
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
		SpaceID:        "space-1",
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
		SpaceID:        string(seen.SpaceID),
		LifecycleState: member.LifecycleActive,
	}, nil
}

func graphCallContext(svc *stubGraphService) CallContext {
	return CallContext{
		Graph:         svc,
		Members:       stubMembers{},
		ProjectID:     "proj-1",
		SpaceID:       "space-1",
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
		SpaceID:       "space-1",
		ActorMemberID: "member-1",
	}
	_, err := NewHandler().Handle(context.Background(), call, json.RawMessage(`{"action":"search","node_type":"task","query":"ship","limit":10}`))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if members.seen.MemberID != member.ID("member-1") || members.seen.SpaceID != spacedomain.SpaceID("space-1") {
		t.Fatalf("caller=%+v", members.seen)
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
	if !strings.Contains(result.Text, `"nodes"`) || strings.Contains(result.Text, `"results"`) {
		t.Fatalf("result text=%s", result.Text)
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
