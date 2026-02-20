package agent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// extractSingleJSONObject parses exactly one JSON object from model output.
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
	if nl := strings.IndexByte(t, '\n'); nl >= 0 {
		t = t[nl+1:]
	} else {
		return ""
	}
	t = strings.TrimSpace(t)
	if strings.HasSuffix(t, "```") {
		t = strings.TrimSpace(t[:len(t)-3])
	}
	return t
}
