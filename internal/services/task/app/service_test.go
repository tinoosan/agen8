package app

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	spacedomain "github.com/tinoosan/agen8-mcp-server/internal/services/space/domain"

	"github.com/google/uuid"
	graphdomain "github.com/tinoosan/agen8-mcp-server/internal/services/graph/domain"
	messagedomain "github.com/tinoosan/agen8-mcp-server/internal/services/message/domain"
	messagechannel "github.com/tinoosan/agen8-mcp-server/internal/services/message/domain/channel"
	"github.com/tinoosan/agen8-mcp-server/internal/services/space/domain/member"
	"github.com/tinoosan/agen8-mcp-server/internal/services/task/domain"
	"github.com/tinoosan/agen8-mcp-server/pkg/types"
)

var serviceTestNow = time.Date(2026, 5, 15, 10, 30, 0, 0, time.UTC)

type fakeTaskRepository struct {
	tasks map[string]domain.Task
}

func newFakeTaskRepository(tasks ...domain.Task) *fakeTaskRepository {
	repo := &fakeTaskRepository{tasks: map[string]domain.Task{}}
	for _, task := range tasks {
		repo.tasks[string(task.ID)] = task
	}
	return repo
}

func (r *fakeTaskRepository) CreateTask(_ context.Context, task domain.Task) error {
	if task.ID == "" {
		return fmt.Errorf("task id is required")
	}
	r.tasks[string(task.ID)] = task
	return nil
}

func (r *fakeTaskRepository) UpdateTask(_ context.Context, task domain.Task) error {
	if _, ok := r.tasks[string(task.ID)]; !ok {
		return fmt.Errorf("task %s not found", task.ID)
	}
	r.tasks[string(task.ID)] = task
	return nil
}

func (r *fakeTaskRepository) GetTask(_ context.Context, taskID domain.TaskID) (domain.Task, error) {
	task, ok := r.tasks[string(taskID)]
	if !ok {
		return domain.Task{}, fmt.Errorf("task %s not found", taskID)
	}
	return task, nil
}

func (r *fakeTaskRepository) ListTasks(_ context.Context, _ domain.TaskFilter) ([]domain.Task, error) {
	out := make([]domain.Task, 0, len(r.tasks))
	for _, task := range r.tasks {
		out = append(out, task)
	}
	return out, nil
}

func (r *fakeTaskRepository) CountTasks(_ context.Context, _ domain.TaskFilter) (int, error) {
	return len(r.tasks), nil
}

type fakeCallerResolver struct {
	caller Caller
}

func (r fakeCallerResolver) ResolveCaller(context.Context) (Caller, error) {
	return r.caller, nil
}

type fakeMemberLoader struct {
	members map[string]member.Record
}

func (r fakeMemberLoader) GetMember(_ context.Context, memberID member.ID) (member.Record, error) {
	rosterMember, ok := r.members[string(memberID)]
	if !ok {
		return member.Record{}, fmt.Errorf("member %s not found", memberID)
	}
	return rosterMember, nil
}

type fakeSpaceLoader struct {
	spaces map[string]spacedomain.SpaceRecord
}

func (r fakeSpaceLoader) Get(_ context.Context, spaceID spacedomain.SpaceID) (spacedomain.SpaceRecord, error) {
	space, ok := r.spaces[string(spaceID)]
	if !ok {
		return spacedomain.SpaceRecord{}, fmt.Errorf("space %s not found", spaceID)
	}
	return space, nil
}

type fakeMessagePublisher struct {
	messages []messagedomain.NewMessageInput
}

func (p *fakeMessagePublisher) PublishAgentMessage(_ context.Context, input messagedomain.NewMessageInput) (types.AgentMessage, error) {
	p.messages = append(p.messages, input)
	return types.AgentMessage{}, nil
}

type fakeGraphLinkWriter struct {
	links []graphdomain.GraphLinkRequest
	err   error
}

func (w *fakeGraphLinkWriter) Link(_ context.Context, req graphdomain.GraphLinkRequest) (graphdomain.GraphEdge, []graphdomain.GraphWarning, error) {
	w.links = append(w.links, req)
	if w.err != nil {
		return graphdomain.GraphEdge{}, nil, w.err
	}
	return graphdomain.GraphEdge{
		SourceType: req.SourceType,
		SourceID:   req.SourceID,
		TargetType: req.TargetType,
		TargetID:   req.TargetID,
		EdgeType:   req.EdgeType,
	}, []graphdomain.GraphWarning{}, nil
}

type fakeMissionResolver struct {
	missions map[string]string
	err      error
}

func (r fakeMissionResolver) KeyResultMission(_ context.Context, keyResultID string) (string, error) {
	if r.err != nil {
		return "", r.err
	}
	return strings.TrimSpace(r.missions[strings.TrimSpace(keyResultID)]), nil
}

func TestCreatePublishesAssignedMessage(t *testing.T) {
	publisher := &fakeMessagePublisher{}
	svc := newServiceForTest(t, member.ID("member-coordinator"), newFakeTaskRepository(), publisher)

	task, err := svc.Create(context.Background(), CreateTaskParams{
		SpaceID:            spacedomain.SpaceID("space-1"),
		AssignedTo:         member.ID("member-worker"),
		Description:        "write the report",
		AcceptanceCriteria: []string{"tests pass"},
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if task.ID == "" {
		t.Fatal("created task has empty id")
	}
	if len(publisher.messages) != 1 {
		t.Fatalf("published messages=%d want 1", len(publisher.messages))
	}
	msg := publisher.messages[0]
	if msg.Content.TaskRef != task.ID {
		t.Fatalf("message taskRef=%q want %q", msg.Content.TaskRef, task.ID)
	}
	if msg.Route.DestinationMemberID != "member-worker" {
		t.Fatalf("message target=%q want member-worker", msg.Route.DestinationMemberID)
	}
	assertMessageKind(t, msg, TaskMessageAssigned)
	if msg.Content.Subject != "Task assigned" {
		t.Fatalf("message subject=%q want Task assigned", msg.Content.Subject)
	}
	if msg.Route.ChannelID != messagechannel.MemberChannelID(spacedomain.SpaceID("space-1"), member.ID("member-worker")) {
		t.Fatalf("message channel=%q want member channel", msg.Route.ChannelID)
	}
	assertTaskMessageRouting(t, msg, task.ID, spacedomain.SpaceID("space-1"), member.ID("member-coordinator"), member.ID("member-worker"), "claim")
	assertTaskMessageDoesNotDuplicateTask(t, msg)
}

func TestCreateAssignedToSelfDoesNotPublishAssignedMessage(t *testing.T) {
	publisher := &fakeMessagePublisher{}
	svc := newServiceForTest(t, member.ID("member-coordinator"), newFakeTaskRepository(), publisher)

	_, err := svc.Create(context.Background(), CreateTaskParams{
		SpaceID:     spacedomain.SpaceID("space-1"),
		AssignedTo:  member.ID("member-coordinator"),
		Description: "write the report",
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if len(publisher.messages) != 0 {
		t.Fatalf("published messages=%d want 0 for self-assignment", len(publisher.messages))
	}
}

func TestCreateWithKeyResultRefCreatesContextLinks(t *testing.T) {
	publisher := &fakeMessagePublisher{}
	links := &fakeGraphLinkWriter{}
	svc := newServiceForTest(t, member.ID("member-coordinator"), newFakeTaskRepository(), publisher)
	svc.SetGraphLinkWriter(links)
	svc.SetKeyResultMissionReader(fakeMissionResolver{missions: map[string]string{"kr-1": "mission-1"}})

	task, err := svc.Create(context.Background(), CreateTaskParams{
		SpaceID:      spacedomain.SpaceID("space-1"),
		AssignedTo:   member.ID("member-worker"),
		Description:  "write the report",
		KeyResultRef: "kr-1",
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	want := map[string]bool{
		"task/" + string(task.ID) + "/key_result/kr-1/serves":   false,
		"task/" + string(task.ID) + "/mission/mission-1/serves": false,
	}
	for _, link := range links.links {
		key := link.SourceType + "/" + link.SourceID + "/" + link.TargetType + "/" + link.TargetID + "/" + link.EdgeType
		if _, ok := want[key]; ok {
			want[key] = true
		}
		if link.ProjectID != "project-1" {
			t.Fatalf("ProjectID=%q want project-1", link.ProjectID)
		}
	}
	for key, seen := range want {
		if !seen {
			t.Fatalf("missing context link %s in %#v", key, links.links)
		}
	}
}

func TestCreateWithMissionMetadataCreatesMissionContextLink(t *testing.T) {
	publisher := &fakeMessagePublisher{}
	links := &fakeGraphLinkWriter{}
	svc := newServiceForTest(t, member.ID("member-coordinator"), newFakeTaskRepository(), publisher)
	svc.SetGraphLinkWriter(links)

	task, err := svc.Create(context.Background(), CreateTaskParams{
		SpaceID:     spacedomain.SpaceID("space-1"),
		AssignedTo:  member.ID("member-worker"),
		Description: "write the report",
		MissionRef:  "mission-1",
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if len(links.links) != 1 {
		t.Fatalf("links=%d want 1: %#v", len(links.links), links.links)
	}
	link := links.links[0]
	if link.SourceType != graphdomain.NodeTypeTask || link.SourceID != string(task.ID) || link.TargetType != graphdomain.NodeTypeMission || link.TargetID != "mission-1" || link.EdgeType != graphdomain.EdgeTypeServes {
		t.Fatalf("unexpected link: %#v", link)
	}
	if task.Metadata["missionRef"] != "mission-1" {
		t.Fatalf("task metadata missionRef=%v want mission-1", task.Metadata["missionRef"])
	}
}

func TestCreateContextLinkErrorStopsAssignmentMessage(t *testing.T) {
	publisher := &fakeMessagePublisher{}
	links := &fakeGraphLinkWriter{err: fmt.Errorf("graph unavailable")}
	svc := newServiceForTest(t, member.ID("member-coordinator"), newFakeTaskRepository(), publisher)
	svc.SetGraphLinkWriter(links)
	svc.SetKeyResultMissionReader(fakeMissionResolver{missions: map[string]string{"kr-1": "mission-1"}})

	_, err := svc.Create(context.Background(), CreateTaskParams{
		SpaceID:      spacedomain.SpaceID("space-1"),
		AssignedTo:   member.ID("member-worker"),
		Description:  "write the report",
		KeyResultRef: "kr-1",
	})
	if err == nil {
		t.Fatal("Create returned nil error")
	}
	if !strings.Contains(err.Error(), "create key_result link") {
		t.Fatalf("error=%v want context link failure", err)
	}
	if len(publisher.messages) != 0 {
		t.Fatalf("published messages=%d want 0", len(publisher.messages))
	}
}

func TestCreateLogsTaskTransition(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil)).With("service", "task")
	publisher := &fakeMessagePublisher{}
	svc := newServiceForCallerTestWithLogger(t, Caller{UserID: "user-1", MemberID: "member-coordinator"}, newFakeTaskRepository(), publisher, logger)

	_, err := svc.Create(context.Background(), CreateTaskParams{
		SpaceID:            spacedomain.SpaceID("space-1"),
		AssignedTo:         member.ID("member-worker"),
		Description:        "write the report",
		AcceptanceCriteria: []string{"tests pass"},
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	out := logs.String()
	for _, want := range []string{
		`msg="task transition"`,
		"service=task",
		"action=create",
		"space_id=space-1",
		"status=pending",
		"assigned_to=member-worker",
		"caller_member_id=member-coordinator",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("log output missing %q:\n%s", want, out)
		}
	}
}

func TestCreatePreservesPlanRefs(t *testing.T) {
	publisher := &fakeMessagePublisher{}
	svc := newServiceForTest(t, member.ID("member-coordinator"), newFakeTaskRepository(), publisher)
	phaseID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	todoID := uuid.MustParse("22222222-2222-2222-2222-222222222222")

	task, err := svc.Create(context.Background(), CreateTaskParams{
		SpaceID:     spacedomain.SpaceID("space-1"),
		AssignedTo:  member.ID("member-worker"),
		Description: "write the report",
		PlanPhaseID: &phaseID,
		PlanTodoID:  &todoID,
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if task.PlanPhaseID == nil || *task.PlanPhaseID != phaseID {
		t.Fatalf("planPhaseId=%v want %s", task.PlanPhaseID, phaseID)
	}
	if task.PlanTodoID == nil || *task.PlanTodoID != todoID {
		t.Fatalf("planTodoId=%v want %s", task.PlanTodoID, todoID)
	}
}

func TestUserCallerCanCreateTaskForOwnedSpace(t *testing.T) {
	publisher := &fakeMessagePublisher{}
	svc := newServiceForCallerTest(t, Caller{UserID: "user-1"}, newFakeTaskRepository(), publisher)

	task, err := svc.Create(context.Background(), CreateTaskParams{
		SpaceID:     spacedomain.SpaceID("space-1"),
		AssignedTo:  member.ID("member-worker"),
		Description: "write the report",
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if task.CreatedBy != "user-1" {
		t.Fatalf("createdBy=%q want user-1", task.CreatedBy)
	}
	if len(publisher.messages) != 1 {
		t.Fatalf("published messages=%d want 1", len(publisher.messages))
	}
	if publisher.messages[0].Route.SourceMemberID != "" {
		t.Fatalf("sourceMemberId=%q want empty for user caller", publisher.messages[0].Route.SourceMemberID)
	}
}

func TestUserCallerCannotCreateTaskForAnotherUsersSpace(t *testing.T) {
	publisher := &fakeMessagePublisher{}
	svc := newServiceForCallerTest(t, Caller{UserID: "user-2"}, newFakeTaskRepository(), publisher)

	_, err := svc.Create(context.Background(), CreateTaskParams{
		SpaceID:     spacedomain.SpaceID("space-1"),
		AssignedTo:  member.ID("member-worker"),
		Description: "write the report",
	})
	if err == nil {
		t.Fatal("Create returned nil error for non-owner user")
	}
}

func TestUpdatePersistsEditableTaskFields(t *testing.T) {
	publisher := &fakeMessagePublisher{}
	repo := newFakeTaskRepository(activeTask())
	svc := newServiceForTest(t, member.ID("member-coordinator"), repo, publisher)
	title := "Revised task"
	description := "write the revised report"
	taskKind := "research"
	keyResultRef := "kr-2"
	phaseID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	todoID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	criteria := []domain.AcceptanceCriterion{
		{ID: "criterion-1", Text: "report is complete", Satisfied: true},
	}

	task, err := svc.Update(context.Background(), UpdateTaskParams{
		TaskID:             domain.TaskID("task-1"),
		Title:              &title,
		Description:        &description,
		AcceptanceCriteria: &criteria,
		TaskKind:           &taskKind,
		KeyResultRef:       &keyResultRef,
		PlanPhaseID:        &phaseID,
		PlanTodoID:         &todoID,
		Metadata:           map[string]any{"history": []any{"updated"}},
	})
	if err != nil {
		t.Fatalf("Update returned error: %v", err)
	}
	if task.Title != title {
		t.Fatalf("title=%q want %q", task.Title, title)
	}
	if task.Description != description {
		t.Fatalf("description=%q want %q", task.Description, description)
	}
	if task.TaskKind != taskKind || task.KeyResultRef != keyResultRef {
		t.Fatalf("task refs kind=%q kr=%q", task.TaskKind, task.KeyResultRef)
	}
	if task.PlanPhaseID == nil || *task.PlanPhaseID != phaseID {
		t.Fatalf("planPhaseId=%v want %s", task.PlanPhaseID, phaseID)
	}
	if task.PlanTodoID == nil || *task.PlanTodoID != todoID {
		t.Fatalf("planTodoId=%v want %s", task.PlanTodoID, todoID)
	}
	if len(task.AcceptanceCriteria) != 1 || task.AcceptanceCriteria[0].Text != "report is complete" || !task.AcceptanceCriteria[0].Satisfied {
		t.Fatalf("acceptanceCriteria=%+v", task.AcceptanceCriteria)
	}
	if task.Metadata["history"] == nil {
		t.Fatalf("metadata=%+v want history", task.Metadata)
	}
	if task.UpdatedAt == nil || !task.UpdatedAt.Equal(serviceTestNow) {
		t.Fatalf("updatedAt=%v want %s", task.UpdatedAt, serviceTestNow)
	}
	if len(publisher.messages) != 0 {
		t.Fatalf("published messages=%d want 0", len(publisher.messages))
	}
}

func TestUserCallerCanUpdateOwnedSpaceTask(t *testing.T) {
	publisher := &fakeMessagePublisher{}
	repo := newFakeTaskRepository(activeTask())
	svc := newServiceForCallerTest(t, Caller{UserID: "user-1"}, repo, publisher)
	description := "updated by user"

	task, err := svc.Update(context.Background(), UpdateTaskParams{
		TaskID:      domain.TaskID("task-1"),
		Description: &description,
	})
	if err != nil {
		t.Fatalf("Update returned error: %v", err)
	}
	if task.Description != description {
		t.Fatalf("description=%q want %q", task.Description, description)
	}
}

func TestUserCallerCannotClaimTask(t *testing.T) {
	publisher := &fakeMessagePublisher{}
	repo := newFakeTaskRepository(pendingTask())
	svc := newServiceForCallerTest(t, Caller{UserID: "user-1"}, repo, publisher)

	_, err := svc.Claim(context.Background(), domain.TaskID("task-1"))
	if err == nil {
		t.Fatal("Claim returned nil error for user-only caller")
	}
	if !strings.Contains(err.Error(), "member id is required") {
		t.Fatalf("Claim error=%v want member id required", err)
	}
}

func TestCompleteUserCreatedTaskDoesNotPublishReviewMessage(t *testing.T) {
	publisher := &fakeMessagePublisher{}
	task := activeTask()
	task.CreatedBy = "user-1"
	repo := newFakeTaskRepository(task)
	svc := newServiceForTest(t, member.ID("member-worker"), repo, publisher)

	_, err := svc.Complete(context.Background(), CompleteTaskParams{
		TaskID:  domain.TaskID("task-1"),
		Summary: "done",
	})
	if err != nil {
		t.Fatalf("Complete returned error: %v", err)
	}
	if len(publisher.messages) != 0 {
		t.Fatalf("published messages=%d want 0 for task without coordinator member creator", len(publisher.messages))
	}
}

func TestCompleteSelfCreatedTaskDoesNotPublishReviewMessage(t *testing.T) {
	publisher := &fakeMessagePublisher{}
	task := activeTask()
	task.CreatedBy = "member-worker"
	repo := newFakeTaskRepository(task)
	svc := newServiceForTest(t, member.ID("member-worker"), repo, publisher)

	_, err := svc.Complete(context.Background(), CompleteTaskParams{
		TaskID:  domain.TaskID("task-1"),
		Summary: "done",
	})
	if err != nil {
		t.Fatalf("Complete returned error: %v", err)
	}
	if len(publisher.messages) != 0 {
		t.Fatalf("published messages=%d want 0 for self-review", len(publisher.messages))
	}
}

func TestCompletePublishesReviewMessageWithSummaryOnly(t *testing.T) {
	publisher := &fakeMessagePublisher{}
	repo := newFakeTaskRepository(activeTask())
	svc := newServiceForTest(t, member.ID("member-worker"), repo, publisher)

	task, err := svc.Complete(context.Background(), CompleteTaskParams{
		TaskID:    domain.TaskID("task-1"),
		Summary:   "implemented the report",
		Artifacts: []string{"reports/summary.md"},
	})
	if err != nil {
		t.Fatalf("Complete returned error: %v", err)
	}
	if len(publisher.messages) != 1 {
		t.Fatalf("published messages=%d want 1", len(publisher.messages))
	}
	msg := publisher.messages[0]
	assertMessageKind(t, msg, TaskMessageReviewRequested)
	assertTaskMessageRouting(t, msg, task.ID, task.SpaceID, member.ID("member-worker"), member.ID("member-coordinator"), "review")
	if got := msg.Content.Body["summary"]; got != "implemented the report" {
		t.Fatalf("summary=%v want worker summary", got)
	}
	assertTaskMessageDoesNotDuplicateTask(t, msg)
}

func TestUpdateRejectsEmptyDescription(t *testing.T) {
	publisher := &fakeMessagePublisher{}
	repo := newFakeTaskRepository(activeTask())
	svc := newServiceForTest(t, member.ID("member-coordinator"), repo, publisher)
	description := " "

	if _, err := svc.Update(context.Background(), UpdateTaskParams{
		TaskID:      domain.TaskID("task-1"),
		Description: &description,
	}); err == nil {
		t.Fatal("Update returned nil error for empty description")
	}
}

func TestAssignPublishesAssignedMessage(t *testing.T) {
	publisher := &fakeMessagePublisher{}
	repo := newFakeTaskRepository(activeTask())
	svc := newServiceForTest(t, member.ID("member-coordinator"), repo, publisher)

	task, err := svc.Assign(context.Background(), AssignTaskParams{
		TaskID:     domain.TaskID("task-1"),
		AssignedTo: member.ID("member-other-worker"),
	})
	if err != nil {
		t.Fatalf("Assign returned error: %v", err)
	}
	if task.AssignedTo != "member-other-worker" {
		t.Fatalf("assignedTo=%q want member-other-worker", task.AssignedTo)
	}
	if task.Status != domain.TaskStatusPending {
		t.Fatalf("status=%q want pending", task.Status)
	}
	if task.ClaimedByMemberID != "" {
		t.Fatalf("claimedBy=%q want empty", task.ClaimedByMemberID)
	}
	if len(publisher.messages) != 1 {
		t.Fatalf("published messages=%d want 1", len(publisher.messages))
	}
	msg := publisher.messages[0]
	assertMessageKind(t, msg, TaskMessageAssigned)
	if msg.Route.DestinationMemberID != "member-other-worker" {
		t.Fatalf("target=%q want member-other-worker", msg.Route.DestinationMemberID)
	}
	assertTaskMessageRouting(t, msg, task.ID, task.SpaceID, member.ID("member-coordinator"), member.ID("member-other-worker"), "claim")
}

func TestAssignToSelfDoesNotPublishAssignedMessage(t *testing.T) {
	publisher := &fakeMessagePublisher{}
	repo := newFakeTaskRepository(activeTask())
	svc := newServiceForTest(t, member.ID("member-coordinator"), repo, publisher)

	task, err := svc.Assign(context.Background(), AssignTaskParams{
		TaskID:     domain.TaskID("task-1"),
		AssignedTo: member.ID("member-coordinator"),
	})
	if err != nil {
		t.Fatalf("Assign returned error: %v", err)
	}
	if task.AssignedTo != "member-coordinator" {
		t.Fatalf("assignedTo=%q want member-coordinator", task.AssignedTo)
	}
	if len(publisher.messages) != 0 {
		t.Fatalf("published messages=%d want 0 for self-reassignment", len(publisher.messages))
	}
}

func TestReviewApprovalAndFailureDoNotPublishMessages(t *testing.T) {
	t.Run("approve", func(t *testing.T) {
		publisher := &fakeMessagePublisher{}
		repo := newFakeTaskRepository(reviewTask())
		svc := newServiceForTest(t, member.ID("member-coordinator"), repo, publisher)

		_, err := svc.ApproveReview(context.Background(), ReviewTaskParams{
			TaskID: domain.TaskID("task-1"),
			Criteria: []domain.CriterionReview{
				{ID: "criterion-1", Satisfied: true},
			},
		})
		if err != nil {
			t.Fatalf("ApproveReview returned error: %v", err)
		}
		if len(publisher.messages) != 0 {
			t.Fatalf("published messages=%d want 0", len(publisher.messages))
		}
	})

	t.Run("fail", func(t *testing.T) {
		publisher := &fakeMessagePublisher{}
		repo := newFakeTaskRepository(reviewTask())
		svc := newServiceForTest(t, member.ID("member-coordinator"), repo, publisher)

		_, err := svc.FailReview(context.Background(), ReviewTaskParams{
			TaskID: domain.TaskID("task-1"),
			Reason: "not enough evidence",
			Criteria: []domain.CriterionReview{
				{ID: "criterion-1", Satisfied: false},
			},
		})
		if err != nil {
			t.Fatalf("FailReview returned error: %v", err)
		}
		if len(publisher.messages) != 0 {
			t.Fatalf("published messages=%d want 0", len(publisher.messages))
		}
	})
}

func TestLifecycleActionsPublishExpectedMessageKinds(t *testing.T) {
	tests := []struct {
		name       string
		caller     member.ID
		task       domain.Task
		run        func(*Service) (domain.Task, error)
		wantKind   TaskMessageKind
		wantTarget string
		wantAction string
	}{
		{
			name:   "retry review notifies assigned worker",
			caller: member.ID("member-coordinator"),
			task:   reviewTask(),
			run: func(s *Service) (domain.Task, error) {
				return s.RetryReview(context.Background(), ReviewTaskParams{
					TaskID: domain.TaskID("task-1"),
					Reason: "tighten summary",
					Criteria: []domain.CriterionReview{
						{ID: "criterion-1", Satisfied: false},
					},
				})
			},
			wantKind:   TaskMessageRetryRequested,
			wantTarget: "member-worker",
			wantAction: "submit",
		},
		{
			name:   "block notifies coordinator",
			caller: member.ID("member-worker"),
			task:   activeTask(),
			run: func(s *Service) (domain.Task, error) {
				return s.Block(context.Background(), domain.TaskID("task-1"), "waiting on access")
			},
			wantKind:   TaskMessageBlocked,
			wantTarget: "member-coordinator",
			wantAction: "unblock",
		},
		{
			name:   "unblock notifies assigned worker",
			caller: member.ID("member-worker"),
			task:   blockedTask(),
			run: func(s *Service) (domain.Task, error) {
				return s.Unblock(context.Background(), domain.TaskID("task-1"), "access granted")
			},
			wantKind:   TaskMessageUnblocked,
			wantTarget: "member-worker",
			wantAction: "submit",
		},
		{
			name:   "release notifies coordinator",
			caller: member.ID("member-worker"),
			task:   activeTask(),
			run: func(s *Service) (domain.Task, error) {
				return s.Release(context.Background(), domain.TaskID("task-1"))
			},
			wantKind:   TaskMessageReleased,
			wantTarget: "member-coordinator",
			wantAction: "assign",
		},
		{
			name:   "cancel notifies worker",
			caller: member.ID("member-coordinator"),
			task:   activeTask(),
			run: func(s *Service) (domain.Task, error) {
				return s.Cancel(context.Background(), domain.TaskID("task-1"), "no longer needed")
			},
			wantKind:   TaskMessageCanceled,
			wantTarget: "member-worker",
			wantAction: "stop",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			publisher := &fakeMessagePublisher{}
			svc := newServiceForTest(t, tt.caller, newFakeTaskRepository(tt.task), publisher)

			if _, err := tt.run(svc); err != nil {
				t.Fatalf("service action returned error: %v", err)
			}
			if len(publisher.messages) != 1 {
				t.Fatalf("published messages=%d want 1", len(publisher.messages))
			}
			msg := publisher.messages[0]
			assertMessageKind(t, msg, tt.wantKind)
			if msg.Route.DestinationMemberID != member.ID(tt.wantTarget) {
				t.Fatalf("target=%q want %q", msg.Route.DestinationMemberID, tt.wantTarget)
			}
			if got := msg.Content.Body["nextAction"]; got != tt.wantAction {
				t.Fatalf("nextAction=%v want %q", got, tt.wantAction)
			}
			assertTaskMessageDoesNotDuplicateTask(t, msg)
		})
	}
}

func TestCoordinatorCanUnblockSameSpaceTask(t *testing.T) {
	publisher := &fakeMessagePublisher{}
	repo := newFakeTaskRepository(blockedTask())
	svc := newServiceForTest(t, member.ID("member-coordinator"), repo, publisher)

	task, err := svc.Unblock(context.Background(), domain.TaskID("task-1"), "access granted")
	if err != nil {
		t.Fatalf("Unblock returned error: %v", err)
	}
	if task.Status != domain.TaskStatusActive {
		t.Fatalf("status=%q want active", task.Status)
	}
	if len(publisher.messages) != 1 {
		t.Fatalf("published messages=%d want 1", len(publisher.messages))
	}
	msg := publisher.messages[0]
	assertMessageKind(t, msg, TaskMessageUnblocked)
	if msg.Route.DestinationMemberID != "member-worker" {
		t.Fatalf("target=%q want member-worker", msg.Route.DestinationMemberID)
	}
}

func TestUnrelatedMemberCannotUnblockTask(t *testing.T) {
	publisher := &fakeMessagePublisher{}
	repo := newFakeTaskRepository(blockedTask())
	svc := newServiceForTest(t, member.ID("member-other-worker"), repo, publisher)

	_, err := svc.Unblock(context.Background(), domain.TaskID("task-1"), "access granted")
	if err == nil {
		t.Fatal("Unblock returned nil error for unrelated worker")
	}
	if !strings.Contains(err.Error(), "not a coordinator") {
		t.Fatalf("Unblock error=%v want coordinator rejection", err)
	}
	if len(publisher.messages) != 0 {
		t.Fatalf("published messages=%d want 0", len(publisher.messages))
	}
}

func newServiceForTest(t *testing.T, caller member.ID, tasks *fakeTaskRepository, messages *fakeMessagePublisher) *Service {
	t.Helper()
	return newServiceForCallerTest(t, Caller{UserID: "user-1", MemberID: caller}, tasks, messages)
}

func newServiceForCallerTest(t *testing.T, caller Caller, tasks *fakeTaskRepository, messages *fakeMessagePublisher) *Service {
	return newServiceForCallerTestWithLogger(t, caller, tasks, messages, nil)
}

func newServiceForCallerTestWithLogger(t *testing.T, caller Caller, tasks *fakeTaskRepository, messages *fakeMessagePublisher, logger *slog.Logger) *Service {
	t.Helper()
	svc, err := NewService(
		tasks,
		domain.FixedClock{T: serviceTestNow},
		fakeCallerResolver{caller: caller},
		fakeMemberLoader{members: map[string]member.Record{
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
			"member-other-worker": {
				ID:             member.ID("member-other-worker"),
				UserID:         "user-1",
				SpaceID:        "space-1",
				MemberType:     member.TypeWorker,
				LifecycleState: member.LifecycleActive,
			},
		}},
		fakeSpaceLoader{spaces: map[string]spacedomain.SpaceRecord{
			"space-1": {
				ID:        spacedomain.SpaceID("space-1"),
				UserID:    "user-1",
				ProjectID: "project-1",
				Status:    spacedomain.SpaceStatusOpen,
			},
		}},
		messages,
		logger,
	)
	if err != nil {
		t.Fatalf("NewService returned error: %v", err)
	}
	return svc
}

func assertMessageKind(t *testing.T, msg messagedomain.NewMessageInput, want TaskMessageKind) {
	t.Helper()
	if got := msg.Content.Body["event"]; got != string(want) {
		t.Fatalf("message event=%v want %s", got, want)
	}
	if got := msg.Producer.IntentID; got != types.IntentID("task:"+string(msg.Content.TaskRef)+":"+string(want)) {
		t.Fatalf("intentID=%q want task:%s:%s", got, msg.Content.TaskRef, want)
	}
	if got := msg.Content.Body["guidance"]; got == "" {
		t.Fatal("message guidance is empty")
	}
	if got := msg.Content.Body["message"]; got == "" {
		t.Fatal("message body is empty")
	}
	if msg.Content.Kind != types.AgentMessageKindSystem {
		t.Fatalf("message kind=%q want system", msg.Content.Kind)
	}
}

func assertTaskMessageRouting(t *testing.T, msg messagedomain.NewMessageInput, taskID domain.TaskID, spaceID spacedomain.SpaceID, actor member.ID, target member.ID, nextAction string) {
	t.Helper()
	body := msg.Content.Body
	checks := map[string]string{
		"taskId":         string(taskID),
		"spaceId":        string(spaceID),
		"actorMemberId":  string(actor),
		"targetMemberId": string(target),
		"nextAction":     nextAction,
	}
	for key, want := range checks {
		if got := body[key]; got != want {
			t.Fatalf("message %s=%v want %q", key, got, want)
		}
	}
	if msg.Route.SourceMemberID != "" {
		t.Fatalf("message source member=%q want empty system source", msg.Route.SourceMemberID)
	}
}

func assertTaskMessageDoesNotDuplicateTask(t *testing.T, msg messagedomain.NewMessageInput) {
	t.Helper()
	for _, key := range []string{"title", "description", "status", "acceptanceCriteria", "artifacts", "metadata", "planPhaseId", "planTodoId", "keyResultRef"} {
		if _, ok := msg.Content.Body[key]; ok {
			t.Fatalf("message body duplicated task field %q", key)
		}
	}
}

func activeTask() domain.Task {
	return domain.Task{
		ID:                domain.TaskID("task-1"),
		SpaceID:           spacedomain.SpaceID("space-1"),
		CreatedBy:         "member-coordinator",
		AssignedTo:        member.ID("member-worker"),
		ClaimedByMemberID: member.ID("member-worker"),
		Description:       "write the report",
		Status:            domain.TaskStatusActive,
		CreatedAt:         &serviceTestNow,
		StartedAt:         &serviceTestNow,
		UpdatedAt:         &serviceTestNow,
	}
}

func pendingTask() domain.Task {
	task := activeTask()
	task.Status = domain.TaskStatusPending
	task.ClaimedByMemberID = ""
	task.StartedAt = nil
	return task
}

func blockedTask() domain.Task {
	task := activeTask()
	task.Status = domain.TaskStatusBlocked
	return task
}

func reviewTask() domain.Task {
	task := activeTask()
	task.Status = domain.TaskStatusInReview
	task.Summary = "done"
	task.AcceptanceCriteria = []domain.AcceptanceCriterion{
		{ID: "criterion-1", Text: "tests pass"},
	}
	return task
}
