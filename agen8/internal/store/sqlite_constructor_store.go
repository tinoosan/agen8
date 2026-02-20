package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/tinoosan/agen8/pkg/config"
	pkgstore "github.com/tinoosan/agen8/pkg/store"
)

// SQLiteConstructorStore persists constructor state/manifest in SQLite.
type SQLiteConstructorStore struct {
	DB *sql.DB
}

func NewSQLiteConstructorStore(cfg config.Config) (*SQLiteConstructorStore, error) {
	db, err := getSQLiteDB(cfg)
	if err != nil {
		return nil, err
	}
	return &SQLiteConstructorStore{DB: db}, nil
}

func (s *SQLiteConstructorStore) GetState(_ context.Context, runID string) ([]byte, error) {
	if err := requireRunID(runID); err != nil {
		return nil, err
	}
	var raw []byte
	if err := s.DB.QueryRow(`SELECT state_json FROM constructor_state WHERE run_id = ?`, runID).Scan(&raw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("constructor state not found: %w", errors.Join(pkgstore.ErrNotFound, os.ErrNotExist))
		}
		return nil, err
	}
	return raw, nil
}

func (s *SQLiteConstructorStore) SetState(_ context.Context, runID string, stateJSON []byte) error {
	if err := requireRunID(runID); err != nil {
		return err
	}
	if len(stateJSON) == 0 {
		return nil
	}
	updatedAt := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.DB.Exec(
		`INSERT INTO constructor_state (run_id, updated_at, state_json)
		 VALUES (?, ?, ?)
		 ON CONFLICT(run_id) DO UPDATE SET
		   updated_at=excluded.updated_at,
		   state_json=excluded.state_json`,
		runID,
		updatedAt,
		stateJSON,
	)
	return err
}

func (s *SQLiteConstructorStore) GetManifest(_ context.Context, runID string) ([]byte, error) {
	if err := requireRunID(runID); err != nil {
		return nil, err
	}
	var raw []byte
	if err := s.DB.QueryRow(`SELECT manifest_json FROM constructor_manifest WHERE run_id = ?`, runID).Scan(&raw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("constructor manifest not found: %w", errors.Join(pkgstore.ErrNotFound, os.ErrNotExist))
		}
		return nil, err
	}
	return raw, nil
}

func (s *SQLiteConstructorStore) SetManifest(_ context.Context, runID string, manifestJSON []byte) error {
	if err := requireRunID(runID); err != nil {
		return err
	}
	if len(manifestJSON) == 0 {
		return nil
	}
	updatedAt := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.DB.Exec(
		`INSERT INTO constructor_manifest (run_id, updated_at, manifest_json)
		 VALUES (?, ?, ?)
		 ON CONFLICT(run_id) DO UPDATE SET
		   updated_at=excluded.updated_at,
		   manifest_json=excluded.manifest_json`,
		runID,
		updatedAt,
		manifestJSON,
	)
	return err
}

func requireRunID(runID string) error {
	if strings.TrimSpace(runID) == "" {
		return fmt.Errorf("runId is required")
	}
	return nil
}
