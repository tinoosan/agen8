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
						"enum": []string{
							"start",
							"navigate",
							"wait",
							"dismiss",
							"click",
							"type",
							"hover",
							"press",
							"scroll",
							"select",
							"check",
							"uncheck",
							"upload",
							"download",
							"back",
							"forward",
							"reload",
							"tab_new",
							"tab_list",
							"tab_switch",
							"tab_close",
							"storage_save",
							"storage_load",
							"set_headers",
							"set_viewport",
							"extract",
							"extract_links",
							"screenshot",
							"pdf",
							"close",
						},
					},
					"options": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"sessionId":      map[string]any{"type": stringOrNull, "description": "Browser session ID (required for all actions except start)."},
							"url":            map[string]any{"type": stringOrNull, "description": "URL (navigate, tab_new, wait-for-url)."},
							"autoDismiss":    map[string]any{"type": boolOrNull, "description": "Navigate only. Best-effort auto-dismiss cookie banners/popups after navigation (default: true)."},
							"kind":           map[string]any{"type": stringOrNull, "description": "Dismiss only. What to dismiss: cookies|popups|all (default: cookies)."},
							"mode":           map[string]any{"type": stringOrNull, "description": "Dismiss only. How to dismiss: accept|reject|close (default: accept for cookies, close for popups)."},
							"maxClicks":      map[string]any{"type": intOrNull, "description": "Dismiss only. Max number of clicks to attempt across strategies (default: 3)."},
							"timeoutMs":      map[string]any{"type": intOrNull, "description": "Optional override timeout (milliseconds) for the current action."},
							"waitType":       map[string]any{"type": stringOrNull, "description": "Wait only. selector|url|load|networkidle|timeout (inferred if omitted)."},
							"state":          map[string]any{"type": stringOrNull, "description": "Wait only. For selector: attached|detached|visible|hidden. For load: load|domcontentloaded|networkidle."},
							"sleepMs":        map[string]any{"type": intOrNull, "description": "Wait only. Sleep/pause for this duration in milliseconds."},
							"selector":       map[string]any{"type": stringOrNull, "description": "CSS selector (click/type/hover/press/select/check/uncheck/upload/download/extract)."},
							"text":           map[string]any{"type": stringOrNull, "description": "Text to fill (type)."},
							"waitFor":        map[string]any{"type": stringOrNull, "description": "Optional selector to wait for after action."},
							"attribute":      map[string]any{"type": stringOrNull, "description": "Attribute/property to extract (default: textContent). For unknown values, uses getAttribute()."},
							"headless":       map[string]any{"type": boolOrNull, "description": "Start only. Launch browser headless (default: true)."},
							"userAgent":      map[string]any{"type": stringOrNull, "description": "Start only. Set a custom user-agent string."},
							"viewportWidth":  map[string]any{"type": intOrNull, "description": "Start/set_viewport. Viewport width in pixels."},
							"viewportHeight": map[string]any{"type": intOrNull, "description": "Start/set_viewport. Viewport height in pixels."},
							"headersJson":    map[string]any{"type": stringOrNull, "description": "Start/set_headers. JSON object of extra HTTP headers (string values)."},
							"expectPopup":    map[string]any{"type": boolOrNull, "description": "Click only. If true, wait for a popup/new tab and return its tabId."},
							"setActive":      map[string]any{"type": boolOrNull, "description": "tab_new only. Make the new tab active (default: true)."},
							"pageId":         map[string]any{"type": stringOrNull, "description": "tab_switch/tab_close only. Tab/page id."},
							"key":            map[string]any{"type": stringOrNull, "description": "press only. Key to press (e.g., Enter, Escape, Control+K)."},
							"dx":             map[string]any{"type": intOrNull, "description": "scroll only. Pixels to scroll horizontally (default: 0)."},
							"dy":             map[string]any{"type": intOrNull, "description": "scroll only. Pixels to scroll vertically (default: 500)."},
							"value":          map[string]any{"type": stringOrNull, "description": "select only. Option value/label to select."},
							"values":         stringArrayOrNull,
							"filePath":       map[string]any{"type": stringOrNull, "description": "upload only. VFS path to file (e.g., /project/resume.pdf or /workspace/resume.pdf)."},
							"filename":       map[string]any{"type": stringOrNull, "description": "download/storage_save/storage_load. Target filename under /workspace."},
							"fullPage":       map[string]any{"type": boolOrNull, "description": "Screenshot only. Capture full page (default: true)."},
						},
						// Structured Outputs requires every field to be listed as required; emulate optional fields with null unions.
						"required": []any{
							"sessionId",
							"url",
							"autoDismiss",
							"kind",
							"mode",
							"maxClicks",
							"timeoutMs",
							"waitType",
							"state",
							"sleepMs",
							"selector",
							"text",
							"waitFor",
							"attribute",
							"headless",
							"userAgent",
							"viewportWidth",
							"viewportHeight",
							"headersJson",
							"expectPopup",
							"setActive",
							"pageId",
							"key",
							"dx",
							"dy",
							"value",
							"values",
							"filePath",
							"filename",
							"fullPage",
						},
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
			SessionID   *string   `json:"sessionId"`
			URL         *string   `json:"url"`
			AutoDismiss *bool     `json:"autoDismiss"`
			Kind        *string   `json:"kind"`
			Mode        *string   `json:"mode"`
			MaxClicks   *int      `json:"maxClicks"`
			TimeoutMs   *int      `json:"timeoutMs"`
			WaitType    *string   `json:"waitType"`
			State       *string   `json:"state"`
			SleepMs     *int      `json:"sleepMs"`
			Selector    *string   `json:"selector"`
			Text        *string   `json:"text"`
			WaitFor     *string   `json:"waitFor"`
			Attribute   *string   `json:"attribute"`
			Headless    *bool     `json:"headless"`
			UserAgent   *string   `json:"userAgent"`
			ViewportW   *int      `json:"viewportWidth"`
			ViewportH   *int      `json:"viewportHeight"`
			HeadersJSON *string   `json:"headersJson"`
			ExpectPopup *bool     `json:"expectPopup"`
			SetActive   *bool     `json:"setActive"`
			PageID      *string   `json:"pageId"`
			Key         *string   `json:"key"`
			DX          *int      `json:"dx"`
			DY          *int      `json:"dy"`
			Value       *string   `json:"value"`
			Values      *[]string `json:"values"`
			FilePath    *string   `json:"filePath"`
			Filename    *string   `json:"filename"`
			FullPage    *bool     `json:"fullPage"`
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
	case "tab_list":
		if payload.Options.SessionID == nil || strings.TrimSpace(*payload.Options.SessionID) == "" {
			return types.HostOpRequest{}, fmt.Errorf("sessionId is required")
		}
	case "dismiss":
		if payload.Options.SessionID == nil || strings.TrimSpace(*payload.Options.SessionID) == "" {
			return types.HostOpRequest{}, fmt.Errorf("sessionId is required")
		}
	case "wait", "back", "forward", "reload", "storage_save", "storage_load", "set_headers", "set_viewport":
		if payload.Options.SessionID == nil || strings.TrimSpace(*payload.Options.SessionID) == "" {
			return types.HostOpRequest{}, fmt.Errorf("sessionId is required")
		}
	case "tab_new":
		if payload.Options.SessionID == nil || strings.TrimSpace(*payload.Options.SessionID) == "" {
			return types.HostOpRequest{}, fmt.Errorf("sessionId is required")
		}
	case "tab_switch", "tab_close":
		if payload.Options.SessionID == nil || strings.TrimSpace(*payload.Options.SessionID) == "" {
			return types.HostOpRequest{}, fmt.Errorf("sessionId is required")
		}
		if payload.Options.PageID == nil || strings.TrimSpace(*payload.Options.PageID) == "" {
			return types.HostOpRequest{}, fmt.Errorf("pageId is required")
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
	case "extract_links":
		if payload.Options.SessionID == nil || strings.TrimSpace(*payload.Options.SessionID) == "" {
			return types.HostOpRequest{}, fmt.Errorf("sessionId is required")
		}
	case "hover", "press", "select", "check", "uncheck", "upload", "download", "scroll":
		if payload.Options.SessionID == nil || strings.TrimSpace(*payload.Options.SessionID) == "" {
			return types.HostOpRequest{}, fmt.Errorf("sessionId is required")
		}
		if action != "scroll" && (payload.Options.Selector == nil || strings.TrimSpace(*payload.Options.Selector) == "") {
			return types.HostOpRequest{}, fmt.Errorf("selector is required")
		}
		if action == "press" && (payload.Options.Key == nil || strings.TrimSpace(*payload.Options.Key) == "") {
			return types.HostOpRequest{}, fmt.Errorf("key is required")
		}
		if action == "upload" && (payload.Options.FilePath == nil || strings.TrimSpace(*payload.Options.FilePath) == "") {
			return types.HostOpRequest{}, fmt.Errorf("filePath is required")
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
	if opts.TimeoutMs != nil {
		input["timeoutMs"] = *opts.TimeoutMs
	}
	if opts.WaitType != nil {
		input["waitType"] = strings.TrimSpace(*opts.WaitType)
	}
	if opts.State != nil {
		input["state"] = strings.TrimSpace(*opts.State)
	}
	if opts.SleepMs != nil {
		input["sleepMs"] = *opts.SleepMs
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
	if opts.UserAgent != nil {
		input["userAgent"] = strings.TrimSpace(*opts.UserAgent)
	}
	if opts.ViewportW != nil {
		input["viewportWidth"] = *opts.ViewportW
	}
	if opts.ViewportH != nil {
		input["viewportHeight"] = *opts.ViewportH
	}
	if opts.HeadersJSON != nil {
		input["headersJson"] = strings.TrimSpace(*opts.HeadersJSON)
	}
	if opts.ExpectPopup != nil {
		input["expectPopup"] = *opts.ExpectPopup
	}
	if opts.SetActive != nil {
		input["setActive"] = *opts.SetActive
	}
	if opts.PageID != nil {
		input["pageId"] = strings.TrimSpace(*opts.PageID)
	}
	if opts.Key != nil {
		input["key"] = strings.TrimSpace(*opts.Key)
	}
	if opts.DX != nil {
		input["dx"] = *opts.DX
	}
	if opts.DY != nil {
		input["dy"] = *opts.DY
	}
	if opts.Value != nil {
		input["value"] = strings.TrimSpace(*opts.Value)
	}
	if opts.Values != nil {
		input["values"] = *opts.Values
	}
	if opts.FilePath != nil {
		input["filePath"] = strings.TrimSpace(*opts.FilePath)
	}
	if opts.Filename != nil {
		input["filename"] = strings.TrimSpace(*opts.Filename)
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
