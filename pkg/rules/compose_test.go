package rules

import "testing"

func TestCompose_RendersAndTracksProvenance(t *testing.T) {
	reg, err := NewRegistry([]Rule[testCtx, testKey]{
		{Name: "turn_contract", Order: 10, AppliesTo: []testKey{"worker"}, Lines: []string{"- first"}},
		{Name: "delegation", Order: 20, AppliesTo: []testKey{"worker"}, Build: func(_ testCtx) string { return "- second\n" }},
	})
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}

	out, err := Compose(ComposeOptions[testCtx, testKey]{
		Registry: reg,
		Key:      "worker",
		Context:  testCtx{},
		Append: []AppendOverride{
			{Name: "delegation", Lines: []string{"- extra"}, Source: Source{Kind: "template.yaml", Path: "template.yaml", Line: 14}},
		},
	})
	if err != nil {
		t.Fatalf("Compose: %v", err)
	}
	if out.Prompt == "" {
		t.Fatalf("expected prompt")
	}
	const wantPrompt = "- first\n- second\n- extra"
	if out.Prompt != wantPrompt {
		t.Fatalf("prompt=%q want=%q", out.Prompt, wantPrompt)
	}
	if len(out.Provenance) != 3 {
		t.Fatalf("provenance len=%d want=3", len(out.Provenance))
	}
}

func TestCompose_DisableLockedRuleFails(t *testing.T) {
	reg, err := NewRegistry([]Rule[testCtx, testKey]{
		{Name: "turn_contract", Order: 10, AppliesTo: []testKey{"worker"}, Locked: true, Lines: []string{"- first"}},
	})
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	_, err = Compose(ComposeOptions[testCtx, testKey]{
		Registry: reg,
		Key:      "worker",
		Disable:  []DisableOverride{{Name: "turn_contract"}},
	})
	if err == nil {
		t.Fatalf("expected locked error")
	}
}
