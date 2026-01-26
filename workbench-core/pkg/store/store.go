package store

import "context"

// ConstructorStateStore persists per-run constructor state and manifests.
// The content is stored as JSON blobs so the agent package owns the schema.
type ConstructorStateStore interface {
	GetState(ctx context.Context, runID string) ([]byte, error)
	SetState(ctx context.Context, runID string, stateJSON []byte) error
	GetManifest(ctx context.Context, runID string) ([]byte, error)
	SetManifest(ctx context.Context, runID string, manifestJSON []byte) error
}
