package app

import (
	"context"
	"strings"
	"time"

	"github.com/tinoosan/agen8/pkg/config"
	"github.com/tinoosan/agen8/pkg/services/team"
)

// loadExistingTeamManifest loads the team manifest from disk via the team package.
// Returns (nil, nil) if the manifest file does not exist.
func loadExistingTeamManifest(cfg config.Config, teamID string) (*team.Manifest, error) {
	return team.NewFileManifestStore(cfg).Load(context.Background(), teamID)
}

// writeTeamManifestFile writes the team manifest to disk via the team package.
func writeTeamManifestFile(cfg config.Config, manifest team.Manifest) error {
	return team.NewFileManifestStore(cfg).Save(context.Background(), manifest)
}

// persistTeamManifestModel updates an existing team manifest's model and modelChange, then saves.
// Used by standalone daemon RPC when updating session model for a team. No-op if teamID/model empty or manifest missing.
func persistTeamManifestModel(cfg config.Config, teamID, model, reason string) error {
	teamID = strings.TrimSpace(teamID)
	model = strings.TrimSpace(model)
	if teamID == "" || model == "" {
		return nil
	}
	store := team.NewFileManifestStore(cfg)
	ctx := context.Background()
	manifest, err := store.Load(ctx, teamID)
	if err != nil || manifest == nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	manifest.TeamModel = model
	manifest.ModelChange = &team.ModelChange{
		RequestedModel: model,
		Status:         "applied",
		RequestedAt:    now,
		AppliedAt:      now,
		Reason:         strings.TrimSpace(reason),
	}
	return store.Save(ctx, *manifest)
}
