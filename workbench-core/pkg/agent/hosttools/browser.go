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
						"enum": []string{"start", "navigate", "dismiss", "click", "type", "extract", "screenshot", "pdf", "close"},
					},
					"options": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"sessionId":   map[string]any{"type": stringOrNull, "description": "Browser session ID (required for all actions except start)."},
							"url":         map[string]any{"type": stringOrNull, "description": "URL (navigate)."},
							"autoDismiss": map[string]any{"type": boolOrNull, "description": "Navigate only. Best-effort auto-dismiss cookie banners/popups after navigation (default: true)."},
							"kind":        map[string]any{"type": stringOrNull, "description": "Dismiss only. What to dismiss: cookies|popups|all (default: cookies)."},
							"mode":        map[string]any{"type": stringOrNull, "description": "Dismiss only. How to dismiss: accept|reject|close (default: accept for cookies, close for popups)."},
							"maxClicks":   map[string]any{"type": intOrNull, "description": "Dismiss only. Max number of clicks to attempt across strategies (default: 3)."},
							"selector":    map[string]any{"type": stringOrNull, "description": "CSS selector (click/type/extract)."},
							"text":        map[string]any{"type": stringOrNull, "description": "Text to fill (type)."},
							"waitFor":     map[string]any{"type": stringOrNull, "description": "Optional selector to wait for after action."},
							"attribute":   map[string]any{"type": stringOrNull, "description": "Attribute/property to extract (default: textContent). For unknown values, uses getAttribute()."},
							"headless":    map[string]any{"type": boolOrNull, "description": "Start only. Launch browser headless (default: true)."},
							"fullPage":    map[string]any{"type": boolOrNull, "description": "Screenshot only. Capture full page (default: true)."},
						},
						// Structured Outputs requires every field to be listed as required; emulate optional fields with null unions.
						"required":             []any{"sessionId", "url", "autoDismiss", "kind", "mode", "maxClicks", "selector", "text", "waitFor", "attribute", "headless", "fullPage"},
						"additionalProperties": false,
					},
				},
				// Structured Outputs requires every field to be listed as required; options fields are nullable.
				"required":             []any{"action", "options"},
				"additionalProperties": false,
			},
		},
	}
}

func (t *BrowserTool) Execute(_ context.Context, args json.RawMessage) (types.HostOpRequest, error) {
	var payload struct {
		Action  string `json:"action"`
		Options struct {
			SessionID   *string `json:"sessionId"`
			URL         *string `json:"url"`
			AutoDismiss *bool   `json:"autoDismiss"`
			Kind        *string `json:"kind"`
			Mode        *string `json:"mode"`
			MaxClicks   *int    `json:"maxClicks"`
			Selector    *string `json:"selector"`
			Text        *string `json:"text"`
			WaitFor     *string `json:"waitFor"`
			Attribute   *string `json:"attribute"`
			Headless    *bool   `json:"headless"`
			FullPage    *bool   `json:"fullPage"`
		} `json:"options"`
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
	case "dismiss":
		if payload.Options.SessionID == nil || strings.TrimSpace(*payload.Options.SessionID) == "" {
			return types.HostOpRequest{}, fmt.Errorf("sessionId is required")
		}
	case "close":
		if payload.Options.SessionID == nil || strings.TrimSpace(*payload.Options.SessionID) == "" {
			return types.HostOpRequest{}, fmt.Errorf("sessionId is required")
		}
	case "navigate":
		if payload.Options.SessionID == nil || strings.TrimSpace(*payload.Options.SessionID) == "" {
			return types.HostOpRequest{}, fmt.Errorf("sessionId is required")
		}
		if payload.Options.URL == nil || strings.TrimSpace(*payload.Options.URL) == "" {
			return types.HostOpRequest{}, fmt.Errorf("url is required")
		}
	case "click":
		if payload.Options.SessionID == nil || strings.TrimSpace(*payload.Options.SessionID) == "" {
			return types.HostOpRequest{}, fmt.Errorf("sessionId is required")
		}
		if payload.Options.Selector == nil || strings.TrimSpace(*payload.Options.Selector) == "" {
			return types.HostOpRequest{}, fmt.Errorf("selector is required")
		}
	case "type":
		if payload.Options.SessionID == nil || strings.TrimSpace(*payload.Options.SessionID) == "" {
			return types.HostOpRequest{}, fmt.Errorf("sessionId is required")
		}
		if payload.Options.Selector == nil || strings.TrimSpace(*payload.Options.Selector) == "" {
			return types.HostOpRequest{}, fmt.Errorf("selector is required")
		}
		if payload.Options.Text == nil {
			return types.HostOpRequest{}, fmt.Errorf("text is required")
		}
	case "extract":
		if payload.Options.SessionID == nil || strings.TrimSpace(*payload.Options.SessionID) == "" {
			return types.HostOpRequest{}, fmt.Errorf("sessionId is required")
		}
		if payload.Options.Selector == nil || strings.TrimSpace(*payload.Options.Selector) == "" {
			return types.HostOpRequest{}, fmt.Errorf("selector is required")
		}
	case "screenshot", "pdf":
		if payload.Options.SessionID == nil || strings.TrimSpace(*payload.Options.SessionID) == "" {
			return types.HostOpRequest{}, fmt.Errorf("sessionId is required")
		}
	default:
		return types.HostOpRequest{}, fmt.Errorf("unknown action %q", action)
	}

	input := map[string]any{"action": action}
	opts := payload.Options
	if opts.SessionID != nil {
		input["sessionId"] = strings.TrimSpace(*opts.SessionID)
	}
	if opts.URL != nil {
		input["url"] = strings.TrimSpace(*opts.URL)
	}
	if opts.AutoDismiss != nil {
		input["autoDismiss"] = *opts.AutoDismiss
	}
	if opts.Kind != nil {
		input["kind"] = strings.TrimSpace(*opts.Kind)
	}
	if opts.Mode != nil {
		input["mode"] = strings.TrimSpace(*opts.Mode)
	}
	if opts.MaxClicks != nil {
		input["maxClicks"] = *opts.MaxClicks
	}
	if opts.Selector != nil {
		input["selector"] = strings.TrimSpace(*opts.Selector)
	}
	if opts.Text != nil {
		input["text"] = *opts.Text
	}
	if opts.WaitFor != nil {
		input["waitFor"] = strings.TrimSpace(*opts.WaitFor)
	}
	if opts.Attribute != nil {
		input["attribute"] = strings.TrimSpace(*opts.Attribute)
	}
	if opts.Headless != nil {
		input["headless"] = *opts.Headless
	}
	if opts.FullPage != nil {
		input["fullPage"] = *opts.FullPage
	}
	payloadBytes, err := json.Marshal(input)
	if err != nil {
		return types.HostOpRequest{}, err
	}
	return types.HostOpRequest{
		Op:    types.HostOpBrowser,
		Input: payloadBytes,
	}, nil
}
