package app

import (
	"context"
	"testing"
)

func TestResolveSessionTitle_UsesGeneratedTitle(t *testing.T) {
	orig := generateSessionTitleFn
	t.Cleanup(func() { generateSessionTitleFn = orig })

	generateSessionTitleFn = func(ctx context.Context, userMsg string) (string, error) {
		_ = ctx
		_ = userMsg
		return "My Generated Title", nil
	}

	var warnings []string
	got := resolveSessionTitle(context.Background(), "", "user msg", &warnings)
	if got != "My Generated Title" {
		t.Fatalf("expected generated title, got %q", got)
	}
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}
}

func TestResolveSessionTitle_EmptyGeneratedFallsBack(t *testing.T) {
	orig := generateSessionTitleFn
	t.Cleanup(func() { generateSessionTitleFn = orig })

	generateSessionTitleFn = func(ctx context.Context, userMsg string) (string, error) {
		_ = ctx
		_ = userMsg
		return "   ", nil
	}

	var warnings []string
	got := resolveSessionTitle(context.Background(), "", "user msg", &warnings)
	if got != "workbench" {
		t.Fatalf("expected fallback title, got %q", got)
	}
	if len(warnings) != 1 {
		t.Fatalf("expected warning when title empty, got %v", warnings)
	}
}
