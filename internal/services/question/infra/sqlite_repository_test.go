package infra

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/tinoosan/agen8/internal/services/question/domain"
	storagedb "github.com/tinoosan/agen8/internal/storage/db"
)

func TestSQLiteRepositoryRoundTrip(t *testing.T) {
	repo := newSQLiteQuestionRepoForTest(t)
	ctx := context.Background()
	now := time.Date(2026, 6, 12, 9, 0, 0, 0, time.UTC)
	question, err := domain.NewQuestion(domain.NewQuestionInput{
		ID:              "question-1",
		ProjectID:       "project-1",
		AskedByMemberID: "member-agent",
		Text:            "Which options?",
		AnswerKind:      domain.AnswerKindMultiSelect,
		Options:         []string{"alpha", "beta"},
		AsDecision:      true,
		TaskRef:         "task-1",
	}, now)
	if err != nil {
		t.Fatalf("NewQuestion: %v", err)
	}
	if err := repo.CreateQuestion(ctx, question); err != nil {
		t.Fatalf("CreateQuestion: %v", err)
	}

	answered, err := question.AnswerWith(domain.AnswerPayload{
		SelectedOptions:    []string{"alpha", "beta"},
		AnsweredByMemberID: "member-human",
	}, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("AnswerWith: %v", err)
	}
	answered, err = answered.WithDecisionID("dec-1", now.Add(2*time.Minute))
	if err != nil {
		t.Fatalf("WithDecisionID: %v", err)
	}
	if err := repo.UpdateQuestion(ctx, answered); err != nil {
		t.Fatalf("UpdateQuestion: %v", err)
	}

	got, err := repo.GetQuestion(ctx, "question-1")
	if err != nil {
		t.Fatalf("GetQuestion: %v", err)
	}
	if got.Status != domain.StatusAnswered || got.DecisionID != "dec-1" || len(got.Answer.SelectedOptions) != 2 {
		t.Fatalf("got question = %#v", got)
	}
}

func TestSQLiteRepositoryEnsureSchemaIsIdempotent(t *testing.T) {
	repo := newSQLiteQuestionRepoForTest(t)
	ctx := context.Background()
	if err := repo.ensureSchema(ctx); err != nil {
		t.Fatalf("ensureSchema first: %v", err)
	}
	if err := repo.ensureSchema(ctx); err != nil {
		t.Fatalf("ensureSchema second: %v", err)
	}
}

func TestSQLiteRepositoryEnsureSchemaAddsMissingColumns(t *testing.T) {
	handle := newRawSQLiteHandleForTest(t)
	db := handle.DB()
	if _, err := db.Exec(`CREATE TABLE questions (question_id TEXT PRIMARY KEY, question_json TEXT)`); err != nil {
		t.Fatalf("create minimal questions table: %v", err)
	}
	if _, err := NewSQLiteRepository(handle); err != nil {
		t.Fatalf("NewSQLiteRepository: %v", err)
	}
	if !hasColumn(t, db, "questions", "decision_id") {
		t.Fatal("decision_id column was not added")
	}
	if !hasColumn(t, db, "questions", "asked_by_member_id") {
		t.Fatal("asked_by_member_id column was not added")
	}
}

func newSQLiteQuestionRepoForTest(t *testing.T) *SQLiteRepository {
	t.Helper()
	handle := newRawSQLiteHandleForTest(t)
	repo, err := NewSQLiteRepository(handle)
	if err != nil {
		t.Fatalf("NewSQLiteRepository: %v", err)
	}
	return repo
}

func newRawSQLiteHandleForTest(t *testing.T) *storagedb.Handle {
	t.Helper()
	handle, err := storagedb.Open(context.Background(), storagedb.Config{
		Driver:       storagedb.DriverSQLite,
		DataDir:      t.TempDir(),
		MigrationKey: t.Name(),
	})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	return handle
}

func hasColumn(t *testing.T, db *sql.DB, table, column string) bool {
	t.Helper()
	rows, err := db.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		t.Fatalf("PRAGMA table_info: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var (
			cid        int
			name       string
			columnType string
			notNull    int
			defaultVal sql.NullString
			pk         int
		)
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultVal, &pk); err != nil {
			t.Fatalf("scan pragma: %v", err)
		}
		if name == column {
			return true
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate pragma: %v", err)
	}
	return false
}
