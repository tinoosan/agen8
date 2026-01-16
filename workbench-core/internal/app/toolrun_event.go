package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

const (
	maxToolRunInputEventBytes = 1024
)

// toolRunInputForEvent returns a small, sanitized JSON string representation of a tool.run input payload
// suitable for emitting in UI events.
//
// Why this exists:
//   - ToolRequest.Input can be arbitrarily large (e.g., formatting a full HTML document).
//   - Some fields are sensitive (e.g., "stdin", "token") and should not be echoed back verbatim.
//   - The UI wants enough structure to render human-friendly action lines (argv/query/paths), without
//     flooding the transcript or inspector.
//
// Behavior:
//   - Best-effort JSON parsing: on parse failure, returns a compact single-line preview.
//   - Redacts large or sensitive string fields (e.g., "text", "stdin", "token").
//   - Produces a compact one-line JSON object and hard-caps its size.
func toolRunInputForEvent(raw json.RawMessage) (sanitized string, truncated bool, originalBytes int) {
	raw = bytes.TrimSpace(raw)
	originalBytes = len(raw)
	if len(raw) == 0 {
		return "", false, 0
	}

	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		s := singleLine(string(raw))
		s2, tr := capBytes(s, maxToolRunInputEventBytes)
		return s2, tr, originalBytes
	}

	out := make(map[string]any, len(obj))
	for k, v := range obj {
		out[k] = redactValue(k, v)
	}

	b, err := json.Marshal(out)
	if err != nil {
		s := singleLine(string(raw))
		s2, tr := capBytes(s, maxToolRunInputEventBytes)
		return s2, tr, originalBytes
	}

	s2, tr := capBytes(singleLine(string(b)), maxToolRunInputEventBytes)
	return s2, tr, originalBytes
}

const maxToolRunOutputPreviewBytes = 1200

// toolRunOutputPreviewForEvent returns a small, human-readable summary of a tool.run output payload.
//
// This is used only for UI events so the Activity details panel can show a preview without reading
// /results/<callId>/response.json.
//
// It is intentionally conservative:
//   - hard-caps size
//   - avoids dumping large structured payloads (e.g., ripgrep matches) in full
func toolRunOutputPreviewForEvent(toolID, actionID string, raw json.RawMessage) string {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return ""
	}

	switch strings.TrimSpace(toolID) {
	case "builtin.bash":
		if strings.TrimSpace(actionID) != "exec" {
			break
		}
		var out struct {
			ExitCode int    `json:"exitCode"`
			Stdout   string `json:"stdout"`
			Stderr   string `json:"stderr"`
		}
		if err := json.Unmarshal(raw, &out); err != nil {
			break
		}
		stdout := strings.TrimSpace(out.Stdout)
		stderr := strings.TrimSpace(out.Stderr)
		s := fmt.Sprintf("exitCode=%d", out.ExitCode)
		if stdout != "" {
			s += " stdout=" + previewText(stdout, 400)
		}
		if stderr != "" {
			s += " stderr=" + previewText(stderr, 400)
		}
		s2, _ := capBytes(singleLine(s), maxToolRunOutputPreviewBytes)
		return s2

	case "builtin.ripgrep":
		if strings.TrimSpace(actionID) != "search" {
			break
		}
		var out struct {
			Matches []any `json:"matches"`
		}
		if err := json.Unmarshal(raw, &out); err != nil {
			break
		}
		s := fmt.Sprintf("matches=%d", len(out.Matches))
		s2, _ := capBytes(s, maxToolRunOutputPreviewBytes)
		return s2
	}

	// Generic fallback: show a compact JSON string preview.
	s2, _ := capBytes(singleLine(string(raw)), maxToolRunOutputPreviewBytes)
	return s2
}

func previewText(s string, max int) string {
	s = strings.ReplaceAll(s, "\r", "")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.TrimSpace(s)
	if max <= 0 || len(s) <= max {
		return s
	}
	if max < 2 {
		return s[:max]
	}
	return s[:max-1] + "…"
}

func redactValue(key string, v any) any {
	k := strings.ToLower(strings.TrimSpace(key))
	switch x := v.(type) {
	case string:
		if isSensitiveKey(k) || len(x) > 256 {
			return "<omitted>"
		}
		return x
	case []any:
		// Keep arrays small and stable (e.g., argv, paths, glob). Redact long strings.
		out := make([]any, 0, len(x))
		for _, it := range x {
			if s, ok := it.(string); ok {
				if len(s) > 256 {
					out = append(out, "<omitted>")
				} else {
					out = append(out, s)
				}
				continue
			}
			out = append(out, it)
		}
		return out
	case map[string]any:
		keys := make([]string, 0, len(x))
		for kk := range x {
			keys = append(keys, kk)
		}
		sort.Strings(keys)
		m := make(map[string]any, len(keys))
		for _, kk := range keys {
			m[kk] = redactValue(kk, x[kk])
		}
		return m
	default:
		return v
	}
}

func isSensitiveKey(k string) bool {
	switch k {
	case "text", "stdin", "authorization", "token", "apikey", "api_key", "secret", "password":
		return true
	default:
		return false
	}
}

func singleLine(s string) string {
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	return strings.TrimSpace(s)
}

func capBytes(s string, max int) (string, bool) {
	if max <= 0 || len(s) <= max {
		return s, false
	}
	if max < 2 {
		return s[:max], true
	}
	return s[:max-1] + "…", true
}
