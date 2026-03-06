package webhook

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tinoosan/agen8/pkg/agent/state"
	"github.com/tinoosan/agen8/pkg/types"
)

// StandalonePayload is the JSON structure for POST /task in standalone mode.
type StandalonePayload struct {
	TaskID   string         `json:"taskId"`
	Goal     string         `json:"goal"`
	Priority int            `json:"priority,omitempty"`
	Inputs   map[string]any `json:"inputs,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

// TeamPayload is the JSON structure for POST /task in team mode.
type TeamPayload struct {
	TaskID       string         `json:"taskId"`
	AssignedRole string         `json:"assignedRole,omitempty"`
	Goal         string         `json:"goal"`
	Priority     int            `json:"priority,omitempty"`
	Inputs       map[string]any `json:"inputs,omitempty"`
	Metadata     map[string]any `json:"metadata,omitempty"`
}

// BuildStandaloneTask parses the payload and builds a types.Task for standalone mode.
func BuildStandaloneTask(payload []byte, run types.Run) (types.Task, error) {
	var p StandalonePayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return types.Task{}, err
	}
	goal := strings.TrimSpace(p.Goal)
	if goal == "" {
		return types.Task{}, errGoalRequired
	}
	origTaskID := strings.TrimSpace(p.TaskID)
	taskID := origTaskID
	if taskID == "" {
		taskID = "task-" + uuid.NewString()
	} else if norm, changed := types.NormalizeTaskID(taskID); changed {
		taskID = norm
		if p.Metadata == nil {
			p.Metadata = map[string]any{}
		}
		p.Metadata["originalTaskId"] = origTaskID
	} else {
		taskID = norm
	}
	now := time.Now()
	task := types.Task{
		TaskID:    taskID,
		SessionID: run.SessionID,
		RunID:     run.RunID,
		Goal:      goal,
		Priority:  p.Priority,
		Status:    types.TaskStatusPending,
		CreatedAt: &now,
		Inputs:    p.Inputs,
		Metadata:  p.Metadata,
	}
	if strings.HasPrefix(taskID, "task-") {
		task.TaskKind = state.TaskKindTask
	}
	return task, nil
}

// BuildTeamTask parses the payload and builds a types.Task for team mode.
func BuildTeamTask(payload []byte, teamID, coordinatorRole string, run types.Run, validRoles map[string]struct{}) (types.Task, error) {
	var p TeamPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return types.Task{}, err
	}
	goal := strings.TrimSpace(p.Goal)
	if goal == "" {
		return types.Task{}, errGoalRequired
	}
	taskID := strings.TrimSpace(p.TaskID)
	if taskID == "" {
		taskID = "task-" + uuid.NewString()
	} else {
		if norm, _ := types.NormalizeTaskID(taskID); norm != "" {
			taskID = norm
		}
	}
	assignedRole := strings.TrimSpace(p.AssignedRole)
	defaultedToCoordinator := false
	if assignedRole == "" {
		assignedRole = coordinatorRole
		defaultedToCoordinator = true
	}
	if len(validRoles) != 0 {
		if _, ok := validRoles[assignedRole]; !ok {
			return types.Task{}, errInvalidRole
		}
	}
	now := time.Now().UTC()
	task := types.Task{
		TaskID:       taskID,
		SessionID:    run.SessionID,
		RunID:        run.RunID,
		TeamID:       teamID,
		AssignedRole: assignedRole,
		CreatedBy:    "webhook",
		Goal:         goal,
		Priority:     p.Priority,
		Status:       types.TaskStatusPending,
		CreatedAt:    &now,
		Inputs:       p.Inputs,
		Metadata:     p.Metadata,
	}
	if task.Inputs == nil {
		task.Inputs = map[string]any{}
	}
	if task.Metadata == nil {
		task.Metadata = map[string]any{}
	}
	task.Metadata["source"] = "webhook"
	if defaultedToCoordinator {
		task.Metadata["routingDefault"] = "coordinator_role"
	}
	return task, nil
}
