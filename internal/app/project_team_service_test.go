package app

import (
	"context"
	"testing"
	"time"

	implstore "github.com/tinoosan/agen8/internal/store"
	"github.com/tinoosan/agen8/pkg/config"
	"github.com/tinoosan/agen8/pkg/services/session"
	"github.com/tinoosan/agen8/pkg/services/team"
	"github.com/tinoosan/agen8/pkg/types"
)

func TestProjectTeamService_RequiresMigrationForLegacyProject(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}
	sessionStore, err := implstore.NewSQLiteSessionStore(cfg)
	if err != nil {
		t.Fatalf("NewSQLiteSessionStore: %v", err)
	}
	sessionSvc := session.NewManager(cfg, sessionStore, nil)
	svc := NewProjectTeamService(cfg, sessionSvc, team.NewFileManifestStore(cfg))

	now := time.Now().UTC()
	sess := types.NewSession("legacy")
	sess.ProjectRoot = "/tmp/legacy-project"
	sess.TeamID = "team-legacy"
	sess.Profile = "startup"
	sess.CurrentRunID = "run-legacy"
	sess.CreatedAt = &now
	sess.UpdatedAt = &now
	if err := sessionSvc.SaveSession(context.Background(), sess); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}
	if err := team.NewFileManifestStore(cfg).Save(context.Background(), team.BuildManifest(
		"team-legacy",
		"startup",
		"lead",
		"run-legacy",
		"openai/gpt-5-mini",
		[]team.RoleRecord{{RoleName: "lead", RunID: "run-legacy", SessionID: sess.SessionID}},
		now.Format(time.RFC3339Nano),
	)); err != nil {
		t.Fatalf("Save manifest: %v", err)
	}

	if _, err := svc.ListTeams(context.Background(), "/tmp/legacy-project"); err == nil {
		t.Fatalf("expected migration-required error")
	}
}

func TestProjectTeamService_MigrateProject(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}
	sessionStore, err := implstore.NewSQLiteSessionStore(cfg)
	if err != nil {
		t.Fatalf("NewSQLiteSessionStore: %v", err)
	}
	sessionSvc := session.NewManager(cfg, sessionStore, nil)
	manifestStore := team.NewFileManifestStore(cfg)
	svc := NewProjectTeamService(cfg, sessionSvc, manifestStore)

	now := time.Now().UTC()
	sess := types.NewSession("legacy")
	sess.ProjectRoot = "/tmp/legacy-project"
	sess.TeamID = "team-legacy"
	sess.Profile = "startup"
	sess.CurrentRunID = "run-legacy"
	sess.CreatedAt = &now
	sess.UpdatedAt = &now
	if err := sessionSvc.SaveSession(context.Background(), sess); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}
	if err := manifestStore.Save(context.Background(), team.BuildManifest(
		"team-legacy",
		"startup",
		"lead",
		"run-legacy",
		"openai/gpt-5-mini",
		[]team.RoleRecord{{RoleName: "lead", RunID: "run-legacy", SessionID: sess.SessionID}},
		now.Format(time.RFC3339Nano),
	)); err != nil {
		t.Fatalf("Save manifest: %v", err)
	}

	migrated, err := svc.MigrateProject(context.Background(), "/tmp/legacy-project", "legacy-project")
	if err != nil {
		t.Fatalf("MigrateProject: %v", err)
	}
	if len(migrated) != 1 {
		t.Fatalf("len(migrated)=%d want 1", len(migrated))
	}

	listed, err := svc.ListTeams(context.Background(), "/tmp/legacy-project")
	if err != nil {
		t.Fatalf("ListTeams after migrate: %v", err)
	}
	if len(listed) != 1 || listed[0].TeamID != "team-legacy" {
		t.Fatalf("listed=%+v", listed)
	}
}
