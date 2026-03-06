package task

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/tinoosan/agen8/pkg/types"
)

const routingVersion = 1

var ErrRoutingCallbackMissingTeamID = errors.New("routing.violation: callback task missing destinationTeamId")

// RoutingOracle validates and canonicalizes task routing before persistence.
type RoutingOracle struct{}

func NewRoutingOracle() *RoutingOracle {
	return &RoutingOracle{}
}

func (o *RoutingOracle) NormalizeCreate(ctx context.Context, loader RunLoader, task types.Task) (types.Task, error) {
	return o.normalize(ctx, loader, task)
}

func (o *RoutingOracle) NormalizeUpdate(ctx context.Context, loader RunLoader, task types.Task) (types.Task, error) {
	return o.normalize(ctx, loader, task)
}

func (o *RoutingOracle) ValidateCompletion(ctx context.Context, loader RunLoader, task types.Task) error {
	_, err := o.normalize(ctx, loader, task)
	return err
}

func (o *RoutingOracle) RepairTask(ctx context.Context, loader RunLoader, task types.Task) (types.Task, bool, error) {
	before := task
	norm, err := o.normalize(ctx, loader, task)
	if err != nil {
		return task, false, err
	}
	changed := !routingEquivalent(before, norm)
	return norm, changed, nil
}

func (o *RoutingOracle) normalize(ctx context.Context, loader RunLoader, task types.Task) (types.Task, error) {
	task.TaskID = strings.TrimSpace(task.TaskID)
	task.RunID = strings.TrimSpace(task.RunID)
	task.NormalizeTeamFields()
	task.AssignedToType = strings.TrimSpace(task.AssignedToType)
	task.AssignedTo = strings.TrimSpace(task.AssignedTo)
	task.AssignedRole = strings.TrimSpace(task.AssignedRole)
	if task.Metadata == nil {
		task.Metadata = map[string]any{}
	}

	source := strings.TrimSpace(fmt.Sprint(task.Metadata["source"]))
	if source == "" {
		source = "task_create"
	}
	task.Metadata["source"] = source
	callback := isCallbackSource(source)

	// Team resolution for callbacks: infer destination team from run when possible.
	if callback && task.DestinationTeamID == "" && loader != nil && task.RunID != "" {
		run, err := loader.LoadRun(ctx, task.RunID)
		if err == nil && run.Runtime != nil {
			task.DestinationTeamID = strings.TrimSpace(run.Runtime.TeamID)
		}
	}
	if callback && task.DestinationTeamID == "" {
		return task, fmt.Errorf("routing.violation: callback task %s missing destinationTeamId: %w", task.TaskID, ErrRoutingCallbackMissingTeamID)
	}
	if task.SourceTeamID == "" && task.DestinationTeamID != "" {
		task.SourceTeamID = task.DestinationTeamID
	}

	if task.DestinationTeamID != "" {
		if task.AssignedToType == "" {
			if task.AssignedRole != "" {
				task.AssignedToType = "role"
				task.AssignedTo = task.AssignedRole
			} else if task.AssignedTo != "" {
				task.AssignedToType = "agent"
			} else {
				return task, fmt.Errorf("routing.violation: team task %s missing assignee route", task.TaskID)
			}
		}
		switch task.AssignedToType {
		case "agent":
			if task.AssignedTo == "" {
				return task, fmt.Errorf("routing.violation: team task %s missing assignedTo for agent route", task.TaskID)
			}
		case "role":
			if task.AssignedRole == "" && task.AssignedTo != "" {
				task.AssignedRole = task.AssignedTo
			}
			if task.AssignedTo == "" {
				task.AssignedTo = task.AssignedRole
			}
			if task.AssignedTo == "" {
				return task, fmt.Errorf("routing.violation: team task %s missing assigned role", task.TaskID)
			}
		case "team":
			if task.AssignedTo == "" {
				task.AssignedTo = task.DestinationTeamID
			}
		default:
			return task, fmt.Errorf("routing.violation: task %s has invalid assignedToType %q", task.TaskID, task.AssignedToType)
		}
	} else {
		if callback {
			return task, fmt.Errorf("routing.violation: callback task %s missing destinationTeamId: %w", task.TaskID, ErrRoutingCallbackMissingTeamID)
		}
		if task.AssignedToType == "" {
			task.AssignedToType = "agent"
		}
		if task.AssignedTo == "" && task.AssignedToType == "agent" {
			task.AssignedTo = task.RunID
		}
	}
	task.TeamID = task.DestinationTeamID

	task.Metadata["routingVersion"] = float64(routingVersion)
	task.Metadata["routingDecisionId"] = "route-" + uuid.NewString()
	task.Metadata["routingReasonCode"] = routingReasonCode(callback, task.DestinationTeamID, task.AssignedToType)
	task.Metadata["routingRecipients"] = []string{task.AssignedToType + ":" + task.AssignedTo}
	task.Metadata["routingScopes"] = routingScopes(task)
	return task, nil
}

func routingReasonCode(callback bool, teamID, assigneeType string) string {
	if callback {
		return "callback.owner_review"
	}
	if teamID != "" {
		return "team.task." + assigneeType
	}
	return "run.task.agent"
}

func routingScopes(task types.Task) []string {
	out := []string{}
	if task.SourceTeamID != "" {
		out = append(out, "source_team")
	}
	if task.DestinationTeamID != "" {
		out = append(out, "team")
	}
	if task.RunID != "" {
		out = append(out, "run")
	}
	switch task.AssignedToType {
	case "role":
		out = append(out, "role")
	case "agent":
		out = append(out, "owner")
	}
	return out
}

func routingEquivalent(a, b types.Task) bool {
	a.NormalizeTeamFields()
	b.NormalizeTeamFields()
	return strings.TrimSpace(a.SourceTeamID) == strings.TrimSpace(b.SourceTeamID) &&
		strings.TrimSpace(a.DestinationTeamID) == strings.TrimSpace(b.DestinationTeamID) &&
		strings.TrimSpace(a.AssignedToType) == strings.TrimSpace(b.AssignedToType) &&
		strings.TrimSpace(a.AssignedTo) == strings.TrimSpace(b.AssignedTo) &&
		strings.TrimSpace(a.AssignedRole) == strings.TrimSpace(b.AssignedRole)
}

func isCallbackSource(source string) bool {
	switch strings.TrimSpace(source) {
	case "subagent.callback", "team.callback", "subagent.batch.callback", "team.batch.callback":
		return true
	default:
		return false
	}
}
