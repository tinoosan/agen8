package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/tinoosan/workbench-core/pkg/agent"
	"github.com/tinoosan/workbench-core/pkg/config"
	"github.com/tinoosan/workbench-core/pkg/events"
	"github.com/tinoosan/workbench-core/internal/store"
	"github.com/tinoosan/workbench-core/pkg/skills"
	"github.com/tinoosan/workbench-core/pkg/vfs"
)

func TestLazyRunner_Skill_DoesNotInitializeSession(t *testing.T) {
	t.Parallel()

	ch := make(chan events.Event, 10)
	r := &lazyNewSessionTurnRunner{
		ctx:  context.Background(),
		opts: resolveRunChatOptions(),
		evCh: ch,
	}

	final, err := r.RunTurn(context.Background(), "/skill demo-skill")
	if err != nil {
		t.Fatalf("RunTurn(/skill): %v", err)
	}
	if final == "" {
		t.Fatalf("expected non-empty response")
	}
	if r.initialized {
		t.Fatalf("expected runner to remain uninitialized")
	}
	if r.opts.SelectedSkill != "demo-skill" {
		t.Fatalf("opts.SelectedSkill=%q, want %q", r.opts.SelectedSkill, "demo-skill")
	}

	found := false
	for {
		select {
		case ev := <-ch:
			if ev.Type == "skill.changed" {
				found = true
				if ev.Data["skill"] != "demo-skill" {
					t.Fatalf("skill.changed skill=%q, want %q", ev.Data["skill"], "demo-skill")
				}
			}
		default:
			if !found {
				t.Fatalf("expected a skill.changed event")
			}
			return
		}
	}
}

func TestTUITurnRunner_Skill_UpdatesSessionAndEmitsEvent(t *testing.T) {
	t.Parallel()

	cfg := config.Config{DataDir: t.TempDir()}
	_, run, err := store.CreateSession(cfg, "test", 1024)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	skillDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(skillDir, "demo-skill"), 0755); err != nil {
		t.Fatalf("mkdir skill dir: %v", err)
	}
	skillContent := "---\nname: Demo Skill\ndescription: Demo\n---\n# Instructions\nDemo.\n"
	if err := os.WriteFile(filepath.Join(skillDir, "demo-skill", "SKILL.md"), []byte(skillContent), 0644); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}
	mgr := skills.NewManager([]string{skillDir})
	if err := mgr.Scan(); err != nil {
		t.Fatalf("skills scan: %v", err)
	}

	var got []events.Event
	r := &tuiTurnRunner{
		cfg:  cfg,
		fs:   vfs.NewFS(),
		run:  run,
		opts: resolveRunChatOptions(),
		agent: &agent.Agent{
			Model: "openai/gpt-5.1-codex-mini",
		},
		model:            "openai/gpt-5.1-codex-mini",
		setHistoryModel:  func(string) {},
		mustEmit:         func(_ context.Context, ev events.Event) { got = append(got, ev) },
		baseSystemPrompt: "",
		constructor:      &agent.ContextConstructor{SkillsManager: mgr},
	}

	if _, handled := r.handleSlashCommand("/skill demo-skill"); !handled {
		t.Fatalf("expected /skill to be handled")
	}
	if r.opts.SelectedSkill != "demo-skill" {
		t.Fatalf("opts.SelectedSkill=%q, want %q", r.opts.SelectedSkill, "demo-skill")
	}
	if r.constructor != nil && r.constructor.SelectedSkill != "demo-skill" {
		t.Fatalf("constructor.SelectedSkill=%q, want %q", r.constructor.SelectedSkill, "demo-skill")
	}

	found := false
	for _, ev := range got {
		if ev.Type == "skill.changed" {
			found = true
			if ev.Data["skill"] != "demo-skill" {
				t.Fatalf("skill.changed skill=%q, want %q", ev.Data["skill"], "demo-skill")
			}
			break
		}
	}
	if !found {
		t.Fatalf("expected a skill.changed event")
	}

	if _, handled := r.handleSlashCommand("/skill none"); !handled {
		t.Fatalf("expected /skill none to be handled")
	}
	if r.opts.SelectedSkill != "" {
		t.Fatalf("opts.SelectedSkill=%q, want empty", r.opts.SelectedSkill)
	}
	if r.constructor != nil && r.constructor.SelectedSkill != "" {
		t.Fatalf("constructor.SelectedSkill=%q, want empty", r.constructor.SelectedSkill)
	}
}
