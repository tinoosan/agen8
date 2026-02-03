package hosttools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	llmtypes "github.com/tinoosan/workbench-core/pkg/llm/types"
	"github.com/tinoosan/workbench-core/pkg/types"
)

// HTTPFetchTool performs an HTTP request.
type HTTPFetchTool struct{}

func (t *HTTPFetchTool) Definition() llmtypes.Tool {
	return llmtypes.Tool{
		Type: "function",
		Function: llmtypes.ToolFunction{
			Name:        "http.fetch",
			Description: "[CORE] Make an HTTP request. Returns status, headers, and body.",
			Strict:      false,
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"url":             map[string]any{"type": "string", "description": "URL to fetch."},
					"method":          map[string]any{"type": stringOrNull, "description": "HTTP method (default: GET)."},
					"headers":         map[string]any{"type": []any{"object", "null"}, "description": "Request headers."},
					"body":            map[string]any{"type": stringOrNull, "description": "Request body."},
					"maxBytes":        map[string]any{"type": intOrNull, "description": "Max response bytes."},
					"followRedirects": map[string]any{"type": boolOrNull, "description": "Follow redirects (default: true)."},
				},
				"required":             []any{"url"},
				"additionalProperties": false,
			},
		},
	}
}

func (t *HTTPFetchTool) Execute(_ context.Context, args json.RawMessage) (types.HostOpRequest, error) {
	var payload struct {
		URL             string            `json:"url"`
		Method          *string           `json:"method"`
		Headers         map[string]string `json:"headers"`
		Body            *string           `json:"body"`
		MaxBytes        *int              `json:"maxBytes"`
		FollowRedirects *bool             `json:"followRedirects"`
	}
	if err := json.Unmarshal(args, &payload); err != nil {
		return types.HostOpRequest{}, err
	}
	url := strings.TrimSpace(payload.URL)
	if url == "" {
		return types.HostOpRequest{}, fmt.Errorf("url is required")
	}
	req := types.HostOpRequest{
		Op:     types.HostOpHTTPFetch,
		URL:    url,
		Method: "",
	}
	if payload.Method != nil {
		req.Method = strings.TrimSpace(*payload.Method)
	}
	if payload.Headers != nil {
		req.Headers = payload.Headers
	}
	if payload.Body != nil && strings.TrimSpace(*payload.Body) != "" {
		req.Body = *payload.Body
	}
	if payload.MaxBytes != nil {
		req.MaxBytes = *payload.MaxBytes
	}
	if payload.FollowRedirects != nil {
		req.FollowRedirects = payload.FollowRedirects
	}
	return req, nil
}
