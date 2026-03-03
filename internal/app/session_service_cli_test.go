package app

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestNoopCLISupervisor_RuntimeControlsReturnExplicitUnsupportedErrors(t *testing.T) {
	s := noopCLISupervisor{}

	stopErr := s.StopRun(context.Background(), "run-1")
	if stopErr == nil {
		t.Fatalf("expected stop error")
	}
	if !errors.Is(stopErr, errCLIRuntimeControlUnsupported) {
		t.Fatalf("expected errCLIRuntimeControlUnsupported, got: %v", stopErr)
	}
	if !strings.Contains(stopErr.Error(), "not supported") {
		t.Fatalf("expected explicit unsupported message, got: %v", stopErr)
	}

	resumeErr := s.ResumeRun(context.Background(), "run-1")
	if resumeErr == nil {
		t.Fatalf("expected resume error")
	}
	if !errors.Is(resumeErr, errCLIRuntimeControlUnsupported) {
		t.Fatalf("expected errCLIRuntimeControlUnsupported, got: %v", resumeErr)
	}
	if !strings.Contains(resumeErr.Error(), "not supported") {
		t.Fatalf("expected explicit unsupported message, got: %v", resumeErr)
	}
}
