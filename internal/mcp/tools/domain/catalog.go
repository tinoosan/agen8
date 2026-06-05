package domain

import (
	"encoding/json"
)

// CatalogEntry is the product-facing listing for a registered tool.
type CatalogEntry struct {
	Name       string       `json:"name"`
	Metadata   ToolMetadata `json:"metadata"`
	SourceID   string       `json:"sourceId"`
	SourceType SourceType   `json:"sourceType"`
	Available  bool         `json:"available"`
}

// RolePolicy captures tool access rules for a single running member.
type RolePolicy struct {
	SpaceID        string   `json:"spaceId,omitempty"`
	MemberCount    int      `json:"memberCount,omitempty"`
	HasReviewer    bool     `json:"hasReviewer,omitempty"`
	AllowedSources []string `json:"allowedSources,omitempty"`
	AllowedTools   []string `json:"allowedTools,omitempty"`
}

// MemberToolCatalog is the mode-aware catalog returned to a running member.
type MemberToolCatalog struct {
	Tools []MemberCatalogEntry `json:"tools"`
}

// MemberCatalogEntry describes one tool in the running member's catalog.
type MemberCatalogEntry struct {
	Name              string          `json:"name"`
	Description       string          `json:"description"`
	DirectAvailable   bool            `json:"directAvailable"`
	BridgeAvailable   bool            `json:"bridgeAvailable"`
	PrimaryInvocation string          `json:"primaryInvocation"`
	UsageNotes        []string        `json:"usageNotes,omitempty"`
	Schema            json.RawMessage `json:"schema,omitempty"`
	SourceType        SourceType      `json:"sourceType"`
	SourceID          string          `json:"sourceId"`
}
