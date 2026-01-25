package app

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/tinoosan/workbench-core/internal/config"
	"github.com/tinoosan/workbench-core/internal/events"
	"github.com/tinoosan/workbench-core/internal/store"
	"github.com/tinoosan/workbench-core/internal/types"
	"github.com/tinoosan/workbench-core/internal/vfs"
	"github.com/tinoosan/workbench-core/pkg/agent"
)

func TestLazyRunner_Model_DoesNotInitializeSession(t *testing.T) {
	t.Parallel()

	oldModel := os.Getenv("OPENROUTER_MODEL")
	t.Cleanup(func() { _ = os.Setenv("OPENROUTER_MODEL", oldModel) })
	_ = os.Setenv("OPENROUTER_MODEL", "openai/gpt-5.2")

	ch := make(chan events.Event, 10)
	r := &lazyNewSessionTurnRunner{
		ctx:  context.Background(),
		opts: resolveRunChatOptions(),
		evCh: ch,
	}

	final, err := r.RunTurn(context.Background(), "/model openai/gpt-4o")
	if err != nil {
		t.Fatalf("RunTurn(/model): %v", err)
	}
	if final == "" {
		t.Fatalf("expected non-empty response")
	}
	if !strings.Contains(final, "openai/gpt-4o") {
		t.Fatalf("final=%q, expected to mention %q", final, "openai/gpt-4o")
	}
	if r.initialized {
		t.Fatalf("expected runner to remain uninitialized")
	}
	if r.opts.Model != "openai/gpt-4o" {
		t.Fatalf("opts.Model=%q, want %q", r.opts.Model, "openai/gpt-4o")
	}

	found := false
	for {
		select {
		case ev := <-ch:
			if ev.Type == "model.changed" {
				found = true
				if ev.Data["to"] != "openai/gpt-4o" {
					t.Fatalf("model.changed to=%q, want %q", ev.Data["to"], "openai/gpt-4o")
				}
			}
		default:
			if !found {
				t.Fatalf("expected a model.changed event")
			}
			return
		}
	}
}

func TestLazyRunner_Approval_DoesNotInitializeSession(t *testing.T) {
	t.Parallel()

	ch := make(chan events.Event, 10)
	r := &lazyNewSessionTurnRunner{
		ctx:  context.Background(),
		opts: resolveRunChatOptions(),
		evCh: ch,
	}

	final, err := r.RunTurn(context.Background(), "/approval disabled")
	if err != nil {
		t.Fatalf("RunTurn(/approval): %v", err)
	}
	if final == "" {
		t.Fatalf("expected non-empty response")
	}
	if r.initialized {
		t.Fatalf("expected runner to remain uninitialized")
	}
	if r.opts.ApprovalsMode != "disabled" {
		t.Fatalf("opts.ApprovalsMode=%q, want %q", r.opts.ApprovalsMode, "disabled")
	}

	found := false
	for {
		select {
		case ev := <-ch:
			if ev.Type == "approvals.changed" {
				found = true
				if ev.Data["mode"] != "disabled" {
					t.Fatalf("approvals.changed mode=%q, want %q", ev.Data["mode"], "disabled")
				}
			}
		default:
			if !found {
				t.Fatalf("expected an approvals.changed event")
			}
			return
		}
	}
}

func TestTUITurnRunner_Model_UpdatesAgentAndSession(t *testing.T) {
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
		opts: resolveRunChatOptions(WithModel("openai/gpt-5.2")),
		agent: &agent.Agent{
			Model: "openai/gpt-5.2",
		},
		model:            "openai/gpt-5.2",
		setHistoryModel:  func(string) {},
		mustEmit:         func(_ context.Context, ev events.Event) { got = append(got, ev) },
		baseSystemPrompt: "",
	}

	if _, handled := r.handleSlashCommand("/model openai/gpt-4o"); !handled {
		t.Fatalf("expected /model to be handled")
	}
	if r.model != "openai/gpt-4o" {
		t.Fatalf("runner model=%q, want %q", r.model, "openai/gpt-4o")
	}
	if r.agent.Model != "openai/gpt-4o" {
		t.Fatalf("agent model=%q, want %q", r.agent.Model, "openai/gpt-4o")
	}

	updated, err := store.LoadSession(cfg, sess.SessionID)
	if err != nil {
		t.Fatalf("LoadSession: %v", err)
	}
	if updated.ActiveModel != "openai/gpt-4o" {
		t.Fatalf("session activeModel=%q, want %q", updated.ActiveModel, "openai/gpt-4o")
	}

	found := false
	for _, ev := range got {
		if ev.Type == "model.changed" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected a model.changed event")
	}
}

func TestTUITurnRunner_Model_InvalidDoesNotChange(t *testing.T) {
	t.Parallel()

	var got []events.Event
	r := &tuiTurnRunner{
		fs:   vfs.NewFS(),
		run:  types.Run{RunId: "run-test", SessionID: "sess-test"},
		opts: resolveRunChatOptions(WithModel("openai/gpt-5.2")),
		agent: &agent.Agent{
			Model: "openai/gpt-5.2",
		},
		model:            "openai/gpt-5.2",
		setHistoryModel:  func(string) {},
		mustEmit:         func(_ context.Context, ev events.Event) { got = append(got, ev) },
		baseSystemPrompt: "",
	}

	if _, handled := r.handleSlashCommand("/model definitely-not-a-model"); !handled {
		t.Fatalf("expected /model to be handled")
	}
	if r.model != "openai/gpt-5.2" {
		t.Fatalf("runner model=%q, want unchanged", r.model)
	}
	if r.agent.Model != "openai/gpt-5.2" {
		t.Fatalf("agent model=%q, want unchanged", r.agent.Model)
	}

	for _, ev := range got {
		if ev.Type == "model.changed" {
			t.Fatalf("did not expect model.changed event for invalid model")
		}
	}
}
