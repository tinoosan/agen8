package app

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/tinoosan/agen8-mcp-server/internal/caller"
	"github.com/tinoosan/agen8-mcp-server/internal/core/types"
	graphdomain "github.com/tinoosan/agen8-mcp-server/internal/services/graph/domain"
	"github.com/tinoosan/agen8-mcp-server/internal/services/project/domain/member"
	"github.com/tinoosan/agen8-mcp-server/internal/services/project/domain/project"
	"github.com/tinoosan/agen8-mcp-server/internal/services/task/domain"
)

type Service struct {
	tasks    domain.TaskRepository
	clock    domain.Clock
	caller   caller.Resolver
	members  MemberLoader
	projects ProjectLoader
	links    GraphLinkWriter
	missions KeyResultMissionReader
	logger   *slog.Logger
}

type Caller = caller.Caller

type MemberLoader interface {
	GetMember(ctx context.Context, memberID member.ID) (member.Record, error)
}

type ProjectLoader interface {
	Get(ctx context.Context, projectID types.ProjectID) (project.Project, error)
}

type GraphLinkWriter interface {
	Link(ctx context.Context, req graphdomain.GraphLinkRequest) (graphdomain.GraphEdge, []graphdomain.GraphWarning, error)
}

type KeyResultMissionReader interface {
	KeyResultMission(ctx context.Context, keyResultID string) (string, error)
}

type CreateTaskParams struct {
	ProjectID          types.ProjectID
	AssignedTo         member.ID
	Description        string
	AcceptanceCriteria []string
	Title              string
	KeyResultRef       string
	MissionRef         string
	Metadata           map[string]any
	TaskKind           string
}

type CompleteTaskParams struct {
	TaskID    domain.TaskID
	Summary   string
	Artifacts []string
	Metadata  map[string]any
}

type AssignTaskParams struct {
	TaskID     domain.TaskID
	AssignedTo member.ID
}

type ReviewTaskParams struct {
	TaskID   domain.TaskID
	Reason   string
	Criteria []domain.CriterionReview
}

type UpdateTaskParams struct {
	TaskID             domain.TaskID
	Title              *string
	Description        *string
	AcceptanceCriteria *[]domain.AcceptanceCriterion
	TaskKind           *string
	KeyResultRef       *string
	Metadata           map[string]any
}

func NewService(
	tasks domain.TaskRepository,
	clock domain.Clock,
	caller caller.Resolver,
	members MemberLoader,
	projects ProjectLoader,
	logger *slog.Logger,
) (*Service, error) {
	switch {
	case tasks == nil:
		return nil, fmt.Errorf("task service: tasks repository is required")
	case clock == nil:
		return nil, fmt.Errorf("task service: clock is required")
	case caller == nil:
		return nil, fmt.Errorf("task service: caller resolver is required")
	case members == nil:
		return nil, fmt.Errorf("task service: member reader is required")
	case projects == nil:
		return nil, fmt.Errorf("task service: project reader is required")
	}
	if logger == nil {
		logger = slog.Default().With("service", "task")
	}
	return &Service{
		tasks:    tasks,
		clock:    clock,
		caller:   caller,
		members:  members,
		projects: projects,
		logger:   logger,
	}, nil
}

func (s *Service) SetGraphLinkWriter(links GraphLinkWriter) {
	s.links = links
}

func (s *Service) SetKeyResultMissionReader(missions KeyResultMissionReader) {
	s.missions = missions
}

func (s *Service) Create(ctx context.Context, params CreateTaskParams) (domain.Task, error) {
	caller, err := s.resolveCaller(ctx)
	if err != nil {
		return domain.Task{}, err
	}
	if err := s.requireCoordinatorOrUserOwner(ctx, caller, params.ProjectID); err != nil {
		return domain.Task{}, err
	}
	assigned, err := s.memberInProject(ctx, params.AssignedTo, params.ProjectID)
	if err != nil {
		return domain.Task{}, err
	}

	missionRef, err := s.resolveCreationMissionRef(ctx, params.KeyResultRef, params.MissionRef)
	if err != nil {
		return domain.Task{}, err
	}
	metadata, err := metadataWithMissionRef(params.Metadata, missionRef)
	if err != nil {
		return domain.Task{}, err
	}

	task, err := domain.NewTask(domain.NewTaskInput{
		ProjectID:          params.ProjectID,
		CreatedBy:          caller.ActorID(),
		CreatedByLabel:     s.callerMemberLabel(ctx, caller, params.ProjectID),
		AssignedTo:         assigned.ID,
		AssignedToLabel:    memberRecordLabel(assigned),
		Description:        params.Description,
		AcceptanceCriteria: params.AcceptanceCriteria,
		Title:              params.Title,
		KeyResultRef:       params.KeyResultRef,
		Metadata:           metadata,
		TaskKind:           params.TaskKind,
	}, s.clock.Now())
	if err != nil {
		return domain.Task{}, err
	}
	next := task
	if err := s.tasks.CreateTask(ctx, next); err != nil {
		return domain.Task{}, fmt.Errorf("create task: %w", err)
	}
	if err := s.emitContextLinks(ctx, next); err != nil {
		return domain.Task{}, err
	}
	s.logTaskTransition("create", next, caller)
	return next, nil
}

func (s *Service) Get(ctx context.Context, taskID domain.TaskID) (domain.Task, error) {
	taskID = trimTaskID(taskID)
	if taskID == "" {
		return domain.Task{}, fmt.Errorf("taskId is required")
	}
	task, err := s.tasks.GetTask(ctx, taskID)
	if err != nil {
		return domain.Task{}, fmt.Errorf("load task: %w", err)
	}
	return task, nil
}

func (s *Service) List(ctx context.Context, filter domain.TaskFilter) ([]domain.Task, error) {
	filter.ProjectID = types.ProjectID(strings.TrimSpace(string(filter.ProjectID)))
	filter.AssignedTo = member.ID(strings.TrimSpace(string(filter.AssignedTo)))
	filter.ClaimedBy = member.ID(strings.TrimSpace(string(filter.ClaimedBy)))
	filter.TaskKind = strings.TrimSpace(filter.TaskKind)
	filter.SortBy = strings.TrimSpace(filter.SortBy)
	if filter.Limit < 0 {
		return nil, fmt.Errorf("task list limit must be non-negative")
	}
	if filter.Offset < 0 {
		return nil, fmt.Errorf("task list offset must be non-negative")
	}
	return s.tasks.ListTasks(ctx, filter)
}

func (s *Service) Count(ctx context.Context, filter domain.TaskFilter) (int, error) {
	if filter.Limit < 0 {
		return 0, fmt.Errorf("task count limit must be non-negative")
	}
	if filter.Offset < 0 {
		return 0, fmt.Errorf("task count offset must be non-negative")
	}
	return s.tasks.CountTasks(ctx, filter)
}

func (s *Service) emitContextLinks(ctx context.Context, task domain.Task) error {
	keyResultRef := strings.TrimSpace(task.KeyResultRef)
	metadataMissionRef, err := missionRefFromMetadata(task.Metadata)
	if err != nil {
		return err
	}
	missionRef := metadataMissionRef
	if keyResultRef != "" {
		if s.missions == nil {
			return fmt.Errorf("task service: key result mission reader is required")
		}
		resolvedMissionRef, err := s.missions.KeyResultMission(ctx, keyResultRef)
		if err != nil {
			return fmt.Errorf("resolve task key result %s mission: %w", keyResultRef, err)
		}
		resolvedMissionRef = strings.TrimSpace(resolvedMissionRef)
		if resolvedMissionRef == "" {
			return fmt.Errorf("task service: key result %s is missing mission linkage", keyResultRef)
		}
		if missionRef != "" && !strings.EqualFold(missionRef, resolvedMissionRef) {
			return fmt.Errorf("task service: key result %s belongs to mission %s, but metadata provided %s", keyResultRef, resolvedMissionRef, missionRef)
		}
		missionRef = resolvedMissionRef
	}
	if keyResultRef == "" && missionRef == "" {
		return nil
	}
	if s.links == nil {
		return fmt.Errorf("task service: graph link writer is required")
	}
	projectID := strings.TrimSpace(string(task.ProjectID))
	if projectID == "" {
		return fmt.Errorf("task service: task %s is missing project id", task.ID)
	}
	specs := []struct {
		targetID, targetType, edge string
	}{
		{keyResultRef, graphdomain.NodeTypeKeyResult, graphdomain.EdgeTypeServes},
		{missionRef, graphdomain.NodeTypeMission, graphdomain.EdgeTypeServes},
	}
	for _, spec := range specs {
		if strings.TrimSpace(spec.targetID) == "" {
			continue
		}
		confidence := 1.0
		if _, _, err := s.links.Link(ctx, graphdomain.GraphLinkRequest{
			ProjectID:  projectID,
			SourceType: graphdomain.NodeTypeTask,
			SourceID:   string(task.ID),
			TargetType: spec.targetType,
			TargetID:   spec.targetID,
			EdgeType:   spec.edge,
			Confidence: &confidence,
			Rationale:  "Task references this work item.",
			Origin:     "reference",
			CreatedBy:  "task_service",
		}); err != nil {
			return fmt.Errorf("create %s link for task %s: %w", spec.targetType, task.ID, err)
		}
	}
	return nil
}

func missionRefFromMetadata(metadata map[string]any) (string, error) {
	for _, key := range []string{"mission_ref", "missionRef", "mission_id", "missionId"} {
		raw, ok := metadata[key]
		if !ok {
			continue
		}
		value, ok := raw.(string)
		if !ok {
			return "", fmt.Errorf("task metadata %q must be a string", key)
		}
		return strings.TrimSpace(value), nil
	}
	return "", nil
}

func metadataWithMissionRef(metadata map[string]any, missionRef string) (map[string]any, error) {
	next := cloneMap(metadata)
	missionRef = strings.TrimSpace(missionRef)
	if missionRef == "" {
		return next, nil
	}
	existing, err := missionRefFromMetadata(next)
	if err != nil {
		return nil, err
	}
	if existing != "" && !strings.EqualFold(existing, missionRef) {
		return nil, fmt.Errorf("task metadata mission ref %s conflicts with mission ref %s", existing, missionRef)
	}
	if next == nil {
		next = map[string]any{}
	}
	next["missionRef"] = missionRef
	return next, nil
}

func mergeTaskMetadata(existing map[string]any, updates map[string]any) map[string]any {
	if len(updates) == 0 {
		return cloneMap(existing)
	}
	next := cloneMap(existing)
	if next == nil {
		next = make(map[string]any, len(updates))
	}
	for key, value := range updates {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		next[key] = value
	}
	return next
}

func (s *Service) resolveCreationMissionRef(ctx context.Context, keyResultRef, missionRef string) (string, error) {
	keyResultRef = strings.TrimSpace(keyResultRef)
	missionRef = strings.TrimSpace(missionRef)
	if keyResultRef == "" {
		return missionRef, nil
	}
	if s.missions == nil {
		return "", fmt.Errorf("task service: key result mission reader is required")
	}
	resolvedMissionRef, err := s.missions.KeyResultMission(ctx, keyResultRef)
	if err != nil {
		return "", fmt.Errorf("resolve task key result %s mission: %w", keyResultRef, err)
	}
	resolvedMissionRef = strings.TrimSpace(resolvedMissionRef)
	if resolvedMissionRef == "" {
		return "", fmt.Errorf("task service: key result %s is missing mission linkage", keyResultRef)
	}
	if missionRef != "" && !strings.EqualFold(missionRef, resolvedMissionRef) {
		return "", fmt.Errorf("task service: key result %s belongs to mission %s, but mission ref %s was provided", keyResultRef, resolvedMissionRef, missionRef)
	}
	return resolvedMissionRef, nil
}

func (s *Service) Update(ctx context.Context, params UpdateTaskParams) (domain.Task, error) {
	caller, err := s.resolveCaller(ctx)
	if err != nil {
		return domain.Task{}, err
	}
	loaded, err := s.Get(ctx, params.TaskID)
	if err != nil {
		return domain.Task{}, err
	}
	if err := s.requireCoordinatorOrUserOwner(ctx, caller, loaded.ProjectID); err != nil {
		return domain.Task{}, err
	}
	if params.Title != nil {
		loaded.Title = strings.TrimSpace(*params.Title)
	}
	if params.Description != nil {
		description := strings.TrimSpace(*params.Description)
		if description == "" {
			return domain.Task{}, fmt.Errorf("task description is required")
		}
		loaded.Description = description
	}
	if params.AcceptanceCriteria != nil {
		criteria := make([]domain.AcceptanceCriterion, 0, len(*params.AcceptanceCriteria))
		seen := map[string]struct{}{}
		for _, criterion := range *params.AcceptanceCriteria {
			criterion.ID = strings.TrimSpace(criterion.ID)
			criterion.Text = strings.TrimSpace(criterion.Text)
			if criterion.ID == "" {
				return domain.Task{}, fmt.Errorf("acceptance criterion id is required")
			}
			if criterion.Text == "" {
				return domain.Task{}, fmt.Errorf("acceptance criterion text is required")
			}
			if _, ok := seen[criterion.ID]; ok {
				return domain.Task{}, fmt.Errorf("duplicate acceptance criterion id %q", criterion.ID)
			}
			seen[criterion.ID] = struct{}{}
			criteria = append(criteria, criterion)
		}
		loaded.AcceptanceCriteria = criteria
	}
	if params.TaskKind != nil {
		loaded.TaskKind = strings.TrimSpace(*params.TaskKind)
	}
	if params.KeyResultRef != nil {
		loaded.KeyResultRef = strings.TrimSpace(*params.KeyResultRef)
	}
	if params.Metadata != nil {
		loaded.Metadata = cloneMap(params.Metadata)
	}
	now := s.clock.Now().UTC()
	loaded.UpdatedAt = &now
	if err := s.tasks.UpdateTask(ctx, loaded); err != nil {
		return domain.Task{}, fmt.Errorf("update task: %w", err)
	}
	s.logTaskTransition("update", loaded, caller)
	return loaded, nil
}

func (s *Service) Claim(ctx context.Context, taskID domain.TaskID) (domain.Task, error) {
	caller, err := s.resolveCaller(ctx)
	if err != nil {
		return domain.Task{}, err
	}
	if err := requireMemberCaller(caller); err != nil {
		return domain.Task{}, err
	}
	loaded, err := s.Get(ctx, taskID)
	if err != nil {
		return domain.Task{}, err
	}
	if err := s.requireAssignedCaller(loaded, caller.MemberID); err != nil {
		return domain.Task{}, err
	}
	next, err := loaded.Claim(caller.MemberID, s.clock.Now())
	if err != nil {
		return domain.Task{}, err
	}
	next.ClaimedByMemberLabel = s.callerMemberLabel(ctx, caller, loaded.ProjectID)
	if next.AssignedToLabel == "" && next.AssignedTo == caller.MemberID {
		next.AssignedToLabel = next.ClaimedByMemberLabel
	}
	if err := s.tasks.UpdateTask(ctx, next); err != nil {
		return domain.Task{}, fmt.Errorf("update task: %w", err)
	}
	s.logTaskTransition("claim", next, caller)
	return next, nil
}

func (s *Service) Assign(ctx context.Context, params AssignTaskParams) (domain.Task, error) {
	caller, err := s.resolveCaller(ctx)
	if err != nil {
		return domain.Task{}, err
	}
	loaded, err := s.Get(ctx, params.TaskID)
	if err != nil {
		return domain.Task{}, err
	}
	if err := s.requireCoordinatorOrUserOwner(ctx, caller, loaded.ProjectID); err != nil {
		return domain.Task{}, err
	}
	assigned, err := s.memberInProject(ctx, params.AssignedTo, loaded.ProjectID)
	if err != nil {
		return domain.Task{}, err
	}
	next, err := loaded.Assign(assigned.ID, s.clock.Now())
	if err != nil {
		return domain.Task{}, err
	}
	next.AssignedToLabel = memberRecordLabel(assigned)
	if err := s.tasks.UpdateTask(ctx, next); err != nil {
		return domain.Task{}, fmt.Errorf("update task: %w", err)
	}
	s.logTaskTransition("assign", next, caller)
	return next, nil
}

func (s *Service) Complete(ctx context.Context, params CompleteTaskParams) (domain.Task, error) {
	caller, err := s.resolveCaller(ctx)
	if err != nil {
		return domain.Task{}, err
	}
	if err := requireMemberCaller(caller); err != nil {
		return domain.Task{}, err
	}
	loaded, err := s.Get(ctx, params.TaskID)
	if err != nil {
		return domain.Task{}, err
	}
	if err := s.requireClaimedCaller(loaded, caller.MemberID); err != nil {
		return domain.Task{}, err
	}
	next, err := loaded.Complete(params.Summary, params.Artifacts, s.clock.Now())
	if err != nil {
		return domain.Task{}, err
	}
	if len(params.Metadata) > 0 {
		next.Metadata = mergeTaskMetadata(next.Metadata, params.Metadata)
	}
	if err := s.tasks.UpdateTask(ctx, next); err != nil {
		return domain.Task{}, fmt.Errorf("update task: %w", err)
	}
	s.logTaskTransition("complete", next, caller)
	return next, nil
}

func (s *Service) ApproveReview(ctx context.Context, params ReviewTaskParams) (domain.Task, error) {
	caller, err := s.resolveCaller(ctx)
	if err != nil {
		return domain.Task{}, err
	}
	loaded, err := s.Get(ctx, params.TaskID)
	if err != nil {
		return domain.Task{}, err
	}
	if err := s.requireCoordinatorOrUserOwner(ctx, caller, loaded.ProjectID); err != nil {
		return domain.Task{}, err
	}
	next, err := loaded.ApproveReview(params.Criteria, s.clock.Now())
	if err != nil {
		return domain.Task{}, err
	}
	if err := s.tasks.UpdateTask(ctx, next); err != nil {
		return domain.Task{}, fmt.Errorf("update task: %w", err)
	}
	s.logTaskTransition("approve_review", next, caller)
	return next, nil
}

func (s *Service) RetryReview(ctx context.Context, params ReviewTaskParams) (domain.Task, error) {
	caller, err := s.resolveCaller(ctx)
	if err != nil {
		return domain.Task{}, err
	}
	loaded, err := s.Get(ctx, params.TaskID)
	if err != nil {
		return domain.Task{}, err
	}
	if err := s.requireCoordinatorOrUserOwner(ctx, caller, loaded.ProjectID); err != nil {
		return domain.Task{}, err
	}
	next, err := loaded.RetryReview(params.Reason, params.Criteria, s.clock.Now())
	if err != nil {
		return domain.Task{}, err
	}
	if err := s.tasks.UpdateTask(ctx, next); err != nil {
		return domain.Task{}, fmt.Errorf("update task: %w", err)
	}
	s.logTaskTransition("retry_review", next, caller)
	return next, nil
}

func (s *Service) FailReview(ctx context.Context, params ReviewTaskParams) (domain.Task, error) {
	caller, err := s.resolveCaller(ctx)
	if err != nil {
		return domain.Task{}, err
	}
	loaded, err := s.Get(ctx, params.TaskID)
	if err != nil {
		return domain.Task{}, err
	}
	if err := s.requireCoordinatorOrUserOwner(ctx, caller, loaded.ProjectID); err != nil {
		return domain.Task{}, err
	}
	next, err := loaded.FailReview(params.Reason, params.Criteria, s.clock.Now())
	if err != nil {
		return domain.Task{}, err
	}
	if err := s.tasks.UpdateTask(ctx, next); err != nil {
		return domain.Task{}, fmt.Errorf("update task: %w", err)
	}
	s.logTaskTransition("fail_review", next, caller)
	return next, nil
}

func (s *Service) Block(ctx context.Context, taskID domain.TaskID, reason string) (domain.Task, error) {
	caller, err := s.resolveCaller(ctx)
	if err != nil {
		return domain.Task{}, err
	}
	if err := requireMemberCaller(caller); err != nil {
		return domain.Task{}, err
	}
	loaded, err := s.Get(ctx, taskID)
	if err != nil {
		return domain.Task{}, err
	}
	if err := s.requireClaimedCaller(loaded, caller.MemberID); err != nil {
		return domain.Task{}, err
	}
	next, err := loaded.Block(reason, s.clock.Now())
	if err != nil {
		return domain.Task{}, err
	}
	if err := s.tasks.UpdateTask(ctx, next); err != nil {
		return domain.Task{}, fmt.Errorf("update task: %w", err)
	}
	s.logTaskTransition("block", next, caller)
	return next, nil
}

func (s *Service) Unblock(ctx context.Context, taskID domain.TaskID, note string) (domain.Task, error) {
	caller, err := s.resolveCaller(ctx)
	if err != nil {
		return domain.Task{}, err
	}
	if err := requireMemberCaller(caller); err != nil {
		return domain.Task{}, err
	}
	loaded, err := s.Get(ctx, taskID)
	if err != nil {
		return domain.Task{}, err
	}
	if err := s.requireUnblockCaller(ctx, loaded, caller.MemberID); err != nil {
		return domain.Task{}, err
	}
	next, err := loaded.Unblock(note, s.clock.Now())
	if err != nil {
		return domain.Task{}, err
	}
	if err := s.tasks.UpdateTask(ctx, next); err != nil {
		return domain.Task{}, fmt.Errorf("update task: %w", err)
	}
	s.logTaskTransition("unblock", next, caller)
	return next, nil
}

func (s *Service) Release(ctx context.Context, taskID domain.TaskID) (domain.Task, error) {
	caller, err := s.resolveCaller(ctx)
	if err != nil {
		return domain.Task{}, err
	}
	if err := requireMemberCaller(caller); err != nil {
		return domain.Task{}, err
	}
	loaded, err := s.Get(ctx, taskID)
	if err != nil {
		return domain.Task{}, err
	}
	if err := s.requireClaimedCaller(loaded, caller.MemberID); err != nil {
		return domain.Task{}, err
	}
	next, err := loaded.Release(s.clock.Now())
	if err != nil {
		return domain.Task{}, err
	}
	if err := s.tasks.UpdateTask(ctx, next); err != nil {
		return domain.Task{}, fmt.Errorf("update task: %w", err)
	}
	s.logTaskTransition("release", next, caller)
	return next, nil
}

func (s *Service) Cancel(ctx context.Context, taskID domain.TaskID, reason string) (domain.Task, error) {
	caller, err := s.resolveCaller(ctx)
	if err != nil {
		return domain.Task{}, err
	}
	loaded, err := s.Get(ctx, taskID)
	if err != nil {
		return domain.Task{}, err
	}
	if err := s.requireCoordinatorOrUserOwner(ctx, caller, loaded.ProjectID); err != nil {
		return domain.Task{}, err
	}
	next, err := loaded.Cancel(reason, s.clock.Now())
	if err != nil {
		return domain.Task{}, err
	}
	if err := s.tasks.UpdateTask(ctx, next); err != nil {
		return domain.Task{}, fmt.Errorf("update task: %w", err)
	}
	s.logTaskTransition("cancel", next, caller)
	return next, nil
}

func (s *Service) resolveCaller(ctx context.Context) (Caller, error) {
	caller, err := s.caller.ResolveCaller(ctx)
	if err != nil {
		return Caller{}, fmt.Errorf("resolve task caller: %w", err)
	}
	caller = caller.Normalize()
	if caller.UserID == "" && caller.MemberID == "" {
		return Caller{}, fmt.Errorf("task caller user id or member id is required")
	}
	return caller, nil
}

func requireMemberCaller(caller Caller) error {
	if caller.MemberID == "" {
		return fmt.Errorf("task caller member id is required")
	}
	return nil
}

func (s *Service) requireCoordinatorOrUserOwner(ctx context.Context, caller Caller, projectID types.ProjectID) error {
	if caller.MemberID != "" {
		return s.requireCoordinator(ctx, caller.MemberID, projectID)
	}
	userID := strings.TrimSpace(caller.UserID)
	if userID == "" {
		return fmt.Errorf("task caller user id is required")
	}
	proj, err := s.projects.Get(ctx, projectID)
	if err != nil {
		return fmt.Errorf("load task project: %w", err)
	}
	if strings.TrimSpace(proj.UserID()) != userID {
		return fmt.Errorf("user %s does not own project %s", userID, projectID)
	}
	return nil
}

func (s *Service) requireCoordinator(ctx context.Context, memberID member.ID, projectID types.ProjectID) error {
	member, err := s.memberInProject(ctx, memberID, projectID)
	if err != nil {
		return err
	}
	if !isCoordinatorType(member.MemberType) {
		return fmt.Errorf("member %s is not a coordinator", memberID)
	}
	return nil
}

func (s *Service) memberInProject(ctx context.Context, memberID member.ID, projectID types.ProjectID) (member.Record, error) {
	memberID = member.ID(strings.TrimSpace(string(memberID)))
	projectID = types.ProjectID(strings.TrimSpace(string(projectID)))
	if memberID == "" {
		return member.Record{}, fmt.Errorf("memberId is required")
	}
	if projectID == "" {
		return member.Record{}, fmt.Errorf("projectId is required")
	}
	rosterMember, err := s.members.GetMember(ctx, memberID)
	if err != nil {
		return member.Record{}, fmt.Errorf("load member: %w", err)
	}
	if types.ProjectID(rosterMember.ProjectID) != projectID {
		return member.Record{}, fmt.Errorf("member %s is not in project %s", memberID, projectID)
	}
	if strings.TrimSpace(rosterMember.LifecycleState) != "" && rosterMember.LifecycleState != member.LifecycleActive {
		return member.Record{}, fmt.Errorf("member %s is not active", memberID)
	}
	return rosterMember, nil
}

func (s *Service) callerMemberLabel(ctx context.Context, caller Caller, projectID types.ProjectID) string {
	if caller.MemberID == "" {
		return ""
	}
	rosterMember, err := s.memberInProject(ctx, caller.MemberID, projectID)
	if err != nil {
		return ""
	}
	return memberRecordLabel(rosterMember)
}

func memberRecordLabel(rosterMember member.Record) string {
	if label := strings.TrimSpace(rosterMember.DisplayName); label != "" {
		return label
	}
	if label := strings.TrimSpace(rosterMember.HarnessKind); label != "" {
		return label
	}
	if label := strings.TrimSpace(rosterMember.MemberType); label != "" {
		return strings.ReplaceAll(label, "_", " ")
	}
	return ""
}

func (s *Service) requireAssignedCaller(task domain.Task, memberID member.ID) error {
	memberID = member.ID(strings.TrimSpace(string(memberID)))
	if memberID == "" {
		return fmt.Errorf("caller member id is required")
	}
	if task.AssignedTo != memberID {
		return fmt.Errorf("task %s is assigned to %s, not %s", task.ID, task.AssignedTo, memberID)
	}
	return nil
}

func (s *Service) requireClaimedCaller(task domain.Task, memberID member.ID) error {
	memberID = member.ID(strings.TrimSpace(string(memberID)))
	if memberID == "" {
		return fmt.Errorf("caller member id is required")
	}
	if task.ClaimedByMemberID != memberID {
		return fmt.Errorf("task %s is claimed by %s, not %s", task.ID, task.ClaimedByMemberID, memberID)
	}
	return nil
}

func (s *Service) requireUnblockCaller(ctx context.Context, task domain.Task, memberID member.ID) error {
	memberID = member.ID(strings.TrimSpace(string(memberID)))
	if memberID == "" {
		return fmt.Errorf("caller member id is required")
	}
	if task.ClaimedByMemberID == memberID {
		return nil
	}
	return s.requireCoordinator(ctx, memberID, task.ProjectID)
}

func (s *Service) logTaskTransition(action string, task domain.Task, caller Caller) {
	s.logger.Info("task transition",
		"action", action,
		"task_id", string(task.ID),
		"project_id", string(task.ProjectID),
		"status", string(task.Status),
		"assigned_to", string(task.AssignedTo),
		"claimed_by_member_id", string(task.ClaimedByMemberID),
		"caller_user_id", caller.UserID,
		"caller_member_id", string(caller.MemberID),
	)
}

func trimTaskID(taskID domain.TaskID) domain.TaskID {
	return domain.TaskID(strings.TrimSpace(string(taskID)))
}

func isCoordinatorType(memberType string) bool {
	return strings.TrimSpace(memberType) == member.TypeCoordinator
}

func cloneMap(src map[string]any) map[string]any {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]any, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}
