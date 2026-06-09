// Package app provides the pin application service: a thin orchestration
// layer over the pin repository.
//
// Pins are per-project and shared. The service owns CreatedAt stamping (via
// the clock) and input trimming/validation, leaving persistence to the
// repository. Pin changes publish lightweight project-scoped events so the UI
// can refresh live without promoting pins into first-class graph entities.
package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/tinoosan/agen8/internal/eventbus"
	pindomain "github.com/tinoosan/agen8/internal/services/pin/domain"
)

// Clock supplies the current time. Injectable so tests can pin CreatedAt.
type Clock interface {
	Now() time.Time
}

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now().UTC() }

// EventPublisher publishes pin lifecycle events after persistence succeeds.
type EventPublisher interface {
	Publish(topic string, event any) error
}

// Service orchestrates pin reads and writes.
type Service struct {
	repo   pindomain.Repository
	clock  Clock
	events EventPublisher
}

// Config wires the service dependencies.
type Config struct {
	Pins   pindomain.Repository
	Clock  Clock
	Events EventPublisher
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
	return &Service{repo: cfg.Pins, clock: clock, events: cfg.Events}, nil
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
	if err := s.publish(ctx, eventbus.PinEventAdded, pin.ProjectID, pin.NodeRef, pin.NodeType); err != nil {
		return pindomain.Pin{}, err
	}
	return pin, nil
}

// Unpin removes a pin. Returns domain.ErrNotFound when the node was not pinned.
func (s *Service) Unpin(ctx context.Context, projectID, nodeRef string) error {
	projectID = strings.TrimSpace(projectID)
	nodeRef = strings.TrimSpace(nodeRef)
	if err := s.repo.Delete(ctx, projectID, nodeRef); err != nil {
		return err
	}
	return s.publish(ctx, eventbus.PinEventRemoved, projectID, nodeRef, "")
}

// List returns every pin in a project, newest first.
func (s *Service) List(ctx context.Context, projectID string) ([]pindomain.Pin, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return nil, fmt.Errorf("pin: projectId is required")
	}
	return s.repo.List(ctx, projectID)
}

func (s *Service) publish(_ context.Context, eventType, projectID, nodeRef, nodeType string) error {
	if s.events == nil {
		return nil
	}
	event := eventbus.PinLifecycleEvent{
		ProjectID: projectID,
		NodeRef:   nodeRef,
		NodeType:  nodeType,
		EventType: eventType,
		Timestamp: s.clock.Now().UTC(),
	}
	if err := s.events.Publish(eventbus.TopicPinLifecycle, event); err != nil {
		return fmt.Errorf("publish pin lifecycle event: %w", err)
	}
	return nil
}
