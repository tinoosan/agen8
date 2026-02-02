package state

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/tinoosan/workbench-core/pkg/types"
)

func TestSQLiteStore_ClaimAndComplete(t *testing.T) {
	dir := t.TempDir()
	s, err := NewSQLiteStore(filepath.Join(dir, "workbench.db"), "agent-test")
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}

	ctx := context.Background()
	got, err := s.Claim(ctx, "t1", 200*time.Millisecond)
	if err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if !got.Claimed || got.Attempts != 1 {
		t.Fatalf("unexpected claim: %+v", got)
	}

	got2, err := s.Claim(ctx, "t1", 200*time.Millisecond)
	if err != nil {
		t.Fatalf("Claim(2): %v", err)
	}
	if got2.Claimed {
		t.Fatalf("expected second claim to be rejected while leased, got: %+v", got2)
	}

	doneAt := time.Now()
	if err := s.Complete(ctx, "t1", types.TaskResult{TaskID: "t1", Status: types.TaskStatusSucceeded, Summary: "ok", CompletedAt: &doneAt}); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	rec, ok, err := s.Get(ctx, "t1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !ok || rec.Status != StatusSucceeded {
		t.Fatalf("unexpected record: %+v ok=%v", rec, ok)
	}
}

func TestSQLiteStore_Claim_Expires(t *testing.T) {
	dir := t.TempDir()
	s, err := NewSQLiteStore(filepath.Join(dir, "workbench.db"), "agent-test")
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	ctx := context.Background()
	if _, err := s.Claim(ctx, "t1", 10*time.Millisecond); err != nil {
		t.Fatalf("Claim: %v", err)
	}
	time.Sleep(20 * time.Millisecond)
	got2, err := s.Claim(ctx, "t1", 50*time.Millisecond)
	if err != nil {
		t.Fatalf("Claim(2): %v", err)
	}
	if !got2.Claimed || got2.Attempts < 2 {
		t.Fatalf("expected re-claim after lease expiry, got: %+v", got2)
	}
}
