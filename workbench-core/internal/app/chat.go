package app

import (
	"fmt"
	"os"
	"strings"

	"github.com/tinoosan/workbench-core/internal/cost"
	"github.com/tinoosan/workbench-core/internal/types"
)

func pricingForModel(modelID, pricingFile string) (inPerM, outPerM float64, known bool, source string) {
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return 0, 0, false, ""
	}

	source = "builtin"
	pf := cost.DefaultPricing()
	if strings.TrimSpace(pricingFile) != "" {
		if fromFile, err := cost.LoadPricingFile(pricingFile); err == nil {
			for k, v := range fromFile.Models {
				pf.Models[k] = v
			}
			source = "file"
		}
	}
	inPerM, outPerM, ok := pf.Lookup(modelID)
	if !ok {
		return 0, 0, false, source
	}
	return inPerM, outPerM, true, source
}

// RunChatOptions controls host-side limits and prompt injection behavior for RunChat.
type RunChatOptions struct {
	// Model is the model identifier used for LLM requests.
	//
	// If empty, the host falls back to OPENROUTER_MODEL.
	Model string

	// WorkDir is the host working directory to mount at /workdir.
	//
	// If empty, the host uses os.Getwd() at startup.
	WorkDir string

	// MaxSteps caps how many agent loop steps are allowed per user turn.
	MaxSteps int

	// MaxTraceBytes caps how many trace bytes ContextUpdater will consider per step.
	MaxTraceBytes int
	// MaxMemoryBytes caps how many run-scoped memory bytes are injected per step.
	MaxMemoryBytes int
	// MaxProfileBytes caps how many global profile bytes are injected per step.
	MaxProfileBytes int

	// RecentHistoryPairs is the number of (user,agent) message pairs to include
	// in the "Recent Conversation" block injected into the system prompt.
	RecentHistoryPairs int

	// UserID is an optional stable identifier for the end user.
	// If set, it is recorded into history/events for provenance.
	UserID string

	// IncludeHistoryOps controls whether the constructor includes environment/host ops
	// from /history in addition to user/agent messages.
	IncludeHistoryOps *bool

	// PriceInPerMTokensUSD is the input token price in USD per 1M tokens.
	//
	// If both PriceInPerMTokensUSD and PriceOutPerMTokensUSD are > 0 and the model returns
	// usage metrics, the host will emit a per-turn cost estimate.
	PriceInPerMTokensUSD float64

	// PriceOutPerMTokensUSD is the output token price in USD per 1M tokens.
	PriceOutPerMTokensUSD float64

	// PricingFile is an optional path to a pricing JSON file.
	//
	// This is used by runtime /model switching to recompute per-turn cost estimation.
	// Pricing is still optional: if no entry exists for a model, cost is "unknown".
	PricingFile string
}

// RunChatOption is a functional option for configuring chat runtime behavior.
//
// Options are resolved at the RunChat* entrypoints via resolveRunChatOptions.
type RunChatOption func(*RunChatOptions)

func defaultRunChatOptions() RunChatOptions {
	return RunChatOptions{
		// Model: empty by default (resolved against session/env at runtime)
		// WorkDir: empty by default (env fallback + Getwd at runtime)
		MaxSteps:           200,
		MaxTraceBytes:      8 * 1024,
		MaxMemoryBytes:     8 * 1024,
		MaxProfileBytes:    4 * 1024,
		RecentHistoryPairs: 8,
		IncludeHistoryOps:  boolPtr(true),
		// Pricing: empty/0 by default (resolved against builtin table at runtime)
	}
}

func resolveRunChatOptions(opts ...RunChatOption) RunChatOptions {
	o := defaultRunChatOptions()
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		opt(&o)
	}

	// Preserve existing env fallbacks.
	if strings.TrimSpace(o.WorkDir) == "" {
		o.WorkDir = strings.TrimSpace(os.Getenv("WORKBENCH_WORKDIR"))
	}
	if strings.TrimSpace(o.PricingFile) == "" {
		o.PricingFile = strings.TrimSpace(os.Getenv("WORKBENCH_PRICING_FILE"))
	}

	// Preserve existing defaults for zero/negative values.
	if o.MaxSteps <= 0 {
		o.MaxSteps = 200
	}
	if o.MaxTraceBytes <= 0 {
		o.MaxTraceBytes = 8 * 1024
	}
	if o.MaxMemoryBytes <= 0 {
		o.MaxMemoryBytes = 8 * 1024
	}
	if o.MaxProfileBytes <= 0 {
		o.MaxProfileBytes = 4 * 1024
	}
	if o.RecentHistoryPairs <= 0 {
		o.RecentHistoryPairs = 8
	}
	if o.IncludeHistoryOps == nil {
		o.IncludeHistoryOps = boolPtr(true)
	}

	return o
}

func WithModel(model string) RunChatOption {
	return func(o *RunChatOptions) {
		o.Model = strings.TrimSpace(model)
	}
}

func WithWorkDir(workDir string) RunChatOption {
	return func(o *RunChatOptions) {
		o.WorkDir = strings.TrimSpace(workDir)
	}
}

func WithMaxSteps(maxSteps int) RunChatOption {
	return func(o *RunChatOptions) {
		o.MaxSteps = maxSteps
	}
}

func WithTraceBytes(maxTraceBytes int) RunChatOption {
	return func(o *RunChatOptions) {
		o.MaxTraceBytes = maxTraceBytes
	}
}

func WithMemoryBytes(maxMemoryBytes int) RunChatOption {
	return func(o *RunChatOptions) {
		o.MaxMemoryBytes = maxMemoryBytes
	}
}

func WithProfileBytes(maxProfileBytes int) RunChatOption {
	return func(o *RunChatOptions) {
		o.MaxProfileBytes = maxProfileBytes
	}
}

func WithRecentHistoryPairs(pairs int) RunChatOption {
	return func(o *RunChatOptions) {
		o.RecentHistoryPairs = pairs
	}
}

func WithUserID(userID string) RunChatOption {
	return func(o *RunChatOptions) {
		o.UserID = strings.TrimSpace(userID)
	}
}

func WithIncludeHistoryOps(include bool) RunChatOption {
	return func(o *RunChatOptions) {
		o.IncludeHistoryOps = boolPtr(include)
	}
}

func WithPricingUSDPerMTokens(priceInPerM, priceOutPerM float64) RunChatOption {
	return func(o *RunChatOptions) {
		o.PriceInPerMTokensUSD = priceInPerM
		o.PriceOutPerMTokensUSD = priceOutPerM
	}
}

func WithPricingFile(pricingFile string) RunChatOption {
	return func(o *RunChatOptions) {
		o.PricingFile = strings.TrimSpace(pricingFile)
	}
}

func (o RunChatOptions) withDefaults() RunChatOptions {
	if strings.TrimSpace(o.WorkDir) == "" {
		o.WorkDir = strings.TrimSpace(os.Getenv("WORKBENCH_WORKDIR"))
	}
	if o.MaxSteps <= 0 {
		o.MaxSteps = 200
	}
	if o.MaxTraceBytes <= 0 {
		o.MaxTraceBytes = 8 * 1024
	}
	if o.MaxMemoryBytes <= 0 {
		o.MaxMemoryBytes = 8 * 1024
	}
	if o.MaxProfileBytes <= 0 {
		o.MaxProfileBytes = 4 * 1024
	}
	if o.RecentHistoryPairs <= 0 {
		o.RecentHistoryPairs = 8
	}
	if o.IncludeHistoryOps == nil {
		o.IncludeHistoryOps = boolPtr(true)
	}
	if strings.TrimSpace(o.PricingFile) == "" {
		o.PricingFile = strings.TrimSpace(os.Getenv("WORKBENCH_PRICING_FILE"))
	}
	return o
}

func boolPtr(v bool) *bool { return &v }

func derefBool(p *bool, def bool) bool {
	if p == nil {
		return def
	}
	return *p
}

func estimateTurnCostUSD(usage types.LLMUsage, priceInPerM, priceOutPerM float64) float64 {
	if usage.InputTokens <= 0 && usage.OutputTokens <= 0 {
		return 0
	}
	if priceInPerM <= 0 && priceOutPerM <= 0 {
		return 0
	}
	in := float64(usage.InputTokens) / 1_000_000.0 * priceInPerM
	out := float64(usage.OutputTokens) / 1_000_000.0 * priceOutPerM
	return in + out
}

func fmtUSD(v float64) string {
	// Keep it stable and compact; this is an estimate based on token usage.
	return fmt.Sprintf("%.6f", v)
}

func fmtBool(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
