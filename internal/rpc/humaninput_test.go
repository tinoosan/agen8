package rpc

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	humaninputapp "github.com/tinoosan/agen8-mcp-server/internal/services/humaninput/app"
	humaninputdomain "github.com/tinoosan/agen8-mcp-server/internal/services/humaninput/domain"
	"github.com/tinoosan/agen8-mcp-server/pkg/protocol"
)

func TestRegisterHumanInputDispatchPendingAndSubmit(t *testing.T) {
	svc, wake := newRPCHumanInputService(t)
	req, err := svc.Declare(context.Background(), humaninputapp.DeclareCommand{
		ToolCallID:     "call-1",
		ToolName:       "decision",
		IdempotencyKey: "idem-1",
		ProjectID:      "project-1",
		SpaceID:        "space-1",
		AskerMemberID:  "member-1",
		ChannelID:      "channel-1",
		Declaration: humaninputdomain.Declaration{
			Kind:    humaninputdomain.PrimitiveQuestions,
			Payload: json.RawMessage(`{"questions":[{"id":"q1","text":"Choose","type":"multiple_choice","options":["A"]}]}`),
		},
	})
	if err != nil {
		t.Fatalf("Declare: %v", err)
	}
	reg := NewRegistry()
	if err := RegisterHumanInput(reg, svc, wake); err != nil {
		t.Fatalf("RegisterHumanInput: %v", err)
	}
	server, err := NewServer(reg)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	ctx := ContextWithIdentity(context.Background(), Identity{UserID: "user-1", MemberID: "resolver-1"})
	pendingResp := server.Dispatch(ctx, Request{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`"pending"`),
		Method:  protocol.MethodChannelHumanInputPending,
		Params:  json.RawMessage(`{"channelId":"channel-1"}`),
	})
	if pendingResp.Error != nil {
		t.Fatalf("pending error: %+v", pendingResp.Error)
	}
	var pending protocol.ChannelHumanInputPendingResult
	if err := json.Unmarshal(pendingResp.Result, &pending); err != nil {
		t.Fatalf("decode pending: %v", err)
	}
	if pending.Pending == nil || pending.Pending.ToolCallID != string(req.ToolCallID) {
		t.Fatalf("unexpected pending result: %+v", pending.Pending)
	}
	submitResp := server.Dispatch(ctx, Request{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`"submit"`),
		Method:  protocol.MethodChannelHumanInputSubmit,
		Params:  json.RawMessage(`{"spaceId":"space-1","memberId":"member-1","toolCallId":"call-1","result":{"answers":[{"questionId":"q1","selectedOption":"A"}]}}`),
	})
	if submitResp.Error != nil {
		t.Fatalf("submit error: %+v", submitResp.Error)
	}
	stored, err := svc.Get(context.Background(), req.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if stored.Status != humaninputdomain.StatusAnswered {
		t.Fatalf("Status=%s want answered", stored.Status)
	}
}

func TestRegisterHumanInputRequiresIdentity(t *testing.T) {
	svc, wake := newRPCHumanInputService(t)
	reg := NewRegistry()
	if err := RegisterHumanInput(reg, svc, wake); err != nil {
		t.Fatalf("RegisterHumanInput: %v", err)
	}
	server, err := NewServer(reg)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	resp := server.Dispatch(context.Background(), Request{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`"pending"`),
		Method:  protocol.MethodChannelHumanInputPending,
		Params:  json.RawMessage(`{"channelId":"channel-1"}`),
	})
	if resp.Error == nil {
		t.Fatal("expected identity error")
	}
}

func newRPCHumanInputService(t *testing.T) (*humaninputapp.Service, *humaninputapp.MemoryWakeRegistry) {
	t.Helper()
	repo := newMemoryRepo()
	svc, err := humaninputapp.NewService(repo, rpcHumanInputClock{}, func() string { return "hi-rpc-1" }, humaninputdomain.DefaultValidator{}, nil)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return svc, humaninputapp.NewMemoryWakeRegistry()
}

type rpcHumanInputClock struct{}

func (rpcHumanInputClock) Now() time.Time {
	return time.Date(2026, 5, 27, 10, 0, 0, 0, time.UTC)
}

type memoryHumanInputRepo struct {
	records map[humaninputdomain.RequestID]humaninputdomain.Request
}

func newMemoryRepo() *memoryHumanInputRepo {
	return &memoryHumanInputRepo{records: map[humaninputdomain.RequestID]humaninputdomain.Request{}}
}

func (r *memoryHumanInputRepo) CreatePending(_ context.Context, req humaninputdomain.Request) (humaninputdomain.Request, error) {
	r.records[req.ID] = req
	return req, nil
}

func (r *memoryHumanInputRepo) Get(_ context.Context, id humaninputdomain.RequestID) (humaninputdomain.Request, error) {
	return r.records[id], nil
}

func (r *memoryHumanInputRepo) FindByIdempotency(context.Context, string, string, humaninputdomain.ToolCallID, string) (humaninputdomain.Request, error) {
	return humaninputdomain.Request{}, nil
}

func (r *memoryHumanInputRepo) ListPending(_ context.Context, filter humaninputdomain.Filter) ([]humaninputdomain.Request, error) {
	out := []humaninputdomain.Request{}
	for _, req := range r.records {
		if req.Status != humaninputdomain.StatusPending {
			continue
		}
		if filter.ChannelID != "" && req.ChannelID != filter.ChannelID {
			continue
		}
		if filter.SpaceID != "" && req.SpaceID != filter.SpaceID {
			continue
		}
		if filter.MemberID != "" && req.AskerMemberID != filter.MemberID {
			continue
		}
		out = append(out, req)
	}
	return out, nil
}

func (r *memoryHumanInputRepo) Resolve(_ context.Context, mutation humaninputdomain.ResolveMutation) (humaninputdomain.Request, error) {
	req := r.records[mutation.ID]
	req.Status = mutation.Status
	req.Result = mutation.Result
	req.Version++
	req.ResolvedAt = &mutation.ResolvedAt
	r.records[mutation.ID] = req
	return req, nil
}

func (r *memoryHumanInputRepo) ExpireDue(context.Context, time.Time, int) (humaninputdomain.ExpireBatch, error) {
	return humaninputdomain.ExpireBatch{}, nil
}

func (r *memoryHumanInputRepo) AbortByToolCall(context.Context, humaninputdomain.ToolCallID, string) (humaninputdomain.Request, error) {
	return humaninputdomain.Request{}, nil
}
