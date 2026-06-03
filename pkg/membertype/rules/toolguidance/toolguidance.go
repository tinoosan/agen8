package toolguidance

import "github.com/tinoosan/agen8-mcp-server/pkg/membertype"

func init() {
	membertype.RegisterRule(membertype.PromptRule{
		Name:      "tool_guidance",
		Order:     600,
		AppliesTo: []membertype.MemberTypeName{membertype.TypeCoordinator, membertype.TypeWorker, membertype.TypeLoneCoordinator},
		Build:     build,
	})
}

func build(ctx membertype.PromptContext) string {
	switch ctx.MemberType {
	case membertype.TypeCoordinator, membertype.TypeLoneCoordinator:
		return membertype.JoinRuleLines([]string{
			"- Runtime tool guidance: exact tool names, parameters, enum values, and availability come from the active runtime or harness schemas. Follow the coordination rules for when to use tool families; follow schemas for how to call them.",
		})
	case membertype.TypeWorker:
		return membertype.JoinRuleLines([]string{
			"- Runtime tool guidance: exact tool names, parameters, enum values, and availability come from the active runtime or harness schemas. Follow your assigned task and role rules; follow schemas for how to call available tools.",
		})
	default:
		return ""
	}
}
