package infra

import (
	"context"
	"testing"
	"time"

	"github.com/tinoosan/agen8-mcp-server/internal/config"
	"github.com/tinoosan/agen8-mcp-server/internal/services/decision/domain"
	implstore "github.com/tinoosan/agen8-mcp-server/internal/store"
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

func newSQLiteDecisionRepoForTest(t *testing.T) *SQLiteRepository {
	t.Helper()
	handle, err := implstore.GetDBHandle(context.Background(), config.Config{DataDir: t.TempDir()})
	if err != nil {
		t.Fatalf("GetDBHandle: %v", err)
	}
	return NewSQLiteRepository(handle)
}
