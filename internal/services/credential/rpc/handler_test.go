package rpc

import (
	"context"
	"testing"
	"time"

	credentialapp "github.com/tinoosan/agen8-mcp-server/internal/services/credential/app"
	credentialdomain "github.com/tinoosan/agen8-mcp-server/internal/services/credential/domain"
)

func TestCredentialCreateReturnsRedactedView(t *testing.T) {
	handler := newTestHandler(t)
	result, err := handler.CredentialCreate(context.Background(), CredentialCreateParams{
		Kind:  string(credentialdomain.KindAPIKey),
		Label: "Alpha Vantage",
		Secrets: map[string]string{
			"paramName": "apikey",
			"value":     "secret-value",
		},
	})
	if err != nil {
		t.Fatalf("CredentialCreate: %v", err)
	}
	if result.Credential.ID == "" || result.Credential.Kind != string(credentialdomain.KindAPIKey) {
		t.Fatalf("credential=%+v", result.Credential)
	}
	if len(result.Credential.Fields) != 2 {
		t.Fatalf("fields=%+v", result.Credential.Fields)
	}
	for _, field := range result.Credential.Fields {
		if field.Name == "value" && field.Kind != string(credentialdomain.FieldSecret) {
			t.Fatalf("value field not secret: %+v", field)
		}
	}
}

func TestCredentialCreateRequiresKind(t *testing.T) {
	handler := newTestHandler(t)
	_, err := handler.CredentialCreate(context.Background(), CredentialCreateParams{})
	if err == nil {
		t.Fatal("expected error")
	}
	if code := err.(interface{ RPCCode() int }).RPCCode(); code != -32602 {
		t.Fatalf("code=%d", code)
	}
}

func TestCredentialGetRequiresCredentialID(t *testing.T) {
	handler := newTestHandler(t)
	_, err := handler.CredentialGet(context.Background(), CredentialGetParams{})
	if err == nil {
		t.Fatal("expected error")
	}
	if code := err.(interface{ RPCCode() int }).RPCCode(); code != -32602 {
		t.Fatalf("code=%d", code)
	}
}

func newTestHandler(t *testing.T) *Handler {
	t.Helper()
	service, err := credentialapp.NewService(credentialapp.Config{
		Repository: newMemoryRepository(),
		Clock:      fixedClock{now: time.Date(2026, 5, 19, 12, 0, 0, 0, time.UTC)},
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	handler, err := NewHandler(service)
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}
	return handler
}

type fixedClock struct {
	now time.Time
}

func (f fixedClock) Now() time.Time {
	return f.now
}

type memoryRepository struct {
	records  map[credentialdomain.ID]credentialdomain.Record
	material map[credentialdomain.ID]credentialdomain.SecretMaterial
}

func newMemoryRepository() *memoryRepository {
	return &memoryRepository{
		records:  map[credentialdomain.ID]credentialdomain.Record{},
		material: map[credentialdomain.ID]credentialdomain.SecretMaterial{},
	}
}

func (r *memoryRepository) Get(_ context.Context, id credentialdomain.ID) (credentialdomain.Record, error) {
	record, ok := r.records[id]
	if !ok {
		return credentialdomain.Record{}, credentialdomain.ErrNotFound
	}
	return record, nil
}

func (r *memoryRepository) List(_ context.Context, filter credentialdomain.Filter) ([]credentialdomain.Record, error) {
	var out []credentialdomain.Record
	for _, record := range r.records {
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

func (r *memoryRepository) Save(_ context.Context, record credentialdomain.Record) (credentialdomain.Record, error) {
	r.records[record.ID] = record
	return record, nil
}

func (r *memoryRepository) Delete(_ context.Context, id credentialdomain.ID) error {
	if _, ok := r.records[id]; !ok {
		return credentialdomain.ErrNotFound
	}
	delete(r.records, id)
	return nil
}

func (r *memoryRepository) PutMaterial(_ context.Context, material credentialdomain.SecretMaterial) error {
	r.material[material.CredentialID] = material
	return nil
}

func (r *memoryRepository) GetMaterial(_ context.Context, id credentialdomain.ID) (credentialdomain.SecretMaterial, error) {
	material, ok := r.material[id]
	if !ok {
		return credentialdomain.SecretMaterial{}, credentialdomain.ErrNotFound
	}
	return material, nil
}

func (r *memoryRepository) DeleteMaterial(_ context.Context, id credentialdomain.ID) error {
	if _, ok := r.material[id]; !ok {
		return credentialdomain.ErrNotFound
	}
	delete(r.material, id)
	return nil
}

var _ credentialdomain.Repository = (*memoryRepository)(nil)
