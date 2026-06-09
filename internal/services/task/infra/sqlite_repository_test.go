package infra

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/tinoosan/agen8/internal/core/types"
	"github.com/tinoosan/agen8/internal/services/project/domain/member"
	"github.com/tinoosan/agen8/internal/services/task/domain"
	storagedb "github.com/tinoosan/agen8/internal/storage/db"
)

var infraTestNow = time.Date(2026, 5, 15, 13, 0, 0, 0, time.UTC)

func TestSQLiteRepositoryCreateGetPreservesTaskDocument(t *testing.T) {
	repo := newSQLiteRepositoryForTest(t)
	task := infraTask("task-1", "project-1", domain.TaskStatusPending)

	if err := repo.CreateTask(context.Background(), task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	got, err := repo.GetTask(context.Background(), domain.TaskID("task-1"))
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}

	if got.ID != task.ID {
		t.Fatalf("id=%q want %q", got.ID, task.ID)
	}
	if got.Metadata["history"].([]any)[0].(map[string]any)["event"] != "created" {
		t.Fatalf("metadata history not preserved: %#v", got.Metadata)
	}
	if len(got.AcceptanceCriteria) != 1 || got.AcceptanceCriteria[0].Text != "tests pass" {
		t.Fatalf("acceptance criteria=%#v want tests pass", got.AcceptanceCriteria)
	}
}

func TestSQLiteRepositoryUpdatePersistsLifecycleState(t *testing.T) {
	repo := newSQLiteRepositoryForTest(t)
	task := infraTask("task-1", "project-1", domain.TaskStatusPending)
	if err := repo.CreateTask(context.Background(), task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	task.Status = domain.TaskStatusInReview
	task.ClaimedByMemberID = member.ID("member-worker")
	task.Summary = "ready for review"
	updatedAt := infraTestNow.Add(time.Minute)
	task.UpdatedAt = &updatedAt
	if err := repo.UpdateTask(context.Background(), task); err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}

	got, err := repo.GetTask(context.Background(), domain.TaskID("task-1"))
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.Status != domain.TaskStatusInReview {
		t.Fatalf("status=%q want in_review", got.Status)
	}
	if got.Summary != "ready for review" {
		t.Fatalf("summary=%q want ready for review", got.Summary)
	}
}

func TestSQLiteRepositoryListAndCountFilters(t *testing.T) {
	repo := newSQLiteRepositoryForTest(t)
	tasks := []domain.Task{
		infraTask("task-1", "project-1", domain.TaskStatusPending),
		infraTask("task-2", "project-1", domain.TaskStatusActive),
		infraTask("task-3", "project-2", domain.TaskStatusActive),
	}
	tasks[1].AssignedTo = member.ID("member-other")
	tasks[1].ClaimedByMemberID = member.ID("member-other")
	tasks[1].TaskKind = "heartbeat"
	for _, task := range tasks {
		if err := repo.CreateTask(context.Background(), task); err != nil {
			t.Fatalf("CreateTask(%s): %v", task.ID, err)
		}
	}

	filter := domain.TaskFilter{
		ProjectID:  types.ProjectID("project-1"),
		AssignedTo: member.ID("member-other"),
		ClaimedBy:  member.ID("member-other"),
		TaskKind:   "heartbeat",
		Status:     []domain.TaskStatus{domain.TaskStatusActive},
	}
	listed, err := repo.ListTasks(context.Background(), filter)
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	if len(listed) != 1 || listed[0].ID != "task-2" {
		t.Fatalf("listed=%v want task-2", listed)
	}
	count, err := repo.CountTasks(context.Background(), filter)
	if err != nil {
		t.Fatalf("CountTasks: %v", err)
	}
	if count != 1 {
		t.Fatalf("count=%d want 1", count)
	}
}

func TestSQLiteRepositoryMissingTaskFailsLoudly(t *testing.T) {
	repo := newSQLiteRepositoryForTest(t)
	_, err := repo.GetTask(context.Background(), domain.TaskID("missing-task"))
	if !errors.Is(err, domain.ErrTaskNotFound) {
		t.Fatalf("err=%v want ErrTaskNotFound", err)
	}
}

func TestSQLiteRepositoryRejectsMetadataFilter(t *testing.T) {
	repo := newSQLiteRepositoryForTest(t)
	_, err := repo.ListTasks(context.Background(), domain.TaskFilter{
		MetadataFilter: map[string]string{"history.event": "created"},
	})
	if err == nil {
		t.Fatal("expected metadata filter error")
	}
}

func TestSQLiteRepositoryFreshSchemaExcludesPlanColumns(t *testing.T) {
	repo := newSQLiteRepositoryForTest(t)
	columns := sqliteTaskColumns(t, repo.db)

	for _, column := range []string{"plan_phase_id", "plan_todo_id"} {
		if columns[column] {
			t.Fatalf("fresh tasks schema contains removed column %q", column)
		}
	}
	for _, column := range []string{"task_id", "project_id", "assigned_to", "claimed_by_member_id", "task_kind", "status", "key_result_ref", "task_json"} {
		if !columns[column] {
			t.Fatalf("fresh tasks schema missing retained column %q", column)
		}
	}
}

func newSQLiteRepositoryForTest(t *testing.T) *SQLiteRepository {
	t.Helper()
	handle, err := storagedb.Open(context.Background(), storagedb.Config{
		Driver:  storagedb.DriverSQLite,
		DataDir: t.TempDir(),
		Migrate: func(ctx context.Context, db *sql.DB, driver storagedb.Driver) error {
			return nil
		},
	})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	repo, err := NewSQLiteRepository(handle)
	if err != nil {
		t.Fatalf("NewSQLiteRepository: %v", err)
	}
	return repo
}

func sqliteTaskColumns(t *testing.T, db *sql.DB) map[string]bool {
	t.Helper()
	rows, err := db.Query(`PRAGMA table_info(tasks)`)
	if err != nil {
		t.Fatalf("pragma tasks: %v", err)
	}
	defer rows.Close()
	columns := map[string]bool{}
	for rows.Next() {
		var (
			cid     int
			name    string
			ctype   string
			notNull int
			dflt    sql.NullString
			pk      int
		)
		if err := rows.Scan(&cid, &name, &ctype, &notNull, &dflt, &pk); err != nil {
			t.Fatalf("scan pragma tasks: %v", err)
		}
		columns[name] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("pragma tasks rows: %v", err)
	}
	return columns
}

func infraTask(id string, projectID string, status domain.TaskStatus) domain.Task {
	return domain.Task{
		ID:                domain.TaskID(id),
		ProjectID:         types.ProjectID(projectID),
		AssignedTo:        member.ID("member-worker"),
		ClaimedByMemberID: "",
		TaskKind:          domain.TaskKindTask,
		CreatedBy:         "member-coordinator",
		Title:             "Write report",
		Description:       "write the report",
		AcceptanceCriteria: []domain.AcceptanceCriterion{
			{ID: "criterion-1", Text: "tests pass"},
		},
		Status:       status,
		CreatedAt:    &infraTestNow,
		UpdatedAt:    &infraTestNow,
		Metadata:     map[string]any{"history": []any{map[string]any{"event": "created"}}},
		KeyResultRef: "kr-1",
	}
}
