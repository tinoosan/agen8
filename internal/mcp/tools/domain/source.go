package domain

import (
	"context"
	"time"
)

const (
	SourceStatusConnected    = "connected"
	SourceStatusDisconnected = "disconnected"
	SourceStatusNeedsLogin   = "needs_login"
	SourceStatusDegraded     = "degraded"
)

// ToolSource discovers and health-checks tools provided by a specific backend.
type ToolSource interface {
	ID() string
	Type() SourceType
	Discover(ctx context.Context) ([]Tool, error)
	Health(ctx context.Context) SourceHealth
}

// SourceHealth describes the current status of a tool source.
type SourceHealth struct {
	Status    string    `json:"status"`
	LastCheck time.Time `json:"lastCheck"`
	Error     string    `json:"error,omitempty"`
	ToolCount int       `json:"toolCount"`
}
