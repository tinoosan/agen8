package domain

import (
	"fmt"
	"strings"
	"time"
)

type ID string

type Kind string

const (
	KindLocal   Kind = "local"
	KindSSH     Kind = "ssh"
	KindManaged Kind = "managed"
)

type Status string

const (
	StatusOnline   Status = "online"
	StatusOffline  Status = "offline"
	StatusNotReady Status = "not_ready"
)

type AuthMode string

const (
	AuthModeSSHAgent AuthMode = "sshAgent"
	AuthModeKeyRef   AuthMode = "keyRef"
)

type ProbeStatus string

const (
	ProbeStatusPassed  ProbeStatus = "passed"
	ProbeStatusFailed  ProbeStatus = "failed"
	ProbeStatusUnknown ProbeStatus = "unknown"
)

type FailureCode string

const (
	FailureCodeAuthFailed       FailureCode = "auth_failed"
	FailureCodeUnreachable      FailureCode = "unreachable"
	FailureCodePermissionDenied FailureCode = "permission_denied"
	FailureCodeRootMissing      FailureCode = "root_missing"
	FailureCodeCodexMissing     FailureCode = "codex_missing"
	FailureCodeExecutionMissing FailureCode = "execution_missing"
)

type DirEntryType string

const (
	DirEntryDirectory DirEntryType = "directory"
	DirEntryFile      DirEntryType = "file"
	DirEntrySymlink   DirEntryType = "symlink"
)

type Address struct {
	Host     string
	Port     int
	Username string
}

type Probe struct {
	Reachable    bool
	FileBrowsing bool
	Exec         bool
	Codex        bool
	Claude       bool
}

type Location struct {
	id             ID
	kind           Kind
	label          string
	address        Address
	status         Status
	ready          bool
	credentialRef  string
	probe          Probe
	lastProbeError string
	lastProbedAt   *time.Time
	createdAt      time.Time
	updatedAt      time.Time
}

type NewInput struct {
	ID             ID
	Kind           Kind
	Label          string
	Address        Address
	Status         Status
	Ready          bool
	CredentialRef  string
	Probe          Probe
	LastProbeError string
	LastProbedAt   *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func New(input NewInput) (Location, error) {
	id := ID(strings.TrimSpace(string(input.ID)))
	if id == "" {
		return Location{}, fmt.Errorf("location id is required")
	}
	kind := input.Kind
	if !validKind(kind) {
		return Location{}, fmt.Errorf("unsupported location kind %q", kind)
	}
	if kind == KindSSH {
		if strings.TrimSpace(input.Address.Host) == "" {
			return Location{}, fmt.Errorf("ssh location host is required")
		}
		if input.Address.Port <= 0 {
			return Location{}, fmt.Errorf("ssh location port is required")
		}
		if strings.TrimSpace(input.Address.Username) == "" {
			return Location{}, fmt.Errorf("ssh location username is required")
		}
	}
	status := input.Status
	if !validStatus(status) {
		return Location{}, fmt.Errorf("unsupported location status %q", status)
	}
	createdAt := input.CreatedAt
	if createdAt.IsZero() {
		return Location{}, fmt.Errorf("location created at is required")
	}
	updatedAt := input.UpdatedAt
	if updatedAt.IsZero() {
		return Location{}, fmt.Errorf("location updated at is required")
	}
	var lastProbedAt *time.Time
	if input.LastProbedAt != nil {
		clone := input.LastProbedAt.UTC()
		lastProbedAt = &clone
	}
	return Location{
		id:             id,
		kind:           kind,
		label:          strings.TrimSpace(input.Label),
		address:        cleanAddress(input.Address),
		status:         status,
		ready:          input.Ready,
		credentialRef:  strings.TrimSpace(input.CredentialRef),
		probe:          input.Probe,
		lastProbeError: strings.TrimSpace(input.LastProbeError),
		lastProbedAt:   lastProbedAt,
		createdAt:      createdAt.UTC(),
		updatedAt:      updatedAt.UTC(),
	}, nil
}

func Wrap(record Record) (Location, error) {
	return New(NewInput(record))
}

func (l Location) ID() ID               { return l.id }
func (l Location) Kind() Kind           { return l.kind }
func (l Location) Label() string        { return l.label }
func (l Location) Address() Address     { return l.address }
func (l Location) Status() Status       { return l.status }
func (l Location) Ready() bool          { return l.ready }
func (l Location) CreatedAt() time.Time { return l.createdAt }
func (l Location) UpdatedAt() time.Time { return l.updatedAt }

func (l Location) Record() Record {
	return Record{
		ID:             l.id,
		Kind:           l.kind,
		Label:          l.label,
		Address:        l.address,
		Status:         l.status,
		Ready:          l.ready,
		CredentialRef:  l.credentialRef,
		Probe:          l.probe,
		LastProbeError: l.lastProbeError,
		LastProbedAt:   l.lastProbedAt,
		CreatedAt:      l.createdAt,
		UpdatedAt:      l.updatedAt,
	}
}

func validKind(kind Kind) bool {
	return kind == KindLocal || kind == KindSSH || kind == KindManaged
}

func validStatus(status Status) bool {
	return status == StatusOnline || status == StatusOffline || status == StatusNotReady
}

func cleanAddress(address Address) Address {
	return Address{
		Host:     strings.TrimSpace(address.Host),
		Port:     address.Port,
		Username: strings.TrimSpace(address.Username),
	}
}
