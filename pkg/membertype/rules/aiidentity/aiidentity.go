package aiidentity

import (
	"strings"

	"github.com/tinoosan/agen8-mcp-server/pkg/membertype"
)

func init() {
	membertype.RegisterRule(membertype.PromptRule{
		Name:      "ai_identity",
		Order:     250,
		AppliesTo: []membertype.MemberTypeName{membertype.TypeCoordinator, membertype.TypeWorker, membertype.TypeLoneCoordinator},
		Locked:    true,
		Build:     build,
	})
}

func build(ctx membertype.PromptContext) string {
	switch ctx.MemberType {
	case membertype.TypeCoordinator, membertype.TypeLoneCoordinator:
		return strings.TrimSpace(membertype.AIIdentityRule(false))
	case membertype.TypeWorker:
		return strings.TrimSpace(membertype.AIIdentityRule(true))
	default:
		return ""
	}
}
