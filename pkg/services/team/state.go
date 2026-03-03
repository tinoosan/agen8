package team

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

// ManifestStore loads and saves team manifests. The default implementation is file-based (see file_store.go).
type ManifestStore interface {
	Load(ctx context.Context, teamID string) (*Manifest, error)
	Save(ctx context.Context, manifest Manifest) error
}

// StateManager holds the in-memory manifest and persists changes via ManifestStore.
// Same behavior as the previous teamStateManager in internal/app/team_state.go.
type StateManager struct {
	store      ManifestStore
	teamID     string
	manifestMu sync.Mutex
	manifest   Manifest
}

// NewStateManager creates a state manager with the given store and initial manifest.
// The initial manifest is kept in memory and should already be saved by the caller if persistence is desired.
func NewStateManager(store ManifestStore, initial Manifest) *StateManager {
	teamID := strings.TrimSpace(initial.TeamID)
	return &StateManager{
		store:    store,
		teamID:   teamID,
		manifest: initial,
	}
}

// ManifestSnapshot returns a copy of the current manifest (caller must not mutate).
func (m *StateManager) ManifestSnapshot() Manifest {
	m.manifestMu.Lock()
	defer m.manifestMu.Unlock()
	return m.manifest
}

func (m *StateManager) updateManifest(ctx context.Context, mutator func(*Manifest)) error {
	m.manifestMu.Lock()
	mutator(&m.manifest)
	manifest := m.manifest
	m.manifestMu.Unlock()
	if ctx == nil {
		ctx = context.Background()
	}
	return m.store.Save(ctx, manifest)
}

// QueueModelChange records a pending model change to be applied when the team is idle.
func (m *StateManager) QueueModelChange(ctx context.Context, model, reason string) error {
	model = strings.TrimSpace(model)
	if model == "" {
		return fmt.Errorf("model is required")
	}
	return m.updateManifest(ctx, func(manifest *Manifest) {
		now := time.Now().UTC().Format(time.RFC3339Nano)
		manifest.ModelChange = &ModelChange{
			RequestedModel: model,
			Status:         "pending",
			RequestedAt:    now,
			Reason:         strings.TrimSpace(reason),
		}
	})
}

// MarkModelApplied updates the manifest after a model change was applied successfully.
func (m *StateManager) MarkModelApplied(ctx context.Context, model string) error {
	model = strings.TrimSpace(model)
	if model == "" {
		return fmt.Errorf("model is required")
	}
	return m.updateManifest(ctx, func(manifest *Manifest) {
		now := time.Now().UTC().Format(time.RFC3339Nano)
		manifest.TeamModel = model
		manifest.ModelChange = &ModelChange{
			RequestedModel: model,
			Status:         "applied",
			RequestedAt:    now,
			AppliedAt:      now,
		}
	})
}

// MarkModelFailed updates the manifest after a model change failed.
func (m *StateManager) MarkModelFailed(ctx context.Context, model string, err error) error {
	errMsg := "unknown error"
	if err != nil {
		errMsg = err.Error()
	}
	return m.updateManifest(ctx, func(manifest *Manifest) {
		now := time.Now().UTC().Format(time.RFC3339Nano)
		manifest.ModelChange = &ModelChange{
			RequestedModel: strings.TrimSpace(model),
			Status:         "failed",
			RequestedAt:    now,
			AppliedAt:      now,
			Error:          errMsg,
		}
	})
}
