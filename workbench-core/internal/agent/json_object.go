package agent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// extractSingleJSONObject parses exactly one JSON object from model output.
//
// It is intentionally strict:
// - the decoded top-level value must be an object
// - there must be no trailing non-whitespace after the object
//
// It is also best-effort:
// - if the output is wrapped in a single markdown code fence, it is stripped
// - if the output has leading text, parsing starts at the first '{'
func extractSingleJSONObject(s string) (string, error) {
	trim := stripOuterCodeFence(s)
	trim = strings.TrimSpace(trim)
	if trim == "" {
		return "", fmt.Errorf("empty output")
	}

	start := 0
	if firstNonWS := strings.TrimLeft(trim, " \t\r\n"); firstNonWS == "" {
		return "", fmt.Errorf("empty output")
	} else if firstNonWS[0] != '{' {
		idx := strings.Index(trim, "{")
		if idx < 0 {
			return "", fmt.Errorf("no JSON object found")
		}
		start = idx
	}

	dec := json.NewDecoder(strings.NewReader(trim[start:]))
	var raw json.RawMessage
	if err := dec.Decode(&raw); err != nil {
		return "", fmt.Errorf("decode JSON: %w", err)
	}
	raw = json.RawMessage(bytes.TrimSpace(raw))
	if len(raw) == 0 || raw[0] != '{' {
		return "", fmt.Errorf("top-level JSON value must be an object")
	}
	// Ensure there is no trailing JSON or non-whitespace data.
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return "", fmt.Errorf("trailing JSON value after object")
		}
		return "", fmt.Errorf("trailing data after object: %w", err)
	}
	return string(raw), nil
}

func stripOuterCodeFence(s string) string {
	t := strings.TrimSpace(s)
	if !strings.HasPrefix(t, "```") {
		return t
	}
	// Remove the opening fence line (``` or ```lang).
	if nl := strings.IndexByte(t, '\n'); nl >= 0 {
		t = t[nl+1:]
	} else {
		// Fence without content; treat as empty.
		return ""
	}
	t = strings.TrimSpace(t)
	// Remove a trailing closing fence if present.
	if strings.HasSuffix(t, "```") {
		t = strings.TrimSpace(t[:len(t)-3])
	}
	return t
}
