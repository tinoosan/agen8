package app

import (
	"context"
	"testing"

	"github.com/tinoosan/workbench-core/internal/config"
	"github.com/tinoosan/workbench-core/pkg/events"
	"github.com/tinoosan/workbench-core/internal/store"
	"github.com/tinoosan/workbench-core/internal/types"
	"github.com/tinoosan/workbench-core/pkg/vfs"
	"github.com/tinoosan/workbench-core/pkg/agent"
)

func TestTUITurnRunner_Reasoning_UpdatesAgentAndSession(t *testing.T) {
	t.Parallel()

	cfg := config.Config{DataDir: t.TempDir()}

	sess, run, err := store.CreateSession(cfg, "test", 1024)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	var got []events.Event
	r := &tuiTurnRunner{
		cfg:  cfg,
		fs:   vfs.NewFS(),
		run:  run,
		opts: resolveRunChatOptions(WithModel("openai/gpt-5.1-codex-mini")),
		agent: &agent.Agent{
			Model: "openai/gpt-5.1-codex-mini",
		},
		model:            "openai/gpt-5.1-codex-mini",
		setHistoryModel:  func(string) {},
		mustEmit:         func(_ context.Context, ev events.Event) { got = append(got, ev) },
		baseSystemPrompt: "",
	}

	if _, handled := r.handleSlashCommand("/reasoning effort high"); !handled {
		t.Fatalf("expected /reasoning to be handled")
	}
	if r.opts.ReasoningEffort != "high" {
		t.Fatalf("opts.ReasoningEffort=%q, want %q", r.opts.ReasoningEffort, "high")
	}
	if r.agent.ReasoningEffort != "high" {
		t.Fatalf("agent.ReasoningEffort=%q, want %q", r.agent.ReasoningEffort, "high")
	}

	if _, handled := r.handleSlashCommand("/reasoning summary concise"); !handled {
		t.Fatalf("expected /reasoning to be handled")
	}
	if r.opts.ReasoningSummary != "concise" {
		t.Fatalf("opts.ReasoningSummary=%q, want %q", r.opts.ReasoningSummary, "concise")
	}
	if r.agent.ReasoningSummary != "concise" {
		t.Fatalf("agent.ReasoningSummary=%q, want %q", r.agent.ReasoningSummary, "concise")
	}

	updated, err := store.LoadSession(cfg, sess.SessionID)
	if err != nil {
		t.Fatalf("LoadSession: %v", err)
	}
	if updated.ReasoningEffort != "high" {
		t.Fatalf("session reasoningEffort=%q, want %q", updated.ReasoningEffort, "high")
	}
	if updated.ReasoningSummary != "concise" {
		t.Fatalf("session reasoningSummary=%q, want %q", updated.ReasoningSummary, "concise")
	}

	found := false
	for _, ev := range got {
		if ev.Type == "reasoning.changed" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected a reasoning.changed event")
	}
}

func TestTUITurnRunner_Reasoning_Info(t *testing.T) {
	t.Parallel()

	cfg := config.Config{DataDir: t.TempDir()}
	_, run, err := store.CreateSession(cfg, "test", 1024)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	r := &tuiTurnRunner{
		cfg:  cfg,
		fs:   vfs.NewFS(),
		run:  run,
		opts: resolveRunChatOptions(WithModel("openai/gpt-5.1-codex-mini")),
		agent: &agent.Agent{
			Model: "openai/gpt-5.1-codex-mini",
		},
		model:            "openai/gpt-5.1-codex-mini",
		setHistoryModel:  func(string) {},
		mustEmit:         func(_ context.Context, _ events.Event) {},
		baseSystemPrompt: "",
	}

	out, handled := r.handleSlashCommand("/reasoning")
	if !handled {
		t.Fatalf("expected /reasoning to be handled")
	}
	// Best-effort: just ensure it returns a non-empty description.
	if out == "" {
		t.Fatalf("expected non-empty output")
	}
	// Ensure it doesn't look like a usage error.
	if out == "Usage: /reasoning effort <...> OR /reasoning summary <...>" {
		t.Fatalf("unexpected usage output")
	}
	_ = types.RunRuntimeConfig{} // silence unused import if Go tooling changes
}
