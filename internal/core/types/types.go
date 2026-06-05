package types

import (
	"encoding/json"
	"time"
)

type ProjectID string
type LocationID string
type RunID string
type EventID string
type ChannelID string

type Lifecycle struct {
	Status      string     `json:"status,omitempty"`
	Phase       string     `json:"phase,omitempty"`
	CreatedAt   *time.Time `json:"createdAt,omitempty"`
	StartedAt   *time.Time `json:"startedAt,omitempty"`
	CompletedAt *time.Time `json:"completedAt,omitempty"`
	UpdatedAt   *time.Time `json:"updatedAt,omitempty"`
}

type LifecyclePhase string

const (
	LifecyclePhaseOpen     LifecyclePhase = "open"
	LifecyclePhaseActive   LifecyclePhase = "active"
	LifecyclePhaseDone     LifecyclePhase = "done"
	LifecyclePhaseArchived LifecyclePhase = "archived"
)

func LifecyclePhaseForStatus(status string) LifecyclePhase {
	switch status {
	case "completed", "done", "approved", "canceled", "cancelled", "failed":
		return LifecyclePhaseDone
	case "archived", "deleted":
		return LifecyclePhaseArchived
	case "active", "in_progress", "in_review", "blocked":
		return LifecyclePhaseActive
	default:
		return LifecyclePhaseOpen
	}
}

type EventRecord struct {
	EventID   EventID           `json:"eventId"`
	RunID     RunID             `json:"runId"`
	Type      string            `json:"type"`
	Message   string            `json:"message"`
	Data      map[string]string `json:"data,omitempty"`
	Timestamp time.Time         `json:"timestamp,omitempty"`
	Origin    string            `json:"origin,omitempty"`
	CreatedAt string            `json:"createdAt,omitempty"`
}

type ArtifactNode struct {
	ID          string            `json:"id,omitempty"`
	Type        string            `json:"type,omitempty"`
	Title       string            `json:"title,omitempty"`
	Path        string            `json:"path,omitempty"`
	MimeType    string            `json:"mimeType,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	NodeKey     string            `json:"nodeKey,omitempty"`
	Kind        string            `json:"kind,omitempty"`
	Label       string            `json:"label,omitempty"`
	DisplayName string            `json:"displayName,omitempty"`
	VPath       string            `json:"vpath,omitempty"`
}

type ToolDiscoveryCatalog struct {
	Tools []ToolDiscoveryEntry `json:"tools"`
}

type ToolDiscoveryEntry struct {
	Name              string            `json:"name"`
	Description       string            `json:"description,omitempty"`
	DirectAvailable   bool              `json:"directAvailable,omitempty"`
	BridgeAvailable   bool              `json:"bridgeAvailable,omitempty"`
	PrimaryInvocation string            `json:"primaryInvocation,omitempty"`
	BridgeCallSyntax  string            `json:"bridgeCallSyntax,omitempty"`
	Usage             []string          `json:"usage,omitempty"`
	Schema            json.RawMessage   `json:"schema,omitempty"`
	Metadata          map[string]string `json:"metadata,omitempty"`
}
