package team

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tinoosan/workbench-core/pkg/config"
	"github.com/tinoosan/workbench-core/pkg/fsutil"
)

const manifestFilename = "team.json"

// FileManifestStore implements ManifestStore using the filesystem (team.json under GetTeamDir).
type FileManifestStore struct {
	cfg config.Config
}

// NewFileManifestStore returns a ManifestStore that reads/writes team.json under cfg.DataDir/teams/<teamID>/.
func NewFileManifestStore(cfg config.Config) ManifestStore {
	return &FileManifestStore{cfg: cfg}
}

// Load reads the manifest for the given team ID. Returns (nil, nil) if the file does not exist.
func (f *FileManifestStore) Load(ctx context.Context, teamID string) (*Manifest, error) {
	teamID = strings.TrimSpace(teamID)
	if teamID == "" {
		return nil, fmt.Errorf("teamID is required")
	}
	path := filepath.Join(fsutil.GetTeamDir(f.cfg.DataDir, teamID), manifestFilename)
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var manifest Manifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return nil, err
	}
	return &manifest, nil
}

// Save writes the manifest to disk (team.json under the team's directory).
func (f *FileManifestStore) Save(ctx context.Context, manifest Manifest) error {
	if strings.TrimSpace(manifest.TeamID) == "" {
		return fmt.Errorf("teamID is required")
	}
	b, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	teamDir := fsutil.GetTeamDir(f.cfg.DataDir, manifest.TeamID)
	if err := os.MkdirAll(teamDir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(teamDir, manifestFilename), b, 0o644)
}
