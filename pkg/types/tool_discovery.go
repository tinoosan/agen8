package types

import "encoding/json"

// ToolDiscoveryEntry describes one effective callable tool for the current run.
type ToolDiscoveryEntry struct {
	Name              string          `json:"name"`
	Description       string          `json:"description,omitempty"`
	Tags              []string        `json:"tags,omitempty"`
	DirectAvailable   bool            `json:"directAvailable,omitempty"`
	BridgeAvailable   bool            `json:"bridgeAvailable,omitempty"`
	PrimaryInvocation string          `json:"primaryInvocation,omitempty"`
	BridgeCallSyntax  string          `json:"bridgeCallSyntax,omitempty"`
	Usage             []string        `json:"usage,omitempty"`
	Schema            json.RawMessage `json:"schema,omitempty"`
}

// ToolDiscoveryCatalog is the structured discovery projection for the current run.
type ToolDiscoveryCatalog struct {
	Tools []ToolDiscoveryEntry `json:"tools,omitempty"`
}
