package store

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tinoosan/workbench-core/internal/types"
)

func TestDiskMemoryStore_UpdateRoundTripAndClear(t *testing.T) {
	s, err := NewDiskMemoryStoreFromDir(t.TempDir())
	if err != nil {
		t.Fatalf("NewDiskMemoryStoreFromDir: %v", err)
	}
	ctx := context.Background()

	if got, err := s.GetUpdate(ctx); err != nil || got != "" {
		t.Fatalf("GetUpdate initial got=%q err=%v", got, err)
	}
	if err := s.SetUpdate(ctx, "hello"); err != nil {
		t.Fatalf("SetUpdate: %v", err)
	}
	if got, err := s.GetUpdate(ctx); err != nil || got != "hello" {
		t.Fatalf("GetUpdate got=%q err=%v", got, err)
	}
	if err := s.ClearUpdate(ctx); err != nil {
		t.Fatalf("ClearUpdate: %v", err)
	}
	if got, err := s.GetUpdate(ctx); err != nil || got != "" {
		t.Fatalf("GetUpdate after clear got=%q err=%v", got, err)
	}
}

func TestDiskMemoryStore_AppendMemory_Appends(t *testing.T) {
	s, err := NewDiskMemoryStoreFromDir(t.TempDir())
	if err != nil {
		t.Fatalf("NewDiskMemoryStoreFromDir: %v", err)
	}
	ctx := context.Background()

	if err := s.AppendMemory(ctx, "a"); err != nil {
		t.Fatalf("AppendMemory: %v", err)
	}
	if err := s.AppendMemory(ctx, "b"); err != nil {
		t.Fatalf("AppendMemory: %v", err)
	}
	got, err := s.GetMemory(ctx)
	if err != nil {
		t.Fatalf("GetMemory: %v", err)
	}
	if !strings.Contains(got, "a") || !strings.Contains(got, "b") {
		t.Fatalf("unexpected memory contents: %q", got)
	}
}

func TestDiskMemoryStore_AppendCommitLog_WritesJSONL(t *testing.T) {
	dir := t.TempDir()
	s, err := NewDiskMemoryStoreFromDir(dir)
	if err != nil {
		t.Fatalf("NewDiskMemoryStoreFromDir: %v", err)
	}
	ctx := context.Background()

	if err := s.AppendCommitLog(ctx, types.MemoryCommitLine{
		Model:    "m",
		Turn:     1,
		Accepted: true,
		Reason:   "accepted",
		Bytes:    12,
		SHA256:   "abc",
	}); err != nil {
		t.Fatalf("AppendCommitLog: %v", err)
	}

	b, err := os.ReadFile(filepath.Join(dir, "commits.jsonl"))
	if err != nil {
		t.Fatalf("ReadFile commits.jsonl: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}
	var parsed types.MemoryCommitLine
	if err := json.Unmarshal([]byte(lines[0]), &parsed); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if parsed.Turn != 1 || parsed.SHA256 != "abc" {
		t.Fatalf("unexpected parsed line: %+v", parsed)
	}
	if strings.TrimSpace(parsed.Timestamp) == "" {
		t.Fatalf("expected timestamp to be set")
	}
}
