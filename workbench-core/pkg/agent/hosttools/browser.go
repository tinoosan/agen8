package hosttools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	llmtypes "github.com/tinoosan/workbench-core/pkg/llm/types"
	"github.com/tinoosan/workbench-core/pkg/types"
)

// BrowserTool provides interactive web browsing backed by a host-side browser session manager.
type BrowserTool struct{}

func (t *BrowserTool) Definition() llmtypes.Tool {
	return llmtypes.Tool{
		Type: "function",
		Function: llmtypes.ToolFunction{
			Name:        "browser",
			Description: "Interactive web browser for JavaScript-rendered sites and multi-step workflows. Use to navigate, click, fill forms, extract data, and capture screenshots/PDFs. For simple HTTP APIs/static pages, prefer http_fetch.",
			Strict:      true,
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"action": map[string]any{
						"type": "string",
						"enum": []string{"start", "navigate", "click", "type", "extract", "screenshot", "pdf", "close"},
					},
					"sessionId": map[string]any{"type": stringOrNull, "description": "Browser session ID (required for all actions except start)."},
					"url":       map[string]any{"type": stringOrNull, "description": "URL (navigate)."},
					"selector":  map[string]any{"type": stringOrNull, "description": "CSS selector (click/type/extract)."},
					"text":      map[string]any{"type": stringOrNull, "description": "Text to fill (type)."},
					"waitFor":   map[string]any{"type": stringOrNull, "description": "Optional selector to wait for after action."},
					"attribute": map[string]any{"type": stringOrNull, "description": "Attribute/property to extract (default: textContent). For unknown values, uses getAttribute()."},
					"headless":  map[string]any{"type": boolOrNull, "description": "Start only. Launch browser headless (default: true)."},
					"fullPage":  map[string]any{"type": boolOrNull, "description": "Screenshot only. Capture full page (default: true)."},
				},
				"required":             []any{"action"},
				"additionalProperties": false,
			},
		},
	}
}

func (t *BrowserTool) Execute(_ context.Context, args json.RawMessage) (types.HostOpRequest, error) {
	var payload struct {
		Action    string  `json:"action"`
		SessionID *string `json:"sessionId"`
		URL       *string `json:"url"`
		Selector  *string `json:"selector"`
		Text      *string `json:"text"`
	}
	if err := json.Unmarshal(args, &payload); err != nil {
		return types.HostOpRequest{}, err
	}
	action := strings.ToLower(strings.TrimSpace(payload.Action))
	if action == "" {
		return types.HostOpRequest{}, fmt.Errorf("action is required")
	}
	switch action {
	case "start":
		// ok
	case "close":
		if payload.SessionID == nil || strings.TrimSpace(*payload.SessionID) == "" {
			return types.HostOpRequest{}, fmt.Errorf("sessionId is required")
		}
	case "navigate":
		if payload.SessionID == nil || strings.TrimSpace(*payload.SessionID) == "" {
			return types.HostOpRequest{}, fmt.Errorf("sessionId is required")
		}
		if payload.URL == nil || strings.TrimSpace(*payload.URL) == "" {
			return types.HostOpRequest{}, fmt.Errorf("url is required")
		}
	case "click":
		if payload.SessionID == nil || strings.TrimSpace(*payload.SessionID) == "" {
			return types.HostOpRequest{}, fmt.Errorf("sessionId is required")
		}
		if payload.Selector == nil || strings.TrimSpace(*payload.Selector) == "" {
			return types.HostOpRequest{}, fmt.Errorf("selector is required")
		}
	case "type":
		if payload.SessionID == nil || strings.TrimSpace(*payload.SessionID) == "" {
			return types.HostOpRequest{}, fmt.Errorf("sessionId is required")
		}
		if payload.Selector == nil || strings.TrimSpace(*payload.Selector) == "" {
			return types.HostOpRequest{}, fmt.Errorf("selector is required")
		}
		if payload.Text == nil {
			return types.HostOpRequest{}, fmt.Errorf("text is required")
		}
	case "extract":
		if payload.SessionID == nil || strings.TrimSpace(*payload.SessionID) == "" {
			return types.HostOpRequest{}, fmt.Errorf("sessionId is required")
		}
		if payload.Selector == nil || strings.TrimSpace(*payload.Selector) == "" {
			return types.HostOpRequest{}, fmt.Errorf("selector is required")
		}
	case "screenshot", "pdf":
		if payload.SessionID == nil || strings.TrimSpace(*payload.SessionID) == "" {
			return types.HostOpRequest{}, fmt.Errorf("sessionId is required")
		}
	default:
		return types.HostOpRequest{}, fmt.Errorf("unknown action %q", action)
	}

	// Host executor interprets args; keep the payload intact for strict parsing.
	return types.HostOpRequest{
		Op:    types.HostOpBrowser,
		Input: args,
	}, nil
}
