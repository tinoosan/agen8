package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/tinoosan/agen8/internal/core/types"
	decisionapp "github.com/tinoosan/agen8/internal/services/decision/app"
	decisiondomain "github.com/tinoosan/agen8/internal/services/decision/domain"
	graphapp "github.com/tinoosan/agen8/internal/services/graph/app"
	missionapp "github.com/tinoosan/agen8/internal/services/mission/app"
	krdomain "github.com/tinoosan/agen8/internal/services/mission/domain/kr"
	missiondomain "github.com/tinoosan/agen8/internal/services/mission/domain/mission"
	taskapp "github.com/tinoosan/agen8/internal/services/task/app"
	taskdomain "github.com/tinoosan/agen8/internal/services/task/domain"
)

type graphTaskHydrationReader struct {
	tasks *taskapp.Service
}

func (r graphTaskHydrationReader) GetTask(ctx context.Context, taskID string) (graphapp.TaskHydrationRow, error) {
	if r.tasks == nil {
		return graphapp.TaskHydrationRow{}, fmt.Errorf("task service is required")
	}
	task, err := r.tasks.Get(ctx, taskdomain.TaskID(strings.TrimSpace(taskID)))
	if err != nil {
		return graphapp.TaskHydrationRow{}, err
	}
	return graphTaskHydrationRow(task), nil
}

func (r graphTaskHydrationReader) ListTasks(ctx context.Context, projectID string, limit int) ([]graphapp.TaskHydrationRow, error) {
	if r.tasks == nil {
		return nil, fmt.Errorf("task service is required")
	}
	tasks, err := r.tasks.List(ctx, taskdomain.TaskFilter{
		ProjectID: types.ProjectID(strings.TrimSpace(projectID)),
		Limit:     limit,
	})
	if err != nil {
		return nil, err
	}
	out := make([]graphapp.TaskHydrationRow, 0, len(tasks))
	for _, task := range tasks {
		out = append(out, graphTaskHydrationRow(task))
	}
	return out, nil
}

func graphTaskHydrationRow(task taskdomain.Task) graphapp.TaskHydrationRow {
	return graphapp.TaskHydrationRow{
		ID:                   strings.TrimSpace(string(task.ID)),
		ProjectID:            strings.TrimSpace(string(task.ProjectID)),
		Description:          task.Description,
		Title:                task.Title,
		Status:               string(task.Status),
		AssignedTo:           strings.TrimSpace(string(task.AssignedTo)),
		AssignedToLabel:      task.AssignedToLabel,
		ClaimedByMemberID:    strings.TrimSpace(string(task.ClaimedByMemberID)),
		ClaimedByMemberLabel: task.ClaimedByMemberLabel,
		CreatedBy:            task.CreatedBy,
		CreatedByLabel:       task.CreatedByLabel,
		TaskKind:             task.TaskKind,
		KeyResultRef:         task.KeyResultRef,
		Metadata:             task.Metadata,
		CreatedAt:            task.CreatedAt,
	}
}

type graphDecisionHydrationReader struct {
	decisions *decisionapp.Service
}

func (r graphDecisionHydrationReader) GetDecision(ctx context.Context, decisionID string) (graphapp.DecisionHydrationRow, error) {
	if r.decisions == nil {
		return graphapp.DecisionHydrationRow{}, fmt.Errorf("decision service is required")
	}
	decision, err := r.decisions.Get(ctx, decisiondomain.DecisionID(strings.TrimSpace(decisionID)))
	if err != nil {
		return graphapp.DecisionHydrationRow{}, err
	}
	return graphDecisionHydrationRow(decision), nil
}

func (r graphDecisionHydrationReader) ListDecisions(ctx context.Context, projectID string, limit int) ([]graphapp.DecisionHydrationRow, error) {
	if r.decisions == nil {
		return nil, fmt.Errorf("decision service is required")
	}
	decisions, err := r.decisions.List(ctx, decisiondomain.DecisionFilter{
		ProjectID: strings.TrimSpace(projectID),
		SortDesc:  true,
		Limit:     limit,
	})
	if err != nil {
		return nil, err
	}
	out := make([]graphapp.DecisionHydrationRow, 0, len(decisions))
	for _, decision := range decisions {
		out = append(out, graphDecisionHydrationRow(decision))
	}
	return out, nil
}

func graphDecisionHydrationRow(decision decisiondomain.Decision) graphapp.DecisionHydrationRow {
	row := graphapp.DecisionHydrationRow{
		ID:             strings.TrimSpace(string(decision.ID)),
		ProjectID:      strings.TrimSpace(decision.ProjectID),
		Source:         string(decision.Source),
		Title:          decision.Title,
		Confidence:     decision.Confidence,
		CreatedAt:      decision.CreatedAt,
		SourceIdentity: decision.SourceIdentity,
		CorrelationRef: decision.CorrelationRef,
		InformedByRef:  decision.InformedByRef,
		Kind:           string(decision.Kind()),
		TaskRef:        decision.TaskRef,
		KeyResultRef:   decision.KeyResultRef,
		MissionRef:     decision.MissionRef,
	}
	if decision.Log != nil {
		row.Rationale = decision.Log.Rationale
		row.Context = decision.Log.Context
		row.AlternativesRejected = decision.Log.AlternativesRejected
		row.InvalidationConditions = append([]string(nil), decision.Log.InvalidationConditions...)
		row.Outcome = decision.Log.Outcome
	}
	return row
}

type graphMissionHydrationReader struct {
	missions *missionapp.Service
}

func (r graphMissionHydrationReader) GetMission(ctx context.Context, missionID string) (graphapp.MissionHydrationRow, error) {
	if r.missions == nil {
		return graphapp.MissionHydrationRow{}, fmt.Errorf("mission service is required")
	}
	mission, err := r.missions.GetMission(ctx, missiondomain.MissionID(strings.TrimSpace(missionID)))
	if err != nil {
		return graphapp.MissionHydrationRow{}, err
	}
	return graphMissionHydrationRow(mission), nil
}

func (r graphMissionHydrationReader) ListMissions(ctx context.Context, projectID string, limit int) ([]graphapp.MissionHydrationRow, error) {
	if r.missions == nil {
		return nil, fmt.Errorf("mission service is required")
	}
	missions, err := r.missions.ListMissions(ctx, strings.TrimSpace(projectID), missiondomain.MissionFilter{Limit: limit})
	if err != nil {
		return nil, err
	}
	out := make([]graphapp.MissionHydrationRow, 0, len(missions))
	for _, mission := range missions {
		out = append(out, graphMissionHydrationRow(mission))
	}
	return out, nil
}

func (r graphMissionHydrationReader) GetKeyResult(ctx context.Context, keyResultID string) (graphapp.KeyResultHydrationRow, error) {
	if r.missions == nil {
		return graphapp.KeyResultHydrationRow{}, fmt.Errorf("mission service is required")
	}
	kr, err := r.missions.GetKeyResult(ctx, krdomain.KeyResultID(strings.TrimSpace(keyResultID)))
	if err != nil {
		return graphapp.KeyResultHydrationRow{}, err
	}
	return graphKeyResultHydrationRow(kr), nil
}

func (r graphMissionHydrationReader) ListKeyResults(ctx context.Context, missionID string) ([]graphapp.KeyResultHydrationRow, error) {
	if r.missions == nil {
		return nil, fmt.Errorf("mission service is required")
	}
	krs, err := r.missions.ListKeyResults(ctx, missiondomain.MissionID(strings.TrimSpace(missionID)))
	if err != nil {
		return nil, err
	}
	out := make([]graphapp.KeyResultHydrationRow, 0, len(krs))
	for _, kr := range krs {
		out = append(out, graphKeyResultHydrationRow(kr))
	}
	return out, nil
}

func graphMissionHydrationRow(mission missiondomain.Mission) graphapp.MissionHydrationRow {
	return graphapp.MissionHydrationRow{
		ID:          strings.TrimSpace(string(mission.ID)),
		ProjectID:   strings.TrimSpace(mission.ProjectID),
		Title:       mission.Title,
		Description: mission.Description,
		Status:      string(mission.Status),
		CreatedAt:   mission.CreatedAt,
		StartDate:   mission.StartDate,
		EndDate:     mission.EndDate,
	}
}

func graphKeyResultHydrationRow(kr krdomain.KeyResult) graphapp.KeyResultHydrationRow {
	return graphapp.KeyResultHydrationRow{
		ID:              strings.TrimSpace(string(kr.ID)),
		MissionID:       strings.TrimSpace(string(kr.MissionID)),
		Title:           kr.Title,
		Description:     kr.Description,
		Status:          string(kr.Status),
		CreatedAt:       kr.CreatedAt,
		MeasurementType: string(kr.MeasurementType),
		Direction:       string(kr.Direction),
		Unit:            kr.Unit,
		TargetValue:     kr.TargetValue,
		CurrentValue:    kr.CurrentValue,
		Baseline:        kr.Baseline,
	}
}
