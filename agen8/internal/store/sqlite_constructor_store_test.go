package store

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/tinoosan/agen8/pkg/config"
	pkgstore "github.com/tinoosan/agen8/pkg/store"
)

func TestSQLiteConstructorStore_GetState_NotFound_Errors(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}

	s, err := NewSQLiteConstructorStore(cfg)
	if err != nil {
		t.Fatalf("NewSQLiteConstructorStore: %v", err)
	}

	ctx := context.Background()
	_, err = s.GetState(ctx, "nonexistent-run")
	if err == nil {
		t.Fatalf("expected error")
	}
	if !errors.Is(err, pkgstore.ErrNotFound) {
		t.Fatalf("expected errors.Is(err, pkgstore.ErrNotFound) to be true, err=%v", err)
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected errors.Is(err, os.ErrNotExist) to be true, err=%v", err)
	}
}

func TestSQLiteConstructorStore_GetManifest_NotFound_Errors(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}

	s, err := NewSQLiteConstructorStore(cfg)
	if err != nil {
		t.Fatalf("NewSQLiteConstructorStore: %v", err)
	}

	ctx := context.Background()
	_, err = s.GetManifest(ctx, "nonexistent-run")
	if err == nil {
		t.Fatalf("expected error")
	}
	if !errors.Is(err, pkgstore.ErrNotFound) {
		t.Fatalf("expected errors.Is(err, pkgstore.ErrNotFound) to be true, err=%v", err)
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected errors.Is(err, os.ErrNotExist) to be true, err=%v", err)
	}
}
