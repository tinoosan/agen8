package infra

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	locationdomain "github.com/tinoosan/agen8/internal/services/location/domain"
)

func TestValidateLocalPath(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		input string
		errOK bool
	}{
		{name: "missing", input: "", errOK: false},
		{name: "relative", input: "tmp/project", errOK: false},
		{name: "backtrack", input: "/tmp/../etc", errOK: false},
		{name: "null-byte", input: "/tmp/\x00bad", errOK: false},
		{name: "tilde", input: "~", errOK: true},
		{name: "absolute", input: "/tmp", errOK: true},
		{name: "absolute-without-traversal", input: "/", errOK: true},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			_, err := validateLocalPath(tt.input)
			if tt.errOK && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !tt.errOK && err == nil {
				t.Fatalf("expected error")
			}
		})
	}
}

func TestTransportLocalMethodsRejectUnsafePaths(t *testing.T) {
	t.Parallel()

	transport := NewTransport()
	location := mustTestLocalLocation(t)
	ctx := context.Background()
	bad := filepath.Join("tmp", "..", "etc")

	if _, err := transport.ListDir(ctx, location, bad); err == nil {
		t.Fatalf("ListDir expected reject")
	}
	if _, err := transport.ListFiles(ctx, location, bad); err == nil {
		t.Fatalf("ListFiles expected reject")
	}
	if _, err := transport.ReadFile(ctx, location, bad, 16); err == nil {
		t.Fatalf("ReadFile expected reject")
	}
	if _, err := transport.StatFile(ctx, location, bad); err == nil {
		t.Fatalf("StatFile expected reject")
	}
	if err := transport.CreateDir(ctx, location, bad); err == nil {
		t.Fatalf("CreateDir expected reject")
	}
	if err := transport.CreateFile(ctx, location, bad); err == nil {
		t.Fatalf("CreateFile expected reject")
	}
	if err := transport.MoveFile(ctx, location, bad, bad); err == nil {
		t.Fatalf("MoveFile expected reject")
	}
	if err := transport.DeleteFile(ctx, location, bad); err == nil {
		t.Fatalf("DeleteFile expected reject")
	}
	if err := transport.WriteFile(ctx, location, bad, []byte("x")); err == nil {
		t.Fatalf("WriteFile expected reject")
	}
	if err := transport.CopyFile(ctx, location, bad, filepath.Join("tmp", "out")); err == nil {
		t.Fatalf("CopyFile expected reject")
	}
}

func TestTransportLocalCopyAndReadFiles(t *testing.T) {
	t.Parallel()

	transport := NewTransport()
	location := mustTestLocalLocation(t)
	ctx := context.Background()
	tmp := t.TempDir()
	src := filepath.Join(tmp, "source.txt")
	dst := filepath.Join(tmp, "nested", "dest.txt")

	if err := os.WriteFile(src, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	if err := transport.CopyFile(ctx, location, src, dst); err != nil {
		t.Fatalf("CopyFile: %v", err)
	}
	read, err := transport.ReadFile(ctx, location, dst, 16)
	if err != nil {
		t.Fatalf("ReadFile copied file: %v", err)
	}
	if string(read.Bytes) != "hello" {
		t.Fatalf("copied content mismatch: %q", string(read.Bytes))
	}
}

func TestListAndReadHelpersRejectUnsafePaths(t *testing.T) {
	t.Parallel()

	if _, err := listLocalDir("tmp"); err == nil {
		t.Fatal("listLocalDir should reject relative path")
	}
	if _, err := listLocalFiles("tmp/../etc"); err == nil {
		t.Fatal("listLocalFiles should reject traversal")
	}
	if _, err := readLocalFile("tmp/../etc", 16); err == nil {
		t.Fatal("readLocalFile should reject traversal")
	}
}

func mustTestLocalLocation(t *testing.T) locationdomain.Location {
	t.Helper()
	record := locationdomain.NewInput{
		ID:        "local",
		Kind:      locationdomain.KindLocal,
		Label:     "local",
		Status:    locationdomain.StatusOnline,
		Ready:     true,
		CreatedAt: fixedTestTime(),
		UpdatedAt: fixedTestTime(),
	}
	location, err := locationdomain.New(record)
	if err != nil {
		t.Fatalf("locationdomain.New: %v", err)
	}
	return location
}

func fixedTestTime() time.Time {
	return time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
}
