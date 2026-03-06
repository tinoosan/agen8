package app

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strings"

	implstore "github.com/tinoosan/agen8/internal/store"
	"github.com/tinoosan/agen8/pkg/config"
	"github.com/tinoosan/agen8/pkg/fsutil"
	pkgsession "github.com/tinoosan/agen8/pkg/services/session"
	"github.com/tinoosan/agen8/pkg/services/team"
)

type TeamDeleteInput struct {
	TeamID      string
	ProjectRoot string
}

type TeamDeleteSummary struct {
	TeamID             string
	ProjectRoot        string
	DeletedSessionIDs  []string
	DeletedArtifactSet bool
}

type TeamDeleteService struct {
	cfg            config.Config
	session        pkgsession.Service
	manifestStore  team.ManifestStore
	projectTeamSvc *ProjectTeamService
}

func NewTeamDeleteService(cfg config.Config, session pkgsession.Service, manifestStore team.ManifestStore, projectTeamSvc *ProjectTeamService) *TeamDeleteService {
	return &TeamDeleteService{
		cfg:            cfg,
		session:        session,
		manifestStore:  manifestStore,
		projectTeamSvc: projectTeamSvc,
	}
}

func (s *TeamDeleteService) DeleteTeam(ctx context.Context, input TeamDeleteInput) (TeamDeleteSummary, error) {
	if s == nil {
		return TeamDeleteSummary{}, fmt.Errorf("team delete service is nil")
	}
	teamID := strings.TrimSpace(input.TeamID)
	if teamID == "" {
		return TeamDeleteSummary{}, fmt.Errorf("team id is required")
	}
	projectRoot := strings.TrimSpace(input.ProjectRoot)

	sessionIDs := make([]string, 0, 4)
	seen := map[string]struct{}{}
	addSessionID := func(sessionID string) {
		sessionID = strings.TrimSpace(sessionID)
		if sessionID == "" {
			return
		}
		if _, ok := seen[sessionID]; ok {
			return
		}
		seen[sessionID] = struct{}{}
		sessionIDs = append(sessionIDs, sessionID)
	}

	if s.manifestStore != nil {
		manifest, err := s.manifestStore.Load(ctx, teamID)
		if err != nil {
			return TeamDeleteSummary{}, fmt.Errorf("load team manifest: %w", err)
		}
		if manifest != nil {
			for _, role := range manifest.Roles {
				addSessionID(role.SessionID)
			}
		}
	}
	if projectRoot != "" && s.projectTeamSvc != nil {
		if summary, err := s.projectTeamSvc.GetTeam(ctx, projectRoot, teamID); err == nil {
			addSessionID(summary.PrimarySessionID)
		}
	}
	if s.session != nil {
		for _, sessionID := range append([]string(nil), sessionIDs...) {
			sess, err := s.session.LoadSession(ctx, sessionID)
			if err != nil {
				continue
			}
			if projectRoot == "" {
				projectRoot = strings.TrimSpace(sess.ProjectRoot)
			}
		}
	}
	if len(sessionIDs) == 0 {
		teamDir := fsutil.GetTeamDir(s.cfg.DataDir, teamID)
		if _, err := os.Stat(teamDir); err != nil {
			if os.IsNotExist(err) {
				return TeamDeleteSummary{}, fmt.Errorf("team %s not found", teamID)
			}
			return TeamDeleteSummary{}, fmt.Errorf("stat team dir: %w", err)
		}
	}

	if err := implstore.DeleteTeamScopedData(ctx, s.cfg, teamID); err != nil {
		return TeamDeleteSummary{}, err
	}
	deletedSessionIDs := make([]string, 0, len(sessionIDs))
	if s.session != nil {
		for _, sessionID := range sessionIDs {
			if err := s.session.Delete(ctx, sessionID); err != nil {
				return TeamDeleteSummary{}, fmt.Errorf("delete team session %s: %w", sessionID, err)
			}
			deletedSessionIDs = append(deletedSessionIDs, sessionID)
		}
	}
	if err := os.RemoveAll(fsutil.GetTeamDir(s.cfg.DataDir, teamID)); err != nil {
		return TeamDeleteSummary{}, fmt.Errorf("remove team dir: %w", err)
	}
	if projectRoot != "" && s.projectTeamSvc != nil {
		if err := s.projectTeamSvc.UnregisterTeam(ctx, projectRoot, teamID); err != nil && !strings.Contains(strings.ToLower(err.Error()), "not found") {
			return TeamDeleteSummary{}, err
		}
		if projectCtx, err := LoadProjectContext(projectRoot); err == nil && projectCtx.Exists && strings.TrimSpace(projectCtx.State.ActiveTeamID) == teamID {
			_, _ = SetActiveSession(projectRoot, ProjectState{LastCommand: "team.delete"})
		}
	}
	slices.Sort(deletedSessionIDs)
	return TeamDeleteSummary{
		TeamID:             teamID,
		ProjectRoot:        projectRoot,
		DeletedSessionIDs:  deletedSessionIDs,
		DeletedArtifactSet: true,
	}, nil
}
