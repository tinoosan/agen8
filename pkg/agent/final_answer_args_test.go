package agent

import (
	"testing"

	"github.com/tinoosan/agen8/pkg/types"
)

func TestParseFinalAnswerArgs_FailedOK(t *testing.T) {
	args, err := parseFinalAnswerArgs(`{"text":"nope","status":"failed","error":"Goal not achievable","artifacts":[]}`)
	if err != nil {
		t.Fatalf("parseFinalAnswerArgs: %v", err)
	}
	if args.Status != types.TaskStatusFailed {
		t.Fatalf("expected status failed, got %q", args.Status)
	}
	if args.Error == "" {
		t.Fatalf("expected non-empty error")
	}
	if args.Text != "nope" {
		t.Fatalf("unexpected text %q", args.Text)
	}
}

func TestParseFinalAnswerArgs_MissingStatus(t *testing.T) {
	_, err := parseFinalAnswerArgs(`{"text":"ok","error":"","artifacts":[]}`)
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestParseFinalAnswerArgs_SucceededWithErrorRejected(t *testing.T) {
	_, err := parseFinalAnswerArgs(`{"text":"ok","status":"succeeded","error":"warn","artifacts":[]}`)
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestParseFinalAnswerArgs_FailedWithoutErrorRejected(t *testing.T) {
	_, err := parseFinalAnswerArgs(`{"text":"ok","status":"failed","error":"","artifacts":[]}`)
	if err == nil {
		t.Fatalf("expected error")
	}
}
