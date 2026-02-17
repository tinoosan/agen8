package state

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"

	"github.com/tinoosan/workbench-core/pkg/timeutil"
	"github.com/tinoosan/workbench-core/pkg/types"
	"github.com/tinoosan/workbench-core/pkg/validate"
)

type SQLiteTaskStore struct {
	path string

	mu   sync.Mutex
	db   *sql.DB
	once sync.Once
}

func NewSQLiteTaskStore(path string) (*SQLiteTaskStore, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("sqlite store path is required")
	}
	return &SQLiteTaskStore{path: path}, nil
}

func (s *SQLiteTaskStore) init() error {
	var initErr error
	s.once.Do(func() {
		if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
			initErr = fmt.Errorf("sqlite: create dir: %w", err)
			return
		}
		db, err := sql.Open("sqlite", s.path)
		if err != nil {
			initErr = fmt.Errorf("sqlite: open: %w", err)
			return
		}
		db.SetMaxOpenConns(1)
		db.SetMaxIdleConns(1)
		if _, err := db.Exec(`PRAGMA journal_mode=WAL;`); err != nil {
			_ = db.Close()
			initErr = fmt.Errorf("sqlite: set journal_mode: %w", err)
			return
		}
		if _, err := db.Exec(`PRAGMA busy_timeout=5000;`); err != nil {
			_ = db.Close()
			initErr = fmt.Errorf("sqlite: set busy_timeout: %w", err)
			return
		}
		if _, err := db.Exec(`PRAGMA foreign_keys=ON;`); err != nil {
			// Best-effort: older databases/driver configs may ignore this.
			_ = err
		}

		if _, err := db.Exec(`
			CREATE TABLE IF NOT EXISTS tasks (
				task_id TEXT PRIMARY KEY,
				session_id TEXT NOT NULL,
				run_id TEXT NOT NULL,
				team_id TEXT DEFAULT '',
				assigned_role TEXT DEFAULT '',
				assigned_to_type TEXT DEFAULT 'agent',
				assigned_to TEXT DEFAULT '',
				claimed_by TEXT DEFAULT '',
				created_by TEXT DEFAULT '',
				goal TEXT NOT NULL,
				inputs_json TEXT,
				priority INTEGER DEFAULT 0,
				status TEXT NOT NULL,
				created_at TEXT NOT NULL,
				started_at TEXT,
				finished_at TEXT,
				completed_at TEXT,
				summary TEXT,
				artifacts_json TEXT,
				error TEXT,
				attempts INTEGER DEFAULT 0,
				lease_until TEXT,
				updated_at TEXT NOT NULL,
				input_tokens INTEGER DEFAULT 0,
				output_tokens INTEGER DEFAULT 0,
				total_tokens INTEGER DEFAULT 0,
				cost_usd REAL DEFAULT 0.0,
				duration_seconds INTEGER DEFAULT 0,
				metadata_json TEXT
			);
		`); err != nil {
			_ = db.Close()
			initErr = fmt.Errorf("sqlite: create tasks: %w", err)
			return
		}
		migrations := []string{
			`ALTER TABLE tasks ADD COLUMN team_id TEXT DEFAULT '';`,
			`ALTER TABLE tasks ADD COLUMN assigned_role TEXT DEFAULT '';`,
			`ALTER TABLE tasks ADD COLUMN assigned_to_type TEXT DEFAULT 'agent';`,
			`ALTER TABLE tasks ADD COLUMN assigned_to TEXT DEFAULT '';`,
			`ALTER TABLE tasks ADD COLUMN claimed_by TEXT DEFAULT '';`,
			`ALTER TABLE tasks ADD COLUMN created_by TEXT DEFAULT '';`,
			`ALTER TABLE tasks ADD COLUMN task_kind TEXT DEFAULT 'task';`,
			`ALTER TABLE tasks ADD COLUMN role_snapshot TEXT DEFAULT '';`,
			`ALTER TABLE tasks ADD COLUMN completed_at TEXT;`,
		}
		for _, migration := range migrations {
			if _, err := db.Exec(migration); err != nil {
				if !strings.Contains(strings.ToLower(err.Error()), "duplicate column name") {
					_ = db.Close()
					initErr = fmt.Errorf("sqlite: migrate tasks: %w", err)
					return
				}
			}
		}
		indexes := []string{
			`CREATE INDEX IF NOT EXISTS idx_tasks_session ON tasks(session_id);`,
			`CREATE INDEX IF NOT EXISTS idx_tasks_run ON tasks(run_id);`,
			`CREATE INDEX IF NOT EXISTS idx_tasks_status ON tasks(status);`,
			`CREATE INDEX IF NOT EXISTS idx_tasks_created ON tasks(created_at DESC);`,
			`CREATE INDEX IF NOT EXISTS idx_tasks_finished ON tasks(finished_at DESC);`,
			`CREATE INDEX IF NOT EXISTS idx_tasks_cost ON tasks(cost_usd DESC);`,
			`CREATE INDEX IF NOT EXISTS idx_tasks_run_status ON tasks(run_id, status);`,
			`CREATE INDEX IF NOT EXISTS idx_tasks_team_role ON tasks(team_id, assigned_role);`,
			`CREATE INDEX IF NOT EXISTS idx_tasks_team_kind ON tasks(team_id, task_kind);`,
			`CREATE INDEX IF NOT EXISTS idx_tasks_assigned_to ON tasks(assigned_to_type, assigned_to, status);`,
			`CREATE INDEX IF NOT EXISTS idx_tasks_claimed_by ON tasks(claimed_by, status);`,
		}
		for _, idx := range indexes {
			if _, err := db.Exec(idx); err != nil {
				_ = db.Close()
				initErr = fmt.Errorf("sqlite: create index: %w", err)
				return
			}
		}
		if _, err := db.Exec(`
			CREATE TABLE IF NOT EXISTS artifacts (
				artifact_id INTEGER PRIMARY KEY AUTOINCREMENT,
				task_id TEXT NOT NULL,
				team_id TEXT DEFAULT '',
				run_id TEXT DEFAULT '',
				role TEXT DEFAULT '',
				task_kind TEXT DEFAULT 'task',
				is_summary INTEGER DEFAULT 0,
				display_name TEXT DEFAULT '',
				vpath TEXT NOT NULL,
				disk_path TEXT DEFAULT '',
				produced_at TEXT NOT NULL,
				day_bucket TEXT NOT NULL
			);
		`); err != nil {
			_ = db.Close()
			initErr = fmt.Errorf("sqlite: create artifacts: %w", err)
			return
		}
		artifactIndexes := []string{
			`CREATE INDEX IF NOT EXISTS idx_artifacts_team_day_role_kind_time ON artifacts(team_id, day_bucket DESC, role, task_kind, produced_at DESC);`,
			`CREATE INDEX IF NOT EXISTS idx_artifacts_task_id ON artifacts(task_id);`,
			`CREATE INDEX IF NOT EXISTS idx_artifacts_team_display_name ON artifacts(team_id, display_name);`,
		}
		for _, idx := range artifactIndexes {
			if _, err := db.Exec(idx); err != nil {
				_ = db.Close()
				initErr = fmt.Errorf("sqlite: create artifact index: %w", err)
				return
			}
		}
		if err := s.backfillArtifactIndex(context.Background(), db); err != nil {
			_ = db.Close()
			initErr = fmt.Errorf("sqlite: backfill artifacts: %w", err)
			return
		}
		if err := migrateDeliverablesToTasks(context.Background(), db, filepath.Dir(s.path)); err != nil {
			_ = db.Close()
			initErr = fmt.Errorf("sqlite: migrate deliverables to tasks: %w", err)
			return
		}
		s.db = db
	})
	return initErr
}

func migrateDeliverablesToTasks(ctx context.Context, db *sql.DB, dataDir string) error {
	dataDir = strings.TrimSpace(dataDir)
	if db == nil || dataDir == "" {
		return nil
	}
	if err := migrateWorkspaceRootsToTasks(dataDir); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, `
		UPDATE tasks
		SET artifacts_json = REPLACE(artifacts_json, '/workspace/deliverables/', '/workspace/tasks/')
		WHERE COALESCE(artifacts_json, '') LIKE '%/workspace/deliverables/%'
	`); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, `
		UPDATE artifacts
		SET vpath = REPLACE(vpath, '/workspace/deliverables/', '/workspace/tasks/'),
		    disk_path = REPLACE(disk_path, '/workspace/deliverables/', '/workspace/tasks/')
		WHERE COALESCE(vpath, '') LIKE '/workspace/deliverables/%'
		   OR COALESCE(disk_path, '') LIKE '%/workspace/deliverables/%'
	`); err != nil {
		return err
	}
	return nil
}

func migrateWorkspaceRootsToTasks(dataDir string) error {
	agentsDir := filepath.Join(dataDir, "agents")
	teamsDir := filepath.Join(dataDir, "teams")
	if err := migrateWorkspaceRoots(filepath.Join(agentsDir), "workspace"); err != nil {
		return err
	}
	if err := migrateWorkspaceRoots(filepath.Join(teamsDir), "workspace"); err != nil {
		return err
	}
	return nil
}

func migrateWorkspaceRoots(root, workspaceLeaf string) error {
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, ent := range entries {
		if !ent.IsDir() {
			continue
		}
		workspaceDir := filepath.Join(root, ent.Name(), workspaceLeaf)
		if err := migrateWorkspaceDeliverablesDir(workspaceDir); err != nil {
			return err
		}
	}
	return nil
}

func migrateWorkspaceDeliverablesDir(workspaceDir string) error {
	legacyDir := filepath.Join(workspaceDir, "deliverables")
	targetDir := filepath.Join(workspaceDir, "tasks")
	if _, err := os.Stat(legacyDir); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if _, err := os.Stat(targetDir); err != nil {
		if os.IsNotExist(err) {
			return os.Rename(legacyDir, targetDir)
		}
		return err
	}
	if err := filepath.WalkDir(legacyDir, func(src string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(legacyDir, src)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		dst := filepath.Join(targetDir, rel)
		if d.IsDir() {
			return os.MkdirAll(dst, 0o755)
		}
		if _, err := os.Stat(dst); err == nil {
			return nil
		} else if !os.IsNotExist(err) {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		return copyFileNoOverwrite(src, dst)
	}); err != nil {
		return err
	}
	return os.RemoveAll(legacyDir)
}

func copyFileNoOverwrite(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		if os.IsExist(err) {
			return nil
		}
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}

func (s *SQLiteTaskStore) dbConn() (*sql.DB, error) {
	if err := s.init(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	db := s.db
	s.mu.Unlock()
	if db == nil {
		return nil, fmt.Errorf("sqlite: db not initialized")
	}
	return db, nil
}

func parseTime(raw string) time.Time {
	return timeutil.ParseRFC3339Nano(raw)
}

func (s *SQLiteTaskStore) CreateTask(ctx context.Context, task types.Task) error {
	if err := validate.NonEmpty("taskId", strings.TrimSpace(task.TaskID)); err != nil {
		return err
	}
	if err := validate.NonEmpty("sessionId", strings.TrimSpace(task.SessionID)); err != nil {
		return err
	}
	if err := validate.NonEmpty("runId", strings.TrimSpace(task.RunID)); err != nil {
		return err
	}
	if err := validate.NonEmpty("goal", strings.TrimSpace(task.Goal)); err != nil {
		return err
	}
	db, err := s.dbConn()
	if err != nil {
		return err
	}

	if task.Inputs == nil {
		task.Inputs = map[string]any{}
	}
	if task.Metadata == nil {
		task.Metadata = map[string]any{}
	}
	task.AssignedToType = strings.TrimSpace(task.AssignedToType)
	task.AssignedTo = strings.TrimSpace(task.AssignedTo)
	if task.AssignedToType == "" {
		if strings.TrimSpace(task.TeamID) != "" {
			if strings.TrimSpace(task.AssignedRole) != "" {
				task.AssignedToType = "role"
				task.AssignedTo = strings.TrimSpace(task.AssignedRole)
			} else {
				task.AssignedToType = "team"
				task.AssignedTo = strings.TrimSpace(task.TeamID)
			}
		} else {
			task.AssignedToType = "agent"
			task.AssignedTo = strings.TrimSpace(task.RunID)
		}
	}
	if task.AssignedTo == "" {
		switch task.AssignedToType {
		case "team":
			task.AssignedTo = strings.TrimSpace(task.TeamID)
		case "role":
			task.AssignedTo = strings.TrimSpace(task.AssignedRole)
		case "agent":
			task.AssignedTo = strings.TrimSpace(task.RunID)
		}
	}

	inputsJSON, _ := json.Marshal(task.Inputs)
	metadataJSON, _ := json.Marshal(task.Metadata)

	now := time.Now().UTC()
	createdAt := now
	if task.CreatedAt != nil && !task.CreatedAt.IsZero() {
		createdAt = task.CreatedAt.UTC()
	}
	status := strings.TrimSpace(string(task.Status))
	if status == "" {
		status = string(types.TaskStatusPending)
	}
	_, err = db.ExecContext(ctx, `
		INSERT INTO tasks (
			task_id, session_id, run_id, team_id, assigned_role, assigned_to_type, assigned_to, claimed_by, role_snapshot, task_kind, created_by,
			goal, inputs_json, priority,
			status, created_at, updated_at, metadata_json
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, strings.TrimSpace(task.TaskID), strings.TrimSpace(task.SessionID), strings.TrimSpace(task.RunID),
		strings.TrimSpace(task.TeamID), strings.TrimSpace(task.AssignedRole), strings.TrimSpace(task.AssignedToType), strings.TrimSpace(task.AssignedTo), strings.TrimSpace(task.ClaimedByAgentID), strings.TrimSpace(task.RoleSnapshot), normalizeTaskKind(task.TaskKind), strings.TrimSpace(task.CreatedBy),
		strings.TrimSpace(task.Goal), string(inputsJSON), task.Priority,
		status, createdAt.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano), string(metadataJSON))
	if err != nil {
		return err
	}
	return nil
}

func (s *SQLiteTaskStore) DeleteTask(ctx context.Context, taskID string) error {
	db, err := s.dbConn()
	if err != nil {
		return err
	}
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return fmt.Errorf("taskID is required")
	}
	_, err = db.ExecContext(ctx, `DELETE FROM tasks WHERE task_id = ?`, taskID)
	return err
}

func (s *SQLiteTaskStore) GetTask(ctx context.Context, taskID string) (types.Task, error) {
	db, err := s.dbConn()
	if err != nil {
		return types.Task{}, err
	}
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return types.Task{}, fmt.Errorf("taskID is required")
	}

	var t types.Task
	var status string
	var inputsJSON string
	var artifactsJSON string
	var metadataJSON string
	var createdRaw string
	var startedRaw string
	var finishedRaw string
	var updatedRaw string
	var leaseRaw string
	err = db.QueryRowContext(ctx, `
		SELECT
			task_id, session_id, run_id, COALESCE(team_id, ''), COALESCE(assigned_role, ''), COALESCE(assigned_to_type, ''), COALESCE(assigned_to, ''), COALESCE(claimed_by, ''), COALESCE(role_snapshot, ''), COALESCE(task_kind, ''), COALESCE(created_by, ''), goal,
			COALESCE(inputs_json, '{}'), priority, status,
			created_at, COALESCE(started_at, ''), COALESCE(finished_at, ''),
			COALESCE(summary, ''), COALESCE(artifacts_json, '[]'),
			COALESCE(error, ''), attempts, COALESCE(lease_until, ''),
			updated_at,
			input_tokens, output_tokens, total_tokens, cost_usd,
			duration_seconds,
			COALESCE(metadata_json, '{}')
		FROM tasks
		WHERE task_id = ?
	`, taskID).Scan(
		&t.TaskID, &t.SessionID, &t.RunID, &t.TeamID, &t.AssignedRole, &t.AssignedToType, &t.AssignedTo, &t.ClaimedByAgentID, &t.RoleSnapshot, &t.TaskKind, &t.CreatedBy, &t.Goal,
		&inputsJSON, &t.Priority, &status,
		&createdRaw, &startedRaw, &finishedRaw,
		&t.Summary, &artifactsJSON,
		&t.Error, &t.Attempts, &leaseRaw,
		&updatedRaw,
		&t.InputTokens, &t.OutputTokens, &t.TotalTokens, &t.CostUSD,
		&t.DurationSeconds,
		&metadataJSON,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return types.Task{}, ErrTaskNotFound
		}
		return types.Task{}, err
	}

	t.Status = types.TaskStatus(strings.TrimSpace(status))
	_ = json.Unmarshal([]byte(inputsJSON), &t.Inputs)
	_ = json.Unmarshal([]byte(artifactsJSON), &t.Artifacts)
	_ = json.Unmarshal([]byte(metadataJSON), &t.Metadata)

	if tt := parseTime(createdRaw); !tt.IsZero() {
		t.CreatedAt = &tt
	}
	if tt := parseTime(startedRaw); !tt.IsZero() {
		t.StartedAt = &tt
	}
	if tt := parseTime(finishedRaw); !tt.IsZero() {
		t.CompletedAt = &tt
	}
	if tt := parseTime(updatedRaw); !tt.IsZero() {
		t.UpdatedAt = &tt
	}
	if tt := parseTime(leaseRaw); !tt.IsZero() {
		t.LeaseUntil = &tt
	}

	return t, nil
}

func allowedSortColumn(sortBy string) (string, bool) {
	switch strings.TrimSpace(sortBy) {
	case "", "created_at":
		return "created_at", true
	case "completed_at", "finished_at":
		return "finished_at", true
	case "cost_usd":
		return "cost_usd", true
	case "priority":
		return "priority", true
	case "updated_at":
		return "updated_at", true
	case "started_at":
		return "started_at", true
	case "status":
		return "status", true
	default:
		return "", false
	}
}

func (s *SQLiteTaskStore) GetRunStats(ctx context.Context, runID string) (RunStats, error) {
	db, err := s.dbConn()
	if err != nil {
		return RunStats{}, err
	}
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return RunStats{}, fmt.Errorf("runID is required")
	}

	var total int
	var succeeded int
	var failed int
	var cost float64
	var tokens int
	var durationSeconds int64

	if err := db.QueryRowContext(ctx, `
		SELECT
			COUNT(*) as total,
			SUM(CASE WHEN status = 'succeeded' THEN 1 ELSE 0 END) as succeeded,
			SUM(CASE WHEN status = 'failed' THEN 1 ELSE 0 END) as failed,
			COALESCE(SUM(cost_usd), 0.0) as cost,
			COALESCE(SUM(total_tokens), 0) as tokens,
			COALESCE(SUM(duration_seconds), 0) as duration
		FROM tasks
		WHERE run_id = ?
	`, runID).Scan(&total, &succeeded, &failed, &cost, &tokens, &durationSeconds); err != nil {
		return RunStats{}, err
	}

	return RunStats{
		TotalTasks:    total,
		Succeeded:     succeeded,
		Failed:        failed,
		TotalCost:     cost,
		TotalTokens:   tokens,
		TotalDuration: time.Duration(durationSeconds) * time.Second,
	}, nil
}

func (s *SQLiteTaskStore) ListTasks(ctx context.Context, filter TaskFilter) ([]types.Task, error) {
	db, err := s.dbConn()
	if err != nil {
		return nil, err
	}
	if filter.Limit < 0 || filter.Offset < 0 {
		return nil, ErrInvalidFilter
	}
	sortBy, ok := allowedSortColumn(filter.SortBy)
	if !ok {
		return nil, ErrInvalidFilter
	}
	sortDir := "ASC"
	if filter.SortDesc {
		sortDir = "DESC"
	}

	q := `
		SELECT
			task_id, session_id, run_id, COALESCE(team_id, ''), COALESCE(assigned_role, ''), COALESCE(assigned_to_type, ''), COALESCE(assigned_to, ''), COALESCE(claimed_by, ''), COALESCE(role_snapshot, ''), COALESCE(task_kind, ''), COALESCE(created_by, ''), goal,
			priority, status, created_at,
			COALESCE(started_at, ''), COALESCE(finished_at, ''),
			COALESCE(summary, ''), COALESCE(error, ''),
			attempts, COALESCE(lease_until, ''),
			input_tokens, output_tokens, total_tokens, cost_usd,
			duration_seconds, updated_at
		FROM tasks
		WHERE 1=1
	`
	args := []any{}

	if strings.TrimSpace(filter.SessionID) != "" {
		q += " AND session_id = ?"
		args = append(args, strings.TrimSpace(filter.SessionID))
	}
	if strings.TrimSpace(filter.RunID) != "" {
		q += " AND run_id = ?"
		args = append(args, strings.TrimSpace(filter.RunID))
	}
	if strings.TrimSpace(filter.TeamID) != "" {
		q += " AND team_id = ?"
		args = append(args, strings.TrimSpace(filter.TeamID))
	}
	if strings.TrimSpace(filter.AssignedRole) != "" {
		q += " AND assigned_role = ?"
		args = append(args, strings.TrimSpace(filter.AssignedRole))
	}
	if strings.TrimSpace(filter.AssignedToType) != "" {
		q += " AND assigned_to_type = ?"
		args = append(args, strings.TrimSpace(filter.AssignedToType))
	}
	if strings.TrimSpace(filter.AssignedTo) != "" {
		q += " AND assigned_to = ?"
		args = append(args, strings.TrimSpace(filter.AssignedTo))
	}
	if strings.TrimSpace(filter.ClaimedBy) != "" {
		q += " AND claimed_by = ?"
		args = append(args, strings.TrimSpace(filter.ClaimedBy))
	}
	if strings.TrimSpace(filter.TaskKind) != "" {
		q += " AND task_kind = ?"
		args = append(args, normalizeTaskKind(filter.TaskKind))
	}
	if filter.UnassignedOnly {
		q += " AND COALESCE(assigned_role, '') = ''"
	}
	switch strings.ToLower(strings.TrimSpace(filter.View)) {
	case "inbox":
		q += " AND status = ?"
		args = append(args, string(types.TaskStatusPending))
	case "outbox":
		q += " AND status IN (?, ?, ?)"
		args = append(args, string(types.TaskStatusSucceeded), string(types.TaskStatusFailed), string(types.TaskStatusCanceled))
	}
	if len(filter.Status) != 0 {
		ph := make([]string, 0, len(filter.Status))
		for _, st := range filter.Status {
			ph = append(ph, "?")
			args = append(args, strings.TrimSpace(string(st)))
		}
		q += " AND status IN (" + strings.Join(ph, ",") + ")"
	}
	if filter.FromDate != nil && !filter.FromDate.IsZero() {
		q += " AND created_at >= ?"
		args = append(args, filter.FromDate.UTC().Format(time.RFC3339Nano))
	}
	if filter.ToDate != nil && !filter.ToDate.IsZero() {
		q += " AND created_at <= ?"
		args = append(args, filter.ToDate.UTC().Format(time.RFC3339Nano))
	}

	orderBy := fmt.Sprintf("%s %s", sortBy, sortDir)
	if sortBy == "priority" {
		orderBy = fmt.Sprintf("priority %s, created_at ASC", sortDir)
	}
	q += " ORDER BY " + orderBy

	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	q += " LIMIT ?"
	args = append(args, limit)
	if filter.Offset > 0 {
		q += " OFFSET ?"
		args = append(args, filter.Offset)
	}

	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []types.Task{}
	for rows.Next() {
		var t types.Task
		var status string
		var createdRaw string
		var startedRaw string
		var finishedRaw string
		var leaseRaw string
		var updatedRaw string
		if err := rows.Scan(
			&t.TaskID, &t.SessionID, &t.RunID, &t.TeamID, &t.AssignedRole, &t.AssignedToType, &t.AssignedTo, &t.ClaimedByAgentID, &t.RoleSnapshot, &t.TaskKind, &t.CreatedBy, &t.Goal,
			&t.Priority, &status, &createdRaw,
			&startedRaw, &finishedRaw,
			&t.Summary, &t.Error,
			&t.Attempts, &leaseRaw,
			&t.InputTokens, &t.OutputTokens, &t.TotalTokens, &t.CostUSD,
			&t.DurationSeconds, &updatedRaw,
		); err != nil {
			return nil, err
		}
		t.Status = types.TaskStatus(strings.TrimSpace(status))
		if tt := parseTime(createdRaw); !tt.IsZero() {
			t.CreatedAt = &tt
		}
		if tt := parseTime(startedRaw); !tt.IsZero() {
			t.StartedAt = &tt
		}
		if tt := parseTime(finishedRaw); !tt.IsZero() {
			t.CompletedAt = &tt
		}
		if tt := parseTime(updatedRaw); !tt.IsZero() {
			t.UpdatedAt = &tt
		}
		if tt := parseTime(leaseRaw); !tt.IsZero() {
			t.LeaseUntil = &tt
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *SQLiteTaskStore) CountTasks(ctx context.Context, filter TaskFilter) (int, error) {
	db, err := s.dbConn()
	if err != nil {
		return 0, err
	}
	if filter.Offset < 0 || filter.Limit < 0 {
		return 0, ErrInvalidFilter
	}
	q := `SELECT COUNT(*) FROM tasks WHERE 1=1`
	args := []any{}
	if strings.TrimSpace(filter.SessionID) != "" {
		q += " AND session_id = ?"
		args = append(args, strings.TrimSpace(filter.SessionID))
	}
	if strings.TrimSpace(filter.RunID) != "" {
		q += " AND run_id = ?"
		args = append(args, strings.TrimSpace(filter.RunID))
	}
	if strings.TrimSpace(filter.TeamID) != "" {
		q += " AND team_id = ?"
		args = append(args, strings.TrimSpace(filter.TeamID))
	}
	if strings.TrimSpace(filter.AssignedRole) != "" {
		q += " AND assigned_role = ?"
		args = append(args, strings.TrimSpace(filter.AssignedRole))
	}
	if strings.TrimSpace(filter.AssignedToType) != "" {
		q += " AND assigned_to_type = ?"
		args = append(args, strings.TrimSpace(filter.AssignedToType))
	}
	if strings.TrimSpace(filter.AssignedTo) != "" {
		q += " AND assigned_to = ?"
		args = append(args, strings.TrimSpace(filter.AssignedTo))
	}
	if strings.TrimSpace(filter.ClaimedBy) != "" {
		q += " AND claimed_by = ?"
		args = append(args, strings.TrimSpace(filter.ClaimedBy))
	}
	if strings.TrimSpace(filter.TaskKind) != "" {
		q += " AND task_kind = ?"
		args = append(args, normalizeTaskKind(filter.TaskKind))
	}
	if filter.UnassignedOnly {
		q += " AND COALESCE(assigned_role, '') = ''"
	}
	switch strings.ToLower(strings.TrimSpace(filter.View)) {
	case "inbox":
		q += " AND status = ?"
		args = append(args, string(types.TaskStatusPending))
	case "outbox":
		q += " AND status IN (?, ?, ?)"
		args = append(args, string(types.TaskStatusSucceeded), string(types.TaskStatusFailed), string(types.TaskStatusCanceled))
	}
	if len(filter.Status) != 0 {
		ph := make([]string, 0, len(filter.Status))
		for _, st := range filter.Status {
			ph = append(ph, "?")
			args = append(args, strings.TrimSpace(string(st)))
		}
		q += " AND status IN (" + strings.Join(ph, ",") + ")"
	}
	if filter.FromDate != nil && !filter.FromDate.IsZero() {
		q += " AND created_at >= ?"
		args = append(args, filter.FromDate.UTC().Format(time.RFC3339Nano))
	}
	if filter.ToDate != nil && !filter.ToDate.IsZero() {
		q += " AND created_at <= ?"
		args = append(args, filter.ToDate.UTC().Format(time.RFC3339Nano))
	}
	var n int
	if err := db.QueryRowContext(ctx, q, args...).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

func (s *SQLiteTaskStore) UpdateTask(ctx context.Context, task types.Task) error {
	taskID := strings.TrimSpace(task.TaskID)
	if taskID == "" {
		return fmt.Errorf("taskID is required")
	}
	if strings.TrimSpace(task.SessionID) == "" || strings.TrimSpace(task.RunID) == "" {
		return fmt.Errorf("sessionId and runId are required")
	}
	db, err := s.dbConn()
	if err != nil {
		return err
	}
	if task.Inputs == nil {
		task.Inputs = map[string]any{}
	}
	if task.Metadata == nil {
		task.Metadata = map[string]any{}
	}
	if task.Artifacts == nil {
		task.Artifacts = []string{}
	}
	inputsJSON, _ := json.Marshal(task.Inputs)
	metadataJSON, _ := json.Marshal(task.Metadata)
	artifactsJSON, _ := json.Marshal(task.Artifacts)

	startedAt := ""
	if timeutil.IsSet(task.StartedAt) {
		startedAt = timeutil.FormatRFC3339Nano(task.StartedAt)
	}
	finishedAt := ""
	if timeutil.IsSet(task.CompletedAt) {
		finishedAt = timeutil.FormatRFC3339Nano(task.CompletedAt)
	}
	leaseUntil := ""
	if task.LeaseUntil != nil && !task.LeaseUntil.IsZero() {
		leaseUntil = task.LeaseUntil.UTC().Format(time.RFC3339Nano)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = db.ExecContext(ctx, `
		UPDATE tasks
		SET session_id = ?, run_id = ?, team_id = ?, assigned_role = ?, assigned_to_type = ?, assigned_to = ?, claimed_by = ?, role_snapshot = ?, task_kind = ?, created_by = ?,
		    goal = ?, inputs_json = ?, priority = ?,
		    status = ?, started_at = ?, finished_at = ?, completed_at = ?, summary = ?, artifacts_json = ?,
		    error = ?, attempts = ?, lease_until = ?, updated_at = ?,
		    input_tokens = ?, output_tokens = ?, total_tokens = ?, cost_usd = ?, duration_seconds = ?,
		    metadata_json = ?
		WHERE task_id = ?
	`, strings.TrimSpace(task.SessionID), strings.TrimSpace(task.RunID), strings.TrimSpace(task.TeamID), strings.TrimSpace(task.AssignedRole), strings.TrimSpace(task.AssignedToType), strings.TrimSpace(task.AssignedTo), strings.TrimSpace(task.ClaimedByAgentID), strings.TrimSpace(task.RoleSnapshot), normalizeTaskKind(task.TaskKind), strings.TrimSpace(task.CreatedBy),
		strings.TrimSpace(task.Goal), string(inputsJSON), task.Priority,
		strings.TrimSpace(string(task.Status)),
		nullIfEmpty(startedAt),
		nullIfEmpty(finishedAt),
		nullIfEmpty(finishedAt),
		strings.TrimSpace(task.Summary),
		string(artifactsJSON),
		strings.TrimSpace(task.Error),
		task.Attempts,
		nullIfEmpty(leaseUntil),
		now,
		task.InputTokens, task.OutputTokens, task.TotalTokens, task.CostUSD, task.DurationSeconds,
		string(metadataJSON),
		taskID,
	)
	return err
}

func nullIfEmpty(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}

func isTerminalStatus(status types.TaskStatus) bool {
	switch status {
	case types.TaskStatusSucceeded, types.TaskStatusFailed, types.TaskStatusCanceled:
		return true
	default:
		return false
	}
}

func (s *SQLiteTaskStore) ClaimTask(ctx context.Context, taskID string, ttl time.Duration) error {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return fmt.Errorf("taskID is required")
	}
	if ttl <= 0 {
		ttl = 2 * time.Minute
	}
	db, err := s.dbConn()
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	leaseUntil := now.Add(ttl)

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var statusRaw string
	var leaseRaw string
	var attempts int
	row := tx.QueryRowContext(ctx, `
		SELECT status, COALESCE(lease_until, ''), attempts
		FROM tasks
		WHERE task_id = ?
	`, taskID)
	if err := row.Scan(&statusRaw, &leaseRaw, &attempts); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrTaskNotFound
		}
		return err
	}

	status := types.TaskStatus(strings.TrimSpace(statusRaw))
	if isTerminalStatus(status) {
		return ErrTaskTerminal
	}
	lease := parseTime(leaseRaw)
	if status == types.TaskStatusActive && !lease.IsZero() && lease.After(now) {
		return ErrTaskClaimed
	}

	attempts++
	_, err = tx.ExecContext(ctx, `
		UPDATE tasks
		SET status = ?, attempts = ?, lease_until = ?, updated_at = ?,
		    started_at = COALESCE(started_at, ?)
		WHERE task_id = ?
	`, string(types.TaskStatusActive), attempts, leaseUntil.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano),
		now.Format(time.RFC3339Nano), taskID)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (s *SQLiteTaskStore) ExtendLease(ctx context.Context, taskID string, ttl time.Duration) error {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return fmt.Errorf("taskID is required")
	}
	if ttl <= 0 {
		ttl = 2 * time.Minute
	}
	db, err := s.dbConn()
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	leaseUntil := now.Add(ttl)
	_, err = db.ExecContext(ctx, `
		UPDATE tasks
		SET lease_until = ?, updated_at = ?
		WHERE task_id = ? AND status = ?
	`, leaseUntil.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano), taskID, string(types.TaskStatusActive))
	return err
}

func (s *SQLiteTaskStore) ReleaseLease(ctx context.Context, taskID string) error {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return fmt.Errorf("taskID is required")
	}
	db, err := s.dbConn()
	if err != nil {
		return err
	}
	// Extra safety: do not release lease on terminal tasks (invariant: terminal never → pending).
	var statusRaw string
	if err := db.QueryRowContext(ctx, `SELECT status FROM tasks WHERE task_id = ?`, taskID).Scan(&statusRaw); err == nil {
		if isTerminalStatus(types.TaskStatus(strings.TrimSpace(statusRaw))) {
			return nil // Idempotent: already terminal.
		}
	}
	// Fall through: update only active (WHERE status = ? already enforces non-terminal).
	now := time.Now().UTC()
	res, err := db.ExecContext(ctx, `
		UPDATE tasks
		SET status = ?, lease_until = NULL, claimed_by = '', updated_at = ?
		WHERE task_id = ? AND status = ?
	`, string(types.TaskStatusPending), now.Format(time.RFC3339Nano), taskID, string(types.TaskStatusActive))
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return nil // Task not active or not found; idempotent
	}
	return nil
}

func (s *SQLiteTaskStore) DelegateTask(ctx context.Context, taskID string) error {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return fmt.Errorf("taskID is required")
	}
	db, err := s.dbConn()
	if err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	res, err := db.ExecContext(ctx, `
		UPDATE tasks
		SET status = ?, lease_until = NULL, updated_at = ?
		WHERE task_id = ? AND status = ?
	`, string(types.TaskStatusDelegated), now, taskID, string(types.TaskStatusActive))
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("delegate: task %s is not in active state", taskID)
	}
	return nil
}

func (s *SQLiteTaskStore) ResumeTask(ctx context.Context, taskID string) error {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return fmt.Errorf("taskID is required")
	}
	db, err := s.dbConn()
	if err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	res, err := db.ExecContext(ctx, `
		UPDATE tasks
		SET status = ?, lease_until = NULL, claimed_by = '', updated_at = ?
		WHERE task_id = ? AND status = ?
	`, string(types.TaskStatusPending), now, taskID, string(types.TaskStatusDelegated))
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		// Idempotent: task already resumed or not in delegated state; treat as success.
		return nil
	}
	return nil
}

func (s *SQLiteTaskStore) CompleteTask(ctx context.Context, taskID string, result types.TaskResult) error {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return fmt.Errorf("taskID is required")
	}
	db, err := s.dbConn()
	if err != nil {
		return err
	}
	status := types.TaskStatus(strings.TrimSpace(string(result.Status)))
	if !isTerminalStatus(status) {
		// Fall back to failed if provided status isn't terminal.
		status = types.TaskStatusFailed
	}
	now := time.Now().UTC()
	finishedAt := now
	if timeutil.IsSet(result.CompletedAt) {
		finishedAt = result.CompletedAt.UTC()
	}
	artifactsJSON, _ := json.Marshal(result.Artifacts)

	total := result.TotalTokens
	if total == 0 {
		total = result.InputTokens + result.OutputTokens
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Only transition from non-terminal states; never overwrite succeeded/failed/canceled.
	res, err := tx.ExecContext(ctx, `
		UPDATE tasks
		SET status = ?, finished_at = ?, completed_at = ?, summary = ?, artifacts_json = ?, error = ?,
		    input_tokens = ?, output_tokens = ?, total_tokens = ?, cost_usd = ?,
		    lease_until = NULL, updated_at = ?
		WHERE task_id = ? AND status IN (?, ?, ?)
`, string(status), finishedAt.Format(time.RFC3339Nano), finishedAt.Format(time.RFC3339Nano),
		strings.TrimSpace(result.Summary), string(artifactsJSON), strings.TrimSpace(result.Error),
		result.InputTokens, result.OutputTokens, total, result.CostUSD,
		now.Format(time.RFC3339Nano), taskID,
		string(types.TaskStatusActive), string(types.TaskStatusPending), string(types.TaskStatusDelegated))
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		// Task already terminal or not found; idempotent success.
		return nil
	}

	var task types.Task
	var finishedRaw string
	var metadataRaw string
	if err := tx.QueryRowContext(ctx, `
		SELECT task_id, COALESCE(team_id, ''), COALESCE(run_id, ''), COALESCE(assigned_role, ''),
		       COALESCE(role_snapshot, ''), COALESCE(task_kind, ''), COALESCE(created_by, ''), COALESCE(goal, ''), COALESCE(status, ''), COALESCE(finished_at, ''),
		       COALESCE(metadata_json, '{}')
		FROM tasks
		WHERE task_id = ?
	`, taskID).Scan(
		&task.TaskID, &task.TeamID, &task.RunID, &task.AssignedRole, &task.RoleSnapshot, &task.TaskKind, &task.CreatedBy, &task.Goal, &task.Status, &finishedRaw, &metadataRaw,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrTaskNotFound
		}
		return err
	}
	if strings.TrimSpace(metadataRaw) != "" {
		_ = json.Unmarshal([]byte(metadataRaw), &task.Metadata)
	}
	if tt := parseTime(finishedRaw); !tt.IsZero() {
		task.CompletedAt = &tt
	}
	if err := s.upsertArtifactsTx(ctx, tx, task, result); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *SQLiteTaskStore) RecoverExpiredLeases(ctx context.Context) error {
	db, err := s.dbConn()
	if err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = db.ExecContext(ctx, `
		UPDATE tasks
		SET status = ?,
		    error = CASE
		      WHEN COALESCE(TRIM(error), '') = '' THEN 'lease expired'
		      ELSE TRIM(error) || '; lease expired'
		    END,
		    lease_until = NULL,
		    updated_at = ?
		WHERE status = ?
		  AND COALESCE(TRIM(lease_until), '') != ''
		  AND lease_until < ?
	`, string(types.TaskStatusFailed), now, string(types.TaskStatusActive), now)
	return err
}

// CancelActiveTasksByRun marks all currently-active tasks for a run as canceled.
// This is used when a run is paused/stopped so in-flight tasks do not remain active.
func (s *SQLiteTaskStore) CancelActiveTasksByRun(ctx context.Context, runID string, reason string) (int, error) {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return 0, fmt.Errorf("runID is required")
	}
	db, err := s.dbConn()
	if err != nil {
		return 0, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "run paused"
	}
	res, err := db.ExecContext(ctx, `
		UPDATE tasks
		SET status = ?,
		    finished_at = ?,
		    completed_at = ?,
		    error = CASE
		      WHEN COALESCE(TRIM(error), '') = '' THEN ?
		      ELSE TRIM(error) || '; ' || ?
		    END,
		    lease_until = NULL,
		    updated_at = ?
		WHERE run_id = ?
		  AND status = ?
	`, string(types.TaskStatusCanceled), now, now, reason, reason, now, runID, string(types.TaskStatusActive))
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, nil
	}
	return int(n), nil
}
