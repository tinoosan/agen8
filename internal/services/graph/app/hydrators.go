package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	decisionapp "github.com/tinoosan/agen8-mcp-server/internal/services/decision/app"
	decisiondomain "github.com/tinoosan/agen8-mcp-server/internal/services/decision/domain"
	"github.com/tinoosan/agen8-mcp-server/internal/services/graph/domain"
	missionapp "github.com/tinoosan/agen8-mcp-server/internal/services/mission/app"
	krdomain "github.com/tinoosan/agen8-mcp-server/internal/services/mission/domain/kr"
	missiondomain "github.com/tinoosan/agen8-mcp-server/internal/services/mission/domain/mission"
	operatordomain "github.com/tinoosan/agen8-mcp-server/internal/services/operator/domain"
	spacedomain "github.com/tinoosan/agen8-mcp-server/internal/services/space/domain"
	taskdomain "github.com/tinoosan/agen8-mcp-server/internal/services/task/domain"
	implstore "github.com/tinoosan/agen8-mcp-server/internal/store"
)

type TaskReader interface {
	Get(ctx context.Context, taskID taskdomain.TaskID) (taskdomain.Task, error)
	List(ctx context.Context, filter taskdomain.TaskFilter) ([]taskdomain.Task, error)
}

type OperatorReader interface {
	Get(ctx context.Context, id operatordomain.OperatorActionID) (operatordomain.OperatorAction, error)
	List(ctx context.Context, projectID string, filter operatordomain.ActionFilter) ([]operatordomain.OperatorAction, error)
	GetEscalation(ctx context.Context, id operatordomain.EscalationID) (operatordomain.Escalation, error)
	ListEscalations(ctx context.Context, projectID string, filter operatordomain.EscalationFilter) ([]operatordomain.Escalation, error)
}

func DefaultHydrators(
	taskSvc TaskReader,
	decisionSvc *decisionapp.Service,
	missionSvc *missionapp.Service,
	operatorSvc OperatorReader,
	spaceID string,
) []domain.NodeHydrator {
	out := make([]domain.NodeHydrator, 0, 6)
	if taskSvc != nil {
		out = append(out, taskHydrator{tasks: taskSvc, spaceID: strings.TrimSpace(spaceID)})
	}
	if decisionSvc != nil {
		out = append(out, decisionHydrator{decisions: decisionSvc})
	}
	if missionSvc != nil {
		out = append(out, missionHydrator{missions: missionSvc})
		out = append(out, keyResultHydrator{missions: missionSvc})
	}
	if operatorSvc != nil {
		out = append(out, operatorActionHydrator{operator: operatorSvc})
		out = append(out, escalationHydrator{operator: operatorSvc})
	}
	return out
}

type taskHydrator struct {
	tasks   TaskReader
	spaceID string
}

func (h taskHydrator) NodeType() string { return domain.NodeTypeTask }

func (h taskHydrator) Fetch(ctx context.Context, _ string, nodeID string) (domain.GraphNodeCore, error) {
	task, err := h.tasks.Get(ctx, taskdomain.TaskID(strings.TrimSpace(nodeID)))
	if err != nil {
		return domain.GraphNodeCore{}, err
	}
	if !taskVisibleToScope(task, h.spaceID) {
		return domain.GraphNodeCore{}, fmt.Errorf("task %q not found", strings.TrimSpace(nodeID))
	}
	createdAt, err := taskCreatedAt(task)
	if err != nil {
		return domain.GraphNodeCore{}, err
	}
	title := strings.TrimSpace(task.Title)
	if title == "" {
		title = strings.TrimSpace(task.Description)
	}
	return domain.GraphNodeCore{
		ID:        strings.TrimSpace(string(task.ID)),
		Type:      domain.NodeTypeTask,
		Title:     title,
		Status:    strings.TrimSpace(string(task.Status)),
		ScopeID:   strings.TrimSpace(string(task.SpaceID)),
		CreatedAt: createdAt.Format(time.RFC3339Nano),
		Fields: map[string]any{
			"description": strings.TrimSpace(task.Description),
			"assigneeRef": strings.TrimSpace(string(task.AssignedTo)),
			"taskKind":    strings.TrimSpace(task.TaskKind),
			"dueDate":     taskDueDate(task),
		},
	}, nil
}

func (h taskHydrator) FetchMany(ctx context.Context, projectID string, nodeIDs []string) ([]domain.GraphNodeSummary, error) {
	return fetchManyUsingFetch(ctx, h, projectID, nodeIDs)
}

func (h taskHydrator) Search(ctx context.Context, _ string, query string, limit int) ([]domain.GraphNodeSummary, error) {
	tasks, err := h.tasks.List(ctx, taskdomain.TaskFilter{
		SpaceID: spacedomain.SpaceID(strings.TrimSpace(h.spaceID)),
		Limit:   max(200, limit*20),
	})
	if err != nil {
		return nil, err
	}
	out := make([]domain.GraphNodeSummary, 0, limit)
	for _, task := range tasks {
		if !taskVisibleToScope(task, h.spaceID) {
			continue
		}
		title := strings.TrimSpace(task.Title)
		if title == "" {
			title = strings.TrimSpace(task.Description)
		}
		if !matchesQuery(query, title, task.Description, string(task.ID)) {
			continue
		}
		createdAt, err := taskCreatedAt(task)
		if err != nil {
			continue
		}
		out = append(out, domain.GraphNodeSummary{
			ID:        strings.TrimSpace(string(task.ID)),
			Type:      domain.NodeTypeTask,
			Title:     title,
			Status:    strings.TrimSpace(string(task.Status)),
			ScopeID:   strings.TrimSpace(string(task.SpaceID)),
			CreatedAt: createdAt.Format(time.RFC3339Nano),
		})
		if len(out) >= limit {
			break
		}
	}
	if out == nil {
		return []domain.GraphNodeSummary{}, nil
	}
	return out, nil
}

type decisionHydrator struct {
	decisions *decisionapp.Service
}

func (h decisionHydrator) NodeType() string { return domain.NodeTypeDecision }

func (h decisionHydrator) Fetch(ctx context.Context, projectID string, nodeID string) (domain.GraphNodeCore, error) {
	decision, err := h.decisions.Get(ctx, decisiondomain.DecisionID(strings.TrimSpace(nodeID)))
	if err != nil {
		return domain.GraphNodeCore{}, err
	}
	if !strings.EqualFold(strings.TrimSpace(projectID), strings.TrimSpace(decision.ProjectID)) {
		return domain.GraphNodeCore{}, fmt.Errorf("decision %q not found", strings.TrimSpace(nodeID))
	}
	fields := map[string]any{
		"confidence":        decision.Confidence,
		"operatorActionRef": strings.TrimSpace(decision.OperatorActionRef),
		"escalationRef":     strings.TrimSpace(decision.EscalationRef),
		"source":            strings.TrimSpace(string(decision.Source)),
		"sourceIdentity":    strings.TrimSpace(decision.SourceIdentity),
		"correlationRef":    strings.TrimSpace(decision.CorrelationRef),
		"informedByRef":     strings.TrimSpace(decision.InformedByRef),
		"kind":              strings.TrimSpace(string(decision.Kind())),
		"taskRef":           strings.TrimSpace(decision.TaskRef),
		"keyResultRef":      strings.TrimSpace(decision.KeyResultRef),
		"missionRef":        strings.TrimSpace(decision.MissionRef),
		"planRef":           strings.TrimSpace(decision.PlanRef),
	}
	if p := decision.Log; p != nil {
		fields["rationale"] = strings.TrimSpace(p.Rationale)
		fields["alternativesRejected"] = strings.TrimSpace(p.AlternativesRejected)
		fields["invalidationConditions"] = append([]string(nil), p.InvalidationConditions...)
	}
	if p := decision.AskUser; p != nil {
		fields["cancelled"] = p.Cancelled
		fields["questionCount"] = len(p.Questions)
		fields["answerCount"] = len(p.Answers)
	}
	return domain.GraphNodeCore{
		ID:        strings.TrimSpace(string(decision.ID)),
		Type:      domain.NodeTypeDecision,
		Title:     strings.TrimSpace(decision.Title),
		Status:    decisionStatus(decision),
		ScopeID:   strings.TrimSpace(decision.SpaceID),
		CreatedAt: decision.CreatedAt.UTC().Format(time.RFC3339Nano),
		Fields:    fields,
	}, nil
}

func (h decisionHydrator) FetchMany(ctx context.Context, projectID string, nodeIDs []string) ([]domain.GraphNodeSummary, error) {
	return fetchManyUsingFetch(ctx, h, projectID, nodeIDs)
}

func (h decisionHydrator) Search(ctx context.Context, projectID, query string, limit int) ([]domain.GraphNodeSummary, error) {
	items, err := h.decisions.List(ctx, decisiondomain.DecisionFilter{
		ProjectID: strings.TrimSpace(projectID),
		Query:     strings.TrimSpace(query),
		SortDesc:  true,
		Limit:     max(limit*5, 100),
	})
	if err != nil {
		return nil, err
	}
	out := make([]domain.GraphNodeSummary, 0, len(items))
	for _, item := range items {
		out = append(out, domain.GraphNodeSummary{
			ID:        strings.TrimSpace(string(item.ID)),
			Type:      domain.NodeTypeDecision,
			Title:     strings.TrimSpace(item.Title),
			Status:    decisionStatus(item),
			ScopeID:   strings.TrimSpace(item.SpaceID),
			CreatedAt: item.CreatedAt.UTC().Format(time.RFC3339Nano),
		})
		if len(out) >= limit {
			break
		}
	}
	if out == nil {
		return []domain.GraphNodeSummary{}, nil
	}
	return out, nil
}

type missionHydrator struct {
	missions *missionapp.Service
}

func (h missionHydrator) NodeType() string { return domain.NodeTypeMission }

func (h missionHydrator) Fetch(ctx context.Context, projectID string, nodeID string) (domain.GraphNodeCore, error) {
	mission, err := h.missions.GetMission(ctx, missiondomain.MissionID(strings.TrimSpace(nodeID)))
	if err != nil {
		return domain.GraphNodeCore{}, err
	}
	if !strings.EqualFold(strings.TrimSpace(projectID), strings.TrimSpace(mission.ProjectID)) {
		return domain.GraphNodeCore{}, fmt.Errorf("mission %q not found", strings.TrimSpace(nodeID))
	}
	return domain.GraphNodeCore{
		ID:        strings.TrimSpace(string(mission.ID)),
		Type:      domain.NodeTypeMission,
		Title:     strings.TrimSpace(mission.Title),
		Status:    strings.TrimSpace(string(mission.Status)),
		ScopeID:   "",
		CreatedAt: mission.CreatedAt.UTC().Format(time.RFC3339Nano),
		Fields: map[string]any{
			"description": strings.TrimSpace(mission.Description),
			"startDate":   timePtrRFC3339(mission.StartDate),
			"endDate":     timePtrRFC3339(mission.EndDate),
		},
	}, nil
}

func (h missionHydrator) FetchMany(ctx context.Context, projectID string, nodeIDs []string) ([]domain.GraphNodeSummary, error) {
	return fetchManyUsingFetch(ctx, h, projectID, nodeIDs)
}

func (h missionHydrator) Search(ctx context.Context, projectID, query string, limit int) ([]domain.GraphNodeSummary, error) {
	items, err := h.missions.ListMissions(ctx, strings.TrimSpace(projectID), missiondomain.MissionFilter{
		Limit: max(limit*5, 100),
	})
	if err != nil {
		return nil, err
	}
	out := make([]domain.GraphNodeSummary, 0, len(items))
	for _, item := range items {
		if !matchesQuery(query, item.Title, item.Description, string(item.ID)) {
			continue
		}
		out = append(out, domain.GraphNodeSummary{
			ID:        strings.TrimSpace(string(item.ID)),
			Type:      domain.NodeTypeMission,
			Title:     strings.TrimSpace(item.Title),
			Status:    strings.TrimSpace(string(item.Status)),
			ScopeID:   "",
			CreatedAt: item.CreatedAt.UTC().Format(time.RFC3339Nano),
		})
		if len(out) >= limit {
			break
		}
	}
	if out == nil {
		return []domain.GraphNodeSummary{}, nil
	}
	return out, nil
}

type keyResultHydrator struct {
	missions *missionapp.Service
}

func (h keyResultHydrator) NodeType() string { return domain.NodeTypeKeyResult }

func (h keyResultHydrator) Fetch(ctx context.Context, projectID string, nodeID string) (domain.GraphNodeCore, error) {
	kr, err := h.missions.GetKeyResult(ctx, krdomain.KeyResultID(strings.TrimSpace(nodeID)))
	if err != nil {
		return domain.GraphNodeCore{}, err
	}
	mission, err := h.missions.GetMission(ctx, kr.MissionID)
	if err != nil {
		return domain.GraphNodeCore{}, err
	}
	if !strings.EqualFold(strings.TrimSpace(projectID), strings.TrimSpace(mission.ProjectID)) {
		return domain.GraphNodeCore{}, fmt.Errorf("key_result %q not found", strings.TrimSpace(nodeID))
	}
	return domain.GraphNodeCore{
		ID:        strings.TrimSpace(string(kr.ID)),
		Type:      domain.NodeTypeKeyResult,
		Title:     strings.TrimSpace(kr.Title),
		Status:    strings.TrimSpace(string(kr.Status)),
		ScopeID:   strings.TrimSpace(kr.SpaceID),
		CreatedAt: kr.CreatedAt.UTC().Format(time.RFC3339Nano),
		Fields: map[string]any{
			"measurementType": strings.TrimSpace(string(kr.MeasurementType)),
			"direction":       strings.TrimSpace(string(kr.Direction)),
			"unit":            strings.TrimSpace(kr.Unit),
			"targetValue":     kr.TargetValue,
			"currentValue":    kr.CurrentValue,
			"baseline":        floatPtrValue(kr.Baseline),
			"missionId":       strings.TrimSpace(string(kr.MissionID)),
		},
	}, nil
}

func (h keyResultHydrator) FetchMany(ctx context.Context, projectID string, nodeIDs []string) ([]domain.GraphNodeSummary, error) {
	return fetchManyUsingFetch(ctx, h, projectID, nodeIDs)
}

func (h keyResultHydrator) Search(ctx context.Context, projectID, query string, limit int) ([]domain.GraphNodeSummary, error) {
	missions, err := h.missions.ListMissions(ctx, strings.TrimSpace(projectID), missiondomain.MissionFilter{
		Limit: max(limit*5, 100),
	})
	if err != nil {
		return nil, err
	}
	out := make([]domain.GraphNodeSummary, 0, limit)
	for _, mission := range missions {
		krs, listErr := h.missions.ListKeyResults(ctx, mission.ID)
		if listErr != nil {
			continue
		}
		for _, kr := range krs {
			if !matchesQuery(query, kr.Title, kr.Description, string(kr.ID), string(kr.MissionID)) {
				continue
			}
			out = append(out, domain.GraphNodeSummary{
				ID:        strings.TrimSpace(string(kr.ID)),
				Type:      domain.NodeTypeKeyResult,
				Title:     strings.TrimSpace(kr.Title),
				Status:    strings.TrimSpace(string(kr.Status)),
				ScopeID:   strings.TrimSpace(kr.SpaceID),
				CreatedAt: kr.CreatedAt.UTC().Format(time.RFC3339Nano),
			})
			if len(out) >= limit {
				return out, nil
			}
		}
	}
	if out == nil {
		return []domain.GraphNodeSummary{}, nil
	}
	return out, nil
}

type operatorActionHydrator struct {
	operator OperatorReader
}

func (h operatorActionHydrator) NodeType() string { return domain.NodeTypeOperatorAction }

func (h operatorActionHydrator) Fetch(ctx context.Context, projectID string, nodeID string) (domain.GraphNodeCore, error) {
	action, err := h.operator.Get(ctx, operatordomain.OperatorActionID(strings.TrimSpace(nodeID)))
	if err != nil {
		return domain.GraphNodeCore{}, err
	}
	if !strings.EqualFold(strings.TrimSpace(projectID), strings.TrimSpace(action.ProjectID)) {
		return domain.GraphNodeCore{}, fmt.Errorf("operator_action %q not found", strings.TrimSpace(nodeID))
	}
	return domain.GraphNodeCore{
		ID:        strings.TrimSpace(string(action.ID)),
		Type:      domain.NodeTypeOperatorAction,
		Title:     strings.TrimSpace(action.Title),
		Status:    strings.TrimSpace(string(action.Status)),
		ScopeID:   strings.TrimSpace(action.SpaceID),
		CreatedAt: action.CreatedAt.UTC().Format(time.RFC3339Nano),
		Fields: map[string]any{
			"description":          strings.TrimSpace(action.Description),
			"urgency":              strings.TrimSpace(string(action.Urgency)),
			"blocking":             action.Blocking,
			"requiresVerification": action.RequiresVerification,
			"deadlineHours":        deadlineHours(action.CreatedAt, action.Deadline),
		},
	}, nil
}

func (h operatorActionHydrator) FetchMany(ctx context.Context, projectID string, nodeIDs []string) ([]domain.GraphNodeSummary, error) {
	return fetchManyUsingFetch(ctx, h, projectID, nodeIDs)
}

func (h operatorActionHydrator) Search(ctx context.Context, projectID, query string, limit int) ([]domain.GraphNodeSummary, error) {
	items, err := h.operator.List(ctx, strings.TrimSpace(projectID), operatordomain.ActionFilter{
		Limit: max(limit*5, 100),
	})
	if err != nil {
		return nil, err
	}
	out := make([]domain.GraphNodeSummary, 0, len(items))
	for _, item := range items {
		if !matchesQuery(query, item.Title, item.Description, string(item.ID), string(item.Category)) {
			continue
		}
		out = append(out, domain.GraphNodeSummary{
			ID:        strings.TrimSpace(string(item.ID)),
			Type:      domain.NodeTypeOperatorAction,
			Title:     strings.TrimSpace(item.Title),
			Status:    strings.TrimSpace(string(item.Status)),
			ScopeID:   strings.TrimSpace(item.SpaceID),
			CreatedAt: item.CreatedAt.UTC().Format(time.RFC3339Nano),
		})
		if len(out) >= limit {
			break
		}
	}
	if out == nil {
		return []domain.GraphNodeSummary{}, nil
	}
	return out, nil
}

type escalationHydrator struct {
	operator OperatorReader
}

func (h escalationHydrator) NodeType() string { return domain.NodeTypeEscalation }

func (h escalationHydrator) Fetch(ctx context.Context, projectID string, nodeID string) (domain.GraphNodeCore, error) {
	escalation, err := h.operator.GetEscalation(ctx, operatordomain.EscalationID(strings.TrimSpace(nodeID)))
	if err != nil {
		return domain.GraphNodeCore{}, err
	}
	if !strings.EqualFold(strings.TrimSpace(projectID), strings.TrimSpace(escalation.ProjectID)) {
		return domain.GraphNodeCore{}, fmt.Errorf("escalation %q not found", strings.TrimSpace(nodeID))
	}
	return domain.GraphNodeCore{
		ID:        strings.TrimSpace(string(escalation.ID)),
		Type:      domain.NodeTypeEscalation,
		Title:     strings.TrimSpace(escalation.Title),
		Status:    strings.TrimSpace(string(escalation.Status)),
		ScopeID:   strings.TrimSpace(escalation.SpaceID),
		CreatedAt: escalation.CreatedAt.UTC().Format(time.RFC3339Nano),
		Fields: map[string]any{
			"category":       strings.TrimSpace(string(escalation.Category)),
			"urgency":        strings.TrimSpace(string(escalation.Urgency)),
			"description":    strings.TrimSpace(escalation.Description),
			"recommendation": strings.TrimSpace(escalation.Recommendation),
			"confidence":     escalation.Confidence,
			"deadlineHours":  deadlineHours(escalation.CreatedAt, escalation.Deadline),
		},
	}, nil
}

func (h escalationHydrator) FetchMany(ctx context.Context, projectID string, nodeIDs []string) ([]domain.GraphNodeSummary, error) {
	return fetchManyUsingFetch(ctx, h, projectID, nodeIDs)
}

func (h escalationHydrator) Search(ctx context.Context, projectID, query string, limit int) ([]domain.GraphNodeSummary, error) {
	items, err := h.operator.ListEscalations(ctx, strings.TrimSpace(projectID), operatordomain.EscalationFilter{
		Limit: max(limit*5, 100),
	})
	if err != nil {
		return nil, err
	}
	out := make([]domain.GraphNodeSummary, 0, len(items))
	for _, item := range items {
		if !matchesQuery(query, item.Title, item.Description, string(item.ID), string(item.Category)) {
			continue
		}
		out = append(out, domain.GraphNodeSummary{
			ID:        strings.TrimSpace(string(item.ID)),
			Type:      domain.NodeTypeEscalation,
			Title:     strings.TrimSpace(item.Title),
			Status:    strings.TrimSpace(string(item.Status)),
			ScopeID:   strings.TrimSpace(item.SpaceID),
			CreatedAt: item.CreatedAt.UTC().Format(time.RFC3339Nano),
		})
		if len(out) >= limit {
			break
		}
	}
	if out == nil {
		return []domain.GraphNodeSummary{}, nil
	}
	return out, nil
}

func fetchManyUsingFetch(
	ctx context.Context,
	hydrator domain.NodeHydrator,
	projectID string,
	nodeIDs []string,
) ([]domain.GraphNodeSummary, error) {
	out := make([]domain.GraphNodeSummary, 0, len(nodeIDs))
	for _, nodeID := range nodeIDs {
		core, err := hydrator.Fetch(ctx, projectID, nodeID)
		if err != nil {
			if isNotFoundError(err) {
				continue
			}
			return nil, err
		}
		out = append(out, domain.GraphNodeSummary{
			ID:        strings.TrimSpace(core.ID),
			Type:      normalizeNodeType(core.Type),
			Title:     strings.TrimSpace(core.Title),
			Status:    strings.TrimSpace(core.Status),
			ScopeID:   strings.TrimSpace(core.ScopeID),
			CreatedAt: strings.TrimSpace(core.CreatedAt),
		})
	}
	if out == nil {
		return []domain.GraphNodeSummary{}, nil
	}
	return out, nil
}

func taskVisibleToScope(task taskdomain.Task, spaceID string) bool {
	spaceID = strings.TrimSpace(spaceID)
	if spaceID == "" {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(string(task.SpaceID)), spaceID)
}

func taskCreatedAt(task taskdomain.Task) (time.Time, error) {
	if task.CreatedAt != nil && !task.CreatedAt.IsZero() {
		return task.CreatedAt.UTC(), nil
	}
	return time.Time{}, fmt.Errorf("task %q missing createdAt", strings.TrimSpace(string(task.ID)))
}

func taskDueDate(task taskdomain.Task) string {
	if task.Metadata == nil {
		return ""
	}
	if dueRaw, ok := task.Metadata["dueDate"]; ok {
		return strings.TrimSpace(fmt.Sprintf("%v", dueRaw))
	}
	if dueRaw, ok := task.Metadata["due_date"]; ok {
		return strings.TrimSpace(fmt.Sprintf("%v", dueRaw))
	}
	return ""
}

func decisionStatus(decision decisiondomain.Decision) string {
	if decision.AskUser != nil && decision.AskUser.Cancelled {
		return "cancelled"
	}
	if kind := decision.Kind(); kind != "" {
		return string(kind)
	}
	return "recorded"
}

func matchesQuery(query string, values ...string) bool {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return true
	}
	for _, value := range values {
		if strings.Contains(strings.ToLower(strings.TrimSpace(value)), query) {
			return true
		}
	}
	return false
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func floatPtrValue(value *float64) any {
	if value == nil {
		return nil
	}
	return *value
}

func timePtrRFC3339(value *time.Time) string {
	if value == nil || value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func deadlineHours(createdAt time.Time, deadline *time.Time) any {
	if deadline == nil || deadline.IsZero() {
		return nil
	}
	if createdAt.IsZero() {
		return 0
	}
	return int(deadline.UTC().Sub(createdAt.UTC()).Hours())
}

func isNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, implstore.ErrNotFound) {
		return true
	}
	return strings.Contains(strings.ToLower(err.Error()), "not found")
}
