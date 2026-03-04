package kit

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

// KindIcon returns an emoji icon for the given operation kind.
func KindIcon(kind string) string {
	switch {
	case strings.HasPrefix(kind, "fs_"):
		return "📄"
	case kind == "shell_exec":
		return "⚡"
	case kind == "http_fetch":
		return "🌐"
	case kind == "browser" || strings.HasPrefix(kind, "browser."):
		return "🖥"
	case kind == "agent_spawn":
		return "🤖"
	case kind == "code_exec":
		return "🐍"
	case kind == "email":
		return "📧"
	case kind == "task_create":
		return "📋"
	case kind == "trace_run":
		return "🔍"
	case strings.HasPrefix(kind, "ui."):
		return "🖼"
	case strings.HasPrefix(kind, "workdir"):
		return "📂"
	case strings.HasPrefix(kind, "llm."):
		return "🔗"
	default:
		return "⚙"
	}
}

func TruncateRight(s string, maxLen int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.TrimSpace(s)
	if maxLen <= 0 {
		return ""
	}
	if runewidth.StringWidth(s) <= maxLen {
		return s
	}
	return runewidth.Truncate(s, maxLen, "…")
}

func TruncateMiddle(s string, maxLen int) string {
	s = strings.TrimSpace(s)
	if maxLen <= 0 {
		return ""
	}
	if runewidth.StringWidth(s) <= maxLen {
		return s
	}
	if maxLen == 1 {
		return "…"
	}
	leftBudget := (maxLen - 1) / 2
	rightBudget := maxLen - 1 - leftBudget
	left := runewidth.Truncate(s, leftBudget, "")
	right := truncateFromRightWidth(s, rightBudget)
	return left + "…" + right
}

func truncateFromRightWidth(s string, maxWidth int) string {
	if maxWidth <= 0 || s == "" {
		return ""
	}
	runes := []rune(s)
	start := len(runes)
	width := 0
	for i := len(runes) - 1; i >= 0; i-- {
		rw := runewidth.RuneWidth(runes[i])
		if width+rw > maxWidth {
			break
		}
		width += rw
		start = i
	}
	return string(runes[start:])
}

func CloneStyle(s lipgloss.Style) *lipgloss.Style {
	c := s.Copy()
	return &c
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func clampInt(value, minVal, maxVal int) int {
	if value < minVal {
		return minVal
	}
	if value > maxVal {
		return maxVal
	}
	return value
}

// Truncate trims s to fit within max display-width characters (appends "…" if truncated).
func Truncate(s string, maxW int) string {
	if maxW <= 0 {
		return ""
	}
	s = strings.TrimSpace(s)
	if s == "" || runewidth.StringWidth(s) <= maxW {
		return s
	}
	if maxW <= 1 {
		return runewidth.Truncate(s, maxW, "")
	}
	return runewidth.Truncate(s, maxW-1, "") + "…"
}

// Fallback returns v if non-blank, otherwise def.
func Fallback(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}

// ViewportSlice returns a window of visibleLines from content, scrolled near targetIdx.
func ViewportSlice(content string, visibleLines, targetIdx int) string {
	lines := strings.Split(content, "\n")
	if visibleLines <= 0 {
		visibleLines = 1
	}
	if len(lines) <= visibleLines {
		return content
	}
	if targetIdx < 0 {
		targetIdx = 0
	}
	if targetIdx >= len(lines) {
		targetIdx = len(lines) - 1
	}
	start := targetIdx
	end := start + visibleLines
	if end > len(lines) {
		end = len(lines)
		start = max(0, end-visibleLines)
	}
	return strings.Join(lines[start:end], "\n")
}

// MaxPage returns the last valid 0-based page index.
func MaxPage(total, pageSize int) int {
	if pageSize <= 0 {
		return 0
	}
	p := (total + pageSize - 1) / pageSize - 1
	if p < 0 {
		return 0
	}
	return p
}
