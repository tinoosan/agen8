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

	"github.com/tinoosan/workbench-core/pkg/types"
)

// MemoryEvaluator is the simplest possible "Context Evaluator v0" for agent memory.
type MemoryEvaluator struct {
	MaxBytes          int
	RequireStructured bool
	DenyRegex         []*regexp.Regexp
}

var kvStructuredLineRE = regexp.MustCompile(`^\s*[A-Za-z][A-Za-z0-9 _-]{0,40}\s*:\s+\S+`)

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
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "- ") {
			return true
		}
		if kvStructuredLineRE.MatchString(line) {
			return true
		}
	}
	return false
}

// AppendCommitLog appends a JSONL audit record to baseDir/commits.jsonl.
func AppendCommitLog(baseDir string, line types.MemoryCommitLine) error {
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
func SHA256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}
