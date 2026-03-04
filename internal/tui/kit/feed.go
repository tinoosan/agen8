package kit

import (
	"fmt"
	"strings"
	"time"
)

// StatusDot returns a colored "●" dot for the given status string.
func StatusDot(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "ok", "done", "completed", "succeeded", "connected":
		return StyleOK.Render("●")
	case "error", "failed", "canceled", "cancelled", "disconnected":
		return StyleErr.Render("●")
	case "pending", "queued", "running":
		return StylePending.Render("●")
	default:
		return StyleDim.Render("●")
	}
}

// Spinner returns the spinner frame string for the given frame index.
func Spinner(frame int) string {
	return SpinnerFrames[frame%len(SpinnerFrames)]
}

// RelativeAge formats a duration as a short human string ("2s", "1m", "3h", "2d").
func RelativeAge(d time.Duration) string {
	if d < 0 {
		return "now"
	}
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}

// VerbFromKind returns a short display verb for an operation kind.
// e.g. "fs_read" → "Read", "shell_exec" → "Bash", "agent_spawn" → "Spawn"
func VerbFromKind(kind string) string {
	k := strings.ToLower(strings.TrimSpace(kind))
	switch k {
	case "fs_read":
		return "Read"
	case "fs_list":
		return "List"
	case "fs_stat":
		return "Stat"
	case "fs_write":
		return "Write"
	case "fs_append":
		return "Append"
	case "fs_edit":
		return "Edit"
	case "fs_patch":
		return "Patch"
	case "fs_search":
		return "Search"
	case "shell_exec":
		return "Bash"
	case "http_fetch":
		return "Fetch"
	case "browser":
		return "Browse"
	case "code_exec":
		return "Python"
	case "email":
		return "Email"
	case "agent_spawn":
		return "Spawn"
	case "task_create":
		return "Create task"
	case "task_review":
		return "Review task"
	case "trace_run":
		return "Trace"
	case "llm.web.search":
		return "Web search"
	}
	if k != "" {
		return kind
	}
	return "op"
}

// RenderHeaderLine renders a single-line header with left content, fill gap, and right content.
// width is total terminal width; padding is horizontal padding applied on each side.
func RenderHeaderLine(left, right string, width, padding int) string {
	leftW := ansiStringWidth(left)
	rightW := ansiStringWidth(right)
	avail := width - 2*padding
	gap := avail - leftW - rightW
	if gap < 1 {
		gap = 1
	}
	line := left + strings.Repeat(" ", gap) + right
	return line
}

// RenderTreeLine wraps content with an ASCII tree branch prefix.
// isLast controls whether "└─" or "├─" is used.
func RenderTreeLine(content string, isLast bool) string {
	prefix := "├─ "
	if isLast {
		prefix = "└─ "
	}
	return StyleDim.Render(prefix) + content
}

// RenderDiffLine renders a single diff line with +/- prefix and colour.
// Lines starting with '+' are green; '-' are red; others are dim.
func RenderDiffLine(line string, lineWidth int) string {
	if strings.HasPrefix(line, "+") {
		return StyleOK.Render(TruncateRight(line, lineWidth))
	}
	if strings.HasPrefix(line, "-") {
		return StyleErr.Render(TruncateRight(line, lineWidth))
	}
	return StyleDim.Render(TruncateRight(line, lineWidth))
}

// ansiStringWidth returns the display width of a string, stripping ANSI escapes.
func ansiStringWidth(s string) int {
	b := make([]rune, 0, len(s))
	inEsc := false
	for _, r := range s {
		if r == '\x1b' {
			inEsc = true
			continue
		}
		if inEsc {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEsc = false
			}
			continue
		}
		b = append(b, r)
	}
	return len(b)
}
