package infra

import (
	"context"
	"testing"
	"time"

	"github.com/tinoosan/agen8/internal/config"
	"github.com/tinoosan/agen8/internal/services/decision/domain"
	implstore "github.com/tinoosan/agen8/internal/store"
)

func TestSQLiteRepositoryPreservesStampedMemberName(t *testing.T) {
	t.Parallel()

	repo := newSQLiteDecisionRepoForTest(t)
	ctx := context.Background()
	createdAt := time.Date(2026, 6, 5, 15, 30, 0, 0, time.UTC)
	decision := domain.Decision{
		ID:             domain.DecisionID("dec-member-name"),
		ProjectID:      "project-1",
		Source:         domain.DecisionSourceAgent,
		SourceIdentity: "member-123",
		MemberName:     "Codex backend engineer",
		Title:          "Keep actor names readable",
		Confidence:     0.9,
		CreatedAt:      createdAt,
		Log: &domain.LogPayload{
			Rationale: "Decision feeds are audit records and should not leak raw member ids.",
		},
	}

	if err := repo.CreateDecision(ctx, decision); err != nil {
		t.Fatalf("CreateDecision: %v", err)
	}
	got, err := repo.GetDecision(ctx, decision.ID)
	if err != nil {
		t.Fatalf("GetDecision: %v", err)
	}
	if got.MemberName != "Codex backend engineer" {
		t.Fatalf("MemberName=%q want %q", got.MemberName, "Codex backend engineer")
	}

	listed, err := repo.ListDecisions(ctx, domain.DecisionFilter{
		ProjectID: "project-1",
		SortDesc:  true,
	})
	if err != nil {
		t.Fatalf("ListDecisions: %v", err)
	}
	if len(listed) != 1 || listed[0].MemberName != "Codex backend engineer" {
		t.Fatalf("listed decisions = %#v", listed)
	}
}

func TestSQLiteRepositoryStatsDecisions(t *testing.T) {
	t.Parallel()

	repo := newSQLiteDecisionRepoForTest(t)
	ctx := context.Background()
	createdAt := time.Date(2026, 6, 5, 15, 30, 0, 0, time.UTC)

	decisions := []domain.Decision{
		{
			ID:             "dec-linked-high",
			ProjectID:      "project-1",
			Source:         domain.DecisionSourceAgent,
			SourceIdentity: "member-1",
			Title:          "Adopt Postgres",
			Confidence:     0.9,
			TaskRef:        "task-1",
			CreatedAt:      createdAt,
			Log: &domain.LogPayload{
				Rationale:              "Relational guarantees fit the billing model.",
				InvalidationConditions: []string{"scale beyond 1M rows"},
			},
		},
		{
			ID:             "dec-unlinked-low",
			ProjectID:      "project-1",
			Source:         domain.DecisionSourceAgent,
			SourceIdentity: "member-1",
			Title:          "Maybe use Redis",
			Confidence:     0.3,
			CreatedAt:      createdAt,
			Log:            &domain.LogPayload{Rationale: "Unsure about the caching layer."},
		},
		{
			ID:             "dec-unlinked-borderline",
			ProjectID:      "project-1",
			Source:         domain.DecisionSourceAgent,
			SourceIdentity: "member-1",
			Title:          "Guess at caching",
			Confidence:     0.49,
			CreatedAt:      createdAt,
			Log:            &domain.LogPayload{Rationale: "Borderline confidence, still exploring."},
		},
		{
			ID:             "dec-linked-mid",
			ProjectID:      "project-1",
			Source:         domain.DecisionSourceAgent,
			SourceIdentity: "member-1",
			Title:          "Adopt CI",
			Confidence:     0.5, // exactly at the threshold - not "low" (cutoff is < 0.5)
			MissionRef:     "mis-1",
			CreatedAt:      createdAt,
			Log:            &domain.LogPayload{Rationale: "Automated checks reduce regressions."},
		},
	}
	for _, d := range decisions {
		if err := repo.CreateDecision(ctx, d); err != nil {
			t.Fatalf("CreateDecision %s: %v", d.ID, err)
		}
	}

	stats, err := repo.StatsDecisions(ctx, domain.DecisionFilter{ProjectID: "project-1"})
	if err != nil {
		t.Fatalf("StatsDecisions: %v", err)
	}
	want := domain.DecisionStats{Total: 4, LowConfidence: 2, Unlinked: 2, WithInvalidationConditions: 1}
	if stats != want {
		t.Fatalf("stats = %#v, want %#v", stats, want)
	}

	// Stats must honor the same filters as list/count: a query narrows the set.
	filtered, err := repo.StatsDecisions(ctx, domain.DecisionFilter{ProjectID: "project-1", Query: "postgres"})
	if err != nil {
		t.Fatalf("StatsDecisions filtered: %v", err)
	}
	wantFiltered := domain.DecisionStats{Total: 1, LowConfidence: 0, Unlinked: 0, WithInvalidationConditions: 1}
	if filtered != wantFiltered {
		t.Fatalf("filtered stats = %#v, want %#v", filtered, wantFiltered)
	}
}

func newSQLiteDecisionRepoForTest(t *testing.T) *SQLiteRepository {
	t.Helper()
	handle, err := implstore.GetDBHandle(context.Background(), config.Config{DataDir: t.TempDir()})
	if err != nil {
		t.Fatalf("GetDBHandle: %v", err)
	}
	return NewSQLiteRepository(handle)
}
