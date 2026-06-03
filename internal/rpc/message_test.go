package rpc

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	messageapp "github.com/tinoosan/agen8-mcp-server/internal/services/message/app"
	messageinfra "github.com/tinoosan/agen8-mcp-server/internal/services/message/infra"
	messagerpc "github.com/tinoosan/agen8-mcp-server/internal/services/message/rpc"
	"github.com/tinoosan/agen8-mcp-server/internal/services/space/domain/member"
	storagedb "github.com/tinoosan/agen8-mcp-server/internal/storage/db"
)

func TestRegisterMessageDispatchSendAndGet(t *testing.T) {
	messageSvc, memberSvc := newRPCMessageServices(t)
	reg := NewRegistry()
	if err := RegisterMessage(reg, messageSvc, memberSvc); err != nil {
		t.Fatalf("RegisterMessage returned error: %v", err)
	}
	server, err := NewServer(reg)
	if err != nil {
		t.Fatalf("NewServer returned error: %v", err)
	}
	ctx := ContextWithIdentity(context.Background(), Identity{
		UserID:   "user-1",
		MemberID: "member-source",
	})

	raw, err := server.Handle(ctx, []byte(`{
		"jsonrpc": "2.0",
		"id": "1",
		"method": "message.send",
		"params": {
			"spaceId": "space-1",
			"destinationMemberId": "member-dest",
			"kind": "inform",
			"subject": "Status",
			"body": { "text": "ready" },
			"intentId": "intent-rpc-send",
			"producer": "rpc-test"
		}
	}`))
	if err != nil {
		t.Fatalf("Handle send returned error: %v", err)
	}
	resp := decodeRPCResponse(t, raw)
	if resp.Error != nil {
		t.Fatalf("send response error=%+v", resp.Error)
	}
	var sent messagerpc.MessageSendResult
	if err := json.Unmarshal(resp.Result, &sent); err != nil {
		t.Fatalf("unmarshal send result: %v", err)
	}
	if sent.Message.MessageID == "" {
		t.Fatalf("send result missing message id: %+v", sent.Message)
	}

	raw, err = server.Handle(ctx, []byte(fmt.Sprintf(`{
		"jsonrpc": "2.0",
		"id": "2",
		"method": "message.get",
		"params": { "messageId": %q }
	}`, sent.Message.MessageID)))
	if err != nil {
		t.Fatalf("Handle get returned error: %v", err)
	}
	resp = decodeRPCResponse(t, raw)
	if resp.Error != nil {
		t.Fatalf("get response error=%+v", resp.Error)
	}
	var got messagerpc.MessageGetResult
	if err := json.Unmarshal(resp.Result, &got); err != nil {
		t.Fatalf("unmarshal get result: %v", err)
	}
	if got.Message.MessageID != sent.Message.MessageID || got.Message.DestinationMemberLabel != "Destination Agent" {
		t.Fatalf("get result=%+v sent=%+v", got.Message, sent.Message)
	}
}

func TestRegisterMessageGetRequiresIdentity(t *testing.T) {
	messageSvc, memberSvc := newRPCMessageServices(t)
	reg := NewRegistry()
	if err := RegisterMessage(reg, messageSvc, memberSvc); err != nil {
		t.Fatalf("RegisterMessage returned error: %v", err)
	}
	server, err := NewServer(reg)
	if err != nil {
		t.Fatalf("NewServer returned error: %v", err)
	}

	raw, err := server.Handle(context.Background(), []byte(`{
		"jsonrpc": "2.0",
		"id": "1",
		"method": "message.get",
		"params": { "messageId": "msg-1" }
	}`))
	if err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}
	resp := decodeRPCResponse(t, raw)
	if resp.Error == nil || resp.Error.Code != CodeInvalidRequest {
		t.Fatalf("response error=%+v want invalid request", resp.Error)
	}
}

func newRPCMessageServices(t *testing.T) (*messageapp.Service, *rpcMessageMembers) {
	t.Helper()
	handle, err := storagedb.Open(context.Background(), storagedb.Config{
		Driver:  storagedb.DriverSQLite,
		DataDir: filepath.Join(t.TempDir(), "data"),
	})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	repo, err := messageinfra.NewSQLiteRepository(handle)
	if err != nil {
		t.Fatalf("NewSQLiteRepository: %v", err)
	}
	messageSvc, err := messageapp.NewService(messageapp.NewServiceParams{
		Repository: repo,
		Clock:      messageapp.FixedClock{T: time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)},
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	members := newRPCMessageMembers(
		member.Record{ID: "member-source", UserID: "user-1", SpaceID: "space-1", DisplayName: "Source Agent", LifecycleState: member.LifecycleActive},
		member.Record{ID: "member-dest", UserID: "user-1", SpaceID: "space-1", DisplayName: "Destination Agent", LifecycleState: member.LifecycleActive},
	)
	return messageSvc, members
}

type rpcMessageMembers struct {
	members map[string]member.Record
}

func newRPCMessageMembers(members ...member.Record) *rpcMessageMembers {
	repo := &rpcMessageMembers{members: map[string]member.Record{}}
	for _, member := range members {
		repo.members[string(member.ID)] = member
	}
	return repo
}

func (r *rpcMessageMembers) GetMember(_ context.Context, id member.ID) (member.Record, error) {
	rosterMember, ok := r.members[string(id)]
	if !ok {
		return member.Record{}, fmt.Errorf("member %s not found", id)
	}
	return rosterMember, nil
}
