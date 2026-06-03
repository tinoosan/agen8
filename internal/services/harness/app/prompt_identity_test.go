package app

import (
	"strings"
	"testing"
)

func TestManagedMemberSystemPromptIncludesAuthoritativeMemberAttributes(t *testing.T) {
	prompt, err := ManagedMemberSystemPrompt(MemberPromptInput{
		MemberID:       "member-1",
		SpaceID:        "space-1",
		ChannelID:      "channel:space-1:member:member-1",
		DisplayName:    "Sarah",
		MemberType:     "coordinator",
		LifecycleState: "active",
		Kind:           "codex",
		Model:          "gpt-5.5",
	})
	if err != nil {
		t.Fatalf("ManagedMemberSystemPrompt: %v", err)
	}

	for _, want := range []string{
		"<system>",
		"<member_autonomous_mode>",
		"<agen8_member_identity>",
		"You are a registered Agen8 space member.",
		`id="member-1" display_name="Sarah" type="coordinator" lifecycle_state="active"`,
		`space id="space-1" channel_id="channel:space-1:member:member-1"`,
		`harness kind="codex"`,
		`runtime_model value="gpt-5.5"`,
		`member_self_description`,
		`Do not answer that you are merely Claude Code, Codex, a terminal assistant, an observer, or outside the agent graph`,
		`member_messages`,
		"</agen8_member_identity>",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestMemberIdentityBlockRequiresCoreAttributes(t *testing.T) {
	_, err := MemberIdentityBlock(MemberPromptInput{
		MemberID:       "member-1",
		SpaceID:        "space-1",
		DisplayName:    "Sarah",
		MemberType:     "worker",
		LifecycleState: "active",
	})
	if err == nil {
		t.Fatal("expected missing channel id error")
	}
	if got := err.Error(); got != "member identity block: channel id is required" {
		t.Fatalf("error=%q", got)
	}
}
