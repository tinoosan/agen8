package app

import (
	"context"
	"fmt"
	"strings"

	implstore "github.com/tinoosan/agen8/internal/store"
	"github.com/tinoosan/agen8/pkg/config"
)

type ProjectRegistrySummary struct {
	ProjectRoot  string
	ProjectID    string
	ManifestPath string
	Enabled      bool
	CreatedAt    string
	UpdatedAt    string
	Metadata     map[string]any
}

type ProjectRegistryService struct {
	cfg config.Config
}

func NewProjectRegistryService(cfg config.Config) *ProjectRegistryService {
	return &ProjectRegistryService{cfg: cfg}
}

func (s *ProjectRegistryService) RegisterProject(ctx context.Context, summary ProjectRegistrySummary) (ProjectRegistrySummary, error) {
	if s == nil {
		return ProjectRegistrySummary{}, fmt.Errorf("project registry service is nil")
	}
	record, err := implstore.UpsertProjectRegistry(ctx, s.cfg, implstore.ProjectRegistryRecord{
		ProjectRoot:  strings.TrimSpace(summary.ProjectRoot),
		ProjectID:    strings.TrimSpace(summary.ProjectID),
		ManifestPath: strings.TrimSpace(summary.ManifestPath),
		Enabled:      summary.Enabled,
		CreatedAt:    strings.TrimSpace(summary.CreatedAt),
		Metadata:     summary.Metadata,
	})
	if err != nil {
		return ProjectRegistrySummary{}, err
	}
	return toProjectRegistrySummary(record), nil
}

func (s *ProjectRegistryService) GetProject(ctx context.Context, projectRoot string) (ProjectRegistrySummary, error) {
	if s == nil {
		return ProjectRegistrySummary{}, fmt.Errorf("project registry service is nil")
	}
	record, err := implstore.LoadProjectRegistry(ctx, s.cfg, strings.TrimSpace(projectRoot))
	if err != nil {
		return ProjectRegistrySummary{}, err
	}
	return toProjectRegistrySummary(record), nil
}

func (s *ProjectRegistryService) ListProjects(ctx context.Context) ([]ProjectRegistrySummary, error) {
	if s == nil {
		return nil, fmt.Errorf("project registry service is nil")
	}
	records, err := implstore.ListProjectRegistry(ctx, s.cfg)
	if err != nil {
		return nil, err
	}
	out := make([]ProjectRegistrySummary, 0, len(records))
	for _, record := range records {
		out = append(out, toProjectRegistrySummary(record))
	}
	return out, nil
}

func toProjectRegistrySummary(record implstore.ProjectRegistryRecord) ProjectRegistrySummary {
	return ProjectRegistrySummary{
		ProjectRoot:  strings.TrimSpace(record.ProjectRoot),
		ProjectID:    strings.TrimSpace(record.ProjectID),
		ManifestPath: strings.TrimSpace(record.ManifestPath),
		Enabled:      record.Enabled,
		CreatedAt:    strings.TrimSpace(record.CreatedAt),
		UpdatedAt:    strings.TrimSpace(record.UpdatedAt),
		Metadata:     record.Metadata,
	}
}
