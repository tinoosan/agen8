package store

import (
	"testing"

	"github.com/tinoosan/workbench-core/pkg/config"
)

func TestSessionIndexesCreated(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}

	db, err := getSQLiteDB(cfg)
	if err != nil {
		t.Fatalf("getSQLiteDB: %v", err)
	}

	indexes := []string{
		"idx_sessions_updated_at",
		"idx_sessions_created_at",
	}

	for _, idx := range indexes {
		var name string
		err := db.QueryRow(
			`SELECT name FROM sqlite_master WHERE type='index' AND name=?`,
			idx,
		).Scan(&name)
		if err != nil {
			t.Errorf("index %s not found: %v", idx, err)
		}
	}
}

func TestEventTypeIndexCreated(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}

	db, err := getSQLiteDB(cfg)
	if err != nil {
		t.Fatalf("getSQLiteDB: %v", err)
	}

	var name string
	err = db.QueryRow(
		`SELECT name FROM sqlite_master WHERE type='index' AND name='idx_events_run_type'`,
	).Scan(&name)
	if err != nil {
		t.Errorf("index idx_events_run_type not found: %v", err)
	}
}
