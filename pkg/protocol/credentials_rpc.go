package protocol

import "time"

const (
	MethodCredentialList   = "credential.list"
	MethodCredentialGet    = "credential.get"
	MethodCredentialCreate = "credential.create"
	MethodCredentialUpdate = "credential.update"
	MethodCredentialDelete = "credential.delete"
)

type CredentialView struct {
	ID        string                `json:"id"`
	Kind      string                `json:"kind"`
	Label     string                `json:"label"`
	Status    string                `json:"status"`
	Fields    []CredentialFieldView `json:"fields,omitempty"`
	CreatedAt *time.Time            `json:"createdAt,omitempty"`
	UpdatedAt *time.Time            `json:"updatedAt,omitempty"`
}

type CredentialFieldView struct {
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
