// Package app provides the pin application service: a thin orchestration
// layer over the pin repository.
//
// Pins are per-project and shared. The service owns CreatedAt stamping (via
// the clock) and input trimming/validation, leaving persistence to the
// repository. There are deliberately no side effects (no events, no graph
// edges): a pin is a lightweight UI affordance, not a first-class work entity.
package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	pindomain "github.com/tinoosan/agen8-mcp-server/internal/services/pin/domain"
)

// Clock supplies the current time. Injectable so tests can pin CreatedAt.
type Clock interface {
	Now() time.Time
}

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now().UTC() }

// Service orchestrates pin reads and writes.
type Service struct {
	repo  pindomain.Repository
	clock Clock
}

// Config wires the service dependencies.
type Config struct {
	Pins  pindomain.Repository
	Clock Clock
}

// NewService builds a pin service. The repository is required; the clock
// defaults to the system clock.
func NewService(cfg Config) (*Service, error) {
	if cfg.Pins == nil {
		return nil, fmt.Errorf("pin service: repository is required")
	}
	clock := cfg.Clock
	if clock == nil {
		clock = systemClock{}
	}
	return &Service{repo: cfg.Pins, clock: clock}, nil
}

// Pin marks a node as pinned in a project. Idempotent: re-pinning an already
// pinned node is a no-op that preserves the original pin time.
func (s *Service) Pin(ctx context.Context, projectID, nodeRef, nodeType string) (pindomain.Pin, error) {
	pin := pindomain.Pin{
		ProjectID: strings.TrimSpace(projectID),
		NodeRef:   strings.TrimSpace(nodeRef),
		NodeType:  strings.TrimSpace(nodeType),
		CreatedAt: s.clock.Now().UTC(),
	}
	if err := pin.Validate(); err != nil {
		return pindomain.Pin{}, err
	}
	if err := s.repo.Save(ctx, pin); err != nil {
		return pindomain.Pin{}, err
	}
	return pin, nil
}

// Unpin removes a pin. Returns domain.ErrNotFound when the node was not pinned.
func (s *Service) Unpin(ctx context.Context, projectID, nodeRef string) error {
	return s.repo.Delete(ctx, strings.TrimSpace(projectID), strings.TrimSpace(nodeRef))
}

// List returns every pin in a project, newest first.
func (s *Service) List(ctx context.Context, projectID string) ([]pindomain.Pin, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return nil, fmt.Errorf("pin: projectId is required")
	}
	return s.repo.List(ctx, projectID)
}
