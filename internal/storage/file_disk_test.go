package storage

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestDiskFileReader_Read(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	content := []byte("hello world")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	r := NewDiskFileReader()
	buf, truncated, err := r.Read(context.Background(), path, 1024)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if truncated {
		t.Error("expected not truncated")
	}
	if string(buf) != string(content) {
		t.Errorf("content = %q, want %q", buf, content)
	}
}

func TestDiskFileReader_Read_Truncated(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "large.txt")
	content := []byte("1234567890") // 10 bytes
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	r := NewDiskFileReader()
	buf, truncated, err := r.Read(context.Background(), path, 5)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if !truncated {
		t.Error("expected truncated")
	}
	if len(buf) != 5 {
		t.Errorf("len(buf) = %d, want 5", len(buf))
	}
	if string(buf) != "12345" {
		t.Errorf("content = %q, want 12345", buf)
	}
}

func TestDiskFileReader_Read_MissingFile(t *testing.T) {
	r := NewDiskFileReader()
	_, _, err := r.Read(context.Background(), "/nonexistent/path/file.txt", 1024)
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}
