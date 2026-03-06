package app

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	implstore "github.com/tinoosan/agen8/internal/store"
	"github.com/tinoosan/agen8/pkg/config"
	"github.com/tinoosan/agen8/pkg/services/session"
	"github.com/tinoosan/agen8/pkg/services/team"
	pkgstore "github.com/tinoosan/agen8/pkg/store"
)

var ErrProjectTeamsMigrationRequired = errors.New("project teams not registered")

type ProjectTeamSummary struct {
	ProjectID        string
	ProjectRoot      string
	TeamID           string
	ProfileID        string
	PrimarySessionID string
	CoordinatorRunID string
	Status           string
	CreatedAt        string
	UpdatedAt        string
	ManifestPresent  bool
}

type legacyProjectTeam struct {
	ProjectRoot      string
	TeamID           string
	ProfileID        string
	PrimarySessionID string
	CoordinatorRunID string
	Status           string
	CreatedAt        string
	UpdatedAt        string
}

// ProjectTeamService is the canonical extension seam for project->team discovery.
// Project-facing callers depend on this service instead of inferring teams from sessions.
type ProjectTeamService struct {
	cfg           config.Config
	session       session.Service
	manifestStore team.ManifestStore
}

func NewProjectTeamService(cfg config.Config, sessionSvc session.Service, manifestStore team.ManifestStore) *ProjectTeamService {
	return &ProjectTeamService{
		cfg:           cfg,
		session:       sessionSvc,
		manifestStore: manifestStore,
	}
}

func MigrateProjectTeamRegistry(ctx context.Context, cfg config.Config, start string) ([]ProjectTeamSummary, error) {
	projectCtx, err := LoadProjectContext(start)
	if err != nil {
		return nil, err
	}
	if !projectCtx.Exists {
		return nil, fmt.Errorf("project is not initialized; run `agen8 project init` first")
	}
	sessionStore, err := implstore.NewSQLiteSessionStore(cfg)
	if err != nil {
		return nil, err
	}
	sessionSvc := session.NewManager(cfg, sessionStore, nil)
	svc := NewProjectTeamService(cfg, sessionSvc, team.NewFileManifestStore(cfg))
	return svc.MigrateProject(ctx, strings.TrimSpace(projectCtx.RootDir), strings.TrimSpace(projectCtx.Config.ProjectID))
}

func (s *ProjectTeamService) RegisterTeam(ctx context.Context, summary ProjectTeamSummary) (ProjectTeamSummary, error) {
	if s == nil {
		return ProjectTeamSummary{}, fmt.Errorf("project team service is nil")
	}
	record, err := implstore.UpsertProjectTeam(ctx, s.cfg, implstore.ProjectTeamRecord{
		ProjectRoot:      strings.TrimSpace(summary.ProjectRoot),
		ProjectID:        strings.TrimSpace(summary.ProjectID),
		TeamID:           strings.TrimSpace(summary.TeamID),
		ProfileID:        strings.TrimSpace(summary.ProfileID),
		PrimarySessionID: strings.TrimSpace(summary.PrimarySessionID),
		CoordinatorRunID: strings.TrimSpace(summary.CoordinatorRunID),
		Status:           normalizeProjectTeamStatus(summary.Status),
		CreatedAt:        strings.TrimSpace(summary.CreatedAt),
		Metadata: map[string]any{
			"source": "session.start",
		},
	})
	if err != nil {
		return ProjectTeamSummary{}, err
	}
	return s.toSummary(ctx, record), nil
}

func (s *ProjectTeamService) UnregisterTeam(ctx context.Context, projectRoot, teamID string) error {
	if s == nil {
		return fmt.Errorf("project team service is nil")
	}
	return implstore.DeleteProjectTeam(ctx, s.cfg, strings.TrimSpace(projectRoot), strings.TrimSpace(teamID))
}

func (s *ProjectTeamService) ListTeams(ctx context.Context, projectRoot string) ([]ProjectTeamSummary, error) {
	if s == nil {
		return nil, fmt.Errorf("project team service is nil")
	}
	projectRoot = strings.TrimSpace(projectRoot)
	if projectRoot == "" {
		return nil, fmt.Errorf("project root is required")
	}
	records, err := implstore.ListProjectTeams(ctx, s.cfg, projectRoot)
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		legacy, legacyErr := s.discoverLegacyTeams(ctx, projectRoot)
		if legacyErr != nil {
			return nil, legacyErr
		}
		if len(legacy) > 0 {
			return nil, fmt.Errorf("%w for project %s; run `agen8 project migrate-teams`", ErrProjectTeamsMigrationRequired, projectRoot)
		}
	}
	out := make([]ProjectTeamSummary, 0, len(records))
	for _, record := range records {
		out = append(out, s.toSummary(ctx, record))
	}
	return out, nil
}

func (s *ProjectTeamService) GetTeam(ctx context.Context, projectRoot, teamID string) (ProjectTeamSummary, error) {
	if s == nil {
		return ProjectTeamSummary{}, fmt.Errorf("project team service is nil")
	}
	record, err := implstore.LoadProjectTeam(ctx, s.cfg, strings.TrimSpace(projectRoot), strings.TrimSpace(teamID))
	if err != nil {
		if errors.Is(err, implstore.ErrNotFound) {
			legacy, legacyErr := s.discoverLegacyTeams(ctx, projectRoot)
			if legacyErr != nil {
				return ProjectTeamSummary{}, legacyErr
			}
			if len(legacy) > 0 {
				return ProjectTeamSummary{}, fmt.Errorf("%w for project %s; run `agen8 project migrate-teams`", ErrProjectTeamsMigrationRequired, projectRoot)
			}
		}
		return ProjectTeamSummary{}, err
	}
	return s.toSummary(ctx, record), nil
}

func (s *ProjectTeamService) MigrateProject(ctx context.Context, projectRoot, projectID string) ([]ProjectTeamSummary, error) {
	projectRoot = strings.TrimSpace(projectRoot)
	projectID = strings.TrimSpace(projectID)
	if projectRoot == "" {
		return nil, fmt.Errorf("project root is required")
	}
	legacy, err := s.discoverLegacyTeams(ctx, projectRoot)
	if err != nil {
		return nil, err
	}
	out := make([]ProjectTeamSummary, 0, len(legacy))
	for _, item := range legacy {
		record, err := implstore.UpsertProjectTeam(ctx, s.cfg, implstore.ProjectTeamRecord{
			ProjectRoot:      projectRoot,
			ProjectID:        projectID,
			TeamID:           item.TeamID,
			ProfileID:        item.ProfileID,
			PrimarySessionID: item.PrimarySessionID,
			CoordinatorRunID: item.CoordinatorRunID,
			Status:           normalizeProjectTeamStatus(item.Status),
			CreatedAt:        item.CreatedAt,
			Metadata: map[string]any{
				"source": "project.migrateTeams",
			},
		})
		if err != nil {
			return nil, err
		}
		out = append(out, s.toSummary(ctx, record))
	}
	return out, nil
}

func (s *ProjectTeamService) discoverLegacyTeams(ctx context.Context, projectRoot string) ([]legacyProjectTeam, error) {
	if s == nil {
		return nil, fmt.Errorf("project team service is nil")
	}
	projectRoot = strings.TrimSpace(projectRoot)
	if projectRoot == "" {
		return nil, fmt.Errorf("project root is required")
	}
	if s.session == nil {
		return nil, nil
	}
	sessions, err := s.session.ListSessionsPaginated(ctx, pkgstore.SessionFilter{
		ProjectRoot:   projectRoot,
		IncludeSystem: true,
		Limit:         5000,
		Offset:        0,
		SortBy:        "updated_at",
		SortDesc:      true,
	})
	if err != nil {
		return nil, err
	}
	seen := map[string]legacyProjectTeam{}
	for _, sess := range sessions {
		teamID := strings.TrimSpace(sess.TeamID)
		if teamID == "" {
			continue
		}
		manifest, err := s.manifestStore.Load(ctx, teamID)
		if err != nil {
			return nil, fmt.Errorf("load manifest for legacy team %s: %w", teamID, err)
		}
		if manifest == nil {
			return nil, fmt.Errorf("legacy project team %s is missing manifest data", teamID)
		}
		item := legacyProjectTeam{
			ProjectRoot:      projectRoot,
			TeamID:           teamID,
			ProfileID:        strings.TrimSpace(manifest.ProfileID),
			PrimarySessionID: strings.TrimSpace(sess.SessionID),
			CoordinatorRunID: strings.TrimSpace(manifest.CoordinatorRun),
			Status:           implstore.ProjectTeamStatusActive,
		}
		if item.ProfileID == "" {
			item.ProfileID = strings.TrimSpace(sess.Profile)
		}
		if item.CoordinatorRunID == "" {
			item.CoordinatorRunID = strings.TrimSpace(sess.CurrentRunID)
		}
		if item.ProfileID == "" || item.CoordinatorRunID == "" {
			return nil, fmt.Errorf("legacy project team %s has incomplete manifest/session ownership data", teamID)
		}
		if sess.CreatedAt != nil && !sess.CreatedAt.IsZero() {
			item.CreatedAt = sess.CreatedAt.UTC().Format(timeFormatRFC3339Nano)
		}
		if sess.UpdatedAt != nil && !sess.UpdatedAt.IsZero() {
			item.UpdatedAt = sess.UpdatedAt.UTC().Format(timeFormatRFC3339Nano)
		}
		if existing, ok := seen[teamID]; ok {
			if existing.PrimarySessionID != item.PrimarySessionID || existing.CoordinatorRunID != item.CoordinatorRunID || existing.ProfileID != item.ProfileID {
				return nil, fmt.Errorf("legacy migration conflict for project %s team %s", projectRoot, teamID)
			}
			continue
		}
		seen[teamID] = item
	}
	out := make([]legacyProjectTeam, 0, len(seen))
	for _, item := range seen {
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].TeamID < out[j].TeamID
	})
	return out, nil
}

func (s *ProjectTeamService) toSummary(ctx context.Context, record implstore.ProjectTeamRecord) ProjectTeamSummary {
	summary := ProjectTeamSummary{
		ProjectID:        strings.TrimSpace(record.ProjectID),
		ProjectRoot:      strings.TrimSpace(record.ProjectRoot),
		TeamID:           strings.TrimSpace(record.TeamID),
		ProfileID:        strings.TrimSpace(record.ProfileID),
		PrimarySessionID: strings.TrimSpace(record.PrimarySessionID),
		CoordinatorRunID: strings.TrimSpace(record.CoordinatorRunID),
		Status:           normalizeProjectTeamStatus(record.Status),
		CreatedAt:        strings.TrimSpace(record.CreatedAt),
		UpdatedAt:        strings.TrimSpace(record.UpdatedAt),
	}
	if s.manifestStore != nil {
		if manifest, err := s.manifestStore.Load(ctx, summary.TeamID); err == nil && manifest != nil {
			summary.ManifestPresent = true
			if summary.ProfileID == "" {
				summary.ProfileID = strings.TrimSpace(manifest.ProfileID)
			}
			if summary.CoordinatorRunID == "" {
				summary.CoordinatorRunID = strings.TrimSpace(manifest.CoordinatorRun)
			}
		}
	}
	return summary
}

func normalizeProjectTeamStatus(status string) string {
	status = strings.ToLower(strings.TrimSpace(status))
	switch status {
	case "", implstore.ProjectTeamStatusRegistered:
		return implstore.ProjectTeamStatusRegistered
	case implstore.ProjectTeamStatusActive:
		return implstore.ProjectTeamStatusActive
	default:
		return status
	}
}

const timeFormatRFC3339Nano = "2006-01-02T15:04:05.999999999Z07:00"
