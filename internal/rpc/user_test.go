package rpc

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	userapp "github.com/tinoosan/agen8/internal/services/user/app"
	user "github.com/tinoosan/agen8/internal/services/user/domain"
	userrpc "github.com/tinoosan/agen8/internal/services/user/rpc"
)

var rpcUserTestNow = time.Date(2026, 5, 17, 15, 0, 0, 0, time.UTC)

func TestRegisterUserDispatchStatusWithoutIdentity(t *testing.T) {
	server := newUserRPCServer(t, newRPCUserRepo())

	raw, err := server.Handle(context.Background(), []byte(`{
		"jsonrpc": "2.0",
		"id": "1",
		"method": "user.status"
	}`))
	if err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}
	resp := decodeUserRPCResponse(t, raw)
	if resp.Error != nil {
		t.Fatalf("response error=%+v", resp.Error)
	}
	var result userrpc.StatusResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if !result.SetupOpen {
		t.Fatal("expected setup open")
	}
	if result.User != nil {
		t.Fatalf("user=%#v want nil", result.User)
	}
}

func TestRegisterUserDispatchGetCurrentUser(t *testing.T) {
	record := rpcUserRecord(t, "user-1", user.RoleAdmin)
	server := newUserRPCServer(t, newRPCUserRepo(record))
	ctx := ContextWithIdentity(context.Background(), Identity{
		UserID: "user-1",
		Role:   "admin",
	})

	raw, err := server.Handle(ctx, []byte(`{
		"jsonrpc": "2.0",
		"id": "1",
		"method": "user.get",
		"params": {}
	}`))
	if err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}
	resp := decodeUserRPCResponse(t, raw)
	if resp.Error != nil {
		t.Fatalf("response error=%+v", resp.Error)
	}
	var result userrpc.GetResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if result.User.ID != "user-1" || result.User.Role != "admin" {
		t.Fatalf("user=%+v want current admin user", result.User)
	}
}

func TestRegisterUserUpdateProfileRequiresIdentity(t *testing.T) {
	record := rpcUserRecord(t, "user-1", user.RoleUser)
	server := newUserRPCServer(t, newRPCUserRepo(record))

	raw, err := server.Handle(context.Background(), []byte(`{
		"jsonrpc": "2.0",
		"id": "1",
		"method": "user.updateProfile",
		"params": { "name": "New Name" }
	}`))
	if err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}
	resp := decodeUserRPCResponse(t, raw)
	if resp.Error == nil || resp.Error.Code != CodeInvalidRequest {
		t.Fatalf("response error=%+v want invalid request", resp.Error)
	}
}

func TestRegisterUserAdminCanSuspend(t *testing.T) {
	record := rpcUserRecord(t, "user-1", user.RoleUser)
	server := newUserRPCServer(t, newRPCUserRepo(record))
	ctx := ContextWithIdentity(context.Background(), Identity{
		UserID: "admin-1",
		Role:   "admin",
	})

	raw, err := server.Handle(ctx, []byte(`{
		"jsonrpc": "2.0",
		"id": "1",
		"method": "user.suspend",
		"params": { "userId": "user-1" }
	}`))
	if err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}
	resp := decodeUserRPCResponse(t, raw)
	if resp.Error != nil {
		t.Fatalf("response error=%+v", resp.Error)
	}
	var result userrpc.SuspendResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if result.User.Lifecycle != "suspended" {
		t.Fatalf("lifecycle=%q want suspended", result.User.Lifecycle)
	}
}

func TestRegisterUserCloseRejectsOtherUserForNonAdmin(t *testing.T) {
	actor := rpcUserRecord(t, "user-1", user.RoleUser)
	other := rpcUserRecord(t, "user-2", user.RoleUser)
	server := newUserRPCServer(t, newRPCUserRepo(actor, other))
	ctx := ContextWithIdentity(context.Background(), Identity{
		UserID: "user-1",
		Role:   "user",
	})

	raw, err := server.Handle(ctx, []byte(`{
		"jsonrpc": "2.0",
		"id": "1",
		"method": "user.close",
		"params": { "userId": "user-2" }
	}`))
	if err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}
	resp := decodeUserRPCResponse(t, raw)
	if resp.Error == nil || resp.Error.Code != CodeInvalidParams {
		t.Fatalf("response error=%+v want invalid params", resp.Error)
	}
}

func newUserRPCServer(t *testing.T, repo user.Repository) *Server {
	t.Helper()
	svc, err := userapp.NewService(repo, user.FixedClock{T: rpcUserTestNow}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	reg := NewRegistry()
	if err := RegisterUser(reg, svc); err != nil {
		t.Fatalf("RegisterUser returned error: %v", err)
	}
	server, err := NewServer(reg)
	if err != nil {
		t.Fatalf("NewServer returned error: %v", err)
	}
	return server
}

func decodeUserRPCResponse(t *testing.T, raw []byte) Response {
	t.Helper()
	var resp Response
	if err := json.Unmarshal(raw, &resp); err != nil {
		t.Fatalf("unmarshal response %s: %v", string(raw), err)
	}
	return resp
}

type rpcUserRepo struct {
	users map[string]user.User
}

func newRPCUserRepo(records ...user.User) *rpcUserRepo {
	repo := &rpcUserRepo{users: map[string]user.User{}}
	for _, record := range records {
		repo.users[record.ID.String()] = record
	}
	return repo
}

func (r *rpcUserRepo) Get(_ context.Context, id user.ID) (user.User, error) {
	record, ok := r.users[id.String()]
	if !ok {
		return user.User{}, user.ErrNotFound
	}
	return record, nil
}

func (r *rpcUserRepo) GetByEmail(_ context.Context, email string) (user.User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	for _, record := range r.users {
		if record.Email == email {
			return record, nil
		}
	}
	return user.User{}, user.ErrNotFound
}

func (r *rpcUserRepo) FirstActive(context.Context) (user.User, error) {
	for _, record := range r.users {
		if record.IsActive() {
			return record, nil
		}
	}
	return user.User{}, user.ErrNotFound
}

func (r *rpcUserRepo) Count(context.Context) (int, error) {
	return len(r.users), nil
}

func (r *rpcUserRepo) Create(_ context.Context, record user.User) error {
	r.users[record.ID.String()] = record
	return nil
}

func (r *rpcUserRepo) Update(_ context.Context, record user.User) error {
	if _, ok := r.users[record.ID.String()]; !ok {
		return user.ErrNotFound
	}
	r.users[record.ID.String()] = record
	return nil
}

func rpcUserRecord(t *testing.T, rawID string, role user.Role) user.User {
	t.Helper()
	id, err := user.NewID(rawID)
	if err != nil {
		t.Fatalf("NewID: %v", err)
	}
	record, err := user.New(user.NewInput{
		ID:    id,
		Email: rawID + "@example.com",
		Name:  "Test User",
		Role:  role,
		Now:   rpcUserTestNow,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return record
}
