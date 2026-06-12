package infra

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/tinoosan/agen8/internal/services/question/domain"
	storagedb "github.com/tinoosan/agen8/internal/storage/db"
)

type SQLiteRepository struct {
	db *sql.DB
}

func NewSQLiteRepository(handle *storagedb.Handle) (*SQLiteRepository, error) {
	if handle == nil || handle.DB() == nil {
		return nil, fmt.Errorf("question sqlite repository: db handle is required")
	}
	if handle.Driver() != storagedb.DriverSQLite {
		return nil, fmt.Errorf("question sqlite repository: storage driver must be sqlite, got %q", handle.Driver())
	}
	repo := &SQLiteRepository{db: handle.DB()}
	if err := repo.ensureSchema(context.Background()); err != nil {
		return nil, err
	}
	return repo, nil
}

func (r *SQLiteRepository) CreateQuestion(ctx context.Context, question domain.Question) error {
	return r.saveQuestion(ctx, question)
}

func (r *SQLiteRepository) UpdateQuestion(ctx context.Context, question domain.Question) error {
	if strings.TrimSpace(string(question.ID)) == "" {
		return fmt.Errorf("question id is required")
	}
	if _, err := r.GetQuestion(ctx, question.ID); err != nil {
		return err
	}
	return r.saveQuestion(ctx, question)
}

func (r *SQLiteRepository) GetQuestion(ctx context.Context, id domain.QuestionID) (domain.Question, error) {
	id = domain.QuestionID(strings.TrimSpace(string(id)))
	if id == "" {
		return domain.Question{}, fmt.Errorf("question id is required")
	}
	var raw []byte
	err := r.db.QueryRowContext(ctx, `
		SELECT question_json
		FROM questions
		WHERE question_id = ?
	`, string(id)).Scan(&raw)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Question{}, domain.ErrQuestionNotFound
		}
		return domain.Question{}, fmt.Errorf("get question %s: %w", id, err)
	}
	question, err := unmarshalQuestion(raw)
	if err != nil {
		return domain.Question{}, fmt.Errorf("unmarshal question %s: %w", id, err)
	}
	return question, nil
}

func (r *SQLiteRepository) saveQuestion(ctx context.Context, question domain.Question) error {
	question.ID = domain.QuestionID(strings.TrimSpace(string(question.ID)))
	question.ProjectID = strings.TrimSpace(question.ProjectID)
	question.AskedByMemberID = strings.TrimSpace(question.AskedByMemberID)
	question.Text = strings.TrimSpace(question.Text)
	question.TaskRef = strings.TrimSpace(question.TaskRef)
	question.KeyResultRef = strings.TrimSpace(question.KeyResultRef)
	question.MissionRef = strings.TrimSpace(question.MissionRef)
	question.DecisionID = strings.TrimSpace(question.DecisionID)
	if err := question.Validate(); err != nil {
		return err
	}
	payload, err := json.Marshal(question)
	if err != nil {
		return fmt.Errorf("marshal question: %w", err)
	}
	_, err = r.db.ExecContext(ctx, `
		INSERT INTO questions (
			question_id, project_id, asked_by_member_id, answer_kind, status,
			as_decision, task_ref, key_result_ref, mission_ref, decision_id,
			created_at, updated_at, answered_at, expired_at, cancelled_at,
			question_json
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(question_id) DO UPDATE SET
			project_id = excluded.project_id,
			asked_by_member_id = excluded.asked_by_member_id,
			answer_kind = excluded.answer_kind,
			status = excluded.status,
			as_decision = excluded.as_decision,
			task_ref = excluded.task_ref,
			key_result_ref = excluded.key_result_ref,
			mission_ref = excluded.mission_ref,
			decision_id = excluded.decision_id,
			created_at = excluded.created_at,
			updated_at = excluded.updated_at,
			answered_at = excluded.answered_at,
			expired_at = excluded.expired_at,
			cancelled_at = excluded.cancelled_at,
			question_json = excluded.question_json
	`,
		string(question.ID),
		question.ProjectID,
		question.AskedByMemberID,
		string(question.AnswerKind),
		string(question.Status),
		boolInt(question.AsDecision),
		question.TaskRef,
		question.KeyResultRef,
		question.MissionRef,
		question.DecisionID,
		timeString(question.CreatedAt),
		timeString(question.UpdatedAt),
		timeString(question.AnsweredAt),
		timeString(question.ExpiredAt),
		timeString(question.CancelledAt),
		string(payload),
	)
	if err != nil {
		return fmt.Errorf("save question %s: %w", question.ID, err)
	}
	return nil
}

func (r *SQLiteRepository) ensureSchema(ctx context.Context) error {
	if _, err := r.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS questions (
			question_id TEXT PRIMARY KEY,
			project_id TEXT NOT NULL DEFAULT '',
			asked_by_member_id TEXT NOT NULL DEFAULT '',
			answer_kind TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT '',
			as_decision INTEGER NOT NULL DEFAULT 0,
			task_ref TEXT NOT NULL DEFAULT '',
			key_result_ref TEXT NOT NULL DEFAULT '',
			mission_ref TEXT NOT NULL DEFAULT '',
			decision_id TEXT NOT NULL DEFAULT '',
			created_at TEXT,
			updated_at TEXT,
			answered_at TEXT,
			expired_at TEXT,
			cancelled_at TEXT,
			question_json TEXT NOT NULL
		)
	`); err != nil {
		return fmt.Errorf("ensure questions table: %w", err)
	}
	for _, column := range []struct {
		name       string
		definition string
	}{
		{"project_id", "TEXT NOT NULL DEFAULT ''"},
		{"asked_by_member_id", "TEXT NOT NULL DEFAULT ''"},
		{"answer_kind", "TEXT NOT NULL DEFAULT ''"},
		{"status", "TEXT NOT NULL DEFAULT ''"},
		{"as_decision", "INTEGER NOT NULL DEFAULT 0"},
		{"task_ref", "TEXT NOT NULL DEFAULT ''"},
		{"key_result_ref", "TEXT NOT NULL DEFAULT ''"},
		{"mission_ref", "TEXT NOT NULL DEFAULT ''"},
		{"decision_id", "TEXT NOT NULL DEFAULT ''"},
		{"created_at", "TEXT"},
		{"updated_at", "TEXT"},
		{"answered_at", "TEXT"},
		{"expired_at", "TEXT"},
		{"cancelled_at", "TEXT"},
		{"question_json", "TEXT"},
	} {
		if err := r.ensureColumn(ctx, "questions", column.name, column.definition); err != nil {
			return err
		}
	}
	for _, stmt := range []string{
		`CREATE INDEX IF NOT EXISTS idx_questions_project ON questions(project_id)`,
		`CREATE INDEX IF NOT EXISTS idx_questions_status ON questions(status)`,
		`CREATE INDEX IF NOT EXISTS idx_questions_task_ref ON questions(task_ref)`,
		`CREATE INDEX IF NOT EXISTS idx_questions_mission_ref ON questions(mission_ref)`,
		`CREATE INDEX IF NOT EXISTS idx_questions_created_at ON questions(created_at)`,
	} {
		if _, err := r.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("ensure questions index: %w", err)
		}
	}
	return nil
}

func (r *SQLiteRepository) ensureColumn(ctx context.Context, table, name, definition string) error {
	if _, err := r.db.ExecContext(ctx, fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, name, definition)); err != nil {
		if isDuplicateColumnError(err) {
			return nil
		}
		return fmt.Errorf("ensure %s.%s column: %w", table, name, err)
	}
	return nil
}

func unmarshalQuestion(raw []byte) (domain.Question, error) {
	var question domain.Question
	if len(raw) == 0 {
		return domain.Question{}, fmt.Errorf("question json is empty")
	}
	if err := json.Unmarshal(raw, &question); err != nil {
		return domain.Question{}, err
	}
	return question, nil
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func timeString(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func isDuplicateColumnError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "duplicate column") || strings.Contains(msg, "already exists")
}
