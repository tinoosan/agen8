package store

import (
	"context"
	"database/sql"
	"testing"

	"github.com/tinoosan/agen8/pkg/config"
	"github.com/tinoosan/agen8/pkg/fsutil"
	_ "modernc.org/sqlite"
)

func TestProjectTeamStore_CRUD(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}
	ctx := context.Background()

	record, err := UpsertProjectTeam(ctx, cfg, ProjectTeamRecord{
		ProjectRoot:      "/tmp/project-a",
		ProjectID:        "project-a",
		TeamID:           "team-alpha",
		ProfileID:        "startup",
		PrimarySessionID: "sess-1",
		CoordinatorRunID: "run-1",
		Status:           ProjectTeamStatusActive,
	})
	if err != nil {
		t.Fatalf("UpsertProjectTeam: %v", err)
	}
	if got := record.TeamID; got != "team-alpha" {
		t.Fatalf("teamID=%q want team-alpha", got)
	}

	loaded, err := LoadProjectTeam(ctx, cfg, "/tmp/project-a", "team-alpha")
	if err != nil {
		t.Fatalf("LoadProjectTeam: %v", err)
	}
	if loaded.PrimarySessionID != "sess-1" {
		t.Fatalf("primarySessionID=%q want sess-1", loaded.PrimarySessionID)
	}

	listed, err := ListProjectTeams(ctx, cfg, "/tmp/project-a")
	if err != nil {
		t.Fatalf("ListProjectTeams: %v", err)
	}
	if len(listed) != 1 {
		t.Fatalf("len(listed)=%d want 1", len(listed))
	}

	if err := DeleteProjectTeam(ctx, cfg, "/tmp/project-a", "team-alpha"); err != nil {
		t.Fatalf("DeleteProjectTeam: %v", err)
	}
	if _, err := LoadProjectTeam(ctx, cfg, "/tmp/project-a", "team-alpha"); err == nil {
		t.Fatalf("expected deleted project team lookup to fail")
	}
}

func TestDeleteTeamScopedData_RemovesTasksMessagesAndArtifacts(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}
	ctx := context.Background()
	db, err := sql.Open("sqlite", fsutil.GetSQLitePath(cfg.DataDir))
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer db.Close()
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS tasks (task_id TEXT PRIMARY KEY, team_id TEXT DEFAULT '')`,
		`CREATE TABLE IF NOT EXISTS messages (message_id TEXT PRIMARY KEY, team_id TEXT DEFAULT '')`,
		`CREATE TABLE IF NOT EXISTS artifacts (artifact_id INTEGER PRIMARY KEY AUTOINCREMENT, team_id TEXT DEFAULT '')`,
		`INSERT INTO tasks(task_id, team_id) VALUES ('task-1', 'team-z')`,
		`INSERT INTO messages(message_id, team_id) VALUES ('msg-1', 'team-z')`,
		`INSERT INTO artifacts(team_id) VALUES ('team-z')`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("exec %q: %v", stmt, err)
		}
	}
	if err := DeleteTeamScopedData(ctx, cfg, "team-z"); err != nil {
		t.Fatalf("DeleteTeamScopedData: %v", err)
	}
	for _, check := range []struct {
		table string
		col   string
	}{
		{table: "tasks", col: "task_id"},
		{table: "messages", col: "message_id"},
		{table: "artifacts", col: "artifact_id"},
	} {
		var count int
		if err := db.QueryRow(`SELECT COUNT(*) FROM `+check.table+` WHERE team_id = ?`, "team-z").Scan(&count); err != nil {
			t.Fatalf("count %s: %v", check.table, err)
		}
		if count != 0 {
			t.Fatalf("%s rows remaining=%d want 0", check.table, count)
		}
	}
}

func TestProjectRegistryStore_CRUD(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}
	ctx := context.Background()

	record, err := UpsertProjectRegistry(ctx, cfg, ProjectRegistryRecord{
		ProjectRoot:  "/tmp/project-a",
		ProjectID:    "project-a",
		ManifestPath: "/tmp/project-a/.agen8/agen8.yaml",
		Enabled:      true,
		Metadata: map[string]any{
			"source": "test",
		},
	})
	if err != nil {
		t.Fatalf("UpsertProjectRegistry: %v", err)
	}
	if got := record.ProjectRoot; got != "/tmp/project-a" {
		t.Fatalf("projectRoot=%q want /tmp/project-a", got)
	}

	loaded, err := LoadProjectRegistry(ctx, cfg, "/tmp/project-a")
	if err != nil {
		t.Fatalf("LoadProjectRegistry: %v", err)
	}
	if got := loaded.ManifestPath; got != "/tmp/project-a/.agen8/agen8.yaml" {
		t.Fatalf("manifestPath=%q want /tmp/project-a/.agen8/agen8.yaml", got)
	}

	listed, err := ListProjectRegistry(ctx, cfg)
	if err != nil {
		t.Fatalf("ListProjectRegistry: %v", err)
	}
	if len(listed) != 1 {
		t.Fatalf("len(listed)=%d want 1", len(listed))
	}
}
