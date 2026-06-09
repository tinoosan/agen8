package app

import (
	"context"
	"testing"
	"time"

	credentialdomain "github.com/tinoosan/agen8/internal/services/credential/domain"
)

func TestCreateCredentialStoresMetadataAndMaterial(t *testing.T) {
	ctx := context.Background()
	repo := newMemoryRepository()
	service := newTestService(t, repo)

	credential, err := service.CreateCredential(ctx, CreateCredentialInput{
		Kind:  credentialdomain.KindAPIKey,
		Label: "Alpha Vantage",
		Secrets: map[string]string{
			"paramName": "apikey",
			"value":     "secret-value",
		},
	})
	if err != nil {
		t.Fatalf("CreateCredential: %v", err)
	}
	if credential.Kind() != credentialdomain.KindAPIKey || credential.Status() != credentialdomain.StatusActive {
		t.Fatalf("credential=%+v", credential.Record())
	}
	if got := repo.material[credential.ID()].StorageKind; got != credentialdomain.StorageLocalEncrypted {
		t.Fatalf("storageKind=%q", got)
	}

	resolved, err := service.ResolveCredential(ctx, ResolveCredentialInput{
		CredentialID: credential.ID(),
		Purpose:      credentialdomain.PurposeHTTPTool,
	})
	if err != nil {
		t.Fatalf("ResolveCredential: %v", err)
	}
	if resolved.Values["value"] != "secret-value" || resolved.Values["paramName"] != "apikey" {
		t.Fatalf("resolved=%+v", resolved.Values)
	}
}

func TestCreateCredentialCanonicalizesAlpacaHeaderAliases(t *testing.T) {
	ctx := context.Background()
	service := newTestService(t, newMemoryRepository())

	credential, err := service.CreateCredential(ctx, CreateCredentialInput{
		Kind:  credentialdomain.KindAPIKey,
		Label: "Alpaca Paper API Key",
		Secrets: map[string]string{
			"host":       "paper-api.alpaca.markets/v2",
			"injection":  "header",
			"headerName": "ALPACA_PAPER_API_KEY",
			"value":      "secret-value",
		},
	})
	if err != nil {
		t.Fatalf("CreateCredential: %v", err)
	}

	resolved, err := service.ResolveCredential(ctx, ResolveCredentialInput{
		CredentialID: credential.ID(),
		Purpose:      credentialdomain.PurposeHTTPTool,
	})
	if err != nil {
		t.Fatalf("ResolveCredential: %v", err)
	}
	if got := resolved.Values["headerName"]; got != "APCA-API-KEY-ID" {
		t.Fatalf("headerName=%q want APCA-API-KEY-ID", got)
	}
}

func TestCreateCredentialRejectsInvalidHTTPHeaderName(t *testing.T) {
	ctx := context.Background()
	service := newTestService(t, newMemoryRepository())

	_, err := service.CreateCredential(ctx, CreateCredentialInput{
		Kind:  credentialdomain.KindAPIKey,
		Label: "Bad Header",
		Secrets: map[string]string{
			"host":       "api.example.com",
			"injection":  "header",
			"headerName": "Bad Header",
			"value":      "secret-value",
		},
	})
	if err == nil {
		t.Fatal("expected invalid headerName error")
	}
	if got := err.Error(); got != `api_key headerName "Bad Header" is not a valid HTTP header name` {
		t.Fatalf("error=%q", got)
	}
}

func TestUpdateCredentialCanonicalizesAlpacaSecretHeaderAlias(t *testing.T) {
	ctx := context.Background()
	service := newTestService(t, newMemoryRepository())
	credential, err := service.CreateCredential(ctx, CreateCredentialInput{
		Kind:    credentialdomain.KindAPIKey,
		Label:   "Alpaca Secret Key",
		Secrets: map[string]string{"value": "old-secret"},
	})
	if err != nil {
		t.Fatalf("CreateCredential: %v", err)
	}

	if _, err := service.UpdateCredential(ctx, UpdateCredentialInput{
		ID: credential.ID(),
		Secrets: map[string]string{
			"host":       "paper-api.alpaca.markets/v2",
			"injection":  "header",
			"headerName": "APCA_PAPER_SECRET_KEY",
			"value":      "new-secret",
		},
	}); err != nil {
		t.Fatalf("UpdateCredential: %v", err)
	}
	resolved, err := service.ResolveCredential(ctx, ResolveCredentialInput{
		CredentialID: credential.ID(),
		Purpose:      credentialdomain.PurposeHTTPTool,
	})
	if err != nil {
		t.Fatalf("ResolveCredential: %v", err)
	}
	if got := resolved.Values["headerName"]; got != "APCA-API-SECRET-KEY" {
		t.Fatalf("headerName=%q want APCA-API-SECRET-KEY", got)
	}
}

func TestResolveRejectsWrongPurpose(t *testing.T) {
	ctx := context.Background()
	service := newTestService(t, newMemoryRepository())
	credential, err := service.CreateCredential(ctx, CreateCredentialInput{
		Kind:    credentialdomain.KindAPIKey,
		Label:   "API",
		Secrets: map[string]string{"value": "secret"},
	})
	if err != nil {
		t.Fatalf("CreateCredential: %v", err)
	}

	_, err = service.ResolveCredential(ctx, ResolveCredentialInput{
		CredentialID: credential.ID(),
		Purpose:      credentialdomain.PurposeLocationSSH,
	})
	if err == nil {
		t.Fatal("expected wrong-purpose error")
	}
}

func TestCreateSSHAgentUsesSSHAgentStorageWithoutSecrets(t *testing.T) {
	ctx := context.Background()
	repo := newMemoryRepository()
	service := newTestService(t, repo)

	credential, err := service.CreateCredential(ctx, CreateCredentialInput{
		Kind:  credentialdomain.KindSSHAgent,
		Label: "Default agent",
	})
	if err != nil {
		t.Fatalf("CreateCredential: %v", err)
	}
	material := repo.material[credential.ID()]
	if material.StorageKind != credentialdomain.StorageSSHAgent || len(material.Payload) != 0 {
		t.Fatalf("material=%+v", material)
	}
}

func TestResolveRejectsDisabledCredential(t *testing.T) {
	ctx := context.Background()
	service := newTestService(t, newMemoryRepository())
	credential, err := service.CreateCredential(ctx, CreateCredentialInput{
		Kind:    credentialdomain.KindAPIKey,
		Label:   "API",
		Secrets: map[string]string{"value": "secret"},
	})
	if err != nil {
		t.Fatalf("CreateCredential: %v", err)
	}
	if _, err := service.UpdateCredential(ctx, UpdateCredentialInput{
		ID:     credential.ID(),
		Status: credentialdomain.StatusDisabled,
	}); err != nil {
		t.Fatalf("UpdateCredential: %v", err)
	}

	_, err = service.ResolveCredential(ctx, ResolveCredentialInput{
		CredentialID: credential.ID(),
		Purpose:      credentialdomain.PurposeHTTPTool,
	})
	if err == nil {
		t.Fatal("expected disabled credential error")
	}
}

func newTestService(t *testing.T, repo credentialdomain.Repository) *Service {
	t.Helper()
	service, err := NewService(Config{
		Repository: repo,
		Clock:      fixedClock{now: time.Date(2026, 5, 19, 12, 0, 0, 0, time.UTC)},
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return service
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
