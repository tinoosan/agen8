package domain

import "context"

// Reader exposes the read side of the pin store.
type Reader interface {
	// List returns every pin in a project, newest first.
	List(ctx context.Context, projectID string) ([]Pin, error)
	// Exists reports whether a specific node is pinned in a project.
	Exists(ctx context.Context, projectID, nodeRef string) (bool, error)
}

// Writer exposes the write side of the pin store.
type Writer interface {
	// Save upserts a pin. Re-pinning an existing (projectID, nodeRef) is a
	// no-op on identity and keeps the original CreatedAt.
	Save(ctx context.Context, pin Pin) error
	// Delete removes a pin. Removing a non-existent pin returns ErrNotFound.
	Delete(ctx context.Context, projectID, nodeRef string) error
}

// Repository is the full persistence contract for pins.
type Repository interface {
	Reader
	Writer
}
