package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/tinoosan/agen8/pkg/config"
	"github.com/tinoosan/agen8/pkg/types"
)

func CountActivities(ctx context.Context, cfg config.Config, runID string) (int, error) {
	if err := cfg.Validate(); err != nil {
		return 0, err
	}
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return 0, fmt.Errorf("runID cannot be blank")
	}

	db, err := getSQLiteDB(cfg)
	if err != nil {
		return 0, err
	}
	if err := ensureActivitiesBackfilled(ctx, db, runID); err != nil {
		return 0, err
	}

	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM activities WHERE run_id = ?`, runID).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func ListActivities(ctx context.Context, cfg config.Config, runID string, limit, offset int) ([]types.Activity, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return nil, fmt.Errorf("runID cannot be blank")
	}
	if limit <= 0 {
		limit = 200
	}
	if offset < 0 {
		offset = 0
	}

	db, err := getSQLiteDB(cfg)
	if err != nil {
		return nil, err
	}
	if err := ensureActivitiesBackfilled(ctx, db, runID); err != nil {
		return nil, err
	}

	rows, err := db.QueryContext(
		ctx,
		`SELECT activity_id, kind, title, status, started_at, finished_at, meta_json
		 FROM activities
		 WHERE run_id = ?
		 ORDER BY seq
		 LIMIT ? OFFSET ?`,
		runID,
		limit,
		offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]types.Activity, 0, limit)
	for rows.Next() {
		var (
			id, kind, title, status, startedAt string
			finishedAt                         sql.NullString
			metaJSON                           sql.NullString
		)
		if err := rows.Scan(&id, &kind, &title, &status, &startedAt, &finishedAt, &metaJSON); err != nil {
			return nil, err
		}

		act := types.Activity{}
		if metaJSON.Valid && strings.TrimSpace(metaJSON.String) != "" {
			_ = json.Unmarshal([]byte(metaJSON.String), &act)
		}
		act.ID = strings.TrimSpace(id)
		act.Kind = strings.TrimSpace(kind)
		act.Title = strings.TrimSpace(title)
		act.Status = types.ActivityStatus(strings.TrimSpace(status))
		if ts, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(startedAt)); err == nil {
			act.StartedAt = ts
		}
		if finishedAt.Valid && strings.TrimSpace(finishedAt.String) != "" {
			if ts, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(finishedAt.String)); err == nil {
				act.FinishedAt = &ts
				if !act.StartedAt.IsZero() {
					act.Duration = ts.Sub(act.StartedAt)
				}
			}
		} else {
			act.FinishedAt = nil
			act.Duration = 0
		}

		out = append(out, act)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func ensureActivitiesBackfilled(ctx context.Context, db *sql.DB, runID string) error {
	if db == nil {
		return fmt.Errorf("db is nil")
	}
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return fmt.Errorf("runID cannot be blank")
	}
	var existing int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM activities WHERE run_id = ?`, runID).Scan(&existing); err != nil {
		return err
	}
	if existing > 0 {
		return nil
	}
	var hasEvents int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE run_id = ? AND type IN ('agent.op.request','agent.op.response')`, runID).Scan(&hasEvents); err != nil {
		return err
	}
	if hasEvents == 0 {
		return nil
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	rows, err := tx.QueryContext(ctx, `SELECT seq, event_json FROM events WHERE run_id = ? ORDER BY seq`, runID)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			seq int64
			raw string
		)
		if err := rows.Scan(&seq, &raw); err != nil {
			return err
		}
		var ev types.EventRecord
		if err := json.Unmarshal([]byte(raw), &ev); err != nil {
			continue
		}
		if err := upsertActivityFromEventTx(tx, runID, seq, ev); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	return nil
}
