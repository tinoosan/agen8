package rpc

import (
	"context"
	"fmt"
	"strings"

	harnessapp "github.com/tinoosan/agen8-mcp-server/internal/services/harness/app"
	"github.com/tinoosan/agen8-mcp-server/internal/services/harness/domain"
)

type Handler struct {
	service *harnessapp.Service
}

func NewHandler(service *harnessapp.Service) (*Handler, error) {
	if service == nil {
		return nil, fmt.Errorf("harness service is required")
	}
	return &Handler{service: service}, nil
}

func MustNewHandler(service *harnessapp.Service) *Handler {
	handler, err := NewHandler(service)
	if err != nil {
		panic(err)
	}
	return handler
}

func (h *Handler) ConfigOptions(ctx context.Context, _ ConfigOptionsParams) (ConfigOptionsResult, error) {
	entries := h.service.CatalogEntries()
	options := make([]HarnessOption, 0, len(entries))
	for _, entry := range entries {
		models := make([]ModelOption, 0, len(entry.Models))
		for _, model := range entry.Models {
			models = append(models, ModelOption{
				ID:      model.ID,
				Name:    model.ID,
				Aliases: append([]string(nil), model.Aliases...),
				Efforts: append([]string(nil), model.Efforts...),
			})
		}
		permissionModes := make([]PermissionModeOption, 0, len(entry.PermissionModes))
		for _, mode := range entry.PermissionModes {
			permissionModes = append(permissionModes, PermissionModeOption{
				ID:                mode.ID,
				Name:              mode.Name,
				Description:       mode.Description,
				Default:           mode.Default,
				RequiresConfigRef: mode.RequiresConfigRef,
			})
		}
		options = append(options, HarnessOption{
			Kind:            entry.Kind,
			Models:          models,
			PermissionModes: permissionModes,
		})
	}
	return ConfigOptionsResult{Harnesses: options}, nil
}

func (h *Handler) SessionGet(ctx context.Context, p SessionGetParams) (SessionGetResult, error) {
	sessionID := strings.TrimSpace(p.SessionID)
	if sessionID == "" {
		return SessionGetResult{}, invalidParams("sessionId is required")
	}
	session, err := h.service.GetSession(ctx, sessionID)
	if err != nil {
		return SessionGetResult{}, internalError("get harness session", err)
	}
	if session == nil {
		return SessionGetResult{}, notFound("harness session not found")
	}
	return SessionGetResult{Session: NewSessionView(session)}, nil
}

func (h *Handler) SessionList(ctx context.Context, p SessionListParams) (SessionListResult, error) {
	spaceID := strings.TrimSpace(p.SpaceID)
	memberID := strings.TrimSpace(p.MemberID)
	if spaceID == "" && memberID == "" {
		return SessionListResult{}, invalidParams("spaceId or memberId is required")
	}
	if spaceID != "" && memberID != "" {
		return SessionListResult{}, invalidParams("provide only one of spaceId or memberId")
	}

	var sessions []SessionView
	if spaceID != "" {
		rows, err := h.service.ListSessionsBySpace(ctx, spaceID)
		if err != nil {
			return SessionListResult{}, internalError("list harness sessions by space", err)
		}
		sessions = sessionViews(rows, p.Limit)
	} else {
		rows, err := h.service.ListSessionsByMember(ctx, memberID)
		if err != nil {
			return SessionListResult{}, internalError("list harness sessions by member", err)
		}
		sessions = sessionViews(rows, p.Limit)
	}
	return SessionListResult{Sessions: sessions}, nil
}

func (h *Handler) RunList(ctx context.Context, p RunListParams) (RunListResult, error) {
	rows, err := h.service.ListRuns(ctx, harnessapp.RunListParams{
		ProjectID: p.ProjectID,
		SpaceID:   p.SpaceID,
		ChannelID: p.ChannelID,
		MemberID:  p.MemberID,
		SessionID: p.SessionID,
		Status:    append([]string(nil), p.Status...),
		Limit:     p.Limit,
	})
	if err != nil {
		if strings.Contains(err.Error(), "invalid run status") {
			return RunListResult{}, invalidParams(err.Error())
		}
		return RunListResult{}, internalError("list harness runs", err)
	}
	return RunListResult{Runs: runViews(rows)}, nil
}

func (h *Handler) TurnCancel(ctx context.Context, requestedBy string, p TurnCancelParams) (TurnCancelResult, error) {
	if strings.TrimSpace(p.RunID) == "" && strings.TrimSpace(p.TurnID) == "" {
		return TurnCancelResult{}, invalidParams("runId or turnId is required")
	}
	run, err := h.service.RequestStop(ctx, harnessapp.RequestStopParams{
		RunID:       p.RunID,
		TurnID:      p.TurnID,
		ChannelID:   p.ChannelID,
		RequestedBy: requestedBy,
	})
	if err != nil {
		message := err.Error()
		switch {
		case strings.Contains(message, "not found"):
			return TurnCancelResult{}, notFound("harness run not found")
		case strings.Contains(message, "no cancel handle"), strings.Contains(message, "cannot request stop"):
			return TurnCancelResult{}, rpcError{code: -32005, message: message}
		case strings.Contains(message, "belongs to channel"):
			return TurnCancelResult{}, invalidParams(message)
		default:
			return TurnCancelResult{}, internalError("cancel harness turn", err)
		}
	}
	return TurnCancelResult{Run: NewRunView(run)}, nil
}

func sessionViews(rows []*domain.Session, limit int) []SessionView {
	if limit < 0 {
		limit = 0
	}
	if limit > 0 && len(rows) > limit {
		rows = rows[:limit]
	}
	views := make([]SessionView, 0, len(rows))
	for _, session := range rows {
		views = append(views, NewSessionView(session))
	}
	return views
}

func invalidParams(message string) error {
	return rpcError{code: -32602, message: strings.TrimSpace(message)}
}

func notFound(message string) error {
	return rpcError{code: -32004, message: strings.TrimSpace(message)}
}

func internalError(action string, err error) error {
	return fmt.Errorf("%s: %w", action, err)
}

type rpcError struct {
	code    int
	message string
}

func (e rpcError) Error() string {
	return e.message
}

func (e rpcError) RPCCode() int {
	return e.code
}
