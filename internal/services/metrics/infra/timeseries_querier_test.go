package infra

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/tinoosan/agen8-mcp-server/internal/services/metrics/domain"
	storagedb "github.com/tinoosan/agen8-mcp-server/internal/storage/db"
)

func setupTimeSeriesHandle(t *testing.T) *storagedb.Handle {
	t.Helper()
	handle, err := storagedb.Open(context.Background(), storagedb.Config{
		Driver:  storagedb.DriverSQLite,
		DataDir: t.TempDir(),
		Migrate: func(ctx context.Context, db *sql.DB, driver storagedb.Driver) error {
			_, err := db.ExecContext(ctx, `
				CREATE TABLE events (
					event_id TEXT PRIMARY KEY,
					run_id TEXT NOT NULL,
					ts TEXT NOT NULL,
					type TEXT NOT NULL,
					message TEXT,
					data_json TEXT
				);
				CREATE TABLE tasks (
					task_id TEXT PRIMARY KEY,
					run_id TEXT,
					status TEXT,
					finished_at TEXT,
					created_at TEXT
				);
			`)
			return err
		},
	})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	return handle
}

func setupTimeSeriesHandleWithoutSchema(t *testing.T) *storagedb.Handle {
	t.Helper()
	handle, err := storagedb.Open(context.Background(), storagedb.Config{
		Driver:  storagedb.DriverSQLite,
		DataDir: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	return handle
}

func TestTimeSeriesQuerier_QueryError_ReturnsError(t *testing.T) {
	handle := setupTimeSeriesHandleWithoutSchema(t)
	// Don't create tables — query will fail
	q := NewSQLTimeSeriesQuerier(handle)

	tr := domain.TimeRange{
		From: time.Now().Add(-1 * time.Hour),
		To:   time.Now(),
	}
	_, err := q.Query(context.Background(), []string{"run-1"}, domain.MetricCost, domain.GranularityHour, tr)
	if err == nil {
		t.Fatalf("expected error when table doesn't exist, got nil")
	}
}

func TestTimeSeriesQuerier_ValidQuery_ReturnsData(t *testing.T) {
	handle := setupTimeSeriesHandle(t)
	db := handle.DB()
	now := time.Now().UTC()
	ts := now.Add(-30 * time.Minute).Format(time.RFC3339)

	if _, err := db.Exec(`INSERT INTO events (event_id, run_id, ts, type, data_json) VALUES (?, ?, ?, ?, ?)`,
		"evt-1", "run-1", ts, "llm.cost.total", `{"costUSD": 0.05}`); err != nil {
		t.Fatalf("insert event: %v", err)
	}

	q := NewSQLTimeSeriesQuerier(handle)
	tr := domain.TimeRange{From: now.Add(-1 * time.Hour), To: now.Add(1 * time.Hour)}

	result, err := q.Query(context.Background(), []string{"run-1"}, domain.MetricCost, domain.GranularityHour, tr)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(result.Points) == 0 {
		t.Fatalf("expected at least 1 data point, got 0")
	}
	if result.Total < 0.04 {
		t.Fatalf("expected total >= 0.04, got %f", result.Total)
	}
}

func TestTimeSeriesQuerier_EmptyRunIDs_ReturnsEmpty(t *testing.T) {
	handle := setupTimeSeriesHandle(t)
	q := NewSQLTimeSeriesQuerier(handle)
	tr := domain.TimeRange{From: time.Now().Add(-1 * time.Hour), To: time.Now()}

	result, err := q.Query(context.Background(), nil, domain.MetricCost, domain.GranularityHour, tr)
	if err != nil {
		t.Fatalf("expected no error for empty runIDs, got %v", err)
	}
	if len(result.Points) != 0 {
		t.Fatalf("expected 0 points for empty runIDs, got %d", len(result.Points))
	}
}
