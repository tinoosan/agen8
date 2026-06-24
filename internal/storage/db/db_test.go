package db

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestOpenSQLiteConcurrentOpenUsesSingleHandle(t *testing.T) {
	dataDir := t.TempDir()
	originalOpen := openSQL
	var openCount int32
	openSQL = func(driverName, dataSourceName string) (*sql.DB, error) {
		atomic.AddInt32(&openCount, 1)
		time.Sleep(10 * time.Millisecond)
		return originalOpen(driverName, dataSourceName)
	}
	t.Cleanup(func() {
		openSQL = originalOpen
	})

	const workers = 20
	start := make(chan struct{})
	handles := make([]*Handle, workers)
	errs := make([]error, workers)
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func(i int) {
			defer wg.Done()
			<-start
			handles[i], errs[i] = Open(context.Background(), Config{DataDir: dataDir})
		}(i)
	}
	close(start)
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("Open[%d]: %v", i, err)
		}
	}
	first := handles[0]
	if first == nil || first.DB() == nil {
		t.Fatal("handle/db is nil")
	}
	for i, handle := range handles {
		if handle != first {
			t.Fatalf("handle[%d]=%p want %p", i, handle, first)
		}
	}
	if got := atomic.LoadInt32(&openCount); got != 1 {
		t.Fatalf("sqlite open count=%d want 1", got)
	}
}

func TestOpenPostgresRequiresDatabaseURL(t *testing.T) {
	_, err := Open(context.Background(), Config{Driver: DriverPostgres})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "database url is required") {
		t.Fatalf("error = %q", err)
	}
}

func TestOpenTracksMigrationsByScope(t *testing.T) {
	dataDir := t.TempDir()
	var runs int
	migrate := func(context.Context, *sql.DB, Driver) error {
		runs++
		return nil
	}

	if _, err := Open(context.Background(), Config{
		DataDir:      dataDir,
		MigrationKey: "one",
		Migrate:      migrate,
	}); err != nil {
		t.Fatalf("open one: %v", err)
	}
	if _, err := Open(context.Background(), Config{
		DataDir:      dataDir,
		MigrationKey: "two",
		Migrate:      migrate,
	}); err != nil {
		t.Fatalf("open two: %v", err)
	}
	if _, err := Open(context.Background(), Config{
		DataDir:      dataDir,
		MigrationKey: "one",
		Migrate:      migrate,
	}); err != nil {
		t.Fatalf("open one again: %v", err)
	}
	if runs != 2 {
		t.Fatalf("migration runs=%d want 2", runs)
	}
}

func TestDialects(t *testing.T) {
	sqlite := sqliteDialect{}
	if got := sqlite.Placeholder(3); got != "?" {
		t.Fatalf("sqlite placeholder = %q", got)
	}
	if got := sqlite.JSONType(); got != "TEXT" {
		t.Fatalf("sqlite json type = %q", got)
	}

	postgres := postgresDialect{}
	if got := postgres.Placeholder(3); got != "$3" {
		t.Fatalf("postgres placeholder = %q", got)
	}
	if got := postgres.JSONType(); got != "JSONB" {
		t.Fatalf("postgres json type = %q", got)
	}
}

func TestOpenSQLiteSecuresDatabaseFileMode(t *testing.T) {
	dataDir := t.TempDir()
	dbPath := filepath.Join(dataDir, "agen8.db")
	if err := os.WriteFile(dbPath, []byte{}, 0o644); err != nil {
		t.Fatalf("seed sqlite file: %v", err)
	}
	if err := os.Chmod(dbPath, 0o644); err != nil {
		t.Fatalf("seed chmod: %v", err)
	}
	if _, err := Open(context.Background(), Config{
		DataDir: dataDir,
	}); err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	info, err := os.Stat(dbPath)
	if err != nil {
		t.Fatalf("stat db: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("sqlite file perm=%#o want 0600", info.Mode().Perm())
	}
}

func TestRebind(t *testing.T) {
	query := `SELECT * FROM auth_users WHERE email = ? AND user_id = ?`
	if got := Rebind(query, sqliteDialect{}); got != query {
		t.Fatalf("sqlite rebind = %q", got)
	}
	if got := Rebind(query, postgresDialect{}); got != `SELECT * FROM auth_users WHERE email = $1 AND user_id = $2` {
		t.Fatalf("postgres rebind = %q", got)
	}
}
