package infra

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/tinoosan/agen8-mcp-server/internal/services/metrics/domain"
	storagedb "github.com/tinoosan/agen8-mcp-server/internal/storage/db"
)

// SQLiteCostAggregator implements domain.CostAggregator using SQLite.
type SQLiteCostAggregator struct {
	db      *sql.DB
	dialect storagedb.Dialect
}

func NewSQLCostAggregator(handle *storagedb.Handle) *SQLiteCostAggregator {
	return &SQLiteCostAggregator{db: handle.DB(), dialect: handle.Dialect()}
}

func (a *SQLiteCostAggregator) rebind(query string) string {
	return storagedb.Rebind(query, a.dialect)
}

// AggregateBySpace returns summed tokens/cost for a space, optionally filtered by role.
// Queries the runs table first (authoritative). Falls back to the tasks table if
// runs data is empty (e.g. after space deletion cleaned up runs).
func (a *SQLiteCostAggregator) AggregateBySpace(ctx context.Context, spaceID, role string) (domain.TokenCost, int, error) {
	if a.db == nil {
		return domain.TokenCost{}, 0, nil
	}
	spaceID = strings.TrimSpace(spaceID)
	if spaceID == "" {
		return domain.TokenCost{}, 0, nil
	}

	tc, count, err := a.aggregateFromRuns(ctx, spaceID, role)
	if err == nil && (!tc.IsZero() || count > 0) {
		return tc, count, nil
	}
	// Runs query failed or returned empty — fall back to tasks table.
	tc, count, err = a.aggregateFromTasks(ctx, spaceID, role)
	return tc, count, err
}

func (a *SQLiteCostAggregator) aggregateFromRuns(ctx context.Context, spaceID, role string) (domain.TokenCost, int, error) {
	query := `
		SELECT
			COALESCE(SUM(CAST(json_extract(run_json, '$.inputTokens') AS INTEGER)), 0),
			COALESCE(SUM(CAST(json_extract(run_json, '$.outputTokens') AS INTEGER)), 0),
			COALESCE(SUM(CAST(json_extract(run_json, '$.costUSD') AS REAL)), 0.0),
			COUNT(*)
		FROM runs
		WHERE space_id = ?
	`
	args := []any{spaceID}
	role = strings.TrimSpace(role)
	if role != "" {
		query += ` AND LOWER(COALESCE(json_extract(run_json, '$.runtime.role'), '')) = LOWER(?)`
		args = append(args, role)
	}

	var tokensIn, tokensOut, count int
	var costUSD float64
	if err := a.db.QueryRowContext(ctx, a.rebind(query), args...).Scan(&tokensIn, &tokensOut, &costUSD, &count); err != nil {
		return domain.TokenCost{}, 0, fmt.Errorf("aggregate from runs: %w", err)
	}
	return domain.TokenCost{InputTokens: tokensIn, OutputTokens: tokensOut, CostUSD: costUSD}, count, nil
}

func (a *SQLiteCostAggregator) aggregateFromTasks(ctx context.Context, spaceID, role string) (domain.TokenCost, int, error) {
	query := `
		SELECT
			COALESCE(SUM(input_tokens), 0),
			COALESCE(SUM(output_tokens), 0),
			COALESCE(SUM(cost_usd), 0.0),
			COUNT(DISTINCT NULLIF(run_id, ''))
		FROM tasks
		WHERE space_id = ?
	`
	args := []any{spaceID}
	role = strings.TrimSpace(role)
	if role != "" {
		query += ` AND assigned_role = ?`
		args = append(args, role)
	}

	var tokensIn, tokensOut, count int
	var costUSD float64
	if err := a.db.QueryRowContext(ctx, a.rebind(query), args...).Scan(&tokensIn, &tokensOut, &costUSD, &count); err != nil {
		return domain.TokenCost{}, 0, fmt.Errorf("aggregate from tasks: %w", err)
	}
	return domain.TokenCost{InputTokens: tokensIn, OutputTokens: tokensOut, CostUSD: costUSD}, count, nil
}
