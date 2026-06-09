package rpc

import (
	"context"
	"fmt"
	"strings"

	userapp "github.com/tinoosan/agen8/internal/services/user/app"
	user "github.com/tinoosan/agen8/internal/services/user/domain"
)

type Identity struct {
	UserID string
	Role   string
}

type IdentityProvider func(context.Context) (Identity, error)

type Handler struct {
	svc      *userapp.Service
	identity IdentityProvider
}

func NewHandler(svc *userapp.Service, identity IdentityProvider) *Handler {
	if svc == nil {
		panic("user RPC handler requires user service")
	}
	if identity == nil {
		panic("user RPC handler requires identity provider")
	}
	return &Handler{svc: svc, identity: identity}
}

func (h *Handler) Status(ctx context.Context, _ StatusParams) (StatusResult, error) {
	setupOpen, err := h.svc.SetupOpen(ctx)
	if err != nil {
		return StatusResult{}, internalError("user status", err)
	}
	identity, err := h.identity(ctx)
	if err != nil {
		return StatusResult{SetupOpen: setupOpen}, nil
	}
	userID, err := parseUserID(identity.UserID, "identity user id")
	if err != nil {
		return StatusResult{}, err
	}
	record, err := h.svc.Get(ctx, userID)
	if err != nil {
		return StatusResult{}, internalError("get current user", err)
	}
	view := NewUserView(record)
	return StatusResult{SetupOpen: setupOpen, User: &view}, nil
}

func (h *Handler) Get(ctx context.Context, p GetParams) (GetResult, error) {
	userID := strings.TrimSpace(p.UserID)
	if userID == "" {
		identity, err := h.identity(ctx)
		if err != nil {
			return GetResult{}, err
		}
		userID = identity.UserID
	}
	id, err := parseUserID(userID, "userId")
	if err != nil {
		return GetResult{}, err
	}
	record, err := h.svc.Get(ctx, id)
	if err != nil {
		return GetResult{}, internalError("get user", err)
	}
	return GetResult{User: NewUserView(record)}, nil
}

func (h *Handler) UpdateProfile(ctx context.Context, p UpdateProfileParams) (UpdateProfileResult, error) {
	identity, err := h.identity(ctx)
	if err != nil {
		return UpdateProfileResult{}, err
	}
	id, err := parseUserID(identity.UserID, "identity user id")
	if err != nil {
		return UpdateProfileResult{}, err
	}
	var preferences *user.Preferences
	if p.Preferences != nil {
		value := p.Preferences.domain()
		preferences = &value
	}
	record, err := h.svc.UpdateProfile(ctx, userapp.UpdateProfileParams{
		UserID:      id,
		Email:       p.Email,
		Name:        p.Name,
		Preferences: preferences,
	})
	if err != nil {
		return UpdateProfileResult{}, internalError("update user profile", err)
	}
	return UpdateProfileResult{User: NewUserView(record)}, nil
}

func (h *Handler) Suspend(ctx context.Context, p SuspendParams) (SuspendResult, error) {
	identity, err := h.identity(ctx)
	if err != nil {
		return SuspendResult{}, err
	}
	if strings.TrimSpace(identity.Role) != string(user.RoleAdmin) {
		return SuspendResult{}, invalidParams("admin role is required")
	}
	id, err := parseUserID(p.UserID, "userId")
	if err != nil {
		return SuspendResult{}, err
	}
	record, err := h.svc.SuspendUser(ctx, id)
	if err != nil {
		return SuspendResult{}, internalError("suspend user", err)
	}
	return SuspendResult{User: NewUserView(record)}, nil
}

func (h *Handler) Close(ctx context.Context, p CloseParams) (CloseResult, error) {
	identity, err := h.identity(ctx)
	if err != nil {
		return CloseResult{}, err
	}
	target := strings.TrimSpace(p.UserID)
	if target == "" {
		target = identity.UserID
	}
	if target != strings.TrimSpace(identity.UserID) && strings.TrimSpace(identity.Role) != string(user.RoleAdmin) {
		return CloseResult{}, invalidParams("admin role is required")
	}
	id, err := parseUserID(target, "userId")
	if err != nil {
		return CloseResult{}, err
	}
	record, err := h.svc.CloseUser(ctx, id)
	if err != nil {
		return CloseResult{}, internalError("close user", err)
	}
	return CloseResult{User: NewUserView(record)}, nil
}

func parseUserID(raw string, field string) (user.ID, error) {
	id, err := user.NewID(raw)
	if err != nil {
		return user.ID{}, invalidParams(field + " is required")
	}
	return id, nil
}

func invalidParams(message string) error {
	return rpcError{code: -32602, message: strings.TrimSpace(message)}
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
