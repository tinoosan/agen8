package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

const (
	maxToolRunInputEventBytes = 1024
)

const (
	maxFSWriteTextPreviewBytes = 2000
)

// toolRunInputForEvent returns a small, sanitized JSON string representation of a tool.run input payload
// suitable for emitting in UI events.
//
// Why this exists:
//   - ToolRequest.Input can be arbitrarily large (e.g., formatting a full HTML document).
//   - Some fields are sensitive (e.g., "stdin", "token") and should not be echoed back verbatim.
//   - The UI wants enough structure to render human-friendly action lines (argv/query/paths), without
//     flooding the transcript or inspector.
//
// Behavior:
//   - Best-effort JSON parsing: on parse failure, returns a compact single-line preview.
//   - Redacts large or sensitive string fields (e.g., "text", "stdin", "token").
//   - Produces a compact one-line JSON object and hard-caps its size.
func toolRunInputForEvent(raw json.RawMessage) (sanitized string, truncated bool, originalBytes int) {
	raw = bytes.TrimSpace(raw)
	originalBytes = len(raw)
	if len(raw) == 0 {
		return "", false, 0
	}

	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		s := singleLine(string(raw))
		s2, tr := capBytes(s, maxToolRunInputEventBytes)
		return s2, tr, originalBytes
	}

	out := make(map[string]any, len(obj))
	for k, v := range obj {
		out[k] = redactValue(k, v)
	}

	b, err := json.Marshal(out)
	if err != nil {
		s := singleLine(string(raw))
		s2, tr := capBytes(s, maxToolRunInputEventBytes)
		return s2, tr, originalBytes
	}

	s2, tr := capBytes(singleLine(string(b)), maxToolRunInputEventBytes)
	return s2, tr, originalBytes
}

const maxToolRunOutputPreviewBytes = 1200

// toolRunOutputPreviewForEvent returns a small, human-readable summary of a tool.run output payload.
//
// This is used only for UI events so the Activity details panel can show a preview without reading
// /results/<callId>/response.json.
//
// It is intentionally conservative:
//   - hard-caps size
//   - avoids dumping large structured payloads (e.g., ripgrep matches) in full
func toolRunOutputPreviewForEvent(toolID, actionID string, raw json.RawMessage) string {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return ""
	}

	switch strings.TrimSpace(toolID) {
	case "builtin.shell":
		if strings.TrimSpace(actionID) != "exec" {
			break
		}
		var out struct {
			ExitCode int    `json:"exitCode"`
			Stdout   string `json:"stdout"`
			Stderr   string `json:"stderr"`
		}
		if err := json.Unmarshal(raw, &out); err != nil {
			break
		}
		stdout := strings.TrimSpace(out.Stdout)
		stderr := strings.TrimSpace(out.Stderr)
		s := fmt.Sprintf("exitCode=%d", out.ExitCode)
		if stdout != "" {
			s += " stdout=" + previewText(stdout, 400)
		}
		if stderr != "" {
			s += " stderr=" + previewText(stderr, 400)
		}
		s2, _ := capBytes(singleLine(s), maxToolRunOutputPreviewBytes)
		return s2

	case "builtin.git":
		switch strings.TrimSpace(actionID) {
		case "status":
			var out struct {
				Head      string `json:"head"`
				Staged    []any  `json:"staged"`
				Unstaged  []any  `json:"unstaged"`
				Untracked []any  `json:"untracked"`
				Conflicts []any  `json:"conflicts"`
				ExitCode  int    `json:"exitCode"`
				Stderr    string `json:"stderr"`
				Stdout    string `json:"stdout"`
			}
			if err := json.Unmarshal(raw, &out); err != nil {
				break
			}
			s := fmt.Sprintf("exitCode=%d staged=%d unstaged=%d untracked=%d conflicts=%d", out.ExitCode, len(out.Staged), len(out.Unstaged), len(out.Untracked), len(out.Conflicts))
			if strings.TrimSpace(out.Head) != "" {
				s += " head=" + previewText(out.Head, 80)
			}
			if strings.TrimSpace(out.Stderr) != "" && out.ExitCode != 0 {
				s += " stderr=" + previewText(out.Stderr, 200)
			}
			s2, _ := capBytes(singleLine(s), maxToolRunOutputPreviewBytes)
			return s2
		case "diff":
			var out struct {
				Truncated bool   `json:"truncated"`
				PatchPath string `json:"patchPath"`
				ExitCode  int    `json:"exitCode"`
				Stderr    string `json:"stderr"`
			}
			if err := json.Unmarshal(raw, &out); err != nil {
				break
			}
			s := fmt.Sprintf("exitCode=%d truncated=%t", out.ExitCode, out.Truncated)
			if strings.TrimSpace(out.PatchPath) != "" {
				s += " patchPath=" + previewText(out.PatchPath, 120)
			}
			if strings.TrimSpace(out.Stderr) != "" && out.ExitCode != 0 {
				s += " stderr=" + previewText(out.Stderr, 200)
			}
			s2, _ := capBytes(singleLine(s), maxToolRunOutputPreviewBytes)
			return s2
		case "commit":
			var out struct {
				Committed bool   `json:"committed"`
				Head      string `json:"head"`
				ExitCode  int    `json:"exitCode"`
				Stderr    string `json:"stderr"`
			}
			if err := json.Unmarshal(raw, &out); err != nil {
				break
			}
			s := fmt.Sprintf("exitCode=%d committed=%t", out.ExitCode, out.Committed)
			if strings.TrimSpace(out.Head) != "" {
				s += " head=" + previewText(out.Head, 12)
			}
			if strings.TrimSpace(out.Stderr) != "" && (out.ExitCode != 0 || !out.Committed) {
				s += " stderr=" + previewText(out.Stderr, 200)
			}
			s2, _ := capBytes(singleLine(s), maxToolRunOutputPreviewBytes)
			return s2
		case "add":
			var out struct {
				AddedFiles []any  `json:"addedFiles"`
				ExitCode   int    `json:"exitCode"`
				Stderr     string `json:"stderr"`
			}
			if err := json.Unmarshal(raw, &out); err != nil {
				break
			}
			s := fmt.Sprintf("exitCode=%d added=%d", out.ExitCode, len(out.AddedFiles))
			if strings.TrimSpace(out.Stderr) != "" && out.ExitCode != 0 {
				s += " stderr=" + previewText(out.Stderr, 200)
			}
			s2, _ := capBytes(singleLine(s), maxToolRunOutputPreviewBytes)
			return s2
		case "log":
			var out struct {
				Commits  []any  `json:"commits"`
				ExitCode int    `json:"exitCode"`
				Stderr   string `json:"stderr"`
			}
			if err := json.Unmarshal(raw, &out); err != nil {
				break
			}
			s := fmt.Sprintf("exitCode=%d commits=%d", out.ExitCode, len(out.Commits))
			if strings.TrimSpace(out.Stderr) != "" && out.ExitCode != 0 {
				s += " stderr=" + previewText(out.Stderr, 200)
			}
			s2, _ := capBytes(singleLine(s), maxToolRunOutputPreviewBytes)
			return s2
		case "branch":
			var out struct {
				Branches []any  `json:"branches"`
				Created  string `json:"created"`
				ExitCode int    `json:"exitCode"`
				Stderr   string `json:"stderr"`
			}
			if err := json.Unmarshal(raw, &out); err != nil {
				break
			}
			s := fmt.Sprintf("exitCode=%d branches=%d", out.ExitCode, len(out.Branches))
			if strings.TrimSpace(out.Created) != "" {
				s += " created=" + previewText(out.Created, 80)
			}
			if strings.TrimSpace(out.Stderr) != "" && out.ExitCode != 0 {
				s += " stderr=" + previewText(out.Stderr, 200)
			}
			s2, _ := capBytes(singleLine(s), maxToolRunOutputPreviewBytes)
			return s2
		}

	case "builtin.find":
		if strings.TrimSpace(actionID) != "files" {
			break
		}
		var out struct {
			Paths     []any  `json:"paths"`
			Truncated bool   `json:"truncated"`
			Limit     string `json:"limit"`
		}
		if err := json.Unmarshal(raw, &out); err != nil {
			break
		}
		s := fmt.Sprintf("paths=%d truncated=%t", len(out.Paths), out.Truncated)
		if strings.TrimSpace(out.Limit) != "" {
			s += " limit=" + previewText(out.Limit, 40)
		}
		s2, _ := capBytes(singleLine(s), maxToolRunOutputPreviewBytes)
		return s2

	case "builtin.test":
		switch strings.TrimSpace(actionID) {
		case "detect":
			var out struct {
				Framework string `json:"framework"`
			}
			if err := json.Unmarshal(raw, &out); err != nil {
				break
			}
			s := "framework=" + previewText(strings.TrimSpace(out.Framework), 40)
			s2, _ := capBytes(singleLine(s), maxToolRunOutputPreviewBytes)
			return s2
		case "run":
			var out struct {
				Framework string `json:"framework"`
				ExitCode  int    `json:"exitCode"`
				Passed    bool   `json:"passed"`
				Stderr    string `json:"stderr"`
			}
			if err := json.Unmarshal(raw, &out); err != nil {
				break
			}
			s := fmt.Sprintf("framework=%s exitCode=%d passed=%t", previewText(strings.TrimSpace(out.Framework), 40), out.ExitCode, out.Passed)
			if strings.TrimSpace(out.Stderr) != "" && out.ExitCode != 0 {
				s += " stderr=" + previewText(out.Stderr, 200)
			}
			s2, _ := capBytes(singleLine(s), maxToolRunOutputPreviewBytes)
			return s2
		case "list":
			var out struct {
				Framework string `json:"framework"`
				Files     []any  `json:"files"`
				Tests     []any  `json:"tests"`
			}
			if err := json.Unmarshal(raw, &out); err != nil {
				break
			}
			s := fmt.Sprintf("framework=%s files=%d tests=%d", previewText(strings.TrimSpace(out.Framework), 40), len(out.Files), len(out.Tests))
			s2, _ := capBytes(singleLine(s), maxToolRunOutputPreviewBytes)
			return s2
		}

	case "builtin.lint":
		switch strings.TrimSpace(actionID) {
		case "detect":
			var out struct {
				Linters    []any `json:"linters"`
				Formatters []any `json:"formatters"`
			}
			if err := json.Unmarshal(raw, &out); err != nil {
				break
			}
			s := fmt.Sprintf("linters=%d formatters=%d", len(out.Linters), len(out.Formatters))
			s2, _ := capBytes(singleLine(s), maxToolRunOutputPreviewBytes)
			return s2
		case "run":
			var out struct {
				Tool     string `json:"tool"`
				ExitCode int    `json:"exitCode"`
				Stderr   string `json:"stderr"`
			}
			if err := json.Unmarshal(raw, &out); err != nil {
				break
			}
			s := fmt.Sprintf("tool=%s exitCode=%d", previewText(strings.TrimSpace(out.Tool), 40), out.ExitCode)
			if strings.TrimSpace(out.Stderr) != "" && out.ExitCode != 0 {
				s += " stderr=" + previewText(out.Stderr, 200)
			}
			s2, _ := capBytes(singleLine(s), maxToolRunOutputPreviewBytes)
			return s2
		case "format":
			var out struct {
				Tool         string `json:"tool"`
				ExitCode     int    `json:"exitCode"`
				ChangedFiles []any  `json:"changedFiles"`
				Stderr       string `json:"stderr"`
			}
			if err := json.Unmarshal(raw, &out); err != nil {
				break
			}
			s := fmt.Sprintf("tool=%s exitCode=%d changedFiles=%d", previewText(strings.TrimSpace(out.Tool), 40), out.ExitCode, len(out.ChangedFiles))
			if strings.TrimSpace(out.Stderr) != "" && out.ExitCode != 0 {
				s += " stderr=" + previewText(out.Stderr, 200)
			}
			s2, _ := capBytes(singleLine(s), maxToolRunOutputPreviewBytes)
			return s2
		}

	case "builtin.ripgrep":
		if strings.TrimSpace(actionID) != "search" {
			break
		}
		var out struct {
			Matches []any `json:"matches"`
		}
		if err := json.Unmarshal(raw, &out); err != nil {
			break
		}
		s := fmt.Sprintf("matches=%d", len(out.Matches))
		s2, _ := capBytes(s, maxToolRunOutputPreviewBytes)
		return s2

	case "builtin.http":
		if strings.TrimSpace(actionID) != "fetch" {
			break
		}
		var out struct {
			Status        int    `json:"status"`
			FinalURL      string `json:"finalUrl"`
			Body          string `json:"body"`
			BodyTruncated bool   `json:"bodyTruncated"`
			BodyPath      string `json:"bodyPath"`
			Warning       string `json:"warning"`
		}
		if err := json.Unmarshal(raw, &out); err != nil {
			break
		}
		s := fmt.Sprintf("status=%d", out.Status)
		if strings.TrimSpace(out.FinalURL) != "" {
			s += " url=" + previewText(out.FinalURL, 200)
		}
		if strings.TrimSpace(out.Warning) != "" {
			s += " warning=" + previewText(out.Warning, 200)
		}
		body := strings.TrimSpace(out.Body)
		if body != "" {
			s += " body=" + previewText(body, 300)
			if out.BodyTruncated && out.BodyPath != "" {
				s += " (full=" + out.BodyPath + ")"
			} else if out.BodyTruncated {
				s += " (truncated)"
			}
		} else if out.BodyPath != "" {
			s += " bodyPath=" + out.BodyPath
		}
		s2, _ := capBytes(singleLine(s), maxToolRunOutputPreviewBytes)
		return s2
	}

	// Generic fallback: show a compact JSON string preview.
	s2, _ := capBytes(singleLine(string(raw)), maxToolRunOutputPreviewBytes)
	return s2
}

// fsWriteTextPreviewForEvent returns a small preview of a fs.write/fs.append payload for UI events.
//
// The goal is to make the Activity details panel more useful (show what was written) without
// storing full file contents in the event stream.
//
// Behavior:
// - Hard-caps size.
// - Best-effort JSON pretty-printing when the content looks like JSON (either by extension or validity).
// - Redacts obvious secrets (bearer tokens, api keys) conservatively.
func fsWriteTextPreviewForEvent(path string, text string) (preview string, truncated bool, redacted bool, originalBytes int, isJSON bool) {
	originalBytes = len([]byte(text))
	if strings.TrimSpace(text) == "" {
		return "", false, false, originalBytes, false
	}

	if looksSensitiveText(text) {
		return "<omitted>", false, true, originalBytes, false
	}

	raw := bytes.TrimSpace([]byte(text))
	ext := strings.ToLower(strings.TrimSpace(path))
	isJSON = strings.HasSuffix(ext, ".json") || json.Valid(raw)

	preview = text
	if isJSON {
		var buf bytes.Buffer
		if err := json.Indent(&buf, raw, "", "  "); err == nil {
			preview = buf.String()
		}
	}

	preview, truncated = capBytes(preview, maxFSWriteTextPreviewBytes)
	return preview, truncated, false, originalBytes, isJSON
}

func previewText(s string, max int) string {
	s = strings.ReplaceAll(s, "\r", "")
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

func redactValue(key string, v any) any {
	k := strings.ToLower(strings.TrimSpace(key))
	switch x := v.(type) {
	case string:
		// First: content-based redaction (catch secrets even under benign keys).
		if looksSensitiveText(x) {
			return "<omitted>"
		}
		// Next: key-based policies.
		if isMessageLikeKey(k) {
			// Keep UX signal without dumping full text.
			return previewText(x, 120)
		}
		if isSensitiveKey(k) || len(x) > 256 {
			return "<omitted>"
		}
		return x
	case []any:
		// Keep arrays small and stable (e.g., argv, paths, glob). Redact long strings.
		out := make([]any, 0, len(x))
		for _, it := range x {
			if s, ok := it.(string); ok {
				if looksSensitiveText(s) {
					out = append(out, "<omitted>")
				} else if len(s) > 256 {
					out = append(out, "<omitted>")
				} else {
					out = append(out, s)
				}
				continue
			}
			out = append(out, it)
		}
		return out
	case map[string]any:
		keys := make([]string, 0, len(x))
		for kk := range x {
			keys = append(keys, kk)
		}
		sort.Strings(keys)
		m := make(map[string]any, len(keys))
		for _, kk := range keys {
			m[kk] = redactValue(kk, x[kk])
		}
		return m
	default:
		return v
	}
}

func isSensitiveKey(k string) bool {
	switch k {
	case "text", "stdin", "authorization", "token", "apikey", "api_key", "secret", "password":
		return true
	default:
		return false
	}
}

func isMessageLikeKey(k string) bool {
	switch k {
	case "message", "body", "patch":
		return true
	default:
		return false
	}
}

func looksSensitiveText(s string) bool {
	low := strings.ToLower(s)
	switch {
	case strings.Contains(low, "authorization: bearer "):
		return true
	case strings.Contains(low, "api_key"):
		return true
	case strings.Contains(low, "apikey"):
		return true
	case strings.Contains(low, "secret"):
		return true
	case strings.Contains(low, "password"):
		return true
	case strings.Contains(s, "sk-"):
		return true
	default:
		return false
	}
}

func singleLine(s string) string {
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	return strings.TrimSpace(s)
}

func capBytes(s string, max int) (string, bool) {
	if max <= 0 || len(s) <= max {
		return s, false
	}
	if max < 2 {
		return s[:max], true
	}
	return s[:max-1] + "…", true
}
