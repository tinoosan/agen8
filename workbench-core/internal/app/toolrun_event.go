package app

import (
	"bytes"
	"encoding/json"
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
