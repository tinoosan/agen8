package types

import "strings"

func NormalizeTaskID(raw string) (normalized string, changed bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}
	switch {
	case strings.HasPrefix(raw, "task-"):
		return raw, false
	case strings.HasPrefix(raw, "heartbeat-"):
		return raw, false
	default:
		return "task-" + raw, true
	}
}

