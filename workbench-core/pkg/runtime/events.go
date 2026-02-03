package runtime

import (
	"bytes"
	"encoding/json"
	"sort"
	"strings"
)

const (
	maxFSWriteTextPreviewBytes = 2000
)

// fsWriteTextPreviewForEvent returns a small preview of a fs.write/fs.append payload for UI events.
func fsWriteTextPreviewForEvent(path string, text string) (preview string, truncated bool, redacted bool, originalBytes int, isJSON bool) {
	originalBytes = len([]byte(text))
	if strings.TrimSpace(text) == "" {
		return "", false, false, originalBytes, false
	}

	if looksSensitiveText(text) {
		return "<omitted>", false, true, originalBytes, false
	}

	raw := bytes.TrimSpace([]byte(text))
	ext := strings.ToLower(strings.TrimSpace(path))
	isJSON = strings.HasSuffix(ext, ".json") || json.Valid(raw)

	preview = text
	if isJSON {
		var buf bytes.Buffer
		if err := json.Indent(&buf, raw, "", "  "); err == nil {
			preview = buf.String()
		}
	}

	preview, truncated = capBytes(preview, maxFSWriteTextPreviewBytes)
	return preview, truncated, false, originalBytes, isJSON
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
		if looksSensitiveText(x) {
			return "<omitted>"
		}
		if isMessageLikeKey(k) {
			return previewText(x, 120)
		}
		if isSensitiveKey(k) || len(x) > 256 {
			return "<omitted>"
		}
		return x
	case []any:
		out := make([]any, 0, len(x))
		for _, it := range x {
			if s, ok := it.(string); ok {
				if looksSensitiveText(s) {
					out = append(out, "<omitted>")
				} else if len(s) > 256 {
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

func isMessageLikeKey(k string) bool {
	switch k {
	case "message", "body", "patch":
		return true
	default:
		return false
	}
}

func looksSensitiveText(s string) bool {
	low := strings.ToLower(s)
	switch {
	case strings.Contains(low, "authorization: bearer "):
		return true
	case strings.Contains(low, "api_key"):
		return true
	case strings.Contains(low, "apikey"):
		return true
	case strings.Contains(low, "secret"):
		return true
	case strings.Contains(low, "password"):
		return true
	case strings.Contains(s, "sk-"):
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
