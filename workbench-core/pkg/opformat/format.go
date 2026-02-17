package opformat

import (
	"sort"
	"strconv"
	"strings"

	"github.com/tinoosan/workbench-core/internal/opmeta"
)

func FormatRequestText(d map[string]string) string {
	op := strings.TrimSpace(d["op"])
	tag := strings.TrimSpace(d["tag"])

	if tag == "task_create" || op == "task_create" {
		return "Create task"
	}

	switch op {
	case "browser":
		action := strings.TrimSpace(d["action"])
		if action == "" {
			return "browse"
		}
		desc := "browse." + action
		target := ""
		switch action {
		case "navigate":
			target = strings.TrimSpace(d["url"])
		case "click", "type", "hover", "check", "uncheck", "upload", "download", "select":
			target = strings.TrimSpace(d["selector"])
		case "extract", "extract_links":
			target = strings.TrimSpace(d["selector"])
			if target == "" {
				target = "page"
			}
		default:
			target = firstNonEmpty(
				strings.TrimSpace(d["url"]),
				strings.TrimSpace(d["selector"]),
			)
		}
		if target != "" {
			return desc + " " + singleLinePreview(target, 120)
		}
		return desc
	case "email":
		to := singleLinePreview(strings.TrimSpace(d["to"]), 48)
		subject := singleLinePreview(strings.TrimSpace(d["subject"]), 72)
		if to != "" && subject != "" {
			return "Email " + to + ": " + subject
		}
		if to != "" {
			return "Email " + to
		}
		return "Email"
	case "agent_spawn":
		return opmeta.FormatRequestTitle(d)
	default:
		if isSharedOpRequestTitleOp(op) {
			return opmeta.FormatRequestTitle(d)
		}
		return compactKV(d, []string{"op", "path"})
	}
}

func FormatResponseText(d map[string]string) string {
	op := strings.TrimSpace(d["op"])
	tag := strings.TrimSpace(d["tag"])
	ok := strings.TrimSpace(d["ok"])
	errStr := strings.TrimSpace(d["err"])

	prefix := "✓"
	if ok != "true" {
		prefix = "✗"
	}

	if tag == "task_create" {
		if ok == "true" {
			return prefix + " " + strings.TrimSpace(d["text"])
		}
		if errStr != "" {
			return prefix + " " + errStr
		}
		return prefix + " task creation failed"
	}

	switch op {
	case "fs_read":
		tr := strings.TrimSpace(d["truncated"])
		if ok == "true" && tr == "true" {
			return prefix + " truncated"
		}
		if ok != "true" && errStr != "" {
			return prefix + " " + errStr
		}
		return prefix + " ok"
	case "shell_exec":
		exitCode := strings.TrimSpace(d["exitCode"])
		if ok == "true" {
			if exitCode != "" {
				return prefix + " exit " + exitCode
			}
			return prefix + " ok"
		}
		if exitCode != "" && errStr != "" {
			return prefix + " exit " + exitCode + ": " + errStr
		}
		if exitCode != "" {
			return prefix + " exit " + exitCode
		}
		if errStr != "" {
			return prefix + " " + errStr
		}
		return prefix + " failed"
	case "http_fetch":
		status := strings.TrimSpace(d["status"])
		if ok == "true" {
			if status != "" {
				return prefix + " " + status
			}
			return prefix + " ok"
		}
		if status != "" && errStr != "" {
			return prefix + " " + status + ": " + errStr
		}
		if errStr != "" {
			return prefix + " " + errStr
		}
		if status != "" {
			return prefix + " " + status
		}
		return prefix + " failed"
	case "fs_search":
		results := strings.TrimSpace(d["results"])
		if ok == "true" && results != "" {
			if results == "1" {
				return prefix + " 1 result"
			}
			return prefix + " " + results + " results"
		}
		if ok != "true" && errStr != "" {
			return prefix + " " + errStr
		}
		return prefix + " ok"
	case "email":
		if ok == "true" {
			return prefix + " sent"
		}
		if errStr != "" {
			return prefix + " " + errStr
		}
		return prefix + " failed"
	case "agent_spawn":
		if ok == "true" {
			return prefix + " child completed"
		}
		if errStr != "" {
			return prefix + " " + errStr
		}
		return prefix + " child failed"
	default:
		if strings.HasPrefix(op, "browser.") || op == "browser" {
			if ok != "true" {
				if errStr != "" {
					return prefix + " " + errStr
				}
				return prefix + " browser failed"
			}
			browserOp := strings.TrimPrefix(firstNonEmpty(strings.TrimSpace(d["browserOp"]), op), "browser.")
			title := strings.TrimSpace(d["title"])
			count := firstNonEmpty(strings.TrimSpace(d["items"]), strings.TrimSpace(d["count"]))
			switch browserOp {
			case "navigate", "back", "forward", "reload":
				if title != "" {
					return prefix + " navigated " + strconv.Quote(singleLinePreview(title, 80))
				}
				return prefix + " navigated"
			case "extract", "extract_links", "tab_list":
				if count != "" {
					return prefix + " extracted " + count + " items"
				}
				return prefix + " extracted"
			case "click":
				return prefix + " clicked"
			case "type":
				return prefix + " typed"
			case "screenshot", "pdf":
				return prefix + " captured"
			case "start":
				return prefix + " browser started"
			case "close":
				return prefix + " browser closed"
			default:
				if browserOp != "" {
					return prefix + " " + strings.ReplaceAll(browserOp, "_", " ")
				}
				return prefix + " browser ok"
			}
		}
		if errStr != "" && ok != "true" {
			return prefix + " " + errStr
		}
		return prefix + " ok"
	}
}

func isSharedOpRequestTitleOp(op string) bool {
	switch strings.TrimSpace(op) {
	case "fs_list", "fs_read", "fs_search", "fs_write", "fs_append", "fs_edit", "fs_patch", "shell_exec", "http_fetch", "trace_run", "agent_spawn", "task_create":
		return true
	default:
		return false
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

func firstNonEmpty(vs ...string) string {
	for _, v := range vs {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func compactKV(m map[string]string, ordered []string) string {
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
			parts = append(parts, k+"="+v)
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
				parts = append(parts, k+"="+v)
			}
		}
	}

	return strings.Join(parts, " ")
}
