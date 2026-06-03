package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/tinoosan/agen8-mcp-server/internal/services/humaninput/domain"
)

type WakeRegistry interface {
	Wait(ctx context.Context, id domain.RequestID) (domain.Request, error)
}

type Awaiter struct {
	service *Service
	wake    WakeRegistry
}

func NewAwaiter(service *Service, wake WakeRegistry) (*Awaiter, error) {
	if service == nil {
		return nil, fmt.Errorf("human input service is required")
	}
	if wake == nil {
		return nil, fmt.Errorf("human input wake registry is required")
	}
	return &Awaiter{service: service, wake: wake}, nil
}

func (a *Awaiter) Await(ctx context.Context, pending domain.PendingRequest) (json.RawMessage, error) {
	if a == nil {
		return nil, fmt.Errorf("human input awaiter is required")
	}
	idempotencyKey := strings.TrimSpace(pending.IdempotencyKey)
	if idempotencyKey == "" {
		idempotencyKey = strings.TrimSpace(pending.ToolCallID)
	}
	req, err := a.service.Declare(ctx, DeclareCommand{
		ToolCallID:     pending.ToolCallID,
		ToolName:       pending.ToolName,
		IdempotencyKey: idempotencyKey,
		ProjectID:      pending.ProjectID,
		SpaceID:        pending.SpaceID,
		AskerMemberID:  pending.MemberID,
		ChannelID:      pending.ChannelID,
		Declaration:    pending.Declaration,
	})
	if err != nil {
		return nil, err
	}
	terminal, err := a.wake.Wait(ctx, req.ID)
	if err != nil {
		return nil, err
	}
	switch terminal.Status {
	case domain.StatusAnswered:
		return append(json.RawMessage(nil), terminal.Result...), nil
	case domain.StatusCancelled:
		return json.RawMessage(`{"cancelled":true}`), nil
	case domain.StatusExpired:
		return nil, fmt.Errorf("human input request %s expired", terminal.ID)
	case domain.StatusAborted:
		return nil, fmt.Errorf("human input request %s aborted", terminal.ID)
	default:
		return nil, fmt.Errorf("human input request %s is not terminal", terminal.ID)
	}
}
