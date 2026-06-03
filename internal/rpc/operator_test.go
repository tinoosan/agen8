package rpc

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	decisiondomain "github.com/tinoosan/agen8-mcp-server/internal/services/decision/domain"
	graphdomain "github.com/tinoosan/agen8-mcp-server/internal/services/graph/domain"
	operatorapp "github.com/tinoosan/agen8-mcp-server/internal/services/operator/app"
	operatordomain "github.com/tinoosan/agen8-mcp-server/internal/services/operator/domain"
	"github.com/tinoosan/agen8-mcp-server/pkg/protocol"
)

func TestRegisterOperatorDispatchCreateAction(t *testing.T) {
	svc := newRPCOperatorService(t)
	reg := NewRegistry()
	if err := RegisterOperator(reg, svc); err != nil {
		t.Fatalf("RegisterOperator: %v", err)
	}
	server, err := NewServer(reg)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	resp := dispatchOperatorRPC(t, server, protocol.MethodOpActionCreate, map[string]any{
		"projectId":   "project-1",
		"spaceId":     "space-1",
		"source":      "member",
		"memberId":    "member-1",
		"category":    "general",
		"urgency":     "low",
		"title":       "Check DNS change",
		"description": "Confirm the resolver update landed.",
	})
	if resp.Error != nil {
		t.Fatalf("opAction.create error=%+v", resp.Error)
	}
	var result protocol.OpActionCreateResult
	decodeResult(t, resp, &result)
	if result.OpAction.ID == "" {
		t.Fatal("opAction.create returned empty action id")
	}
	if result.OpAction.Title != "Check DNS change" || result.OpAction.Status != "pending" {
		t.Fatalf("opAction.create result=%+v", result.OpAction)
	}
}

func TestRegisterOperatorCreateActionMapsInvalidParams(t *testing.T) {
	svc := newRPCOperatorService(t)
	reg := NewRegistry()
	if err := RegisterOperator(reg, svc); err != nil {
		t.Fatalf("RegisterOperator: %v", err)
	}
	server, err := NewServer(reg)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	resp := dispatchOperatorRPC(t, server, protocol.MethodOpActionCreate, map[string]any{
		"projectId": "project-1",
	})
	if resp.Error == nil || resp.Error.Code != CodeInvalidParams {
		t.Fatalf("opAction.create error=%+v want invalid params", resp.Error)
	}
	if !strings.Contains(resp.Error.Message, "title is required") {
		t.Fatalf("opAction.create message=%q", resp.Error.Message)
	}
}

func dispatchOperatorRPC(t *testing.T, server *Server, method string, params map[string]any) Response {
	t.Helper()
	rawParams, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}
	raw, err := server.Handle(context.Background(), []byte(fmt.Sprintf(`{"jsonrpc":"2.0","id":"1","method":%q,"params":%s}`, method, rawParams)))
	if err != nil {
		t.Fatalf("Handle %s: %v", method, err)
	}
	return decodeRPCResponse(t, raw)
}

func decodeResult(t *testing.T, resp Response, out any) {
	t.Helper()
	if err := json.Unmarshal(resp.Result, out); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
}

func errNotFound(kind string, id string) error {
	return fmt.Errorf("%s not found: %s", kind, id)
}

func newRPCOperatorService(t *testing.T) *operatorapp.Service {
	t.Helper()
	repo := newRPCOperatorRepo()
	svc, err := operatorapp.NewService(
		repo,
		repo,
		rpcOperatorTaskPort{},
		rpcOperatorGraphPort{},
		rpcOperatorEventPublisher{},
		rpcOperatorDecisionPort{},
		rpcOperatorMessagePublisher{},
		rpcOperatorMissionRefs{},
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	if err != nil {
		t.Fatalf("operator NewService: %v", err)
	}
	return svc
}

type rpcOperatorRepo struct {
	actions     map[operatordomain.OperatorActionID]operatordomain.OperatorAction
	escalations map[operatordomain.EscalationID]operatordomain.Escalation
}

func newRPCOperatorRepo() *rpcOperatorRepo {
	return &rpcOperatorRepo{
		actions:     map[operatordomain.OperatorActionID]operatordomain.OperatorAction{},
		escalations: map[operatordomain.EscalationID]operatordomain.Escalation{},
	}
}

func (r *rpcOperatorRepo) SaveAction(_ context.Context, action operatordomain.OperatorAction) error {
	r.actions[action.ID] = action
	return nil
}

func (r *rpcOperatorRepo) GetAction(_ context.Context, id operatordomain.OperatorActionID) (operatordomain.OperatorAction, error) {
	action, ok := r.actions[id]
	if !ok {
		return operatordomain.OperatorAction{}, errNotFound("operator action", string(id))
	}
	return action, nil
}

func (r *rpcOperatorRepo) FindActionsByProject(_ context.Context, projectID string, _ operatordomain.ActionFilter) ([]operatordomain.OperatorAction, error) {
	out := []operatordomain.OperatorAction{}
	for _, action := range r.actions {
		if action.ProjectID == projectID {
			out = append(out, action)
		}
	}
	return out, nil
}

func (r *rpcOperatorRepo) FindActionsByTask(_ context.Context, taskRef string) ([]operatordomain.OperatorAction, error) {
	out := []operatordomain.OperatorAction{}
	for _, action := range r.actions {
		if action.TaskRef == taskRef {
			out = append(out, action)
		}
	}
	return out, nil
}

func (r *rpcOperatorRepo) FindPendingActions(_ context.Context, projectID string) ([]operatordomain.OperatorAction, error) {
	return r.FindActionsByProject(context.Background(), projectID, operatordomain.ActionFilter{})
}

func (r *rpcOperatorRepo) CountActionsByStatus(_ context.Context, projectID string) (map[operatordomain.OAStatus]int, error) {
	counts := map[operatordomain.OAStatus]int{}
	for _, action := range r.actions {
		if action.ProjectID == projectID {
			counts[action.Status]++
		}
	}
	return counts, nil
}

func (r *rpcOperatorRepo) FindActionByAttachmentID(_ context.Context, attachmentID string) (operatordomain.OperatorAction, error) {
	return operatordomain.OperatorAction{}, errNotFound("attachment", attachmentID)
}

func (r *rpcOperatorRepo) SaveEscalation(_ context.Context, esc operatordomain.Escalation) error {
	r.escalations[esc.ID] = esc
	return nil
}

func (r *rpcOperatorRepo) GetEscalation(_ context.Context, id operatordomain.EscalationID) (operatordomain.Escalation, error) {
	esc, ok := r.escalations[id]
	if !ok {
		return operatordomain.Escalation{}, errNotFound("escalation", string(id))
	}
	return esc, nil
}

func (r *rpcOperatorRepo) FindEscalationsByProject(_ context.Context, projectID string, _ operatordomain.EscalationFilter) ([]operatordomain.Escalation, error) {
	out := []operatordomain.Escalation{}
	for _, esc := range r.escalations {
		if esc.ProjectID == projectID {
			out = append(out, esc)
		}
	}
	return out, nil
}

func (r *rpcOperatorRepo) FindPendingEscalationsByTask(_ context.Context, taskRef string) ([]operatordomain.Escalation, error) {
	out := []operatordomain.Escalation{}
	for _, esc := range r.escalations {
		if esc.TaskRef == taskRef && esc.Status == operatordomain.StatusPending {
			out = append(out, esc)
		}
	}
	return out, nil
}

func (r *rpcOperatorRepo) CountPendingEscalations(_ context.Context, projectID string) (int, error) {
	count := 0
	for _, esc := range r.escalations {
		if esc.ProjectID == projectID && esc.Status == operatordomain.StatusPending {
			count++
		}
	}
	return count, nil
}

func (r *rpcOperatorRepo) UpdateEscalationStatus(_ context.Context, id operatordomain.EscalationID, status operatordomain.Status, resolution operatordomain.Resolution, note string, resolvedBy string) error {
	esc, ok := r.escalations[id]
	if !ok {
		return errNotFound("escalation", string(id))
	}
	esc.Status = status
	esc.Resolution = resolution
	esc.ResolutionNote = note
	esc.ResolvedBy = resolvedBy
	r.escalations[id] = esc
	return nil
}

func (r *rpcOperatorRepo) FindExpiredPendingEscalations(_ context.Context, _ time.Time) ([]operatordomain.Escalation, error) {
	return nil, nil
}

func (r *rpcOperatorRepo) EscalateEscalationUrgency(_ context.Context, id operatordomain.EscalationID, newUrgency operatordomain.Urgency, originalUrgency operatordomain.Urgency, escalatedAt time.Time) error {
	esc, ok := r.escalations[id]
	if !ok {
		return errNotFound("escalation", string(id))
	}
	esc.Urgency = newUrgency
	esc.OriginalUrgency = originalUrgency
	esc.EscalatedAt = &escalatedAt
	r.escalations[id] = esc
	return nil
}

func (r *rpcOperatorRepo) FindPendingEscalationDuplicate(_ context.Context, spaceID, taskRef string, category operatordomain.Category, urgency operatordomain.Urgency, _ time.Time) (operatordomain.Escalation, bool, error) {
	for _, esc := range r.escalations {
		if esc.SpaceID == spaceID && esc.TaskRef == taskRef && esc.Category == category && esc.Urgency == urgency && esc.Status == operatordomain.StatusPending {
			return esc, true, nil
		}
	}
	return operatordomain.Escalation{}, false, nil
}

type rpcOperatorTaskPort struct{}

func (rpcOperatorTaskPort) BlockTask(context.Context, string, string) error   { return nil }
func (rpcOperatorTaskPort) UnblockTask(context.Context, string, string) error { return nil }
func (rpcOperatorTaskPort) GetTaskKeyResultRef(context.Context, string) (string, error) {
	return "", nil
}

type rpcOperatorGraphPort struct{}

func (rpcOperatorGraphPort) Link(context.Context, graphdomain.GraphLinkRequest) (graphdomain.GraphEdge, []graphdomain.GraphWarning, error) {
	return graphdomain.GraphEdge{}, nil, nil
}

type rpcOperatorEventPublisher struct{}

func (rpcOperatorEventPublisher) PublishOperatorEvent(context.Context, any) error { return nil }

type rpcOperatorDecisionPort struct{}

func (rpcOperatorDecisionPort) CreateDecision(_ context.Context, decision decisiondomain.Decision) (decisiondomain.DecisionID, error) {
	if decision.ID != "" {
		return decision.ID, nil
	}
	return "dec-rpc-operator", nil
}

type rpcOperatorMessagePublisher struct{}

func (rpcOperatorMessagePublisher) ResolveEscalation(context.Context, operatordomain.Escalation, operatorapp.ResolveEscalationParams) error {
	return nil
}

func (rpcOperatorMessagePublisher) CompleteAction(context.Context, operatordomain.OperatorAction) error {
	return nil
}

func (rpcOperatorMessagePublisher) CommentOnAction(context.Context, operatordomain.OperatorAction, operatordomain.Comment) error {
	return nil
}

func (rpcOperatorMessagePublisher) BlockAction(context.Context, operatordomain.OperatorAction, string) error {
	return nil
}

type rpcOperatorMissionRefs struct{}

func (rpcOperatorMissionRefs) GetMissionFromKeyResult(context.Context, string) (string, error) {
	return "", nil
}

func (rpcOperatorMissionRefs) ValidateMission(context.Context, string) error { return nil }
