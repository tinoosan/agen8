package membertype

import (
	"sync"

	"github.com/tinoosan/agen8-mcp-server/pkg/rules"
)

type PromptRule = rules.Rule[PromptContext, MemberTypeName]
type PromptRegistry = rules.Registry[PromptContext, MemberTypeName]
type PromptComposeOptions = rules.ComposeOptions[PromptContext, MemberTypeName]
type PromptComposeResult = rules.ComposeResult
type PromptProvenance = rules.Provenance
type PromptSource = rules.Source
type PromptRuleOverride = rules.RuleOverride[PromptContext, MemberTypeName]

type PromptDisableOverride = rules.DisableOverride
type PromptAppendOverride = rules.AppendOverride

var (
	defaultPromptRegistryOnce sync.Once
	defaultPromptRegistry     *PromptRegistry
	defaultPromptRegistryErr  error
)

// DefaultRegistry returns the built-in prompt rule registry.
func DefaultRegistry() (*PromptRegistry, error) {
	defaultPromptRegistryOnce.Do(func() {
		defaultPromptRegistry, defaultPromptRegistryErr = rules.NewRegistry(allRegisteredRules())
	})
	if defaultPromptRegistryErr != nil {
		return nil, defaultPromptRegistryErr
	}
	return defaultPromptRegistry, nil
}

// MustDefaultRegistry returns the built-in registry or panics on construction errors.
func MustDefaultRegistry() *PromptRegistry {
	reg, err := DefaultRegistry()
	if err != nil {
		panic(err)
	}
	return reg
}

func ComposePromptRules(opts PromptComposeOptions) (PromptComposeResult, error) {
	return rules.Compose(opts)
}
