package rpc

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/tinoosan/agen8-mcp-server/internal/services/auth/apikey"
	authapp "github.com/tinoosan/agen8-mcp-server/internal/services/auth/app"
	auth "github.com/tinoosan/agen8-mcp-server/internal/services/auth/domain"
	user "github.com/tinoosan/agen8-mcp-server/internal/services/user/domain"
)

type IdentityProvider func(context.Context) (Identity, error)

type Handler struct {
	svc      *authapp.Service
	identity IdentityProvider
}

func NewHandler(svc *authapp.Service, identity IdentityProvider) *Handler {
	if svc == nil {
		panic("auth RPC handler requires auth service")
	}
	if identity == nil {
		panic("auth RPC handler requires identity provider")
	}
	return &Handler{svc: svc, identity: identity}
}

func (h *Handler) AuthStatus(ctx context.Context, _ AuthStatusParams) (AuthStatusResult, error) {
	identity, err := h.identity(ctx)
	if err != nil {
		return AuthStatusResult{Authenticated: false}, nil
	}
	identity.UserID = strings.TrimSpace(identity.UserID)
	identity.Role = strings.TrimSpace(identity.Role)
	if identity.UserID == "" {
		return AuthStatusResult{}, fmt.Errorf("identity user id is required")
	}
	return AuthStatusResult{
		Authenticated: true,
		UserID:        identity.UserID,
		Role:          identity.Role,
		User: &User{
			ID:   identity.UserID,
			Role: identity.Role,
		},
	}, nil
}

func (h *Handler) Login(ctx context.Context, p LoginParams) (LoginResult, error) {
	result, err := h.svc.Login(ctx, authapp.LoginParams{
		Email:    p.Email,
		Password: p.Password,
	})
	if err != nil {
		return LoginResult{}, mapAuthError(err)
	}
	return LoginResult{
		UserID:    result.User.ID.String(),
		Role:      string(result.User.Role),
		Token:     result.Token,
		ExpiresAt: result.Session.ExpiresAt,
	}, nil
}

func (h *Handler) Logout(ctx context.Context, p LogoutParams) (LogoutResult, error) {
	token := strings.TrimSpace(p.Token)
	if token == "" {
		return LogoutResult{}, rpcError{code: -32602, message: "token is required"}
	}
	if err := h.svc.RevokeSession(ctx, token); err != nil {
		return LogoutResult{}, mapAuthError(err)
	}
	return LogoutResult{Revoked: true}, nil
}

func (h *Handler) CreateAPIKey(ctx context.Context, p CreateAPIKeyParams) (CreateAPIKeyResult, error) {
	identity, err := h.identity(ctx)
	if err != nil {
		return CreateAPIKeyResult{}, mapAuthError(err)
	}
	userID, err := user.NewID(identity.UserID)
	if err != nil {
		return CreateAPIKeyResult{}, rpcError{code: -32602, message: "identity user id is required"}
	}
	result, err := h.svc.CreateAPIKey(ctx, authapp.CreateAPIKeyParams{
		UserID:    userID,
		Name:      p.Name,
		ExpiresAt: p.ExpiresAt,
	})
	if err != nil {
		return CreateAPIKeyResult{}, err
	}
	return CreateAPIKeyResult{
		ID:        result.APIKey.ID.String(),
		Name:      result.APIKey.Name,
		Prefix:    result.APIKey.Prefix,
		Token:     result.Token,
		ExpiresAt: result.APIKey.ExpiresAt,
	}, nil
}

func (h *Handler) RevokeAPIKey(ctx context.Context, p RevokeAPIKeyParams) (RevokeAPIKeyResult, error) {
	if _, err := h.identity(ctx); err != nil {
		return RevokeAPIKeyResult{}, err
	}
	id, err := apikey.NewID(p.ID)
	if err != nil {
		return RevokeAPIKeyResult{}, rpcError{code: -32602, message: "api key id is required"}
	}
	if err := h.svc.RevokeAPIKey(ctx, id); err != nil {
		return RevokeAPIKeyResult{}, mapAuthError(err)
	}
	return RevokeAPIKeyResult{Revoked: true}, nil
}

func mapAuthError(err error) error {
	switch {
	case errors.Is(err, auth.ErrInvalidCredential):
		return rpcError{code: -32602, message: "invalid credentials"}
	case errors.Is(err, auth.ErrPasswordTooShort):
		return rpcError{code: -32602, message: auth.ErrPasswordTooShort.Error()}
	case errors.Is(err, auth.ErrTokenNotFound), errors.Is(err, auth.ErrTokenExpired), errors.Is(err, auth.ErrTokenRevoked):
		return rpcError{code: -32602, message: err.Error()}
	case errors.Is(err, auth.ErrUserInactive):
		return rpcError{code: -32600, message: err.Error()}
	default:
		return err
	}
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
