package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/tinoosan/agen8-mcp-server/internal/core/types"
	decisionapp "github.com/tinoosan/agen8-mcp-server/internal/services/decision/app"
	decisiondomain "github.com/tinoosan/agen8-mcp-server/internal/services/decision/domain"
	"github.com/tinoosan/agen8-mcp-server/internal/services/graph/domain"
	missionapp "github.com/tinoosan/agen8-mcp-server/internal/services/mission/app"
	krdomain "github.com/tinoosan/agen8-mcp-server/internal/services/mission/domain/kr"
	missiondomain "github.com/tinoosan/agen8-mcp-server/internal/services/mission/domain/mission"
	taskdomain "github.com/tinoosan/agen8-mcp-server/internal/services/task/domain"
	implstore "github.com/tinoosan/agen8-mcp-server/internal/store"
)

type TaskReader interface {
	Get(ctx context.Context, taskID taskdomain.TaskID) (taskdomain.Task, error)
	List(ctx context.Context, filter taskdomain.TaskFilter) ([]taskdomain.Task, error)
}

func DefaultHydrators(
	taskSvc TaskReader,
	decisionSvc *decisionapp.Service,
	missionSvc *missionapp.Service,
) []domain.NodeHydrator {
	out := make([]domain.NodeHydrator, 0, 4)
	if taskSvc != nil {
		out = append(out, taskHydrator{tasks: taskSvc})
	}
	if decisionSvc != nil {
		out = append(out, decisionHydrator{decisions: decisionSvc})
	}
	if missionSvc != nil {
		out = append(out, missionHydrator{missions: missionSvc})
		out = append(out, keyResultHydrator{missions: missionSvc})
	}
	return out
}

type taskHydrator struct {
	tasks TaskReader
}

func (h taskHydrator) NodeType() string { return domain.NodeTypeTask }

func (h taskHydrator) Fetch(ctx context.Context, projectID string, nodeID string) (domain.GraphNodeCore, error) {
	task, err := h.tasks.Get(ctx, taskdomain.TaskID(strings.TrimSpace(nodeID)))
	if err != nil {
		return domain.GraphNodeCore{}, err
	}
	if !taskVisibleToProject(task, projectID) {
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
		ScopeID:   strings.TrimSpace(string(task.ProjectID)),
		CreatedAt: createdAt.Format(time.RFC3339Nano),
		Fields: map[string]any{
			"description":          strings.TrimSpace(task.Description),
			"assigneeRef":          strings.TrimSpace(string(task.AssignedTo)),
			"assignedTo":           strings.TrimSpace(string(task.AssignedTo)),
			"assignedToLabel":      strings.TrimSpace(task.AssignedToLabel),
			"claimedByMemberId":    strings.TrimSpace(string(task.ClaimedByMemberID)),
			"claimedByMemberLabel": strings.TrimSpace(task.ClaimedByMemberLabel),
			"createdBy":            strings.TrimSpace(task.CreatedBy),
			"createdByLabel":       strings.TrimSpace(task.CreatedByLabel),
			"taskKind":             strings.TrimSpace(task.TaskKind),
			"dueDate":              taskDueDate(task),
		},
	}, nil
}

func (h taskHydrator) FetchMany(ctx context.Context, projectID string, nodeIDs []string) ([]domain.GraphNodeSummary, error) {
	return fetchManyUsingFetch(ctx, h, projectID, nodeIDs)
}

func (h taskHydrator) Search(ctx context.Context, projectID string, query string, limit int) ([]domain.GraphNodeSummary, error) {
	tasks, err := h.tasks.List(ctx, taskdomain.TaskFilter{
		ProjectID: types.ProjectID(strings.TrimSpace(projectID)),
		Limit:     max(200, limit*20),
	})
	if err != nil {
		return nil, err
	}
	out := make([]domain.GraphNodeSummary, 0, limit)
	for _, task := range tasks {
		if !taskVisibleToProject(task, projectID) {
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
			ScopeID:   strings.TrimSpace(string(task.ProjectID)),
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
		"confidence":     decision.Confidence,
		"source":         strings.TrimSpace(string(decision.Source)),
		"sourceIdentity": strings.TrimSpace(decision.SourceIdentity),
		"correlationRef": strings.TrimSpace(decision.CorrelationRef),
		"informedByRef":  strings.TrimSpace(decision.InformedByRef),
		"kind":           strings.TrimSpace(string(decision.Kind())),
		"taskRef":        strings.TrimSpace(decision.TaskRef),
		"keyResultRef":   strings.TrimSpace(decision.KeyResultRef),
		"missionRef":     strings.TrimSpace(decision.MissionRef),
	}
	if p := decision.Log; p != nil {
		fields["rationale"] = strings.TrimSpace(p.Rationale)
		fields["context"] = strings.TrimSpace(p.Context)
		fields["alternativesRejected"] = strings.TrimSpace(p.AlternativesRejected)
		fields["invalidationConditions"] = append([]string(nil), p.InvalidationConditions...)
	}
	return domain.GraphNodeCore{
		ID:        strings.TrimSpace(string(decision.ID)),
		Type:      domain.NodeTypeDecision,
		Title:     strings.TrimSpace(decision.Title),
		Status:    decisionStatus(decision),
		ScopeID:   strings.TrimSpace(decision.ProjectID),
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
		SortDesc:  true,
		Limit:     max(limit*20, 200),
	})
	if err != nil {
		return nil, err
	}
	out := make([]domain.GraphNodeSummary, 0, len(items))
	for _, item := range items {
		var logValues []string
		if item.Log != nil {
			logValues = []string{
				item.Log.Rationale,
				item.Log.Context,
				item.Log.AlternativesRejected,
				strings.Join(item.Log.InvalidationConditions, " "),
				item.Log.Outcome,
			}
		}
		if !matchesQuery(query, append([]string{
			item.Title,
			string(item.ID),
			item.TaskRef,
			item.KeyResultRef,
			item.MissionRef,
		}, logValues...)...) {
			continue
		}
		out = append(out, domain.GraphNodeSummary{
			ID:        strings.TrimSpace(string(item.ID)),
			Type:      domain.NodeTypeDecision,
			Title:     strings.TrimSpace(item.Title),
			Status:    decisionStatus(item),
			ScopeID:   strings.TrimSpace(item.ProjectID),
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
		ScopeID:   strings.TrimSpace(kr.ProjectID),
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
				ScopeID:   strings.TrimSpace(kr.ProjectID),
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

func taskVisibleToProject(task taskdomain.Task, projectID string) bool {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(string(task.ProjectID)), projectID)
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
	if kind := decision.Kind(); kind != "" {
		return string(kind)
	}
	return "recorded"
}

func matchesQuery(query string, values ...string) bool {
	normalizedQuery := strings.ToLower(strings.TrimSpace(query))
	if normalizedQuery == "" {
		return true
	}
	tokens := searchTokens(normalizedQuery)
	for _, value := range values {
		normalizedValue := strings.ToLower(strings.TrimSpace(value))
		if normalizedValue == "" {
			continue
		}
		if strings.Contains(normalizedValue, normalizedQuery) {
			return true
		}
		if queryTokenMatchCount(tokens, normalizedValue) >= requiredTokenMatches(len(tokens)) {
			return true
		}
	}
	return false
}

func searchTokens(query string) []string {
	seen := map[string]struct{}{}
	raw := strings.FieldsFunc(strings.ToLower(strings.TrimSpace(query)), func(r rune) bool {
		return (r < 'a' || r > 'z') && (r < '0' || r > '9')
	})
	out := make([]string, 0, len(raw))
	for _, token := range raw {
		token = strings.TrimSpace(token)
		if len(token) < 3 {
			continue
		}
		if _, ok := seen[token]; ok {
			continue
		}
		seen[token] = struct{}{}
		out = append(out, token)
	}
	return out
}

func queryTokenMatchCount(tokens []string, normalizedValue string) int {
	if len(tokens) == 0 || strings.TrimSpace(normalizedValue) == "" {
		return 0
	}
	matches := 0
	for _, token := range tokens {
		if strings.Contains(normalizedValue, token) {
			matches++
		}
	}
	return matches
}

func requiredTokenMatches(tokenCount int) int {
	if tokenCount <= 0 {
		return 1
	}
	if tokenCount <= 2 {
		return 1
	}
	return 2
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
