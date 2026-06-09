package infra

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/tinoosan/agen8/internal/core/types"
	"github.com/tinoosan/agen8/internal/services/project/domain/member"
	"github.com/tinoosan/agen8/internal/services/task/domain"
	storagedb "github.com/tinoosan/agen8/internal/storage/db"
)

type SQLiteRepository struct {
	db *sql.DB
}

func NewSQLiteRepository(handle *storagedb.Handle) (*SQLiteRepository, error) {
	if handle == nil || handle.DB() == nil {
		return nil, fmt.Errorf("task sqlite repository: db handle is required")
	}
	if handle.Driver() != storagedb.DriverSQLite {
		return nil, fmt.Errorf("task sqlite repository: storage driver must be sqlite, got %q", handle.Driver())
	}
	repo := &SQLiteRepository{db: handle.DB()}
	if err := repo.ensureSchema(context.Background()); err != nil {
		return nil, err
	}
	return repo, nil
}

func (r *SQLiteRepository) CreateTask(ctx context.Context, task domain.Task) error {
	return r.saveTask(ctx, task)
}

func (r *SQLiteRepository) UpdateTask(ctx context.Context, task domain.Task) error {
	if strings.TrimSpace(string(task.ID)) == "" {
		return fmt.Errorf("task id is required")
	}
	if _, err := r.GetTask(ctx, task.ID); err != nil {
		return err
	}
	return r.saveTask(ctx, task)
}

func (r *SQLiteRepository) GetTask(ctx context.Context, taskID domain.TaskID) (domain.Task, error) {
	taskID = domain.TaskID(strings.TrimSpace(string(taskID)))
	if taskID == "" {
		return domain.Task{}, fmt.Errorf("task id is required")
	}
	var raw []byte
	err := r.db.QueryRowContext(ctx, `
		SELECT task_json
		FROM tasks
		WHERE task_id = ?
	`, string(taskID)).Scan(&raw)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Task{}, domain.ErrTaskNotFound
		}
		return domain.Task{}, fmt.Errorf("get task %s: %w", taskID, err)
	}
	task, err := unmarshalTask(raw)
	if err != nil {
		return domain.Task{}, fmt.Errorf("unmarshal task %s: %w", taskID, err)
	}
	return task, nil
}

func (r *SQLiteRepository) ListTasks(ctx context.Context, filter domain.TaskFilter) ([]domain.Task, error) {
	where, args, err := taskWhere(filter)
	if err != nil {
		return nil, err
	}
	query := "SELECT task_json FROM tasks" + where + taskOrderBy(filter)
	if filter.Limit > 0 {
		query += " LIMIT ?"
		args = append(args, filter.Limit)
	}
	if filter.Offset > 0 {
		query += " OFFSET ?"
		args = append(args, filter.Offset)
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}
	defer rows.Close()

	var tasks []domain.Task
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, fmt.Errorf("scan task: %w", err)
		}
		task, err := unmarshalTask(raw)
		if err != nil {
			return nil, fmt.Errorf("unmarshal task: %w", err)
		}
		tasks = append(tasks, task)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate tasks: %w", err)
	}
	return tasks, nil
}

func (r *SQLiteRepository) CountTasks(ctx context.Context, filter domain.TaskFilter) (int, error) {
	where, args, err := taskWhere(filter)
	if err != nil {
		return 0, err
	}
	var count int
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM tasks"+where, args...).Scan(&count); err != nil {
		return 0, fmt.Errorf("count tasks: %w", err)
	}
	return count, nil
}

func (r *SQLiteRepository) saveTask(ctx context.Context, task domain.Task) error {
	task.ID = domain.TaskID(strings.TrimSpace(string(task.ID)))
	if task.ID == "" {
		return fmt.Errorf("task id is required")
	}
	task.ProjectID = types.ProjectID(strings.TrimSpace(string(task.ProjectID)))
	if task.ProjectID == "" {
		return fmt.Errorf("task project id is required")
	}
	task.AssignedTo = member.ID(strings.TrimSpace(string(task.AssignedTo)))
	task.ClaimedByMemberID = member.ID(strings.TrimSpace(string(task.ClaimedByMemberID)))
	task.TaskKind = strings.TrimSpace(task.TaskKind)
	task.Status = domain.TaskStatus(strings.TrimSpace(string(task.Status)))
	if task.Status == "" {
		return fmt.Errorf("task status is required")
	}
	payload, err := json.Marshal(task)
	if err != nil {
		return fmt.Errorf("marshal task: %w", err)
	}
	_, err = r.db.ExecContext(ctx, `
		INSERT INTO tasks (
			task_id, project_id, assigned_to, claimed_by_member_id, task_kind, status,
			key_result_ref, created_at, updated_at,
			completed_at, task_json
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(task_id) DO UPDATE SET
			project_id = excluded.project_id,
			assigned_to = excluded.assigned_to,
			claimed_by_member_id = excluded.claimed_by_member_id,
			task_kind = excluded.task_kind,
			status = excluded.status,
			key_result_ref = excluded.key_result_ref,
			created_at = excluded.created_at,
			updated_at = excluded.updated_at,
			completed_at = excluded.completed_at,
			task_json = excluded.task_json
	`,
		string(task.ID),
		string(task.ProjectID),
		string(task.AssignedTo),
		string(task.ClaimedByMemberID),
		task.TaskKind,
		string(task.Status),
		strings.TrimSpace(task.KeyResultRef),
		timeString(task.CreatedAt),
		timeString(task.UpdatedAt),
		timeString(task.CompletedAt),
		string(payload),
	)
	if err != nil {
		return fmt.Errorf("save task %s: %w", task.ID, err)
	}
	return nil
}

func (r *SQLiteRepository) ensureSchema(ctx context.Context) error {
	if _, err := r.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS tasks (
			task_id TEXT PRIMARY KEY,
			project_id TEXT NOT NULL DEFAULT '',
			assigned_to TEXT NOT NULL DEFAULT '',
			claimed_by_member_id TEXT NOT NULL DEFAULT '',
			task_kind TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT '',
			key_result_ref TEXT NOT NULL DEFAULT '',
			created_at TEXT,
			updated_at TEXT,
			completed_at TEXT,
			task_json TEXT NOT NULL
		)
	`); err != nil {
		return fmt.Errorf("ensure tasks table: %w", err)
	}
	for _, stmt := range []string{
		`ALTER TABLE tasks ADD COLUMN assigned_to TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE tasks ADD COLUMN claimed_by_member_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE tasks ADD COLUMN task_kind TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE tasks ADD COLUMN key_result_ref TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE tasks ADD COLUMN task_json TEXT`,
	} {
		if _, err := r.db.ExecContext(ctx, stmt); err != nil && !isDuplicateColumnError(err) {
			return fmt.Errorf("ensure tasks column: %w", err)
		}
	}
	for _, stmt := range []string{
		`CREATE INDEX IF NOT EXISTS idx_tasks_project ON tasks(project_id)`,
		`CREATE INDEX IF NOT EXISTS idx_tasks_assigned_to ON tasks(assigned_to)`,
		`CREATE INDEX IF NOT EXISTS idx_tasks_claimed_by_member_id ON tasks(claimed_by_member_id)`,
		`CREATE INDEX IF NOT EXISTS idx_tasks_status ON tasks(status)`,
		`CREATE INDEX IF NOT EXISTS idx_tasks_kind ON tasks(task_kind)`,
		`CREATE INDEX IF NOT EXISTS idx_tasks_created_at ON tasks(created_at)`,
	} {
		if _, err := r.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("ensure tasks index: %w", err)
		}
	}
	return nil
}
