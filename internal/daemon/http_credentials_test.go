package daemon

import (
	"context"
	"testing"
	"time"

	credentialapp "github.com/tinoosan/agen8-mcp-server/internal/services/credential/app"
	credentialdomain "github.com/tinoosan/agen8-mcp-server/internal/services/credential/domain"
)

func TestHTTPCredentialResolverAggregatesAlpacaHeaders(t *testing.T) {
	ctx := context.Background()
	service := newHTTPResolverCredentialService(t)
	if _, err := service.CreateCredential(ctx, credentialapp.CreateCredentialInput{
		Kind:  credentialdomain.KindAPIKey,
		Label: "Alpaca Paper API Key",
		Secrets: map[string]string{
			"host":       "paper-api.alpaca.markets/v2",
			"injection":  "header",
			"headerName": "ALPACA_PAPER_API_KEY",
			"value":      "key-id",
		},
	}); err != nil {
		t.Fatalf("CreateCredential key id: %v", err)
	}
	if _, err := service.CreateCredential(ctx, credentialapp.CreateCredentialInput{
		Kind:  credentialdomain.KindAPIKey,
		Label: "Alpaca Secret Key",
		Secrets: map[string]string{
			"host":       "https://paper-api.alpaca.markets/v2",
			"injection":  "header",
			"headerName": "APCA_PAPER_SECRET_KEY",
			"value":      "secret-key",
		},
	}); err != nil {
		t.Fatalf("CreateCredential secret key: %v", err)
	}

	record, found, err := (httpCredentialResolver{credentials: service}).ResolveHTTP(ctx, "paper-api.alpaca.markets:443")
	if err != nil {
		t.Fatalf("ResolveHTTP: %v", err)
	}
	if !found {
		t.Fatal("expected credential match")
	}
	if got := record.Headers["APCA-API-KEY-ID"]; got != "key-id" {
		t.Fatalf("APCA-API-KEY-ID=%q", got)
	}
	if got := record.Headers["APCA-API-SECRET-KEY"]; got != "secret-key" {
		t.Fatalf("APCA-API-SECRET-KEY=%q", got)
	}
}

func TestHTTPCredentialResolverReturnsFalseWhenNoHostMatches(t *testing.T) {
	ctx := context.Background()
	service := newHTTPResolverCredentialService(t)
	if _, err := service.CreateCredential(ctx, credentialapp.CreateCredentialInput{
		Kind:  credentialdomain.KindAPIKey,
		Label: "Other API",
		Secrets: map[string]string{
			"host":      "api.example.com",
			"injection": "bearer",
			"value":     "token",
		},
	}); err != nil {
		t.Fatalf("CreateCredential: %v", err)
	}

	_, found, err := (httpCredentialResolver{credentials: service}).ResolveHTTP(ctx, "paper-api.alpaca.markets")
	if err != nil {
		t.Fatalf("ResolveHTTP: %v", err)
	}
	if found {
		t.Fatal("expected no credential match")
	}
}

func newHTTPResolverCredentialService(t *testing.T) *credentialapp.Service {
	t.Helper()
	service, err := credentialapp.NewService(credentialapp.Config{
		Repository: newHTTPResolverMemoryRepository(),
		Clock:      httpResolverClock{now: time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)},
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return service
}

type httpResolverClock struct {
	now time.Time
}

func (c httpResolverClock) Now() time.Time {
	return c.now
}

type httpResolverMemoryRepository struct {
	records  map[credentialdomain.ID]credentialdomain.Record
	material map[credentialdomain.ID]credentialdomain.SecretMaterial
}

func newHTTPResolverMemoryRepository() *httpResolverMemoryRepository {
	return &httpResolverMemoryRepository{
		records:  map[credentialdomain.ID]credentialdomain.Record{},
		material: map[credentialdomain.ID]credentialdomain.SecretMaterial{},
	}
}

func (r *httpResolverMemoryRepository) Get(_ context.Context, id credentialdomain.ID) (credentialdomain.Record, error) {
	record, ok := r.records[id]
	if !ok {
		return credentialdomain.Record{}, credentialdomain.ErrNotFound
	}
	return record, nil
}

func (r *httpResolverMemoryRepository) List(_ context.Context, filter credentialdomain.Filter) ([]credentialdomain.Record, error) {
	out := make([]credentialdomain.Record, 0, len(r.records))
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

func (r *httpResolverMemoryRepository) Save(_ context.Context, record credentialdomain.Record) (credentialdomain.Record, error) {
	r.records[record.ID] = record
	return record, nil
}

func (r *httpResolverMemoryRepository) Delete(_ context.Context, id credentialdomain.ID) error {
	if _, ok := r.records[id]; !ok {
		return credentialdomain.ErrNotFound
	}
	delete(r.records, id)
	return nil
}

func (r *httpResolverMemoryRepository) PutMaterial(_ context.Context, material credentialdomain.SecretMaterial) error {
	r.material[material.CredentialID] = material
	return nil
}

func (r *httpResolverMemoryRepository) GetMaterial(_ context.Context, id credentialdomain.ID) (credentialdomain.SecretMaterial, error) {
	material, ok := r.material[id]
	if !ok {
		return credentialdomain.SecretMaterial{}, credentialdomain.ErrNotFound
	}
	return material, nil
}

func (r *httpResolverMemoryRepository) DeleteMaterial(_ context.Context, id credentialdomain.ID) error {
	if _, ok := r.material[id]; !ok {
		return credentialdomain.ErrNotFound
	}
	delete(r.material, id)
	return nil
}

var _ credentialdomain.Repository = (*httpResolverMemoryRepository)(nil)
