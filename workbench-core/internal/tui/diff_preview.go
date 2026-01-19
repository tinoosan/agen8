package tui

import (
	"strings"

	"github.com/pmezard/go-difflib/difflib"
)

const (
	maxDiffLines = 260
)

// buildFileChangePreview returns markdown content for a file change block body.
// It also returns whether the preview was truncated.
func buildFileChangePreview(op, path, before, after string, hadBefore bool, afterTruncated bool, patchPreview string, patchTruncated bool, patchRedacted bool) (md string, truncated bool) {
	op = strings.TrimSpace(op)
	path = strings.TrimSpace(path)

	// fs.patch: show the patch itself when available (it is already a diff).
	if op == "fs.patch" && strings.TrimSpace(patchPreview) != "" && !patchRedacted {
		body, tr := capLines(patchPreview, maxDiffLines)
		truncated = tr || patchTruncated
		return "```diff\n" + strings.TrimRight(body, "\n") + "\n```", truncated
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

	ud := difflib.UnifiedDiff{
		A:        difflib.SplitLines(normalizeNewlines(before)),
		B:        difflib.SplitLines(normalizeNewlines(after)),
		FromFile: fromFile,
		ToFile:   toFile,
		Context:  3,
	}
	diffText, _ := difflib.GetUnifiedDiffString(ud)
	diffText = strings.TrimSpace(diffText)
	if diffText == "" {
		// Still render a diff block so UX is consistent even when the write/patch
		// doesn't change content.
		return "```diff\n(no changes)\n```", false
	}

	diffText, tr := capLines(diffText, maxDiffLines)
	truncated = tr || afterTruncated
	return "```diff\n" + strings.TrimRight(diffText, "\n") + "\n```", truncated
}

func normalizeNewlines(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return s
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

