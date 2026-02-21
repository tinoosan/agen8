package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/tinoosan/agen8/pkg/config"
)

type PruneResult struct {
	Sessions            int64
	Runs                int64
	Events              int64
	History             int64
	Activities          int64
	ConstructorState    int64
	ConstructorManifest int64
}

// PruneOldSessions deletes sessions with updated_at older than (now - olderThan),
// along with associated rows in child tables.
//
// Note: the current schema does not enforce foreign keys between session/run tables,
// so this function deletes from dependent tables explicitly.
func PruneOldSessions(ctx context.Context, cfg config.Config, olderThan time.Duration) (PruneResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := cfg.Validate(); err != nil {
		return PruneResult{}, err
	}
	if olderThan <= 0 {
		return PruneResult{}, fmt.Errorf("olderThan must be > 0: %w", ErrInvalid)
	}

	db, err := getSQLiteDB(cfg)
	if err != nil {
		return PruneResult{}, err
	}

	cutoff := time.Now().Add(-olderThan).UTC().Format(time.RFC3339Nano)

	tx, err := db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return PruneResult{}, fmt.Errorf("sqlite: begin prune: %w", err)
	}
	defer tx.Rollback()

	// Delete child rows first (derived from sessions.updated_at).
	res := PruneResult{}
	if res.Events, err = execRowsAffected(ctx, tx,
		`DELETE FROM events
		  WHERE run_id IN (
		    SELECT run_id FROM runs
		     WHERE session_id IN (SELECT session_id FROM sessions WHERE updated_at < ?)
		  )`,
		cutoff,
	); err != nil {
		return PruneResult{}, err
	}
	if res.History, err = execRowsAffected(ctx, tx,
		`DELETE FROM history WHERE session_id IN (SELECT session_id FROM sessions WHERE updated_at < ?)`,
		cutoff,
	); err != nil {
		return PruneResult{}, err
	}
	if res.Activities, err = execRowsAffected(ctx, tx,
		`DELETE FROM activities
		  WHERE run_id IN (
		    SELECT run_id FROM runs
		     WHERE session_id IN (SELECT session_id FROM sessions WHERE updated_at < ?)
		  )`,
		cutoff,
	); err != nil {
		return PruneResult{}, err
	}
	if res.ConstructorState, err = execRowsAffected(ctx, tx,
		`DELETE FROM constructor_state
		  WHERE run_id IN (
		    SELECT run_id FROM runs
		     WHERE session_id IN (SELECT session_id FROM sessions WHERE updated_at < ?)
		  )`,
		cutoff,
	); err != nil {
		return PruneResult{}, err
	}
	if res.ConstructorManifest, err = execRowsAffected(ctx, tx,
		`DELETE FROM constructor_manifest
		  WHERE run_id IN (
		    SELECT run_id FROM runs
		     WHERE session_id IN (SELECT session_id FROM sessions WHERE updated_at < ?)
		  )`,
		cutoff,
	); err != nil {
		return PruneResult{}, err
	}

	if res.Runs, err = execRowsAffected(ctx, tx,
		`DELETE FROM runs WHERE session_id IN (SELECT session_id FROM sessions WHERE updated_at < ?)`,
		cutoff,
	); err != nil {
		return PruneResult{}, err
	}
	if res.Sessions, err = execRowsAffected(ctx, tx,
		`DELETE FROM sessions WHERE updated_at < ?`,
		cutoff,
	); err != nil {
		return PruneResult{}, err
	}

	if err := tx.Commit(); err != nil {
		return PruneResult{}, fmt.Errorf("sqlite: commit prune: %w", err)
	}
	return res, nil
}

func execRowsAffected(ctx context.Context, tx *sql.Tx, query string, args ...any) (int64, error) {
	r, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, fmt.Errorf("sqlite: exec prune: %w", err)
	}
	n, err := r.RowsAffected()
	if err != nil {
		// If not supported, return 0 but do not fail pruning.
		return 0, nil
	}
	return n, nil
}

// Vacuum reclaims unused database space. It can be expensive and blocks writers.
func Vacuum(ctx context.Context, cfg config.Config) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := cfg.Validate(); err != nil {
		return err
	}
	db, err := getSQLiteDB(cfg)
	if err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, `VACUUM;`); err != nil {
		return fmt.Errorf("sqlite: vacuum: %w", err)
	}
	return nil
}
