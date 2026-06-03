package membertype

import "fmt"

type PromptRenderOptions struct {
	Add     []PromptRuleOverride
	Disable []PromptDisableOverride
	Append  []PromptAppendOverride
}

func RenderPromptRules(memberType MemberTypeName, ctx PromptContext, opts PromptRenderOptions) (PromptComposeResult, error) {
	registry, err := DefaultRegistry()
	if err != nil {
		return PromptComposeResult{}, err
	}
	ctx.MemberType = memberType
	return ComposePromptRules(PromptComposeOptions{
		Registry: registry,
		Key:      memberType,
		Context:  ctx,
		Add:      opts.Add,
		Disable:  opts.Disable,
		Append:   opts.Append,
	})
}

func renderPromptRules(memberType MemberTypeName, ctx PromptContext) string {
	result, err := RenderPromptRules(memberType, ctx, PromptRenderOptions{})
	if err != nil {
		panic(fmt.Sprintf("membertype prompt composition for %s: %v", memberType, err))
	}
	return result.Prompt
}
