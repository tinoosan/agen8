package rpc

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/tinoosan/agen8/internal/services/auth/apikey"
	authapp "github.com/tinoosan/agen8/internal/services/auth/app"
	auth "github.com/tinoosan/agen8/internal/services/auth/domain"
	"github.com/tinoosan/agen8/internal/services/auth/linktoken"
	"github.com/tinoosan/agen8/internal/services/auth/password"
	authrpc "github.com/tinoosan/agen8/internal/services/auth/rpc"
	"github.com/tinoosan/agen8/internal/services/auth/session"
	user "github.com/tinoosan/agen8/internal/services/user/domain"
)

var rpcAuthTestNow = time.Date(2026, 5, 17, 18, 0, 0, 0, time.UTC)

func TestRegisterAuthDispatchLogin(t *testing.T) {
	account := rpcAuthUserRecord(t, "user-1", user.LifecycleActive)
	server, svc := newAuthRPCServer(t, account)
	if err := svc.CreatePassword(context.Background(), authapp.CreatePasswordParams{
		UserID:   account.ID,
		Password: "valid-password",
	}); err != nil {
		t.Fatalf("CreatePassword: %v", err)
	}

	raw, err := server.Handle(context.Background(), []byte(`{
		"jsonrpc": "2.0",
		"id": "1",
		"method": "auth.login",
		"params": { "email": " USER-1@example.COM ", "password": "valid-password" }
	}`))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	resp := decodeAuthRPCResponse(t, raw)
	if resp.Error != nil {
		t.Fatalf("response error=%+v", resp.Error)
	}
	var result authrpc.LoginResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if result.UserID != "user-1" || result.Token == "" {
		t.Fatalf("login result=%+v", result)
	}
}

func TestRegisterAuthLoginRejectsBadPassword(t *testing.T) {
	account := rpcAuthUserRecord(t, "user-1", user.LifecycleActive)
	server, svc := newAuthRPCServer(t, account)
	if err := svc.CreatePassword(context.Background(), authapp.CreatePasswordParams{
		UserID:   account.ID,
		Password: "valid-password",
	}); err != nil {
		t.Fatalf("CreatePassword: %v", err)
	}

	raw, err := server.Handle(context.Background(), []byte(`{
		"jsonrpc": "2.0",
		"id": "1",
		"method": "auth.login",
		"params": { "email": "user-1@example.com", "password": "wrong-password" }
	}`))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	resp := decodeAuthRPCResponse(t, raw)
	if resp.Error == nil || resp.Error.Code != CodeInvalidParams {
		t.Fatalf("response error=%+v want invalid params for invalid credentials", resp.Error)
	}
}

func TestRegisterAuthAPIKeyCreateRequiresIdentity(t *testing.T) {
	account := rpcAuthUserRecord(t, "user-1", user.LifecycleActive)
	server, _ := newAuthRPCServer(t, account)

	raw, err := server.Handle(context.Background(), []byte(`{
		"jsonrpc": "2.0",
		"id": "1",
		"method": "auth.apiKey.create",
		"params": { "name": "CLI" }
	}`))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	resp := decodeAuthRPCResponse(t, raw)
	if resp.Error == nil || resp.Error.Code != CodeInvalidRequest {
		t.Fatalf("response error=%+v want invalid request", resp.Error)
	}
}

func TestRegisterAuthAPIKeyCreate(t *testing.T) {
	account := rpcAuthUserRecord(t, "user-1", user.LifecycleActive)
	server, _ := newAuthRPCServer(t, account)
	ctx := ContextWithIdentity(context.Background(), Identity{UserID: "user-1", Role: "admin"})

	raw, err := server.Handle(ctx, []byte(`{
		"jsonrpc": "2.0",
		"id": "1",
		"method": "auth.apiKey.create",
		"params": { "name": "CLI" }
	}`))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	resp := decodeAuthRPCResponse(t, raw)
	if resp.Error != nil {
		t.Fatalf("response error=%+v", resp.Error)
	}
	var result authrpc.CreateAPIKeyResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if result.ID == "" || result.Token == "" || result.Name != "CLI" {
		t.Fatalf("api key result=%+v", result)
	}
}

func TestRegisterAuthAPIKeyList(t *testing.T) {
	account := rpcAuthUserRecord(t, "user-1", user.LifecycleActive)
	server, _ := newAuthRPCServer(t, account)
	ctx := ContextWithIdentity(context.Background(), Identity{UserID: "user-1", Role: "admin"})
	createAPIKeyOverRPC(t, ctx, server, "first")
	createAPIKeyOverRPC(t, ctx, server, "second")

	raw, err := server.Handle(ctx, []byte(`{
		"jsonrpc": "2.0",
		"id": "1",
		"method": "auth.apiKey.list",
		"params": {}
	}`))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	resp := decodeAuthRPCResponse(t, raw)
	if resp.Error != nil {
		t.Fatalf("response error=%+v", resp.Error)
	}
	var result authrpc.ListAPIKeysResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if len(result.Keys) != 2 {
		t.Fatalf("len(keys)=%d want 2", len(result.Keys))
	}
	for _, key := range result.Keys {
		if key.ID == "" || key.Prefix == "" || key.CreatedAt.IsZero() || !key.Active {
			t.Fatalf("invalid key view=%+v", key)
		}
	}
}

func TestRegisterAuthAPIKeyRevokeRejectsOtherUsersKey(t *testing.T) {
	account := rpcAuthUserRecord(t, "user-1", user.LifecycleActive)
	other := rpcAuthUserRecord(t, "user-2", user.LifecycleActive)
	server, _ := newAuthRPCServer(t, account, other)
	otherCtx := ContextWithIdentity(context.Background(), Identity{UserID: "user-2", Role: "admin"})
	otherKey := createAPIKeyOverRPC(t, otherCtx, server, "other")

	raw, err := server.Handle(ContextWithIdentity(context.Background(), Identity{UserID: "user-1", Role: "admin"}), []byte(`{
		"jsonrpc": "2.0",
		"id": "1",
		"method": "auth.apiKey.revoke",
		"params": { "id": "`+otherKey.ID+`" }
	}`))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	resp := decodeAuthRPCResponse(t, raw)
	if resp.Error == nil || !strings.Contains(resp.Error.Message, "token not found") {
		t.Fatalf("response error=%+v want token not found", resp.Error)
	}
}

func createAPIKeyOverRPC(t *testing.T, ctx context.Context, server *Server, name string) authrpc.CreateAPIKeyResult {
	t.Helper()
	raw, err := server.Handle(ctx, []byte(`{
		"jsonrpc": "2.0",
		"id": "create",
		"method": "auth.apiKey.create",
		"params": { "name": "`+name+`" }
	}`))
	if err != nil {
		t.Fatalf("Handle create: %v", err)
	}
	resp := decodeAuthRPCResponse(t, raw)
	if resp.Error != nil {
		t.Fatalf("create response error=%+v", resp.Error)
	}
	var result authrpc.CreateAPIKeyResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal create result: %v", err)
	}
	return result
}

func newAuthRPCServer(t *testing.T, users ...user.User) (*Server, *authapp.Service) {
	t.Helper()
	svc, err := authapp.NewService(
		&rpcAuthPasswordRepo{records: map[string]password.Credential{}},
		&rpcAuthSessionRepo{records: map[string]session.Session{}},
		&rpcAuthAPIKeyRepo{records: map[string]apikey.Key{}},
		&rpcAuthLinkTokenRepo{records: map[string]linktoken.LinkToken{}},
		newRPCAuthUserLoader(users...),
		auth.FixedClock{T: rpcAuthTestNow},
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	reg := NewRegistry()
	if err := RegisterAuth(reg, svc); err != nil {
		t.Fatalf("RegisterAuth: %v", err)
	}
	server, err := NewServer(reg)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	return server, svc
}

func decodeAuthRPCResponse(t *testing.T, raw []byte) Response {
	t.Helper()
	var resp Response
	if err := json.Unmarshal(raw, &resp); err != nil {
		t.Fatalf("unmarshal response %s: %v", string(raw), err)
	}
	return resp
}

type rpcAuthUserLoader struct {
	users map[string]user.User
}

func newRPCAuthUserLoader(records ...user.User) *rpcAuthUserLoader {
	loader := &rpcAuthUserLoader{users: map[string]user.User{}}
	for _, record := range records {
		loader.users[record.ID.String()] = record
	}
	return loader
}

func (l *rpcAuthUserLoader) Get(_ context.Context, id user.ID) (user.User, error) {
	record, ok := l.users[id.String()]
	if !ok {
		return user.User{}, user.ErrNotFound
	}
	return record, nil
}

func (l *rpcAuthUserLoader) GetByEmail(_ context.Context, email string) (user.User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	for _, record := range l.users {
		if record.Email == email {
			return record, nil
		}
	}
	return user.User{}, user.ErrNotFound
}

type rpcAuthPasswordRepo struct {
	records map[string]password.Credential
}

func (r *rpcAuthPasswordRepo) Get(_ context.Context, userID user.ID) (password.Credential, error) {
	record, ok := r.records[userID.String()]
	if !ok {
		return password.Credential{}, auth.ErrTokenNotFound
	}
	return record, nil
}

func (r *rpcAuthPasswordRepo) Save(_ context.Context, record password.Credential) error {
	r.records[record.UserID.String()] = record
	return nil
}

func (r *rpcAuthPasswordRepo) Delete(_ context.Context, userID user.ID) error {
	delete(r.records, userID.String())
	return nil
}

type rpcAuthSessionRepo struct {
	records map[string]session.Session
}

func (r *rpcAuthSessionRepo) GetByTokenHash(_ context.Context, tokenHash string) (session.Session, error) {
	record, ok := r.records[tokenHash]
	if !ok {
		return session.Session{}, auth.ErrTokenNotFound
	}
	return record, nil
}

func (r *rpcAuthSessionRepo) Create(_ context.Context, record session.Session) error {
	r.records[record.TokenHash] = record
	return nil
}

func (r *rpcAuthSessionRepo) Update(_ context.Context, record session.Session) error {
	r.records[record.TokenHash] = record
	return nil
}

type rpcAuthAPIKeyRepo struct {
	records map[string]apikey.Key
}

func (r *rpcAuthAPIKeyRepo) GetByTokenHash(_ context.Context, tokenHash string) (apikey.Key, error) {
	record, ok := r.records[tokenHash]
	if !ok {
		return apikey.Key{}, auth.ErrTokenNotFound
	}
	return record, nil
}

func (r *rpcAuthAPIKeyRepo) Get(_ context.Context, id apikey.ID) (apikey.Key, error) {
	for _, record := range r.records {
		if record.ID == id {
			return record, nil
		}
	}
	return apikey.Key{}, auth.ErrTokenNotFound
}

func (r *rpcAuthAPIKeyRepo) ListByUser(_ context.Context, userID user.ID) ([]apikey.Key, error) {
	var records []apikey.Key
	for _, record := range r.records {
		if record.UserID == userID {
			records = append(records, record)
		}
	}
	return records, nil
}

func (r *rpcAuthAPIKeyRepo) Create(_ context.Context, record apikey.Key) error {
	r.records[record.TokenHash] = record
	return nil
}

func (r *rpcAuthAPIKeyRepo) Update(_ context.Context, record apikey.Key) error {
	r.records[record.TokenHash] = record
	return nil
}

type rpcAuthLinkTokenRepo struct {
	records map[string]linktoken.LinkToken
}

func (r *rpcAuthLinkTokenRepo) GetByTokenHash(_ context.Context, tokenHash string) (linktoken.LinkToken, error) {
	record, ok := r.records[tokenHash]
	if !ok {
		return linktoken.LinkToken{}, auth.ErrTokenNotFound
	}
	return record, nil
}

func (r *rpcAuthLinkTokenRepo) Get(_ context.Context, id linktoken.ID) (linktoken.LinkToken, error) {
	for _, record := range r.records {
		if record.ID == id {
			return record, nil
		}
	}
	return linktoken.LinkToken{}, auth.ErrTokenNotFound
}

func (r *rpcAuthLinkTokenRepo) List(_ context.Context, filter linktoken.Filter) ([]linktoken.LinkToken, error) {
	var out []linktoken.LinkToken
	for _, record := range r.records {
		if filter.ProjectID != "" && record.ProjectID != filter.ProjectID {
			continue
		}
		if filter.UserID != "" && record.UserID.String() != filter.UserID {
			continue
		}
		out = append(out, record)
	}
	return out, nil
}

func (r *rpcAuthLinkTokenRepo) Create(_ context.Context, record linktoken.LinkToken) error {
	r.records[record.TokenHash] = record
	return nil
}

func (r *rpcAuthLinkTokenRepo) Update(_ context.Context, record linktoken.LinkToken) error {
	r.records[record.TokenHash] = record
	return nil
}

func rpcAuthUserRecord(t *testing.T, rawID string, lifecycle user.Lifecycle) user.User {
	t.Helper()
	id, err := user.NewID(rawID)
	if err != nil {
		t.Fatalf("NewID: %v", err)
	}
	record, err := user.New(user.NewInput{
		ID:    id,
		Email: rawID + "@example.com",
		Name:  "Test User",
		Role:  user.RoleAdmin,
		Now:   rpcAuthTestNow,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	record.Lifecycle = lifecycle
	return record
}
