package store

import (
	"database/sql"
	"testing"
)

// RunTestMigrations runs all SQLite migrations on the provided in-memory DB.
// This is exported for use by test files in other packages.
func RunTestMigrations(t *testing.T, db *sql.DB) {
	t.Helper()
	stmts, err := loadMigrationSQL()
	if err != nil {
		t.Fatalf("load migrations: %v", err)
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("migrate: %v (stmt: %s)", err, stmt[:min(80, len(stmt))])
		}
	}
}
