package attention

import (
	"context"
	"testing"
	"time"

	"github.com/tinoosan/agen8/internal/eventbus"
	"github.com/tinoosan/agen8/internal/services/project/domain/member"
)

type fakeMembers struct {
	records []member.Record
	gotRef  string
}

func (f *fakeMembers) ListMembers(_ context.Context, filter member.Filter) ([]member.Record, error) {
	f.gotRef = filter.NativeSessionRef
	return f.records, nil
}

type capturingBus struct {
	events []eventbus.AttentionEvent
}

func (b *capturingBus) Publish(_ string, event any) error {
	if e, ok := event.(eventbus.AttentionEvent); ok {
		b.events = append(b.events, e)
	}
	return nil
}

type tickClock struct{ t time.Time }

func (c *tickClock) now() time.Time { return c.t }

func newServiceForTest(t *testing.T, members *fakeMembers, bus *capturingBus, clock *tickClock, ttl time.Duration) *Service {
	t.Helper()
	svc, err := NewService(members, bus, clock.now, ttl, nil)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return svc
}

func TestReportAttributesToMemberAndPublishes(t *testing.T) {
	members := &fakeMembers{records: []member.Record{{
		ID: "member-1", ProjectID: "proj-1", DisplayName: "Sora (Full Stack Engineer)", HarnessKind: "claude-code",
	}}}
	bus := &capturingBus{}
	clock := &tickClock{t: time.Unix(1700000000, 0).UTC()}
	svc := newServiceForTest(t, members, bus, clock, DefaultTTL)

	entry, err := svc.Report(context.Background(), "user-1", Event{SessionRef: "sess-abc", Harness: "claude", Kind: KindWaiting})
	if err != nil {
		t.Fatalf("Report: %v", err)
	}
	if entry.MemberID != "member-1" || entry.ProjectID != "proj-1" || entry.MemberName != "Sora (Full Stack Engineer)" {
		t.Fatalf("attribution wrong: %+v", entry)
	}
	if entry.Harness != "claude-code" {
		t.Fatalf("member harness should win over event harness, got %q", entry.Harness)
	}
	if members.gotRef != "sess-abc" {
		t.Fatalf("lookup used ref %q", members.gotRef)
	}
	if len(bus.events) != 1 || bus.events[0].EventType != eventbus.AttentionEventRaised {
		t.Fatalf("expected one raised event, got %+v", bus.events)
	}
}

func TestUnmatchedSessionIsKeptUnattributed(t *testing.T) {
	members := &fakeMembers{}
	bus := &capturingBus{}
	clock := &tickClock{t: time.Unix(1700000000, 0).UTC()}
	svc := newServiceForTest(t, members, bus, clock, DefaultTTL)

	entry, err := svc.Report(context.Background(), "user-1", Event{SessionRef: "sess-unknown", Harness: "codex", Kind: KindNeedsApproval})
	if err != nil {
		t.Fatalf("Report: %v", err)
	}
	if entry.MemberID != "" || entry.ProjectID != "" {
		t.Fatalf("expected unattributed entry, got %+v", entry)
	}
	// No project room to publish into, but it must still be listed everywhere.
	if len(bus.events) != 0 {
		t.Fatalf("unattributed entries must not publish, got %+v", bus.events)
	}
	listed := svc.List(context.Background(), "any-project")
	if len(listed) != 1 || listed[0].SessionRef != "sess-unknown" {
		t.Fatalf("unattributed entry missing from list: %+v", listed)
	}
}

func TestClearedRemovesAndPublishes(t *testing.T) {
	members := &fakeMembers{records: []member.Record{{ID: "member-1", ProjectID: "proj-1", DisplayName: "Sora"}}}
	bus := &capturingBus{}
	clock := &tickClock{t: time.Unix(1700000000, 0).UTC()}
	svc := newServiceForTest(t, members, bus, clock, DefaultTTL)

	_, _ = svc.Report(context.Background(), "u", Event{SessionRef: "s1", Kind: KindWaiting})
	_, err := svc.Report(context.Background(), "u", Event{SessionRef: "s1", Kind: KindCleared})
	if err != nil {
		t.Fatalf("clear: %v", err)
	}
	if got := svc.List(context.Background(), "proj-1"); len(got) != 0 {
		t.Fatalf("expected empty list after clear, got %+v", got)
	}
	if len(bus.events) != 2 || bus.events[1].EventType != eventbus.AttentionEventCleared {
		t.Fatalf("expected raised then cleared, got %+v", bus.events)
	}
	// Clearing an unknown session is a quiet no-op.
	if _, err := svc.Report(context.Background(), "u", Event{SessionRef: "never-seen", Kind: KindCleared}); err != nil {
		t.Fatalf("clear unknown: %v", err)
	}
	if len(bus.events) != 2 {
		t.Fatalf("clearing unknown session must not publish, got %+v", bus.events)
	}
}

func TestSameKindRefreshKeepsSince(t *testing.T) {
	members := &fakeMembers{records: []member.Record{{ID: "m", ProjectID: "p"}}}
	bus := &capturingBus{}
	clock := &tickClock{t: time.Unix(1700000000, 0).UTC()}
	svc := newServiceForTest(t, members, bus, clock, DefaultTTL)

	first, _ := svc.Report(context.Background(), "u", Event{SessionRef: "s1", Kind: KindWaiting})
	clock.t = clock.t.Add(30 * time.Minute)
	refreshed, _ := svc.Report(context.Background(), "u", Event{SessionRef: "s1", Kind: KindWaiting})
	if !refreshed.Since.Equal(first.Since) {
		t.Fatalf("refresh reset Since: %v -> %v", first.Since, refreshed.Since)
	}
	// A kind change is a new wait: Since resets.
	clock.t = clock.t.Add(time.Minute)
	escalated, _ := svc.Report(context.Background(), "u", Event{SessionRef: "s1", Kind: KindNeedsApproval})
	if escalated.Since.Equal(first.Since) {
		t.Fatal("kind change should reset Since")
	}
}

func TestTTLExpiresStaleEntries(t *testing.T) {
	members := &fakeMembers{records: []member.Record{{ID: "m", ProjectID: "p"}}}
	bus := &capturingBus{}
	clock := &tickClock{t: time.Unix(1700000000, 0).UTC()}
	svc := newServiceForTest(t, members, bus, clock, time.Hour)

	_, _ = svc.Report(context.Background(), "u", Event{SessionRef: "s1", Kind: KindWaiting})
	clock.t = clock.t.Add(59 * time.Minute)
	if got := svc.List(context.Background(), "p"); len(got) != 1 {
		t.Fatalf("entry expired too early: %+v", got)
	}
	clock.t = clock.t.Add(2 * time.Minute)
	if got := svc.List(context.Background(), "p"); len(got) != 0 {
		t.Fatalf("entry should have expired: %+v", got)
	}
}

func TestListScopesByProjectAndSortsNewestFirst(t *testing.T) {
	members := &fakeMembers{}
	bus := &capturingBus{}
	clock := &tickClock{t: time.Unix(1700000000, 0).UTC()}
	svc := newServiceForTest(t, members, bus, clock, DefaultTTL)

	members.records = []member.Record{{ID: "m1", ProjectID: "p1", DisplayName: "A"}}
	_, _ = svc.Report(context.Background(), "u", Event{SessionRef: "s1", Kind: KindWaiting})
	clock.t = clock.t.Add(time.Minute)
	members.records = []member.Record{{ID: "m2", ProjectID: "p2", DisplayName: "B"}}
	_, _ = svc.Report(context.Background(), "u", Event{SessionRef: "s2", Kind: KindWaiting})
	clock.t = clock.t.Add(time.Minute)
	members.records = []member.Record{{ID: "m3", ProjectID: "p1", DisplayName: "C"}}
	_, _ = svc.Report(context.Background(), "u", Event{SessionRef: "s3", Kind: KindWaiting})

	got := svc.List(context.Background(), "p1")
	if len(got) != 2 {
		t.Fatalf("expected 2 entries for p1, got %+v", got)
	}
	if got[0].MemberID != "m3" || got[1].MemberID != "m1" {
		t.Fatalf("expected newest-first [m3, m1], got [%s, %s]", got[0].MemberID, got[1].MemberID)
	}
}

func TestReportValidation(t *testing.T) {
	svc := newServiceForTest(t, &fakeMembers{}, &capturingBus{}, &tickClock{t: time.Unix(1700000000, 0).UTC()}, DefaultTTL)
	if _, err := svc.Report(context.Background(), "u", Event{Kind: KindWaiting}); err == nil {
		t.Fatal("expected error for missing sessionRef")
	}
	if _, err := svc.Report(context.Background(), "u", Event{SessionRef: "s", Kind: "bogus"}); err == nil {
		t.Fatal("expected error for invalid kind")
	}
}
