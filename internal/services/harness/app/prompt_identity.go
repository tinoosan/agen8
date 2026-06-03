package app

import (
	"fmt"
	"strings"
)

type MemberPromptInput struct {
	MemberID       string
	SpaceID        string
	ChannelID      string
	DisplayName    string
	MemberType     string
	LifecycleState string
	Kind           string
	Model          string
}

func ManagedMemberSystemPrompt(input MemberPromptInput) (string, error) {
	identity, err := MemberIdentityBlock(input)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(DefaultMemberModeSystemPrompt()) + "\n\n" + identity, nil
}

func MemberIdentityBlock(input MemberPromptInput) (string, error) {
	memberID := strings.TrimSpace(input.MemberID)
	if memberID == "" {
		return "", fmt.Errorf("member identity block: member id is required")
	}
	spaceID := strings.TrimSpace(input.SpaceID)
	if spaceID == "" {
		return "", fmt.Errorf("member identity block: space id is required")
	}
	channelID := strings.TrimSpace(input.ChannelID)
	if channelID == "" {
		return "", fmt.Errorf("member identity block: channel id is required")
	}
	displayName := strings.TrimSpace(input.DisplayName)
	if displayName == "" {
		return "", fmt.Errorf("member identity block: display name is required")
	}
	memberType := strings.TrimSpace(input.MemberType)
	if memberType == "" {
		return "", fmt.Errorf("member identity block: member type is required")
	}
	lifecycleState := strings.TrimSpace(input.LifecycleState)
	if lifecycleState == "" {
		return "", fmt.Errorf("member identity block: lifecycle state is required")
	}

	lines := []string{
		"<agen8_member_identity>",
		"  <rule id=\"member_identity_authoritative\">You are a registered Agen8 space member. Treat this block as authoritative for your Agen8 identity; do not infer that you are outside the agent graph just because a discovery tool omits your own member record.</rule>",
		"  <rule id=\"member_self_description\">If asked who you are, whether you are the coordinator, what space you are in, or what member you are, answer from this Agen8 member identity. Do not answer that you are merely Claude Code, Codex, a terminal assistant, an observer, or outside the agent graph.</rule>",
		"  <rule id=\"member_coordination_scope\">When using Agen8 tools, act as this member in this space. Use the space tool for member discovery and cross-space coordinator messaging, and preserve this identity in coordination decisions.</rule>",
		"  <rule id=\"member_messages\">When another member sends you a space message, treat it as member-to-member coordination. Use Agen8 tools for acknowledgements, responses, and handoffs when the message expects one.</rule>",
		fmt.Sprintf("  <member id=%q display_name=%q type=%q lifecycle_state=%q />", memberID, displayName, memberType, lifecycleState),
		fmt.Sprintf("  <space id=%q channel_id=%q />", spaceID, channelID),
	}
	if kind := strings.TrimSpace(input.Kind); kind != "" {
		lines = append(lines, fmt.Sprintf("  <harness kind=%q />", kind))
	}
	if model := strings.TrimSpace(input.Model); model != "" {
		lines = append(lines, fmt.Sprintf("  <runtime_model value=%q />", model))
	}
	lines = append(lines, "</agen8_member_identity>")
	return strings.Join(lines, "\n"), nil
}
