package rpc

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/tinoosan/agen8-mcp-server/internal/caller"
	messagedomain "github.com/tinoosan/agen8-mcp-server/internal/services/message/domain"
	spacedomain "github.com/tinoosan/agen8-mcp-server/internal/services/space/domain"
	"github.com/tinoosan/agen8-mcp-server/internal/services/space/domain/member"
	taskapp "github.com/tinoosan/agen8-mcp-server/internal/services/task/app"
	"github.com/tinoosan/agen8-mcp-server/internal/services/task/domain"
	taskrpc "github.com/tinoosan/agen8-mcp-server/internal/services/task/rpc"
	"github.com/tinoosan/agen8-mcp-server/pkg/types"
)

var rpcTaskTestNow = time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)

func TestRegistryRejectsDuplicateMethod(t *testing.T) {
	reg := NewRegistry()
	handler := HandlerFunc(func(context.Context, json.RawMessage) (any, error) {
		return map[string]string{"ok": "true"}, nil
	})

	if err := reg.Add("task.create", handler); err != nil {
		t.Fatalf("first Add returned error: %v", err)
	}
	if err := reg.Add("task.create", handler); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("second Add error=%v want duplicate registration error", err)
	}
}

func TestServerHandleReturnsParseError(t *testing.T) {
	server, err := NewServer(NewRegistry())
	if err != nil {
		t.Fatalf("NewServer returned error: %v", err)
	}

	raw, err := server.Handle(context.Background(), []byte(`{`))
	if err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}
	resp := decodeRPCResponse(t, raw)
	if resp.Error == nil || resp.Error.Code != CodeParseError {
		t.Fatalf("response error=%+v want parse error", resp.Error)
	}
}

func TestServerDispatchUnknownMethod(t *testing.T) {
	server, err := NewServer(NewRegistry())
	if err != nil {
		t.Fatalf("NewServer returned error: %v", err)
	}

	resp := server.Dispatch(context.Background(), Request{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`"1"`),
		Method:  "missing.method",
	})
	if resp.Error == nil || resp.Error.Code != CodeMethodNotFound {
		t.Fatalf("response error=%+v want method not found", resp.Error)
	}
}

func TestRegisterTaskDispatchCreate(t *testing.T) {
	svc, publisher := newRPCTaskService(t, newRPCTaskRepo())
	reg := NewRegistry()
	if err := RegisterTask(reg, svc); err != nil {
		t.Fatalf("RegisterTask returned error: %v", err)
	}
	server, err := NewServer(reg)
	if err != nil {
		t.Fatalf("NewServer returned error: %v", err)
	}
	ctx := ContextWithIdentity(context.Background(), Identity{
		UserID:   "user-1",
		MemberID: "member-coordinator",
	})

	raw, err := server.Handle(ctx, []byte(`{
		"jsonrpc": "2.0",
		"id": "1",
		"method": "task.create",
		"params": {
			"spaceId": "space-1",
			"assignedTo": "member-worker",
			"description": "write a report",
			"acceptanceCriteria": ["tests pass"]
		}
	}`))
	if err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}
	resp := decodeRPCResponse(t, raw)
	if resp.Error != nil {
		t.Fatalf("response error=%+v", resp.Error)
	}
	var result taskrpc.TaskCreateResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if result.Task.ID == "" || result.Task.AssignedTo != "member-worker" || result.Task.Description != "write a report" {
		t.Fatalf("task result=%+v", result.Task)
	}
	if len(publisher.messages) != 1 {
		t.Fatalf("published messages=%d want 1", len(publisher.messages))
	}
}

func TestRegisterTaskRequiresIdentity(t *testing.T) {
	svc, _ := newRPCTaskService(t, newRPCTaskRepo())
	reg := NewRegistry()
	if err := RegisterTask(reg, svc); err != nil {
		t.Fatalf("RegisterTask returned error: %v", err)
	}
	server, err := NewServer(reg)
	if err != nil {
		t.Fatalf("NewServer returned error: %v", err)
	}

	raw, err := server.Handle(context.Background(), []byte(`{
		"jsonrpc": "2.0",
		"id": "1",
		"method": "task.get",
		"params": { "taskId": "task-1" }
	}`))
	if err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}
	resp := decodeRPCResponse(t, raw)
	if resp.Error == nil || resp.Error.Code != CodeInvalidRequest {
		t.Fatalf("response error=%+v want invalid request", resp.Error)
	}
}

func TestRegisterTaskMapsInvalidParams(t *testing.T) {
	svc, _ := newRPCTaskService(t, newRPCTaskRepo())
	reg := NewRegistry()
	if err := RegisterTask(reg, svc); err != nil {
		t.Fatalf("RegisterTask returned error: %v", err)
	}
	server, err := NewServer(reg)
	if err != nil {
		t.Fatalf("NewServer returned error: %v", err)
	}
	ctx := ContextWithIdentity(context.Background(), Identity{
		UserID:   "user-1",
		MemberID: "member-coordinator",
	})

	raw, err := server.Handle(ctx, []byte(`{
		"jsonrpc": "2.0",
		"id": "1",
		"method": "task.create",
		"params": { "spaceId": "space-1", "assignedTo": "member-worker" }
	}`))
	if err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}
	resp := decodeRPCResponse(t, raw)
	if resp.Error == nil || resp.Error.Code != CodeInvalidParams {
		t.Fatalf("response error=%+v want invalid params", resp.Error)
	}
}

func decodeRPCResponse(t *testing.T, raw []byte) Response {
	t.Helper()
	var resp Response
	if err := json.Unmarshal(raw, &resp); err != nil {
		t.Fatalf("unmarshal response %s: %v", string(raw), err)
	}
	return resp
}

type rpcTaskRepo struct {
	tasks map[string]domain.Task
}

func newRPCTaskRepo(tasks ...domain.Task) *rpcTaskRepo {
	repo := &rpcTaskRepo{tasks: map[string]domain.Task{}}
	for _, task := range tasks {
		repo.tasks[string(task.ID)] = task
	}
	return repo
}

func (r *rpcTaskRepo) CreateTask(_ context.Context, task domain.Task) error {
	if task.ID == "" {
		return fmt.Errorf("task id is required")
	}
	r.tasks[string(task.ID)] = task
	return nil
}

func (r *rpcTaskRepo) UpdateTask(_ context.Context, task domain.Task) error {
	if _, ok := r.tasks[string(task.ID)]; !ok {
		return fmt.Errorf("task %s not found", task.ID)
	}
	r.tasks[string(task.ID)] = task
	return nil
}

func (r *rpcTaskRepo) GetTask(_ context.Context, taskID domain.TaskID) (domain.Task, error) {
	task, ok := r.tasks[string(taskID)]
	if !ok {
		return domain.Task{}, fmt.Errorf("task %s not found", taskID)
	}
	return task, nil
}

func (r *rpcTaskRepo) ListTasks(_ context.Context, filter domain.TaskFilter) ([]domain.Task, error) {
	out := make([]domain.Task, 0, len(r.tasks))
	for _, task := range r.tasks {
		if filter.SpaceID != "" && task.SpaceID != filter.SpaceID {
			continue
		}
		out = append(out, task)
	}
	return out, nil
}

func (r *rpcTaskRepo) CountTasks(ctx context.Context, filter domain.TaskFilter) (int, error) {
	tasks, err := r.ListTasks(ctx, filter)
	if err != nil {
		return 0, err
	}
	return len(tasks), nil
}

type rpcMemberReader struct {
	members map[string]member.Record
}

func (r rpcMemberReader) GetMember(_ context.Context, memberID member.ID) (member.Record, error) {
	rosterMember, ok := r.members[string(memberID)]
	if !ok {
		return member.Record{}, fmt.Errorf("member %s not found", memberID)
	}
	return rosterMember, nil
}

type rpcSpaceReader struct {
	spaces map[string]spacedomain.SpaceRecord
}

func (r rpcSpaceReader) Get(_ context.Context, spaceID spacedomain.SpaceID) (spacedomain.SpaceRecord, error) {
	space, ok := r.spaces[string(spaceID)]
	if !ok {
		return spacedomain.SpaceRecord{}, fmt.Errorf("space %s not found", spaceID)
	}
	return space, nil
}

type rpcMessagePublisher struct {
	messages []messagedomain.NewMessageInput
}

func (p *rpcMessagePublisher) PublishAgentMessage(_ context.Context, input messagedomain.NewMessageInput) (types.AgentMessage, error) {
	p.messages = append(p.messages, input)
	return types.AgentMessage{}, nil
}

func newRPCTaskService(t *testing.T, repo *rpcTaskRepo) (*taskapp.Service, *rpcMessagePublisher) {
	t.Helper()
	publisher := &rpcMessagePublisher{}
	svc, err := taskapp.NewService(
		repo,
		domain.FixedClock{T: rpcTaskTestNow},
		caller.ContextResolver{},
		rpcMemberReader{members: map[string]member.Record{
			"member-coordinator": {
				ID:             member.ID("member-coordinator"),
				UserID:         "user-1",
				SpaceID:        "space-1",
				MemberType:     member.TypeCoordinator,
				LifecycleState: member.LifecycleActive,
			},
			"member-worker": {
				ID:             member.ID("member-worker"),
				UserID:         "user-1",
				SpaceID:        "space-1",
				MemberType:     member.TypeWorker,
				LifecycleState: member.LifecycleActive,
			},
		}},
		rpcSpaceReader{spaces: map[string]spacedomain.SpaceRecord{
			"space-1": {
				ID:     spacedomain.SpaceID("space-1"),
				UserID: "user-1",
				Status: spacedomain.SpaceStatusOpen,
			},
		}},
		publisher,
		nil,
	)
	if err != nil {
		t.Fatalf("NewService returned error: %v", err)
	}
	return svc, publisher
}
