package store

import (
	"context"
	"path/filepath"
	"testing"
)

func TestDiskHistoryStore_CursorSince(t *testing.T) {
	tmp := t.TempDir()
	s, err := NewDiskHistoryStoreFromPath(filepath.Join(tmp, "history.jsonl"))
	if err != nil {
		t.Fatalf("NewDiskHistoryStoreFromPath: %v", err)
	}
	if err := s.AppendLine(context.Background(), []byte(`{"k":"v1"}`)); err != nil {
		t.Fatalf("AppendLine: %v", err)
	}
	if err := s.AppendLine(context.Background(), []byte(`{"k":"v2"}`)); err != nil {
		t.Fatalf("AppendLine: %v", err)
	}

	b1, err := s.LinesSince(context.Background(), HistoryCursorFromInt64(0), HistorySinceOptions{MaxBytes: 1024, Limit: 10})
	if err != nil {
		t.Fatalf("LinesSince: %v", err)
	}
	if len(b1.Lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(b1.Lines))
	}
	after1, err := HistoryCursorToInt64(b1.CursorAfter)
	if err != nil || after1 == 0 {
		t.Fatalf("expected cursorAfter > 0, got %v err=%v", after1, err)
	}

	// No new lines => empty batch, cursor unchanged.
	b2, err := s.LinesSince(context.Background(), b1.CursorAfter, HistorySinceOptions{MaxBytes: 1024, Limit: 10})
	if err != nil {
		t.Fatalf("LinesSince 2: %v", err)
	}
	if len(b2.Lines) != 0 {
		t.Fatalf("expected 0 lines, got %d", len(b2.Lines))
	}
	if b2.CursorAfter != b1.CursorAfter {
		t.Fatalf("expected cursorAfter unchanged")
	}
}
