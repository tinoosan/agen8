package agent

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
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
	type hunk struct {
		oldStart int
		oldCount int
		newStart int
		newCount int
		lines    []string
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
				return "", fmt.Errorf("invalid hunk header: %q", ln)
			}
			os, oc, err := parseRange(fields[0])
			if err != nil {
				return "", fmt.Errorf("invalid old range in hunk header %q: %w", ln, err)
			}
			ns, nc, err := parseRange(fields[1])
			if err != nil {
				return "", fmt.Errorf("invalid new range in hunk header %q: %w", ln, err)
			}
			hunks = append(hunks, hunk{oldStart: os, oldCount: oc, newStart: ns, newCount: nc})
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
	if len(hunks) == 0 {
		return "", fmt.Errorf("no hunks found in patch")
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
		idx := (hk.oldStart - 1) + offset
		if idx < 0 {
			idx = 0
		}
		if idx > len(outLines) {
			return "", fmt.Errorf("hunk out of range (oldStart=%d) for file with %d lines", hk.oldStart, len(outLines))
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
					return "", fmt.Errorf("patch did not apply cleanly (context mismatch)")
				}
				newPart = append(newPart, content)
				suffixStart++
				consumed++
			case '-':
				if suffixStart >= len(outLines) || outLines[suffixStart] != content {
					return "", fmt.Errorf("patch did not apply cleanly (delete mismatch)")
				}
				suffixStart++
				consumed++
			case '+':
				newPart = append(newPart, content)
			}
		}
		if hk.oldCount != 0 && consumed != hk.oldCount {
			return "", fmt.Errorf("patch did not apply cleanly (hunk expected to consume %d lines, consumed %d)", hk.oldCount, consumed)
		}
		outLines = append(prefix, append(newPart, outLines[suffixStart:]...)...)
		offset += hk.newCount - hk.oldCount
	}

	out := strings.Join(outLines, "\n")
	if hadFinalNL {
		out += "\n"
	}
	return out, nil
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
