package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/tinoosan/agen8/internal/services/graph/domain"
)

type TaskHydrationReader interface {
	GetTask(ctx context.Context, taskID string) (TaskHydrationRow, error)
	ListTasks(ctx context.Context, projectID string, limit int) ([]TaskHydrationRow, error)
}

type DecisionHydrationReader interface {
	GetDecision(ctx context.Context, decisionID string) (DecisionHydrationRow, error)
	ListDecisions(ctx context.Context, projectID string, limit int) ([]DecisionHydrationRow, error)
}

type MissionHydrationReader interface {
	GetMission(ctx context.Context, missionID string) (MissionHydrationRow, error)
	ListMissions(ctx context.Context, projectID string, limit int) ([]MissionHydrationRow, error)
	GetKeyResult(ctx context.Context, keyResultID string) (KeyResultHydrationRow, error)
	ListKeyResults(ctx context.Context, missionID string) ([]KeyResultHydrationRow, error)
}

type TaskHydrationRow struct {
	ID                   string
	ProjectID            string
	Description          string
	Title                string
	Status               string
	AssignedTo           string
	AssignedToLabel      string
	ClaimedByMemberID    string
	ClaimedByMemberLabel string
	CreatedBy            string
	CreatedByLabel       string
	TaskKind             string
	KeyResultRef         string
	Metadata             map[string]any
	CreatedAt            *time.Time
}

type DecisionHydrationRow struct {
	ID                     string
	ProjectID              string
	Source                 string
	Title                  string
	Confidence             float64
	CreatedAt              time.Time
	SourceIdentity         string
	CorrelationRef         string
	InformedByRef          string
	Kind                   string
	TaskRef                string
	KeyResultRef           string
	MissionRef             string
	Rationale              string
	Context                string
	AlternativesRejected   string
	InvalidationConditions []string
	Outcome                string
}

type MissionHydrationRow struct {
	ID          string
	ProjectID   string
	Title       string
	Description string
	Status      string
	CreatedAt   time.Time
	StartDate   *time.Time
	EndDate     *time.Time
}

type KeyResultHydrationRow struct {
	ID              string
	MissionID       string
	Title           string
	Description     string
	Status          string
	CreatedAt       time.Time
	MeasurementType string
	Direction       string
	Unit            string
	TargetValue     float64
	CurrentValue    float64
	Baseline        *float64
}

func DefaultHydrators(
	taskSvc TaskHydrationReader,
	decisionSvc DecisionHydrationReader,
	missionSvc MissionHydrationReader,
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
	tasks TaskHydrationReader
}

func (h taskHydrator) NodeType() string { return domain.NodeTypeTask }

func (h taskHydrator) Fetch(ctx context.Context, projectID string, nodeID string) (domain.GraphNodeCore, error) {
	task, err := h.tasks.GetTask(ctx, strings.TrimSpace(nodeID))
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
		ID:        strings.TrimSpace(task.ID),
		Type:      domain.NodeTypeTask,
		Title:     title,
		Status:    strings.TrimSpace(task.Status),
		ScopeID:   strings.TrimSpace(task.ProjectID),
		CreatedAt: createdAt.Format(time.RFC3339Nano),
		Fields: map[string]any{
			"description":          strings.TrimSpace(task.Description),
			"assigneeRef":          strings.TrimSpace(task.AssignedTo),
			"assignedTo":           strings.TrimSpace(task.AssignedTo),
			"assignedToLabel":      strings.TrimSpace(task.AssignedToLabel),
			"claimedByMemberId":    strings.TrimSpace(task.ClaimedByMemberID),
			"claimedByMemberLabel": strings.TrimSpace(task.ClaimedByMemberLabel),
			"createdBy":            strings.TrimSpace(task.CreatedBy),
			"createdByLabel":       strings.TrimSpace(task.CreatedByLabel),
			"taskKind":             strings.TrimSpace(task.TaskKind),
			"dueDate":              taskDueDate(task),
			"keyResultRef":         strings.TrimSpace(task.KeyResultRef),
			"missionRef":           taskMissionRef(task),
		},
	}, nil
}

func (h taskHydrator) FetchMany(ctx context.Context, projectID string, nodeIDs []string) ([]domain.GraphNodeSummary, error) {
	return fetchManyUsingFetch(ctx, h, projectID, nodeIDs)
}

func (h taskHydrator) Search(ctx context.Context, projectID string, query string, limit int) ([]domain.GraphNodeSummary, error) {
	tasks, err := h.tasks.ListTasks(ctx, strings.TrimSpace(projectID), max(200, limit*20))
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
		if !matchesQuery(query, title, task.Description, task.ID) {
			continue
		}
		createdAt, err := taskCreatedAt(task)
		if err != nil {
			continue
		}
		out = append(out, domain.GraphNodeSummary{
			ID:        strings.TrimSpace(task.ID),
			Type:      domain.NodeTypeTask,
			Title:     title,
			Status:    strings.TrimSpace(task.Status),
			ScopeID:   strings.TrimSpace(task.ProjectID),
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
	decisions DecisionHydrationReader
}

func (h decisionHydrator) NodeType() string { return domain.NodeTypeDecision }

func (h decisionHydrator) Fetch(ctx context.Context, projectID string, nodeID string) (domain.GraphNodeCore, error) {
	decision, err := h.decisions.GetDecision(ctx, strings.TrimSpace(nodeID))
	if err != nil {
		return domain.GraphNodeCore{}, err
	}
	if !strings.EqualFold(strings.TrimSpace(projectID), strings.TrimSpace(decision.ProjectID)) {
		return domain.GraphNodeCore{}, fmt.Errorf("decision %q not found", strings.TrimSpace(nodeID))
	}
	fields := map[string]any{
		"confidence":     decision.Confidence,
		"source":         strings.TrimSpace(decision.Source),
		"sourceIdentity": strings.TrimSpace(decision.SourceIdentity),
		"correlationRef": strings.TrimSpace(decision.CorrelationRef),
		"informedByRef":  strings.TrimSpace(decision.InformedByRef),
		"kind":           strings.TrimSpace(decision.Kind),
		"taskRef":        strings.TrimSpace(decision.TaskRef),
		"keyResultRef":   strings.TrimSpace(decision.KeyResultRef),
		"missionRef":     strings.TrimSpace(decision.MissionRef),
	}
	if decision.Rationale != "" || decision.Context != "" || decision.AlternativesRejected != "" || len(decision.InvalidationConditions) > 0 {
		fields["rationale"] = strings.TrimSpace(decision.Rationale)
		fields["context"] = strings.TrimSpace(decision.Context)
		fields["alternativesRejected"] = strings.TrimSpace(decision.AlternativesRejected)
		fields["invalidationConditions"] = append([]string(nil), decision.InvalidationConditions...)
	}
	return domain.GraphNodeCore{
		ID:        strings.TrimSpace(decision.ID),
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
	items, err := h.decisions.ListDecisions(ctx, strings.TrimSpace(projectID), max(limit*20, 200))
	if err != nil {
		return nil, err
	}
	out := make([]domain.GraphNodeSummary, 0, len(items))
	for _, item := range items {
		logValues := []string{
			item.Rationale,
			item.Context,
			item.AlternativesRejected,
			strings.Join(item.InvalidationConditions, " "),
			item.Outcome,
		}
		if !matchesQuery(query, append([]string{
			item.Title,
			item.ID,
			item.TaskRef,
			item.KeyResultRef,
			item.MissionRef,
		}, logValues...)...) {
			continue
		}
		out = append(out, domain.GraphNodeSummary{
			ID:        strings.TrimSpace(item.ID),
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
	missions MissionHydrationReader
}

func (h missionHydrator) NodeType() string { return domain.NodeTypeMission }

func (h missionHydrator) Fetch(ctx context.Context, projectID string, nodeID string) (domain.GraphNodeCore, error) {
	mission, err := h.missions.GetMission(ctx, strings.TrimSpace(nodeID))
	if err != nil {
		return domain.GraphNodeCore{}, err
	}
	if !strings.EqualFold(strings.TrimSpace(projectID), strings.TrimSpace(mission.ProjectID)) {
		return domain.GraphNodeCore{}, fmt.Errorf("mission %q not found", strings.TrimSpace(nodeID))
	}
	return domain.GraphNodeCore{
		ID:        strings.TrimSpace(mission.ID),
		Type:      domain.NodeTypeMission,
		Title:     strings.TrimSpace(mission.Title),
		Status:    strings.TrimSpace(mission.Status),
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
	items, err := h.missions.ListMissions(ctx, strings.TrimSpace(projectID), max(limit*5, 100))
	if err != nil {
		return nil, err
	}
	out := make([]domain.GraphNodeSummary, 0, len(items))
	for _, item := range items {
		if !matchesQuery(query, item.Title, item.Description, item.ID) {
			continue
		}
		out = append(out, domain.GraphNodeSummary{
			ID:        strings.TrimSpace(item.ID),
			Type:      domain.NodeTypeMission,
			Title:     strings.TrimSpace(item.Title),
			Status:    strings.TrimSpace(item.Status),
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
	missions MissionHydrationReader
}

func (h keyResultHydrator) NodeType() string { return domain.NodeTypeKeyResult }

func (h keyResultHydrator) Fetch(ctx context.Context, projectID string, nodeID string) (domain.GraphNodeCore, error) {
	kr, err := h.missions.GetKeyResult(ctx, strings.TrimSpace(nodeID))
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
		ID:        strings.TrimSpace(kr.ID),
		Type:      domain.NodeTypeKeyResult,
		Title:     strings.TrimSpace(kr.Title),
		Status:    strings.TrimSpace(kr.Status),
		ScopeID:   strings.TrimSpace(mission.ProjectID),
		CreatedAt: kr.CreatedAt.UTC().Format(time.RFC3339Nano),
		Fields: map[string]any{
			"measurementType": strings.TrimSpace(kr.MeasurementType),
			"direction":       strings.TrimSpace(kr.Direction),
			"unit":            strings.TrimSpace(kr.Unit),
			"targetValue":     kr.TargetValue,
			"currentValue":    kr.CurrentValue,
			"baseline":        floatPtrValue(kr.Baseline),
			"missionId":       strings.TrimSpace(kr.MissionID),
		},
	}, nil
}

func (h keyResultHydrator) FetchMany(ctx context.Context, projectID string, nodeIDs []string) ([]domain.GraphNodeSummary, error) {
	return fetchManyUsingFetch(ctx, h, projectID, nodeIDs)
}

func (h keyResultHydrator) Search(ctx context.Context, projectID, query string, limit int) ([]domain.GraphNodeSummary, error) {
	missions, err := h.missions.ListMissions(ctx, strings.TrimSpace(projectID), max(limit*5, 100))
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
			if !matchesQuery(query, kr.Title, kr.Description, kr.ID, kr.MissionID) {
				continue
			}
			out = append(out, domain.GraphNodeSummary{
				ID:        strings.TrimSpace(kr.ID),
				Type:      domain.NodeTypeKeyResult,
				Title:     strings.TrimSpace(kr.Title),
				Status:    strings.TrimSpace(kr.Status),
				ScopeID:   strings.TrimSpace(mission.ProjectID),
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

func taskVisibleToProject(task TaskHydrationRow, projectID string) bool {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(task.ProjectID), projectID)
}

func taskCreatedAt(task TaskHydrationRow) (time.Time, error) {
	if task.CreatedAt != nil && !task.CreatedAt.IsZero() {
		return task.CreatedAt.UTC(), nil
	}
	return time.Time{}, fmt.Errorf("task %q missing createdAt", strings.TrimSpace(task.ID))
}

// taskMissionRef reads the task's mission ref out of metadata. Tasks store the
// mission link in metadata (under any of these aliases) rather than a dedicated
// field, so the structural resolver and graph_query consumers read it here.
func taskMissionRef(task TaskHydrationRow) string {
	if task.Metadata == nil {
		return ""
	}
	for _, key := range []string{"mission_ref", "missionRef", "mission_id", "missionId"} {
		if raw, ok := task.Metadata[key]; ok {
			if value, ok := raw.(string); ok {
				return strings.TrimSpace(value)
			}
		}
	}
	return ""
}

func taskDueDate(task TaskHydrationRow) string {
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

func decisionStatus(decision DecisionHydrationRow) string {
	if kind := strings.TrimSpace(decision.Kind); kind != "" {
		return kind
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

func isNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "not found")
}
