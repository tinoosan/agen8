package decision

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	decisionapp "github.com/tinoosan/agen8-mcp-server/internal/services/decision/app"
	humaninput "github.com/tinoosan/agen8-mcp-server/internal/services/humaninput/domain"
)

type stubDecisionService struct {
	logReq    decisionapp.LogRequest
	askReq    decisionapp.AskUserRequest
	askResult humaninput.QuestionsResult
}

func (s *stubDecisionService) Log(_ context.Context, req decisionapp.LogRequest) (decisionapp.Result, error) {
	s.logReq = req
	return decisionapp.Result{
		ID:           "dec-1",
		Kind:         "log",
		Title:        req.Title,
		TaskRef:      req.TaskRef,
		KeyResultRef: req.KeyResultRef,
		MemberID:     req.MemberID,
		SourceType:   "agent",
	}, nil
}

func (s *stubDecisionService) CompleteAskUser(_ context.Context, req decisionapp.AskUserRequest, result humaninput.QuestionsResult) (decisionapp.Result, error) {
	s.askReq = req
	s.askResult = result
	return decisionapp.Result{
		ID:        "dec-ask-1",
		Kind:      "ask_user",
		Title:     req.Title,
		Cancelled: result.Cancelled,
		Answers:   append([]humaninput.Answer(nil), result.Answers...),
		MemberID:  req.MemberID,
	}, nil
}

func TestHandle_LogRecordsDecision(t *testing.T) {
	service := &stubDecisionService{}
	result, err := NewHandler().Handle(context.Background(), CallContext{
		Decisions:     service,
		ProjectID:     "proj-1",
		SpaceID:       "space-1",
		ActorMemberID: "member-1",
	}, json.RawMessage(`{"action":"log","title":"Use Redis","rationale":"faster reads","confidence":0.9,"task_ref":"task-1","key_result_ref":"kr-1"}`))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if service.logReq.ProjectID != "proj-1" || service.logReq.MemberID != "member-1" {
		t.Fatalf("log req identity=%+v", service.logReq)
	}
	if service.logReq.Title != "Use Redis" || service.logReq.Rationale != "faster reads" {
		t.Fatalf("log req=%+v", service.logReq)
	}
	if !strings.Contains(result.Text, `"decision"`) {
		t.Fatalf("result text=%s", result.Text)
	}
}

func TestHandle_AskUserRequiresResolvedResult(t *testing.T) {
	_, err := NewHandler().Handle(context.Background(), CallContext{
		Decisions:     &stubDecisionService{},
		ProjectID:     "proj-1",
		SpaceID:       "space-1",
		ActorMemberID: "member-1",
	}, json.RawMessage(`{"action":"ask_user","title":"Choose one","questions":[{"id":"q1","text":"Which?","type":"multiple_choice","options":["A","B"]}]}`))
	if err == nil || !strings.Contains(err.Error(), "requires resolved answers") {
		t.Fatalf("err=%v want resolved answers error", err)
	}
}

func TestHandle_AskUserRecordsResolvedAnswer(t *testing.T) {
	service := &stubDecisionService{}
	answer := `{"questionId":"q1","selectedOption":"A"}`
	result, err := NewHandler().Handle(context.Background(), CallContext{
		Decisions:     service,
		ProjectID:     "proj-1",
		SpaceID:       "space-1",
		ActorMemberID: "member-1",
	}, json.RawMessage(`{"action":"ask_user","title":"Choose one","questions":[{"id":"q1","text":"Which?","type":"multiple_choice","options":["A","B"]}],"answers":[`+answer+`]}`))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if service.askReq.Title != "Choose one" || len(service.askReq.Questions) != 1 {
		t.Fatalf("ask req=%+v", service.askReq)
	}
	if len(service.askResult.Answers) != 1 || service.askResult.Answers[0].QuestionID != "q1" {
		t.Fatalf("ask result=%+v", service.askResult)
	}
	if !strings.Contains(result.Text, `"ask_user"`) {
		t.Fatalf("result text=%s", result.Text)
	}
}

func TestDeclareHumanInputBuildsQuestionsDeclaration(t *testing.T) {
	declaration, ok, err := NewHandler().DeclareHumanInput(context.Background(), json.RawMessage(`{"action":"ask_user","title":"Choose one","context":"Need input","questions":[{"id":"q1","text":"Which?","type":"multiple_choice","options":["A","B"]}]}`))
	if err != nil {
		t.Fatalf("DeclareHumanInput: %v", err)
	}
	if !ok {
		t.Fatal("expected human input declaration")
	}
	if declaration.Kind != humaninput.PrimitiveQuestions {
		t.Fatalf("kind=%q", declaration.Kind)
	}
	var payload humaninput.QuestionsPayload
	if err := json.Unmarshal(declaration.Payload, &payload); err != nil {
		t.Fatalf("payload: %v", err)
	}
	if payload.Title != "Choose one" || len(payload.Questions) != 1 {
		t.Fatalf("payload=%+v", payload)
	}
}

func TestHandle_ListActionIsNotSupported(t *testing.T) {
	_, err := NewHandler().Handle(context.Background(), CallContext{
		Decisions:     &stubDecisionService{},
		ProjectID:     "proj-1",
		SpaceID:       "space-1",
		ActorMemberID: "member-1",
	}, json.RawMessage(`{"action":"list"}`))
	if err == nil || !strings.Contains(err.Error(), `unsupported action "list"`) {
		t.Fatalf("err=%v want unsupported list action", err)
	}
}

func TestDecodeRejectsFieldFromOtherAction(t *testing.T) {
	_, err := decode(json.RawMessage(`{"action":"log","title":"Pick","rationale":"clear","questions":[{"id":"q1","text":"Which?","type":"text"}]}`))
	if err == nil || !strings.Contains(err.Error(), `field "questions" is not valid for action "log"`) {
		t.Fatalf("err=%v want action field error", err)
	}
}

func TestDecodeRejectsNullFieldCutover(t *testing.T) {
	_, err := decode(json.RawMessage(`{"action":"log","title":"Pick","rationale":"clear","task_ref":null}`))
	if err == nil || !strings.Contains(err.Error(), `field "task_ref" must be omitted instead of null`) {
		t.Fatalf("err=%v want null field error", err)
	}
}

func TestSchemaRequiresOnlyActionAtTopLevel(t *testing.T) {
	var schema struct {
		Type                 string                     `json:"type"`
		Properties           map[string]json.RawMessage `json:"properties"`
		Required             []string                   `json:"required"`
		AdditionalProperties bool                       `json:"additionalProperties"`
	}
	if err := json.Unmarshal(NewHandler().Schema(), &schema); err != nil {
		t.Fatalf("schema: %v", err)
	}
	if schema.Type != "object" {
		t.Fatalf("type=%q want object", schema.Type)
	}
	if schema.Properties["action"] == nil {
		t.Fatal("schema missing action")
	}
	if schema.Properties["questions"] == nil {
		t.Fatal("schema missing questions")
	}
	assertRequiredExactly(t, schema.Required, []string{"action"})
	if schema.AdditionalProperties {
		t.Fatal("schema should reject additional properties")
	}
}

func assertRequiredExactly(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("required=%v want %v", got, want)
	}
	seen := make(map[string]bool, len(got))
	for _, field := range got {
		seen[field] = true
	}
	for _, field := range want {
		if !seen[field] {
			t.Fatalf("required=%v missing %q", got, field)
		}
	}
}
