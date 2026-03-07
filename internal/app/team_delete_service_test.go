package app

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	implstore "github.com/tinoosan/agen8/internal/store"
	"github.com/tinoosan/agen8/pkg/config"
	"github.com/tinoosan/agen8/pkg/fsutil"
	"github.com/tinoosan/agen8/pkg/services/session"
	"github.com/tinoosan/agen8/pkg/services/team"
	"github.com/tinoosan/agen8/pkg/types"
	_ "modernc.org/sqlite"
)

func TestTeamDeleteService_DeleteTeam_RemovesRelatedData(t *testing.T) {
	ctx := context.Background()
	cfg := config.Config{DataDir: t.TempDir()}
	projectRoot := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(projectRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll(projectRoot): %v", err)
	}
	if _, err := InitProject(projectRoot, ProjectConfig{ProjectID: "p1"}); err != nil {
		t.Fatalf("InitProject: %v", err)
	}

	sessionStore, err := implstore.NewSQLiteSessionStore(cfg)
	if err != nil {
		t.Fatalf("NewSQLiteSessionStore: %v", err)
	}
	sessionSvc := session.NewManager(cfg, sessionStore, nil)
	manifestStore := team.NewFileManifestStore(cfg)
	projectTeamSvc := NewProjectTeamService(cfg, sessionSvc, manifestStore)

	sess := types.NewSession("goal")
	sess.ProjectRoot = projectRoot
	sess.TeamID = "team-delete"
	sess.Profile = "startup"
	run := types.NewRun("goal", 8*1024, sess.SessionID)
	sess.CurrentRunID = run.RunID
	sess.Runs = []string{run.RunID}
	if err := sessionSvc.SaveSession(ctx, sess); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}
	if err := sessionSvc.SaveRun(ctx, run); err != nil {
		t.Fatalf("SaveRun: %v", err)
	}
	if err := manifestStore.Save(ctx, team.BuildManifest(
		sess.TeamID,
		"startup",
		"lead",
		run.RunID,
		"openai/gpt-5-mini",
		[]team.RoleRecord{{RoleName: "lead", RunID: run.RunID, SessionID: sess.SessionID}},
		nil,
		"",
	)); err != nil {
		t.Fatalf("Save manifest: %v", err)
	}
	if _, err := projectTeamSvc.RegisterTeam(ctx, ProjectTeamSummary{
		ProjectRoot:      projectRoot,
		ProjectID:        "p1",
		TeamID:           sess.TeamID,
		ProfileID:        "startup",
		PrimarySessionID: sess.SessionID,
		CoordinatorRunID: run.RunID,
		Status:           "active",
	}); err != nil {
		t.Fatalf("RegisterTeam: %v", err)
	}
	if _, err := SetActiveSession(projectRoot, ProjectState{
		ActiveSessionID: sess.SessionID,
		ActiveTeamID:    sess.TeamID,
		ActiveRunID:     run.RunID,
		LastCommand:     "team.start",
	}); err != nil {
		t.Fatalf("SetActiveSession: %v", err)
	}

	db, err := sql.Open("sqlite", fsutil.GetSQLitePath(cfg.DataDir))
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer db.Close()
	for _, stmt := range []string{
		`CREATE TABLE IF NOT EXISTS tasks (task_id TEXT PRIMARY KEY, team_id TEXT DEFAULT '')`,
		`CREATE TABLE IF NOT EXISTS messages (message_id TEXT PRIMARY KEY, team_id TEXT DEFAULT '')`,
		`CREATE TABLE IF NOT EXISTS artifacts (artifact_id INTEGER PRIMARY KEY AUTOINCREMENT, team_id TEXT DEFAULT '')`,
		`INSERT INTO tasks(task_id, team_id) VALUES ('task-1', 'team-delete')`,
		`INSERT INTO messages(message_id, team_id) VALUES ('msg-1', 'team-delete')`,
		`INSERT INTO artifacts(team_id) VALUES ('team-delete')`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("exec %q: %v", stmt, err)
		}
	}

	svc := NewTeamDeleteService(cfg, sessionSvc, manifestStore, projectTeamSvc)
	out, err := svc.DeleteTeam(ctx, TeamDeleteInput{
		TeamID:      sess.TeamID,
		ProjectRoot: projectRoot,
	})
	if err != nil {
		t.Fatalf("DeleteTeam: %v", err)
	}
	if len(out.DeletedSessionIDs) != 1 || out.DeletedSessionIDs[0] != sess.SessionID {
		t.Fatalf("DeletedSessionIDs=%v", out.DeletedSessionIDs)
	}
	if _, err := sessionSvc.LoadSession(ctx, sess.SessionID); err == nil {
		t.Fatalf("expected session to be deleted")
	}
	if _, err := projectTeamSvc.GetTeam(ctx, projectRoot, sess.TeamID); err == nil {
		t.Fatalf("expected project team row to be deleted")
	}
	if _, err := os.Stat(fsutil.GetTeamDir(cfg.DataDir, sess.TeamID)); !os.IsNotExist(err) {
		t.Fatalf("expected team dir removed, stat err=%v", err)
	}
	for _, table := range []string{"tasks", "messages", "artifacts"} {
		var count int
		if err := db.QueryRow(`SELECT COUNT(*) FROM `+table+` WHERE team_id = ?`, sess.TeamID).Scan(&count); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		if count != 0 {
			t.Fatalf("%s rows remaining=%d want 0", table, count)
		}
	}
	projectCtx, err := LoadProjectContext(projectRoot)
	if err != nil {
		t.Fatalf("LoadProjectContext: %v", err)
	}
	if projectCtx.State.ActiveTeamID != "" || projectCtx.State.ActiveSessionID != "" || projectCtx.State.ActiveRunID != "" {
		t.Fatalf("project state not cleared: %+v", projectCtx.State)
	}
}
