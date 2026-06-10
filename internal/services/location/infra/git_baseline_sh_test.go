package infra

import (
	"os/exec"
	"strings"
	"testing"
)

// runShEcho passes a pre-quoted argument to `printf %s` through a real shell.
// If the quoting were wrong, embedded metacharacters would alter the command
// (or execute) instead of echoing back verbatim.
func runShEcho(t *testing.T, quotedArg string) string {
	t.Helper()
	out, err := exec.Command("sh", "-c", "printf %s "+quotedArg).CombinedOutput()
	if err != nil {
		t.Fatalf("sh -c failed for %s: %v (out=%q)", quotedArg, err, out)
	}
	return strings.TrimRight(string(out), "\n")
}

// runShWordCount counts how many shell words a command string resolves to,
// proving quoted arguments stay single words. Replaces the command verb with a
// counter so nothing executes.
func runShWordCount(t *testing.T, command string) int {
	t.Helper()
	// Strip the leading "git" and feed the rest as positional args to a counter.
	rest := strings.TrimSpace(strings.TrimPrefix(command, "git"))
	out, err := exec.Command("sh", "-c", `set -- `+rest+`; echo $#`).CombinedOutput()
	if err != nil {
		t.Fatalf("word count failed: %v (out=%q)", err, out)
	}
	n := 0
	for _, c := range strings.TrimSpace(string(out)) {
		n = n*10 + int(c-'0')
	}
	return n + 1 // +1 for the stripped "git"
}
