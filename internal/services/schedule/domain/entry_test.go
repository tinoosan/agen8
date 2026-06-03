package domain

import (
	"testing"
	"time"

	spacedomain "github.com/tinoosan/agen8-mcp-server/internal/services/space/domain"
	"github.com/tinoosan/agen8-mcp-server/internal/services/space/domain/member"
)

func TestNewEntryCreatesActiveEntryWithNextRun(t *testing.T) {
	now := time.Date(2026, 3, 22, 10, 0, 0, 0, time.UTC)
	runAt := now.Add(time.Hour)
	entry, err := NewEntry(NewEntryInput{
		ID:        "schedule-test",
		SpaceID:   spacedomain.SpaceID("space-1"),
		CreatedBy: ActorRef("member-1"),
		Title:     "Check admission status",
		Timing:    TimingExpression{Mode: TimingModeOnce, RunAt: &runAt},
		Target: Target{
			Kind: TargetKindTaskCreate,
			TaskCreate: TaskCreatePayload{
				TargetMemberID: member.ID("worker-1"),
				Title:          "Check admission status",
				Description:    "Look for a status update",
			},
		},
	}, now)
	if err != nil {
		t.Fatalf("NewEntry() failed: %v", err)
	}
	if entry.Status != EntryStatusActive {
		t.Fatalf("Status = %q, want %q", entry.Status, EntryStatusActive)
	}
	if entry.NextRunAt == nil || !entry.NextRunAt.Equal(runAt) {
		t.Fatalf("NextRunAt = %v, want %v", entry.NextRunAt, runAt)
	}
}

func TestEntryAdvanceOnceMarksTriggered(t *testing.T) {
	now := time.Date(2026, 3, 22, 10, 0, 0, 0, time.UTC)
	runAt := now.Add(time.Hour)
	entry, err := NewEntry(NewEntryInput{
		ID:        "schedule-test",
		SpaceID:   spacedomain.SpaceID("space-1"),
		CreatedBy: ActorRef("member-1"),
		Title:     "Check admission status",
		Timing:    TimingExpression{Mode: TimingModeOnce, RunAt: &runAt},
		Target: Target{
			Kind: TargetKindTaskCreate,
			TaskCreate: TaskCreatePayload{
				TargetMemberID: member.ID("worker-1"),
				Title:          "Check admission status",
				Description:    "Look for a status update",
			},
		},
	}, now)
	if err != nil {
		t.Fatalf("NewEntry() failed: %v", err)
	}
	next, err := entry.AdvanceAfterRun(runAt, runAt.Add(time.Minute))
	if err != nil {
		t.Fatalf("AdvanceAfterRun() failed: %v", err)
	}
	if next.Status != EntryStatusTriggered {
		t.Fatalf("Status = %q, want %q", next.Status, EntryStatusTriggered)
	}
	if next.NextRunAt != nil {
		t.Fatalf("NextRunAt = %v, want nil", next.NextRunAt)
	}
}

func TestRunRequiresTargetObjectIDOnSuccess(t *testing.T) {
	now := time.Date(2026, 3, 22, 10, 0, 0, 0, time.UTC)
	entry := Entry{
		ID:        "schedule-test",
		SpaceID:   spacedomain.SpaceID("space-1"),
		Target:    Target{Kind: TargetKindTaskCreate},
		CreatedAt: now,
		UpdatedAt: now,
	}
	run, err := NewStartedRun(entry, now, now)
	if err != nil {
		t.Fatalf("NewStartedRun() failed: %v", err)
	}
	if _, err := run.Succeed("", now); err == nil {
		t.Fatalf("Succeed() should reject empty target object ID")
	}
}
