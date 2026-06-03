package rpc

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	spacedomain "github.com/tinoosan/agen8-mcp-server/internal/services/space/domain"

	"github.com/google/uuid"
	graphdomain "github.com/tinoosan/agen8-mcp-server/internal/services/graph/domain"
	messagedomain "github.com/tinoosan/agen8-mcp-server/internal/services/message/domain"
	"github.com/tinoosan/agen8-mcp-server/internal/services/space/domain/member"
	taskapp "github.com/tinoosan/agen8-mcp-server/internal/services/task/app"
	"github.com/tinoosan/agen8-mcp-server/internal/services/task/domain"
	"github.com/tinoosan/agen8-mcp-server/pkg/types"
)

var handlerTestNow = time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)

func TestHandlerCreateCallsTaskService(t *testing.T) {
	handler, _, publisher := newHandlerForTest(t, member.ID("member-coordinator"), newHandlerTaskRepo())
	phaseID := "11111111-1111-4111-8111-111111111111"
	todoID := "22222222-2222-4222-8222-222222222222"

	result, err := handler.Create(context.Background(), TaskCreateParams{
		SpaceID:            "space-1",
		AssignedTo:         "member-worker",
		Description:        "write a report",
		AcceptanceCriteria: []string{"tests pass"},
		Title:              "Report",
		TaskKind:           "research",
		KeyResultRef:       "kr-1",
		MissionRef:         "mission-1",
		PlanPhaseID:        phaseID,
		PlanTodoID:         todoID,
		Metadata:           map[string]any{"history": []any{"created"}},
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if result.Task.ID == "" {
		t.Fatal("created task id is empty")
	}
	if result.Task.SpaceID != "space-1" || result.Task.AssignedTo != "member-worker" {
		t.Fatalf("task routing = %q/%q", result.Task.SpaceID, result.Task.AssignedTo)
	}
	if result.Task.PlanPhaseID != phaseID || result.Task.PlanTodoID != todoID {
		t.Fatalf("plan refs = %q/%q", result.Task.PlanPhaseID, result.Task.PlanTodoID)
	}
	if result.Task.MissionRef != "mission-1" {
		t.Fatalf("missionRef=%q want mission-1", result.Task.MissionRef)
	}
	if len(publisher.messages) != 1 {
		t.Fatalf("published messages=%d want 1", len(publisher.messages))
	}
}

func TestHandlerCreateRejectsMissingDescription(t *testing.T) {
	handler, _, _ := newHandlerForTest(t, member.ID("member-coordinator"), newHandlerTaskRepo())

	_, err := handler.Create(context.Background(), TaskCreateParams{
		SpaceID:    "space-1",
		AssignedTo: "member-worker",
	})
	if err == nil || !strings.Contains(err.Error(), "description is required") {
		t.Fatalf("Create error=%v want description required", err)
	}
}

func TestHandlerGetReturnsTaskView(t *testing.T) {
	handler, _, _ := newHandlerForTest(t, member.ID("member-coordinator"), newHandlerTaskRepo(handlerTask()))

	result, err := handler.Get(context.Background(), TaskGetParams{TaskID: "task-1"})
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if result.Task.ID != "task-1" || result.Task.Description != "write a report" {
		t.Fatalf("task view=%+v", result.Task)
	}
}

func TestHandlerGetRejectsMissingTaskID(t *testing.T) {
	handler, _, _ := newHandlerForTest(t, member.ID("member-coordinator"), newHandlerTaskRepo())

	_, err := handler.Get(context.Background(), TaskGetParams{})
	if err == nil || !strings.Contains(err.Error(), "taskId is required") {
		t.Fatalf("Get error=%v want taskId required", err)
	}
}

func TestHandlerListBuildsServiceFilter(t *testing.T) {
	task := handlerTask()
	phaseID := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	todoID := uuid.MustParse("22222222-2222-4222-8222-222222222222")
	task.PlanPhaseID = &phaseID
	task.PlanTodoID = &todoID
	repo := newHandlerTaskRepo(task)
	handler, repo, _ := newHandlerForTest(t, member.ID("member-coordinator"), repo)

	result, err := handler.List(context.Background(), TaskListParams{
		SpaceID:     "space-1",
		AssignedTo:  "member-worker",
		TaskKind:    "research",
		Status:      []string{"pending"},
		PlanPhaseID: phaseID.String(),
		PlanTodoID:  todoID.String(),
		Limit:       25,
		Offset:      5,
		SortBy:      "updatedAt",
		SortDesc:    true,
	})
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(result.Tasks) != 1 || result.TotalCount != 1 {
		t.Fatalf("list result=%+v", result)
	}
	if repo.lastListFilter.SpaceID != "space-1" || repo.lastListFilter.AssignedTo != "member-worker" {
		t.Fatalf("filter routing=%+v", repo.lastListFilter)
	}
	if repo.lastListFilter.PlanPhaseID == nil || *repo.lastListFilter.PlanPhaseID != phaseID {
		t.Fatalf("filter planPhaseId=%v want %s", repo.lastListFilter.PlanPhaseID, phaseID)
	}
	if repo.lastListFilter.PlanTodoID == nil || *repo.lastListFilter.PlanTodoID != todoID {
		t.Fatalf("filter planTodoId=%v want %s", repo.lastListFilter.PlanTodoID, todoID)
	}
	if repo.lastListFilter.Limit != 25 || repo.lastListFilter.Offset != 5 || repo.lastListFilter.SortBy != "updatedAt" || !repo.lastListFilter.SortDesc {
		t.Fatalf("filter paging=%+v", repo.lastListFilter)
	}
}

func TestHandlerListRejectsInvalidPlanPhaseID(t *testing.T) {
	handler, _, _ := newHandlerForTest(t, member.ID("member-coordinator"), newHandlerTaskRepo())

	_, err := handler.List(context.Background(), TaskListParams{PlanPhaseID: "not-a-uuid"})
	if err == nil || !strings.Contains(err.Error(), "planPhaseId must be a valid UUID") {
		t.Fatalf("List error=%v want invalid plan phase", err)
	}
}

func TestHandlerUpdateCallsTaskService(t *testing.T) {
	handler, repo, _ := newHandlerForTest(t, member.ID("member-coordinator"), newHandlerTaskRepo(handlerTask()))
	description := "write a revised report"
	title := "Revised"
	taskKind := "implementation"

	result, err := handler.Update(context.Background(), TaskUpdateParams{
		TaskID:      "task-1",
		Title:       &title,
		Description: &description,
		TaskKind:    &taskKind,
		Metadata:    map[string]any{"history": []any{"updated"}},
	})
	if err != nil {
		t.Fatalf("Update returned error: %v", err)
	}
	if result.Task.Title != title || result.Task.Description != description || result.Task.TaskKind != taskKind {
		t.Fatalf("updated task=%+v", result.Task)
	}
	stored := repo.tasks["task-1"]
	if stored.UpdatedAt == nil || !stored.UpdatedAt.Equal(handlerTestNow) {
		t.Fatalf("stored updatedAt=%v want %s", stored.UpdatedAt, handlerTestNow)
	}
}

func TestHandlerUpdateRejectsMissingTaskID(t *testing.T) {
	handler, _, _ := newHandlerForTest(t, member.ID("member-coordinator"), newHandlerTaskRepo(handlerTask()))

	_, err := handler.Update(context.Background(), TaskUpdateParams{})
	if err == nil || !strings.Contains(err.Error(), "taskId is required") {
		t.Fatalf("Update error=%v want taskId required", err)
	}
}

func TestHandlerCancelCallsTaskService(t *testing.T) {
	handler, repo, publisher := newHandlerForTest(t, member.ID("member-coordinator"), newHandlerTaskRepo(handlerTask()))

	result, err := handler.Cancel(context.Background(), TaskCancelParams{
		TaskID: "task-1",
		Reason: "no longer needed",
	})
	if err != nil {
		t.Fatalf("Cancel returned error: %v", err)
	}
	if result.Task.Status != string(domain.TaskStatusCanceled) {
		t.Fatalf("cancelled status=%q want canceled", result.Task.Status)
	}
	if result.Task.Error != "no longer needed" {
		t.Fatalf("cancelled reason=%q want %q", result.Task.Error, "no longer needed")
	}
	stored := repo.tasks["task-1"]
	if stored.Status != domain.TaskStatusCanceled {
		t.Fatalf("stored status=%q want canceled", stored.Status)
	}
	if len(publisher.messages) != 1 {
		t.Fatalf("published messages=%d want 1", len(publisher.messages))
	}
}

func TestHandlerCancelRejectsMissingTaskID(t *testing.T) {
	handler, _, _ := newHandlerForTest(t, member.ID("member-coordinator"), newHandlerTaskRepo(handlerTask()))

	_, err := handler.Cancel(context.Background(), TaskCancelParams{Reason: "x"})
	if err == nil || !strings.Contains(err.Error(), "taskId is required") {
		t.Fatalf("Cancel error=%v want taskId required", err)
	}
}

func TestHandlerCancelRejectsMissingReason(t *testing.T) {
	handler, _, _ := newHandlerForTest(t, member.ID("member-coordinator"), newHandlerTaskRepo(handlerTask()))

	_, err := handler.Cancel(context.Background(), TaskCancelParams{TaskID: "task-1"})
	if err == nil || !strings.Contains(err.Error(), "reason is required") {
		t.Fatalf("Cancel error=%v want reason required", err)
	}
}

type handlerTaskRepo struct {
	tasks           map[string]domain.Task
	lastListFilter  domain.TaskFilter
	lastCountFilter domain.TaskFilter
}

func newHandlerTaskRepo(tasks ...domain.Task) *handlerTaskRepo {
	repo := &handlerTaskRepo{tasks: map[string]domain.Task{}}
	for _, task := range tasks {
		repo.tasks[string(task.ID)] = task
	}
	return repo
}

func (r *handlerTaskRepo) CreateTask(_ context.Context, task domain.Task) error {
	if task.ID == "" {
		return fmt.Errorf("task id is required")
	}
	r.tasks[string(task.ID)] = task
	return nil
}

func (r *handlerTaskRepo) UpdateTask(_ context.Context, task domain.Task) error {
	if _, ok := r.tasks[string(task.ID)]; !ok {
		return fmt.Errorf("task %s not found", task.ID)
	}
	r.tasks[string(task.ID)] = task
	return nil
}

func (r *handlerTaskRepo) GetTask(_ context.Context, taskID domain.TaskID) (domain.Task, error) {
	task, ok := r.tasks[string(taskID)]
	if !ok {
		return domain.Task{}, fmt.Errorf("task %s not found", taskID)
	}
	return task, nil
}

func (r *handlerTaskRepo) ListTasks(_ context.Context, filter domain.TaskFilter) ([]domain.Task, error) {
	r.lastListFilter = filter
	out := make([]domain.Task, 0, len(r.tasks))
	for _, task := range r.tasks {
		if filter.SpaceID != "" && task.SpaceID != filter.SpaceID {
			continue
		}
		if filter.AssignedTo != "" && task.AssignedTo != filter.AssignedTo {
			continue
		}
		if filter.TaskKind != "" && task.TaskKind != filter.TaskKind {
			continue
		}
		out = append(out, task)
	}
	return out, nil
}

func (r *handlerTaskRepo) CountTasks(_ context.Context, filter domain.TaskFilter) (int, error) {
	r.lastCountFilter = filter
	tasks, err := r.ListTasks(context.Background(), filter)
	if err != nil {
		return 0, err
	}
	return len(tasks), nil
}

type handlerCallerResolver struct {
	caller taskapp.Caller
}

func (r handlerCallerResolver) ResolveCaller(context.Context) (taskapp.Caller, error) {
	return r.caller, nil
}

type handlerMemberReader struct {
	members map[string]member.Record
}

func (r handlerMemberReader) GetMember(_ context.Context, memberID member.ID) (member.Record, error) {
	rosterMember, ok := r.members[string(memberID)]
	if !ok {
		return member.Record{}, fmt.Errorf("member %s not found", memberID)
	}
	return rosterMember, nil
}

type handlerSpaceReader struct {
	spaces map[string]spacedomain.SpaceRecord
}

func (r handlerSpaceReader) Get(_ context.Context, spaceID spacedomain.SpaceID) (spacedomain.SpaceRecord, error) {
	space, ok := r.spaces[string(spaceID)]
	if !ok {
		return spacedomain.SpaceRecord{}, fmt.Errorf("space %s not found", spaceID)
	}
	return space, nil
}

type handlerMessagePublisher struct {
	messages []messagedomain.NewMessageInput
}

func (p *handlerMessagePublisher) PublishAgentMessage(_ context.Context, input messagedomain.NewMessageInput) (types.AgentMessage, error) {
	p.messages = append(p.messages, input)
	return types.AgentMessage{}, nil
}

type handlerGraphLinkWriter struct{}

func (handlerGraphLinkWriter) Link(_ context.Context, req graphdomain.GraphLinkRequest) (graphdomain.GraphEdge, []graphdomain.GraphWarning, error) {
	return graphdomain.GraphEdge{
		SourceType: req.SourceType,
		SourceID:   req.SourceID,
		TargetType: req.TargetType,
		TargetID:   req.TargetID,
		EdgeType:   req.EdgeType,
	}, nil, nil
}

type handlerMissionResolver struct{}

func (handlerMissionResolver) KeyResultMission(_ context.Context, keyResultID string) (string, error) {
	if strings.TrimSpace(keyResultID) == "kr-1" {
		return "mission-1", nil
	}
	return "", fmt.Errorf("key result %s not found", keyResultID)
}

func newHandlerForTest(t *testing.T, caller member.ID, repo *handlerTaskRepo) (*Handler, *handlerTaskRepo, *handlerMessagePublisher) {
	t.Helper()
	publisher := &handlerMessagePublisher{}
	svc, err := taskapp.NewService(
		repo,
		domain.FixedClock{T: handlerTestNow},
		handlerCallerResolver{caller: taskapp.Caller{UserID: "user-1", MemberID: caller}},
		handlerMemberReader{members: map[string]member.Record{
			"member-coordinator": {
				ID:             member.ID("member-coordinator"),
				UserID:         "user-1",
				SpaceID:        "space-1",
				MemberType:     member.TypeCoordinator,
				LifecycleState: member.LifecycleActive,
			},
			"member-worker": {
				ID:             member.ID("member-worker"),
				UserID:         "user-1",
				SpaceID:        "space-1",
				MemberType:     member.TypeWorker,
				LifecycleState: member.LifecycleActive,
			},
		}},
		handlerSpaceReader{spaces: map[string]spacedomain.SpaceRecord{
			"space-1": {
				ID:        spacedomain.SpaceID("space-1"),
				UserID:    "user-1",
				ProjectID: "project-1",
				Status:    spacedomain.SpaceStatusOpen,
			},
		}},
		publisher,
		nil,
	)
	if err != nil {
		t.Fatalf("NewService returned error: %v", err)
	}
	svc.SetGraphLinkWriter(handlerGraphLinkWriter{})
	svc.SetKeyResultMissionReader(handlerMissionResolver{})
	return NewHandler(svc), repo, publisher
}

func handlerTask() domain.Task {
	return domain.Task{
		ID:          domain.TaskID("task-1"),
		SpaceID:     spacedomain.SpaceID("space-1"),
		AssignedTo:  member.ID("member-worker"),
		TaskKind:    "research",
		CreatedBy:   "member-coordinator",
		Title:       "Report",
		Description: "write a report",
		Status:      domain.TaskStatusPending,
		CreatedAt:   &handlerTestNow,
		UpdatedAt:   &handlerTestNow,
	}
}
