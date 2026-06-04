package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/tinoosan/agen8-mcp-server/internal/caller"
	decisionapp "github.com/tinoosan/agen8-mcp-server/internal/services/decision/app"
	humaninput "github.com/tinoosan/agen8-mcp-server/internal/services/humaninput/domain"
	messagedomain "github.com/tinoosan/agen8-mcp-server/internal/services/message/domain"
	missionapp "github.com/tinoosan/agen8-mcp-server/internal/services/mission/app"
	krdomain "github.com/tinoosan/agen8-mcp-server/internal/services/mission/domain/kr"
	missiondomain "github.com/tinoosan/agen8-mcp-server/internal/services/mission/domain/mission"
	operatorapp "github.com/tinoosan/agen8-mcp-server/internal/services/operator/app"
	operatordomain "github.com/tinoosan/agen8-mcp-server/internal/services/operator/domain"
	spaceapp "github.com/tinoosan/agen8-mcp-server/internal/services/space/app"
	spacedomain "github.com/tinoosan/agen8-mcp-server/internal/services/space/domain"
	"github.com/tinoosan/agen8-mcp-server/internal/services/space/domain/member"
	taskapp "github.com/tinoosan/agen8-mcp-server/internal/services/task/app"
	taskdomain "github.com/tinoosan/agen8-mcp-server/internal/services/task/domain"
	"github.com/tinoosan/agen8-mcp-server/pkg/types"
)

var fixedMCPTime = time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)

type testMCPJSONRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

type testMCPToolsListResult struct {
	Tools []struct {
		Name        string          `json:"name"`
		Description string          `json:"description,omitempty"`
		InputSchema json.RawMessage `json:"inputSchema,omitempty"`
	} `json:"tools"`
}

type testMCPToolCallResult struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text,omitempty"`
	} `json:"content"`
	StructuredContent json.RawMessage `json:"structuredContent,omitempty"`
	IsError           bool            `json:"isError,omitempty"`
}

func TestMCPSessionRefsExtractsCodexThreadMetadata(t *testing.T) {
	req := &sdkmcp.CallToolRequest{Params: &sdkmcp.CallToolParamsRaw{
		Meta: sdkmcp.Meta{
			"progressToken": 42,
			"threadId":      "thread-top",
			"x-codex-turn-metadata": map[string]any{
				"session_id": "session-nested",
				"thread_id":  "thread-nested",
			},
		},
	}}

	sessionID, threadID := mcpSessionRefs(req)
	if sessionID != "session-nested" {
		t.Fatalf("sessionID=%q want session-nested", sessionID)
	}
	if threadID != "thread-top" {
		t.Fatalf("threadID=%q want thread-top", threadID)
	}
}

func TestMCPSessionRefsFallsBackToNestedCodexThreadMetadata(t *testing.T) {
	req := &sdkmcp.CallToolRequest{Params: &sdkmcp.CallToolParamsRaw{
		Meta: sdkmcp.Meta{
			"x-codex-turn-metadata": map[string]any{
				"session_id": "session-nested",
				"thread_id":  "thread-nested",
			},
		},
	}}

	sessionID, threadID := mcpSessionRefs(req)
	if sessionID != "session-nested" {
		t.Fatalf("sessionID=%q want session-nested", sessionID)
	}
	if threadID != "thread-nested" {
		t.Fatalf("threadID=%q want thread-nested", threadID)
	}
}

func TestSessionRefsFromJSONRPCBodyExtractsCodexMetadata(t *testing.T) {
	sessionID, threadID := SessionRefsFromJSONRPCBody([]byte(`{
		"jsonrpc": "2.0",
		"id": "1",
		"method": "tools/call",
		"params": {
			"name": "task",
			"_meta": {
				"x-codex-turn-metadata": {
					"session_id": "session-nested",
					"thread_id": "thread-nested"
				}
			},
			"arguments": {"action": "list"}
		}
	}`))
	if sessionID != "session-nested" {
		t.Fatalf("sessionID=%q want session-nested", sessionID)
	}
	if threadID != "thread-nested" {
		t.Fatalf("threadID=%q want thread-nested", threadID)
	}
}

func TestSessionRefsFromHTTPHeaderExtractsMCPSessionID(t *testing.T) {
	header := http.Header{}
	header.Set("Mcp-Session-Id", "claude-session-1")

	sessionID, threadID := SessionRefsFromHTTPHeader(header)
	if sessionID != "claude-session-1" {
		t.Fatalf("sessionID=%q want claude-session-1", sessionID)
	}
	if threadID != "" {
		t.Fatalf("threadID=%q want empty", threadID)
	}
}

type stubMCPTaskService struct{}

func (stubMCPTaskService) Create(context.Context, taskapp.CreateTaskParams) (taskdomain.Task, error) {
	return taskdomain.Task{}, nil
}
func (stubMCPTaskService) Get(context.Context, taskdomain.TaskID) (taskdomain.Task, error) {
	return taskdomain.Task{}, nil
}
func (stubMCPTaskService) List(_ context.Context, req taskdomain.TaskFilter) ([]taskdomain.Task, error) {
	return []taskdomain.Task{{ID: "task-1", SpaceID: req.SpaceID, AssignedTo: req.AssignedTo, Status: taskdomain.TaskStatusPending}}, nil
}
func (stubMCPTaskService) Claim(context.Context, taskdomain.TaskID) (taskdomain.Task, error) {
	return taskdomain.Task{}, nil
}
func (stubMCPTaskService) Release(context.Context, taskdomain.TaskID) (taskdomain.Task, error) {
	return taskdomain.Task{}, nil
}
func (stubMCPTaskService) Complete(context.Context, taskapp.CompleteTaskParams) (taskdomain.Task, error) {
	return taskdomain.Task{}, nil
}
func (stubMCPTaskService) Block(context.Context, taskdomain.TaskID, string) (taskdomain.Task, error) {
	return taskdomain.Task{}, nil
}
func (stubMCPTaskService) Unblock(context.Context, taskdomain.TaskID, string) (taskdomain.Task, error) {
	return taskdomain.Task{}, nil
}
func (stubMCPTaskService) Assign(context.Context, taskapp.AssignTaskParams) (taskdomain.Task, error) {
	return taskdomain.Task{}, nil
}
func (stubMCPTaskService) Cancel(context.Context, taskdomain.TaskID, string) (taskdomain.Task, error) {
	return taskdomain.Task{}, nil
}
func (stubMCPTaskService) ApproveReview(context.Context, taskapp.ReviewTaskParams) (taskdomain.Task, error) {
	return taskdomain.Task{}, nil
}
func (stubMCPTaskService) RetryReview(context.Context, taskapp.ReviewTaskParams) (taskdomain.Task, error) {
	return taskdomain.Task{}, nil
}
func (stubMCPTaskService) FailReview(context.Context, taskapp.ReviewTaskParams) (taskdomain.Task, error) {
	return taskdomain.Task{}, nil
}

type stubMCPMissionService struct{}

func (stubMCPMissionService) CreateMission(context.Context, missionapp.CreateMissionParams) (missiondomain.Mission, error) {
	return missiondomain.Mission{}, nil
}
func (stubMCPMissionService) GetMission(context.Context, missiondomain.MissionID) (missiondomain.Mission, error) {
	return missiondomain.Mission{}, nil
}
func (stubMCPMissionService) ListMissions(_ context.Context, projectID string, filter missiondomain.MissionFilter) ([]missiondomain.Mission, error) {
	return []missiondomain.Mission{{ID: "mission-1", ProjectID: projectID, Title: "Mission", Status: missiondomain.MissionStatusActive, CreatedAt: fixedMCPTime, UpdatedAt: fixedMCPTime}}, nil
}
func (stubMCPMissionService) UpdateMission(context.Context, missionapp.UpdateMissionParams) (missiondomain.Mission, error) {
	return missiondomain.Mission{}, nil
}
func (stubMCPMissionService) DeleteMission(context.Context, missionapp.DeleteMissionParams) (missiondomain.Mission, error) {
	return missiondomain.Mission{}, nil
}
func (stubMCPMissionService) GetLifecycleHistory(context.Context, missiondomain.MissionID, missionapp.LifecycleHistoryFilter) (missionapp.LifecycleHistory, error) {
	return missionapp.LifecycleHistory{}, nil
}
func (stubMCPMissionService) CreateKeyResult(context.Context, missionapp.CreateKeyResultParams) (krdomain.KeyResult, error) {
	return krdomain.KeyResult{}, nil
}
func (stubMCPMissionService) GetKeyResult(context.Context, krdomain.KeyResultID) (krdomain.KeyResult, error) {
	return krdomain.KeyResult{}, nil
}
func (stubMCPMissionService) ListKeyResults(context.Context, missiondomain.MissionID) ([]krdomain.KeyResult, error) {
	return nil, nil
}
func (stubMCPMissionService) UpdateKeyResult(context.Context, missionapp.UpdateKeyResultParams) (krdomain.KeyResult, error) {
	return krdomain.KeyResult{}, nil
}
func (stubMCPMissionService) AssignKeyResultSpace(context.Context, krdomain.KeyResultID, string) (krdomain.KeyResult, error) {
	return krdomain.KeyResult{}, nil
}
func (stubMCPMissionService) DeleteKeyResult(context.Context, missionapp.DeleteKeyResultParams) (krdomain.KeyResult, error) {
	return krdomain.KeyResult{}, nil
}
func (stubMCPMissionService) ReopenKeyResult(context.Context, missionapp.ReopenKeyResultParams) (krdomain.KeyResult, error) {
	return krdomain.KeyResult{}, nil
}
func (stubMCPMissionService) UpdateProgress(context.Context, missionapp.UpdateProgressParams) (krdomain.KeyResult, error) {
	return krdomain.KeyResult{}, nil
}
func (stubMCPMissionService) GetProgressHistory(context.Context, krdomain.KeyResultID) ([]krdomain.ProgressEntry, error) {
	return nil, nil
}
func (stubMCPMissionService) ComputeMissionProgress(context.Context, missiondomain.MissionID) (missionapp.MissionProgress, error) {
	return missionapp.MissionProgress{}, nil
}

type stubMCPSpaceReader struct {
	getFn  func(context.Context, spacedomain.SpaceID) (spacedomain.SpaceRecord, error)
	listFn func(context.Context, spacedomain.SpaceFilter) ([]spacedomain.SpaceRecord, error)
}

func (s stubMCPSpaceReader) Get(ctx context.Context, id spacedomain.SpaceID) (spacedomain.SpaceRecord, error) {
	if s.getFn != nil {
		return s.getFn(ctx, id)
	}
	return spacedomain.SpaceRecord{ID: id, Title: "Runtime", Status: spacedomain.SpaceStatusOpen}, nil
}

func (s stubMCPSpaceReader) List(ctx context.Context, filter spacedomain.SpaceFilter) ([]spacedomain.SpaceRecord, error) {
	if s.listFn != nil {
		return s.listFn(ctx, filter)
	}
	return nil, nil
}

type stubMCPMemberDirectory struct {
	getFn  func(context.Context, member.ID) (member.Record, error)
	listFn func(context.Context, member.Filter) ([]member.Record, error)
}

func (s stubMCPMemberDirectory) GetMember(ctx context.Context, id member.ID) (member.Record, error) {
	if s.getFn != nil {
		return s.getFn(ctx, id)
	}
	return member.Record{ID: id, SpaceID: "space-session", DisplayName: string(id), MemberType: member.TypeWorker, LifecycleState: member.LifecycleActive}, nil
}

func (s stubMCPMemberDirectory) ListMembers(ctx context.Context, filter member.Filter) ([]member.Record, error) {
	if s.listFn != nil {
		return s.listFn(ctx, filter)
	}
	return nil, nil
}

type stubMCPMemberRegistrar struct {
	registerFn     func(context.Context, member.Record) (spaceapp.RegisterMemberResult, error)
	updateConfigFn func(context.Context, member.ID, string, string, string, ...string) (member.Record, error)
	removeFn       func(context.Context, member.ID) (member.Record, error)
}

func (s stubMCPMemberRegistrar) RegisterMember(ctx context.Context, rosterMember member.Record) (spaceapp.RegisterMemberResult, error) {
	if s.registerFn != nil {
		return s.registerFn(ctx, rosterMember)
	}
	rosterMember.ID = "member-created"
	rosterMember.ChannelID = "channel:" + rosterMember.SpaceID + ":member:member-created"
	rosterMember.LifecycleState = member.LifecycleActive
	return spaceapp.RegisterMemberResult{Member: rosterMember, GrantedMemberType: rosterMember.MemberType}, nil
}

func (s stubMCPMemberRegistrar) UpdateMemberConfig(ctx context.Context, id member.ID, model, effort, harnessKind string, permissionFields ...string) (member.Record, error) {
	if s.updateConfigFn != nil {
		return s.updateConfigFn(ctx, id, model, effort, harnessKind, permissionFields...)
	}
	return member.Record{ID: id, Model: model, Effort: effort, HarnessKind: harnessKind, LifecycleState: member.LifecycleActive}, nil
}

func (s stubMCPMemberRegistrar) RemoveMember(ctx context.Context, id member.ID) (member.Record, error) {
	if s.removeFn != nil {
		return s.removeFn(ctx, id)
	}
	return member.Record{ID: id, LifecycleState: member.LifecycleRemoved}, nil
}

type stubMCPMessagePublisher struct {
	publishFn func(context.Context, messagedomain.NewMessageInput) (types.AgentMessage, error)
	listFn    func(context.Context, messagedomain.MessageFilter) ([]types.AgentMessage, error)
	wakes     <-chan messagedomain.MessageWake
}

type stubMCPDecisionService struct{}

func (stubMCPDecisionService) Log(_ context.Context, req decisionapp.LogRequest) (decisionapp.Result, error) {
	return decisionapp.Result{ID: "dec-1", Kind: "log", Title: req.Title, MemberID: req.MemberID, SourceType: "agent"}, nil
}

func (stubMCPDecisionService) CompleteAskUser(_ context.Context, req decisionapp.AskUserRequest, result humaninput.QuestionsResult) (decisionapp.Result, error) {
	return decisionapp.Result{ID: "dec-ask-1", Kind: "ask_user", Title: req.Title, Cancelled: result.Cancelled, Answers: result.Answers, MemberID: req.MemberID, SourceType: "agent"}, nil
}

type stubMCPOperatorService struct {
	createReq operatordomain.CreateParams
}

func (s *stubMCPOperatorService) Create(_ context.Context, req operatordomain.CreateParams) (operatordomain.OperatorAction, error) {
	s.createReq = req
	return operatordomain.OperatorAction{
		ID:        "oa-mcp-1",
		ProjectID: req.ProjectID,
		SpaceID:   req.SpaceID,
		MemberID:  req.MemberID,
		Title:     req.Title,
		Status:    operatordomain.OAStatusPending,
		CreatedAt: fixedMCPTime,
	}, nil
}

func (s *stubMCPOperatorService) CreateEscalation(_ context.Context, req operatorapp.CreateEscalationParams) (operatordomain.Escalation, error) {
	return operatordomain.Escalation{
		ID:        "esc-mcp-1",
		ProjectID: req.ProjectID,
		SpaceID:   req.SpaceID,
		MemberID:  req.MemberID,
		Title:     req.Title,
		Status:    operatordomain.StatusPending,
		CreatedAt: fixedMCPTime,
	}, nil
}

type stubHumanInputAwaiter struct {
	fn func(context.Context, humaninput.PendingRequest) (json.RawMessage, error)
}

func (s stubHumanInputAwaiter) Await(ctx context.Context, req humaninput.PendingRequest) (json.RawMessage, error) {
	return s.fn(ctx, req)
}

func (s stubMCPMessagePublisher) PublishAgentMessage(ctx context.Context, input messagedomain.NewMessageInput) (types.AgentMessage, error) {
	if s.publishFn != nil {
		return s.publishFn(ctx, input)
	}
	msg, err := messagedomain.NewMessage(input, fixedMCPTime)
	if err != nil {
		return types.AgentMessage{}, err
	}
	return msg.Inner(), nil
}

func (s stubMCPMessagePublisher) ListMessages(ctx context.Context, filter messagedomain.MessageFilter) ([]types.AgentMessage, error) {
	if s.listFn != nil {
		return s.listFn(ctx, filter)
	}
	return nil, nil
}

func (s stubMCPMessagePublisher) SubscribeMemberWake(member.ID) (<-chan messagedomain.MessageWake, func()) {
	if s.wakes == nil {
		return nil, func() {}
	}
	return s.wakes, func() {}
}

func TestTokenStore_RegisterResolveRevoke(t *testing.T) {
	store := NewTokenStore()
	store.Register("token-1", Session{
		ChannelID: types.ChannelID("channel:test"),
	})

	sess, err := store.Resolve("token-1")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if sess.ChannelID != "channel:test" {
		t.Fatalf("channelID=%q", sess.ChannelID)
	}

	store.Revoke("token-1")
	if _, err := store.Resolve("token-1"); err == nil {
		t.Fatal("expected resolve-after-revoke error")
	}
}

func TestServer_ToolsListReturnsNativeTools(t *testing.T) {
	store := NewTokenStore()
	store.Register("token-list", Session{})
	server, err := NewServer(store)
	if err != nil {
		t.Fatalf("new server: %v", err)
	}

	resp := postMCPRequest(t, server.Handler(), "token-list", map[string]any{
		"jsonrpc": "2.0",
		"id":      "1",
		"method":  "tools/list",
	})
	if resp.Error != nil {
		t.Fatalf("unexpected rpc error: %+v", resp.Error)
	}
	var result testMCPToolsListResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("decode tools/list result: %v", err)
	}
	expectedDefs, err := buildToolDefs()
	if err != nil {
		t.Fatalf("buildToolDefs: %v", err)
	}
	expectedCount := 0
	for _, def := range expectedDefs {
		if !def.native.internal {
			expectedCount++
		}
	}
	if len(result.Tools) != expectedCount {
		t.Fatalf("tools/list count=%d want=%d", len(result.Tools), expectedCount)
	}
	found := map[string]bool{}
	for _, tool := range result.Tools {
		normalized := strings.ToLower(strings.TrimSpace(tool.Name))
		if normalized == "browser" {
			t.Fatalf("tools/list exposed forbidden tool %q", tool.Name)
		}
		if normalized == "harness_approval" {
			t.Fatalf("tools/list exposed internal tool %q to generic session", tool.Name)
		}
		found[tool.Name] = true
	}
	for _, def := range expectedDefs {
		if def.native.internal {
			continue
		}
		if !found[def.name()] {
			t.Fatalf("tools/list missing %q", def.name())
		}
	}
}

func TestServer_ToolsListIncludesHarnessApprovalForClaudeSession(t *testing.T) {
	store := NewTokenStore()
	store.Register("token-list-claude", Session{HarnessKind: "claude-cli"})
	server, err := NewServer(store)
	if err != nil {
		t.Fatalf("new server: %v", err)
	}

	resp := postMCPRequest(t, server.Handler(), "token-list-claude", map[string]any{
		"jsonrpc": "2.0",
		"id":      "1",
		"method":  "tools/list",
	})
	if resp.Error != nil {
		t.Fatalf("unexpected rpc error: %+v", resp.Error)
	}
	var result testMCPToolsListResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("decode tools/list result: %v", err)
	}
	for _, tool := range result.Tools {
		if tool.Name == "harness_approval" {
			return
		}
	}
	t.Fatalf("tools/list missing harness_approval for claude session: %+v", result.Tools)
}

func TestServer_ToolsCallExecutesNativeTaskHandler(t *testing.T) {
	store := NewTokenStore()
	store.Register("token-call", Session{
		SpaceID:     "space-session",
		MemberID:    "member-session",
		TaskService: stubMCPTaskService{},
		TaskMembers: stubMCPMemberDirectory{
			getFn: func(_ context.Context, id member.ID) (member.Record, error) {
				return member.Record{ID: id, SpaceID: "space-session", DisplayName: "Worker", MemberType: member.TypeWorker, LifecycleState: member.LifecycleActive}, nil
			},
		},
	})
	server, err := NewServer(store)
	if err != nil {
		t.Fatalf("new server: %v", err)
	}

	resp := postMCPRequest(t, server.Handler(), "token-call", map[string]any{
		"jsonrpc": "2.0",
		"id":      "2",
		"method":  "tools/call",
		"params": map[string]any{
			"name": "task",
			"arguments": map[string]any{
				"action": "list",
				"limit":  10,
			},
		},
	})
	if resp.Error != nil {
		t.Fatalf("unexpected rpc error: %+v", resp.Error)
	}
	var result testMCPToolCallResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("decode tools/call result: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success tool result: %+v", result)
	}
	if len(result.Content) == 0 || result.Content[0].Text == "" {
		t.Fatalf("missing tool content: %+v", result)
	}
	if result.Content[0].Type != "text" {
		t.Fatalf("content[0].type=%q", result.Content[0].Type)
	}
	if len(result.StructuredContent) == 0 {
		t.Fatalf("missing structuredContent: %+v", result)
	}
	var structured map[string]any
	if err := json.Unmarshal(result.StructuredContent, &structured); err != nil {
		t.Fatalf("decode structuredContent: %v", err)
	}
	if got := strings.TrimSpace(anyString(structured["tool"])); got != "task" {
		t.Fatalf("structuredContent.tool=%q want task", got)
	}
	if got := anyBool(structured["ok"]); !got {
		t.Fatalf("structuredContent.ok=%v want true", got)
	}
}

func TestServer_ToolsCallExecutesNativeDecisionHandler(t *testing.T) {
	store := NewTokenStore()
	store.Register("token-decision", Session{
		ProjectID:       "proj-1",
		SpaceID:         "space-session",
		MemberID:        "member-session",
		DecisionService: stubMCPDecisionService{},
	})
	server, err := NewServer(store)
	if err != nil {
		t.Fatalf("new server: %v", err)
	}

	resp := postMCPRequest(t, server.Handler(), "token-decision", map[string]any{
		"jsonrpc": "2.0",
		"id":      "decision-call",
		"method":  "tools/call",
		"params": map[string]any{
			"name": "decision",
			"arguments": map[string]any{
				"action":     "log",
				"title":      "Use Redis",
				"rationale":  "faster reads",
				"confidence": 0.85,
			},
		},
	})
	if resp.Error != nil {
		t.Fatalf("unexpected rpc error: %+v", resp.Error)
	}
	var result testMCPToolCallResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("decode tools/call result: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success tool result: %+v", result)
	}
	var structured map[string]any
	if err := json.Unmarshal(result.StructuredContent, &structured); err != nil {
		t.Fatalf("decode structuredContent: %v", err)
	}
	if got := strings.TrimSpace(anyString(structured["tool"])); got != "decision" {
		t.Fatalf("structuredContent.tool=%q want decision", got)
	}
	if got := anyBool(structured["ok"]); !got {
		t.Fatalf("structuredContent.ok=%v want true", got)
	}
}

func TestServer_DecisionAskUserUsesHumanInputAwaiter(t *testing.T) {
	store := NewTokenStore()
	var pending humaninput.PendingRequest
	store.Register("token-decision-human", Session{
		ProjectID:       "proj-1",
		SpaceID:         "space-session",
		MemberID:        "member-session",
		ChannelID:       "channel-session",
		DecisionService: stubMCPDecisionService{},
		HumanInputAwaiter: stubHumanInputAwaiter{fn: func(_ context.Context, req humaninput.PendingRequest) (json.RawMessage, error) {
			pending = req
			return json.RawMessage(`{"answers":[{"questionId":"q1","selectedOption":"A"}]}`), nil
		}},
	})
	server, err := NewServer(store)
	if err != nil {
		t.Fatalf("new server: %v", err)
	}

	resp := postMCPRequest(t, server.Handler(), "token-decision-human", map[string]any{
		"jsonrpc": "2.0",
		"id":      "decision-ask",
		"method":  "tools/call",
		"params": map[string]any{
			"name": "decision",
			"arguments": map[string]any{
				"action": "ask_user",
				"title":  "Choose one",
				"questions": []map[string]any{{
					"id":      "q1",
					"text":    "Which?",
					"type":    "multiple_choice",
					"options": []string{"A", "B"},
				}},
			},
		},
	})
	if resp.Error != nil {
		t.Fatalf("unexpected rpc error: %+v", resp.Error)
	}
	if pending.ToolName != "decision" || pending.ProjectID != "proj-1" || pending.ChannelID != "channel-session" {
		t.Fatalf("pending request=%+v", pending)
	}
	if !strings.HasPrefix(pending.ToolCallID, "mcp:decision:") || pending.IdempotencyKey != pending.ToolCallID {
		t.Fatalf("pending tool call identity not stable/idempotent: %+v", pending)
	}
	var result testMCPToolCallResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("decode tools/call result: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success tool result: %+v", result)
	}
	var structured map[string]any
	if err := json.Unmarshal(result.StructuredContent, &structured); err != nil {
		t.Fatalf("decode structuredContent: %v", err)
	}
	if got := strings.TrimSpace(anyString(structured["action"])); got != "ask_user" {
		t.Fatalf("structuredContent.action=%q want ask_user", got)
	}
}

func TestServer_HarnessApprovalUsesHumanInputAwaiter(t *testing.T) {
	store := NewTokenStore()
	var pending humaninput.PendingRequest
	store.Register("token-harness-approval", Session{
		ProjectID:   "proj-1",
		SpaceID:     "space-session",
		MemberID:    "member-session",
		ChannelID:   "channel-session",
		HarnessKind: "claude-cli",
		HumanInputAwaiter: stubHumanInputAwaiter{fn: func(_ context.Context, req humaninput.PendingRequest) (json.RawMessage, error) {
			pending = req
			return json.RawMessage(`{"decision":"approve"}`), nil
		}},
	})
	server, err := NewServer(store)
	if err != nil {
		t.Fatalf("new server: %v", err)
	}

	resp := postMCPRequest(t, server.Handler(), "token-harness-approval", map[string]any{
		"jsonrpc": "2.0",
		"id":      "approval-call",
		"method":  "tools/call",
		"params": map[string]any{
			"name": "harness_approval",
			"arguments": map[string]any{
				"tool_name":   "Bash",
				"tool_use_id": "toolu-1",
				"cwd":         "/repo",
				"input": map[string]any{
					"command": "rm -rf dist",
				},
			},
		},
	})
	if resp.Error != nil {
		t.Fatalf("unexpected rpc error: %+v", resp.Error)
	}
	if pending.ToolCallID != "toolu-1" || pending.ToolName != "harness_approval" || pending.ProjectID != "proj-1" || pending.ChannelID != "channel-session" {
		t.Fatalf("pending request=%+v", pending)
	}
	if pending.Declaration.Kind != humaninput.PrimitiveApproveReject {
		t.Fatalf("declaration kind=%q", pending.Declaration.Kind)
	}
	var payload humaninput.ApproveRejectPayload
	if err := json.Unmarshal(pending.Declaration.Payload, &payload); err != nil {
		t.Fatalf("decode approval payload: %v", err)
	}
	if !strings.Contains(payload.Description, "rm -rf dist") ||
		!strings.Contains(payload.Context, "cwd=/repo") ||
		!strings.Contains(payload.Context, "harness=claude-code") ||
		!strings.Contains(payload.Context, "memberId=member-session") ||
		!strings.Contains(payload.Context, "spaceId=space-session") ||
		!strings.Contains(payload.Context, "channelId=channel-session") {
		t.Fatalf("approval payload=%+v", payload)
	}
	var result testMCPToolCallResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("decode tools/call result: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success tool result: %+v", result)
	}
	if len(result.StructuredContent) != 0 {
		t.Fatalf("harness approval must return only text content for Claude permission parser: %s", result.StructuredContent)
	}
	if len(result.Content) != 1 || result.Content[0].Type != "text" {
		t.Fatalf("unexpected content blocks: %+v", result.Content)
	}
	var structured map[string]any
	if err := json.Unmarshal([]byte(result.Content[0].Text), &structured); err != nil {
		t.Fatalf("decode permission text result: %v", err)
	}
	if got := strings.TrimSpace(anyString(structured["behavior"])); got != "allow" {
		t.Fatalf("behavior=%q want allow", got)
	}
	if _, ok := structured["updatedInput"].(map[string]any); !ok {
		t.Fatalf("missing updatedInput: %+v", structured)
	}
}

func TestServer_HarnessApprovalAutoAllowsDecisionAskUser(t *testing.T) {
	store := NewTokenStore()
	called := false
	store.Register("token-harness-approval-auto", Session{
		ProjectID:   "proj-1",
		SpaceID:     "space-session",
		MemberID:    "member-session",
		ChannelID:   "channel-session",
		HarnessKind: "claude-cli",
		HumanInputAwaiter: stubHumanInputAwaiter{fn: func(context.Context, humaninput.PendingRequest) (json.RawMessage, error) {
			called = true
			return nil, nil
		}},
	})
	server, err := NewServer(store)
	if err != nil {
		t.Fatalf("new server: %v", err)
	}

	resp := postMCPRequest(t, server.Handler(), "token-harness-approval-auto", map[string]any{
		"jsonrpc": "2.0",
		"id":      "approval-call",
		"method":  "tools/call",
		"params": map[string]any{
			"name": "harness_approval",
			"arguments": map[string]any{
				"tool_name": "mcp__agen8__decision",
				"input": map[string]any{
					"action": "ask_user",
					"title":  "Choose one",
					"questions": []map[string]any{{
						"id":      "q1",
						"text":    "Which?",
						"type":    "multiple_choice",
						"options": []string{"A", "B"},
					}},
				},
			},
		},
	})
	if resp.Error != nil {
		t.Fatalf("unexpected rpc error: %+v", resp.Error)
	}
	if called {
		t.Fatal("approval awaiter must not be called for decision ask_user permission prompt")
	}
	var result testMCPToolCallResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("decode tools/call result: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success tool result: %+v", result)
	}
	if len(result.StructuredContent) != 0 {
		t.Fatalf("harness approval must return only text content for Claude permission parser: %s", result.StructuredContent)
	}
	if len(result.Content) != 1 || result.Content[0].Type != "text" {
		t.Fatalf("unexpected content blocks: %+v", result.Content)
	}
	var structured map[string]any
	if err := json.Unmarshal([]byte(result.Content[0].Text), &structured); err != nil {
		t.Fatalf("decode permission text result: %v", err)
	}
	if got := strings.TrimSpace(anyString(structured["behavior"])); got != "allow" {
		t.Fatalf("behavior=%q want allow", got)
	}
	updated, ok := structured["updatedInput"].(map[string]any)
	if !ok {
		t.Fatalf("missing updatedInput: %+v", structured)
	}
	if got := strings.TrimSpace(anyString(updated["action"])); got != "ask_user" {
		t.Fatalf("updatedInput.action=%q want ask_user", got)
	}
}

func TestServer_HarnessApprovalHiddenFromCodexSession(t *testing.T) {
	store := NewTokenStore()
	store.Register("token-harness-approval-codex", Session{
		ProjectID:   "proj-1",
		SpaceID:     "space-session",
		MemberID:    "member-session",
		ChannelID:   "channel-session",
		HarnessKind: "codex",
		HumanInputAwaiter: stubHumanInputAwaiter{fn: func(context.Context, humaninput.PendingRequest) (json.RawMessage, error) {
			t.Fatal("approval awaiter must not be called for codex-visible mcp tool call")
			return nil, nil
		}},
	})
	server, err := NewServer(store)
	if err != nil {
		t.Fatalf("new server: %v", err)
	}

	resp := postMCPRequest(t, server.Handler(), "token-harness-approval-codex", map[string]any{
		"jsonrpc": "2.0",
		"id":      "approval-call",
		"method":  "tools/call",
		"params": map[string]any{
			"name":      "harness_approval",
			"arguments": map[string]any{"tool_name": "Bash"},
		},
	})
	if resp.Error != nil {
		t.Fatalf("expected mcp result error, got rpc error: %+v", resp.Error)
	}
	var result testMCPToolCallResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("decode tools/call result: %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected hidden harness_approval to return error result: %+v", result)
	}
}

func TestServer_ToolsCallExecutesNativeMissionHandler(t *testing.T) {
	store := NewTokenStore()
	store.Register("token-mission", Session{
		ProjectID:       "project-1",
		SpaceID:         "space-session",
		MemberID:        "member-session",
		MissionService:  stubMCPMissionService{},
		MissionKRs:      stubMCPMissionService{},
		MissionProgress: stubMCPMissionService{},
		MemberDirectory: stubMCPMemberDirectory{
			getFn: func(_ context.Context, id member.ID) (member.Record, error) {
				return member.Record{ID: id, UserID: "user-1", SpaceID: "space-session", DisplayName: "Coordinator", MemberType: member.TypeCoordinator, LifecycleState: member.LifecycleActive}, nil
			},
		},
	})
	server, err := NewServer(store)
	if err != nil {
		t.Fatalf("new server: %v", err)
	}

	resp := postMCPRequest(t, server.Handler(), "token-mission", map[string]any{
		"jsonrpc": "2.0",
		"id":      "mission-1",
		"method":  "tools/call",
		"params": map[string]any{
			"name": "mission",
			"arguments": map[string]any{
				"action": "list",
				"limit":  10,
			},
		},
	})
	if resp.Error != nil {
		t.Fatalf("unexpected rpc error: %+v", resp.Error)
	}
	var result testMCPToolCallResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("decode tools/call result: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success tool result: %+v", result)
	}
	var structured map[string]any
	if err := json.Unmarshal(result.StructuredContent, &structured); err != nil {
		t.Fatalf("decode structuredContent: %v", err)
	}
	if structured["tool"] != "mission" || structured["action"] != "list" {
		t.Fatalf("structured=%v", structured)
	}
}

func TestServer_ToolsCallExecutesNativeOperatorHandler(t *testing.T) {
	operatorSvc := &stubMCPOperatorService{}
	store := NewTokenStore()
	store.Register("token-operator", Session{
		ProjectID:       "project-1",
		SpaceID:         "space-session",
		MemberID:        "member-session",
		OperatorService: operatorSvc,
	})
	server, err := NewServer(store)
	if err != nil {
		t.Fatalf("new server: %v", err)
	}

	resp := postMCPRequest(t, server.Handler(), "token-operator", map[string]any{
		"jsonrpc": "2.0",
		"id":      "operator-1",
		"method":  "tools/call",
		"params": map[string]any{
			"name": "operator",
			"arguments": map[string]any{
				"action":      "request",
				"title":       "Rotate token",
				"description": "Rotate the stale token",
				"category":    "general",
				"urgency":     "high",
			},
		},
	})
	if resp.Error != nil {
		t.Fatalf("unexpected rpc error: %+v", resp.Error)
	}
	var result testMCPToolCallResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("decode tools/call result: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success tool result: %+v", result)
	}
	if operatorSvc.createReq.ProjectID != "project-1" || operatorSvc.createReq.SpaceID != "space-session" || operatorSvc.createReq.MemberID != "member-session" {
		t.Fatalf("operator identity=%+v", operatorSvc.createReq)
	}
	var structured map[string]any
	if err := json.Unmarshal(result.StructuredContent, &structured); err != nil {
		t.Fatalf("decode structuredContent: %v", err)
	}
	if structured["tool"] != "operator" || structured["action"] != "request" {
		t.Fatalf("structured=%v", structured)
	}
}

func TestServer_ToolsCallExecutesNativeSpaceHandler(t *testing.T) {
	store := NewTokenStore()
	store.Register("token-space", Session{
		SpaceID:   "space-session",
		MemberID:  "member-session",
		ProjectID: "project-session",
		SpaceReader: stubMCPSpaceReader{
			getFn: func(_ context.Context, id spacedomain.SpaceID) (spacedomain.SpaceRecord, error) {
				if id != "space-session" {
					t.Fatalf("space id=%q want space-session", id)
				}
				return spacedomain.SpaceRecord{
					ID:        id,
					ProjectID: "project-session",
					Title:     "Runtime",
					Status:    spacedomain.SpaceStatusOpen,
				}, nil
			},
		},
		MemberDirectory: stubMCPMemberDirectory{
			listFn: func(_ context.Context, filter member.Filter) ([]member.Record, error) {
				if filter.SpaceID != "space-session" {
					t.Fatalf("filter space id=%q want space-session", filter.SpaceID)
				}
				if filter.LifecycleState != member.LifecycleActive {
					t.Fatalf("filter lifecycle=%q want active", filter.LifecycleState)
				}
				return []member.Record{{
					ID:             "member-session",
					SpaceID:        "space-session",
					ChannelID:      "channel:space-session:member:member-session",
					DisplayName:    "Worker",
					MemberType:     member.TypeWorker,
					LifecycleState: member.LifecycleActive,
				}}, nil
			},
		},
	})
	server, err := NewServer(store)
	if err != nil {
		t.Fatalf("new server: %v", err)
	}

	resp := postMCPRequest(t, server.Handler(), "token-space", map[string]any{
		"jsonrpc": "2.0",
		"id":      "space-call",
		"method":  "tools/call",
		"params": map[string]any{
			"name": "space",
			"arguments": map[string]any{
				"action": "member_list",
			},
		},
	})
	if resp.Error != nil {
		t.Fatalf("unexpected rpc error: %+v", resp.Error)
	}
	var result testMCPToolCallResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("decode tools/call result: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success tool result: %+v", result)
	}
	if len(result.Content) == 0 || result.Content[0].Text == "" {
		t.Fatalf("missing tool content: %+v", result)
	}
	var structured map[string]any
	if err := json.Unmarshal(result.StructuredContent, &structured); err != nil {
		t.Fatalf("decode structuredContent: %v", err)
	}
	if got := strings.TrimSpace(anyString(structured["tool"])); got != "space" {
		t.Fatalf("structuredContent.tool=%q want space", got)
	}
	if got := strings.TrimSpace(anyString(structured["action"])); got != "member_list" {
		t.Fatalf("structuredContent.action=%q want member_list", got)
	}
	if _, exists := structured["op"]; exists {
		t.Fatalf("structuredContent contains legacy op field: %+v", structured)
	}
}

func TestServer_ToolsCallExecutesNativeSpaceCreateMemberHandler(t *testing.T) {
	store := NewTokenStore()
	store.Register("token-space-create", Session{
		SpaceID:   "space-session",
		MemberID:  "member-coordinator",
		ProjectID: "project-session",
		SpaceReader: stubMCPSpaceReader{
			getFn: func(_ context.Context, id spacedomain.SpaceID) (spacedomain.SpaceRecord, error) {
				if id != "space-session" {
					t.Fatalf("space id=%q want space-session", id)
				}
				return spacedomain.SpaceRecord{
					ID:        id,
					ProjectID: "project-session",
					Title:     "Runtime",
					Status:    spacedomain.SpaceStatusOpen,
				}, nil
			},
		},
		MemberDirectory: stubMCPMemberDirectory{
			getFn: func(_ context.Context, id member.ID) (member.Record, error) {
				if id != "member-coordinator" {
					t.Fatalf("actor member id=%q want member-coordinator", id)
				}
				return member.Record{
					ID:             id,
					UserID:         "user-session",
					SpaceID:        "space-session",
					DisplayName:    "Coordinator",
					MemberType:     member.TypeCoordinator,
					LifecycleState: member.LifecycleActive,
				}, nil
			},
		},
		MemberRegistrar: stubMCPMemberRegistrar{
			registerFn: func(ctx context.Context, rosterMember member.Record) (spaceapp.RegisterMemberResult, error) {
				resolved, err := (caller.ContextResolver{}).ResolveCaller(ctx)
				if err != nil {
					t.Fatalf("register missing caller: %v", err)
				}
				if resolved.MemberID != "member-coordinator" || resolved.SpaceID != "space-session" {
					t.Fatalf("caller=%+v", resolved)
				}
				if rosterMember.SpaceID != "space-session" || rosterMember.ProjectID != "project-session" {
					t.Fatalf("member scope=%+v", rosterMember)
				}
				if rosterMember.MemberType != member.TypeWorker || rosterMember.HarnessKind != "codex" || rosterMember.Model != "gpt-5" || rosterMember.Effort != "medium" {
					t.Fatalf("member config=%+v", rosterMember)
				}
				created := rosterMember
				created.ID = "member-worker"
				created.ChannelID = "channel:space-session:member:member-worker"
				created.LifecycleState = member.LifecycleActive
				return spaceapp.RegisterMemberResult{Member: created, GrantedMemberType: member.TypeWorker}, nil
			},
		},
	})
	server, err := NewServer(store)
	if err != nil {
		t.Fatalf("new server: %v", err)
	}

	resp := postMCPRequest(t, server.Handler(), "token-space-create", map[string]any{
		"jsonrpc": "2.0",
		"id":      "space-create-call",
		"method":  "tools/call",
		"params": map[string]any{
			"name": "space",
			"arguments": map[string]any{
				"action":       "member_create",
				"display_name": "Backend lead",
				"harness_kind": "codex",
				"model":        "gpt-5",
				"effort":       "medium",
			},
		},
	})
	if resp.Error != nil {
		t.Fatalf("unexpected rpc error: %+v", resp.Error)
	}
	var result testMCPToolCallResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("decode tools/call result: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success tool result: %+v", result)
	}
	var structured map[string]any
	if err := json.Unmarshal(result.StructuredContent, &structured); err != nil {
		t.Fatalf("decode structuredContent: %v", err)
	}
	if got := strings.TrimSpace(anyString(structured["action"])); got != "member_create" {
		t.Fatalf("structuredContent.action=%q want member_create", got)
	}
	memberPayload, ok := structured["member"].(map[string]any)
	if !ok {
		t.Fatalf("member payload=%T", structured["member"])
	}
	if got := strings.TrimSpace(anyString(memberPayload["id"])); got != "member-worker" {
		t.Fatalf("member id=%q want member-worker", got)
	}
}

func TestServer_ToolsCallExecutesNativeMessageHandler(t *testing.T) {
	store := NewTokenStore()
	store.Register("token-message", Session{
		SpaceID:   "space-source",
		MemberID:  "member-source",
		ProjectID: "project-session",
		MemberDirectory: stubMCPMemberDirectory{
			getFn: func(_ context.Context, id member.ID) (member.Record, error) {
				switch id {
				case "member-source":
					return member.Record{ID: id, SpaceID: "space-source", DisplayName: "Source", LifecycleState: member.LifecycleActive}, nil
				case "member-target":
					return member.Record{ID: id, SpaceID: "space-source", DisplayName: "Target", LifecycleState: member.LifecycleActive}, nil
				default:
					t.Fatalf("member id=%q", id)
					return member.Record{}, nil
				}
			},
		},
		MessagePublisher: stubMCPMessagePublisher{
			publishFn: func(_ context.Context, input messagedomain.NewMessageInput) (types.AgentMessage, error) {
				if input.Route.SourceMemberID != "member-source" {
					t.Fatalf("source member id=%q", input.Route.SourceMemberID)
				}
				if input.Route.DestinationMemberID != "member-target" {
					t.Fatalf("destination member id=%q", input.Route.DestinationMemberID)
				}
				if input.Route.ChannelID != "channel:space-source:member:member-target" {
					t.Fatalf("channel id=%q", input.Route.ChannelID)
				}
				msg, err := messagedomain.NewMessage(input, fixedMCPTime)
				if err != nil {
					return types.AgentMessage{}, err
				}
				return msg.Inner(), nil
			},
		},
	})
	server, err := NewServer(store)
	if err != nil {
		t.Fatalf("new server: %v", err)
	}

	resp := postMCPRequest(t, server.Handler(), "token-message", map[string]any{
		"jsonrpc": "2.0",
		"id":      "message-call",
		"method":  "tools/call",
		"params": map[string]any{
			"name": "message",
			"arguments": map[string]any{
				"action":                "send",
				"destination_member_id": "member-target",
				"kind":                  "inform",
				"subject":               "Review",
				"body":                  "Please check this",
			},
		},
	})
	if resp.Error != nil {
		t.Fatalf("unexpected rpc error: %+v", resp.Error)
	}
	var result testMCPToolCallResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("decode tools/call result: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success tool result: %+v", result)
	}
	var structured map[string]any
	if err := json.Unmarshal(result.StructuredContent, &structured); err != nil {
		t.Fatalf("decode structuredContent: %v", err)
	}
	if got := strings.TrimSpace(anyString(structured["tool"])); got != "message" {
		t.Fatalf("structuredContent.tool=%q want message", got)
	}
	if got := strings.TrimSpace(anyString(structured["action"])); got != "send" {
		t.Fatalf("structuredContent.action=%q want send", got)
	}
	if got := strings.TrimSpace(anyString(structured["guidance"])); !strings.Contains(got, "asynchronous") {
		t.Fatalf("structuredContent.guidance=%q want asynchronous guidance", got)
	}
	if _, exists := structured["op"]; exists {
		t.Fatalf("structuredContent contains legacy op field: %+v", structured)
	}
}

func TestServer_ToolsCallReadsNativeMessageInbox(t *testing.T) {
	store := NewTokenStore()
	store.Register("token-message-inbox", Session{
		SpaceID:   "space-source",
		MemberID:  "member-source",
		ProjectID: "project-session",
		MemberDirectory: stubMCPMemberDirectory{
			getFn: func(_ context.Context, id member.ID) (member.Record, error) {
				if id != "member-source" {
					t.Fatalf("member id=%q", id)
				}
				return member.Record{ID: id, SpaceID: "space-source", DisplayName: "Source", LifecycleState: member.LifecycleActive}, nil
			},
		},
		MessagePublisher: stubMCPMessagePublisher{
			listFn: func(_ context.Context, filter messagedomain.MessageFilter) ([]types.AgentMessage, error) {
				if filter.SpaceID != "space-source" || filter.DestinationMemberID != "member-source" {
					t.Fatalf("filter route=%+v", filter)
				}
				if len(filter.Statuses) != 1 || filter.Statuses[0] != types.MessageStatusQueuedTyped {
					t.Fatalf("filter statuses=%+v", filter.Statuses)
				}
				if filter.Limit != 5 {
					t.Fatalf("filter limit=%d want 5", filter.Limit)
				}
				return []types.AgentMessage{{
					ID:                  "msg-1",
					SpaceID:             "space-source",
					DestinationMemberID: "member-source",
					ChannelID:           "channel:space-source:member:member-source",
					Kind:                types.AgentMessageKindSystem,
					Subject:             "Task assigned",
					Body:                map[string]any{"taskId": "task-1", "nextAction": "claim"},
					Producer:            "task-service",
					CorrelationID:       "task:task-1",
					TaskRef:             "task-1",
					Status:              types.MessageStatusQueuedTyped,
					VisibleAt:           fixedMCPTime,
					CreatedAt:           fixedMCPTime,
				}}, nil
			},
		},
	})
	server, err := NewServer(store)
	if err != nil {
		t.Fatalf("new server: %v", err)
	}

	resp := postMCPRequest(t, server.Handler(), "token-message-inbox", map[string]any{
		"jsonrpc": "2.0",
		"id":      "message-inbox-call",
		"method":  "tools/call",
		"params": map[string]any{
			"name": "message",
			"arguments": map[string]any{
				"action": "inbox",
				"status": "queued",
				"limit":  5,
			},
		},
	})
	if resp.Error != nil {
		t.Fatalf("unexpected rpc error: %+v", resp.Error)
	}
	var result testMCPToolCallResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("decode tools/call result: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success tool result: %+v", result)
	}
	var structured struct {
		Action   string `json:"action"`
		MemberID string `json:"memberId"`
		Count    int    `json:"count"`
		Messages []struct {
			MessageID string         `json:"messageId"`
			Subject   string         `json:"subject"`
			TaskRef   string         `json:"taskRef"`
			Body      map[string]any `json:"body"`
			Status    string         `json:"status"`
			Producer  string         `json:"producer"`
			CreatedAt string         `json:"createdAt"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(result.StructuredContent, &structured); err != nil {
		t.Fatalf("decode structuredContent: %v", err)
	}
	if structured.Action != "inbox" || structured.MemberID != "member-source" || structured.Count != 1 {
		t.Fatalf("structured=%+v", structured)
	}
	if structured.Messages[0].MessageID != "msg-1" || structured.Messages[0].Subject != "Task assigned" || structured.Messages[0].TaskRef != "task-1" {
		t.Fatalf("message=%+v", structured.Messages[0])
	}
	if structured.Messages[0].Body["nextAction"] != "claim" || structured.Messages[0].Producer != "task-service" || structured.Messages[0].Status != "queued" {
		t.Fatalf("message details=%+v", structured.Messages[0])
	}
	if strings.TrimSpace(structured.Messages[0].CreatedAt) == "" {
		t.Fatalf("createdAt missing: %+v", structured.Messages[0])
	}
}

func TestServer_InitializeAdvertisesInboxResourceSubscriptions(t *testing.T) {
	store := NewTokenStore()
	store.Register("token-resource-init", Session{
		SpaceID:          "space-source",
		MemberID:         "member-source",
		MessagePublisher: stubMCPMessagePublisher{},
	})
	server, err := NewServer(store)
	if err != nil {
		t.Fatalf("new server: %v", err)
	}

	resp := postMCPRequest(t, server.Handler(), "token-resource-init", map[string]any{
		"jsonrpc": "2.0",
		"id":      "init-1",
		"method":  "initialize",
		"params": map[string]any{
			"protocolVersion": "2025-06-18",
			"capabilities":    map[string]any{},
			"clientInfo": map[string]any{
				"name":    "test-client",
				"version": "0.0.0",
			},
		},
	})
	if resp.Error != nil {
		t.Fatalf("unexpected rpc error: %+v", resp.Error)
	}
	var result struct {
		Capabilities struct {
			Resources struct {
				ListChanged bool `json:"listChanged"`
				Subscribe   bool `json:"subscribe"`
			} `json:"resources"`
		} `json:"capabilities"`
	}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("decode initialize result: %v", err)
	}
	if !result.Capabilities.Resources.ListChanged || !result.Capabilities.Resources.Subscribe {
		t.Fatalf("resource capabilities=%+v", result.Capabilities.Resources)
	}
}

func TestServer_BootstrapInitializeAdvertisesInboxResourceSubscriptions(t *testing.T) {
	store := NewTokenStore()
	store.Register("token-resource-bootstrap-init", Session{
		Bootstrap: true,
	})
	server, err := NewServer(store)
	if err != nil {
		t.Fatalf("new server: %v", err)
	}

	resp := postMCPRequest(t, server.Handler(), "token-resource-bootstrap-init", map[string]any{
		"jsonrpc": "2.0",
		"id":      "bootstrap-init-1",
		"method":  "initialize",
		"params": map[string]any{
			"protocolVersion": "2025-06-18",
			"capabilities":    map[string]any{},
			"clientInfo": map[string]any{
				"name":    "test-client",
				"version": "0.0.0",
			},
		},
	})
	if resp.Error != nil {
		t.Fatalf("unexpected rpc error: %+v", resp.Error)
	}
	var result struct {
		Capabilities struct {
			Resources struct {
				ListChanged bool `json:"listChanged"`
				Subscribe   bool `json:"subscribe"`
			} `json:"resources"`
		} `json:"capabilities"`
	}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("decode initialize result: %v", err)
	}
	if !result.Capabilities.Resources.ListChanged || !result.Capabilities.Resources.Subscribe {
		t.Fatalf("bootstrap resource capabilities=%+v", result.Capabilities.Resources)
	}
}

func TestServer_ResourcesListExposesCurrentMemberInbox(t *testing.T) {
	store := NewTokenStore()
	store.Register("token-resource-list", Session{
		SpaceID:          "space-source",
		MemberID:         "member-source",
		MessagePublisher: stubMCPMessagePublisher{},
	})
	server, err := NewServer(store)
	if err != nil {
		t.Fatalf("new server: %v", err)
	}

	resp := postMCPRequest(t, server.Handler(), "token-resource-list", map[string]any{
		"jsonrpc": "2.0",
		"id":      "resource-list-1",
		"method":  "resources/list",
	})
	if resp.Error != nil {
		t.Fatalf("unexpected rpc error: %+v", resp.Error)
	}
	var result struct {
		Resources []struct {
			URI      string `json:"uri"`
			Name     string `json:"name"`
			Title    string `json:"title"`
			MIMEType string `json:"mimeType"`
		} `json:"resources"`
	}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("decode resources/list result: %v", err)
	}
	if len(result.Resources) != 1 {
		t.Fatalf("resources=%+v", result.Resources)
	}
	got := result.Resources[0]
	if got.URI != agen8InboxResourceURI || got.Name != "agen8-current-member-inbox" || got.MIMEType != "application/json" {
		t.Fatalf("resource=%+v", got)
	}
}

func TestServer_BootstrapResourcesListExposesCurrentMemberInbox(t *testing.T) {
	store := NewTokenStore()
	store.Register("token-resource-bootstrap-list", Session{
		Bootstrap: true,
	})
	server, err := NewServer(store)
	if err != nil {
		t.Fatalf("new server: %v", err)
	}

	resp := postMCPRequest(t, server.Handler(), "token-resource-bootstrap-list", map[string]any{
		"jsonrpc": "2.0",
		"id":      "bootstrap-resource-list-1",
		"method":  "resources/list",
	})
	if resp.Error != nil {
		t.Fatalf("unexpected rpc error: %+v", resp.Error)
	}
	var result struct {
		Resources []struct {
			URI      string `json:"uri"`
			Name     string `json:"name"`
			MIMEType string `json:"mimeType"`
		} `json:"resources"`
	}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("decode resources/list result: %v", err)
	}
	if len(result.Resources) != 1 {
		t.Fatalf("resources=%+v", result.Resources)
	}
	got := result.Resources[0]
	if got.URI != agen8InboxResourceURI || got.Name != "agen8-current-member-inbox" || got.MIMEType != "application/json" {
		t.Fatalf("resource=%+v", got)
	}
}

func TestServer_ResourcesReadReturnsCurrentMemberQueuedInbox(t *testing.T) {
	store := NewTokenStore()
	store.Register("token-resource-read", Session{
		SpaceID:  "space-source",
		MemberID: "member-source",
		MessagePublisher: stubMCPMessagePublisher{
			listFn: func(_ context.Context, filter messagedomain.MessageFilter) ([]types.AgentMessage, error) {
				if filter.SpaceID != "space-source" || filter.DestinationMemberID != "member-source" {
					t.Fatalf("filter route=%+v", filter)
				}
				if len(filter.Statuses) != 1 || filter.Statuses[0] != types.MessageStatusQueuedTyped {
					t.Fatalf("filter statuses=%+v", filter.Statuses)
				}
				if filter.Limit != 25 {
					t.Fatalf("filter limit=%d want 25", filter.Limit)
				}
				return []types.AgentMessage{{
					ID:                  "msg-resource-1",
					SpaceID:             "space-source",
					DestinationMemberID: "member-source",
					ChannelID:           "channel:space-source:member:member-source",
					Kind:                types.AgentMessageKindSystem,
					Subject:             "Task assigned",
					Body:                map[string]any{"taskId": "task-resource-1", "nextAction": "claim"},
					Producer:            "task-service",
					CorrelationID:       "task:task-resource-1",
					TaskRef:             "task-resource-1",
					Status:              types.MessageStatusQueuedTyped,
					VisibleAt:           fixedMCPTime,
					CreatedAt:           fixedMCPTime,
				}}, nil
			},
		},
	})
	server, err := NewServer(store)
	if err != nil {
		t.Fatalf("new server: %v", err)
	}

	resp := postMCPRequest(t, server.Handler(), "token-resource-read", map[string]any{
		"jsonrpc": "2.0",
		"id":      "resource-read-1",
		"method":  "resources/read",
		"params": map[string]any{
			"uri": agen8InboxResourceURI,
		},
	})
	if resp.Error != nil {
		t.Fatalf("unexpected rpc error: %+v", resp.Error)
	}
	var result struct {
		Contents []struct {
			URI      string `json:"uri"`
			MIMEType string `json:"mimeType"`
			Text     string `json:"text"`
		} `json:"contents"`
	}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("decode resources/read result: %v", err)
	}
	if len(result.Contents) != 1 || result.Contents[0].URI != agen8InboxResourceURI || result.Contents[0].MIMEType != "application/json" {
		t.Fatalf("contents=%+v", result.Contents)
	}
	var payload struct {
		Resource string `json:"resource"`
		SpaceID  string `json:"spaceId"`
		MemberID string `json:"memberId"`
		Status   string `json:"status"`
		Count    int    `json:"count"`
		Messages []struct {
			ID        string         `json:"id"`
			Subject   string         `json:"subject"`
			TaskRef   string         `json:"taskRef"`
			Body      map[string]any `json:"body"`
			Producer  string         `json:"producer"`
			CreatedAt string         `json:"createdAt"`
		} `json:"messages"`
	}
	if err := json.Unmarshal([]byte(result.Contents[0].Text), &payload); err != nil {
		t.Fatalf("decode inbox payload: %v", err)
	}
	if payload.Resource != agen8InboxResourceURI || payload.SpaceID != "space-source" || payload.MemberID != "member-source" || payload.Status != "queued" || payload.Count != 1 {
		t.Fatalf("payload=%+v", payload)
	}
	if payload.Messages[0].ID != "msg-resource-1" || payload.Messages[0].Subject != "Task assigned" || payload.Messages[0].TaskRef != "task-resource-1" {
		t.Fatalf("message=%+v", payload.Messages[0])
	}
	if payload.Messages[0].Body["nextAction"] != "claim" || payload.Messages[0].Producer != "task-service" || strings.TrimSpace(payload.Messages[0].CreatedAt) == "" {
		t.Fatalf("message details=%+v", payload.Messages[0])
	}
}

func TestServer_BootstrapResourcesReadRequiresRegistration(t *testing.T) {
	store := NewTokenStore()
	store.Register("token-resource-bootstrap-read", Session{
		Bootstrap:        true,
		MessagePublisher: stubMCPMessagePublisher{},
	})
	server, err := NewServer(store)
	if err != nil {
		t.Fatalf("new server: %v", err)
	}

	resp := postMCPRequest(t, server.Handler(), "token-resource-bootstrap-read", map[string]any{
		"jsonrpc": "2.0",
		"id":      "bootstrap-resource-read-1",
		"method":  "resources/read",
		"params": map[string]any{
			"uri": agen8InboxResourceURI,
		},
	})
	if resp.Error != nil {
		t.Fatalf("unexpected rpc error: %+v", resp.Error)
	}
	var result struct {
		Contents []struct {
			URI  string `json:"uri"`
			Text string `json:"text"`
		} `json:"contents"`
	}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("decode resources/read result: %v", err)
	}
	if len(result.Contents) != 1 || result.Contents[0].URI != agen8InboxResourceURI {
		t.Fatalf("contents=%+v", result.Contents)
	}
	var payload struct {
		Resource             string `json:"resource"`
		Registered           bool   `json:"registered"`
		Status               string `json:"status"`
		NextAction           string `json:"nextAction"`
		RequiredTool         string `json:"requiredTool"`
		RequiredToolAction   string `json:"requiredToolAction"`
		RequiresRegistration bool   `json:"requiresRegistration"`
		Messages             []any  `json:"messages"`
	}
	if err := json.Unmarshal([]byte(result.Contents[0].Text), &payload); err != nil {
		t.Fatalf("decode unbound resource payload: %v", err)
	}
	if payload.Resource != agen8InboxResourceURI || payload.Registered || payload.Status != "unbound" || payload.NextAction != "register" || payload.RequiredTool != "space" || payload.RequiredToolAction != "register" || !payload.RequiresRegistration || len(payload.Messages) != 0 {
		t.Fatalf("payload=%+v", payload)
	}
}

func TestServer_ResourcesSubscribeValidatesInboxURI(t *testing.T) {
	store := NewTokenStore()
	store.Register("token-resource-subscribe", Session{
		SpaceID:          "space-source",
		MemberID:         "member-source",
		MessagePublisher: stubMCPMessagePublisher{},
	})
	server, err := NewServer(store)
	if err != nil {
		t.Fatalf("new server: %v", err)
	}

	resp := postMCPRequest(t, server.Handler(), "token-resource-subscribe", map[string]any{
		"jsonrpc": "2.0",
		"id":      "resource-subscribe-1",
		"method":  "resources/subscribe",
		"params": map[string]any{
			"uri": agen8InboxResourceURI,
		},
	})
	if resp.Error != nil {
		t.Fatalf("unexpected rpc error: %+v", resp.Error)
	}

	resp = postMCPRequest(t, server.Handler(), "token-resource-subscribe", map[string]any{
		"jsonrpc": "2.0",
		"id":      "resource-subscribe-unknown",
		"method":  "resources/subscribe",
		"params": map[string]any{
			"uri": "agen8://unknown",
		},
	})
	if resp.Error == nil {
		t.Fatal("expected rpc error for unknown resource subscription")
	}
	if !strings.Contains(resp.Error.Message, "not found") {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}
}

func TestServer_ResourcesSubscribeEmitsInboxUpdateOnMemberWake(t *testing.T) {
	wakes := make(chan messagedomain.MessageWake, 1)
	store := NewTokenStore()
	store.Register("token-resource-update", Session{
		SpaceID:  "space-source",
		MemberID: "member-source",
		MessagePublisher: stubMCPMessagePublisher{
			wakes: wakes,
		},
	})
	server, err := NewServer(store)
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	httpServer := httptest.NewServer(server.Handler())
	t.Cleanup(httpServer.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	updates := make(chan string, 1)
	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "agen8-resource-test", Version: "0.0.0"}, &sdkmcp.ClientOptions{
		ResourceUpdatedHandler: func(_ context.Context, req *sdkmcp.ResourceUpdatedNotificationRequest) {
			if req == nil || req.Params == nil {
				updates <- ""
				return
			}
			updates <- strings.TrimSpace(req.Params.URI)
		},
	})
	session, err := client.Connect(ctx, &sdkmcp.StreamableClientTransport{
		Endpoint: httpServer.URL + "/mcp?token=token-resource-update",
	}, nil)
	if err != nil {
		t.Fatalf("connect client: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })

	if err := session.Subscribe(ctx, &sdkmcp.SubscribeParams{URI: agen8InboxResourceURI}); err != nil {
		t.Fatalf("subscribe inbox resource: %v", err)
	}
	wakes <- messagedomain.MessageWake{
		MessageID:           "msg-resource-update-1",
		SpaceID:             "space-source",
		DestinationMemberID: "member-source",
		ChannelID:           "channel:space-source:member:member-source",
		Kind:                types.AgentMessageKindInform,
	}

	select {
	case uri := <-updates:
		if uri != agen8InboxResourceURI {
			t.Fatalf("resource update uri=%q want %q", uri, agen8InboxResourceURI)
		}
	case <-ctx.Done():
		t.Fatalf("timed out waiting for resource update notification: %v", ctx.Err())
	}
}

func TestServer_ToolsCallExecutesNativeHTTPHandler(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("method=%q want GET", r.Method)
		}
		w.Header().Set("Content-Type", "text/plain")
		io.WriteString(w, "ok")
	}))
	t.Cleanup(target.Close)

	store := NewTokenStore()
	store.Register("token-http", Session{
		ProjectID: "proj-http",
	})
	server, err := NewServer(store)
	if err != nil {
		t.Fatalf("new server: %v", err)
	}

	resp := postMCPRequest(t, server.Handler(), "token-http", map[string]any{
		"jsonrpc": "2.0",
		"id":      "http-1",
		"method":  "tools/call",
		"params": map[string]any{
			"name": "http",
			"arguments": map[string]any{
				"url":    target.URL,
				"method": "GET",
			},
		},
	})
	if resp.Error != nil {
		t.Fatalf("unexpected rpc error: %+v", resp.Error)
	}
	var result testMCPToolCallResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("decode tools/call result: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected successful http tool call, got error result: %+v", result)
	}
}

func TestServer_InvalidTokenReturnsJSONRPCError(t *testing.T) {
	store := NewTokenStore()
	server, err := NewServer(store)
	if err != nil {
		t.Fatalf("new server: %v", err)
	}

	resp := postMCPRequest(t, server.Handler(), "bad-token", map[string]any{
		"jsonrpc": "2.0",
		"id":      "3",
		"method":  "tools/list",
	})
	if resp.Error == nil {
		t.Fatal("expected rpc error for invalid token")
	}
	if resp.Error.Code == 0 {
		t.Fatalf("missing rpc error code: %+v", resp.Error)
	}
}

func TestServer_UnknownToolReturnsMCPErrorResult(t *testing.T) {
	store := NewTokenStore()
	store.Register("token-unknown", Session{})
	server, err := NewServer(store)
	if err != nil {
		t.Fatalf("new server: %v", err)
	}

	resp := postMCPRequest(t, server.Handler(), "token-unknown", map[string]any{
		"jsonrpc": "2.0",
		"id":      "4",
		"method":  "tools/call",
		"params": map[string]any{
			"name":      "not_a_real_tool",
			"arguments": map[string]any{},
		},
	})
	if resp.Error != nil {
		t.Fatalf("expected mcp result error, got rpc error: %+v", resp.Error)
	}
	var result testMCPToolCallResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("decode tools/call result: %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected isError=true: %+v", result)
	}
}

func TestServer_FilteredLegacyToolsReturnUnknownResult(t *testing.T) {
	store := NewTokenStore()
	store.Register("token-filtered", Session{})
	server, err := NewServer(store)
	if err != nil {
		t.Fatalf("new server: %v", err)
	}

	resp := postMCPRequest(t, server.Handler(), "token-filtered", map[string]any{
		"jsonrpc": "2.0",
		"id":      "note-1",
		"method":  "tools/call",
		"params": map[string]any{
			"name":      "note",
			"arguments": map[string]any{"text": "hidden"},
		},
	})
	if resp.Error != nil {
		t.Fatalf("expected MCP error-result for filtered tool, got rpc error: %+v", resp.Error)
	}
	var result testMCPToolCallResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("decode tools/call result: %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected isError=true for filtered tool: %+v", result)
	}
}

func TestServer_FilteredCodeExecReturnsUnknownResult(t *testing.T) {
	store := NewTokenStore()
	store.Register("token-filtered-codeexec", Session{})
	server, err := NewServer(store)
	if err != nil {
		t.Fatalf("new server: %v", err)
	}

	resp := postMCPRequest(t, server.Handler(), "token-filtered-codeexec", map[string]any{
		"jsonrpc": "2.0",
		"id":      "codeexec-1",
		"method":  "tools/call",
		"params": map[string]any{
			"name": "code_exec",
			"arguments": map[string]any{
				"language": "python",
				"code":     "result = 1",
			},
		},
	})
	if resp.Error != nil {
		t.Fatalf("expected MCP error-result for filtered tool, got rpc error: %+v", resp.Error)
	}
	var result testMCPToolCallResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("decode tools/call result: %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected isError=true for filtered code_exec: %+v", result)
	}
}

func TestServer_FilteredToolReturnsUnknownResult(t *testing.T) {
	store := NewTokenStore()
	store.Register("token-filtered", Session{})
	server, err := NewServer(store)
	if err != nil {
		t.Fatalf("new server: %v", err)
	}

	for _, tc := range []struct {
		name      string
		arguments map[string]any
	}{
		{name: "browser", arguments: map[string]any{"action": "navigate", "url": "https://example.com"}},
	} {
		resp := postMCPRequest(t, server.Handler(), "token-filtered", map[string]any{
			"jsonrpc": "2.0",
			"id":      tc.name + "-1",
			"method":  "tools/call",
			"params": map[string]any{
				"name":      tc.name,
				"arguments": tc.arguments,
			},
		})
		if resp.Error != nil {
			t.Fatalf("%s: expected MCP error-result for filtered tool, got rpc error: %+v", tc.name, resp.Error)
		}
		var result testMCPToolCallResult
		if err := json.Unmarshal(resp.Result, &result); err != nil {
			t.Fatalf("%s: decode tools/call result: %v", tc.name, err)
		}
		if !result.IsError {
			t.Fatalf("%s: expected isError=true for filtered tool: %+v", tc.name, result)
		}
		if len(result.Content) == 0 || !strings.Contains(strings.ToLower(result.Content[0].Text), `unknown tool "`+tc.name+`"`) {
			t.Fatalf("%s: expected unknown tool message, got %+v", tc.name, result)
		}
	}
}

func TestExposeTool_RejectsEmptyName(t *testing.T) {
	if exposeTool("") {
		t.Fatal("exposeTool(\"\")=true want false")
	}
	if !exposeTool("task") {
		t.Fatal("exposeTool(task)=false want true")
	}
	if exposeTool("final_answer") {
		t.Fatal("exposeTool(final_answer)=true want false")
	}
}

func postMCPRequest(t *testing.T, handler http.Handler, token string, payload map[string]any) testMCPJSONRPCResponse {
	t.Helper()
	sessionID := ""
	if method, _ := payload["method"].(string); method != "initialize" {
		initResp, initSessionID := postMCPRequestRaw(t, handler, token, "", map[string]any{
			"jsonrpc": "2.0",
			"id":      "test-init",
			"method":  "initialize",
			"params": map[string]any{
				"protocolVersion": "2025-06-18",
				"capabilities":    map[string]any{},
				"clientInfo": map[string]any{
					"name":    "agen8-test",
					"version": "0.0.0",
				},
			},
		})
		if initResp.Error != nil {
			return initResp
		}
		if strings.TrimSpace(initSessionID) == "" {
			t.Fatal("initialize response missing Mcp-Session-Id")
		}
		sessionID = initSessionID
		_ = postMCPNotification(t, handler, token, sessionID, map[string]any{
			"jsonrpc": "2.0",
			"method":  "notifications/initialized",
			"params":  map[string]any{},
		})
	}
	resp, _ := postMCPRequestRaw(t, handler, token, sessionID, payload)
	return resp
}

func postMCPRequestRaw(t *testing.T, handler http.Handler, token string, sessionID string, payload map[string]any) (testMCPJSONRPCResponse, string) {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/mcp?token="+token, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if strings.TrimSpace(sessionID) != "" {
		req.Header.Set("Mcp-Session-Id", strings.TrimSpace(sessionID))
	}
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var resp testMCPJSONRPCResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v (body=%s)", err, rr.Body.String())
	}
	return resp, strings.TrimSpace(rr.Header().Get("Mcp-Session-Id"))
}

func postMCPNotification(t *testing.T, handler http.Handler, token string, sessionID string, payload map[string]any) int {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal notification: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/mcp?token="+token, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if strings.TrimSpace(sessionID) != "" {
		req.Header.Set("Mcp-Session-Id", strings.TrimSpace(sessionID))
	}
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted && rr.Code != http.StatusOK {
		t.Fatalf("notification status=%d body=%s", rr.Code, rr.Body.String())
	}
	return rr.Code
}

func anyString(value any) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprintf("%v", value))
}

func anyBool(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	default:
		return false
	}
}
