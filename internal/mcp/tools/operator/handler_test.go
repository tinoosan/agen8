package operator

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	operatorapp "github.com/tinoosan/agen8-mcp-server/internal/services/operator/app"
	operatordomain "github.com/tinoosan/agen8-mcp-server/internal/services/operator/domain"
)

type stubOperatorService struct {
	createReq     operatordomain.CreateParams
	escalationReq operatorapp.CreateEscalationParams
}

func (s *stubOperatorService) Create(_ context.Context, req operatordomain.CreateParams) (operatordomain.OperatorAction, error) {
	s.createReq = req
	return operatordomain.OperatorAction{
		ID:                   "oa-1",
		ProjectID:            req.ProjectID,
		SpaceID:              req.SpaceID,
		TaskRef:              req.TaskRef,
		KeyResultRef:         req.KeyResultRef,
		MissionRef:           req.MissionRef,
		Blocking:             req.Blocking,
		RequiresVerification: req.RequiresVerification,
		Status:               operatordomain.OAStatusPending,
		Title:                req.Title,
		CreatedAt:            time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC),
	}, nil
}

func (s *stubOperatorService) CreateEscalation(_ context.Context, req operatorapp.CreateEscalationParams) (operatordomain.Escalation, error) {
	s.escalationReq = req
	return operatordomain.Escalation{
		ID:             "esc-1",
		ProjectID:      req.ProjectID,
		SpaceID:        req.SpaceID,
		TaskRef:        req.TaskRef,
		KeyResultRef:   req.KeyResultRef,
		MissionRef:     req.MissionRef,
		Status:         operatordomain.StatusPending,
		Title:          req.Title,
		Recommendation: req.Recommendation,
		Confidence:     req.Confidence,
		CreatedAt:      time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC),
	}, nil
}

func TestHandleRequestCreatesOperatorAction(t *testing.T) {
	service := &stubOperatorService{}
	result, err := NewHandler().Handle(context.Background(), CallContext{
		Operator:      service,
		ProjectID:     "proj-1",
		SpaceID:       "space-1",
		ActorMemberID: "member-1",
	}, json.RawMessage(`{"action":"request","title":"Rotate token","description":"Rotate the stale API token","category":"general","urgency":"high","task_ref":"task-1","blocking":true,"requires_verification":true}`))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if service.createReq.ProjectID != "proj-1" || service.createReq.SpaceID != "space-1" || service.createReq.MemberID != "member-1" {
		t.Fatalf("identity=%+v", service.createReq)
	}
	if service.createReq.Title != "Rotate token" || !service.createReq.Blocking || !service.createReq.RequiresVerification {
		t.Fatalf("create req=%+v", service.createReq)
	}
	if !strings.Contains(result.Text, `"operator"`) {
		t.Fatalf("result text=%s", result.Text)
	}
}

func TestHandleEscalateCreatesEscalation(t *testing.T) {
	service := &stubOperatorService{}
	_, err := NewHandler().Handle(context.Background(), CallContext{
		Operator:      service,
		ProjectID:     "proj-1",
		SpaceID:       "space-1",
		ActorMemberID: "member-1",
	}, json.RawMessage(`{"action":"escalate","title":"Approve resolver fix","description":"Need operator approval","recommendation":"Approve the resolver patch","category":"code","urgency":"critical","confidence":0.8,"mission_ref":"mission-1"}`))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if service.escalationReq.ProjectID != "proj-1" || service.escalationReq.MemberID != "member-1" {
		t.Fatalf("identity=%+v", service.escalationReq)
	}
	if service.escalationReq.Recommendation != "Approve the resolver patch" || service.escalationReq.Confidence != 0.8 {
		t.Fatalf("escalation req=%+v", service.escalationReq)
	}
}

func TestHandleRequiresTitle(t *testing.T) {
	_, err := NewHandler().Handle(context.Background(), CallContext{
		Operator:      &stubOperatorService{},
		ProjectID:     "proj-1",
		ActorMemberID: "member-1",
	}, json.RawMessage(`{"action":"request","description":"missing title"}`))
	if err == nil || !strings.Contains(err.Error(), "title is required") {
		t.Fatalf("err=%v want title error", err)
	}
}

func TestDecodeRejectsNullField(t *testing.T) {
	_, err := decode(json.RawMessage(`{"action":"request","title":"Rotate token","description":"Rotate it","category":"general","urgency":"high","run_id":null}`))
	if err == nil || !strings.Contains(err.Error(), `field "run_id" must be omitted instead of null`) {
		t.Fatalf("err=%v want null field error", err)
	}
}

func TestDecodeRejectsFieldFromOtherAction(t *testing.T) {
	_, err := decode(json.RawMessage(`{"action":"request","title":"Rotate token","description":"Rotate it","category":"general","urgency":"high","recommendation":"approve"}`))
	if err == nil || !strings.Contains(err.Error(), `field "recommendation" is not valid for action "request"`) {
		t.Fatalf("err=%v want action field error", err)
	}
}

func TestHandleRejectsNonStringMetadataValues(t *testing.T) {
	_, err := NewHandler().Handle(context.Background(), CallContext{
		Operator:      &stubOperatorService{},
		ProjectID:     "proj-1",
		ActorMemberID: "member-1",
	}, json.RawMessage(`{"action":"request","title":"Rotate token","description":"Rotate it","category":"general","urgency":"high","metadata":{"test":true}}`))
	if err == nil || !strings.Contains(err.Error(), `metadata value for "test" must be a string`) {
		t.Fatalf("err=%v want metadata string-value error", err)
	}
}

func TestSchemaAdvertisesStringMetadataValues(t *testing.T) {
	var schema map[string]any
	if err := json.Unmarshal(NewHandler().Schema(), &schema); err != nil {
		t.Fatalf("decode schema: %v", err)
	}
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("schema properties=%T", schema["properties"])
	}
	metadata, ok := properties["metadata"].(map[string]any)
	if !ok {
		t.Fatalf("metadata schema=%T", properties["metadata"])
	}
	if metadata["type"] != "object" {
		t.Fatalf("metadata type=%v want object", metadata["type"])
	}
	required := schema["required"].([]any)
	assertRequiredFields(t, required, []string{"action", "title", "description", "category", "urgency"})
	if _, ok := properties["title"].(map[string]any)["anyOf"]; ok {
		t.Fatalf("title should not be nullable: %+v", properties["title"])
	}
	additional, ok := metadata["additionalProperties"].(map[string]any)
	if !ok || additional["type"] != "string" {
		t.Fatalf("metadata additionalProperties=%v want string schema", metadata["additionalProperties"])
	}
}

func assertRequiredFields(t *testing.T, got []any, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("required=%v want %v", got, want)
	}
	seen := make(map[string]bool, len(got))
	for _, field := range got {
		seen[field.(string)] = true
	}
	for _, field := range want {
		if !seen[field] {
			t.Fatalf("required=%v missing %q", got, field)
		}
	}
}
