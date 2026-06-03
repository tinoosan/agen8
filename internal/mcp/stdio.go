package mcp

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// RunStdio serves Agen8's MCP tool surface over stdin/stdout for local harnesses.
// The supplied session may be minimal during development; handlers validate the
// identity and service dependencies they need at call time.
func RunStdio(ctx context.Context, session Session) error {
	registry, err := NewRegistry()
	if err != nil {
		return err
	}
	server := (&Server{registry: registry}).newMCPServerForSession(session)
	return server.Run(ctx, &mcp.StdioTransport{})
}
