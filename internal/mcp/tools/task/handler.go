package task

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	spacedomain "github.com/tinoosan/agen8-mcp-server/internal/services/space/domain"

	"github.com/google/uuid"
	"github.com/tinoosan/agen8-mcp-server/internal/caller"
	"github.com/tinoosan/agen8-mcp-server/internal/services/space/domain/member"
	taskapp "github.com/tinoosan/agen8-mcp-server/internal/services/task/app"
	taskdomain "github.com/tinoosan/agen8-mcp-server/internal/services/task/domain"
)

type Handler struct{}

func NewHandler() Handler {
	return Handler{}
}

func (h Handler) Handle(ctx context.Context, call CallContext, args json.RawMessage) (Result, error) {
	input, err := decode(args)
	if err != nil {
		return Result{}, err
	}
	if call.Tasks == nil {
		return Result{}, fmt.Errorf("task: task service is not configured")
	}
	if call.Members == nil {
		return Result{}, fmt.Errorf("task: member service is not configured")
	}
	ctx = contextWithSessionActor(ctx, call.ActorMemberID, call.SpaceID)
	actor, err := h.actor(ctx, call)
	if err != nil {
		return Result{}, err
	}
	taskCtx := caller.ContextWithCaller(ctx, caller.Caller{UserID: actor.UserID, MemberID: actor.MemberID, SpaceID: actor.SpaceID})

	switch input.Action {
	case "create":
		description, err := requireString(input.Description, "description")
		if err != nil {
			return Result{}, err
		}
		assignee, err := h.assignee(ctx, call, actor, input.AssigneeMemberID)
		if err != nil {
			return Result{}, err
		}
		task, err := call.Tasks.Create(taskCtx, taskapp.CreateTaskParams{
			SpaceID:            spacedomain.SpaceID(actor.SpaceID),
			AssignedTo:         assignee.MemberID,
			Description:        description,
			AcceptanceCriteria: input.AcceptanceCriteria,
			Title:              input.Title,
			KeyResultRef:       input.KeyResultRef,
			MissionRef:         input.MissionRef,
			Metadata:           input.Metadata,
			TaskKind:           input.TaskKind,
		})
		return h.taskResult("create", task, err, map[string]any{"assignee": assignee})
	case "get":
		id, err := requireTaskID(input.TaskID)
		if err != nil {
			return Result{}, err
		}
		task, err := call.Tasks.Get(taskCtx, id)
		if err == nil {
			err = h.canSeeTask(actor, task)
		}
		return h.taskResult("get", task, err, nil)
	case "list":
		filter, err := h.listFilter(actor, input)
		if err != nil {
			return Result{}, err
		}
		tasks, err := call.Tasks.List(taskCtx, filter)
		return h.listResult(tasks, err, input)
	case "claim":
		id, err := requireTaskID(input.TaskID)
		if err != nil {
			return Result{}, err
		}
		task, err := call.Tasks.Claim(taskCtx, id)
		return h.taskResult("claim", task, err, nil)
	case "release":
		id, err := requireTaskID(input.TaskID)
		if err != nil {
			return Result{}, err
		}
		task, err := call.Tasks.Release(taskCtx, id)
		return h.taskResult("release", task, err, nil)
	case "submit":
		id, err := requireTaskID(input.TaskID)
		if err != nil {
			return Result{}, err
		}
		summary, err := requireString(input.Summary, "summary")
		if err != nil {
			return Result{}, err
		}
		task, err := call.Tasks.Complete(taskCtx, taskapp.CompleteTaskParams{TaskID: id, Summary: summary, Artifacts: input.Artifacts})
		return h.taskResult("submit", task, err, nil)
	case "block":
		id, err := requireTaskID(input.TaskID)
		if err != nil {
			return Result{}, err
		}
		reason, err := requireString(input.Reason, "reason")
		if err != nil {
			return Result{}, err
		}
		task, err := call.Tasks.Block(taskCtx, id, reason)
		return h.taskResult("block", task, err, nil)
	case "unblock":
		id, err := requireTaskID(input.TaskID)
		if err != nil {
			return Result{}, err
		}
		task, err := call.Tasks.Unblock(taskCtx, id, input.Note)
		return h.taskResult("unblock", task, err, nil)
	case "reassign":
		id, err := requireTaskID(input.TaskID)
		if err != nil {
			return Result{}, err
		}
		assignee, err := h.assignee(ctx, call, actor, input.AssigneeMemberID)
		if err != nil {
			return Result{}, err
		}
		task, err := call.Tasks.Assign(taskCtx, taskapp.AssignTaskParams{TaskID: id, AssignedTo: assignee.MemberID})
		return h.taskResult("reassign", task, err, map[string]any{"assignee": assignee})
	case "cancel":
		id, err := requireTaskID(input.TaskID)
		if err != nil {
			return Result{}, err
		}
		reason, err := requireString(input.Reason, "reason")
		if err != nil {
			return Result{}, err
		}
		task, err := call.Tasks.Cancel(taskCtx, id, reason)
		return h.taskResult("cancel", task, err, nil)
	case "review":
		id, err := requireTaskID(input.TaskID)
		if err != nil {
			return Result{}, err
		}
		params := taskapp.ReviewTaskParams{TaskID: id, Reason: input.Reason, Criteria: reviewCriteria(input.Criteria)}
		var task taskdomain.Task
		switch input.Decision {
		case "approve":
			task, err = call.Tasks.ApproveReview(taskCtx, params)
		case "retry":
			reason, err := requireString(params.Reason, "reason")
			if err != nil {
				return Result{}, err
			}
			params.Reason = reason
			task, err = call.Tasks.RetryReview(taskCtx, params)
		case "fail":
			reason, err := requireString(params.Reason, "reason")
			if err != nil {
				return Result{}, err
			}
			params.Reason = reason
			task, err = call.Tasks.FailReview(taskCtx, params)
		default:
			return Result{}, fmt.Errorf("task: decision must be approve, retry, or fail")
		}
		return h.taskResult("review", task, err, map[string]any{"decision": input.Decision})
	default:
		return Result{}, fmt.Errorf("task: unsupported action %q", input.Action)
	}
}

type actor struct {
	MemberID   member.ID           `json:"memberId"`
	UserID     string              `json:"userId"`
	SpaceID    spacedomain.SpaceID `json:"spaceId"`
	MemberType string              `json:"memberType"`
	Label      string              `json:"label"`
}

func contextWithSessionActor(ctx context.Context, actorMemberID, spaceID string) context.Context {
	actorMemberID = strings.TrimSpace(actorMemberID)
	spaceID = strings.TrimSpace(spaceID)
	if actorMemberID == "" && spaceID == "" {
		return ctx
	}
	return caller.ContextWithCaller(ctx, caller.Caller{
		MemberID: member.ID(actorMemberID),
		SpaceID:  spacedomain.SpaceID(spaceID),
	})
}

func (h Handler) actor(ctx context.Context, call CallContext) (actor, error) {
	memberID := strings.TrimSpace(call.ActorMemberID)
	if memberID == "" {
		return actor{}, fmt.Errorf("task: caller member is required")
	}
	rosterMember, err := call.Members.GetMember(ctx, member.ID(memberID))
	if err != nil {
		return actor{}, fmt.Errorf("task: load caller member: %w", err)
	}
	if strings.TrimSpace(rosterMember.LifecycleState) != "" && !strings.EqualFold(rosterMember.LifecycleState, member.LifecycleActive) {
		return actor{}, fmt.Errorf("task: caller member %q is not active", memberID)
	}
	spaceID := strings.TrimSpace(call.SpaceID)
	if spaceID == "" {
		spaceID = strings.TrimSpace(string(rosterMember.SpaceID))
	}
	if spaceID == "" {
		return actor{}, fmt.Errorf("task: caller space is required")
	}
	return actor{
		MemberID:   member.ID(memberID),
		UserID:     strings.TrimSpace(rosterMember.UserID),
		SpaceID:    spacedomain.SpaceID(spaceID),
		MemberType: strings.TrimSpace(rosterMember.MemberType),
		Label:      memberLabel(rosterMember),
	}, nil
}

func (h Handler) assignee(ctx context.Context, call CallContext, caller actor, memberID string) (actor, error) {
	memberID = strings.TrimSpace(memberID)
	if memberID == "" {
		return actor{}, fmt.Errorf("task: assignee_member_id is required")
	}
	rosterMember, err := call.Members.GetMember(ctx, member.ID(memberID))
	if err != nil {
		return actor{}, fmt.Errorf("task: load assignee member: %w", err)
	}
	if strings.TrimSpace(rosterMember.LifecycleState) != "" && !strings.EqualFold(rosterMember.LifecycleState, member.LifecycleActive) {
		return actor{}, fmt.Errorf("task: assignee member %q is not active", memberID)
	}
	if strings.TrimSpace(string(rosterMember.SpaceID)) != strings.TrimSpace(string(caller.SpaceID)) {
		return actor{}, fmt.Errorf("task: assignee member %q belongs to space %q, not %q", memberID, rosterMember.SpaceID, caller.SpaceID)
	}
	return actor{
		MemberID:   member.ID(memberID),
		SpaceID:    spacedomain.SpaceID(strings.TrimSpace(string(rosterMember.SpaceID))),
		MemberType: strings.TrimSpace(rosterMember.MemberType),
		Label:      memberLabel(rosterMember),
	}, nil
}

func (h Handler) listFilter(caller actor, input requestInput) (taskdomain.TaskFilter, error) {
	filter := taskdomain.TaskFilter{
		SpaceID: spacedomain.SpaceID(strings.TrimSpace(string(caller.SpaceID))),
		Limit:   input.Limit,
		Offset:  input.Offset,
	}
	if input.Status != "" {
		status, err := parseStatus(input.Status)
		if err != nil {
			return taskdomain.TaskFilter{}, err
		}
		filter.Status = []taskdomain.TaskStatus{status}
	}
	if isCoordinatorType(caller.MemberType) {
		if input.AssigneeMemberID != "" {
			filter.AssignedTo = member.ID(input.AssigneeMemberID)
		}
		return filter, nil
	}
	filter.AssignedTo = caller.MemberID
	return filter, nil
}

func (h Handler) canSeeTask(caller actor, task taskdomain.Task) error {
	if task.SpaceID != caller.SpaceID {
		return fmt.Errorf("task: task %s is not visible in space %s", task.ID, caller.SpaceID)
	}
	if isCoordinatorType(caller.MemberType) {
		return nil
	}
	if task.AssignedTo == caller.MemberID || task.ClaimedByMemberID == caller.MemberID {
		return nil
	}
	return fmt.Errorf("task: task %s is not visible to member %s", task.ID, caller.MemberID)
}

func requireTaskID(value string) (taskdomain.TaskID, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("task: task_id is required")
	}
	return taskdomain.TaskID(value), nil
}

func requireString(value, field string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("task: %s is required", field)
	}
	return value, nil
}

func parseOptionalUUID(field, value string) (*uuid.UUID, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	parsed, err := uuid.Parse(value)
	if err != nil {
		return nil, fmt.Errorf("task: %s must be a UUID", field)
	}
	return &parsed, nil
}

func parseStatus(value string) (taskdomain.TaskStatus, error) {
	switch taskdomain.TaskStatus(strings.TrimSpace(value)) {
	case taskdomain.TaskStatusPending, taskdomain.TaskStatusActive, taskdomain.TaskStatusBlocked, taskdomain.TaskStatusInReview, taskdomain.TaskStatusSucceeded, taskdomain.TaskStatusFailed, taskdomain.TaskStatusCanceled:
		return taskdomain.TaskStatus(value), nil
	default:
		return "", fmt.Errorf("task: unsupported status %q", value)
	}
}

func reviewCriteria(values []reviewCriterionInput) []taskdomain.CriterionReview {
	if len(values) == 0 {
		return nil
	}
	out := make([]taskdomain.CriterionReview, 0, len(values))
	for _, value := range values {
		out = append(out, taskdomain.CriterionReview{ID: strings.TrimSpace(value.ID), Satisfied: value.Satisfied})
	}
	return out
}

func isCoordinatorType(memberType string) bool {
	return strings.TrimSpace(memberType) == member.TypeCoordinator
}
