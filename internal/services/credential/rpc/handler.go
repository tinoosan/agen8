package rpc

import (
	"context"
	"fmt"
	"strings"

	credentialapp "github.com/tinoosan/agen8/internal/services/credential/app"
	credentialdomain "github.com/tinoosan/agen8/internal/services/credential/domain"
)

type Handler struct {
	service *credentialapp.Service
}

func NewHandler(service *credentialapp.Service) (*Handler, error) {
	if service == nil {
		return nil, fmt.Errorf("credential service is required")
	}
	return &Handler{service: service}, nil
}

func MustNewHandler(service *credentialapp.Service) *Handler {
	handler, err := NewHandler(service)
	if err != nil {
		panic(err)
	}
	return handler
}

func (h *Handler) CredentialList(ctx context.Context, p CredentialListParams) (CredentialListResult, error) {
	credentials, err := h.service.ListCredentials(ctx, credentialdomain.Filter{
		Kind:   credentialdomain.Kind(strings.TrimSpace(p.Kind)),
		Status: credentialdomain.Status(strings.TrimSpace(p.Status)),
	})
	if err != nil {
		return CredentialListResult{}, internalError("list credentials", err)
	}
	views := make([]CredentialView, 0, len(credentials))
	for _, credential := range credentials {
		views = append(views, NewCredentialView(credential))
	}
	return CredentialListResult{Credentials: views}, nil
}

func (h *Handler) CredentialGet(ctx context.Context, p CredentialGetParams) (CredentialResult, error) {
	id, err := requireCredentialID(p.CredentialID)
	if err != nil {
		return CredentialResult{}, err
	}
	credential, err := h.service.GetCredential(ctx, id)
	if err != nil {
		return CredentialResult{}, internalError("get credential", err)
	}
	return CredentialResult{Credential: NewCredentialView(credential)}, nil
}

func (h *Handler) CredentialCreate(ctx context.Context, p CredentialCreateParams) (CredentialResult, error) {
	kind := credentialdomain.Kind(strings.TrimSpace(p.Kind))
	if kind == "" {
		return CredentialResult{}, invalidParams("kind is required")
	}
	if kind != credentialdomain.KindSSHAgent && len(p.Secrets) == 0 {
		return CredentialResult{}, invalidParams("secrets are required")
	}
	credential, err := h.service.CreateCredential(ctx, credentialapp.CreateCredentialInput{
		Kind:        kind,
		Label:       strings.TrimSpace(p.Label),
		StorageKind: credentialdomain.StorageKind(strings.TrimSpace(p.StorageKind)),
		Secrets:     p.Secrets,
	})
	if err != nil {
		return CredentialResult{}, internalError("create credential", err)
	}
	return CredentialResult{Credential: NewCredentialView(credential)}, nil
}

func (h *Handler) CredentialUpdate(ctx context.Context, p CredentialUpdateParams) (CredentialResult, error) {
	id, err := requireCredentialID(p.CredentialID)
	if err != nil {
		return CredentialResult{}, err
	}
	credential, err := h.service.UpdateCredential(ctx, credentialapp.UpdateCredentialInput{
		ID:          id,
		Label:       strings.TrimSpace(p.Label),
		Status:      credentialdomain.Status(strings.TrimSpace(p.Status)),
		StorageKind: credentialdomain.StorageKind(strings.TrimSpace(p.StorageKind)),
		Secrets:     p.Secrets,
	})
	if err != nil {
		return CredentialResult{}, internalError("update credential", err)
	}
	return CredentialResult{Credential: NewCredentialView(credential)}, nil
}

func (h *Handler) CredentialDelete(ctx context.Context, p CredentialDeleteParams) (struct{}, error) {
	id, err := requireCredentialID(p.CredentialID)
	if err != nil {
		return struct{}{}, err
	}
	if err := h.service.DeleteCredential(ctx, id); err != nil {
		return struct{}{}, internalError("delete credential", err)
	}
	return struct{}{}, nil
}

func requireCredentialID(value string) (credentialdomain.ID, error) {
	id := credentialdomain.ID(strings.TrimSpace(value))
	if id == "" {
		return "", invalidParams("credentialId is required")
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
