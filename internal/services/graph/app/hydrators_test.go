package app

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/tinoosan/agen8/internal/core/types"
	decisionapp "github.com/tinoosan/agen8/internal/services/decision/app"
	decisiondomain "github.com/tinoosan/agen8/internal/services/decision/domain"
	graphdomain "github.com/tinoosan/agen8/internal/services/graph/domain"
	taskdomain "github.com/tinoosan/agen8/internal/services/task/domain"
)

// noopDecisionDeps satisfies decisionapp.GraphLinkWriter,
// GraphLinkDeleter, and EventPublisher with no-ops, for tests
// that exercise the decision service but don't care about its side
// effects.
type noopDecisionDeps struct{}

func (noopDecisionDeps) Link(context.Context, graphdomain.GraphLinkRequest) (graphdomain.GraphEdge, []graphdomain.GraphWarning, error) {
	return graphdomain.GraphEdge{}, nil, nil
}
func (noopDecisionDeps) DeleteLinksForNode(context.Context, string, string) error { return nil }
func (noopDecisionDeps) Publish(string, any) error                                { return nil }

type stubTaskReader struct {
	byID  map[taskdomain.TaskID]taskdomain.Task
	tasks []taskdomain.Task
}

func (s stubTaskReader) Get(_ context.Context, taskID taskdomain.TaskID) (taskdomain.Task, error) {
	if task, ok := s.byID[taskID]; ok {
		return task, nil
	}
	return taskdomain.Task{}, taskdomain.ErrTaskNotFound
}

func (s stubTaskReader) List(_ context.Context, filter taskdomain.TaskFilter) ([]taskdomain.Task, error) {
	if filter.ProjectID == "" {
		return append([]taskdomain.Task(nil), s.tasks...), nil
	}
	out := make([]taskdomain.Task, 0, len(s.tasks))
	for _, task := range s.tasks {
		if task.ProjectID == filter.ProjectID {
			out = append(out, task)
		}
	}
	return out, nil
}

func TestTaskHydrator_FetchCoordinatorScopeCanReadCrossProjectTask(t *testing.T) {
	now := time.Now().UTC()
	other := taskdomain.Task{
		ID:          "task-project-b",
		ProjectID:   types.ProjectID("project-b"),
		Description: "Investigate prior work",
		Title:       "Investigate prior work",
		Status:      taskdomain.TaskStatusPending,
		CreatedAt:   &now,
	}
	reader := stubTaskReader{
		byID: map[taskdomain.TaskID]taskdomain.Task{
			"task-project-b": other,
		},
		tasks: []taskdomain.Task{other},
	}

	coordinatorHydrator := taskHydrator{tasks: reader}
	if _, err := coordinatorHydrator.Fetch(context.Background(), "", "task-project-b"); err != nil {
		t.Fatalf("Fetch with empty project scope: %v", err)
	}

	if _, err := coordinatorHydrator.Fetch(context.Background(), "project-a", "task-project-b"); err == nil {
		t.Fatal("expected project-scoped fetch to reject cross-project task")
	}
}

func TestTaskHydrator_FetchIncludesReadableActorLabels(t *testing.T) {
	now := time.Now().UTC()
	task := taskdomain.Task{
		ID:                   "task-readable-actors",
		ProjectID:            types.ProjectID("project-a"),
		Description:          "Make task actors readable",
		Title:                "Make task actors readable",
		Status:               taskdomain.TaskStatusActive,
		AssignedTo:           "member-worker",
		AssignedToLabel:      "Backend engineer",
		ClaimedByMemberID:    "member-reviewer",
		ClaimedByMemberLabel: "Reviewer",
		CreatedBy:            "member-coordinator",
		CreatedByLabel:       "Coordinator",
		CreatedAt:            &now,
	}
	reader := stubTaskReader{
		byID: map[taskdomain.TaskID]taskdomain.Task{
			"task-readable-actors": task,
		},
		tasks: []taskdomain.Task{task},
	}

	hydrator := taskHydrator{tasks: reader}
	node, err := hydrator.Fetch(context.Background(), "project-a", "task-readable-actors")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	for key, want := range map[string]string{
		"assigneeRef":          "member-worker",
		"assignedTo":           "member-worker",
		"assignedToLabel":      "Backend engineer",
		"claimedByMemberId":    "member-reviewer",
		"claimedByMemberLabel": "Reviewer",
		"createdBy":            "member-coordinator",
		"createdByLabel":       "Coordinator",
	} {
		if got, _ := node.Fields[key].(string); got != want {
			t.Fatalf("field %s=%q want %q; fields=%+v", key, got, want, node.Fields)
		}
	}
}

func TestTaskHydrator_SearchCoordinatorScopeListsTasksAcrossProjects(t *testing.T) {
	now := time.Now().UTC()
	taskA := taskdomain.Task{
		ID:          "task-project-a",
		ProjectID:   types.ProjectID("project-a"),
		Description: "Cross-project search A",
		Title:       "Cross-project search A",
		Status:      taskdomain.TaskStatusPending,
		CreatedAt:   &now,
	}
	taskB := taskdomain.Task{
		ID:          "task-project-b",
		ProjectID:   types.ProjectID("project-b"),
		Description: "Cross-project search B",
		Title:       "Cross-project search B",
		Status:      taskdomain.TaskStatusPending,
		CreatedAt:   &now,
	}
	reader := stubTaskReader{
		byID: map[taskdomain.TaskID]taskdomain.Task{
			"task-project-a": taskA,
			"task-project-b": taskB,
		},
		tasks: []taskdomain.Task{taskA, taskB},
	}

	coordinatorHydrator := taskHydrator{tasks: reader}
	results, err := coordinatorHydrator.Search(context.Background(), "", "cross-project", 10)
	if err != nil {
		t.Fatalf("Search with empty project scope: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("results=%d want 2", len(results))
	}
}

type stubRepository struct {
	byID map[decisiondomain.DecisionID]decisiondomain.Decision
}

func (s stubRepository) CreateDecision(context.Context, decisiondomain.Decision) error { return nil }
func (s stubRepository) GetDecision(_ context.Context, id decisiondomain.DecisionID) (decisiondomain.Decision, error) {
	if decision, ok := s.byID[id]; ok {
		return decision, nil
	}
	return decisiondomain.Decision{}, fmt.Errorf("decision not found")
}
func (s stubRepository) DeleteDecision(context.Context, decisiondomain.DecisionID) error {
	return nil
}
func (s stubRepository) ListDecisions(context.Context, decisiondomain.DecisionFilter) ([]decisiondomain.Decision, error) {
	return nil, nil
}
func (s stubRepository) ListDecisionsByKeyResult(context.Context, string) ([]decisiondomain.Decision, error) {
	return nil, nil
}
func (s stubRepository) CountDecisions(context.Context, decisiondomain.DecisionFilter) (int, error) {
	return 0, nil
}
func (s stubRepository) StatsDecisions(context.Context, decisiondomain.DecisionFilter) (decisiondomain.DecisionStats, error) {
	return decisiondomain.DecisionStats{}, nil
}
func (s stubRepository) ExportDecisions(context.Context, decisiondomain.DecisionFilter) ([]decisiondomain.Decision, error) {
	return nil, nil
}
func (s stubRepository) DecisionExistsByFingerprint(context.Context, string, string, string, string, time.Time) (bool, error) {
	return false, nil
}

func TestDecisionHydrator_FetchIncludesInvalidationConditions(t *testing.T) {
	repo := stubRepository{byID: map[decisiondomain.DecisionID]decisiondomain.Decision{
		"dec-1": {
			ID:         "dec-1",
			ProjectID:  "proj-1",
			Source:     decisiondomain.DecisionSourceAgent,
			Title:      "Prioritize metered pricing",
			Confidence: 0.8,
			CreatedAt:  time.Now().UTC(),
			Log: &decisiondomain.LogPayload{
				Rationale:              "It tests willingness to pay.",
				Context:                "Customers asked for usage-based billing during onboarding.",
				InvalidationConditions: []string{"Conversion drops below baseline", "Metering error rate exceeds 1%"},
			},
		},
	}}
	stub := noopDecisionDeps{}
	decisionSvc, err := decisionapp.NewService(repo, decisiondomain.SystemClock{}, stub, stub, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	node, err := (decisionHydrator{decisions: decisionSvc}).Fetch(context.Background(), "proj-1", "dec-1")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	got, ok := node.Fields["invalidationConditions"].([]string)
	if !ok {
		t.Fatalf("invalidationConditions type=%T value=%v", node.Fields["invalidationConditions"], node.Fields["invalidationConditions"])
	}
	if len(got) != 2 || got[0] != "Conversion drops below baseline" {
		t.Fatalf("invalidationConditions=%v", got)
	}
	if node.Fields["context"] != "Customers asked for usage-based billing during onboarding." {
		t.Fatalf("context=%q", node.Fields["context"])
	}
}
