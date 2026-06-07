package infra

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/tinoosan/agen8-mcp-server/internal/services/mission/domain/kr"
	"github.com/tinoosan/agen8-mcp-server/internal/services/mission/domain/mission"
	storagedb "github.com/tinoosan/agen8-mcp-server/internal/storage/db"
)

type PostgresRepository struct {
	db      *sql.DB
	dialect storagedb.Dialect
}

func NewPostgresRepository(handle *storagedb.Handle) (*PostgresRepository, error) {
	if handle == nil || handle.DB() == nil {
		return nil, fmt.Errorf("mission postgres repository: db handle is required")
	}
	if handle.Driver() != storagedb.DriverPostgres {
		return nil, fmt.Errorf("mission postgres repository: storage driver must be postgres, got %q", handle.Driver())
	}
	repo := &PostgresRepository{db: handle.DB(), dialect: handle.Dialect()}
	if err := repo.ensureSchema(context.Background()); err != nil {
		return nil, err
	}
	return repo, nil
}

func (r *PostgresRepository) GetMission(ctx context.Context, missionID mission.MissionID) (mission.Mission, error) {
	missionID = mission.MissionID(strings.TrimSpace(string(missionID)))
	if missionID == "" {
		return mission.Mission{}, fmt.Errorf("mission id is required")
	}
	var raw []byte
	err := r.db.QueryRowContext(ctx, r.rebind(`SELECT mission_json FROM missions WHERE mission_id = ?`), string(missionID)).Scan(&raw)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return mission.Mission{}, fmt.Errorf("mission %s not found", missionID)
		}
		return mission.Mission{}, fmt.Errorf("get mission %s: %w", missionID, err)
	}
	out, err := unmarshalMission(raw)
	if err != nil {
		return mission.Mission{}, fmt.Errorf("unmarshal mission %s: %w", missionID, err)
	}
	return out, nil
}

func (r *PostgresRepository) ListMissions(ctx context.Context, filter mission.MissionFilter) ([]mission.Mission, error) {
	where, args := missionWhere(filter)
	query := "SELECT mission_json FROM missions" + where + " ORDER BY created_at DESC, mission_id ASC"
	if filter.Limit > 0 {
		query += " LIMIT ?"
		args = append(args, filter.Limit)
	}
	if filter.Offset > 0 {
		query += " OFFSET ?"
		args = append(args, filter.Offset)
	}
	rows, err := r.db.QueryContext(ctx, r.rebind(query), args...)
	if err != nil {
		return nil, fmt.Errorf("list missions: %w", err)
	}
	defer rows.Close()
	return scanMissions(rows)
}

func (r *PostgresRepository) CreateMission(ctx context.Context, mission mission.Mission) error {
	return r.saveMission(ctx, mission)
}

func (r *PostgresRepository) UpdateMission(ctx context.Context, mission mission.Mission) error {
	if strings.TrimSpace(string(mission.ID)) == "" {
		return fmt.Errorf("mission id is required")
	}
	if _, err := r.GetMission(ctx, mission.ID); err != nil {
		return err
	}
	return r.saveMission(ctx, mission)
}

// DeleteMission hard-deletes a mission and all of its descendants in one
// transaction. Order matters: progress entries reference key results, so they
// go first, then the key results, the mission's lifecycle events, and finally
// the mission row itself. If any step fails the whole delete rolls back, so we
// never strand orphaned key results or progress entries.
func (r *PostgresRepository) DeleteMission(ctx context.Context, missionID mission.MissionID) error {
	missionID = mission.MissionID(strings.TrimSpace(string(missionID)))
	if missionID == "" {
		return fmt.Errorf("mission id is required")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("delete mission %s: begin tx: %w", missionID, err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	stmts := []struct {
		desc  string
		query string
	}{
		{"progress entries", `DELETE FROM key_result_progress_entries WHERE key_result_id IN (SELECT key_result_id FROM key_results WHERE mission_id = ?)`},
		{"key results", `DELETE FROM key_results WHERE mission_id = ?`},
		{"lifecycle events", `DELETE FROM mission_lifecycle_events WHERE mission_id = ?`},
		{"mission", `DELETE FROM missions WHERE mission_id = ?`},
	}
	for _, stmt := range stmts {
		if _, err := tx.ExecContext(ctx, r.rebind(stmt.query), string(missionID)); err != nil {
			return fmt.Errorf("delete mission %s (%s): %w", missionID, stmt.desc, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("delete mission %s: commit: %w", missionID, err)
	}
	committed = true
	return nil
}

func (r *PostgresRepository) GetKeyResult(ctx context.Context, keyResultID kr.KeyResultID) (kr.KeyResult, error) {
	keyResultID = kr.KeyResultID(strings.TrimSpace(string(keyResultID)))
	if keyResultID == "" {
		return kr.KeyResult{}, fmt.Errorf("key result id is required")
	}
	var raw []byte
	err := r.db.QueryRowContext(ctx, r.rebind(`SELECT key_result_json FROM key_results WHERE key_result_id = ?`), string(keyResultID)).Scan(&raw)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return kr.KeyResult{}, fmt.Errorf("key result %s not found", keyResultID)
		}
		return kr.KeyResult{}, fmt.Errorf("get key result %s: %w", keyResultID, err)
	}
	out, err := unmarshalKeyResult(raw)
	if err != nil {
		return kr.KeyResult{}, fmt.Errorf("unmarshal key result %s: %w", keyResultID, err)
	}
	return out, nil
}

func (r *PostgresRepository) ListKeyResults(ctx context.Context, missionID mission.MissionID) ([]kr.KeyResult, error) {
	missionID = mission.MissionID(strings.TrimSpace(string(missionID)))
	if missionID == "" {
		return nil, fmt.Errorf("mission id is required")
	}
	rows, err := r.db.QueryContext(ctx, r.rebind(`
		SELECT key_result_json
		FROM key_results
		WHERE mission_id = ?
		ORDER BY created_at ASC, key_result_id ASC
	`), string(missionID))
	if err != nil {
		return nil, fmt.Errorf("list key results: %w", err)
	}
	defer rows.Close()
	return scanKeyResults(rows)
}

func (r *PostgresRepository) CreateKeyResult(ctx context.Context, keyResult kr.KeyResult) error {
	return r.saveKeyResult(ctx, keyResult)
}

func (r *PostgresRepository) UpdateKeyResult(ctx context.Context, keyResult kr.KeyResult) error {
	if strings.TrimSpace(string(keyResult.ID)) == "" {
		return fmt.Errorf("key result id is required")
	}
	if _, err := r.GetKeyResult(ctx, keyResult.ID); err != nil {
		return err
	}
	return r.saveKeyResult(ctx, keyResult)
}

func (r *PostgresRepository) AppendProgressEntry(ctx context.Context, entry kr.ProgressEntry) error {
	if strings.TrimSpace(entry.ID) == "" {
		return fmt.Errorf("progress entry id is required")
	}
	if strings.TrimSpace(string(entry.KeyResultID)) == "" {
		return fmt.Errorf("progress entry key result id is required")
	}
	payload, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshal progress entry: %w", err)
	}
	_, err = r.db.ExecContext(ctx, r.rebind(`
		INSERT INTO key_result_progress_entries (
			progress_entry_id, key_result_id, created_at, progress_entry_json
		) VALUES (?, ?, ?, ?)
	`), entry.ID, string(entry.KeyResultID), timeString(entry.CreatedAt), string(payload))
	if err != nil {
		return fmt.Errorf("append progress entry %s: %w", entry.ID, err)
	}
	return nil
}

func (r *PostgresRepository) ListProgressEntries(ctx context.Context, keyResultID kr.KeyResultID) ([]kr.ProgressEntry, error) {
	keyResultID = kr.KeyResultID(strings.TrimSpace(string(keyResultID)))
	if keyResultID == "" {
		return nil, fmt.Errorf("key result id is required")
	}
	rows, err := r.db.QueryContext(ctx, r.rebind(`
		SELECT progress_entry_json
		FROM key_result_progress_entries
		WHERE key_result_id = ?
		ORDER BY created_at ASC, progress_entry_id ASC
	`), string(keyResultID))
	if err != nil {
		return nil, fmt.Errorf("list progress entries: %w", err)
	}
	defer rows.Close()
	var out []kr.ProgressEntry
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, fmt.Errorf("scan progress entry: %w", err)
		}
		var entry kr.ProgressEntry
		if err := json.Unmarshal(raw, &entry); err != nil {
			return nil, fmt.Errorf("unmarshal progress entry: %w", err)
		}
		out = append(out, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate progress entries: %w", err)
	}
	return out, nil
}

func (r *PostgresRepository) saveMission(ctx context.Context, item mission.Mission) error {
	item.ID = mission.MissionID(strings.TrimSpace(string(item.ID)))
	if item.ID == "" {
		return fmt.Errorf("mission id is required")
	}
	item.ProjectID = strings.TrimSpace(item.ProjectID)
	if item.ProjectID == "" {
		return fmt.Errorf("mission project id is required")
	}
	item.Status = mission.MissionStatus(strings.TrimSpace(string(item.Status)))
	if item.Status == "" {
		return fmt.Errorf("mission status is required")
	}
	payload, err := json.Marshal(item)
	if err != nil {
		return fmt.Errorf("marshal mission: %w", err)
	}
	_, err = r.db.ExecContext(ctx, r.rebind(`
		INSERT INTO missions (
			mission_id, project_id, status, created_at, updated_at, completed_at, mission_json
		) VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(mission_id) DO UPDATE SET
			project_id = excluded.project_id,
			status = excluded.status,
			updated_at = excluded.updated_at,
			completed_at = excluded.completed_at,
			mission_json = excluded.mission_json
	`), string(item.ID), item.ProjectID, string(item.Status), timeString(item.CreatedAt), timeString(item.UpdatedAt), optionalTimeString(item.CompletedAt), string(payload))
	if err != nil {
		return fmt.Errorf("save mission %s: %w", item.ID, err)
	}
	return nil
}

func (r *PostgresRepository) saveKeyResult(ctx context.Context, keyResult kr.KeyResult) error {
	keyResult.ID = kr.KeyResultID(strings.TrimSpace(string(keyResult.ID)))
	if keyResult.ID == "" {
		return fmt.Errorf("key result id is required")
	}
	keyResult.MissionID = mission.MissionID(strings.TrimSpace(string(keyResult.MissionID)))
	if keyResult.MissionID == "" {
		return fmt.Errorf("key result mission id is required")
	}
	keyResult.Status = kr.KeyResultStatus(strings.TrimSpace(string(keyResult.Status)))
	if keyResult.Status == "" {
		return fmt.Errorf("key result status is required")
	}
	payload, err := json.Marshal(keyResult)
	if err != nil {
		return fmt.Errorf("marshal key result: %w", err)
	}
	_, err = r.db.ExecContext(ctx, r.rebind(`
		INSERT INTO key_results (
			key_result_id, mission_id, status, project_id, created_at, updated_at, completed_at, key_result_json
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(key_result_id) DO UPDATE SET
			mission_id = excluded.mission_id,
			status = excluded.status,
			project_id = excluded.project_id,
			updated_at = excluded.updated_at,
			completed_at = excluded.completed_at,
			key_result_json = excluded.key_result_json
	`), string(keyResult.ID), string(keyResult.MissionID), string(keyResult.Status), strings.TrimSpace(keyResult.ProjectID), timeString(keyResult.CreatedAt), timeString(keyResult.UpdatedAt), optionalTimeString(keyResult.CompletedAt), string(payload))
	if err != nil {
		return fmt.Errorf("save key result %s: %w", keyResult.ID, err)
	}
	return nil
}

func (r *PostgresRepository) ensureSchema(ctx context.Context) error {
	for _, stmt := range []string{
		`CREATE TABLE IF NOT EXISTS missions (
			mission_id TEXT PRIMARY KEY,
			project_id TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT '',
			created_at TEXT,
			updated_at TEXT,
			completed_at TEXT,
			mission_json JSONB NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_missions_project ON missions(project_id)`,
		`CREATE INDEX IF NOT EXISTS idx_missions_status ON missions(status)`,
		`CREATE INDEX IF NOT EXISTS idx_missions_created_at ON missions(created_at)`,
		`CREATE TABLE IF NOT EXISTS key_results (
			key_result_id TEXT PRIMARY KEY,
			mission_id TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT '',
			project_id TEXT NOT NULL DEFAULT '',
			created_at TEXT,
			updated_at TEXT,
			completed_at TEXT,
			key_result_json JSONB NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_key_results_mission ON key_results(mission_id)`,
		`CREATE INDEX IF NOT EXISTS idx_key_results_status ON key_results(status)`,
		`CREATE INDEX IF NOT EXISTS idx_key_results_project ON key_results(project_id)`,
		`CREATE TABLE IF NOT EXISTS key_result_progress_entries (
			progress_entry_id TEXT PRIMARY KEY,
			key_result_id TEXT NOT NULL DEFAULT '',
			created_at TEXT,
			progress_entry_json JSONB NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_key_result_progress_key_result ON key_result_progress_entries(key_result_id)`,
		`CREATE TABLE IF NOT EXISTS mission_lifecycle_events (
			event_id TEXT PRIMARY KEY,
			mission_id TEXT NOT NULL DEFAULT '',
			key_result_id TEXT NOT NULL DEFAULT '',
			event_type TEXT NOT NULL DEFAULT '',
			created_at TEXT,
			event_json JSONB NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_mission_lifecycle_events_mission ON mission_lifecycle_events(mission_id, created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_mission_lifecycle_events_type ON mission_lifecycle_events(event_type)`,
	} {
		if _, err := r.db.ExecContext(ctx, r.rebind(stmt)); err != nil {
			return fmt.Errorf("ensure mission schema: %w", err)
		}
	}
	return nil
}

func (r *PostgresRepository) rebind(query string) string {
	return storagedb.Rebind(query, r.dialect)
}
