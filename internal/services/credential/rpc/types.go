package rpc

import (
	"time"

	credentialdomain "github.com/tinoosan/agen8-mcp-server/internal/services/credential/domain"
)

type CredentialView struct {
	ID        string      `json:"id"`
	Kind      string      `json:"kind"`
	Label     string      `json:"label"`
	Status    string      `json:"status"`
	Fields    []FieldView `json:"fields,omitempty"`
	CreatedAt *time.Time  `json:"createdAt,omitempty"`
	UpdatedAt *time.Time  `json:"updatedAt,omitempty"`
}

func NewCredentialView(credential credentialdomain.Credential) CredentialView {
	fields := credential.Fields()
	fieldViews := make([]FieldView, 0, len(fields))
	for _, field := range fields {
		fieldViews = append(fieldViews, FieldView{
			Name:       field.Name,
			Kind:       string(field.Kind),
			Configured: true,
		})
	}
	return CredentialView{
		ID:        string(credential.ID()),
		Kind:      string(credential.Kind()),
		Label:     credential.Label(),
		Status:    string(credential.Status()),
		Fields:    fieldViews,
		CreatedAt: cloneTime(credential.CreatedAt()),
		UpdatedAt: cloneTime(credential.UpdatedAt()),
	}
}

func cloneTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	clone := t.UTC()
	return &clone
}

type FieldView struct {
	Name       string `json:"name"`
	Kind       string `json:"kind"`
	Configured bool   `json:"configured"`
}

type CredentialListParams struct {
	Kind   string `json:"kind,omitempty"`
	Status string `json:"status,omitempty"`
}

type CredentialListResult struct {
	Credentials []CredentialView `json:"credentials"`
}

type CredentialGetParams struct {
	CredentialID string `json:"credentialId"`
}

type CredentialResult struct {
	Credential CredentialView `json:"credential"`
}

type CredentialCreateParams struct {
	Kind        string            `json:"kind"`
	Label       string            `json:"label"`
	StorageKind string            `json:"storageKind,omitempty"`
	Secrets     map[string]string `json:"secrets"`
}

type CredentialUpdateParams struct {
	CredentialID string            `json:"credentialId"`
	Label        string            `json:"label,omitempty"`
	Status       string            `json:"status,omitempty"`
	StorageKind  string            `json:"storageKind,omitempty"`
	Secrets      map[string]string `json:"secrets,omitempty"`
}

type CredentialDeleteParams struct {
	CredentialID string `json:"credentialId"`
}
