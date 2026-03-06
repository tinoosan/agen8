package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tinoosan/agen8/pkg/profile"
	"github.com/tinoosan/agen8/pkg/protocol"
	"github.com/tinoosan/agen8/pkg/services/team"
	"github.com/tinoosan/agen8/pkg/types"
)

type sessionStartService struct {
	server *RPCServer
}

func newSessionStartService(server *RPCServer) *sessionStartService {
	return &sessionStartService{server: server}
}

func (s *sessionStartService) sessionStart(ctx context.Context, p protocol.SessionStartParams) (protocol.SessionStartResult, error) {
	srv := s.server
	if srv == nil {
		return protocol.SessionStartResult{}, fmt.Errorf("rpc server is nil")
	}
	if _, err := srv.resolveThreadID(p.ThreadID); err != nil {
		return protocol.SessionStartResult{}, err
	}
	if err := EnsureRuntimeAuthReady(ctx, srv.cfg.DataDir, ""); err != nil {
		return protocol.SessionStartResult{}, &protocol.ProtocolError{
			Code:    protocol.CodeInvalidState,
			Message: err.Error(),
		}
	}
	requestedMode := strings.ToLower(strings.TrimSpace(p.Mode))
	if requestedMode != "" && requestedMode != "single-agent" && requestedMode != "multi-agent" {
		return protocol.SessionStartResult{}, &protocol.ProtocolError{
			Code:    protocol.CodeInvalidParams,
			Message: "mode must be single-agent or multi-agent",
		}
	}
	profileRef := strings.TrimSpace(p.Profile)
	if profileRef == "" {
		profileRef = "general"
	}
	prof, _, err := resolveProfileRef(srv.cfg, profileRef)
	if err != nil {
		return protocol.SessionStartResult{}, &protocol.ProtocolError{Code: protocol.CodeInvalidParams, Message: "load profile: " + err.Error()}
	}
	if prof == nil {
		return protocol.SessionStartResult{}, &protocol.ProtocolError{Code: protocol.CodeInvalidParams, Message: "profile not found"}
	}
	roles, err := prof.RolesForSession()
	if err != nil {
		return protocol.SessionStartResult{}, &protocol.ProtocolError{Code: protocol.CodeInvalidParams, Message: err.Error()}
	}
	_, coordinatorRole, err := team.ValidateTeamRoles(roles)
	if err != nil {
		return protocol.SessionStartResult{}, &protocol.ProtocolError{Code: protocol.CodeInvalidParams, Message: err.Error()}
	}
	teamRoles := append([]profile.RoleConfig(nil), roles...)
	reviewerCfg, reviewerEnabled := prof.ReviewerForSession()
	mode := "single-agent"
	if len(teamRoles) > 1 || reviewerEnabled {
		mode = "multi-agent"
	}

	goal := strings.TrimSpace(p.Goal)
	maxContext := srv.run.MaxBytesForContext
	if maxContext <= 0 {
		maxContext = 8 * 1024
	}
	sess := types.NewSession(goal)
	sess.CurrentGoal = goal
	sess.Profile = strings.TrimSpace(prof.ID)
	sess.ProjectRoot = strings.TrimSpace(p.ProjectRoot)
	teamID := "team-" + uuid.NewString()
	sess.TeamID = teamID
	sess.Mode = mode

	teamModel := strings.TrimSpace(p.Model)
	if teamModel == "" {
		teamModel = prof.TeamModelForSession()
	}
	if teamModel == "" {
		teamModel = strings.TrimSpace(prof.Model)
	}
	if teamModel != "" {
		sess.ActiveModel = teamModel
	}
	ensureSessionReasoningForModel(&sess, sess.ActiveModel, "", "")
	if err := srv.session.SaveSession(ctx, sess); err != nil {
		return protocol.SessionStartResult{}, err
	}

	runIDs := make([]string, 0, len(teamRoles))
	manifestRoles := make([]team.RoleRecord, 0, len(teamRoles)+1)
	primaryRunID := ""
	for _, role := range teamRoles {
		roleName := strings.TrimSpace(role.Name)
		if roleName == "" {
			continue
		}
		roleGoal := strings.TrimSpace(role.Description)
		if roleGoal == "" {
			roleGoal = goal
		}
		run := types.NewRun(roleGoal, maxContext, strings.TrimSpace(sess.SessionID))
		roleModel := resolveRoleModel(role, teamModel)
		if roleModel == "" {
			return protocol.SessionStartResult{}, &protocol.ProtocolError{Code: protocol.CodeInvalidParams, Message: "no model resolved for role " + roleName}
		}
		run.Runtime = &types.RunRuntimeConfig{
			Profile:     strings.TrimSpace(prof.ID),
			Model:       roleModel,
			TeamID:      strings.TrimSpace(teamID),
			Role:        roleName,
			WorkerClass: "persistent",
		}
		if err := srv.session.SaveRun(ctx, run); err != nil {
			return protocol.SessionStartResult{}, err
		}
		runID := strings.TrimSpace(run.RunID)
		exists := false
		for _, id := range sess.Runs {
			if strings.TrimSpace(id) == runID {
				exists = true
				break
			}
		}
		if !exists {
			sess.Runs = append(sess.Runs, runID)
		}
		runIDs = append(runIDs, runID)
		manifestRoles = append(manifestRoles, team.RoleRecord{
			RoleName:  roleName,
			RunID:     runID,
			SessionID: strings.TrimSpace(sess.SessionID),
		})
		if strings.EqualFold(roleName, coordinatorRole) && primaryRunID == "" {
			primaryRunID = runID
		}
	}
	if reviewerEnabled && reviewerCfg != nil {
		reviewerGoal := strings.TrimSpace(reviewerCfg.Description)
		if reviewerGoal == "" {
			reviewerGoal = goal
		}
		reviewerRun := types.NewRun(reviewerGoal, maxContext, strings.TrimSpace(sess.SessionID))
		reviewerModel := strings.TrimSpace(reviewerCfg.Model)
		if reviewerModel == "" {
			reviewerModel = teamModel
		}
		if reviewerModel == "" {
			return protocol.SessionStartResult{}, &protocol.ProtocolError{Code: protocol.CodeInvalidParams, Message: "no model resolved for reviewer"}
		}
		reviewerName := strings.TrimSpace(reviewerCfg.EffectiveName())
		reviewerRun.Runtime = &types.RunRuntimeConfig{
			Profile:     strings.TrimSpace(prof.ID),
			Model:       reviewerModel,
			TeamID:      strings.TrimSpace(teamID),
			Role:        reviewerName,
			WorkerClass: "persistent",
		}
		if err := srv.session.SaveRun(ctx, reviewerRun); err != nil {
			return protocol.SessionStartResult{}, err
		}
		reviewerRunID := strings.TrimSpace(reviewerRun.RunID)
		sess.Runs = append(sess.Runs, reviewerRunID)
		runIDs = append(runIDs, reviewerRunID)
		manifestRoles = append(manifestRoles, team.RoleRecord{
			RoleName:  reviewerName,
			RunID:     reviewerRunID,
			SessionID: strings.TrimSpace(sess.SessionID),
		})
	}
	if primaryRunID == "" && len(runIDs) > 0 {
		primaryRunID = runIDs[0]
	}
	if primaryRunID == "" {
		return protocol.SessionStartResult{}, &protocol.ProtocolError{Code: protocol.CodeInvalidState, Message: "profile produced no runs"}
	}
	sess.CurrentRunID = primaryRunID
	if err := srv.session.SaveSession(ctx, sess); err != nil {
		return protocol.SessionStartResult{}, err
	}

	if err := srv.workspacePreparer.PrepareTeamWorkspace(ctx, teamID); err != nil {
		return protocol.SessionStartResult{}, err
	}
	manifest := team.BuildManifest(teamID, strings.TrimSpace(prof.ID), coordinatorRole, primaryRunID, teamModel, manifestRoles, time.Now().UTC().Format(time.RFC3339Nano))
	if err := srv.manifestStore.Save(ctx, manifest); err != nil {
		return protocol.SessionStartResult{}, err
	}
	if goal != "" {
		if err := team.SeedCoordinatorTask(ctx, srv.taskService, strings.TrimSpace(sess.SessionID), primaryRunID, teamID, coordinatorRole, goal); err != nil {
			return protocol.SessionStartResult{}, err
		}
	}
	return protocol.SessionStartResult{
		SessionID:    strings.TrimSpace(sess.SessionID),
		PrimaryRunID: primaryRunID,
		Mode:         sess.Mode,
		Profile:      strings.TrimSpace(prof.ID),
		Model:        teamModel,
		TeamID:       teamID,
		RunIDs:       runIDs,
	}, nil
}
