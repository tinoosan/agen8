package agent

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// MemoryEvaluator is the simplest possible "Context Evaluator v0" for agent memory.
//
// The current memory architecture uses a two-phase commit:
//  1. The agent writes a candidate update to /memory/update.md (VFS).
//  2. The host evaluates it (this evaluator) and decides whether to commit it.
//
// This file intentionally does NOT call any LLMs. It is a deterministic policy gate
// that:
//   - reduces accidental pollution of memory ("random paragraphs")
//   - blocks obvious secret leakage into memory
//   - records an immutable audit trail to /memory/commits.jsonl
//
// This matches the Workbench architecture principle:
//   - /memory is run-scoped working memory
//   - /history (later) will be the shared global provenance record
type MemoryEvaluator struct {
	// MaxBytes is the maximum size of a memory update the host will consider.
	// If zero, a default is used.
	MaxBytes int

	// RequireStructured requires the update to look like short structured notes.
	// If true, the update must either:
	//   - contain at least one markdown bullet line ("- ...")
	//   - or start with "RULE:" / "NOTE:" / "OBS:" / "LEARNED:"
	RequireStructured bool

	// DenyRegex blocks updates that match any of these patterns.
	// This is used to prevent common secret/token leaks from being persisted.
	DenyRegex []*regexp.Regexp
}

func DefaultMemoryEvaluator() *MemoryEvaluator {
	return &MemoryEvaluator{
		MaxBytes:          2048,
		RequireStructured: true,
		DenyRegex: []*regexp.Regexp{
			regexp.MustCompile(`(?i)OPENROUTER_API_KEY`),
			regexp.MustCompile(`(?i)OPENAI_API_KEY`),
			regexp.MustCompile(`(?i)Authorization:\s*Bearer\s+`),
			regexp.MustCompile(`(?i)Bearer\s+[A-Za-z0-9_-]{16,}`),
		},
	}
}

// Evaluate checks whether a memory update should be committed to /memory/memory.md.
//
// It returns:
//   - accepted: whether the host should commit the update
//   - reason: a short machine-readable reason (useful for auditing)
//   - cleaned: the normalized text to commit (when accepted)
func (e *MemoryEvaluator) Evaluate(update string) (accepted bool, reason string, cleaned string) {
	if e == nil {
		return false, "evaluator_missing", ""
	}

	update = strings.TrimSpace(update)
	if update == "" {
		return false, "empty", ""
	}

	maxBytes := e.MaxBytes
	if maxBytes == 0 {
		maxBytes = 2048
	}
	if len(update) > maxBytes {
		return false, "too_large", ""
	}

	for _, re := range e.DenyRegex {
		if re != nil && re.MatchString(update) {
			return false, "denied_pattern", ""
		}
	}

	if e.RequireStructured {
		if !looksStructured(update) {
			return false, "unstructured", ""
		}
	}

	// Keep it consistent: single trailing newline, no leading/trailing whitespace.
	return true, "accepted", strings.TrimSpace(update) + "\n"
}

func looksStructured(s string) bool {
	upper := strings.ToUpper(strings.TrimSpace(s))
	for _, prefix := range []string{"RULE:", "NOTE:", "OBS:", "LEARNED:"} {
		if strings.HasPrefix(upper, prefix) {
			return true
		}
	}
	for _, line := range strings.Split(s, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "- ") {
			return true
		}
	}
	return false
}

// MemoryCommitLine is a single audit record appended to /memory/commits.jsonl.
//
// The audit log is immutable and append-only within a run. This enables debugging:
// "what did the agent try to write to memory, and why was it accepted/rejected?"
type MemoryCommitLine struct {
	Timestamp string `json:"timestamp"`
	Model     string `json:"model,omitempty"`
	Turn      int    `json:"turn"`

	Accepted bool   `json:"accepted"`
	Reason   string `json:"reason"`

	Bytes  int    `json:"bytes"`
	SHA256 string `json:"sha256"`
}

// AppendCommitLog appends a JSONL audit record to baseDir/commits.jsonl.
func AppendCommitLog(baseDir string, line MemoryCommitLine) error {
	if strings.TrimSpace(baseDir) == "" {
		return fmt.Errorf("baseDir is required")
	}
	if line.Timestamp == "" {
		line.Timestamp = time.Now().UTC().Format(time.RFC3339Nano)
	}
	b, err := json.Marshal(line)
	if err != nil {
		return err
	}

	p := filepath.Join(baseDir, "commits.jsonl")
	f, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(b, '\n'))
	return err
}

// SHA256Hex returns the SHA-256 digest of s, encoded as lowercase hex.
//
// This is used for audit logging so updates can be referenced without storing
// the full plaintext in the audit line.
func SHA256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}
