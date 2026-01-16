package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/tinoosan/workbench-core/internal/events"
)

// formatEventLine converts a structured Workbench event into a single timeline line.
//
// The goal is compact, "high signal" output that still preserves the details that
// matter during interactive debugging. The canonical event record is still the
// on-disk JSONL log; the TUI is just a human view.
func formatEventLine(ev events.Event) string {
	switch ev.Type {
	case "agent.op.request":
		return fmt.Sprintf("* %s", formatKV(ev.Data, []string{"op", "path", "toolId", "actionId", "maxBytes", "timeoutMs"}))
	case "agent.op.response":
		return fmt.Sprintf("* %s", formatKV(ev.Data, []string{"op", "ok", "bytesLen", "truncated", "callId", "err"}))
	case "agent.error":
		return fmt.Sprintf("! %s", firstNonEmpty(ev.Data["err"], ev.Message))
	case "context.update", "context.constructor":
		return fmt.Sprintf("* %s", formatKV(ev.Data, nil))
	case "llm.usage.total":
		return fmt.Sprintf("* %s", formatKV(ev.Data, []string{"input", "output", "total"}))
	case "llm.cost.total":
		return fmt.Sprintf("* %s", formatKV(ev.Data, []string{"costUsd", "input", "output", "total"}))
	case "memory.evaluate", "memory.commit", "memory.audit.append",
		"profile.evaluate", "profile.commit", "profile.audit.append":
		return fmt.Sprintf("* %s", formatKV(ev.Data, nil))
	case "run.started", "run.completed", "session.update":
		return fmt.Sprintf("* %s", formatKV(ev.Data, nil))
	default:
		if strings.TrimSpace(ev.Message) != "" {
			if len(ev.Data) == 0 {
				return "* " + strings.TrimSpace(ev.Message)
			}
			return fmt.Sprintf("* %s (%s)", strings.TrimSpace(ev.Message), formatKV(ev.Data, nil))
		}
		if len(ev.Data) != 0 {
			return "* " + formatKV(ev.Data, nil)
		}
		return "* " + strings.TrimSpace(ev.Type)
	}
}

func firstNonEmpty(vs ...string) string {
	for _, v := range vs {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func formatKV(m map[string]string, ordered []string) string {
	if len(m) == 0 {
		return ""
	}
	var keys []string
	if len(ordered) != 0 {
		keys = ordered
	} else {
		keys = make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)
	}

	parts := make([]string, 0, len(keys))
	seen := map[string]bool{}
	for _, k := range keys {
		seen[k] = true
		if v := strings.TrimSpace(m[k]); v != "" {
			parts = append(parts, fmt.Sprintf("%s=%s", k, v))
		}
	}
	if len(ordered) != 0 {
		rest := make([]string, 0, len(m))
		for k := range m {
			if seen[k] {
				continue
			}
			rest = append(rest, k)
		}
		sort.Strings(rest)
		for _, k := range rest {
			if v := strings.TrimSpace(m[k]); v != "" {
				parts = append(parts, fmt.Sprintf("%s=%s", k, v))
			}
		}
	}

	return strings.Join(parts, " ")
}
