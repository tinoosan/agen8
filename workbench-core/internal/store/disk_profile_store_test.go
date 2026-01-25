package store

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tinoosan/workbench-core/pkg/types"
)

func TestDiskProfileStore_Basics(t *testing.T) {
	tmp := t.TempDir()
	s, err := NewDiskProfileStoreFromDir(filepath.Join(tmp, "profile"))
	if err != nil {
		t.Fatalf("NewDiskProfileStoreFromDir: %v", err)
	}

	if got, err := s.GetProfile(context.Background()); err != nil || got != "" {
		t.Fatalf("GetProfile empty: got=%q err=%v", got, err)
	}

	if err := s.SetUpdate(context.Background(), "birthday: 1994-11-27\n"); err != nil {
		t.Fatalf("SetUpdate: %v", err)
	}
	if got, err := s.GetUpdate(context.Background()); err != nil || got == "" {
		t.Fatalf("GetUpdate: got=%q err=%v", got, err)
	}
	if err := s.ClearUpdate(context.Background()); err != nil {
		t.Fatalf("ClearUpdate: %v", err)
	}
	if got, _ := s.GetUpdate(context.Background()); strings.TrimSpace(got) != "" {
		t.Fatalf("expected cleared update, got=%q", got)
	}

	if err := s.AppendProfile(context.Background(), "birthday: 1994-11-27\n"); err != nil {
		t.Fatalf("AppendProfile: %v", err)
	}
	if got, err := s.GetProfile(context.Background()); err != nil || !strings.Contains(got, "birthday") {
		t.Fatalf("GetProfile: got=%q err=%v", got, err)
	}

	line := types.MemoryCommitLine{
		Scope:     "profile",
		SessionID: "sess-1",
		RunID:     "run-1",
		Model:     "m",
		Turn:      1,
		Accepted:  true,
		Reason:    "accepted",
		Bytes:     3,
		SHA256:    "x",
	}
	if err := s.AppendCommitLog(context.Background(), line); err != nil {
		t.Fatalf("AppendCommitLog: %v", err)
	}

	b, err := os.ReadFile(filepath.Join(s.Dir, "commits.jsonl"))
	if err != nil {
		t.Fatalf("ReadFile commits.jsonl: %v", err)
	}
	sline := strings.TrimSpace(string(b))
	if sline == "" {
		t.Fatalf("expected jsonl content")
	}
	var parsed types.MemoryCommitLine
	if err := json.Unmarshal([]byte(sline), &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if parsed.Scope != "profile" || parsed.RunID != "run-1" || parsed.Timestamp == "" {
		t.Fatalf("unexpected parsed line: %+v", parsed)
	}
}
