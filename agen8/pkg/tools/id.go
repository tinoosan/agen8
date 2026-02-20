package tools

import (
	"fmt"
	"regexp"
	"strings"
)

// ToolID identifies a tool implementation (e.g. "builtin.shell" or "github.com.acme.tool").
type ToolID string

func (t ToolID) String() string { return strings.TrimSpace(string(t)) }

var (
	toolIDRE   = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*[a-zA-Z0-9]$`)
	actionIDRE = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*[a-zA-Z0-9]$`)
)

func ParseToolID(s string) (ToolID, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", fmt.Errorf("toolId is required")
	}
	if strings.ContainsAny(s, " \t\r\n/\\") {
		return "", fmt.Errorf("invalid toolId %q", s)
	}
	if !toolIDRE.MatchString(s) {
		return "", fmt.Errorf("invalid toolId %q", s)
	}
	return ToolID(s), nil
}

func ParseActionID(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", fmt.Errorf("actionId is required")
	}
	if strings.ContainsAny(s, " \t\r\n/\\") {
		return "", fmt.Errorf("invalid actionId %q", s)
	}
	if !actionIDRE.MatchString(s) {
		return "", fmt.Errorf("invalid actionId %q", s)
	}
	return s, nil
}
