package rpc

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/tinoosan/agen8/internal/caller"
	credentialapp "github.com/tinoosan/agen8/internal/services/credential/app"
	credentialdomain "github.com/tinoosan/agen8/internal/services/credential/domain"
)

func TestRegisterCredentialDispatchCreateListGetAndDelete(t *testing.T) {
	reg := NewRegistry()
	svc := newRPCCredentialService(t)
	if err := RegisterCredential(reg, svc); err != nil {
		t.Fatalf("RegisterCredential: %v", err)
	}
	server, err := NewServer(reg)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	createResp := server.Dispatch(context.Background(), Request{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`"1"`),
		Method:  MethodCredentialCreate,
		Params:  json.RawMessage(`{"kind":"api_key","label":"Alpha","secrets":{"paramName":"apikey","value":"secret"}}`),
	})
	if createResp.Error != nil {
		t.Fatalf("create error=%+v", createResp.Error)
	}
	var created struct {
		Credential struct {
			ID     string `json:"id"`
			Kind   string `json:"kind"`
			Fields []struct {
				Name string `json:"name"`
				Kind string `json:"kind"`
			} `json:"fields"`
		} `json:"credential"`
	}
	if err := json.Unmarshal(createResp.Result, &created); err != nil {
		t.Fatalf("decode create: %v", err)
	}
	if created.Credential.ID == "" || created.Credential.Kind != "api_key" {
		t.Fatalf("created=%+v", created)
	}

	listResp := server.Dispatch(context.Background(), Request{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`"2"`),
		Method:  MethodCredentialList,
		Params:  json.RawMessage(`{}`),
	})
	if listResp.Error != nil {
		t.Fatalf("list error=%+v", listResp.Error)
	}

	getResp := server.Dispatch(context.Background(), Request{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`"3"`),
		Method:  MethodCredentialGet,
		Params:  json.RawMessage(`{"credentialId":"` + created.Credential.ID + `"}`),
	})
	if getResp.Error != nil {
		t.Fatalf("get error=%+v", getResp.Error)
	}

	deleteResp := server.Dispatch(context.Background(), Request{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`"4"`),
		Method:  MethodCredentialDelete,
		Params:  json.RawMessage(`{"credentialId":"` + created.Credential.ID + `"}`),
	})
	if deleteResp.Error != nil {
		t.Fatalf("delete error=%+v", deleteResp.Error)
	}
}

func TestRegisterCredentialMapsInvalidParams(t *testing.T) {
	reg := NewRegistry()
	if err := RegisterCredential(reg, newRPCCredentialService(t)); err != nil {
		t.Fatalf("RegisterCredential: %v", err)
	}
	server, err := NewServer(reg)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	resp := server.Dispatch(context.Background(), Request{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`"1"`),
		Method:  MethodCredentialGet,
		Params:  json.RawMessage(`{}`),
	})
	if resp.Error == nil || resp.Error.Code != CodeInvalidParams {
		t.Fatalf("error=%+v want invalid params", resp.Error)
	}
}

func newRPCCredentialService(t *testing.T) *credentialapp.Service {
	t.Helper()
	svc, err := credentialapp.NewService(credentialapp.Config{
		Repository: newRPCCredentialRepo(),
		Clock:      rpcCredentialClock{now: time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC)},
		Caller:     rpcCredentialCaller{userID: "user-a"},
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return svc
}

type rpcCredentialCaller struct{ userID string }

func (c rpcCredentialCaller) ResolveCaller(context.Context) (caller.Caller, error) {
	return caller.Caller{UserID: c.userID}, nil
}

type rpcCredentialClock struct {
	now time.Time
}

func (c rpcCredentialClock) Now() time.Time {
	return c.now
}

type rpcCredentialRepo struct {
	records  map[credentialdomain.ID]credentialdomain.Record
	material map[credentialdomain.ID]credentialdomain.SecretMaterial
}

func newRPCCredentialRepo() *rpcCredentialRepo {
	return &rpcCredentialRepo{
		records:  map[credentialdomain.ID]credentialdomain.Record{},
		material: map[credentialdomain.ID]credentialdomain.SecretMaterial{},
	}
}

func (r *rpcCredentialRepo) Get(_ context.Context, id credentialdomain.ID) (credentialdomain.Record, error) {
	record, ok := r.records[id]
	if !ok {
		return credentialdomain.Record{}, credentialdomain.ErrNotFound
	}
	return record, nil
}

func (r *rpcCredentialRepo) List(_ context.Context, filter credentialdomain.Filter) ([]credentialdomain.Record, error) {
	var out []credentialdomain.Record
	for _, record := range r.records {
		if record.UserID != filter.Scope.UserID || record.ProjectID != "" {
			continue
		}
		if filter.Kind != "" && record.Kind != filter.Kind {
			continue
		}
		if filter.Status != "" && record.Status != filter.Status {
			continue
		}
		out = append(out, record)
	}
	return out, nil
}

func (r *rpcCredentialRepo) Save(_ context.Context, record credentialdomain.Record) (credentialdomain.Record, error) {
	r.records[record.ID] = record
	return record, nil
}

func (r *rpcCredentialRepo) Delete(_ context.Context, id credentialdomain.ID) error {
	if _, ok := r.records[id]; !ok {
		return credentialdomain.ErrNotFound
	}
	delete(r.records, id)
	return nil
}

func (r *rpcCredentialRepo) PutMaterial(_ context.Context, material credentialdomain.SecretMaterial) error {
	r.material[material.CredentialID] = material
	return nil
}

func (r *rpcCredentialRepo) GetMaterial(_ context.Context, id credentialdomain.ID) (credentialdomain.SecretMaterial, error) {
	material, ok := r.material[id]
	if !ok {
		return credentialdomain.SecretMaterial{}, credentialdomain.ErrNotFound
	}
	return material, nil
}

func (r *rpcCredentialRepo) DeleteMaterial(_ context.Context, id credentialdomain.ID) error {
	if _, ok := r.material[id]; !ok {
		return credentialdomain.ErrNotFound
	}
	delete(r.material, id)
	return nil
}

var _ credentialdomain.Repository = (*rpcCredentialRepo)(nil)
