package infra

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tinoosan/agen8/internal/config"
	credentialdomain "github.com/tinoosan/agen8/internal/services/credential/domain"
	implstore "github.com/tinoosan/agen8/internal/store"
)

func TestSQLiteRepositorySavesListsGetsAndDeletesCredential(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepository(t)
	now := time.Date(2026, 5, 19, 10, 0, 0, 0, time.UTC)
	record := credentialdomain.Record{
		ID:     "cred_alpha",
		UserID: "user-a",
		Kind:   credentialdomain.KindAPIKey,
		Label:  "Alpha Vantage",
		Status: credentialdomain.StatusActive,
		Fields: []credentialdomain.FieldRef{
			{Name: "paramName", Kind: credentialdomain.FieldPublic},
			{Name: "value", Kind: credentialdomain.FieldSecret},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	saved, err := repo.Save(ctx, record)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if saved.ID != record.ID || saved.Kind != record.Kind || len(saved.Fields) != 2 {
		t.Fatalf("saved=%+v", saved)
	}

	got, err := repo.Get(ctx, record.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Label != "Alpha Vantage" || got.Fields[1].Kind != credentialdomain.FieldSecret {
		t.Fatalf("got=%+v", got)
	}

	listed, err := repo.List(ctx, credentialdomain.Filter{Scope: credentialdomain.Scope{UserID: "user-a"}, Kind: credentialdomain.KindAPIKey})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(listed) != 1 || listed[0].ID != record.ID {
		t.Fatalf("listed=%+v", listed)
	}

	if err := repo.Delete(ctx, record.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := repo.Get(ctx, record.ID); !errors.Is(err, credentialdomain.ErrNotFound) {
		t.Fatalf("Get after delete err=%v want ErrNotFound", err)
	}
}

func TestSQLiteRepositoryEncryptsLocalMaterial(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	repo := newTestRepositoryWithDataDir(t, dataDir)
	now := time.Date(2026, 5, 19, 10, 0, 0, 0, time.UTC)
	saveTestCredential(t, ctx, repo, "cred_secret", credentialdomain.KindAPIKey, now)
	material := credentialdomain.SecretMaterial{
		CredentialID: "cred_secret",
		StorageKind:  credentialdomain.StorageLocalEncrypted,
		Payload:      []byte(`{"value":"super-secret"}`),
		UpdatedAt:    now,
	}

	if err := repo.PutMaterial(ctx, material); err != nil {
		t.Fatalf("PutMaterial: %v", err)
	}
	got, err := repo.GetMaterial(ctx, material.CredentialID)
	if err != nil {
		t.Fatalf("GetMaterial: %v", err)
	}
	if string(got.Payload) != string(material.Payload) {
		t.Fatalf("payload=%q want %q", got.Payload, material.Payload)
	}

	keyPath := filepath.Join(dataDir, "credentials", "local_encrypted.key")
	info, err := os.Stat(keyPath)
	if err != nil {
		t.Fatalf("stat key: %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Fatalf("key permissions=%#o want 0600", mode)
	}
}

func TestSQLiteRepositoryStoresSSHAgentMaterialWithoutPayload(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepository(t)
	now := time.Date(2026, 5, 19, 10, 0, 0, 0, time.UTC)
	saveTestCredential(t, ctx, repo, "cred_agent", credentialdomain.KindSSHAgent, now)

	if err := repo.PutMaterial(ctx, credentialdomain.SecretMaterial{
		CredentialID: "cred_agent",
		StorageKind:  credentialdomain.StorageSSHAgent,
		Payload:      []byte("ignored"),
		UpdatedAt:    now,
	}); err != nil {
		t.Fatalf("PutMaterial: %v", err)
	}
	got, err := repo.GetMaterial(ctx, "cred_agent")
	if err != nil {
		t.Fatalf("GetMaterial: %v", err)
	}
	if got.StorageKind != credentialdomain.StorageSSHAgent || len(got.Payload) != 0 {
		t.Fatalf("got=%+v", got)
	}
}

func TestSQLiteRepositoryListRequiresAndAppliesOwnerScope(t *testing.T) {
	ctx := context.Background()
	repository := newTestRepository(t)
	now := time.Date(2026, 5, 19, 10, 0, 0, 0, time.UTC)
	for _, record := range []credentialdomain.Record{
		{ID: "cred-a", UserID: "user-a", Kind: credentialdomain.KindAPIKey, Label: "A", Status: credentialdomain.StatusActive, Fields: []credentialdomain.FieldRef{{Name: "value", Kind: credentialdomain.FieldSecret}}, CreatedAt: now, UpdatedAt: now},
		{ID: "cred-b", UserID: "user-b", Kind: credentialdomain.KindAPIKey, Label: "B", Status: credentialdomain.StatusActive, Fields: []credentialdomain.FieldRef{{Name: "value", Kind: credentialdomain.FieldSecret}}, CreatedAt: now, UpdatedAt: now},
	} {
		if _, err := repository.Save(ctx, record); err != nil {
			t.Fatalf("Save(%s): %v", record.ID, err)
		}
	}

	listed, err := repository.List(ctx, credentialdomain.Filter{Scope: credentialdomain.Scope{UserID: "user-a"}})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(listed) != 1 || listed[0].ID != "cred-a" {
		t.Fatalf("listed=%+v want only user-a credential", listed)
	}
	if _, err := repository.List(ctx, credentialdomain.Filter{}); err == nil {
		t.Fatal("expected list without owner scope to fail")
	}
}

func newTestRepository(t *testing.T) credentialdomain.Repository {
	t.Helper()
	return newTestRepositoryWithDataDir(t, t.TempDir())
}

func newTestRepositoryWithDataDir(t *testing.T, dataDir string) credentialdomain.Repository {
	t.Helper()
	handle, err := implstore.GetDBHandle(context.Background(), config.Config{DataDir: dataDir})
	if err != nil {
		t.Fatalf("GetDBHandle: %v", err)
	}
	repo, err := NewRepository(handle, dataDir)
	if err != nil {
		t.Fatalf("NewRepository: %v", err)
	}
	return repo
}

func saveTestCredential(t *testing.T, ctx context.Context, repo credentialdomain.Repository, id credentialdomain.ID, kind credentialdomain.Kind, now time.Time) {
	t.Helper()
	_, err := repo.Save(ctx, credentialdomain.Record{
		ID:        id,
		UserID:    "user-a",
		Kind:      kind,
		Label:     string(id),
		Status:    credentialdomain.StatusActive,
		Fields:    []credentialdomain.FieldRef{{Name: "value", Kind: credentialdomain.FieldSecret}},
		CreatedAt: now,
		UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("Save credential %s: %v", id, err)
	}
}
