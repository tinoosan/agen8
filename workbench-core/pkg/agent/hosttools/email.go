package hosttools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	llmtypes "github.com/tinoosan/workbench-core/pkg/llm/types"
	"github.com/tinoosan/workbench-core/pkg/types"
)

// EmailTool sends email notifications.
type EmailTool struct{}

func (t *EmailTool) Definition() llmtypes.Tool {
	return llmtypes.Tool{
		Type: "function",
		Function: llmtypes.ToolFunction{
			Name:        "email",
			Description: "Send email notifications. Use for task completion reports or important updates.",
			Strict:      true,
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"to":      map[string]any{"type": "string", "description": "Recipient email address (or comma-separated list)."},
					"subject": map[string]any{"type": "string", "description": "Email subject line."},
					"body":    map[string]any{"type": "string", "description": "Email body (plain text)."},
				},
				"required":             []any{"to", "subject", "body"},
				"additionalProperties": false,
			},
		},
	}
}

func (t *EmailTool) Execute(_ context.Context, args json.RawMessage) (types.HostOpRequest, error) {
	var payload struct {
		To      string `json:"to"`
		Subject string `json:"subject"`
		Body    string `json:"body"`
	}
	if err := json.Unmarshal(args, &payload); err != nil {
		return types.HostOpRequest{}, err
	}
	if strings.TrimSpace(payload.To) == "" || strings.TrimSpace(payload.Subject) == "" || strings.TrimSpace(payload.Body) == "" {
		return types.HostOpRequest{}, fmt.Errorf("to, subject, and body are required")
	}
	return types.HostOpRequest{
		Op:    types.HostOpEmail,
		Input: args,
	}, nil
}
