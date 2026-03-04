package tui

import (
	"fmt"
	"strings"

	"github.com/hexops/gotextdiff"
	"github.com/hexops/gotextdiff/myers"
	"github.com/hexops/gotextdiff/span"
)

const (
	maxDiffLines = 260
)

// DiffLineKind classifies a parsed diff line without exposing gotextdiff types.
type DiffLineKind int

const (
	DiffLineContext DiffLineKind = iota
	DiffLineInsert
	DiffLineDelete
	DiffLineSep // hunk separator between hunks
)

// DiffLine is a single parsed line from a unified diff.
type DiffLine struct {
	Kind    DiffLineKind
	LineNo  int
	Content string
}

// ParseDiffLines parses a unified diff string into typed lines for custom rendering.
// Returns nil if the input cannot be parsed as a valid unified diff.
func ParseDiffLines(unified string) (lines []DiffLine, added int, deleted int) {
	internal, a, d := parseUnifiedNumberedLines(unified)
	if len(internal) == 0 {
		return nil, 0, 0
	}
	result := make([]DiffLine, 0, len(internal))
	for _, l := range internal {
		var kind DiffLineKind
		if l.meta {
			kind = DiffLineSep
		} else {
			switch l.kind {
			case gotextdiff.Insert:
				kind = DiffLineInsert
			case gotextdiff.Delete:
				kind = DiffLineDelete
			default:
				kind = DiffLineContext
			}
		}
		result = append(result, DiffLine{Kind: kind, LineNo: l.lineNo, Content: l.content})
	}
	return result, a, d
}

// DiffStat returns added/deleted line counts from a unified diff string.
func DiffStat(unified string) (added, deleted int) {
	return diffStat(unified)
}

// buildFileChangePreview returns markdown content for a file change block body.
// It also returns whether the preview was truncated.
func buildFileChangePreview(op, path, before, after string, hadBefore bool, afterTruncated bool, patchPreview string, patchTruncated bool, patchRedacted bool) (md string, truncated bool, added int, deleted int) {
	op = strings.TrimSpace(op)
	path = strings.TrimSpace(path)

	// Prefer a host-provided diff preview (already a diff) when available.
	// - fs_patch: host previews the patch itself
	// - fs_write/fs_append/fs_edit: host may provide a diff to avoid client-side races
	if strings.TrimSpace(patchPreview) != "" && !patchRedacted {
		preview, statsAdded, statsDeleted, ok := renderUnifiedPreview(patchPreview)
		if ok {
			added, deleted = statsAdded, statsDeleted
			body, tr := capLines(preview, maxDiffLines)
			truncated = tr || patchTruncated
			return wrapDiffFence(body), truncated, added, deleted
		}
		// Fallback to the raw preview if parsing fails.
		added, deleted = diffStat(patchPreview)
		body, tr := capLines(patchPreview, maxDiffLines)
		truncated = tr || patchTruncated
		return "```diff\n" + strings.TrimRight(body, "\n") + "\n```", truncated, added, deleted
	}

	// Otherwise, compute a unified diff from before->after.
	// If before wasn't known, treat it as empty (Created).
	if !hadBefore {
		before = ""
	}

	fromFile := "a" + path
	toFile := "b" + path
	if !hadBefore {
		fromFile = "/dev/null"
		toFile = "b" + path
	}

	before = normalizeNewlines(before)
	after = normalizeNewlines(after)
	edits := myers.ComputeEdits(span.URIFromPath(path), before, after)
	ud := gotextdiff.ToUnified(fromFile, toFile, before, edits)
	preview, statsAdded, statsDeleted := renderUnified(ud)
	if strings.TrimSpace(preview) == "" {
		// Still render a diff block so UX is consistent even when the write/patch
		// doesn't change content.
		return "```text\n(no changes)\n```", false, 0, 0
	}
	added, deleted = statsAdded, statsDeleted
	preview, tr := capLines(preview, maxDiffLines)
	truncated = tr || afterTruncated
	return wrapDiffFence(preview), truncated, added, deleted
}

func normalizeNewlines(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return s
}

func diffStat(unified string) (added int, deleted int) {
	unified = strings.TrimSpace(unified)
	if unified == "" {
		return 0, 0
	}
	// Best-effort: count line prefixes in the unified diff text.
	for _, ln := range strings.Split(strings.ReplaceAll(unified, "\r\n", "\n"), "\n") {
		if strings.HasPrefix(ln, "+++") || strings.HasPrefix(ln, "---") {
			continue
		}
		if strings.HasPrefix(ln, "+") {
			added++
			continue
		}
		if strings.HasPrefix(ln, "-") {
			deleted++
			continue
		}
	}
	return added, deleted
}

type numberedLine struct {
	kind    gotextdiff.OpKind
	lineNo  int
	content string
	meta    bool
}

func renderUnified(u gotextdiff.Unified) (string, int, int) {
	lines, added, deleted := numberedLinesFromUnified(u)
	if len(lines) == 0 {
		return "", 0, 0
	}
	return renderNumberedLines(lines), added, deleted
}

func numberedLinesFromUnified(u gotextdiff.Unified) ([]numberedLine, int, int) {
	var lines []numberedLine
	added := 0
	deleted := 0
	for hIdx, h := range u.Hunks {
		if hIdx > 0 {
			lines = append(lines, numberedLine{meta: true})
		}
		fromLine := h.FromLine
		toLine := h.ToLine
		for _, ln := range h.Lines {
			entry := numberedLine{kind: ln.Kind, content: ln.Content}
			switch ln.Kind {
			case gotextdiff.Delete:
				entry.lineNo = fromLine
				fromLine++
				deleted++
			case gotextdiff.Insert:
				entry.lineNo = toLine
				toLine++
				added++
			default:
				entry.lineNo = toLine
				fromLine++
				toLine++
			}
			lines = append(lines, entry)
		}
	}
	return lines, added, deleted
}

func renderUnifiedPreview(unified string) (string, int, int, bool) {
	lines, added, deleted := parseUnifiedNumberedLines(unified)
	if len(lines) == 0 {
		return "", 0, 0, false
	}
	return renderNumberedLines(lines), added, deleted, true
}

func parseUnifiedNumberedLines(unified string) ([]numberedLine, int, int) {
	var lines []numberedLine
	added := 0
	deleted := 0
	fromLine := 0
	toLine := 0
	inHunk := false
	for _, raw := range strings.Split(strings.ReplaceAll(unified, "\r\n", "\n"), "\n") {
		line := strings.TrimRight(raw, "\r")
		if strings.HasPrefix(line, "@@") {
			startFrom, startTo, ok := parseHunkHeader(line)
			if !ok {
				inHunk = false
				continue
			}
			if len(lines) > 0 {
				lines = append(lines, numberedLine{meta: true})
			}
			fromLine = startFrom
			toLine = startTo
			inHunk = true
			continue
		}
		if !inHunk {
			continue
		}
		if line == "" {
			continue
		}
		prefix := line[0]
		content := ""
		if len(line) > 1 {
			content = line[1:]
		}
		switch prefix {
		case '-':
			lines = append(lines, numberedLine{kind: gotextdiff.Delete, lineNo: fromLine, content: content})
			fromLine++
			deleted++
		case '+':
			lines = append(lines, numberedLine{kind: gotextdiff.Insert, lineNo: toLine, content: content})
			toLine++
			added++
		case ' ':
			lines = append(lines, numberedLine{kind: gotextdiff.Equal, lineNo: toLine, content: content})
			fromLine++
			toLine++
		default:
			// Ignore unrecognized lines inside hunks.
		}
	}
	return lines, added, deleted
}

func parseHunkHeader(line string) (fromLine int, toLine int, ok bool) {
	fields := strings.Fields(line)
	if len(fields) < 3 {
		return 0, 0, false
	}
	if fields[0] != "@@" {
		return 0, 0, false
	}
	fromSpec := strings.TrimPrefix(fields[1], "-")
	toSpec := strings.TrimPrefix(fields[2], "+")
	fromLine, okFrom := parseHunkLineNumber(fromSpec)
	toLine, okTo := parseHunkLineNumber(toSpec)
	if !okFrom || !okTo {
		return 0, 0, false
	}
	return fromLine, toLine, true
}

func parseHunkLineNumber(spec string) (int, bool) {
	if spec == "" {
		return 0, false
	}
	if idx := strings.IndexByte(spec, ','); idx >= 0 {
		spec = spec[:idx]
	}
	return parsePositiveInt(spec)
}

func parsePositiveInt(s string) (int, bool) {
	if s == "" {
		return 0, false
	}
	n := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '9' {
			return 0, false
		}
		n = n*10 + int(c-'0')
	}
	return n, true
}

func renderNumberedLines(lines []numberedLine) string {
	maxLine := 0
	for _, ln := range lines {
		if ln.lineNo > maxLine {
			maxLine = ln.lineNo
		}
	}
	width := 1
	if maxLine > 9 {
		width = 1
		for n := maxLine; n > 0; n /= 10 {
			width++
		}
		width--
	}
	var b strings.Builder
	for i, ln := range lines {
		if ln.meta {
			b.WriteString(ln.content)
		} else {
			prefix := " "
			switch ln.kind {
			case gotextdiff.Delete:
				prefix = "-"
			case gotextdiff.Insert:
				prefix = "+"
			default:
				prefix = " "
			}
			content := strings.TrimSuffix(ln.content, "\n")
			fmt.Fprintf(&b, "%s%*d | %s", prefix, width, ln.lineNo, content)
		}
		if i != len(lines)-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func wrapDiffFence(body string) string {
	body = strings.TrimRight(body, "\n")
	if strings.TrimSpace(body) == "" {
		return "```text\n(no changes)\n```"
	}
	return "```diff\n" + body + "\n```"
}

func capLines(s string, max int) (string, bool) {
	if max <= 0 {
		return s, false
	}
	s = strings.ReplaceAll(s, "\r\n", "\n")
	lines := strings.Split(s, "\n")
	if len(lines) <= max {
		return s, false
	}
	return strings.Join(lines[:max], "\n") + "\n…", true
}
