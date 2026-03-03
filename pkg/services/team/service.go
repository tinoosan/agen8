package team

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tinoosan/agen8/pkg/agent/hosttools"
	"github.com/tinoosan/agen8/pkg/agent/state"
	"github.com/tinoosan/agen8/pkg/profile"
	"github.com/tinoosan/agen8/pkg/services/session"
	"github.com/tinoosan/agen8/pkg/services/task"
	"github.com/tinoosan/agen8/pkg/types"
)

// Sentinel errors for validation and RPC mapping.
var (
	ErrMissingCoordinator = errors.New("team profile must define one coordinator role")
)

// RunStopper cancels a run's loop. Used by EscalateToCoordinator so the team service
// can trigger "cancel this run" without depending on app's cancel map.
type RunStopper interface {
	StopRun(runID string) error
}

// EscalateToCoordinator creates an escalation task and stops the child run (single policy).
// Logic extracted from team_daemon and daemon_runtime_supervisor so it lives in one place.
func EscalateToCoordinator(
	ctx context.Context,
	taskCreator task.RetryEscalationCreator,
	sessionService session.Service,
	runStopper RunStopper,
	callbackTaskID string,
	data hosttools.EscalationData,
) error {
	if err := taskCreator.CreateEscalationTask(ctx, callbackTaskID, data); err != nil {
		return err
	}
	childRunID := strings.TrimSpace(data.SourceRunID)
	if childRunID != "" {
		var stopErr error
		if runStopper != nil {
			if err := runStopper.StopRun(childRunID); err != nil {
				stopErr = errors.Join(stopErr, fmt.Errorf("stop child run loop %s: %w", childRunID, err))
			}
		}
		if sessionService != nil {
			if _, err := sessionService.StopRun(ctx, childRunID, types.RunStatusFailed, "escalated"); err != nil {
				stopErr = errors.Join(stopErr, fmt.Errorf("persist child run stop %s: %w", childRunID, err))
			}
		}
		if stopErr != nil {
			return stopErr
		}
	}
	return nil
}

// SeedCoordinatorTask creates the single initial pending task for the coordinator (team goal).
func SeedCoordinatorTask(
	ctx context.Context,
	taskStore state.TaskStore,
	sessionID, runID, teamID, coordinatorRole, goal string,
) error {
	sessionID = strings.TrimSpace(sessionID)
	runID = strings.TrimSpace(runID)
	teamID = strings.TrimSpace(teamID)
	coordinatorRole = strings.TrimSpace(coordinatorRole)
	goal = strings.TrimSpace(goal)
	if sessionID == "" || runID == "" || teamID == "" || coordinatorRole == "" {
		return fmt.Errorf("sessionID, runID, teamID, and coordinatorRole are required")
	}
	now := time.Now().UTC()
	t := types.Task{
		TaskID:       "task-" + uuid.NewString(),
		SessionID:    sessionID,
		RunID:        runID,
		TeamID:       teamID,
		AssignedRole: coordinatorRole,
		CreatedBy:    "user",
		Goal:         goal,
		Priority:     0,
		Status:       types.TaskStatusPending,
		CreatedAt:    &now,
		Inputs:       map[string]any{},
		Metadata:     map[string]any{"source": "team.goal"},
	}
	return taskStore.CreateTask(ctx, t)
}

// ValidateTeamRoles validates role configs: unique non-empty names, exactly one coordinator.
func ValidateTeamRoles(roles []profile.RoleConfig) (roleNames []string, coordinatorRole string, err error) {
	out := make([]string, 0, len(roles))
	seen := map[string]struct{}{}
	coordinatorRole = ""
	for _, role := range roles {
		name := strings.TrimSpace(role.Name)
		if name == "" {
			return nil, "", fmt.Errorf("team role name is required")
		}
		if _, ok := seen[name]; ok {
			return nil, "", fmt.Errorf("duplicate team role name %q", name)
		}
		seen[name] = struct{}{}
		out = append(out, name)
		if role.Coordinator {
			coordinatorRole = name
		}
	}
	if coordinatorRole == "" {
		return nil, "", ErrMissingCoordinator
	}
	return out, coordinatorRole, nil
}

func ResolveReviewerRole(prof *profile.Profile, coordinatorRole string) string {
	coordinatorRole = strings.TrimSpace(coordinatorRole)
	if prof == nil {
		return coordinatorRole
	}
	if reviewerCfg, ok := prof.ReviewerForSession(); ok && reviewerCfg != nil {
		if reviewer := strings.TrimSpace(reviewerCfg.EffectiveName()); reviewer != "" {
			return reviewer
		}
	}
	return coordinatorRole
}

// BuildManifest builds a Manifest from role records and metadata.
func BuildManifest(teamID, profileID, coordinatorRole, coordinatorRunID, teamModel string, roles []RoleRecord, createdAt string) Manifest {
	return Manifest{
		TeamID:          strings.TrimSpace(teamID),
		ProfileID:       strings.TrimSpace(profileID),
		TeamModel:       strings.TrimSpace(teamModel),
		CoordinatorRole: strings.TrimSpace(coordinatorRole),
		CoordinatorRun:  strings.TrimSpace(coordinatorRunID),
		Roles:           roles,
		CreatedAt:       strings.TrimSpace(createdAt),
	}
}

// IsTeamIdle returns true when the team has no active/pending tasks (excluding heartbeats).
func IsTeamIdle(ctx context.Context, taskStore state.TaskStore, teamID string) bool {
	teamID = strings.TrimSpace(teamID)
	if teamID == "" {
		return false
	}
	active, err := taskStore.CountTasks(ctx, state.TaskFilter{
		TeamID: teamID,
		Status: []types.TaskStatus{types.TaskStatusPending, types.TaskStatusActive},
	})
	if err != nil {
		return false
	}
	heartbeat, err := taskStore.CountTasks(ctx, state.TaskFilter{
		TeamID:   teamID,
		TaskKind: state.TaskKindHeartbeat,
		Status:   []types.TaskStatus{types.TaskStatusPending, types.TaskStatusActive},
	})
	if err != nil {
		return false
	}
	return active-heartbeat <= 0
}

// ModelApplier applies a model change to runtimes. App implements this by iterating runtimes and calling SetModel.
type ModelApplier interface {
	ApplyModel(ctx context.Context, model, target string) (appliedRunIDs []string, err error)
}

// RequestModelChange applies or queues a team model change (same policy as requestTeamModelChange in app).
func RequestModelChange(
	ctx context.Context,
	taskStore state.TaskStore,
	stateMgr *StateManager,
	applier ModelApplier,
	model, target, reason string,
) ([]string, error) {
	model = strings.TrimSpace(model)
	if model == "" {
		return nil, fmt.Errorf("model is required")
	}
	target = strings.TrimSpace(target)
	if target != "" {
		appliedTo, err := applier.ApplyModel(ctx, model, target)
		if err != nil {
			_ = stateMgr.MarkModelFailed(ctx, model, err)
			return nil, err
		}
		return appliedTo, stateMgr.MarkModelApplied(ctx, model)
	}
	if IsTeamIdle(ctx, taskStore, stateMgr.teamID) {
		appliedTo, err := applier.ApplyModel(ctx, model, "")
		if err != nil {
			_ = stateMgr.MarkModelFailed(ctx, model, err)
			return nil, err
		}
		return appliedTo, stateMgr.MarkModelApplied(ctx, model)
	}
	if err := stateMgr.QueueModelChange(ctx, model, reason); err != nil {
		return nil, err
	}
	return []string{}, nil
}
