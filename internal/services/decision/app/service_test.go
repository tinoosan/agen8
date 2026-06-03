package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/tinoosan/agen8-mcp-server/internal/eventbus"
	"github.com/tinoosan/agen8-mcp-server/internal/services/decision/domain"
	graphdomain "github.com/tinoosan/agen8-mcp-server/internal/services/graph/domain"
)

// ── Mock repository ─────────────────────────────────────────────────────

type mockDecisionRepo struct {
	saved   []domain.Decision
	deleted []domain.DecisionID
}

func (m *mockDecisionRepo) CreateDecision(_ context.Context, d domain.Decision) error {
	m.saved = append(m.saved, d)
	return nil
}

func (m *mockDecisionRepo) GetDecision(_ context.Context, id domain.DecisionID) (domain.Decision, error) {
	for _, d := range m.saved {
		if d.ID == id {
			return d, nil
		}
	}
	return domain.Decision{}, fmt.Errorf("not found: %s", id)
}

func (m *mockDecisionRepo) DeleteDecision(_ context.Context, id domain.DecisionID) error {
	for i, d := range m.saved {
		if d.ID == id {
			m.saved = append(m.saved[:i], m.saved[i+1:]...)
			m.deleted = append(m.deleted, id)
			return nil
		}
	}
	return fmt.Errorf("not found: %s", id)
}

func (m *mockDecisionRepo) ListDecisions(_ context.Context, filter domain.DecisionFilter) ([]domain.Decision, error) {
	var out []domain.Decision
	for _, d := range m.saved {
		if d.ProjectID != filter.ProjectID {
			continue
		}
		if len(filter.Sources) > 0 {
			matched := false
			for _, s := range filter.Sources {
				if d.Source == s {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
		}
		if filter.SpaceID != "" && d.SpaceID != filter.SpaceID {
			continue
		}
		out = append(out, d)
	}
	return out, nil
}

func (m *mockDecisionRepo) ListDecisionsByKeyResult(_ context.Context, keyResultRef string) ([]domain.Decision, error) {
	var out []domain.Decision
	for _, d := range m.saved {
		if d.KeyResultRef == keyResultRef {
			out = append(out, d)
		}
	}
	return out, nil
}

func (m *mockDecisionRepo) CountDecisions(_ context.Context, filter domain.DecisionFilter) (int, error) {
	results, err := m.ListDecisions(context.Background(), filter)
	if err != nil {
		return 0, err
	}
	return len(results), nil
}

func (m *mockDecisionRepo) ExportDecisions(_ context.Context, filter domain.DecisionFilter) ([]domain.Decision, error) {
	return m.ListDecisions(context.Background(), filter)
}
func (m *mockDecisionRepo) DecisionExistsByFingerprint(_ context.Context, projectID, sourceIdentity, title, taskRef string, since time.Time) (bool, error) {
	for _, d := range m.saved {
		if d.ProjectID == projectID &&
			strings.TrimSpace(d.SourceIdentity) == strings.TrimSpace(sourceIdentity) &&
			strings.TrimSpace(d.Title) == strings.TrimSpace(title) &&
			strings.TrimSpace(d.TaskRef) == strings.TrimSpace(taskRef) &&
			!d.CreatedAt.Before(since) {
			return true, nil
		}
	}
	return false, nil
}

// ── Test callbacks ──────────────────────────────────────────────────────

type testCallbacks struct {
	links        []graphdomain.GraphLinkRequest
	deletedLinks []string
	topics       []string
	events       []eventbus.DecisionLoggedEvent
	publishErr   error
}

func (cb *testCallbacks) Link(_ context.Context, req graphdomain.GraphLinkRequest) (graphdomain.GraphEdge, []graphdomain.GraphWarning, error) {
	cb.links = append(cb.links, req)
	return graphdomain.GraphEdge{
		SourceType: req.SourceType,
		SourceID:   req.SourceID,
		TargetType: req.TargetType,
		TargetID:   req.TargetID,
		EdgeType:   req.EdgeType,
	}, []graphdomain.GraphWarning{}, nil
}

func (cb *testCallbacks) DeleteLinksForNode(_ context.Context, nodeType, nodeID string) error {
	cb.deletedLinks = append(cb.deletedLinks, nodeType+"/"+nodeID)
	return nil
}

func (cb *testCallbacks) Publish(topic string, event any) error {
	cb.topics = append(cb.topics, topic)
	if cb.publishErr != nil {
		return cb.publishErr
	}
	decisionEvent, ok := event.(eventbus.DecisionLoggedEvent)
	if !ok {
		return fmt.Errorf("unexpected event type %T", event)
	}
	cb.events = append(cb.events, decisionEvent)
	return nil
}

type testRefResolver struct {
	taskKeyResult    func(context.Context, string) (string, error)
	keyResultMission func(context.Context, string) (string, error)
}

func (r testRefResolver) TaskKeyResult(ctx context.Context, taskID string) (string, error) {
	if r.taskKeyResult == nil {
		return "", nil
	}
	return r.taskKeyResult(ctx, taskID)
}

func (r testRefResolver) KeyResultMission(ctx context.Context, keyResultID string) (string, error) {
	if r.keyResultMission == nil {
		return "", nil
	}
	return r.keyResultMission(ctx, keyResultID)
}

func setupService() (*Service, *mockDecisionRepo, *testCallbacks) {
	repo := &mockDecisionRepo{}
	cb := &testCallbacks{}
	svc, err := NewService(repo, domain.SystemClock{}, cb, cb, cb, nil, nil, nil, nil)
	if err != nil {
		panic(err)
	}
	return svc, repo, cb
}

func validDecision() domain.Decision {
	return domain.Decision{
		ProjectID:  "proj-1",
		SpaceID:    "space-1",
		Source:     domain.DecisionSourceAgent,
		Title:      "Use Redis for caching",
		Confidence: 0.9,
		Log:        &domain.LogPayload{Rationale: "Lower latency than DB queries"},
	}
}

// ── Create tests ────────────────────────────────────────────────────────

func TestCreate_Basic(t *testing.T) {
	svc, repo, cb := setupService()
	d, err := svc.Create(context.Background(), validDecision())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if d.ID == "" {
		t.Error("expected generated ID")
	}
	if len(repo.saved) != 1 {
		t.Fatalf("expected 1 saved, got %d", len(repo.saved))
	}
	if len(cb.events) != 1 || cb.events[0].Title != "Use Redis for caching" {
		t.Error("expected 1 decision.logged event")
	}
}

func TestLog_PersistsInvalidationConditions(t *testing.T) {
	svc, repo, _ := setupService()

	result, err := svc.Log(context.Background(), LogRequest{
		ProjectID:              "proj-1",
		SpaceID:                "space-1",
		MemberID:               "member-cfo",
		Title:                  "Prioritize metered pricing",
		Rationale:              "It tests willingness to pay and billing feasibility.",
		AlternativesRejected:   "Flat-only pricing",
		InvalidationConditions: []string{"Conversion drops below baseline", "Metering error rate exceeds 1%"},
		Confidence:             0.85,
	})
	if err != nil {
		t.Fatalf("Log: %v", err)
	}
	if result.ID == "" {
		t.Fatal("expected result id")
	}
	if len(repo.saved) != 1 {
		t.Fatalf("saved=%d want 1", len(repo.saved))
	}
	log := repo.saved[0].Log
	if log == nil {
		t.Fatalf("expected Log payload, got nil")
	}
	if strings.Join(log.InvalidationConditions, "|") != "Conversion drops below baseline|Metering error rate exceeds 1%" {
		t.Fatalf("InvalidationConditions=%v", log.InvalidationConditions)
	}
}

func TestCreate_KeyResultRef_CreatesServesLink(t *testing.T) {
	svc, _, cb := setupService()
	d := validDecision()
	d.KeyResultRef = "kr-42"

	svc.Create(context.Background(), d)

	servesLinks := 0
	for _, l := range cb.links {
		if l.EdgeType == "serves" && l.TargetID == "kr-42" {
			servesLinks++
		}
	}
	if servesLinks != 1 {
		t.Errorf("expected 1 serves link to kr-42, got %d", servesLinks)
	}
}

func TestCreate_TaskRef_CreatesMadeDuringLink(t *testing.T) {
	svc, _, cb := setupService()
	d := validDecision()
	d.TaskRef = "task-99"

	svc.Create(context.Background(), d)

	madeDuringLinks := 0
	for _, l := range cb.links {
		if l.EdgeType == "made_during" && l.TargetID == "task-99" {
			madeDuringLinks++
		}
	}
	if madeDuringLinks != 1 {
		t.Errorf("expected 1 made_during link to task-99, got %d", madeDuringLinks)
	}
}

func TestCreate_PlanRef_CreatesRelatesToLink(t *testing.T) {
	svc, _, cb := setupService()
	d := validDecision()
	d.PlanRef = "plan-42"

	svc.Create(context.Background(), d)

	planLinks := 0
	for _, l := range cb.links {
		if l.EdgeType == "relates_to" && l.TargetType == "plan" && l.TargetID == "plan-42" {
			planLinks++
		}
	}
	if planLinks != 1 {
		t.Errorf("expected 1 relates_to link to plan-42, got %d", planLinks)
	}
}

func TestCreate_BothRefs_CreatesBothLinks(t *testing.T) {
	svc, _, cb := setupService()
	d := validDecision()
	d.KeyResultRef = "kr-1"
	d.TaskRef = "task-1"

	svc.Create(context.Background(), d)

	if len(cb.links) != 2 {
		t.Fatalf("expected 2 links, got %d", len(cb.links))
	}
}

func TestCreate_TaskRef_ResolvesKeyResultAndMissionLinks(t *testing.T) {
	repo := &mockDecisionRepo{}
	cb := &testCallbacks{}
	refs := testRefResolver{
		taskKeyResult: func(_ context.Context, taskID string) (string, error) {
			if taskID != "task-1" {
				t.Fatalf("taskID=%q want task-1", taskID)
			}
			return "kr-1", nil
		},
		keyResultMission: func(_ context.Context, keyResultID string) (string, error) {
			if keyResultID != "kr-1" {
				t.Fatalf("keyResultID=%q want kr-1", keyResultID)
			}
			return "mission-1", nil
		},
	}
	svc, err := NewService(repo, domain.SystemClock{}, cb, cb, cb, refs, refs, nil, nil)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	d := validDecision()
	d.TaskRef = "task-1"

	created, err := svc.Create(context.Background(), d)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.KeyResultRef != "kr-1" {
		t.Fatalf("KeyResultRef=%q want kr-1", created.KeyResultRef)
	}
	if created.MissionRef != "mission-1" {
		t.Fatalf("MissionRef=%q want mission-1", created.MissionRef)
	}
	if repo.saved[0].KeyResultRef != "kr-1" || repo.saved[0].MissionRef != "mission-1" {
		t.Fatalf("saved refs keyResult=%q mission=%q", repo.saved[0].KeyResultRef, repo.saved[0].MissionRef)
	}

	want := map[string]bool{
		"key_result/kr-1/" + "serves":   false,
		"mission/mission-1/" + "serves": false,
		"task/task-1/" + "made_during":  false,
	}
	for _, link := range cb.links {
		key := link.TargetType + "/" + link.TargetID + "/" + link.EdgeType
		if _, ok := want[key]; ok {
			want[key] = true
		}
	}
	for key, seen := range want {
		if !seen {
			t.Fatalf("missing context link %s in %#v", key, cb.links)
		}
	}
}

func TestDelete_RemovesDecisionAndContextLinks(t *testing.T) {
	svc, repo, cb := setupService()
	d := validDecision()
	d.ID = "dec-delete-me"
	if _, err := svc.Create(context.Background(), d); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := svc.Delete(context.Background(), "dec-delete-me"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if len(repo.deleted) != 1 || repo.deleted[0] != "dec-delete-me" {
		t.Fatalf("deleted=%v want [dec-delete-me]", repo.deleted)
	}
	if len(cb.deletedLinks) != 1 || cb.deletedLinks[0] != "decision/dec-delete-me" {
		t.Fatalf("deletedLinks=%v want [decision/dec-delete-me]", cb.deletedLinks)
	}
}

func TestDelete_RejectsMissingID(t *testing.T) {
	svc, _, _ := setupService()
	if err := svc.Delete(context.Background(), " "); err == nil {
		t.Fatal("expected missing id error")
	}
}

func TestCreate_NoRefs_NoLinks(t *testing.T) {
	svc, _, cb := setupService()
	svc.Create(context.Background(), validDecision())

	if len(cb.links) != 0 {
		t.Errorf("expected 0 links without refs, got %d", len(cb.links))
	}
}

func TestCreate_ValidationError(t *testing.T) {
	svc, _, _ := setupService()
	d := validDecision()
	d.Title = "" // required

	_, err := svc.Create(context.Background(), d)
	if err == nil {
		t.Error("expected validation error for empty title")
	}
}

func TestCreate_InvalidConfidence(t *testing.T) {
	svc, _, _ := setupService()
	d := validDecision()
	d.Confidence = 1.5

	_, err := svc.Create(context.Background(), d)
	if err == nil {
		t.Error("expected validation error for confidence > 1.0")
	}
}

func TestNewService_RejectsNilDependencies(t *testing.T) {
	cb := &testCallbacks{}
	repo := &mockDecisionRepo{}
	cases := []struct {
		name        string
		repo        domain.Repository
		clock       domain.Clock
		links       GraphLinkWriter
		linkDeleter GraphLinkDeleter
		events      EventPublisher
		want        string
	}{
		{
			name: "nil repo",
			repo: nil, clock: domain.SystemClock{}, links: cb, linkDeleter: cb, events: cb,
			want: "repository is required",
		},
		{
			name: "nil clock",
			repo: repo, clock: nil, links: cb, linkDeleter: cb, events: cb,
			want: "clock is required",
		},
		{
			name: "nil links",
			repo: repo, clock: domain.SystemClock{}, links: nil, linkDeleter: cb, events: cb,
			want: "graph link writer is required",
		},
		{
			name: "nil delete links",
			repo: repo, clock: domain.SystemClock{}, links: cb, linkDeleter: nil, events: cb,
			want: "graph link deleter is required",
		},
		{
			name: "nil publisher",
			repo: repo, clock: domain.SystemClock{}, links: cb, linkDeleter: cb, events: nil,
			want: "event publisher is required",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewService(tc.repo, tc.clock, tc.links, tc.linkDeleter, tc.events, nil, nil, nil, nil)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err=%v want %q", err, tc.want)
			}
		})
	}
}

func TestCreate_DuplicateWithinWindowIsDeduped(t *testing.T) {
	svc, repo, cb := setupService()
	d := validDecision()
	d.SourceIdentity = "cfo"
	d.TaskRef = "task-42"

	// First call — should save.
	first, err := svc.Create(context.Background(), d)
	if err != nil {
		t.Fatalf("first Create: %v", err)
	}
	if len(repo.saved) != 1 {
		t.Fatalf("expected 1 saved after first, got %d", len(repo.saved))
	}

	// Second call with identical fingerprint — should be deduped.
	d2 := validDecision()
	d2.SourceIdentity = "cfo"
	d2.TaskRef = "task-42"
	second, err := svc.Create(context.Background(), d2)
	if err != nil {
		t.Fatalf("second Create: %v", err)
	}
	if len(repo.saved) != 1 {
		t.Fatalf("expected still 1 saved after dedup, got %d", len(repo.saved))
	}
	// Deduped call should return a decision (not error), but not create a new one.
	if second.Title != first.Title {
		t.Errorf("deduped decision title = %q, want %q", second.Title, first.Title)
	}
	// Only 1 event should have been published (from the first Create).
	if len(cb.events) != 1 {
		t.Errorf("expected 1 event (not 2), got %d", len(cb.events))
	}
}

func TestCreate_DifferentTitleNotDeduped(t *testing.T) {
	svc, repo, _ := setupService()
	d := validDecision()
	d.SourceIdentity = "cfo"
	d.TaskRef = "task-42"

	if _, err := svc.Create(context.Background(), d); err != nil {
		t.Fatalf("first Create: %v", err)
	}

	// Different title — should NOT be deduped.
	d2 := validDecision()
	d2.SourceIdentity = "cfo"
	d2.TaskRef = "task-42"
	d2.Title = "A completely different decision"
	if _, err := svc.Create(context.Background(), d2); err != nil {
		t.Fatalf("second Create: %v", err)
	}
	if len(repo.saved) != 2 {
		t.Fatalf("expected 2 saved (different titles), got %d", len(repo.saved))
	}
}

func TestCreate_EventContainsDecisionID(t *testing.T) {
	svc, _, cb := setupService()
	d, err := svc.Create(context.Background(), validDecision())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if len(cb.events) != 1 {
		t.Fatalf("expected 1 event")
	}
	if cb.events[0].DecisionID != string(d.ID) {
		t.Errorf("event decisionID = %q, want %q", cb.events[0].DecisionID, d.ID)
	}
}

func TestCreate_ReturnsErrorWhenEventPublishFails(t *testing.T) {
	repo := &mockDecisionRepo{}
	cb := &testCallbacks{publishErr: fmt.Errorf("bus down")}
	svc, err := NewService(repo, domain.SystemClock{}, cb, cb, cb, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	_, err = svc.Create(context.Background(), validDecision())
	if err == nil {
		t.Fatal("expected publish error")
	}
	if !strings.Contains(err.Error(), "publish decision logged event") {
		t.Fatalf("error = %v, want publish context", err)
	}
	if len(repo.saved) != 1 {
		t.Fatalf("saved=%d want 1", len(repo.saved))
	}
	if len(cb.topics) != 1 || cb.topics[0] != eventbus.TopicDecisionLogged {
		t.Fatalf("topics=%v want decision.logged", cb.topics)
	}
}

// ── Query filter tests ──────────────────────────────────────────────────

func TestList_FiltersProjectID(t *testing.T) {
	svc, _, _ := setupService()
	d1 := validDecision()
	d1.ProjectID = "proj-1"
	d2 := validDecision()
	d2.ProjectID = "proj-2"
	d2.Title = "Different decision"

	if _, err := svc.Create(context.Background(), d1); err != nil {
		t.Fatalf("Create d1: %v", err)
	}
	if _, err := svc.Create(context.Background(), d2); err != nil {
		t.Fatalf("Create d2: %v", err)
	}

	results, err := svc.List(context.Background(), domain.DecisionFilter{ProjectID: "proj-1"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result for proj-1, got %d", len(results))
	}
	if results[0].ProjectID != "proj-1" {
		t.Errorf("expected proj-1, got %q", results[0].ProjectID)
	}
}

func TestListByKeyResult_FiltersRef(t *testing.T) {
	svc, _, _ := setupService()
	d1 := validDecision()
	d1.KeyResultRef = "kr-1"
	d2 := validDecision()
	d2.KeyResultRef = "kr-2"
	d2.Title = "Another"

	if _, err := svc.Create(context.Background(), d1); err != nil {
		t.Fatalf("Create d1: %v", err)
	}
	if _, err := svc.Create(context.Background(), d2); err != nil {
		t.Fatalf("Create d2: %v", err)
	}

	results, err := svc.ListByKeyResult(context.Background(), "kr-1")
	if err != nil {
		t.Fatalf("ListDecisionsByKeyResult: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result for kr-1, got %d", len(results))
	}
	if results[0].KeyResultRef != "kr-1" {
		t.Errorf("expected kr-1, got %q", results[0].KeyResultRef)
	}
}

func TestCount_FiltersProjectID(t *testing.T) {
	svc, _, _ := setupService()
	d1 := validDecision()
	d1.ProjectID = "proj-1"
	d2 := validDecision()
	d2.ProjectID = "proj-2"
	d2.Title = "Different"

	if _, err := svc.Create(context.Background(), d1); err != nil {
		t.Fatalf("Create d1: %v", err)
	}
	if _, err := svc.Create(context.Background(), d2); err != nil {
		t.Fatalf("Create d2: %v", err)
	}

	count, err := svc.Count(context.Background(), domain.DecisionFilter{ProjectID: "proj-1"})
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if count != 1 {
		t.Errorf("expected count 1 for proj-1, got %d", count)
	}
}

// ── Event bus integration tests ──────────────────────────────────────────

func TestCreate_PublishesEventToRealBus(t *testing.T) {
	bus := eventbus.New(nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch, err := bus.Subscribe(ctx, eventbus.TopicDecisionLogged)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	repo := &mockDecisionRepo{}
	cb := &testCallbacks{}
	svc, err := NewService(repo, domain.SystemClock{}, cb, cb, bus, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	d := validDecision()
	created, err := svc.Create(context.Background(), d)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	select {
	case msg := <-ch:
		msg.Ack()
		var received eventbus.DecisionLoggedEvent
		if err := json.Unmarshal(msg.Payload, &received); err != nil {
			t.Fatalf("unmarshal event: %v", err)
		}
		if received.DecisionID != string(created.ID) {
			t.Errorf("event decisionID = %q, want %q", received.DecisionID, created.ID)
		}
		if received.Title != d.Title {
			t.Errorf("event title = %q, want %q", received.Title, d.Title)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for event on bus")
	}
}
