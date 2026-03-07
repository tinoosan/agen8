package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const ProjectDesiredStateFilename = "agen8.yaml"

type ProjectDesiredState struct {
	ProjectID string                    `yaml:"projectId"`
	Teams     []ProjectDesiredStateTeam `yaml:"teams"`
}

type ProjectDesiredStateTeam struct {
	Profile   string                            `yaml:"profile"`
	Enabled   bool                              `yaml:"enabled"`
	Heartbeat *ProjectDesiredStateTeamHeartbeat `yaml:"heartbeat,omitempty"`
}

type ProjectDesiredStateTeamHeartbeat struct {
	OverrideInterval string `yaml:"overrideInterval,omitempty"`
}

func projectDesiredStatePath(root string) string {
	return filepath.Join(strings.TrimSpace(root), ProjectDirName, ProjectDesiredStateFilename)
}

func defaultProjectDesiredState(projectID string) ProjectDesiredState {
	return ProjectDesiredState{
		ProjectID: strings.TrimSpace(projectID),
		Teams:     []ProjectDesiredStateTeam{},
	}
}

func normalizeProjectDesiredState(state ProjectDesiredState, root string) ProjectDesiredState {
	state.ProjectID = strings.TrimSpace(state.ProjectID)
	if state.ProjectID == "" {
		state.ProjectID = defaultProjectConfig(root).ProjectID
	}
	teams := make([]ProjectDesiredStateTeam, 0, len(state.Teams))
	for _, item := range state.Teams {
		item.Profile = strings.TrimSpace(item.Profile)
		if item.Heartbeat != nil {
			item.Heartbeat.OverrideInterval = strings.TrimSpace(item.Heartbeat.OverrideInterval)
			if item.Heartbeat.OverrideInterval == "" {
				item.Heartbeat = nil
			}
		}
		teams = append(teams, item)
	}
	state.Teams = teams
	return state
}

func readProjectDesiredState(root string) (ProjectDesiredState, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return ProjectDesiredState{}, fmt.Errorf("project root is required")
	}
	path := projectDesiredStatePath(root)
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return defaultProjectDesiredState(defaultProjectConfig(root).ProjectID), nil
		}
		return ProjectDesiredState{}, fmt.Errorf("read %s: %w", path, err)
	}
	var state ProjectDesiredState
	if err := yaml.Unmarshal(b, &state); err != nil {
		return ProjectDesiredState{}, fmt.Errorf("parse %s: %w", path, err)
	}
	return normalizeProjectDesiredState(state, root), nil
}

func validateProjectDesiredState(state ProjectDesiredState, root string) error {
	seenProfiles := make(map[string]struct{}, len(state.Teams))
	for _, item := range state.Teams {
		if item.Profile == "" {
			return fmt.Errorf("%s contains a team entry with an empty profile", projectDesiredStatePath(root))
		}
		key := strings.ToLower(item.Profile)
		if _, exists := seenProfiles[key]; exists {
			return fmt.Errorf("%s contains duplicate desired team profile %q", projectDesiredStatePath(root), item.Profile)
		}
		seenProfiles[key] = struct{}{}
	}
	return nil
}

func writeProjectDesiredState(root string, state ProjectDesiredState) error {
	root = strings.TrimSpace(root)
	if root == "" {
		return fmt.Errorf("project root is required")
	}
	state = normalizeProjectDesiredState(state, root)
	if err := validateProjectDesiredState(state, root); err != nil {
		return err
	}
	b, err := yaml.Marshal(state)
	if err != nil {
		return fmt.Errorf("encode %s: %w", projectDesiredStatePath(root), err)
	}
	if err := os.MkdirAll(filepath.Dir(projectDesiredStatePath(root)), 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(projectDesiredStatePath(root)), err)
	}
	return os.WriteFile(projectDesiredStatePath(root), b, 0o644)
}

func ensureProjectDesiredState(root string, projectID string) error {
	path := projectDesiredStatePath(root)
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat %s: %w", path, err)
	}
	return writeProjectDesiredState(root, defaultProjectDesiredState(projectID))
}
