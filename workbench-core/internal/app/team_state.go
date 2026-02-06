package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/tinoosan/workbench-core/pkg/config"
	"github.com/tinoosan/workbench-core/pkg/fsutil"
)

const teamControlSetModelFile = "set-model.json"

type teamControlSetModel struct {
	Type        string `json:"type"`
	Command     string `json:"command"`
	Model       string `json:"model"`
	RequestedAt string `json:"requestedAt"`
}

type teamStateManager struct {
	cfg        config.Config
	teamID     string
	manifestMu sync.Mutex
	manifest   teamManifest
}

func newTeamStateManager(cfg config.Config, manifest teamManifest) *teamStateManager {
	return &teamStateManager{
		cfg:      cfg,
		teamID:   strings.TrimSpace(manifest.TeamID),
		manifest: manifest,
	}
}

func (m *teamStateManager) teamDir() string {
	return fsutil.GetTeamDir(m.cfg.DataDir, m.teamID)
}

func (m *teamStateManager) controlDir() string {
	return filepath.Join(m.teamDir(), "control")
}

func (m *teamStateManager) controlPath() string {
	return filepath.Join(m.controlDir(), teamControlSetModelFile)
}

func (m *teamStateManager) currentModel() string {
	m.manifestMu.Lock()
	defer m.manifestMu.Unlock()
	return strings.TrimSpace(m.manifest.TeamModel)
}

func (m *teamStateManager) manifestSnapshot() teamManifest {
	m.manifestMu.Lock()
	defer m.manifestMu.Unlock()
	return m.manifest
}

func (m *teamStateManager) saveManifest() error {
	m.manifestMu.Lock()
	manifest := m.manifest
	m.manifestMu.Unlock()
	return writeTeamManifestFile(m.cfg, manifest)
}

func (m *teamStateManager) updateManifest(mutator func(*teamManifest)) error {
	m.manifestMu.Lock()
	mutator(&m.manifest)
	manifest := m.manifest
	m.manifestMu.Unlock()
	return writeTeamManifestFile(m.cfg, manifest)
}

func (m *teamStateManager) queueModelChange(model, reason string) error {
	model = strings.TrimSpace(model)
	if model == "" {
		return fmt.Errorf("model is required")
	}
	return m.updateManifest(func(manifest *teamManifest) {
		now := time.Now().UTC().Format(time.RFC3339Nano)
		manifest.ModelChange = &teamModelChange{
			RequestedModel: model,
			Status:         "pending",
			RequestedAt:    now,
			Reason:         strings.TrimSpace(reason),
		}
	})
}

func (m *teamStateManager) markModelApplied(model string) error {
	model = strings.TrimSpace(model)
	if model == "" {
		return fmt.Errorf("model is required")
	}
	return m.updateManifest(func(manifest *teamManifest) {
		now := time.Now().UTC().Format(time.RFC3339Nano)
		manifest.TeamModel = model
		manifest.ModelChange = &teamModelChange{
			RequestedModel: model,
			Status:         "applied",
			RequestedAt:    now,
			AppliedAt:      now,
		}
	})
}

func (m *teamStateManager) markModelFailed(model string, err error) error {
	errMsg := "unknown error"
	if err != nil {
		errMsg = err.Error()
	}
	return m.updateManifest(func(manifest *teamManifest) {
		now := time.Now().UTC().Format(time.RFC3339Nano)
		manifest.ModelChange = &teamModelChange{
			RequestedModel: strings.TrimSpace(model),
			Status:         "failed",
			RequestedAt:    now,
			AppliedAt:      now,
			Error:          errMsg,
		}
	})
}

func (m *teamStateManager) readControlRequest() (teamControlSetModel, bool, error) {
	path := m.controlPath()
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return teamControlSetModel{}, false, nil
		}
		return teamControlSetModel{}, false, err
	}
	var req teamControlSetModel
	if err := json.Unmarshal(raw, &req); err != nil {
		return teamControlSetModel{}, false, err
	}
	if strings.ToLower(strings.TrimSpace(req.Type)) != "team_control" {
		return teamControlSetModel{}, false, nil
	}
	if strings.ToLower(strings.TrimSpace(req.Command)) != "set_team_model" {
		return teamControlSetModel{}, false, nil
	}
	req.Model = strings.TrimSpace(req.Model)
	if req.Model == "" {
		return teamControlSetModel{}, false, nil
	}
	return req, true, nil
}

func (m *teamStateManager) clearControlRequest() error {
	path := m.controlPath()
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func loadExistingTeamManifest(cfg config.Config, teamID string) (*teamManifest, error) {
	teamID = strings.TrimSpace(teamID)
	if teamID == "" {
		return nil, fmt.Errorf("teamID is required")
	}
	path := filepath.Join(fsutil.GetTeamDir(cfg.DataDir, teamID), "team.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var manifest teamManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return nil, err
	}
	return &manifest, nil
}

func writeTeamManifestFile(cfg config.Config, manifest teamManifest) error {
	if strings.TrimSpace(manifest.TeamID) == "" {
		return fmt.Errorf("teamID is required")
	}
	b, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	teamDir := fsutil.GetTeamDir(cfg.DataDir, manifest.TeamID)
	if err := os.MkdirAll(teamDir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(teamDir, "team.json"), b, 0o644)
}
