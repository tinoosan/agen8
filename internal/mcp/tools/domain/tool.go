package domain

import (
	"context"
	"encoding/json"
	"time"
)

// Tool is the unified contract implemented by all tool backends.
type Tool interface {
	Name() string
	Source() SourceType
	Metadata() ToolMetadata
	Definition() ToolDefinition
	Execute(ctx context.Context, args json.RawMessage) (ToolResult, error)
}

// ToolMetadata is the product-facing identity used to present a tool to operators.
type ToolMetadata struct {
	DisplayName string       `json:"displayName"`
	Category    ToolCategory `json:"category"`
	Description string       `json:"description"`
	System      bool         `json:"system"`
	UsageNotes  []string     `json:"usageNotes,omitempty"`
}

// ToolCategory groups tools in the product UI.
type ToolCategory string

const (
	CategoryExecution     ToolCategory = "Execution"
	CategoryFileSystem    ToolCategory = "File System"
	CategoryIntegration   ToolCategory = "Integration"
	CategoryCoordination  ToolCategory = "Coordination"
	CategoryObservability ToolCategory = "Observability"
	CategorySystem        ToolCategory = "System"
)

// ToolResult is the uniform result envelope returned by all tools.
type ToolResult struct {
	Text       string          `json:"text"`
	Data       json.RawMessage `json:"data,omitempty"`
	SourceType SourceType      `json:"sourceType"`
	Duration   time.Duration   `json:"duration"`
	Ok         bool            `json:"ok"`
	ErrorCode  string          `json:"errorCode,omitempty"`
	Retryable  bool            `json:"retryable,omitempty"`
	Truncated  bool            `json:"truncated,omitempty"`
	Terminal   bool            `json:"terminal,omitempty"`

	// IdempotencyKey prevents duplicate side effects on retry. When set, the
	// dispatch pipeline caches successful results and returns the cached copy
	// on subsequent calls with the same key+tool combination.
	IdempotencyKey string `json:"idempotencyKey,omitempty"`

	// DryRun indicates this result is a preview, not an actual execution.
	// The tool was NOT executed; DryRunPlan describes what would happen.
	DryRun bool `json:"dryRun,omitempty"`

	// DryRunPlan is a human-readable description of what the tool would do
	// if executed. Only set when DryRun=true.
	DryRunPlan string `json:"dryRunPlan,omitempty"`
}

// SourceType identifies the backing source for a tool.
type SourceType string

const (
	SourceBuiltin SourceType = "builtin"
	SourceMCP     SourceType = "mcp"
	SourceCLI     SourceType = "cli"
	SourceHTTP    SourceType = "http"
)
