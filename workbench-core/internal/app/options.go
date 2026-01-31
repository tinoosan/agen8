package app

import (
	"os"
	"strconv"
	"strings"
)

// RunChatOptions captures runtime options shared by daemon and monitor flows.
// It is intentionally minimal and only includes fields still used outside the chat TUI.
type RunChatOptions struct {
	Model            string
	Role             string
	WorkDir          string
	WebhookAddr      string
	ResultWebhookURL string
	HealthAddr       string
	ApprovalsMode    string
	ReasoningEffort  string
	ReasoningSummary string
	WebSearchEnabled bool

	RecentHistoryPairs int
	IncludeHistoryOps  *bool

	MaxTraceBytes   int
	MaxMemoryBytes  int
	MaxProfileBytes int

	PriceInPerMTokensUSD  float64
	PriceOutPerMTokensUSD float64
}

type RunChatOption func(*RunChatOptions)

func resolveRunChatOptions(opts ...RunChatOption) RunChatOptions {
	o := RunChatOptions{
		Model:            strings.TrimSpace(os.Getenv("OPENROUTER_MODEL")),
		Role:             strings.TrimSpace(os.Getenv("WORKBENCH_ROLE")),
		WorkDir:          strings.TrimSpace(os.Getenv("WORKBENCH_WORKDIR")),
		WebhookAddr:      strings.TrimSpace(os.Getenv("WORKBENCH_WEBHOOK_ADDR")),
		ResultWebhookURL: strings.TrimSpace(os.Getenv("WORKBENCH_RESULT_WEBHOOK_URL")),
		HealthAddr:       strings.TrimSpace(os.Getenv("WORKBENCH_HEALTH_ADDR")),
		ApprovalsMode:    "disabled",
	}
	for _, opt := range opts {
		if opt != nil {
			opt(&o)
		}
	}
	return o
}

func WithModel(model string) RunChatOption {
	return func(o *RunChatOptions) {
		model = strings.TrimSpace(model)
		if model != "" {
			o.Model = model
		}
	}
}

func WithRole(role string) RunChatOption {
	return func(o *RunChatOptions) {
		role = strings.TrimSpace(role)
		if role != "" {
			o.Role = role
		}
	}
}

func WithWorkDir(dir string) RunChatOption {
	return func(o *RunChatOptions) {
		dir = strings.TrimSpace(dir)
		if dir != "" {
			o.WorkDir = dir
		}
	}
}

func WithWebhookAddr(addr string) RunChatOption {
	return func(o *RunChatOptions) {
		addr = strings.TrimSpace(addr)
		if addr != "" {
			o.WebhookAddr = addr
		}
	}
}

func WithResultWebhookURL(url string) RunChatOption {
	return func(o *RunChatOptions) {
		url = strings.TrimSpace(url)
		if url != "" {
			o.ResultWebhookURL = url
		}
	}
}

func WithHealthAddr(addr string) RunChatOption {
	return func(o *RunChatOptions) {
		addr = strings.TrimSpace(addr)
		if addr != "" {
			o.HealthAddr = addr
		}
	}
}

func WithApprovalsMode(mode string) RunChatOption {
	return func(o *RunChatOptions) {
		mode = strings.TrimSpace(mode)
		if mode != "" {
			o.ApprovalsMode = mode
		}
	}
}

func WithReasoningEffort(effort string) RunChatOption {
	return func(o *RunChatOptions) {
		effort = strings.TrimSpace(effort)
		if effort != "" {
			o.ReasoningEffort = effort
		}
	}
}

func WithReasoningSummary(summary string) RunChatOption {
	return func(o *RunChatOptions) {
		summary = strings.TrimSpace(summary)
		if summary != "" {
			o.ReasoningSummary = summary
		}
	}
}

func WithWebSearch(enabled bool) RunChatOption {
	return func(o *RunChatOptions) {
		o.WebSearchEnabled = enabled
	}
}

func WithRecentHistoryPairs(pairs int) RunChatOption {
	return func(o *RunChatOptions) {
		o.RecentHistoryPairs = pairs
	}
}

func WithIncludeHistoryOps(enabled bool) RunChatOption {
	return func(o *RunChatOptions) {
		o.IncludeHistoryOps = &enabled
	}
}

func WithTraceBytes(maxBytes int) RunChatOption {
	return func(o *RunChatOptions) {
		o.MaxTraceBytes = maxBytes
	}
}

func WithMemoryBytes(maxBytes int) RunChatOption {
	return func(o *RunChatOptions) {
		o.MaxMemoryBytes = maxBytes
	}
}

func WithProfileBytes(maxBytes int) RunChatOption {
	return func(o *RunChatOptions) {
		o.MaxProfileBytes = maxBytes
	}
}

func WithPriceInPerMTokensUSD(price float64) RunChatOption {
	return func(o *RunChatOptions) {
		o.PriceInPerMTokensUSD = price
	}
}

func WithPriceOutPerMTokensUSD(price float64) RunChatOption {
	return func(o *RunChatOptions) {
		o.PriceOutPerMTokensUSD = price
	}
}

func WithEnvWebSearch() RunChatOption {
	return func(o *RunChatOptions) {
		raw := strings.TrimSpace(os.Getenv("WORKBENCH_WEB_SEARCH"))
		if raw == "" {
			return
		}
		if v, err := strconv.ParseBool(raw); err == nil {
			o.WebSearchEnabled = v
		}
	}
}

func derefBool(v *bool, def bool) bool {
	if v == nil {
		return def
	}
	return *v
}
