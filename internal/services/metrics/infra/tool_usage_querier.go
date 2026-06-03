package infra

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/tinoosan/agen8-mcp-server/internal/services/metrics/domain"
	storagedb "github.com/tinoosan/agen8-mcp-server/internal/storage/db"
)

// SQLiteToolUsageQuerier implements domain.ToolUsageQuerier using SQLite.
type SQLiteToolUsageQuerier struct {
	db      *sql.DB
	dialect storagedb.Dialect
}

func NewSQLToolUsageQuerier(handle *storagedb.Handle) *SQLiteToolUsageQuerier {
	return &SQLiteToolUsageQuerier{db: handle.DB(), dialect: handle.Dialect()}
}

func (q *SQLiteToolUsageQuerier) rebind(query string) string {
	return storagedb.Rebind(query, q.dialect)
}

func (q *SQLiteToolUsageQuerier) QueryByRun(ctx context.Context, runID string, limit int) ([]domain.ToolUsage, error) {
	if q.db == nil || runID == "" {
		return []domain.ToolUsage{}, nil
	}
	if limit <= 0 {
		limit = 20
	}

	durationExpr := `CASE WHEN completed_at IS NOT NULL AND created_at IS NOT NULL
				THEN (julianday(completed_at) - julianday(created_at)) * 86400000
				ELSE 0 END`
	if q.dialect != nil && q.dialect.Placeholder(1) != "?" {
		durationExpr = `CASE WHEN completed_at IS NOT NULL AND created_at IS NOT NULL
				THEN EXTRACT(EPOCH FROM (completed_at::timestamptz - created_at::timestamptz)) * 1000
				ELSE 0 END`
	}

	rows, err := q.db.QueryContext(ctx, q.rebind(`
		SELECT title, COUNT(*) AS cnt,
			COALESCE(AVG(`+durationExpr+`), 0) AS avg_ms
		FROM agent_space_entries
		WHERE run_id = ?
		  AND kind = 'tool_call'
		GROUP BY title
		ORDER BY cnt DESC
		LIMIT ?
	`), runID, limit)
	if err != nil {
		return nil, fmt.Errorf("tool usage query for run %s: %w", runID, err)
	}
	defer rows.Close()

	var result []domain.ToolUsage
	for rows.Next() {
		var kind string
		var cnt int
		var avgMs float64
		if err := rows.Scan(&kind, &cnt, &avgMs); err == nil {
			result = append(result, domain.ToolUsage{
				Tool:          kind,
				Count:         cnt,
				AvgDurationMs: int64(avgMs),
			})
		}
	}
	if result == nil {
		result = []domain.ToolUsage{}
	}
	return result, nil
}
