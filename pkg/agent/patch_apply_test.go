package agent

import (
	"strings"
	"testing"
)

func TestApplyUnifiedDiffWithDiagnostics_SuccessApply(t *testing.T) {
	before := "# Title\nBody\n"
	patch := "@@ -1,2 +1,2 @@\n # Title\n-Body\n+Body updated\n"

	after, diag, err := ApplyUnifiedDiffWithDiagnostics(before, patch, false, false)
	if err != nil {
		t.Fatalf("ApplyUnifiedDiffWithDiagnostics: %v", err)
	}
	if after != "# Title\nBody updated\n" {
		t.Fatalf("after = %q", after)
	}
	if diag.Mode != "apply" || diag.HunksTotal != 1 || diag.HunksApplied != 1 {
		t.Fatalf("unexpected diagnostics: %+v", diag)
	}
	if diag.FailureReason != "" {
		t.Fatalf("expected no failure reason on success, got %+v", diag)
	}
}

func TestApplyUnifiedDiffWithDiagnostics_SuccessDryRun(t *testing.T) {
	before := "alpha\nbeta\n"
	patch := "@@ -1,2 +1,2 @@\n alpha\n-beta\n+gamma\n"

	after, diag, err := ApplyUnifiedDiffWithDiagnostics(before, patch, true, false)
	if err != nil {
		t.Fatalf("ApplyUnifiedDiffWithDiagnostics: %v", err)
	}
	if after != "alpha\ngamma\n" {
		t.Fatalf("after = %q", after)
	}
	if diag.Mode != "dry_run" || diag.HunksTotal != 1 || diag.HunksApplied != 1 {
		t.Fatalf("unexpected diagnostics: %+v", diag)
	}
}

func TestApplyUnifiedDiffWithDiagnostics_ContextMismatch(t *testing.T) {
	before := "alpha\nbeta\n"
	patch := "@@ -1,2 +1,2 @@\n gamma\n-beta\n+delta\n"

	_, diag, err := ApplyUnifiedDiffWithDiagnostics(before, patch, false, true)
	if err == nil {
		t.Fatalf("expected context mismatch error")
	}
	if !strings.Contains(err.Error(), "context mismatch") {
		t.Fatalf("unexpected error: %v", err)
	}
	if diag.FailureReason != "context_mismatch" {
		t.Fatalf("failure reason = %q", diag.FailureReason)
	}
	if diag.FailedHunk != 1 {
		t.Fatalf("failed hunk = %d", diag.FailedHunk)
	}
	if diag.TargetLine != 1 {
		t.Fatalf("target line = %d", diag.TargetLine)
	}
	if len(diag.ExpectedContext) == 0 || diag.ExpectedContext[0] != "gamma" {
		t.Fatalf("expected context missing: %+v", diag)
	}
	if len(diag.ActualContext) == 0 || diag.ActualContext[0] != "alpha" {
		t.Fatalf("actual context missing: %+v", diag)
	}
	if strings.TrimSpace(diag.Suggestion) == "" {
		t.Fatalf("expected suggestion in diagnostics")
	}
}

func TestApplyUnifiedDiffWithDiagnostics_DeleteMismatch(t *testing.T) {
	before := "alpha\nbeta\n"
	patch := "@@ -1,2 +1,1 @@\n alpha\n-gamma\n"

	_, diag, err := ApplyUnifiedDiffWithDiagnostics(before, patch, false, false)
	if err == nil {
		t.Fatalf("expected delete mismatch error")
	}
	if !strings.Contains(err.Error(), "delete mismatch") {
		t.Fatalf("unexpected error: %v", err)
	}
	if diag.FailureReason != "delete_mismatch" {
		t.Fatalf("failure reason = %q", diag.FailureReason)
	}
	if diag.FailedHunk != 1 {
		t.Fatalf("failed hunk = %d", diag.FailedHunk)
	}
	if diag.TargetLine != 2 {
		t.Fatalf("target line = %d", diag.TargetLine)
	}
}
