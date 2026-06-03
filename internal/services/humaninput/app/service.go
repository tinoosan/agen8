package app

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tinoosan/agen8-mcp-server/internal/services/humaninput/domain"
)

type IDGenerator func() string

type Notifier interface {
	NotifyHumanInputChanged(ctx context.Context, req domain.Request) error
}

type Service struct {
	requests   domain.Repository
	clock      domain.Clock
	ids        IDGenerator
	validators domain.PrimitiveValidator
	logger     *slog.Logger
	notifier   Notifier
}

func NewService(repo domain.Repository, clock domain.Clock, ids IDGenerator, validators domain.PrimitiveValidator, logger *slog.Logger) (*Service, error) {
	if repo == nil {
		return nil, fmt.Errorf("human input repository is required")
	}
	if clock == nil {
		return nil, fmt.Errorf("human input clock is required")
	}
	if ids == nil {
		ids = func() string { return "hi_" + uuid.NewString() }
	}
	if validators == nil {
		validators = domain.DefaultValidator{}
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{requests: repo, clock: clock, ids: ids, validators: validators, logger: logger}, nil
}

func (s *Service) SetNotifier(notifier Notifier) {
	if s == nil {
		return
	}
	s.notifier = notifier
}

type DeclareCommand struct {
	ToolCallID     string
	ToolName       string
	IdempotencyKey string
	ProjectID      string
	SpaceID        string
	AskerMemberID  string
	ChannelID      string
	Declaration    domain.Declaration
	TTL            time.Duration
}

func (s *Service) Declare(ctx context.Context, cmd DeclareCommand) (domain.Request, error) {
	if s == nil {
		return domain.Request{}, fmt.Errorf("human input service is required")
	}
	if err := s.validators.ValidateDeclaration(cmd.Declaration); err != nil {
		return domain.Request{}, err
	}
	now := s.clock.Now().UTC()
	ttl := cmd.TTL
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	id := strings.TrimSpace(s.ids())
	if id == "" {
		return domain.Request{}, fmt.Errorf("human input id generator returned empty id")
	}
	req, err := domain.NewPending(domain.PendingInput{
		ID:             domain.RequestID(id),
		ToolCallID:     domain.ToolCallID(cmd.ToolCallID),
		ToolName:       cmd.ToolName,
		IdempotencyKey: cmd.IdempotencyKey,
		ProjectID:      cmd.ProjectID,
		SpaceID:        cmd.SpaceID,
		AskerMemberID:  cmd.AskerMemberID,
		ChannelID:      cmd.ChannelID,
		Declaration:    cmd.Declaration,
		CreatedAt:      now,
		ExpiresAt:      now.Add(ttl),
	})
	if err != nil {
		return domain.Request{}, err
	}
	created, err := s.requests.CreatePending(ctx, req)
	if err != nil {
		return domain.Request{}, fmt.Errorf("declare human input: %w", err)
	}
	if err := s.notifyChanged(ctx, created); err != nil {
		return domain.Request{}, err
	}
	return created, nil
}

type ResolveCommand struct {
	RequestID        domain.RequestID
	ExpectedVersion  int64
	Result           json.RawMessage
	ResolverUserID   string
	ResolverMemberID string
}

func (s *Service) Resolve(ctx context.Context, cmd ResolveCommand) (domain.Request, error) {
	req, err := s.requests.Get(ctx, cmd.RequestID)
	if err != nil {
		return domain.Request{}, err
	}
	if err := s.validators.ValidateResult(req.Declaration, cmd.Result); err != nil {
		return domain.Request{}, err
	}
	resolved, err := s.requests.Resolve(ctx, domain.ResolveMutation{
		ID:               cmd.RequestID,
		ExpectedVersion:  cmd.ExpectedVersion,
		Status:           domain.StatusAnswered,
		Result:           append(json.RawMessage(nil), cmd.Result...),
		ResolverUserID:   strings.TrimSpace(cmd.ResolverUserID),
		ResolverMemberID: strings.TrimSpace(cmd.ResolverMemberID),
		ResolvedAt:       s.clock.Now().UTC(),
	})
	if err != nil {
		return domain.Request{}, fmt.Errorf("resolve human input: %w", err)
	}
	if err := s.notifyChanged(ctx, resolved); err != nil {
		return domain.Request{}, err
	}
	return resolved, nil
}

func (s *Service) Cancel(ctx context.Context, id domain.RequestID, expectedVersion int64, resolverUserID, resolverMemberID string) (domain.Request, error) {
	cancelled, err := s.requests.Resolve(ctx, domain.ResolveMutation{
		ID:               id,
		ExpectedVersion:  expectedVersion,
		Status:           domain.StatusCancelled,
		Result:           json.RawMessage(`{"cancelled":true}`),
		ResolverUserID:   strings.TrimSpace(resolverUserID),
		ResolverMemberID: strings.TrimSpace(resolverMemberID),
		TerminalReason:   "cancelled",
		ResolvedAt:       s.clock.Now().UTC(),
	})
	if err != nil {
		return domain.Request{}, fmt.Errorf("cancel human input: %w", err)
	}
	if err := s.notifyChanged(ctx, cancelled); err != nil {
		return domain.Request{}, err
	}
	return cancelled, nil
}

func (s *Service) Get(ctx context.Context, id domain.RequestID) (domain.Request, error) {
	return s.requests.Get(ctx, id)
}

func (s *Service) ListPending(ctx context.Context, filter domain.Filter) ([]domain.Request, error) {
	return s.requests.ListPending(ctx, filter)
}

func (s *Service) ExpireDue(ctx context.Context, now time.Time, limit int) (domain.ExpireBatch, error) {
	if now.IsZero() {
		now = s.clock.Now()
	}
	batch, err := s.requests.ExpireDue(ctx, now.UTC(), limit)
	if err != nil {
		return domain.ExpireBatch{}, err
	}
	for _, req := range batch.Requests {
		if err := s.notifyChanged(ctx, req); err != nil {
			return batch, err
		}
	}
	return batch, nil
}

func (s *Service) notifyChanged(ctx context.Context, req domain.Request) error {
	if s == nil || s.notifier == nil {
		return nil
	}
	if err := s.notifier.NotifyHumanInputChanged(ctx, req); err != nil {
		s.logger.Error("notify human input changed failed", "error", err, "requestId", req.ID, "channelId", req.ChannelID, "status", req.Status)
		return fmt.Errorf("notify human input changed: %w", err)
	}
	return nil
}
