package store

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDiskMemoryStore_DailyRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s, err := NewDiskMemoryStoreFromDir(dir)
	if err != nil {
		t.Fatalf("NewDiskMemoryStoreFromDir: %v", err)
	}
	ctx := context.Background()

	today := time.Now().Format("2006-01-02")
	content := "09:00 | decision | switched to daily memory files"
	if err := s.WriteMemory(ctx, today, content); err != nil {
		t.Fatalf("WriteMemory: %v", err)
	}
	got, err := s.ReadMemory(ctx, today)
	if err != nil {
		t.Fatalf("ReadMemory: %v", err)
	}
	if got != content {
		t.Fatalf("ReadMemory mismatch: got %q want %q", got, content)
	}

	if err := s.AppendMemory(ctx, today, "\nmore"); err != nil {
		t.Fatalf("AppendMemory: %v", err)
	}
	got, _ = s.ReadMemory(ctx, today)
	if got != content+"\nmore" {
		t.Fatalf("AppendMemory result mismatch: %q", got)
	}
}

func TestDiskMemoryStore_ListMemoryFiles(t *testing.T) {
	dir := t.TempDir()
	s, err := NewDiskMemoryStoreFromDir(dir)
	if err != nil {
		t.Fatalf("NewDiskMemoryStoreFromDir: %v", err)
	}
	ctx := context.Background()

	// Create yesterday file directly to ensure multiple files exist.
	yesterday := time.Now().Add(-24 * time.Hour).Format("2006-01-02")
	yesterdayPath := filepath.Join(dir, yesterday+"-memory.md")
	if err := os.WriteFile(yesterdayPath, []byte("old"), 0644); err != nil {
		t.Fatalf("write yesterday file: %v", err)
	}

	files, err := s.ListMemoryFiles(ctx)
	if err != nil {
		t.Fatalf("ListMemoryFiles: %v", err)
	}
	if len(files) < 2 {
		t.Fatalf("expected at least 2 files, got %d: %v", len(files), files)
	}
	foundMaster := false
	for _, f := range files {
		if f == "MEMORY.MD" {
			foundMaster = true
		}
	}
	if !foundMaster {
		t.Fatalf("MEMORY.MD missing from list: %v", files)
	}

	// Verify files exist on disk.
	for _, f := range files {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Fatalf("stat %s: %v", f, err)
		}
	}
}

func TestDiskMemoryStore_WriteMemory_RejectsNonToday(t *testing.T) {
	dir := t.TempDir()
	s, err := NewDiskMemoryStoreFromDir(dir)
	if err != nil {
		t.Fatalf("NewDiskMemoryStoreFromDir: %v", err)
	}
	ctx := context.Background()

	yesterday := time.Now().Add(-24 * time.Hour).Format("2006-01-02")
	err = s.WriteMemory(ctx, yesterday, "old")
	if err == nil {
		t.Fatalf("expected error when writing non-today memory file")
	}
	if !errors.Is(err, ErrMemoryWriteOnlyToday) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDiskMemoryStore_AppendMemory_RejectsNonToday(t *testing.T) {
	dir := t.TempDir()
	s, err := NewDiskMemoryStoreFromDir(dir)
	if err != nil {
		t.Fatalf("NewDiskMemoryStoreFromDir: %v", err)
	}
	ctx := context.Background()

	yesterday := time.Now().Add(-24 * time.Hour).Format("2006-01-02")
	err = s.AppendMemory(ctx, yesterday, "old")
	if err == nil {
		t.Fatalf("expected error when appending non-today memory file")
	}
	if !errors.Is(err, ErrMemoryWriteOnlyToday) {
		t.Fatalf("unexpected error: %v", err)
	}
}
