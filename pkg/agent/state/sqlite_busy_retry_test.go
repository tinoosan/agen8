package state

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
)

func TestWithSQLiteBusyRetry_RetriesBusyErrors(t *testing.T) {
	calls := 0
	out, err := withSQLiteBusyRetry(context.Background(), func() (string, error) {
		calls++
		if calls < 3 {
			return "", fmt.Errorf("database is locked (5) (SQLITE_BUSY)")
		}
		return "ok", nil
	})
	if err != nil {
		t.Fatalf("withSQLiteBusyRetry: %v", err)
	}
	if out != "ok" {
		t.Fatalf("result = %q, want ok", out)
	}
	if calls != 3 {
		t.Fatalf("calls = %d, want 3", calls)
	}
}

func TestSQLiteTaskStore_UsesBusyTimeoutEnv(t *testing.T) {
	t.Setenv("AGEN8_SQLITE_BUSY_TIMEOUT_MS", "16000")
	store, err := NewSQLiteTaskStore(filepath.Join(t.TempDir(), "agen8.db"))
	if err != nil {
		t.Fatalf("NewSQLiteTaskStore: %v", err)
	}
	db, err := store.dbConn()
	if err != nil {
		t.Fatalf("dbConn: %v", err)
	}
	var timeoutMS int
	if err := db.QueryRow(`PRAGMA busy_timeout;`).Scan(&timeoutMS); err != nil {
		t.Fatalf("PRAGMA busy_timeout: %v", err)
	}
	if timeoutMS != 16000 {
		t.Fatalf("busy_timeout=%d, want 16000", timeoutMS)
	}
}
