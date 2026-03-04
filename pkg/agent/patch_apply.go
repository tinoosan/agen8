package agent

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/tinoosan/agen8/pkg/types"
)

// ApplyStructuredEdits applies structured edits (fs_edit) to the provided text.
func ApplyStructuredEdits(before string, input json.RawMessage) (string, error) {
	var in struct {
		Edits []struct {
			Old        string `json:"old"`
			New        string `json:"new"`
			Occurrence int    `json:"occurrence"`
		} `json:"edits"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("invalid input JSON: %w", err)
	}
	if len(in.Edits) == 0 {
		return "", fmt.Errorf("input.edits must be non-empty")
	}

	after := before
	for i, e := range in.Edits {
		if strings.TrimSpace(e.Old) == "" {
			return "", fmt.Errorf("edit[%d].old must be non-empty", i)
		}
		if e.Occurrence <= 0 {
			return "", fmt.Errorf("edit[%d].occurrence must be >= 1", i)
		}
		var err error
		after, err = replaceNth(after, e.Old, e.New, e.Occurrence)
		if err != nil {
			return "", fmt.Errorf("edit[%d] failed: %w", i, err)
		}
	}
	return after, nil
}

// ApplyUnifiedDiffStrict applies a unified diff patch (fs_patch) with no fuzz.
func ApplyUnifiedDiffStrict(oldText string, patch string) (string, error) {
	after, _, err := ApplyUnifiedDiffWithDiagnostics(oldText, patch, false, false)
	return after, err
}

// ApplyUnifiedDiffWithDiagnostics applies a unified diff patch and reports structured diagnostics.
func ApplyUnifiedDiffWithDiagnostics(oldText string, patch string, dryRun bool, verbose bool) (string, types.PatchDiagnostics, error) {
	type hunk struct {
		oldStart int
		oldCount int
		newStart int
		newCount int
		header   string
		lines    []string
	}

	diag := types.PatchDiagnostics{Mode: "apply"}
	if dryRun {
		diag.Mode = "dry_run"
	}

	parseRange := func(s string) (start, count int, err error) {
		s = strings.TrimSpace(s)
		if s == "" {
			return 0, 0, fmt.Errorf("empty range")
		}
		parts := strings.SplitN(s, ",", 2)
		startStr := strings.TrimLeft(parts[0], "+-")
		start, err = strconv.Atoi(startStr)
		if err != nil {
			return 0, 0, err
		}
		count = 1
		if len(parts) == 2 {
			count, err = strconv.Atoi(parts[1])
			if err != nil {
				return 0, 0, err
			}
		}
		return start, count, nil
	}

	patch = strings.ReplaceAll(patch, "\r\n", "\n")
	lines := strings.Split(patch, "\n")
	hunks := make([]hunk, 0, 8)
	var cur *hunk
	for _, ln := range lines {
		if strings.HasPrefix(ln, "@@") {
			clean := strings.TrimPrefix(ln, "@@")
			if idx := strings.Index(clean, "@@"); idx != -1 {
				clean = clean[:idx]
			}
			inner := strings.TrimSpace(clean)

			fields := strings.Fields(inner)
			if len(fields) < 2 {
				diag.FailureReason = "invalid_hunk_header"
				diag.FailedHunk = len(hunks) + 1
				diag.HunkHeader = ln
				diag.Suggestion = "Ensure hunk headers use unified diff format like @@ -start,count +start,count @@."
				return "", diag, fmt.Errorf("patch did not apply cleanly (invalid hunk header)")
			}
			os, oc, err := parseRange(fields[0])
			if err != nil {
				diag.FailureReason = "invalid_hunk_header"
				diag.FailedHunk = len(hunks) + 1
				diag.HunkHeader = ln
				diag.Suggestion = "Check the old-range segment in the hunk header."
				return "", diag, fmt.Errorf("patch did not apply cleanly (invalid hunk header)")
			}
			ns, nc, err := parseRange(fields[1])
			if err != nil {
				diag.FailureReason = "invalid_hunk_header"
				diag.FailedHunk = len(hunks) + 1
				diag.HunkHeader = ln
				diag.Suggestion = "Check the new-range segment in the hunk header."
				return "", diag, fmt.Errorf("patch did not apply cleanly (invalid hunk header)")
			}
			hunks = append(hunks, hunk{
				oldStart: os,
				oldCount: oc,
				newStart: ns,
				newCount: nc,
				header:   ln,
			})
			cur = &hunks[len(hunks)-1]
			continue
		}
		if cur == nil {
			continue
		}
		if ln == `\ No newline at end of file` {
			continue
		}
		if ln == "" {
			continue
		}
		pfx := ln[0]
		if pfx != ' ' && pfx != '+' && pfx != '-' {
			continue
		}
		cur.lines = append(cur.lines, ln)
	}
	diag.HunksTotal = len(hunks)
	if len(hunks) == 0 {
		diag.FailureReason = "no_hunks"
		diag.Suggestion = "Provide a unified diff with at least one @@ ... @@ hunk."
		return "", diag, fmt.Errorf("patch did not apply cleanly (no hunks found)")
	}

	oldText = strings.ReplaceAll(oldText, "\r\n", "\n")
	hadFinalNL := strings.HasSuffix(oldText, "\n")
	if hadFinalNL {
		oldText = strings.TrimSuffix(oldText, "\n")
	}
	oldLines := []string{}
	if strings.TrimSpace(oldText) != "" || oldText != "" {
		oldLines = strings.Split(oldText, "\n")
	}

	outLines := oldLines
	offset := 0
	for _, hk := range hunks {
		hunkIndex := diag.HunksApplied + 1
		idx := (hk.oldStart - 1) + offset
		if idx < 0 {
			idx = 0
		}
		if idx > len(outLines) {
			diag.FailureReason = "hunk_out_of_range"
			diag.FailedHunk = hunkIndex
			diag.HunkHeader = hk.header
			diag.TargetLine = hk.oldStart
			diag.Suggestion = "Re-read the file and regenerate patch hunks using current line numbers."
			return "", diag, fmt.Errorf("patch did not apply cleanly (hunk out of range)")
		}
		prefix := append([]string(nil), outLines[:idx]...)
		suffixStart := idx
		consumed := 0
		newPart := make([]string, 0, hk.newCount+4)
		for _, pl := range hk.lines {
			if len(pl) == 0 {
				continue
			}
			tag := pl[0]
			content := pl[1:]
			switch tag {
			case ' ':
				if suffixStart >= len(outLines) || outLines[suffixStart] != content {
					diag.FailureReason = "context_mismatch"
					diag.FailedHunk = hunkIndex
					diag.HunkHeader = hk.header
					diag.TargetLine = suffixStart + 1
					diag.ExpectedContext = []string{content}
					if suffixStart >= len(outLines) {
						diag.ActualContext = []string{}
					} else {
						diag.ActualContext = []string{outLines[suffixStart]}
					}
					if verbose {
						diag.ExpectedContext = expectedContextWindow(hk.lines, pl)
						diag.ActualContext = actualContextWindow(outLines, suffixStart)
					}
					diag.Suggestion = "Re-read the target file and regenerate the patch using exact current context lines."
					return "", diag, fmt.Errorf("patch did not apply cleanly (context mismatch)")
				}
				newPart = append(newPart, content)
				suffixStart++
				consumed++
			case '-':
				if suffixStart >= len(outLines) || outLines[suffixStart] != content {
					diag.FailureReason = "delete_mismatch"
					diag.FailedHunk = hunkIndex
					diag.HunkHeader = hk.header
					diag.TargetLine = suffixStart + 1
					diag.ExpectedContext = []string{content}
					if suffixStart >= len(outLines) {
						diag.ActualContext = []string{}
					} else {
						diag.ActualContext = []string{outLines[suffixStart]}
					}
					if verbose {
						diag.ExpectedContext = expectedContextWindow(hk.lines, pl)
						diag.ActualContext = actualContextWindow(outLines, suffixStart)
					}
					diag.Suggestion = "Ensure lines marked with '-' exactly match the current file before applying."
					return "", diag, fmt.Errorf("patch did not apply cleanly (delete mismatch)")
				}
				suffixStart++
				consumed++
			case '+':
				newPart = append(newPart, content)
			}
		}
		if hk.oldCount != 0 && consumed != hk.oldCount {
			diag.FailureReason = "consumed_mismatch"
			diag.FailedHunk = hunkIndex
			diag.HunkHeader = hk.header
			diag.TargetLine = hk.oldStart
			diag.Suggestion = "Verify old-range counts in the hunk header and ensure context/delete lines are complete."
			return "", diag, fmt.Errorf("patch did not apply cleanly (hunk expected to consume %d lines, consumed %d)", hk.oldCount, consumed)
		}
		outLines = append(prefix, append(newPart, outLines[suffixStart:]...)...)
		offset += hk.newCount - hk.oldCount
		diag.HunksApplied++
	}

	out := strings.Join(outLines, "\n")
	if hadFinalNL {
		out += "\n"
	}
	return out, diag, nil
}

func expectedContextWindow(hunkLines []string, failedLine string) []string {
	if len(hunkLines) == 0 {
		return nil
	}
	start := 0
	for i, ln := range hunkLines {
		if ln == failedLine {
			start = i
			break
		}
	}
	out := make([]string, 0, 3)
	for i := start; i < len(hunkLines) && len(out) < 3; i++ {
		ln := hunkLines[i]
		if len(ln) == 0 {
			continue
		}
		switch ln[0] {
		case ' ', '-':
			out = append(out, ln[1:])
		}
	}
	if len(out) == 0 && len(failedLine) > 0 {
		return []string{failedLine[1:]}
	}
	return out
}

func actualContextWindow(lines []string, index int) []string {
	if len(lines) == 0 {
		return nil
	}
	if index < 0 {
		index = 0
	}
	if index > len(lines)-1 {
		index = len(lines) - 1
	}
	end := index + 3
	if end > len(lines) {
		end = len(lines)
	}
	out := make([]string, 0, end-index)
	for i := index; i < end; i++ {
		out = append(out, lines[i])
	}
	return out
}

func replaceNth(s, old, new string, occurrence int) (string, error) {
	if old == "" {
		return s, fmt.Errorf("old must be non-empty")
	}
	if occurrence <= 0 {
		return s, fmt.Errorf("occurrence must be >= 1")
	}
	idx := 0
	for i := 1; i <= occurrence; i++ {
		pos := strings.Index(s[idx:], old)
		if pos < 0 {
			return s, fmt.Errorf("old not found at occurrence %d", occurrence)
		}
		pos = idx + pos
		if i == occurrence {
			return s[:pos] + new + s[pos+len(old):], nil
		}
		idx = pos + len(old)
	}
	return s, fmt.Errorf("old not found at occurrence %d", occurrence)
}
