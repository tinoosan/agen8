package infra

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/tinoosan/agen8-mcp-server/internal/services/decision/domain"
	humaninput "github.com/tinoosan/agen8-mcp-server/internal/services/humaninput/domain"
	storagedb "github.com/tinoosan/agen8-mcp-server/internal/storage/db"

	_ "modernc.org/sqlite"
)

// migrationSQL creates the decisions table with all V2 columns.
// This matches the result of running migrations 004 + 007 in sequence.
const migrationSQL = `
CREATE TABLE IF NOT EXISTS decisions (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL,
    space_id TEXT DEFAULT '',
    source TEXT NOT NULL,
    kind TEXT NOT NULL DEFAULT 'log',
    source_identity TEXT DEFAULT '',
    run_id TEXT DEFAULT '',
    title TEXT NOT NULL,
    rationale TEXT NOT NULL,
    context TEXT DEFAULT '',
    questions_json TEXT DEFAULT '[]',
    answers_json TEXT DEFAULT '[]',
    cancelled INTEGER NOT NULL DEFAULT 0,
    alternatives_rejected TEXT DEFAULT '',
    invalidation_conditions_json TEXT DEFAULT '[]',
    confidence REAL NOT NULL DEFAULT 1.0,
    outcome TEXT DEFAULT '',
    task_ref TEXT DEFAULT '',
    key_result_ref TEXT DEFAULT '',
    mission_ref TEXT DEFAULT '',
    plan_ref TEXT DEFAULT '',
    operator_action_ref TEXT DEFAULT '',
    escalation_ref TEXT DEFAULT '',
    correlation_ref TEXT DEFAULT '',
    informed_by_ref TEXT DEFAULT '',
    tags_json TEXT DEFAULT '[]',
    metadata_json TEXT DEFAULT '{}',
    created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_dec_project ON decisions(project_id);
CREATE INDEX IF NOT EXISTS idx_dec_key_result ON decisions(key_result_ref);
CREATE INDEX IF NOT EXISTS idx_dec_operator_action ON decisions(operator_action_ref);
CREATE INDEX IF NOT EXISTS idx_dec_source ON decisions(source);
CREATE INDEX IF NOT EXISTS idx_dec_task ON decisions(task_ref);
CREATE INDEX IF NOT EXISTS idx_dec_mission ON decisions(mission_ref);
CREATE INDEX IF NOT EXISTS idx_dec_plan ON decisions(plan_ref);
CREATE INDEX IF NOT EXISTS idx_dec_escalation ON decisions(escalation_ref);
CREATE INDEX IF NOT EXISTS idx_dec_space ON decisions(space_id);
`

func setupTestDB(t *testing.T) *storagedb.Handle {
	t.Helper()
	handle, err := storagedb.Open(context.Background(), storagedb.Config{
		Driver:  storagedb.DriverSQLite,
		DataDir: t.TempDir(),
		Migrate: func(ctx context.Context, db *sql.DB, driver storagedb.Driver) error {
			_, err := db.ExecContext(ctx, migrationSQL)
			return err
		},
	})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	return handle
}

func makeDecision(id string, projectID string, opts ...func(*domain.Decision)) domain.Decision {
	d := domain.Decision{
		ID:         domain.DecisionID(id),
		ProjectID:  projectID,
		SpaceID:    "space-1",
		Source:     domain.DecisionSourceAgent,
		Title:      "Test decision",
		Confidence: 0.8,
		CreatedAt:  time.Now().UTC(),
		Log:        &domain.LogPayload{Rationale: "Test rationale"},
	}
	for _, opt := range opts {
		opt(&d)
	}
	return d
}

// makeDecisionAskUser builds a fixture with an AskUserPayload.
func makeDecisionAskUser(id string, projectID string, opts ...func(*domain.Decision)) domain.Decision {
	d := domain.Decision{
		ID:         domain.DecisionID(id),
		ProjectID:  projectID,
		SpaceID:    "space-1",
		Source:     domain.DecisionSourceAgent,
		Title:      "Test ask",
		Confidence: 0.5,
		CreatedAt:  time.Now().UTC(),
		AskUser:    &domain.AskUserPayload{Questions: []humaninput.Question{{ID: "q1", Text: "?"}}},
	}
	for _, opt := range opts {
		opt(&d)
	}
	return d
}

// -- CreateDecision and GetDecision ----------------------------------------------------------

func TestSQLiteRepository_SaveAndGet(t *testing.T) {
	handle := setupTestDB(t)
	repo := NewSQLiteRepository(handle)
	ctx := context.Background()

	decision := domain.Decision{
		ID:                "dec-test-123",
		ProjectID:         "proj-1",
		SpaceID:           "space-1",
		Source:            domain.DecisionSourceAgent,
		SourceIdentity:    "researcher",
		RunID:             "run-abc",
		Title:             "Use PostgreSQL",
		Confidence:        0.85,
		TaskRef:           "task-xyz",
		KeyResultRef:      "kr-abc",
		MissionRef:        "mission-1",
		PlanRef:           "plan-1",
		OperatorActionRef: "oa-456",
		EscalationRef:     "esc-789",
		CorrelationRef:    "corr-111",
		InformedByRef:     "dec-prior-222",
		Tags:              []string{"architecture", "database"},
		Metadata:          map[string]string{"env": "production", "priority": "high"},
		CreatedAt:         time.Now().UTC().Truncate(time.Microsecond),
		Log: &domain.LogPayload{
			Rationale:            "Better OLAP performance",
			AlternativesRejected: "ClickHouse, DuckDB",
			InvalidationConditions: []string{
				"Query latency exceeds target",
				"Operational complexity blocks adoption",
			},
			Outcome: "PostgreSQL deployed",
		},
	}

	if err := repo.CreateDecision(ctx, decision); err != nil {
		t.Fatalf("CreateDecision: %v", err)
	}

	got, err := repo.GetDecision(ctx, "dec-test-123")
	if err != nil {
		t.Fatalf("GetDecision: %v", err)
	}

	// Verify all fields.
	if got.ID != decision.ID {
		t.Errorf("ID = %q, want %q", got.ID, decision.ID)
	}
	if got.ProjectID != decision.ProjectID {
		t.Errorf("ProjectID = %q, want %q", got.ProjectID, decision.ProjectID)
	}
	if got.SpaceID != decision.SpaceID {
		t.Errorf("SpaceID = %q, want %q", got.SpaceID, decision.SpaceID)
	}
	if got.Source != decision.Source {
		t.Errorf("Source = %q, want %q", got.Source, decision.Source)
	}
	if got.SourceIdentity != decision.SourceIdentity {
		t.Errorf("SourceIdentity = %q, want %q", got.SourceIdentity, decision.SourceIdentity)
	}
	if got.RunID != decision.RunID {
		t.Errorf("RunID = %q, want %q", got.RunID, decision.RunID)
	}
	if got.Title != decision.Title {
		t.Errorf("Title = %q, want %q", got.Title, decision.Title)
	}
	if got.Log == nil {
		t.Fatalf("expected Log payload, got nil")
	}
	wantLog := decision.Log
	if got.Log.Rationale != wantLog.Rationale {
		t.Errorf("Rationale = %q, want %q", got.Log.Rationale, wantLog.Rationale)
	}
	if got.Log.AlternativesRejected != wantLog.AlternativesRejected {
		t.Errorf("AlternativesRejected = %q, want %q", got.Log.AlternativesRejected, wantLog.AlternativesRejected)
	}
	if strings.Join(got.Log.InvalidationConditions, "|") != strings.Join(wantLog.InvalidationConditions, "|") {
		t.Errorf("InvalidationConditions = %v, want %v", got.Log.InvalidationConditions, wantLog.InvalidationConditions)
	}
	if got.Confidence != decision.Confidence {
		t.Errorf("Confidence = %v, want %v", got.Confidence, decision.Confidence)
	}
	if got.Log.Outcome != wantLog.Outcome {
		t.Errorf("Outcome = %q, want %q", got.Log.Outcome, wantLog.Outcome)
	}
	if got.TaskRef != decision.TaskRef {
		t.Errorf("TaskRef = %q, want %q", got.TaskRef, decision.TaskRef)
	}
	if got.KeyResultRef != decision.KeyResultRef {
		t.Errorf("KeyResultRef = %q, want %q", got.KeyResultRef, decision.KeyResultRef)
	}
	if got.MissionRef != decision.MissionRef {
		t.Errorf("MissionRef = %q, want %q", got.MissionRef, decision.MissionRef)
	}
	if got.PlanRef != decision.PlanRef {
		t.Errorf("PlanRef = %q, want %q", got.PlanRef, decision.PlanRef)
	}
	if got.OperatorActionRef != decision.OperatorActionRef {
		t.Errorf("OperatorActionRef = %q, want %q", got.OperatorActionRef, decision.OperatorActionRef)
	}
	if got.EscalationRef != decision.EscalationRef {
		t.Errorf("EscalationRef = %q, want %q", got.EscalationRef, decision.EscalationRef)
	}
	if got.CorrelationRef != decision.CorrelationRef {
		t.Errorf("CorrelationRef = %q, want %q", got.CorrelationRef, decision.CorrelationRef)
	}
	if got.InformedByRef != decision.InformedByRef {
		t.Errorf("InformedByRef = %q, want %q", got.InformedByRef, decision.InformedByRef)
	}

	// CreatedAt.
	if !got.CreatedAt.UTC().Truncate(time.Microsecond).Equal(decision.CreatedAt) {
		t.Errorf("CreatedAt = %v, want %v", got.CreatedAt, decision.CreatedAt)
	}

	// Tags.
	if len(got.Tags) != 2 {
		t.Fatalf("Tags length = %d, want 2", len(got.Tags))
	}
	if got.Tags[0] != "architecture" {
		t.Errorf("Tags[0] = %q, want %q", got.Tags[0], "architecture")
	}
	if got.Tags[1] != "database" {
		t.Errorf("Tags[1] = %q, want %q", got.Tags[1], "database")
	}

	// Metadata.
	if len(got.Metadata) != 2 {
		t.Errorf("Metadata length = %d, want 2", len(got.Metadata))
	}
	if got.Metadata["env"] != "production" {
		t.Errorf("Metadata[env] = %q, want %q", got.Metadata["env"], "production")
	}
	if got.Metadata["priority"] != "high" {
		t.Errorf("Metadata[priority] = %q, want %q", got.Metadata["priority"], "high")
	}
}

func TestSQLiteRepository_SaveGeneratesID(t *testing.T) {
	handle := setupTestDB(t)
	repo := NewSQLiteRepository(handle)
	ctx := context.Background()

	decision := makeDecision("", "proj-1")

	if err := repo.CreateDecision(ctx, decision); err != nil {
		t.Fatalf("CreateDecision: %v", err)
	}

	results, err := repo.ListDecisions(ctx, domain.DecisionFilter{ProjectID: "proj-1"})
	if err != nil {
		t.Fatalf("ListDecisions: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if results[0].ID == "" {
		t.Error("expected generated ID, got empty")
	}
	if !strings.HasPrefix(string(results[0].ID), "dec-") {
		t.Errorf("expected ID with dec- prefix, got %q", results[0].ID)
	}
}

func TestSQLiteRepository_SaveUpsert(t *testing.T) {
	handle := setupTestDB(t)
	repo := NewSQLiteRepository(handle)
	ctx := context.Background()

	decision := makeDecision("dec-upsert", "proj-1")
	decision.Title = "Original title"

	if err := repo.CreateDecision(ctx, decision); err != nil {
		t.Fatalf("first CreateDecision: %v", err)
	}

	decision.Title = "Updated title"
	if err := repo.CreateDecision(ctx, decision); err != nil {
		t.Fatalf("second CreateDecision: %v", err)
	}

	got, err := repo.GetDecision(ctx, "dec-upsert")
	if err != nil {
		t.Fatalf("GetDecision: %v", err)
	}
	if got.Title != "Updated title" {
		t.Errorf("Title = %q, want %q", got.Title, "Updated title")
	}
}

func TestSQLiteRepository_GetNotFound(t *testing.T) {
	handle := setupTestDB(t)
	repo := NewSQLiteRepository(handle)
	ctx := context.Background()

	_, err := repo.GetDecision(ctx, "dec-does-not-exist")
	if err == nil {
		t.Fatal("expected error for non-existent ID, got nil")
	}
}

func TestSQLiteRepository_SaveAndGet_NilMetadata(t *testing.T) {
	handle := setupTestDB(t)
	repo := NewSQLiteRepository(handle)
	ctx := context.Background()

	decision := makeDecision("dec-nil-meta", "proj-1")
	decision.Metadata = nil

	if err := repo.CreateDecision(ctx, decision); err != nil {
		t.Fatalf("CreateDecision: %v", err)
	}

	got, err := repo.GetDecision(ctx, "dec-nil-meta")
	if err != nil {
		t.Fatalf("GetDecision: %v", err)
	}
	if got.Metadata != nil {
		t.Errorf("Metadata = %v, want nil", got.Metadata)
	}
}

func TestSQLiteRepository_SaveAndGet_NilTags(t *testing.T) {
	handle := setupTestDB(t)
	repo := NewSQLiteRepository(handle)
	ctx := context.Background()

	decision := makeDecision("dec-nil-tags", "proj-1")
	decision.Tags = nil

	if err := repo.CreateDecision(ctx, decision); err != nil {
		t.Fatalf("CreateDecision: %v", err)
	}

	got, err := repo.GetDecision(ctx, "dec-nil-tags")
	if err != nil {
		t.Fatalf("GetDecision: %v", err)
	}
	if got.Tags != nil {
		t.Errorf("Tags = %v, want nil", got.Tags)
	}
}

// -- ListDecisions ---------------------------------------------------------

func TestSQLiteRepository_ListDecisions(t *testing.T) {
	handle := setupTestDB(t)
	repo := NewSQLiteRepository(handle)
	ctx := context.Background()

	decisions := []domain.Decision{
		makeDecision("dec-1", "proj-1"),
		makeDecision("dec-2", "proj-1"),
		makeDecision("dec-3", "proj-2"),
	}
	for _, d := range decisions {
		if err := repo.CreateDecision(ctx, d); err != nil {
			t.Fatalf("CreateDecision %s: %v", d.ID, err)
		}
	}

	results, err := repo.ListDecisions(ctx, domain.DecisionFilter{ProjectID: "proj-1"})
	if err != nil {
		t.Fatalf("ListDecisions: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}
	for _, r := range results {
		if r.ProjectID != "proj-1" {
			t.Errorf("got ProjectID = %q, want proj-1", r.ProjectID)
		}
	}
}

func TestSQLiteRepository_ListDecisions_FilterBySources(t *testing.T) {
	handle := setupTestDB(t)
	repo := NewSQLiteRepository(handle)
	ctx := context.Background()

	decisions := []domain.Decision{
		makeDecision("dec-1", "proj-1", func(d *domain.Decision) { d.Source = domain.DecisionSourceAgent }),
		makeDecision("dec-2", "proj-1", func(d *domain.Decision) { d.Source = domain.DecisionSourceOperator }),
		makeDecision("dec-3", "proj-1", func(d *domain.Decision) { d.Source = domain.DecisionSourceOperator }),
		makeDecision("dec-4", "proj-1", func(d *domain.Decision) { d.Source = domain.DecisionSourceAgent }),
	}
	for _, d := range decisions {
		if err := repo.CreateDecision(ctx, d); err != nil {
			t.Fatalf("CreateDecision %s: %v", d.ID, err)
		}
	}

	// Single source filter.
	results, err := repo.ListDecisions(ctx, domain.DecisionFilter{ProjectID: "proj-1", Sources: []domain.DecisionSource{domain.DecisionSourceAgent}})
	if err != nil {
		t.Fatalf("ListDecisions: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}
	for _, r := range results {
		if r.Source != domain.DecisionSourceAgent {
			t.Errorf("got source = %q, want agent", r.Source)
		}
	}

	// Multiple sources filter (AND with project, OR between sources).
	results, err = repo.ListDecisions(ctx, domain.DecisionFilter{ProjectID: "proj-1",
		Sources: []domain.DecisionSource{domain.DecisionSourceAgent, domain.DecisionSourceOperator},
	})
	if err != nil {
		t.Fatalf("ListDecisions multi-source: %v", err)
	}
	if len(results) != 4 {
		t.Fatalf("got %d results, want 4", len(results))
	}
}

func TestSQLiteRepository_ListDecisions_FilterBySpaceID(t *testing.T) {
	handle := setupTestDB(t)
	repo := NewSQLiteRepository(handle)
	ctx := context.Background()

	decisions := []domain.Decision{
		makeDecision("dec-1", "proj-1", func(d *domain.Decision) { d.SpaceID = "space-a" }),
		makeDecision("dec-2", "proj-1", func(d *domain.Decision) { d.SpaceID = "space-b" }),
		makeDecision("dec-3", "proj-1", func(d *domain.Decision) { d.SpaceID = "space-a" }),
	}
	for _, d := range decisions {
		if err := repo.CreateDecision(ctx, d); err != nil {
			t.Fatalf("CreateDecision %s: %v", d.ID, err)
		}
	}

	results, err := repo.ListDecisions(ctx, domain.DecisionFilter{ProjectID: "proj-1", SpaceID: "space-a"})
	if err != nil {
		t.Fatalf("ListDecisions: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}
	for _, r := range results {
		if r.SpaceID != "space-a" {
			t.Errorf("got SpaceID = %q, want space-a", r.SpaceID)
		}
	}
}

func TestSQLiteRepository_ListDecisions_FilterBySince(t *testing.T) {
	handle := setupTestDB(t)
	repo := NewSQLiteRepository(handle)
	ctx := context.Background()

	base := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	decisions := []domain.Decision{
		makeDecision("dec-1", "proj-1", func(d *domain.Decision) { d.CreatedAt = base }),
		makeDecision("dec-2", "proj-1", func(d *domain.Decision) { d.CreatedAt = base.Add(24 * time.Hour) }),
		makeDecision("dec-3", "proj-1", func(d *domain.Decision) { d.CreatedAt = base.Add(48 * time.Hour) }),
	}
	for _, d := range decisions {
		if err := repo.CreateDecision(ctx, d); err != nil {
			t.Fatalf("CreateDecision %s: %v", d.ID, err)
		}
	}

	since := base.Add(24 * time.Hour)
	results, err := repo.ListDecisions(ctx, domain.DecisionFilter{ProjectID: "proj-1", Since: &since})
	if err != nil {
		t.Fatalf("ListDecisions: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}
}

func TestSQLiteRepository_ListDecisions_FilterByTags(t *testing.T) {
	handle := setupTestDB(t)
	repo := NewSQLiteRepository(handle)
	ctx := context.Background()

	decisions := []domain.Decision{
		makeDecision("dec-1", "proj-1", func(d *domain.Decision) { d.Tags = []string{"architecture", "database"} }),
		makeDecision("dec-2", "proj-1", func(d *domain.Decision) { d.Tags = []string{"architecture"} }),
		makeDecision("dec-3", "proj-1", func(d *domain.Decision) { d.Tags = []string{"database"} }),
		makeDecision("dec-4", "proj-1", func(d *domain.Decision) { d.Tags = nil }),
	}
	for _, d := range decisions {
		if err := repo.CreateDecision(ctx, d); err != nil {
			t.Fatalf("CreateDecision %s: %v", d.ID, err)
		}
	}

	// Single tag filter.
	results, err := repo.ListDecisions(ctx, domain.DecisionFilter{ProjectID: "proj-1", Tags: []string{"architecture"}})
	if err != nil {
		t.Fatalf("ListDecisions: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("got %d results for tag 'architecture', want 2", len(results))
	}

	// Multi-tag filter (AND semantics: both tags must be present).
	results, err = repo.ListDecisions(ctx, domain.DecisionFilter{ProjectID: "proj-1", Tags: []string{"architecture", "database"}})
	if err != nil {
		t.Fatalf("ListDecisions multi-tag: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d results for both tags, want 1", len(results))
	}
	if string(results[0].ID) != "dec-1" {
		t.Errorf("got ID = %q, want dec-1", results[0].ID)
	}
}

func TestSQLiteRepository_ListDecisions_FilterByQueryUntilAndSort(t *testing.T) {
	handle := setupTestDB(t)
	repo := NewSQLiteRepository(handle)
	ctx := context.Background()

	base := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	decisions := []domain.Decision{
		makeDecision("dec-1", "proj-1", func(d *domain.Decision) {
			d.Title = "Use PostgreSQL"
			d.Log = &domain.LogPayload{Rationale: "Reliable and boring"}
			d.CreatedAt = base
		}),
		makeDecision("dec-2", "proj-1", func(d *domain.Decision) {
			d.Title = "Delay migration"
			d.Log = &domain.LogPayload{Rationale: "Need more evidence"}
			d.CreatedAt = base.Add(24 * time.Hour)
		}),
		makeDecision("dec-3", "proj-1", func(d *domain.Decision) {
			d.Title = "Review PostgreSQL backup plan"
			d.Log = &domain.LogPayload{Rationale: "Operator asked for follow-up"}
			d.CreatedAt = base.Add(48 * time.Hour)
		}),
	}
	for _, d := range decisions {
		if err := repo.CreateDecision(ctx, d); err != nil {
			t.Fatalf("CreateDecision %s: %v", d.ID, err)
		}
	}

	until := base.Add(24 * time.Hour)
	results, err := repo.ListDecisions(ctx, domain.DecisionFilter{ProjectID: "proj-1",
		Query:    "postgresql",
		Until:    &until,
		SortDesc: false,
	})
	if err != nil {
		t.Fatalf("ListDecisions: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if string(results[0].ID) != "dec-1" {
		t.Fatalf("got ID = %q, want dec-1", results[0].ID)
	}
}

func TestSQLiteRepository_ListDecisions_CombinedFilters(t *testing.T) {
	handle := setupTestDB(t)
	repo := NewSQLiteRepository(handle)
	ctx := context.Background()

	base := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	decisions := []domain.Decision{
		makeDecision("dec-1", "proj-1", func(d *domain.Decision) {
			d.Source = domain.DecisionSourceAgent
			d.SpaceID = "space-a"
			d.Tags = []string{"arch"}
			d.CreatedAt = base
		}),
		makeDecision("dec-2", "proj-1", func(d *domain.Decision) {
			d.Source = domain.DecisionSourceAgent
			d.SpaceID = "space-a"
			d.Tags = []string{"arch"}
			d.CreatedAt = base.Add(48 * time.Hour)
		}),
		makeDecision("dec-3", "proj-1", func(d *domain.Decision) {
			d.Source = domain.DecisionSourceOperator
			d.SpaceID = "space-a"
			d.Tags = []string{"arch"}
			d.CreatedAt = base.Add(48 * time.Hour)
		}),
	}
	for _, d := range decisions {
		if err := repo.CreateDecision(ctx, d); err != nil {
			t.Fatalf("CreateDecision %s: %v", d.ID, err)
		}
	}

	since := base.Add(24 * time.Hour)
	results, err := repo.ListDecisions(ctx, domain.DecisionFilter{ProjectID: "proj-1",
		Sources: []domain.DecisionSource{domain.DecisionSourceAgent},
		SpaceID: "space-a",
		Tags:    []string{"arch"},
		Since:   &since,
	})
	if err != nil {
		t.Fatalf("ListDecisions: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if string(results[0].ID) != "dec-2" {
		t.Errorf("got ID = %q, want dec-2", results[0].ID)
	}
}

func TestSQLiteRepository_ListDecisions_Pagination(t *testing.T) {
	handle := setupTestDB(t)
	repo := NewSQLiteRepository(handle)
	ctx := context.Background()

	base := time.Now().UTC()
	for i := 0; i < 5; i++ {
		d := makeDecision("", "proj-1", func(d *domain.Decision) {
			d.CreatedAt = base.Add(time.Duration(i) * time.Second)
		})
		if err := repo.CreateDecision(ctx, d); err != nil {
			t.Fatalf("CreateDecision: %v", err)
		}
	}

	// Limit to 2.
	results, err := repo.ListDecisions(ctx, domain.DecisionFilter{ProjectID: "proj-1", Limit: 2})
	if err != nil {
		t.Fatalf("ListDecisions with limit: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}

	// Offset 2, limit 2.
	results, err = repo.ListDecisions(ctx, domain.DecisionFilter{ProjectID: "proj-1", Limit: 2, Offset: 2})
	if err != nil {
		t.Fatalf("ListDecisions with offset: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}
}

// -- ListDecisionsByKeyResult -------------------------------------------------------

func TestSQLiteRepository_ListDecisionsByKeyResult(t *testing.T) {
	handle := setupTestDB(t)
	repo := NewSQLiteRepository(handle)
	ctx := context.Background()

	decisions := []domain.Decision{
		makeDecision("dec-1", "proj-1", func(d *domain.Decision) { d.KeyResultRef = "kr-abc" }),
		makeDecision("dec-2", "proj-1", func(d *domain.Decision) { d.KeyResultRef = "kr-abc" }),
		makeDecision("dec-3", "proj-1", func(d *domain.Decision) { d.KeyResultRef = "kr-xyz" }),
	}
	for _, d := range decisions {
		if err := repo.CreateDecision(ctx, d); err != nil {
			t.Fatalf("CreateDecision %s: %v", d.ID, err)
		}
	}

	results, err := repo.ListDecisionsByKeyResult(ctx, "kr-abc")
	if err != nil {
		t.Fatalf("ListDecisionsByKeyResult: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}
	for _, r := range results {
		if r.KeyResultRef != "kr-abc" {
			t.Errorf("got KeyResultRef = %q, want kr-abc", r.KeyResultRef)
		}
	}
}

// -- CountDecisions -----------------------------------------------------------------

func TestSQLiteRepository_Count(t *testing.T) {
	handle := setupTestDB(t)
	repo := NewSQLiteRepository(handle)
	ctx := context.Background()

	decisions := []domain.Decision{
		makeDecision("dec-1", "proj-1", func(d *domain.Decision) { d.Source = domain.DecisionSourceAgent }),
		makeDecision("dec-2", "proj-1", func(d *domain.Decision) { d.Source = domain.DecisionSourceOperator }),
		makeDecision("dec-3", "proj-1", func(d *domain.Decision) { d.Source = domain.DecisionSourceAgent }),
		makeDecision("dec-4", "proj-2", func(d *domain.Decision) { d.Source = domain.DecisionSourceAgent }),
	}
	for _, d := range decisions {
		if err := repo.CreateDecision(ctx, d); err != nil {
			t.Fatalf("CreateDecision %s: %v", d.ID, err)
		}
	}

	// CountDecisions all for proj-1.
	count, err := repo.CountDecisions(ctx, domain.DecisionFilter{ProjectID: "proj-1"})
	if err != nil {
		t.Fatalf("CountDecisions: %v", err)
	}
	if count != 3 {
		t.Fatalf("got %d, want 3", count)
	}

	// CountDecisions with source filter.
	count, err = repo.CountDecisions(ctx, domain.DecisionFilter{ProjectID: "proj-1", Sources: []domain.DecisionSource{domain.DecisionSourceAgent}})
	if err != nil {
		t.Fatalf("CountDecisions: %v", err)
	}
	if count != 2 {
		t.Fatalf("got %d, want 2", count)
	}

	// CountDecisions for proj-2.
	count, err = repo.CountDecisions(ctx, domain.DecisionFilter{ProjectID: "proj-2"})
	if err != nil {
		t.Fatalf("CountDecisions: %v", err)
	}
	if count != 1 {
		t.Fatalf("got %d, want 1", count)
	}
}

// -- ExportDecisions ----------------------------------------------------------------

func TestSQLiteRepository_Export(t *testing.T) {
	handle := setupTestDB(t)
	repo := NewSQLiteRepository(handle)
	ctx := context.Background()

	for i := 0; i < 10; i++ {
		d := makeDecision("", "proj-1")
		if err := repo.CreateDecision(ctx, d); err != nil {
			t.Fatalf("CreateDecision: %v", err)
		}
	}

	// ExportDecisions ignores pagination — returns all.
	results, err := repo.ExportDecisions(ctx, domain.DecisionFilter{ProjectID: "proj-1"})
	if err != nil {
		t.Fatalf("ExportDecisions: %v", err)
	}
	if len(results) != 10 {
		t.Fatalf("got %d results, want 10", len(results))
	}
}

func TestSQLiteRepository_Export_WithFilter(t *testing.T) {
	handle := setupTestDB(t)
	repo := NewSQLiteRepository(handle)
	ctx := context.Background()

	decisions := []domain.Decision{
		makeDecision("dec-1", "proj-1", func(d *domain.Decision) { d.Source = domain.DecisionSourceAgent }),
		makeDecision("dec-2", "proj-1", func(d *domain.Decision) { d.Source = domain.DecisionSourceOperator }),
		makeDecision("dec-3", "proj-1", func(d *domain.Decision) { d.Source = domain.DecisionSourceOperator }),
	}
	for _, d := range decisions {
		if err := repo.CreateDecision(ctx, d); err != nil {
			t.Fatalf("CreateDecision %s: %v", d.ID, err)
		}
	}

	results, err := repo.ExportDecisions(ctx, domain.DecisionFilter{ProjectID: "proj-1",
		Sources: []domain.DecisionSource{domain.DecisionSourceAgent, domain.DecisionSourceOperator},
	})
	if err != nil {
		t.Fatalf("ExportDecisions: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("got %d results, want 3", len(results))
	}
}

// -- CreateDecision with new FK fields -----------------------------------------------

func TestSQLiteRepository_SaveWithAllFKFields(t *testing.T) {
	handle := setupTestDB(t)
	repo := NewSQLiteRepository(handle)
	ctx := context.Background()

	d := domain.Decision{
		ID:                "dec-fk-test",
		ProjectID:         "proj-1",
		SpaceID:           "space-1",
		Source:            domain.DecisionSourceOperator,
		SourceIdentity:    "operator",
		RunID:             "run-xyz",
		Title:             "Auto-escalate overdue task",
		Confidence:        0.95,
		TaskRef:           "task-100",
		KeyResultRef:      "kr-50",
		MissionRef:        "mission-10",
		OperatorActionRef: "oa-200",
		EscalationRef:     "esc-300",
		CorrelationRef:    "corr-400",
		InformedByRef:     "dec-500",
		Tags:              []string{"escalation", "deadline"},
		Metadata:          map[string]string{"original_urgency": "medium"},
		CreatedAt:         time.Now().UTC().Truncate(time.Microsecond),
		Log: &domain.LogPayload{
			Rationale:            "Task exceeded deadline by 2 hours",
			AlternativesRejected: "Wait another hour, cancel task",
			Outcome:              "Task escalated to critical",
		},
	}

	if err := repo.CreateDecision(ctx, d); err != nil {
		t.Fatalf("CreateDecision: %v", err)
	}

	got, err := repo.GetDecision(ctx, "dec-fk-test")
	if err != nil {
		t.Fatalf("GetDecision: %v", err)
	}

	if got.MissionRef != "mission-10" {
		t.Errorf("MissionRef = %q, want mission-10", got.MissionRef)
	}
	if got.EscalationRef != "esc-300" {
		t.Errorf("EscalationRef = %q, want esc-300", got.EscalationRef)
	}
	if got.CorrelationRef != "corr-400" {
		t.Errorf("CorrelationRef = %q, want corr-400", got.CorrelationRef)
	}
	if got.InformedByRef != "dec-500" {
		t.Errorf("InformedByRef = %q, want dec-500", got.InformedByRef)
	}
	if got.RunID != "run-xyz" {
		t.Errorf("RunID = %q, want run-xyz", got.RunID)
	}
	if got.Log == nil {
		t.Fatalf("expected Log payload, got nil")
	}
	if got.Log.Outcome != "Task escalated to critical" {
		t.Errorf("Outcome = %q, want 'Task escalated to critical'", got.Log.Outcome)
	}
	if got.Source != domain.DecisionSourceOperator {
		t.Errorf("Source = %q, want operator", got.Source)
	}
}
