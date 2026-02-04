package store

import (
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/tinoosan/workbench-core/pkg/config"
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

	s1, _, err := CreateSession(cfg, "first", 64)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	time.Sleep(10 * time.Millisecond)
	s2, _, err := CreateSession(cfg, "second", 64)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	time.Sleep(10 * time.Millisecond)
	s3, _, err := CreateSession(cfg, "third", 64)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
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

	_ = s2
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
