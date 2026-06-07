package rpc

import "time"

import locationdomain "github.com/tinoosan/agen8-mcp-server/internal/services/location/domain"

const (
	LocationKindLocal   = "local"
	LocationKindSSH     = "ssh"
	LocationKindManaged = "managed"

	LocationStatusOnline   = "online"
	LocationStatusOffline  = "offline"
	LocationStatusNotReady = "not_ready"

	AuthModeSSHAgent = "sshAgent"
	AuthModeKeyRef   = "keyRef"

	ProbeStatusPassed  = "passed"
	ProbeStatusFailed  = "failed"
	ProbeStatusUnknown = "unknown"

	FailureAuthFailed       = "auth_failed"
	FailureUnreachable      = "unreachable"
	FailurePermissionDenied = "permission_denied"
	FailureRootMissing      = "root_missing"
	FailureCodexMissing     = "codex_missing"
	FailureExecutionMissing = "execution_missing"

	DirEntryDirectory = "directory"
	DirEntryFile      = "file"
	DirEntrySymlink   = "symlink"
)

type LocationListParams struct {
	Kind   string `json:"kind,omitempty"`
	Status string `json:"status,omitempty"`
	Ready  *bool  `json:"ready,omitempty"`
	Limit  int    `json:"limit,omitempty"`
	Offset int    `json:"offset,omitempty"`
}

type LocationListResult struct {
	Locations []LocationView `json:"locations"`
}

type LocationGetParams struct {
	LocationID string `json:"locationId"`
}

type LocationResult struct {
	Location LocationView `json:"location"`
}

type LocationCreateParams struct {
	Kind    string              `json:"kind"`
	Label   string              `json:"label"`
	Address LocationAddressView `json:"address,omitempty"`
	Auth    LocationAuthView    `json:"auth,omitempty"`
}

type LocationUpdateParams struct {
	LocationID string               `json:"locationId"`
	Label      string               `json:"label,omitempty"`
	Address    *LocationAddressView `json:"address,omitempty"`
	Auth       *LocationAuthView    `json:"auth,omitempty"`
}

type LocationDeleteParams struct {
	LocationID string `json:"locationId"`
}

type LocationProbeParams struct {
	LocationID string `json:"locationId"`
}

type LocationView struct {
	ID           string               `json:"id"`
	Kind         string               `json:"kind"`
	Label        string               `json:"label"`
	Address      *LocationAddressView `json:"address,omitempty"`
	Status       string               `json:"status"`
	Ready        bool                 `json:"ready"`
	Capabilities []CapabilityView     `json:"capabilities,omitempty"`
	Auth         LocationAuthView     `json:"auth,omitempty"`
	LastProbe    *ProbeView           `json:"lastProbe,omitempty"`
	CreatedAt    *time.Time           `json:"createdAt,omitempty"`
	UpdatedAt    *time.Time           `json:"updatedAt,omitempty"`
}

type LocationAddressView struct {
	Host     string `json:"host,omitempty"`
	Port     int    `json:"port,omitempty"`
	Username string `json:"username,omitempty"`
}

type CapabilityView struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

type LocationAuthView struct {
	Mode          string `json:"mode,omitempty"`
	CredentialID  string `json:"credentialId,omitempty"`
	HasCredential bool   `json:"hasCredential,omitempty"`
}

type ProbeView struct {
	Status      string     `json:"status,omitempty"`
	FailureCode string     `json:"failureCode,omitempty"`
	Message     string     `json:"message,omitempty"`
	ProbedAt    *time.Time `json:"probedAt,omitempty"`
}

type LocationFSListDirParams struct {
	LocationID string `json:"locationId"`
	Path       string `json:"path"`
}

type LocationFSListDirResult struct {
	Entries []DirEntry `json:"entries"`
}

type DirEntry struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Type string `json:"type"`
	Size int64  `json:"size,omitempty"`
}

func NewLocationView(location locationdomain.Location) LocationView {
	record := location.Record()
	address := LocationAddressView{
		Host:     record.Address.Host,
		Port:     record.Address.Port,
		Username: record.Address.Username,
	}
	var addressPtr *LocationAddressView
	if address.Host != "" || address.Port != 0 || address.Username != "" {
		addressPtr = &address
	}
	view := LocationView{
		ID:           string(record.ID),
		Kind:         string(record.Kind),
		Label:        record.Label,
		Address:      addressPtr,
		Status:       string(record.Status),
		Ready:        record.Ready,
		Capabilities: capabilityViews(record.Probe),
		Auth: LocationAuthView{
			CredentialID:  record.CredentialRef,
			HasCredential: record.CredentialRef != "",
		},
		CreatedAt: cloneTime(record.CreatedAt),
		UpdatedAt: cloneTime(record.UpdatedAt),
	}
	if record.LastProbedAt != nil || record.LastProbeError != "" {
		view.LastProbe = &ProbeView{
			Status:   probeStatus(record),
			Message:  record.LastProbeError,
			ProbedAt: cloneOptionalTime(record.LastProbedAt),
		}
	}
	return view
}

func capabilityViews(probe locationdomain.Probe) []CapabilityView {
	return []CapabilityView{
		{Name: "reachable", Status: boolStatus(probe.Reachable)},
		{Name: "fileBrowsing", Status: boolStatus(probe.FileBrowsing)},
	}
}

func boolStatus(value bool) string {
	if value {
		return ProbeStatusPassed
	}
	return ProbeStatusUnknown
}

func probeStatus(record locationdomain.Record) string {
	if record.LastProbeError != "" {
		return ProbeStatusFailed
	}
	if record.Probe.Reachable && record.Probe.FileBrowsing {
		return ProbeStatusPassed
	}
	return ProbeStatusUnknown
}

func cloneTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	clone := t.UTC()
	return &clone
}

func cloneOptionalTime(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	return cloneTime(*t)
}
