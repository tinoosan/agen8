package opmeta

import (
	"fmt"
	"strings"
)

func ShouldHideRoutingNoiseOp(op, path string) bool {
	op = strings.TrimSpace(op)
	path = strings.TrimSpace(path)
	if path == "" {
		return false
	}
	switch op {
	case "fs_list", "fs_stat", "fs_read":
		return strings.HasPrefix(path, "/workspace/deliverables/") || strings.HasPrefix(path, "/workspace/quarantine/")
	default:
		return false
	}
}

func FormatRequestTitle(d map[string]string) string {
	op := strings.TrimSpace(d["op"])
	path := strings.TrimSpace(d["path"])

	switch op {
	case "fs_list":
		return "List " + path
	case "fs_stat":
		return "Stat " + path
	case "fs_read":
		return "Read " + path
	case "fs_search":
		q := strings.TrimSpace(d["query"])
		if path != "" && q != "" {
			return fmt.Sprintf("Search %s for %q", path, q)
		}
		if path != "" {
			return "Search " + path
		}
		return "Search"
	case "fs_write":
		return "Write " + path
	case "fs_append":
		return "Append " + path
	case "fs_edit":
		return "Edit " + path
	case "fs_patch":
		return "Patch " + path
	case "shell_exec":
		if cmd := strings.TrimSpace(d["argvPreview"]); cmd != "" {
			return cmd
		}
		if argv0 := strings.TrimSpace(d["argv0"]); argv0 != "" {
			return argv0
		}
		return "shell_exec"
	case "code_exec":
		lang := strings.TrimSpace(d["language"])
		if lang == "" {
			lang = "python"
		}
		return "Run " + strings.ToLower(lang) + " code"
	case "http_fetch":
		u := strings.TrimSpace(d["url"])
		if u != "" {
			m := strings.ToUpper(strings.TrimSpace(d["method"]))
			if m == "" {
				m = "GET"
			}
			desc := m + " " + u
			if body := strings.TrimSpace(d["body"]); body != "" {
				bodyText := "body: " + singleLinePreview(body, 120)
				if strings.TrimSpace(d["bodyTruncated"]) == "true" {
					bodyText += " truncated"
				}
				desc = desc + " " + bodyText
			}
			return desc
		}
		return "http_fetch"
	case "trace_run":
		action := strings.TrimSpace(d["traceAction"])
		key := strings.TrimSpace(d["traceKey"])
		if action != "" {
			if key != "" {
				return fmt.Sprintf("trace.%s %s", action, key)
			}
			return "trace." + action
		}
		return "trace_run"
	case "agent_spawn":
		goal := strings.TrimSpace(d["goal"])
		model := strings.TrimSpace(d["model"])
		depth := strings.TrimSpace(d["currentDepth"])
		maxDepth := strings.TrimSpace(d["maxDepth"])
		desc := "Spawn child agent"
		if goal != "" {
			desc += ": " + singleLinePreview(goal, 96)
		}
		details := make([]string, 0, 2)
		if model != "" {
			details = append(details, "model="+model)
		}
		if depth != "" && maxDepth != "" {
			details = append(details, "depth="+depth+"/"+maxDepth)
		}
		if len(details) == 0 {
			return desc
		}
		return desc + " (" + strings.Join(details, ", ") + ")"
	case "task_create":
		goal := strings.TrimSpace(d["goal"])
		taskId := strings.TrimSpace(d["taskId"])
		if goal != "" {
			return "Create task: " + singleLinePreview(goal, 96)
		}
		if taskId != "" {
			return "Create task " + taskId
		}
		return "Create task"
	case "obsidian":
		cmd := strings.TrimSpace(d["command"])
		if cmd != "" {
			return "Obsidian " + cmd
		}
		return "Obsidian"
	case "task_review":
		taskID := strings.TrimSpace(d["taskId"])
		decision := strings.TrimSpace(d["decision"])
		if taskID != "" && decision != "" {
			return fmt.Sprintf("Review task %s (%s)", taskID, decision)
		}
		if taskID != "" {
			return "Review task " + taskID
		}
		return "Review task"
	case "soul_update":
		if reason := strings.TrimSpace(d["reason"]); reason != "" {
			return "Update soul: " + singleLinePreview(reason, 80)
		}
		return "Update soul"
	default:
		if op != "" && path != "" {
			return op + " " + path
		}
		if op != "" {
			return op
		}
		return "op"
	}
}

func singleLinePreview(s string, max int) string {
	s = strings.ReplaceAll(s, "\r", " ")
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
