package infra

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/tinoosan/agen8-mcp-server/internal/services/metrics/domain"
	storagedb "github.com/tinoosan/agen8-mcp-server/internal/storage/db"
)

// SQLiteTimeSeriesQuerier implements domain.TimeSeriesQuerier using SQLite.
type SQLiteTimeSeriesQuerier struct {
	db      *sql.DB
	dialect storagedb.Dialect
}

func NewSQLTimeSeriesQuerier(handle *storagedb.Handle) *SQLiteTimeSeriesQuerier {
	return &SQLiteTimeSeriesQuerier{db: handle.DB(), dialect: handle.Dialect()}
}

func (q *SQLiteTimeSeriesQuerier) rebind(query string) string {
	return storagedb.Rebind(query, q.dialect)
}

func (q *SQLiteTimeSeriesQuerier) Query(ctx context.Context, runIDs []string, metric domain.Metric, granularity domain.Granularity, tr domain.TimeRange) (domain.TimeSeries, error) {
	empty := domain.TimeSeries{Points: []domain.TimeSeriesPoint{}}
	if q.db == nil || len(runIDs) == 0 {
		return empty, nil
	}

	timeFmt := granularityFormat(granularity)

	placeholders := make([]string, len(runIDs))
	args := make([]any, 0, len(runIDs)+2)
	for i, runID := range runIDs {
		placeholders[i] = "?"
		args = append(args, runID)
	}
	args = append(args, tr.From.Format(time.RFC3339), tr.To.Format(time.RFC3339))
	inClause := strings.Join(placeholders, ",")

	query, err := buildTimeSeriesQuery(metric, timeFmt, inClause)
	if err != nil {
		return empty, err
	}

	rows, err := q.db.QueryContext(ctx, q.rebind(query), args...)
	if err != nil {
		return empty, fmt.Errorf("timeseries query (%s): %w", metric, err)
	}
	defer rows.Close()

	var points []domain.TimeSeriesPoint
	total := 0.0
	for rows.Next() {
		var bucket sql.NullString
		var value sql.NullFloat64
		if err := rows.Scan(&bucket, &value); err == nil && bucket.Valid {
			v := value.Float64
			points = append(points, domain.TimeSeriesPoint{T: bucket.String, V: v})
			total += v
		}
	}
	if points == nil {
		points = []domain.TimeSeriesPoint{}
	}
	return domain.TimeSeries{Points: points, Total: total}, nil
}

func granularityFormat(g domain.Granularity) string {
	switch g {
	case domain.GranularityDay:
		return "%Y-%m-%dT00:00:00Z"
	case domain.GranularityMinute:
		return "%Y-%m-%dT%H:%M:00Z"
	default:
		return "%Y-%m-%dT%H:00:00Z"
	}
}

func buildTimeSeriesQuery(metric domain.Metric, timeFmt, inClause string) (string, error) {
	switch metric {
	case domain.MetricCost:
		return fmt.Sprintf(`
			SELECT strftime('%s', ts) AS bucket, SUM(CAST(json_extract(data_json, '$.costUSD') AS REAL)) AS value
			FROM events
			WHERE type = 'llm.cost.total'
			  AND run_id IN (%s)
			  AND ts >= ? AND ts < ?
			GROUP BY bucket
			ORDER BY bucket
		`, timeFmt, inClause), nil
	case domain.MetricTokens, domain.MetricTokensIn:
		return fmt.Sprintf(`
			SELECT strftime('%s', ts) AS bucket, SUM(CAST(json_extract(data_json, '$.input') AS INTEGER)) AS value
			FROM events
			WHERE type = 'llm.usage.total'
			  AND run_id IN (%s)
			  AND ts >= ? AND ts < ?
			GROUP BY bucket
			ORDER BY bucket
		`, timeFmt, inClause), nil
	case domain.MetricTokensOut:
		return fmt.Sprintf(`
			SELECT strftime('%s', ts) AS bucket, SUM(CAST(json_extract(data_json, '$.output') AS INTEGER)) AS value
			FROM events
			WHERE type = 'llm.usage.total'
			  AND run_id IN (%s)
			  AND ts >= ? AND ts < ?
			GROUP BY bucket
			ORDER BY bucket
		`, timeFmt, inClause), nil
	case domain.MetricCalls:
		return fmt.Sprintf(`
			SELECT strftime('%s', ts) AS bucket, COUNT(*) AS value
			FROM events
			WHERE type = 'llm.usage.total'
			  AND run_id IN (%s)
			  AND ts >= ? AND ts < ?
			GROUP BY bucket
			ORDER BY bucket
		`, timeFmt, inClause), nil
	case domain.MetricTasks:
		return fmt.Sprintf(`
			SELECT strftime('%s', finished_at) AS bucket, COUNT(*) AS value
			FROM tasks
			WHERE status = 'succeeded'
			  AND finished_at IS NOT NULL
			  AND run_id IN (%s)
			  AND finished_at >= ? AND finished_at < ?
			GROUP BY bucket
			ORDER BY bucket
		`, timeFmt, inClause), nil
	default:
		return "", &domain.ErrInvalidInput{Field: "metric", Message: fmt.Sprintf("unsupported metric: %s", metric)}
	}
}
