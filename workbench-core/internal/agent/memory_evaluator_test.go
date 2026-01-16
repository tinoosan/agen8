package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tinoosan/workbench-core/internal/types"
)

func TestMemoryEvaluator_Evaluate(t *testing.T) {
	e := DefaultMemoryEvaluator()

	t.Run("AcceptsStructuredBullet", func(t *testing.T) {
		ok, reason, cleaned := e.Evaluate("- Use stdout + fs.write\n")
		if !ok || reason != "accepted" {
			t.Fatalf("expected accepted, got ok=%v reason=%q", ok, reason)
		}
		if !strings.HasSuffix(cleaned, "\n") {
			t.Fatalf("expected newline-terminated cleaned output")
		}
	})

	t.Run("AcceptsKeyValueFact", func(t *testing.T) {
		ok, reason, cleaned := e.Evaluate("birthday: 1994-11-27\n")
		if !ok || reason != "accepted" {
			t.Fatalf("expected accepted, got ok=%v reason=%q", ok, reason)
		}
		if !strings.HasSuffix(cleaned, "\n") {
			t.Fatalf("expected newline-terminated cleaned output")
		}
	})

	t.Run("RejectsUnstructured", func(t *testing.T) {
		ok, reason, _ := e.Evaluate("this is a long paragraph with no structure")
		if ok || reason != "unstructured" {
			t.Fatalf("expected unstructured rejection, got ok=%v reason=%q", ok, reason)
		}
	})

	t.Run("RejectsObviousSecrets", func(t *testing.T) {
		ok, reason, _ := e.Evaluate("Authorization: Bearer sk-12345678901234567890")
		if ok || reason != "denied_pattern" {
			t.Fatalf("expected denied_pattern, got ok=%v reason=%q", ok, reason)
		}
	})
}

func TestAppendCommitLog_WritesJSONL(t *testing.T) {
	tmp := t.TempDir()

	line := types.MemoryCommitLine{
		Turn:     1,
		Model:    "m",
		Accepted: true,
		Reason:   "accepted",
		Bytes:    3,
		SHA256:   SHA256Hex("abc"),
	}
	if err := AppendCommitLog(tmp, line); err != nil {
		t.Fatalf("AppendCommitLog: %v", err)
	}

	b, err := os.ReadFile(filepath.Join(tmp, "commits.jsonl"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	s := strings.TrimSpace(string(b))
	if s == "" {
		t.Fatalf("expected jsonl content")
	}

	var parsed types.MemoryCommitLine
	if err := json.Unmarshal([]byte(s), &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if parsed.Turn != 1 || parsed.SHA256 == "" || parsed.Timestamp == "" {
		t.Fatalf("unexpected parsed line: %+v", parsed)
	}
}
