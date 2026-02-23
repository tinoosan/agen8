package store

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/tinoosan/agen8/pkg/types"
)

func TestMemorySessionStore_SaveLoad_Roundtrip(t *testing.T) {
	st := NewMemorySessionStore()
	ctx := context.Background()

	s := types.NewSession("t")
	s.CurrentGoal = "g"
	if err := st.SaveSession(ctx, s); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}
	got, err := st.LoadSession(ctx, s.SessionID)
	if err != nil {
		t.Fatalf("LoadSession: %v", err)
	}
	if got.SessionID != s.SessionID {
		t.Fatalf("SessionID=%q want %q", got.SessionID, s.SessionID)
	}
	if got.Title != "t" || got.CurrentGoal != "g" {
		t.Fatalf("unexpected fields: title=%q goal=%q", got.Title, got.CurrentGoal)
	}
}

func TestMemorySessionStore_Load_NotFound_IsErrNotFound(t *testing.T) {
	st := NewMemorySessionStore()
	_, err := st.LoadSession(context.Background(), "missing")
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

func TestMemorySessionStore_Save_MergesRunsAndPreservesCurrentRun(t *testing.T) {
	st := NewMemorySessionStore()
	ctx := context.Background()

	s := types.NewSession("t")
	s.Runs = []string{"run-0", "run-1"}
	s.CurrentRunID = "run-1"
	if err := st.SaveSession(ctx, s); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}

	// Incoming omits current run and adds a new run; it should keep the old current run.
	in := s
	in.Runs = []string{"run-1", "run-2"}
	in.CurrentRunID = ""
	if err := st.SaveSession(ctx, in); err != nil {
		t.Fatalf("SaveSession(merge): %v", err)
	}

	got, err := st.LoadSession(ctx, s.SessionID)
	if err != nil {
		t.Fatalf("LoadSession: %v", err)
	}
	if got.CurrentRunID != "run-1" {
		t.Fatalf("CurrentRunID=%q want %q", got.CurrentRunID, "run-1")
	}
	if len(got.Runs) != 3 || got.Runs[0] != "run-0" || got.Runs[1] != "run-1" || got.Runs[2] != "run-2" {
		t.Fatalf("Runs=%v", got.Runs)
	}
}

func TestMemorySessionStore_ListSessionsPaginated_SortsByUpdatedAtMeta(t *testing.T) {
	st := NewMemorySessionStore()
	ctx := context.Background()

	s1 := types.NewSession("a")
	s2 := types.NewSession("b")
	if err := st.SaveSession(ctx, s1); err != nil {
		t.Fatalf("SaveSession(s1): %v", err)
	}
	time.Sleep(10 * time.Millisecond)
	if err := st.SaveSession(ctx, s2); err != nil {
		t.Fatalf("SaveSession(s2): %v", err)
	}

	out, err := st.ListSessionsPaginated(ctx, SessionFilter{Limit: 10, SortBy: "updated_at", SortDesc: true})
	if err != nil {
		t.Fatalf("ListSessionsPaginated: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("len=%d want 2", len(out))
	}
	if out[0].SessionID != s2.SessionID {
		t.Fatalf("newest first: got %q want %q", out[0].SessionID, s2.SessionID)
	}
}

func TestMemorySessionStore_ListSessionsPaginated_TitleFilterAndCount(t *testing.T) {
	st := NewMemorySessionStore()
	ctx := context.Background()

	a := types.NewSession("alpha project")
	b := types.NewSession("beta project")
	c := types.NewSession("gamma task")
	if err := st.SaveSession(ctx, a); err != nil {
		t.Fatalf("SaveSession(a): %v", err)
	}
	if err := st.SaveSession(ctx, b); err != nil {
		t.Fatalf("SaveSession(b): %v", err)
	}
	if err := st.SaveSession(ctx, c); err != nil {
		t.Fatalf("SaveSession(c): %v", err)
	}

	filter := SessionFilter{TitleContains: "project", Limit: 10, SortBy: "title", SortDesc: false}
	count, err := st.CountSessions(ctx, filter)
	if err != nil {
		t.Fatalf("CountSessions: %v", err)
	}
	if count != 2 {
		t.Fatalf("count=%d want 2", count)
	}
	out, err := st.ListSessionsPaginated(ctx, filter)
	if err != nil {
		t.Fatalf("ListSessionsPaginated: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("len=%d want 2", len(out))
	}
}

func TestMemorySessionStore_ListSessionsPaginated_InvalidSortBy(t *testing.T) {
	st := NewMemorySessionStore()
	_, err := st.ListSessionsPaginated(context.Background(), SessionFilter{SortBy: "invalid_column"})
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestMemorySessionStore_ListSessionsPaginated_ProjectRootFilter(t *testing.T) {
	st := NewMemorySessionStore()
	ctx := context.Background()

	a := types.NewSession("proj a")
	a.ProjectRoot = "/proj/a"
	b := types.NewSession("proj b")
	b.ProjectRoot = "/proj/b"
	c := types.NewSession("no proj")
	if err := st.SaveSession(ctx, a); err != nil {
		t.Fatalf("SaveSession(a): %v", err)
	}
	if err := st.SaveSession(ctx, b); err != nil {
		t.Fatalf("SaveSession(b): %v", err)
	}
	if err := st.SaveSession(ctx, c); err != nil {
		t.Fatalf("SaveSession(c): %v", err)
	}

	filter := SessionFilter{ProjectRoot: "/proj/a", Limit: 10}
	out, err := st.ListSessionsPaginated(ctx, filter)
	if err != nil {
		t.Fatalf("ListSessionsPaginated: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("ProjectRoot /proj/a: len=%d want 1", len(out))
	}
	if strings.TrimSpace(out[0].SessionID) != strings.TrimSpace(a.SessionID) {
		t.Fatalf("ProjectRoot /proj/a: got %q want %q", out[0].SessionID, a.SessionID)
	}

	count, err := st.CountSessions(ctx, SessionFilter{ProjectRoot: "/proj/b"})
	if err != nil {
		t.Fatalf("CountSessions: %v", err)
	}
	if count != 1 {
		t.Fatalf("CountSessions ProjectRoot /proj/b: got %d want 1", count)
	}

	empty, err := st.ListSessionsPaginated(ctx, SessionFilter{ProjectRoot: "/nonexistent", Limit: 10})
	if err != nil {
		t.Fatalf("ListSessionsPaginated: %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("ProjectRoot /nonexistent: len=%d want 0", len(empty))
	}
}
