package domain

import (
	"fmt"
	"strings"
	"time"
)

type ID string

type Kind string

const (
	KindSSHAgent    Kind = "ssh_agent"
	KindSSHKey      Kind = "ssh_key"
	KindSSHPassword Kind = "ssh_password"
	KindAPIKey      Kind = "api_key"
)

type Status string

const (
	StatusActive   Status = "active"
	StatusDisabled Status = "disabled"
	StatusInvalid  Status = "invalid"
)

type StorageKind string

const (
	StorageLocalEncrypted StorageKind = "local_encrypted"
	StorageSSHAgent       StorageKind = "ssh_agent"
)

type FieldKind string

const (
	FieldPublic FieldKind = "public"
	FieldSecret FieldKind = "secret"
)

type Purpose string

const (
	PurposeLocationSSH Purpose = "location_ssh"
	PurposeHTTPTool    Purpose = "http_tool"
)

type InjectionMode string

const (
	InjectionBearer InjectionMode = "bearer"
	InjectionHeader InjectionMode = "header"
	InjectionQuery  InjectionMode = "query"
)

type FieldRef struct {
	Name string
	Kind FieldKind
}

type Credential struct {
	id        ID
	kind      Kind
	label     string
	status    Status
	fields    []FieldRef
	createdAt time.Time
	updatedAt time.Time
}

type NewInput struct {
	ID        ID
	Kind      Kind
	Label     string
	Status    Status
	Fields    []FieldRef
	CreatedAt time.Time
	UpdatedAt time.Time
}

func New(input NewInput) (Credential, error) {
	id := ID(strings.TrimSpace(string(input.ID)))
	if id == "" {
		return Credential{}, fmt.Errorf("credential id is required")
	}
	if !validKind(input.Kind) {
		return Credential{}, fmt.Errorf("unsupported credential kind %q", input.Kind)
	}
	status := input.Status
	if status == "" {
		status = StatusActive
	}
	if !validStatus(status) {
		return Credential{}, fmt.Errorf("unsupported credential status %q", status)
	}
	createdAt := input.CreatedAt
	if createdAt.IsZero() {
		return Credential{}, fmt.Errorf("credential created at is required")
	}
	updatedAt := input.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = createdAt
	}
	fields, err := cleanFields(input.Fields)
	if err != nil {
		return Credential{}, err
	}
	return Credential{
		id:        id,
		kind:      input.Kind,
		label:     strings.TrimSpace(input.Label),
		status:    status,
		fields:    fields,
		createdAt: createdAt.UTC(),
		updatedAt: updatedAt.UTC(),
	}, nil
}

func Wrap(record Record) (Credential, error) {
	return New(NewInput(record))
}

func (c Credential) ID() ID               { return c.id }
func (c Credential) Kind() Kind           { return c.kind }
func (c Credential) Label() string        { return c.label }
func (c Credential) Status() Status       { return c.status }
func (c Credential) Fields() []FieldRef   { return append([]FieldRef(nil), c.fields...) }
func (c Credential) CreatedAt() time.Time { return c.createdAt }
func (c Credential) UpdatedAt() time.Time { return c.updatedAt }

func (c Credential) Record() Record {
	return Record{
		ID:        c.id,
		Kind:      c.kind,
		Label:     c.label,
		Status:    c.status,
		Fields:    c.Fields(),
		CreatedAt: c.createdAt,
		UpdatedAt: c.updatedAt,
	}
}

func validKind(kind Kind) bool {
	return kind == KindSSHAgent || kind == KindSSHKey || kind == KindSSHPassword || kind == KindAPIKey
}

func validStatus(status Status) bool {
	return status == StatusActive || status == StatusDisabled || status == StatusInvalid
}

func cleanFields(fields []FieldRef) ([]FieldRef, error) {
	out := make([]FieldRef, 0, len(fields))
	seen := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		name := strings.TrimSpace(field.Name)
		if name == "" {
			return nil, fmt.Errorf("credential field name is required")
		}
		if field.Kind != FieldPublic && field.Kind != FieldSecret {
			return nil, fmt.Errorf("unsupported credential field kind %q", field.Kind)
		}
		if _, ok := seen[name]; ok {
			return nil, fmt.Errorf("credential field %q is duplicated", name)
		}
		seen[name] = struct{}{}
		out = append(out, FieldRef{Name: name, Kind: field.Kind})
	}
	return out, nil
}
