package app

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tinoosan/agen8/internal/storage"
	"github.com/tinoosan/agen8/pkg/protocol"
	"github.com/tinoosan/agen8/pkg/services/team"
	"github.com/tinoosan/agen8/pkg/types"
)

const desiredStateManagedBy = "desired-state"

type projectTeamRuntimeState struct {
	running bool
	paused  bool
	stale   bool
}

type projectReconcileNotification struct {
	ProjectRoot string                            `json:"projectRoot,omitempty"`
	ProjectID   string                            `json:"projectId,omitempty"`
	Converged   bool                              `json:"converged"`
	Status      string                            `json:"status,omitempty"`
	Actions     []protocol.ProjectReconcileAction `json:"actions,omitempty"`
	TeamIDs     []string                          `json:"teamIds,omitempty"`
	TickAt      string                            `json:"tickAt,omitempty"`
	Error       string                            `json:"error,omitempty"`
}

func (s *runtimeSupervisor) DiffProject(ctx context.Context, projectRoot string) (protocol.ProjectDiffResult, error) {
	return s.projectDiff(ctx, projectRoot, false, false)
}

func (s *runtimeSupervisor) ApplyProject(ctx context.Context, projectRoot string) (protocol.ProjectDiffResult, error) {
	return s.projectDiff(ctx, projectRoot, true, true)
}

func (s *runtimeSupervisor) projectDiff(ctx context.Context, projectRoot string, apply bool, notify bool) (protocol.ProjectDiffResult, error) {
	if s == nil {
		return protocol.ProjectDiffResult{}, fmt.Errorf("runtime supervisor is nil")
	}
	if s.projectTeamSvc == nil {
		return protocol.ProjectDiffResult{}, fmt.Errorf("project team service is not configured")
	}
	projectRoot = strings.TrimSpace(projectRoot)
	if projectRoot == "" {
		return protocol.ProjectDiffResult{}, fmt.Errorf("project root is required")
	}
	projectCtx, err := LoadProjectContext(projectRoot)
	if err != nil {
		return protocol.ProjectDiffResult{}, err
	}
	if !projectCtx.Exists {
		return protocol.ProjectDiffResult{}, fmt.Errorf("project is not initialized; run `agen8 project init` first")
	}
	state, err := readProjectDesiredState(projectRoot)
	if err != nil {
		return protocol.ProjectDiffResult{}, err
	}
	if err := validateProjectDesiredState(state, projectRoot); err != nil {
		return protocol.ProjectDiffResult{}, err
	}
	if expected := strings.TrimSpace(projectCtx.Config.ProjectID); expected != "" && strings.TrimSpace(state.ProjectID) != expected {
		return protocol.ProjectDiffResult{}, fmt.Errorf("%s projectId=%q does not match initialized projectId=%q", projectDesiredStatePath(projectRoot), state.ProjectID, expected)
	}
	if apply && s.projectRegistrySvc != nil {
		if _, err := s.projectRegistrySvc.RegisterProject(ctx, ProjectRegistrySummary{
			ProjectRoot:  projectRoot,
			ProjectID:    strings.TrimSpace(state.ProjectID),
			ManifestPath: projectDesiredStatePath(projectRoot),
			Enabled:      true,
			Metadata: map[string]any{
				"source": "project.apply",
			},
		}); err != nil {
			return protocol.ProjectDiffResult{}, err
		}
	}
	diff, err := s.computeProjectDiff(ctx, projectRoot, state)
	if err != nil {
		if apply {
			_ = s.notifyProjectReconcile(protocol.NotifyProjectReconcileFailed, projectRoot, strings.TrimSpace(state.ProjectID), "failed", nil, nil, err.Error())
		}
		return protocol.ProjectDiffResult{}, err
	}
	if apply && notify {
		_ = s.notifyProjectReconcile(protocol.NotifyProjectReconcileStarted, projectRoot, strings.TrimSpace(state.ProjectID), "reconciling", nil, projectTeamIDsForDiff(diff), "")
	}
	if !apply {
		return diff, nil
	}
	for _, action := range diff.Actions {
		switch strings.TrimSpace(action.Action) {
		case "spawn", "recreate":
			if _, err := s.startDesiredProjectTeam(ctx, projectRoot, diff.ProjectID, strings.TrimSpace(action.Profile), strings.TrimSpace(action.TeamID)); err != nil {
				return protocol.ProjectDiffResult{}, err
			}
		case "delete":
			teamID := strings.TrimSpace(action.TeamID)
			if teamID == "" {
				continue
			}
			deleteSvc := NewTeamDeleteService(s.cfg, s.sessionService, team.NewFileManifestStore(s.cfg), s.projectTeamSvc)
			if _, err := deleteSvc.DeleteTeam(ctx, TeamDeleteInput{
				TeamID:      teamID,
				ProjectRoot: projectRoot,
			}); err != nil {
				return protocol.ProjectDiffResult{}, err
			}
		case "stop":
			teamID := strings.TrimSpace(action.TeamID)
			if teamID == "" || s.projectTeamSvc == nil {
				continue
			}
			summary, err := s.projectTeamSvc.GetTeam(ctx, projectRoot, teamID)
			if err != nil {
				return protocol.ProjectDiffResult{}, err
			}
			if strings.TrimSpace(summary.PrimarySessionID) != "" {
				if _, err := s.StopSession(ctx, strings.TrimSpace(summary.PrimarySessionID)); err != nil {
					return protocol.ProjectDiffResult{}, err
				}
			}
			if _, err := s.projectTeamSvc.SetStatus(ctx, projectRoot, teamID, "inactive", map[string]any{
				"reconcileStatus": "drifting",
				"desiredEnabled":  false,
			}); err != nil {
				return protocol.ProjectDiffResult{}, err
			}
		}
	}
	applied := diff
	if len(applied.Actions) == 0 {
		applied.Converged = true
		applied.Status = "converged"
		if notify {
			_ = s.notifyProjectReconcile(protocol.NotifyProjectReconcileConverged, projectRoot, strings.TrimSpace(state.ProjectID), applied.Status, applied.Actions, projectTeamIDsForDiff(applied), "")
		}
	} else {
		applied.Converged = false
		applied.Status = "reconciling"
		_ = s.notifyProjectReconcile(protocol.NotifyProjectReconcileDrift, projectRoot, strings.TrimSpace(state.ProjectID), applied.Status, applied.Actions, projectTeamIDsForDiff(applied), "")
	}
	return applied, nil
}

func (s *runtimeSupervisor) reconcileRegisteredProjects(ctx context.Context, startup bool) error {
	if s == nil || s.projectRegistrySvc == nil {
		return nil
	}
	projects, err := s.projectRegistrySvc.ListProjects(ctx)
	if err != nil {
		return err
	}
	for _, project := range projects {
		if !project.Enabled {
			continue
		}
		if _, err := s.projectDiff(ctx, strings.TrimSpace(project.ProjectRoot), true, false); err != nil {
			slog.Error("project desired-state reconcile failed", "component", "supervisor", "project_root", project.ProjectRoot, "startup", startup, "error", err)
		}
	}
	return nil
}

func (s *runtimeSupervisor) computeProjectDiff(ctx context.Context, projectRoot string, state ProjectDesiredState) (protocol.ProjectDiffResult, error) {
	out := protocol.ProjectDiffResult{
		ProjectRoot:  projectRoot,
		ProjectID:    strings.TrimSpace(state.ProjectID),
		DesiredTeams: make([]protocol.ProjectDesiredTeam, 0, len(state.Teams)),
		Status:       "converged",
		Converged:    true,
	}
	actual, err := s.projectTeamSvc.ListByProject(ctx, projectRoot)
	if err != nil {
		return protocol.ProjectDiffResult{}, err
	}
	actualByProfile := make(map[string][]ProjectTeamSummary)
	out.ActualTeams = make([]protocol.ProjectTeamSummary, 0, len(actual))
	for _, team := range actual {
		key := strings.ToLower(strings.TrimSpace(team.ProfileID))
		actualByProfile[key] = append(actualByProfile[key], team)
		out.ActualTeams = append(out.ActualTeams, protocol.ProjectTeamSummary{
			ProjectID:        team.ProjectID,
			ProjectRoot:      team.ProjectRoot,
			TeamID:           team.TeamID,
			ProfileID:        team.ProfileID,
			PrimarySessionID: team.PrimarySessionID,
			CoordinatorRunID: team.CoordinatorRunID,
			Status:           team.Status,
			CreatedAt:        team.CreatedAt,
			UpdatedAt:        team.UpdatedAt,
			ManifestPresent:  team.ManifestPresent,
			DesiredEnabled:   team.DesiredEnabled,
			ReconcileStatus:  team.ReconcileStatus,
			ManagedBy:        team.ManagedBy,
		})
	}
	desiredProfiles := map[string]ProjectDesiredStateTeam{}
	for _, desired := range state.Teams {
		desiredProfiles[strings.ToLower(strings.TrimSpace(desired.Profile))] = desired
		out.DesiredTeams = append(out.DesiredTeams, protocol.ProjectDesiredTeam{
			Profile:          strings.TrimSpace(desired.Profile),
			Enabled:          desired.Enabled,
			OverrideInterval: heartbeatOverride(desired),
		})
		if !desired.Enabled {
			continue
		}
		match := s.pickProjectTeamMatch(actualByProfile[strings.ToLower(strings.TrimSpace(desired.Profile))], desired)
		if match == nil {
			out.Actions = append(out.Actions, protocol.ProjectReconcileAction{
				Action:  "spawn",
				Profile: strings.TrimSpace(desired.Profile),
				Reason:  "desired team is missing",
			})
			out.Converged = false
			out.Status = "drifting"
			continue
		}
		runtimeState, err := s.projectTeamRuntimeState(ctx, *match)
		if err != nil {
			return protocol.ProjectDiffResult{}, err
		}
		if runtimeState.paused || runtimeState.stale || !runtimeState.running {
			action := "recreate"
			reason := "desired team is not running"
			if runtimeState.paused {
				reason = "desired team is paused"
			} else if runtimeState.stale {
				reason = "desired team has stale runtime state"
			}
			out.Actions = append(out.Actions, protocol.ProjectReconcileAction{
				Action:  action,
				Profile: strings.TrimSpace(desired.Profile),
				TeamID:  strings.TrimSpace(match.TeamID),
				Reason:  reason,
				Managed: strings.EqualFold(strings.TrimSpace(match.ManagedBy), desiredStateManagedBy),
			})
			out.Converged = false
			out.Status = "drifting"
		}
	}
	for _, team := range actual {
		if !strings.EqualFold(strings.TrimSpace(team.ManagedBy), desiredStateManagedBy) {
			continue
		}
		desired, exists := desiredProfiles[strings.ToLower(strings.TrimSpace(team.ProfileID))]
		if exists && desired.Enabled {
			continue
		}
		runtimeState, err := s.projectTeamRuntimeState(ctx, team)
		if err != nil {
			return protocol.ProjectDiffResult{}, err
		}
		if runtimeState.running || runtimeState.paused || runtimeState.stale {
			action := "delete"
			reason := "managed team is absent from desired state"
			if exists && !desired.Enabled {
				action = "stop"
				reason = "managed team is disabled in desired state"
			}
			out.Actions = append(out.Actions, protocol.ProjectReconcileAction{
				Action:  action,
				Profile: strings.TrimSpace(team.ProfileID),
				TeamID:  strings.TrimSpace(team.TeamID),
				Reason:  reason,
				Managed: true,
			})
			out.Converged = false
			out.Status = "drifting"
		}
	}
	for i := range out.ActualTeams {
		actualTeam := &out.ActualTeams[i]
		desired, exists := desiredProfiles[strings.ToLower(strings.TrimSpace(actualTeam.ProfileID))]
		actualTeam.DesiredEnabled = exists && desired.Enabled
		actualTeam.ReconcileStatus = out.Status
		actualTeam.ManagedBy = strings.TrimSpace(actualTeam.ManagedBy)
		for _, action := range out.Actions {
			if strings.TrimSpace(action.TeamID) == strings.TrimSpace(actualTeam.TeamID) ||
				(strings.TrimSpace(action.TeamID) == "" && strings.EqualFold(strings.TrimSpace(action.Profile), strings.TrimSpace(actualTeam.ProfileID))) {
				actualTeam.ReconcileStatus = "drifting"
				break
			}
		}
	}
	return out, nil
}

func (s *runtimeSupervisor) pickProjectTeamMatch(candidates []ProjectTeamSummary, desired ProjectDesiredStateTeam) *ProjectTeamSummary {
	if len(candidates) == 0 {
		return nil
	}
	for i := range candidates {
		if strings.EqualFold(strings.TrimSpace(candidates[i].ManagedBy), desiredStateManagedBy) {
			return &candidates[i]
		}
	}
	for i := range candidates {
		if strings.EqualFold(strings.TrimSpace(candidates[i].ProfileID), strings.TrimSpace(desired.Profile)) {
			return &candidates[i]
		}
	}
	return &candidates[0]
}

func (s *runtimeSupervisor) projectTeamRuntimeState(ctx context.Context, team ProjectTeamSummary) (projectTeamRuntimeState, error) {
	state := projectTeamRuntimeState{}
	if s == nil || s.sessionService == nil {
		return state, nil
	}
	sessionID := strings.TrimSpace(team.PrimarySessionID)
	if sessionID == "" {
		return state, nil
	}
	sess, err := s.sessionService.LoadSession(ctx, sessionID)
	if err != nil {
		return state, nil
	}
	runIDs := collectSessionRunIDs(sess)
	if len(runIDs) == 0 && strings.TrimSpace(team.CoordinatorRunID) != "" {
		runIDs = []string{strings.TrimSpace(team.CoordinatorRunID)}
	}
	for _, runID := range runIDs {
		runID = strings.TrimSpace(runID)
		if runID == "" {
			continue
		}
		run, err := s.sessionService.LoadRun(ctx, runID)
		if err != nil {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(run.Status)) {
		case types.RunStatusPaused:
			state.paused = true
		case types.RunStatusRunning:
			snapshot, ok := s.getSnapshot(runID)
			if !ok || !snapshot.WorkerPresent {
				state.stale = true
				continue
			}
			if snapshot.HandleState == handleStatePaused {
				state.paused = true
				continue
			}
			state.running = true
		}
	}
	return state, nil
}

func (s *runtimeSupervisor) startDesiredProjectTeam(ctx context.Context, projectRoot, projectID, profileID, teamID string) (protocol.SessionStartResult, error) {
	server := NewRPCServer(RPCServerConfig{
		Cfg:               s.cfg,
		Run:               types.Run{MaxBytesForContext: 8 * 1024},
		AllowAnyThread:    true,
		TaskService:       nil,
		Session:           s.sessionService,
		ManifestStore:     nil,
		ProjectTeamSvc:    s.projectTeamSvc,
		WorkspacePreparer: storage.NewDiskWorkspacePreparer(s.cfg.DataDir),
	})
	service := newSessionStartService(server)
	out, err := service.sessionStart(ctx, protocol.SessionStartParams{
		ThreadID:    protocol.ThreadID("detached-control"),
		Profile:     strings.TrimSpace(profileID),
		ProjectID:   strings.TrimSpace(projectID),
		ProjectRoot: strings.TrimSpace(projectRoot),
		TeamID:      strings.TrimSpace(teamID),
	})
	if err != nil {
		return protocol.SessionStartResult{}, err
	}
	if s.projectTeamSvc != nil {
		if _, err := s.projectTeamSvc.SetStatus(ctx, projectRoot, out.TeamID, "active", map[string]any{
			"managedBy":       desiredStateManagedBy,
			"desiredProfile":  strings.TrimSpace(profileID),
			"desiredEnabled":  true,
			"reconcileStatus": "reconciling",
			"reconcileRun":    uuid.NewString(),
		}); err != nil {
			return protocol.SessionStartResult{}, err
		}
	}
	if s.projectRegistrySvc != nil {
		_, _ = s.projectRegistrySvc.RegisterProject(ctx, ProjectRegistrySummary{
			ProjectRoot:  projectRoot,
			ProjectID:    projectID,
			ManifestPath: projectDesiredStatePath(projectRoot),
			Enabled:      true,
			Metadata: map[string]any{
				"source": "desired-state",
			},
		})
	}
	if s.sessionService != nil {
		sess, err := s.sessionService.LoadSession(ctx, strings.TrimSpace(out.SessionID))
		if err == nil {
			for _, runID := range collectSessionRunIDs(sess) {
				_ = s.trySendCmd(supervisorCmd{
					kind:      cmdSpawn,
					runID:     strings.TrimSpace(runID),
					sessionID: strings.TrimSpace(out.SessionID),
					sess:      &sess,
				})
			}
		}
	}
	return out, nil
}

func (s *runtimeSupervisor) notifyProjectReconcile(method, projectRoot, projectID, status string, actions []protocol.ProjectReconcileAction, teamIDs []string, errText string) error {
	if s == nil || s.broadcaster == nil {
		return nil
	}
	return s.broadcaster.Notify(method, projectReconcileNotification{
		ProjectRoot: strings.TrimSpace(projectRoot),
		ProjectID:   strings.TrimSpace(projectID),
		Converged:   strings.EqualFold(strings.TrimSpace(status), "converged"),
		Status:      strings.TrimSpace(status),
		Actions:     actions,
		TeamIDs:     teamIDs,
		TickAt:      time.Now().UTC().Format(time.RFC3339Nano),
		Error:       strings.TrimSpace(errText),
	})
}

func projectTeamIDsForDiff(diff protocol.ProjectDiffResult) []string {
	seen := map[string]struct{}{}
	teamIDs := make([]string, 0, len(diff.ActualTeams)+len(diff.Actions))
	for _, team := range diff.ActualTeams {
		teamID := strings.TrimSpace(team.TeamID)
		if teamID == "" {
			continue
		}
		if _, ok := seen[teamID]; ok {
			continue
		}
		seen[teamID] = struct{}{}
		teamIDs = append(teamIDs, teamID)
	}
	for _, action := range diff.Actions {
		teamID := strings.TrimSpace(action.TeamID)
		if teamID == "" {
			continue
		}
		if _, ok := seen[teamID]; ok {
			continue
		}
		seen[teamID] = struct{}{}
		teamIDs = append(teamIDs, teamID)
	}
	return teamIDs
}

func heartbeatOverride(item ProjectDesiredStateTeam) string {
	if item.Heartbeat == nil {
		return ""
	}
	return strings.TrimSpace(item.Heartbeat.OverrideInterval)
}
