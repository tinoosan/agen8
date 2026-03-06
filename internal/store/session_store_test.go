package store

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tinoosan/agen8/pkg/config"
	"github.com/tinoosan/agen8/pkg/fsutil"
	"github.com/tinoosan/agen8/pkg/types"
)

func TestCreateSessionAndAddRun(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}

	s, run, err := CreateSession(cfg, "test session", 128)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if run.SessionID != s.SessionID {
		t.Fatalf("run.SessionID=%q want session %q", run.SessionID, s.SessionID)
	}
	if s.SessionID == "" {
		t.Fatalf("expected sessionId")
	}

	// Add a run and ensure it becomes current.
	updated, err := AddRunToSession(cfg, s.SessionID, "run-1")
	if err != nil {
		t.Fatalf("AddRunToSession: %v", err)
	}
	if updated.CurrentRunID != "run-1" {
		t.Fatalf("currentRunId=%q", updated.CurrentRunID)
	}
	if len(updated.Runs) != 2 || updated.Runs[0] != run.RunID || updated.Runs[1] != "run-1" {
		t.Fatalf("runs=%v", updated.Runs)
	}

	loaded, err := LoadSession(cfg, s.SessionID)
	if err != nil {
		t.Fatalf("LoadSession: %v", err)
	}
	if loaded.CurrentRunID != "run-1" {
		t.Fatalf("loaded currentRunId=%q", loaded.CurrentRunID)
	}
}

func TestRecordTurnInSession_UpdatesGoalAndSummary(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}

	s, _, err := CreateSession(cfg, "t", 64)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	updated, err := RecordTurnInSession(cfg, s.SessionID, "run-1", "do the thing", "done")
	if err != nil {
		t.Fatalf("RecordTurnInSession: %v", err)
	}
	if updated.CurrentGoal == "" {
		t.Fatalf("expected currentGoal to be set")
	}
	if updated.Summary == "" {
		t.Fatalf("expected summary to be set")
	}
	if updated.UpdatedAt == nil {
		t.Fatalf("expected updatedAt to be set")
	}
}

func TestLoadSession_NotFound_IsErrNotFound(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}

	_, err := LoadSession(cfg, "does-not-exist")
	if err == nil {
		t.Fatalf("expected error")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected errors.Is(err, ErrNotFound) to be true, err=%v", err)
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected errors.Is(err, os.ErrNotExist) to be true, err=%v", err)
	}
}

func TestListSessionsPaginated_Basic(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}

	for i := 0; i < 5; i++ {
		if _, _, err := CreateSession(cfg, fmt.Sprintf("session %d", i), 64); err != nil {
			t.Fatalf("CreateSession %d: %v", i, err)
		}
	}

	filter := SessionFilter{Limit: 2, Offset: 0, SortDesc: true}
	sessions, err := ListSessionsPaginated(cfg, filter)
	if err != nil {
		t.Fatalf("ListSessionsPaginated: %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(sessions))
	}

	filter.Offset = 2
	sessions2, err := ListSessionsPaginated(cfg, filter)
	if err != nil {
		t.Fatalf("ListSessionsPaginated page 2: %v", err)
	}
	if len(sessions2) != 2 {
		t.Fatalf("expected 2 sessions on page 2, got %d", len(sessions2))
	}

	seen := map[string]struct{}{}
	for _, s := range sessions {
		seen[s.SessionID] = struct{}{}
	}
	for _, s := range sessions2 {
		if _, ok := seen[s.SessionID]; ok {
			t.Fatalf("session %s appears on both pages", s.SessionID)
		}
	}
}

func TestListSessionsPaginated_Sorting(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}

	base := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	t1 := base.Add(1 * time.Minute)
	t2 := base.Add(2 * time.Minute)
	t3 := base.Add(3 * time.Minute)

	s1 := types.NewSession("first")
	s1.CreatedAt = &t1
	if err := SaveSession(cfg, s1); err != nil {
		t.Fatalf("SaveSession(s1): %v", err)
	}
	s2 := types.NewSession("second")
	s2.CreatedAt = &t2
	if err := SaveSession(cfg, s2); err != nil {
		t.Fatalf("SaveSession(s2): %v", err)
	}
	s3 := types.NewSession("third")
	s3.CreatedAt = &t3
	if err := SaveSession(cfg, s3); err != nil {
		t.Fatalf("SaveSession(s3): %v", err)
	}

	filter := SessionFilter{Limit: 10, SortBy: "created_at", SortDesc: true}
	sessions, err := ListSessionsPaginated(cfg, filter)
	if err != nil {
		t.Fatalf("ListSessionsPaginated: %v", err)
	}
	if len(sessions) != 3 {
		t.Fatalf("expected 3 sessions, got %d", len(sessions))
	}
	if sessions[0].SessionID != s3.SessionID {
		t.Fatalf("expected newest session first, got %s", sessions[0].SessionID)
	}
	if sessions[2].SessionID != s1.SessionID {
		t.Fatalf("expected oldest session last, got %s", sessions[2].SessionID)
	}

	filter.SortDesc = false
	sessions, err = ListSessionsPaginated(cfg, filter)
	if err != nil {
		t.Fatalf("ListSessionsPaginated asc: %v", err)
	}
	if sessions[0].SessionID != s1.SessionID {
		t.Fatalf("expected oldest session first, got %s", sessions[0].SessionID)
	}
	if sessions[2].SessionID != s3.SessionID {
		t.Fatalf("expected newest session last, got %s", sessions[2].SessionID)
	}

}

func TestListSessionsPaginated_TitleFilter(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}

	if _, _, err := CreateSession(cfg, "alpha project", 64); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if _, _, err := CreateSession(cfg, "beta project", 64); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if _, _, err := CreateSession(cfg, "gamma task", 64); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	filter := SessionFilter{TitleContains: "project", Limit: 10}
	sessions, err := ListSessionsPaginated(cfg, filter)
	if err != nil {
		t.Fatalf("ListSessionsPaginated: %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions matching 'project', got %d", len(sessions))
	}

	filter.TitleContains = "gamma"
	sessions, err = ListSessionsPaginated(cfg, filter)
	if err != nil {
		t.Fatalf("ListSessionsPaginated: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session matching 'gamma', got %d", len(sessions))
	}
}

func TestListSessionsPaginated_ProjectRootFilter(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}

	s1, _, err := CreateSession(cfg, "proj a goal", 64)
	if err != nil {
		t.Fatalf("CreateSession 1: %v", err)
	}
	s1.ProjectRoot = "/proj/a"
	if err := SaveSession(cfg, s1); err != nil {
		t.Fatalf("SaveSession 1: %v", err)
	}

	s2, _, err := CreateSession(cfg, "proj b goal", 64)
	if err != nil {
		t.Fatalf("CreateSession 2: %v", err)
	}
	s2.ProjectRoot = "/proj/b"
	if err := SaveSession(cfg, s2); err != nil {
		t.Fatalf("SaveSession 2: %v", err)
	}

	_, _, err = CreateSession(cfg, "no proj goal", 64)
	if err != nil {
		t.Fatalf("CreateSession 3: %v", err)
	}
	// session 3 has no ProjectRoot

	filter := SessionFilter{ProjectRoot: "/proj/a", Limit: 10}
	sessions, err := ListSessionsPaginated(cfg, filter)
	if err != nil {
		t.Fatalf("ListSessionsPaginated: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session for /proj/a, got %d", len(sessions))
	}
	if strings.TrimSpace(sessions[0].SessionID) != strings.TrimSpace(s1.SessionID) {
		t.Fatalf("expected session %s, got %s", s1.SessionID, sessions[0].SessionID)
	}

	filter.ProjectRoot = "/proj/b"
	sessions, err = ListSessionsPaginated(cfg, filter)
	if err != nil {
		t.Fatalf("ListSessionsPaginated: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session for /proj/b, got %d", len(sessions))
	}
	if strings.TrimSpace(sessions[0].SessionID) != strings.TrimSpace(s2.SessionID) {
		t.Fatalf("expected session %s, got %s", s2.SessionID, sessions[0].SessionID)
	}

	filter.ProjectRoot = "/nonexistent"
	sessions, err = ListSessionsPaginated(cfg, filter)
	if err != nil {
		t.Fatalf("ListSessionsPaginated: %v", err)
	}
	if len(sessions) != 0 {
		t.Fatalf("expected 0 sessions for /nonexistent, got %d", len(sessions))
	}

	count, err := CountSessions(cfg, SessionFilter{ProjectRoot: "/proj/a"})
	if err != nil {
		t.Fatalf("CountSessions: %v", err)
	}
	if count != 1 {
		t.Fatalf("CountSessions project /proj/a: got %d want 1", count)
	}
}

func TestCountSessions(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}

	count, err := CountSessions(cfg, SessionFilter{})
	if err != nil {
		t.Fatalf("CountSessions: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected 0 sessions, got %d", count)
	}

	CreateSession(cfg, "one", 64)
	CreateSession(cfg, "two", 64)
	CreateSession(cfg, "three", 64)

	count, err = CountSessions(cfg, SessionFilter{})
	if err != nil {
		t.Fatalf("CountSessions: %v", err)
	}
	if count != 3 {
		t.Fatalf("expected 3 sessions, got %d", count)
	}

	count, err = CountSessions(cfg, SessionFilter{TitleContains: "two"})
	if err != nil {
		t.Fatalf("CountSessions: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 session matching 'two', got %d", count)
	}
}

func TestListSessionsPaginated_InvalidSortColumn(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}

	filter := SessionFilter{SortBy: "invalid_column; DROP TABLE sessions;--"}
	_, err := ListSessionsPaginated(cfg, filter)
	if err == nil {
		t.Fatalf("expected error for invalid sort column")
	}
}

func TestListSessionsPaginated_EmptyResult(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}

	sessions, err := ListSessionsPaginated(cfg, SessionFilter{Limit: 10})
	if err != nil {
		t.Fatalf("ListSessionsPaginated: %v", err)
	}
	if len(sessions) != 0 {
		t.Fatalf("expected 0 sessions, got %d", len(sessions))
	}
}

func TestListSessionsPaginated_OffsetBeyondTotal(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}

	CreateSession(cfg, "only one", 64)

	filter := SessionFilter{Limit: 10, Offset: 100}
	sessions, err := ListSessionsPaginated(cfg, filter)
	if err != nil {
		t.Fatalf("ListSessionsPaginated: %v", err)
	}
	if len(sessions) != 0 {
		t.Fatalf("expected 0 sessions with offset beyond total, got %d", len(sessions))
	}
}

func TestAllowedSessionSortColumn(t *testing.T) {
	tests := []struct {
		input   string
		wantCol string
		wantOK  bool
	}{
		{"", "updated_at", true},
		{"updated_at", "updated_at", true},
		{"UPDATED_AT", "updated_at", true},
		{"created_at", "created_at", true},
		{"title", "title", true},
		{"invalid", "", false},
		{"session_id", "", false},
		{"session_json", "", false},
	}

	for _, tc := range tests {
		col, ok := allowedSessionSortColumn(tc.input)
		if ok != tc.wantOK || col != tc.wantCol {
			t.Errorf("allowedSessionSortColumn(%q) = (%q, %v), want (%q, %v)",
				tc.input, col, ok, tc.wantCol, tc.wantOK)
		}
	}
}

func TestListSessionsPaginated_FallbackSessionIDFromRow(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}
	sess, _, err := CreateSession(cfg, "fallback id", 64)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	db, err := getSQLiteDB(cfg)
	if err != nil {
		t.Fatalf("getSQLiteDB: %v", err)
	}
	if _, err := db.Exec(`UPDATE sessions SET session_json = ? WHERE session_id = ?`, `{"title":"fallback id"}`, sess.SessionID); err != nil {
		t.Fatalf("UPDATE sessions: %v", err)
	}
	rows, err := ListSessionsPaginated(cfg, SessionFilter{Limit: 10})
	if err != nil {
		t.Fatalf("ListSessionsPaginated: %v", err)
	}
	if len(rows) == 0 {
		t.Fatalf("expected at least one row")
	}
	if rows[0].SessionID == "" {
		t.Fatalf("expected session id fallback from row key")
	}
}

func TestClearHistoryForSession_ClearsPersistedRunArtifacts(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}
	sess, runA, err := CreateSession(cfg, "clear-history", 64)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	runB := types.NewRun("child", 64, sess.SessionID)
	if err := SaveRun(cfg, runB); err != nil {
		t.Fatalf("SaveRun runB: %v", err)
	}
	if _, err := AddRunToSession(cfg, sess.SessionID, runB.RunID); err != nil {
		t.Fatalf("AddRunToSession runB: %v", err)
	}
	db, err := getSQLiteDB(cfg)
	if err != nil {
		t.Fatalf("getSQLiteDB: %v", err)
	}
	for _, runID := range []string{runA.RunID, runB.RunID} {
		if _, err := db.Exec(`INSERT INTO events (event_id, run_id, ts, type, message, data_json, event_json) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			"ev-"+runID, runID, time.Now().UTC().Format(time.RFC3339Nano), "agent.op.request", "msg", "{}", `{"id":"x"}`); err != nil {
			t.Fatalf("insert events: %v", err)
		}
		if _, err := db.Exec(`INSERT INTO history (id, session_id, run_id, ts, origin, kind, message, model, data_json, line_json) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			"hist-"+runID, sess.SessionID, runID, time.Now().UTC().Format(time.RFC3339Nano), "agent", "assistant", "hello", "", "{}", `{"id":"x","runId":"`+runID+`"}`); err != nil {
			t.Fatalf("insert history: %v", err)
		}
		if _, err := db.Exec(`INSERT INTO activities (run_id, activity_id, seq, kind, title, status, started_at, finished_at, meta_json) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			runID, "act-"+runID, 1, "tool", "activity", "completed", time.Now().UTC().Format(time.RFC3339Nano), time.Now().UTC().Format(time.RFC3339Nano), "{}"); err != nil {
			t.Fatalf("insert activities: %v", err)
		}
		if _, err := db.Exec(`INSERT INTO constructor_state (run_id, updated_at, state_json) VALUES (?, ?, ?)`,
			runID, time.Now().UTC().Format(time.RFC3339Nano), `{"ok":true}`); err != nil {
			t.Fatalf("insert constructor_state: %v", err)
		}
		if _, err := db.Exec(`INSERT INTO constructor_manifest (run_id, updated_at, manifest_json) VALUES (?, ?, ?)`,
			runID, time.Now().UTC().Format(time.RFC3339Nano), `{"ok":true}`); err != nil {
			t.Fatalf("insert constructor_manifest: %v", err)
		}
	}

	out, err := ClearHistoryForSession(cfg, sess.SessionID)
	if err != nil {
		t.Fatalf("ClearHistoryForSession: %v", err)
	}
	if len(out.SourceRuns) != 2 {
		t.Fatalf("expected 2 source runs, got %d", len(out.SourceRuns))
	}
	if out.EventsDeleted == 0 || out.HistoryDeleted == 0 || out.ActivitiesDeleted == 0 {
		t.Fatalf("expected non-zero deletes, got %+v", out)
	}

	assertCount := func(query string, args ...any) {
		t.Helper()
		var n int
		if err := db.QueryRow(query, args...).Scan(&n); err != nil {
			t.Fatalf("count query failed: %v", err)
		}
		if n != 0 {
			t.Fatalf("expected 0 rows for query %q, got %d", query, n)
		}
	}
	assertCount(`SELECT COUNT(*) FROM events WHERE run_id IN (?, ?)`, runA.RunID, runB.RunID)
	assertCount(`SELECT COUNT(*) FROM history WHERE run_id IN (?, ?)`, runA.RunID, runB.RunID)
	assertCount(`SELECT COUNT(*) FROM activities WHERE run_id IN (?, ?)`, runA.RunID, runB.RunID)
	assertCount(`SELECT COUNT(*) FROM constructor_state WHERE run_id IN (?, ?)`, runA.RunID, runB.RunID)
	assertCount(`SELECT COUNT(*) FROM constructor_manifest WHERE run_id IN (?, ?)`, runA.RunID, runB.RunID)
}

func TestClearHistoryForRunIDs_OnlyTargetRuns(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}
	sessA, runA, err := CreateSession(cfg, "A", 64)
	if err != nil {
		t.Fatalf("CreateSession A: %v", err)
	}
	_, runB, err := CreateSession(cfg, "B", 64)
	if err != nil {
		t.Fatalf("CreateSession B: %v", err)
	}
	db, err := getSQLiteDB(cfg)
	if err != nil {
		t.Fatalf("getSQLiteDB: %v", err)
	}
	for _, runID := range []string{runA.RunID, runB.RunID} {
		sessionID := sessA.SessionID
		if runID == runB.RunID {
			sessionID = runB.SessionID
		}
		if _, err := db.Exec(`INSERT INTO events (event_id, run_id, ts, type, message, data_json, event_json) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			"ev-"+runID, runID, time.Now().UTC().Format(time.RFC3339Nano), "agent.op.request", "msg", "{}", `{"id":"x"}`); err != nil {
			t.Fatalf("insert events: %v", err)
		}
		if _, err := db.Exec(`INSERT INTO history (id, session_id, run_id, ts, origin, kind, message, model, data_json, line_json) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			"hist-"+runID, sessionID, runID, time.Now().UTC().Format(time.RFC3339Nano), "agent", "assistant", "hello", "", "{}", `{"id":"x","runId":"`+runID+`"}`); err != nil {
			t.Fatalf("insert history: %v", err)
		}
	}

	out, err := ClearHistoryForRunIDs(cfg, []string{runA.RunID, runA.RunID})
	if err != nil {
		t.Fatalf("ClearHistoryForRunIDs: %v", err)
	}
	if len(out.SourceRuns) != 1 || out.SourceRuns[0] != runA.RunID {
		t.Fatalf("unexpected source runs: %+v", out.SourceRuns)
	}
	var remainA, remainB int
	if err := db.QueryRow(`SELECT COUNT(*) FROM events WHERE run_id = ?`, runA.RunID).Scan(&remainA); err != nil {
		t.Fatalf("count runA: %v", err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM events WHERE run_id = ?`, runB.RunID).Scan(&remainB); err != nil {
		t.Fatalf("count runB: %v", err)
	}
	if remainA != 0 {
		t.Fatalf("expected runA rows cleared, got %d", remainA)
	}
	if remainB == 0 {
		t.Fatalf("expected runB rows to remain")
	}
}

func TestDeleteSession_CascadesRunRows(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}
	sessA, runA, err := CreateSession(cfg, "A", 64)
	if err != nil {
		t.Fatalf("CreateSession A: %v", err)
	}
	runA2 := types.NewRun("A2", 64, sessA.SessionID)
	if err := SaveRun(cfg, runA2); err != nil {
		t.Fatalf("SaveRun A2: %v", err)
	}
	if _, err := AddRunToSession(cfg, sessA.SessionID, runA2.RunID); err != nil {
		t.Fatalf("AddRunToSession A2: %v", err)
	}
	_, runB, err := CreateSession(cfg, "B", 64)
	if err != nil {
		t.Fatalf("CreateSession B: %v", err)
	}
	db, err := getSQLiteDB(cfg)
	if err != nil {
		t.Fatalf("getSQLiteDB: %v", err)
	}
	for _, runID := range []string{runA.RunID, runA2.RunID, runB.RunID} {
		if err := os.MkdirAll(fsutil.GetAgentDir(cfg.DataDir, runID), 0o755); err != nil {
			t.Fatalf("mkdir run dir %s: %v", runID, err)
		}
	}
	for _, runID := range []string{runA.RunID, runA2.RunID, runB.RunID} {
		sessionID := sessA.SessionID
		if runID == runB.RunID {
			sessionID = runB.SessionID
		}
		if _, err := db.Exec(`INSERT INTO events (event_id, run_id, ts, type, message, data_json, event_json) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			"ev-"+runID, runID, time.Now().UTC().Format(time.RFC3339Nano), "agent.op.request", "msg", "{}", `{"id":"x"}`); err != nil {
			t.Fatalf("insert events: %v", err)
		}
		if _, err := db.Exec(`INSERT INTO history (id, session_id, run_id, ts, origin, kind, message, model, data_json, line_json) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			"hist-"+runID, sessionID, runID, time.Now().UTC().Format(time.RFC3339Nano), "agent", "assistant", "hello", "", "{}", `{"id":"x","runId":"`+runID+`"}`); err != nil {
			t.Fatalf("insert history: %v", err)
		}
	}

	if err := DeleteSession(cfg, sessA.SessionID); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}

	var sessionRows int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sessions WHERE session_id = ?`, sessA.SessionID).Scan(&sessionRows); err != nil {
		t.Fatalf("count sessions: %v", err)
	}
	if sessionRows != 0 {
		t.Fatalf("expected deleted session row")
	}
	var runsA int
	if err := db.QueryRow(`SELECT COUNT(*) FROM runs WHERE session_id = ?`, sessA.SessionID).Scan(&runsA); err != nil {
		t.Fatalf("count runsA: %v", err)
	}
	if runsA != 0 {
		t.Fatalf("expected deleted session runs")
	}
	var runsB int
	if err := db.QueryRow(`SELECT COUNT(*) FROM runs WHERE session_id = ?`, runB.SessionID).Scan(&runsB); err != nil {
		t.Fatalf("count runsB: %v", err)
	}
	if runsB == 0 {
		t.Fatalf("expected non-target session runs to remain")
	}

	for _, runID := range []string{runA.RunID, runA2.RunID} {
		if _, err := os.Stat(fsutil.GetAgentDir(cfg.DataDir, runID)); !os.IsNotExist(err) {
			t.Fatalf("expected run dir deleted for %s, err=%v", runID, err)
		}
	}
	if _, err := os.Stat(fsutil.GetAgentDir(cfg.DataDir, runB.RunID)); err != nil {
		t.Fatalf("expected non-target run dir to remain, err=%v", err)
	}
}

func TestDeleteSession_PreservesTeamDirectory(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}
	sess, run, err := CreateSession(cfg, "team session", 64)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	sess.Mode = "team"
	sess.TeamID = "team-delete-test"
	if err := SaveSession(cfg, sess); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}

	teamDir := fsutil.GetTeamDir(cfg.DataDir, sess.TeamID)
	if err := os.MkdirAll(teamDir, 0o755); err != nil {
		t.Fatalf("mkdir team dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(teamDir, "team.json"), []byte(`{"teamId":"team-delete-test"}`), 0o644); err != nil {
		t.Fatalf("write team manifest: %v", err)
	}
	if err := os.MkdirAll(fsutil.GetAgentDir(cfg.DataDir, run.RunID), 0o755); err != nil {
		t.Fatalf("mkdir run dir: %v", err)
	}

	if err := DeleteSession(cfg, sess.SessionID); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}
	if _, err := os.Stat(teamDir); err != nil {
		t.Fatalf("expected team dir preserved, err=%v", err)
	}
}

func TestDeleteSession_MarksProjectTeamInactiveAndPreservesTeamScopedData(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}
	sess, run, err := CreateSession(cfg, "team session", 64)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	sess.Mode = "team"
	sess.ProjectRoot = "/tmp/project-a"
	sess.TeamID = "team-delete-test"
	sess.CurrentRunID = run.RunID
	sess.Runs = []string{run.RunID}
	if err := SaveSession(cfg, sess); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}
	run.SessionID = sess.SessionID
	run.RunID = strings.TrimSpace(run.RunID)
	if err := SaveRun(cfg, run); err != nil {
		t.Fatalf("SaveRun: %v", err)
	}
	if _, err := UpsertProjectTeam(context.Background(), cfg, ProjectTeamRecord{
		ProjectRoot:      sess.ProjectRoot,
		ProjectID:        "project-a",
		TeamID:           sess.TeamID,
		ProfileID:        "startup",
		PrimarySessionID: sess.SessionID,
		CoordinatorRunID: run.RunID,
		Status:           ProjectTeamStatusActive,
	}); err != nil {
		t.Fatalf("UpsertProjectTeam: %v", err)
	}

	teamDir := fsutil.GetTeamDir(cfg.DataDir, sess.TeamID)
	if err := os.MkdirAll(teamDir, 0o755); err != nil {
		t.Fatalf("mkdir team dir: %v", err)
	}
	db, err := getSQLiteDB(cfg)
	if err != nil {
		t.Fatalf("getSQLiteDB: %v", err)
	}
	for _, stmt := range []string{
		`CREATE TABLE IF NOT EXISTS tasks (task_id TEXT PRIMARY KEY, team_id TEXT DEFAULT '')`,
		`CREATE TABLE IF NOT EXISTS messages (message_id TEXT PRIMARY KEY, team_id TEXT DEFAULT '')`,
		`CREATE TABLE IF NOT EXISTS artifacts (artifact_id INTEGER PRIMARY KEY AUTOINCREMENT, team_id TEXT DEFAULT '')`,
		`INSERT INTO tasks(task_id, team_id) VALUES ('task-1', 'team-delete-test')`,
		`INSERT INTO messages(message_id, team_id) VALUES ('msg-1', 'team-delete-test')`,
		`INSERT INTO artifacts(team_id) VALUES ('team-delete-test')`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("exec %q: %v", stmt, err)
		}
	}

	if err := DeleteSession(cfg, sess.SessionID); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}

	projectTeam, err := LoadProjectTeam(context.Background(), cfg, sess.ProjectRoot, sess.TeamID)
	if err != nil {
		t.Fatalf("LoadProjectTeam: %v", err)
	}
	if got := strings.TrimSpace(projectTeam.Status); got != ProjectTeamStatusInactive {
		t.Fatalf("status=%q want %q", got, ProjectTeamStatusInactive)
	}
	if projectTeam.PrimarySessionID != "" {
		t.Fatalf("primarySessionID=%q want empty", projectTeam.PrimarySessionID)
	}
	if projectTeam.CoordinatorRunID != "" {
		t.Fatalf("coordinatorRunID=%q want empty", projectTeam.CoordinatorRunID)
	}
	if _, err := os.Stat(teamDir); err != nil {
		t.Fatalf("expected team dir preserved, err=%v", err)
	}
	for _, table := range []string{"tasks", "messages", "artifacts"} {
		var count int
		if err := db.QueryRow(`SELECT COUNT(*) FROM `+table+` WHERE team_id = ?`, sess.TeamID).Scan(&count); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		if count != 1 {
			t.Fatalf("%s rows remaining=%d want 1", table, count)
		}
	}
}
