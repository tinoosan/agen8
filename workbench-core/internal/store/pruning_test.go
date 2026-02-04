package store

import (
	"context"
	"testing"
	"time"

	"github.com/tinoosan/workbench-core/pkg/config"
)

func TestPruneOldSessions_RemovesLinkedData(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}

	oldSess, oldRun, err := CreateSession(cfg, "old", 64)
	if err != nil {
		t.Fatalf("CreateSession(old): %v", err)
	}
	newSess, newRun, err := CreateSession(cfg, "new", 64)
	if err != nil {
		t.Fatalf("CreateSession(new): %v", err)
	}

	db, err := getSQLiteDB(cfg)
	if err != nil {
		t.Fatalf("getSQLiteDB: %v", err)
	}

	// Force the old session's updated_at into the past so it prunes.
	oldUpdated := time.Now().Add(-31 * 24 * time.Hour).UTC().Format(time.RFC3339Nano)
	if _, err := db.Exec(`UPDATE sessions SET updated_at = ? WHERE session_id = ?`, oldUpdated, oldSess.SessionID); err != nil {
		t.Fatalf("set old session updated_at: %v", err)
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)

	// Seed child tables for both sessions so we can verify the cascade behavior.
	for _, tc := range []struct {
		sessionID string
		runID     string
	}{
		{sessionID: oldSess.SessionID, runID: oldRun.RunID},
		{sessionID: newSess.SessionID, runID: newRun.RunID},
	} {
		if _, err := db.Exec(
			`INSERT INTO events (event_id, run_id, ts, type, message, data_json, event_json) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			"ev-"+tc.runID,
			tc.runID,
			now,
			"test",
			"hello",
			nil,
			`{"type":"test","message":"hello"}`,
		); err != nil {
			t.Fatalf("insert event: %v", err)
		}
		if _, err := db.Exec(
			`INSERT INTO history (id, session_id, run_id, ts, origin, kind, message, model, data_json, line_json)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			"h-"+tc.runID,
			tc.sessionID,
			tc.runID,
			now,
			"test",
			"test",
			"hello",
			nil,
			nil,
			`{"id":"h","ts":"`+now+`","runId":"`+tc.runID+`","origin":"test","kind":"test","message":"hello"}`,
		); err != nil {
			t.Fatalf("insert history: %v", err)
		}
		if _, err := db.Exec(
			`INSERT INTO constructor_state (run_id, updated_at, state_json) VALUES (?, ?, ?)`,
			tc.runID,
			now,
			`{"ok":true}`,
		); err != nil {
			t.Fatalf("insert constructor_state: %v", err)
		}
		if _, err := db.Exec(
			`INSERT INTO constructor_manifest (run_id, updated_at, manifest_json) VALUES (?, ?, ?)`,
			tc.runID,
			now,
			`{"ok":true}`,
		); err != nil {
			t.Fatalf("insert constructor_manifest: %v", err)
		}
		if _, err := db.Exec(
			`INSERT INTO activities (run_id, activity_id, seq, kind, title, status, started_at, finished_at, meta_json)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			tc.runID,
			"act-"+tc.runID,
			1,
			"test",
			"title",
			"done",
			now,
			nil,
			nil,
		); err != nil {
			t.Fatalf("insert activities: %v", err)
		}
	}

	if _, err := PruneOldSessions(context.Background(), cfg, 30*24*time.Hour); err != nil {
		t.Fatalf("PruneOldSessions: %v", err)
	}

	assertCountEq := func(query string, want int, args ...any) {
		t.Helper()
		var got int
		if err := db.QueryRow(query, args...).Scan(&got); err != nil {
			t.Fatalf("count query: %v", err)
		}
		if got != want {
			t.Fatalf("count mismatch for %q: want %d got %d", query, want, got)
		}
	}

	// Old session and all its dependent rows should be gone.
	assertCountEq(`SELECT COUNT(*) FROM sessions WHERE session_id = ?`, 0, oldSess.SessionID)
	assertCountEq(`SELECT COUNT(*) FROM runs WHERE session_id = ?`, 0, oldSess.SessionID)
	assertCountEq(`SELECT COUNT(*) FROM events WHERE run_id = ?`, 0, oldRun.RunID)
	assertCountEq(`SELECT COUNT(*) FROM history WHERE session_id = ?`, 0, oldSess.SessionID)
	assertCountEq(`SELECT COUNT(*) FROM activities WHERE run_id = ?`, 0, oldRun.RunID)
	assertCountEq(`SELECT COUNT(*) FROM constructor_state WHERE run_id = ?`, 0, oldRun.RunID)
	assertCountEq(`SELECT COUNT(*) FROM constructor_manifest WHERE run_id = ?`, 0, oldRun.RunID)

	// New session and its dependent rows should remain.
	assertCountEq(`SELECT COUNT(*) FROM sessions WHERE session_id = ?`, 1, newSess.SessionID)
	assertCountEq(`SELECT COUNT(*) FROM runs WHERE session_id = ?`, 1, newSess.SessionID)
	assertCountEq(`SELECT COUNT(*) FROM events WHERE run_id = ?`, 1, newRun.RunID)
	assertCountEq(`SELECT COUNT(*) FROM history WHERE session_id = ?`, 1, newSess.SessionID)
	assertCountEq(`SELECT COUNT(*) FROM activities WHERE run_id = ?`, 1, newRun.RunID)
	assertCountEq(`SELECT COUNT(*) FROM constructor_state WHERE run_id = ?`, 1, newRun.RunID)
	assertCountEq(`SELECT COUNT(*) FROM constructor_manifest WHERE run_id = ?`, 1, newRun.RunID)
}
