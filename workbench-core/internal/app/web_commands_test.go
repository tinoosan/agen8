package app

import (
	"context"
	"testing"

	"github.com/tinoosan/workbench-core/internal/events"
	"github.com/tinoosan/workbench-core/internal/types"
	"github.com/tinoosan/workbench-core/internal/vfs"
	"github.com/tinoosan/workbench-core/pkg/agent"
)

func TestLazyRunner_Web_TogglesWithoutInitializingSession(t *testing.T) {
	t.Parallel()

	ch := make(chan events.Event, 10)
	r := &lazyNewSessionTurnRunner{
		ctx:  context.Background(),
		opts: resolveRunChatOptions(),
		evCh: ch,
	}
	if r.initialized {
		t.Fatalf("expected runner to start uninitialized")
	}
	if r.opts.WebSearchEnabled {
		t.Fatalf("expected default web search to be off")
	}

	final, err := r.RunTurn(context.Background(), "/web")
	if err != nil {
		t.Fatalf("RunTurn(/web): %v", err)
	}
	if final != "web search: on" {
		t.Fatalf("final=%q, want %q", final, "web search: on")
	}
	if r.initialized {
		t.Fatalf("expected runner to remain uninitialized")
	}
	if !r.opts.WebSearchEnabled {
		t.Fatalf("expected WebSearchEnabled to be toggled on")
	}

	found := false
	for {
		select {
		case ev := <-ch:
			if ev.Type == "web.changed" {
				found = true
				if ev.Data["enabled"] != "true" {
					t.Fatalf("web.changed enabled=%q, want %q", ev.Data["enabled"], "true")
				}
			}
		default:
			if !found {
				t.Fatalf("expected a web.changed event")
			}
			return
		}
	}
}

func TestTUITurnRunner_Web_TogglesAndUpdatesAgent(t *testing.T) {
	t.Parallel()

	var got []events.Event
	r := &tuiTurnRunner{
		fs:   vfs.NewFS(),
		run:  fakeRunForTests(),
		opts: resolveRunChatOptions(WithModel("openai/gpt-5.1-codex-mini")),
		agent: &agent.Agent{
			Model:           "openai/gpt-5.1-codex-mini",
			EnableWebSearch: false,
		},
		model:            "openai/gpt-5.1-codex-mini",
		setHistoryModel:  func(string) {},
		mustEmit:         func(_ context.Context, ev events.Event) { got = append(got, ev) },
		baseSystemPrompt: "",
	}

	if r.opts.WebSearchEnabled {
		t.Fatalf("expected opts.WebSearchEnabled default off")
	}
	if r.agent.EnableWebSearch {
		t.Fatalf("expected agent.EnableWebSearch default off")
	}

	out, handled := r.handleSlashCommand("/web")
	if !handled {
		t.Fatalf("expected /web to be handled")
	}
	if out != "web search: on" {
		t.Fatalf("out=%q, want %q", out, "web search: on")
	}
	if !r.opts.WebSearchEnabled {
		t.Fatalf("expected opts.WebSearchEnabled on after toggle")
	}
	if !r.agent.EnableWebSearch {
		t.Fatalf("expected agent.EnableWebSearch on after toggle")
	}

	found := false
	for _, ev := range got {
		if ev.Type == "web.changed" {
			found = true
			if ev.Data["enabled"] != "true" {
				t.Fatalf("web.changed enabled=%q, want %q", ev.Data["enabled"], "true")
			}
			break
		}
	}
	if !found {
		t.Fatalf("expected a web.changed event")
	}
}

func fakeRunForTests() (run types.Run) {
	return types.Run{RunId: "run-test", SessionID: "sess-test"}
}

