package tui

import (
	"strings"
	"unicode"
)

// splitMonitorCommand returns the slash command and the remaining argument text.
//
// It is whitespace-tolerant (spaces, tabs, newlines) and preserves the original
// argument text (except for trimming leading whitespace before args).
//
// Special case: "/memory search" is treated as a single command token.
func splitMonitorCommand(raw string) (cmd string, rest string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", ""
	}

	// Read first token.
	i := 0
	for i < len(raw) && unicode.IsSpace(rune(raw[i])) {
		i++
	}
	start := i
	for i < len(raw) && !unicode.IsSpace(rune(raw[i])) {
		i++
	}
	if start == i {
		return "", ""
	}
	t1 := raw[start:i]

	// Skip whitespace after first token.
	j := i
	for j < len(raw) && unicode.IsSpace(rune(raw[j])) {
		j++
	}
	if t1 == "/memory" {
		// Read second token.
		k := j
		for k < len(raw) && !unicode.IsSpace(rune(raw[k])) {
			k++
		}
		t2 := raw[j:k]
		if t2 == "search" {
			cmd = "/memory search"
			rest = raw[k:]
			rest = strings.TrimLeftFunc(rest, unicode.IsSpace)
			return cmd, rest
		}
	}

	cmd = t1
	rest = raw[i:]
	rest = strings.TrimLeftFunc(rest, unicode.IsSpace)
	return cmd, rest
}

