package team

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/tinoosan/workbench-core/pkg/config"
	"github.com/tinoosan/workbench-core/pkg/fsutil"
)

func TestFileManifestStore_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Config{DataDir: dir}
	store := NewFileManifestStore(cfg)

	ctx := context.Background()
	teamID := "team-test-1"
	m := Manifest{
		TeamID:          teamID,
		ProfileID:       "profile-1",
		TeamModel:       "gpt-5",
		CoordinatorRole: "ceo",
		CoordinatorRun:  "run-1",
		Roles:           []RoleRecord{{RoleName: "ceo", RunID: "run-1", SessionID: "sess-1"}},
		CreatedAt:       "2025-01-01T00:00:00Z",
	}

	err := store.Save(ctx, m)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	path := filepath.Join(fsutil.GetTeamDir(cfg.DataDir, teamID), manifestFilename)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatalf("manifest file was not created at %s", path)
	}

	loaded, err := store.Load(ctx, teamID)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded == nil {
		t.Fatal("Load returned nil")
	}
	if loaded.TeamID != m.TeamID || loaded.ProfileID != m.ProfileID || loaded.TeamModel != m.TeamModel {
		t.Fatalf("loaded = %+v", loaded)
	}
	if len(loaded.Roles) != 1 || loaded.Roles[0].RoleName != "ceo" {
		t.Fatalf("loaded.Roles = %+v", loaded.Roles)
	}
}

func TestFileManifestStore_LoadMissing(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Config{DataDir: dir}
	store := NewFileManifestStore(cfg)

	loaded, err := store.Load(context.Background(), "nonexistent-team")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded != nil {
		t.Fatalf("expected nil manifest for missing team, got %+v", loaded)
	}
}

func TestFileManifestStore_LoadEmptyTeamID(t *testing.T) {
	store := NewFileManifestStore(config.Config{DataDir: t.TempDir()})
	_, err := store.Load(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty teamID")
	}
}

func TestFileManifestStore_SaveEmptyTeamID(t *testing.T) {
	store := NewFileManifestStore(config.Config{DataDir: t.TempDir()})
	err := store.Save(context.Background(), Manifest{})
	if err == nil {
		t.Fatal("expected error for empty teamID in manifest")
	}
}
