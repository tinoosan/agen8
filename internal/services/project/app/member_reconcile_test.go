package app

import (
	"context"
	"testing"
	"time"

	"github.com/tinoosan/agen8/internal/services/project/domain/member"
)

// TestReconcileDuplicateMembersRetiresForksKeepsEarliest seeds the exact mess the old
// harness-label fork bug left behind — one real session sitting in the table as two
// active rows — and proves the reconciliation retires the later row, keeps the earliest,
// leaves cross-project and ref-less members alone, and is safe to run twice.
func TestReconcileDuplicateMembersRetiresForksKeepsEarliest(t *testing.T) {
	service := newProjectServiceForMCPContextTest(t)
	ctx := context.Background()

	base := time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)

	// A harness-label fork: one session (sess-1 in proj-1) registered under two labels,
	// so it sits as two active rows. The "claude" row registered first.
	earliest := activeMemberRecord("member-aaa", "local", "proj-1", "sess-1", "claude", base)
	later := activeMemberRecord("member-bbb", "local", "proj-1", "sess-1", "claude-cli", base.Add(time.Minute))

	// Same native ref but a different project: a different group entirely. Must be left
	// alone — reconciliation never merges across a project boundary.
	otherProject := activeMemberRecord("member-ccc", "local", "proj-2", "sess-1", "claude", base)

	// No native session ref: not a harness session, so it can't be a fork. Untouched.
	noRef := activeMemberRecord("member-ddd", "local", "proj-1", "", "", base)

	for _, m := range []member.Record{earliest, later, otherProject, noRef} {
		if err := service.members.Create(ctx, m); err != nil {
			t.Fatalf("seed member %s: %v", m.ID, err)
		}
	}

	retired, err := service.ReconcileDuplicateMembers(ctx)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if retired != 1 {
		t.Fatalf("expected exactly 1 member retired, got %d", retired)
	}

	assertMemberLifecycle(t, ctx, service, "member-aaa", member.LifecycleActive)  // earliest survives
	assertMemberLifecycle(t, ctx, service, "member-bbb", member.LifecycleRemoved) // later fork retired
	assertMemberLifecycle(t, ctx, service, "member-ccc", member.LifecycleActive)  // other project untouched
	assertMemberLifecycle(t, ctx, service, "member-ddd", member.LifecycleActive)  // ref-less member untouched

	// Idempotent: re-running on now-clean data retires nothing.
	retired2, err := service.ReconcileDuplicateMembers(ctx)
	if err != nil {
		t.Fatalf("reconcile (second run): %v", err)
	}
	if retired2 != 0 {
		t.Fatalf("expected 0 retired on the second run, got %d", retired2)
	}
}

func activeMemberRecord(id, userID, projectID, nativeRef, harnessKind string, registeredAt time.Time) member.Record {
	return member.Record{
		ID:               member.ID(id),
		UserID:           userID,
		ProjectID:        projectID,
		NativeSessionRef: nativeRef,
		HarnessKind:      harnessKind,
		MemberType:       member.TypeCoordinator,
		LifecycleState:   member.LifecycleActive,
		RegisteredAt:     registeredAt,
		UpdatedAt:        registeredAt,
	}
}

func assertMemberLifecycle(t *testing.T, ctx context.Context, service *Service, id, want string) {
	t.Helper()
	got, err := service.members.Get(ctx, id)
	if err != nil {
		t.Fatalf("get member %s: %v", id, err)
	}
	if got.LifecycleState != want {
		t.Fatalf("member %s lifecycle = %q, want %q", id, got.LifecycleState, want)
	}
}
