package agent

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/tinoosan/workbench-core/internal/tools"
	"github.com/tinoosan/workbench-core/internal/types"
	"github.com/tinoosan/workbench-core/internal/vfs"
)

// HostOpExecutor is a tiny "host primitive" dispatcher for demos/tests.
//
// This is not the final host API; it is a concrete reference for the agent-facing
// request/response flow:
//   - fs.list/fs.read/fs.write/fs.append are always available
//   - tool.run executes via tools.Runner and returns a ToolResponse
type HostOpExecutor struct {
	FS     *vfs.FS
	Runner *tools.Runner

	DefaultMaxBytes int

	// MaxReadBytes caps fs.read payload size returned to the model.
	//
	// This protects the model context window and cost from accidental "read the whole file"
	// requests (e.g. reading large HTML pages, logs, or binary blobs).
	//
	// If zero, no explicit cap is applied beyond DefaultMaxBytes / req.MaxBytes behavior.
	MaxReadBytes int
}

func (x *HostOpExecutor) Exec(ctx context.Context, req types.HostOpRequest) types.HostOpResponse {
	if x == nil || x.FS == nil {
		return types.HostOpResponse{Op: req.Op, Ok: false, Error: "host executor missing FS"}
	}
	if err := req.Validate(); err != nil {
		return types.HostOpResponse{Op: req.Op, Ok: false, Error: err.Error()}
	}

	switch req.Op {
	case types.HostOpFSList:
		entries, err := x.FS.List(req.Path)
		if err != nil {
			return types.HostOpResponse{Op: req.Op, Ok: false, Error: err.Error()}
		}
		out := make([]string, 0, len(entries))
		for _, e := range entries {
			out = append(out, e.Path)
		}
		return types.HostOpResponse{Op: req.Op, Ok: true, Entries: out}

	case types.HostOpFSRead:
		b, err := x.FS.Read(req.Path)
		if err != nil {
			return types.HostOpResponse{Op: req.Op, Ok: false, Error: err.Error()}
		}
		maxBytes := req.MaxBytes
		if maxBytes == 0 {
			maxBytes = x.DefaultMaxBytes
		}
		if maxBytes <= 0 {
			maxBytes = 4096
		}
		if x.MaxReadBytes > 0 && maxBytes > x.MaxReadBytes {
			maxBytes = x.MaxReadBytes
		}
		text, b64, truncated := encodeReadPayload(b, maxBytes)
		return types.HostOpResponse{
			Op:        req.Op,
			Ok:        true,
			BytesLen:  len(b),
			Text:      text,
			BytesB64:  b64,
			Truncated: truncated,
		}

	case types.HostOpFSWrite:
		if err := x.FS.Write(req.Path, []byte(req.Text)); err != nil {
			return types.HostOpResponse{Op: req.Op, Ok: false, Error: err.Error()}
		}
		return types.HostOpResponse{Op: req.Op, Ok: true}

	case types.HostOpFSAppend:
		if err := x.FS.Append(req.Path, []byte(req.Text)); err != nil {
			return types.HostOpResponse{Op: req.Op, Ok: false, Error: err.Error()}
		}
		return types.HostOpResponse{Op: req.Op, Ok: true}

	case types.HostOpFSEdit:
		// Structured edit apply (exact-match semantics; minimal tokens).
		beforeBytes, err := x.FS.Read(req.Path)
		if err != nil {
			// Treat missing as empty (edits will fail if old text isn't found).
			if errors.Is(err, fs.ErrNotExist) || strings.Contains(err.Error(), "not found") {
				beforeBytes = nil
			} else {
				return types.HostOpResponse{Op: req.Op, Ok: false, Error: err.Error()}
			}
		}

		var in struct {
			Edits []struct {
				Old        string `json:"old"`
				New        string `json:"new"`
				Occurrence int    `json:"occurrence"`
			} `json:"edits"`
		}
		if err := json.Unmarshal(req.Input, &in); err != nil {
			return types.HostOpResponse{Op: req.Op, Ok: false, Error: "invalid input JSON: " + err.Error()}
		}
		if len(in.Edits) == 0 {
			return types.HostOpResponse{Op: req.Op, Ok: false, Error: "input.edits must be non-empty"}
		}

		before := string(beforeBytes)
		after := before
		for i, e := range in.Edits {
			if strings.TrimSpace(e.Old) == "" {
				return types.HostOpResponse{Op: req.Op, Ok: false, Error: fmt.Sprintf("edit[%d].old must be non-empty", i)}
			}
			if e.Occurrence <= 0 {
				return types.HostOpResponse{Op: req.Op, Ok: false, Error: fmt.Sprintf("edit[%d].occurrence must be >= 1", i)}
			}
			var err error
			after, err = replaceNth(after, e.Old, e.New, e.Occurrence)
			if err != nil {
				return types.HostOpResponse{Op: req.Op, Ok: false, Error: fmt.Sprintf("edit[%d] failed: %v", i, err)}
			}
		}

		if err := x.FS.Write(req.Path, []byte(after)); err != nil {
			return types.HostOpResponse{Op: req.Op, Ok: false, Error: err.Error()}
		}
		return types.HostOpResponse{Op: req.Op, Ok: true}

	case types.HostOpFSPatch:
		// Strict patch apply (must apply cleanly).
		beforeBytes, err := x.FS.Read(req.Path)
		if err != nil {
			// Treat missing as empty so patches can create new files, but surface other errors.
			if errors.Is(err, fs.ErrNotExist) || strings.Contains(err.Error(), "not found") {
				beforeBytes = nil
			} else {
				return types.HostOpResponse{Op: req.Op, Ok: false, Error: err.Error()}
			}
		}
		after, err := applyUnifiedDiffStrict(string(beforeBytes), req.Text)
		if err != nil {
			return types.HostOpResponse{Op: req.Op, Ok: false, Error: err.Error()}
		}
		if err := x.FS.Write(req.Path, []byte(after)); err != nil {
			return types.HostOpResponse{Op: req.Op, Ok: false, Error: err.Error()}
		}
		return types.HostOpResponse{Op: req.Op, Ok: true}

	case types.HostOpToolRun:
		if x.Runner == nil {
			return types.HostOpResponse{Op: req.Op, Ok: false, Error: "host executor missing Runner"}
		}
		resp, err := x.Runner.Run(ctx, req.ToolID, req.ActionID, req.Input, req.TimeoutMs)
		if err != nil {
			return types.HostOpResponse{Op: req.Op, Ok: false, Error: err.Error()}
		}
		return types.HostOpResponse{Op: req.Op, Ok: true, ToolResponse: &resp}

	default:
		return types.HostOpResponse{Op: req.Op, Ok: false, Error: fmt.Sprintf("unknown op %q", req.Op)}
	}
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
		// Move past this match (non-overlapping occurrences).
		idx = pos + len(old)
	}
	return s, fmt.Errorf("old not found at occurrence %d", occurrence)
}

// applyUnifiedDiffStrict applies a unified diff patch to oldText with no fuzz.
// It returns an error if the patch does not apply cleanly.
//
// This is a minimal strict applier intended for fs.patch.
func applyUnifiedDiffStrict(oldText string, patch string) (string, error) {
	type hunk struct {
		oldStart int
		oldCount int
		newStart int
		newCount int
		lines    []string // raw hunk lines including prefix char
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
			// @@ -a,b +c,d @@ optional heading
			// Relaxed parsing: allow missing trailing @@.
			clean := strings.TrimPrefix(ln, "@@")
			if idx := strings.Index(clean, "@@"); idx != -1 {
				clean = clean[:idx]
			}
			inner := strings.TrimSpace(clean)

			// inner like "-1,3 +1,4"
			fields := strings.Fields(inner)
			if len(fields) < 2 {
				return "", fmt.Errorf("invalid hunk header: %q", ln)
			}
			oldR := fields[0]
			newR := fields[1]
			os, oc, err := parseRange(oldR)
			if err != nil {
				return "", fmt.Errorf("invalid old range in hunk header %q: %w", ln, err)
			}
			ns, nc, err := parseRange(newR)
			if err != nil {
				return "", fmt.Errorf("invalid new range in hunk header %q: %w", ln, err)
			}
			hunks = append(hunks, hunk{oldStart: os, oldCount: oc, newStart: ns, newCount: nc})
			cur = &hunks[len(hunks)-1]
			continue
		}
		if cur == nil {
			// Ignore file headers (---/+++), diff headers, etc.
			continue
		}
		if ln == `\ No newline at end of file` {
			// Ignore marker for now; strict patching is line-based.
			continue
		}
		if ln == "" {
			// Empty line is a valid context/add/del only if prefixed in patch.
			// If it's unprefixed, treat as context mismatch.
			// (This situation happens if patch had a trailing newline; ignore.)
			continue
		}
		pfx := ln[0]
		if pfx != ' ' && pfx != '+' && pfx != '-' {
			// Ignore unexpected.
			continue
		}
		cur.lines = append(cur.lines, ln)
	}
	if len(hunks) == 0 {
		return "", fmt.Errorf("no hunks found in patch")
	}

	// Split old into lines without trailing newline.
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
			// Enforce strict counts when specified.
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

// PrettyJSON is a small helper for demos/logging.
func PrettyJSON(v any) string {
	b, err := types.MarshalPretty(v)
	if err != nil {
		return "<json marshal error: " + err.Error() + ">"
	}
	return string(b)
}

func encodeReadPayload(b []byte, maxBytes int) (text string, bytesB64 string, truncated bool) {
	if maxBytes <= 0 {
		maxBytes = len(b)
	}
	n := len(b)
	if n > maxBytes {
		n = maxBytes
		truncated = true
	}
	head := b[:n]

	// Prefer returning text when valid UTF-8.
	// If we truncated, try trimming bytes until the prefix is valid UTF-8.
	for len(head) > 0 && !utf8.Valid(head) {
		head = head[:len(head)-1]
	}
	if len(head) > 0 && utf8.Valid(head) {
		return string(head), "", truncated
	}

	// Binary or non-UTF8: return base64 so the contract is lossless.
	return "", base64.StdEncoding.EncodeToString(b[:n]), truncated
}

func AgentSay(logf func(string, ...any), exec func(types.HostOpRequest) types.HostOpResponse, req types.HostOpRequest) types.HostOpResponse {
	logf("agent -> host:\n%s", PrettyJSON(req))
	resp := exec(req)
	// Avoid dumping huge raw bytes; HostOpResponse may contain truncated text or base64.
	logf("host -> agent:\n%s", strings.TrimSpace(PrettyJSON(resp)))
	return resp
}
