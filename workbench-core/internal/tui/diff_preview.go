package tui

import (
	"strings"

	"github.com/pmezard/go-difflib/difflib"
	"github.com/sourcegraph/go-diff/diff"
)

const (
	maxDiffLines = 260
)

// buildFileChangePreview returns markdown content for a file change block body.
// It also returns whether the preview was truncated.
func buildFileChangePreview(op, path, before, after string, hadBefore bool, afterTruncated bool, patchPreview string, patchTruncated bool, patchRedacted bool) (md string, truncated bool, added int, deleted int) {
	op = strings.TrimSpace(op)
	path = strings.TrimSpace(path)

	// Prefer a host-provided diff preview (already a diff) when available.
	// - fs.patch: host previews the patch itself
	// - fs.write/fs.append/fs.edit: host may provide a diff to avoid client-side races
	if strings.TrimSpace(patchPreview) != "" && !patchRedacted {
		// Stats should reflect the full diff (not the capped preview).
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
		return "```diff\n(no changes)\n```", false, 0, 0
	}

	// Stats should reflect the full diff (not the capped preview).
	added, deleted = diffStat(diffText)
	diffText, tr := capLines(diffText, maxDiffLines)
	truncated = tr || afterTruncated
	return "```diff\n" + strings.TrimRight(diffText, "\n") + "\n```", truncated, added, deleted
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
	// Prefer ParseFileDiff for "bare" unified diffs (---/+++/@@) without a leading
	// `diff --git` header. This is the format produced by our difflib renderer.
	if fd, err := diff.ParseFileDiff([]byte(unified)); err == nil && fd != nil {
		st := fd.Stat()
		// go-diff treats adjacent '-' + '+' as a "Changed" line (replacement).
		// For our UI, we want Cursor-like (+A -D) counts where a replacement counts
		// as one deletion and one addition.
		return int(st.Added + st.Changed), int(st.Deleted + st.Changed)
	}
	// Fallback: multi-file parse (git-style diffs).
	if fds, err := diff.ParseMultiFileDiff([]byte(unified)); err == nil && len(fds) != 0 {
		for _, fd := range fds {
			st := fd.Stat()
			added += int(st.Added + st.Changed)
			deleted += int(st.Deleted + st.Changed)
		}
		return added, deleted
	}
	// Fallback (best-effort): count line prefixes in the unified diff text.
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
