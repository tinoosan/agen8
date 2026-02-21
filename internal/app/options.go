package app

import (
	"os"
	"strconv"
	"strings"

	"github.com/tinoosan/agen8/pkg/protocol"
)

// RunChatOptions captures runtime options shared by daemon and monitor flows.
// It is intentionally minimal and only includes fields still used outside the chat TUI.
type RunChatOptions struct {
	Model            string
	SubagentModel    string
	Profile          string
	WorkDir          string
	ProtocolStdio    bool
	RPCListen        string
	WebhookAddr      string
	ResultWebhookURL string
	HealthAddr       string
	ApprovalsMode    string
	ReasoningEffort  string
	ReasoningSummary string
	WebSearchEnabled bool

	RecentHistoryPairs int
	IncludeHistoryOps  *bool

	MaxTraceBytes  int
	MaxMemoryBytes int

	PriceInPerMTokensUSD  float64
	PriceOutPerMTokensUSD float64
}

type RunChatOption func(*RunChatOptions) error

func (o RunChatOptions) WithDefaults() RunChatOptions {
	if strings.TrimSpace(o.ApprovalsMode) == "" {
		o.ApprovalsMode = "disabled"
	}
	return o
}

func resolveRunChatOptions(opts ...RunChatOption) (RunChatOptions, error) {
	o := RunChatOptions{
		Model:            strings.TrimSpace(os.Getenv("OPENROUTER_MODEL")),
		SubagentModel:    strings.TrimSpace(os.Getenv("AGEN8_SUBAGENT_MODEL")),
		Profile:          strings.TrimSpace(os.Getenv("AGEN8_PROFILE")),
		WorkDir:          strings.TrimSpace(os.Getenv("AGEN8_WORKDIR")),
		RPCListen:        strings.TrimSpace(os.Getenv("AGEN8_RPC_ENDPOINT")),
		WebhookAddr:      strings.TrimSpace(os.Getenv("AGEN8_WEBHOOK_ADDR")),
		ResultWebhookURL: strings.TrimSpace(os.Getenv("AGEN8_RESULT_WEBHOOK_URL")),
		HealthAddr:       strings.TrimSpace(os.Getenv("AGEN8_HEALTH_ADDR")),
	}
	if strings.TrimSpace(o.RPCListen) == "" {
		o.RPCListen = protocol.DefaultRPCEndpoint
	}
	for _, opt := range opts {
		if opt != nil {
			if err := opt(&o); err != nil {
				return RunChatOptions{}, err
			}
		}
	}
	return o.WithDefaults(), nil
}

func WithModel(model string) RunChatOption {
	return func(o *RunChatOptions) error {
		model = strings.TrimSpace(model)
		if model != "" {
			o.Model = model
		}
		return nil
	}
}

func WithSubagentModel(model string) RunChatOption {
	return func(o *RunChatOptions) error {
		model = strings.TrimSpace(model)
		if model != "" {
			o.SubagentModel = model
		}
		return nil
	}
}

func WithProfile(profile string) RunChatOption {
	return func(o *RunChatOptions) error {
		profile = strings.TrimSpace(profile)
		if profile != "" {
			o.Profile = profile
		}
		return nil
	}
}

func WithWorkDir(dir string) RunChatOption {
	return func(o *RunChatOptions) error {
		dir = strings.TrimSpace(dir)
		if dir != "" {
			o.WorkDir = dir
		}
		return nil
	}
}

func WithProtocolStdio(enabled bool) RunChatOption {
	return func(o *RunChatOptions) error {
		o.ProtocolStdio = enabled
		return nil
	}
}

func WithRPCListen(addr string) RunChatOption {
	return func(o *RunChatOptions) error {
		addr = strings.TrimSpace(addr)
		if addr != "" {
			o.RPCListen = addr
		}
		return nil
	}
}

func WithWebhookAddr(addr string) RunChatOption {
	return func(o *RunChatOptions) error {
		addr = strings.TrimSpace(addr)
		if addr != "" {
			o.WebhookAddr = addr
		}
		return nil
	}
}

func WithResultWebhookURL(url string) RunChatOption {
	return func(o *RunChatOptions) error {
		url = strings.TrimSpace(url)
		if url != "" {
			o.ResultWebhookURL = url
		}
		return nil
	}
}

func WithHealthAddr(addr string) RunChatOption {
	return func(o *RunChatOptions) error {
		addr = strings.TrimSpace(addr)
		if addr != "" {
			o.HealthAddr = addr
		}
		return nil
	}
}

func WithApprovalsMode(mode string) RunChatOption {
	return func(o *RunChatOptions) error {
		mode = strings.TrimSpace(mode)
		if mode != "" {
			o.ApprovalsMode = mode
		}
		return nil
	}
}

func WithReasoningEffort(effort string) RunChatOption {
	return func(o *RunChatOptions) error {
		effort = strings.TrimSpace(effort)
		if effort != "" {
			o.ReasoningEffort = effort
		}
		return nil
	}
}

func WithReasoningSummary(summary string) RunChatOption {
	return func(o *RunChatOptions) error {
		summary = strings.TrimSpace(summary)
		if summary != "" {
			o.ReasoningSummary = summary
		}
		return nil
	}
}

func WithWebSearch(enabled bool) RunChatOption {
	return func(o *RunChatOptions) error {
		o.WebSearchEnabled = enabled
		return nil
	}
}

func WithRecentHistoryPairs(pairs int) RunChatOption {
	return func(o *RunChatOptions) error {
		o.RecentHistoryPairs = pairs
		return nil
	}
}

func WithIncludeHistoryOps(enabled bool) RunChatOption {
	return func(o *RunChatOptions) error {
		o.IncludeHistoryOps = &enabled
		return nil
	}
}

func WithTraceBytes(maxBytes int) RunChatOption {
	return func(o *RunChatOptions) error {
		o.MaxTraceBytes = maxBytes
		return nil
	}
}

func WithMemoryBytes(maxBytes int) RunChatOption {
	return func(o *RunChatOptions) error {
		o.MaxMemoryBytes = maxBytes
		return nil
	}
}

func WithPriceInPerMTokensUSD(price float64) RunChatOption {
	return func(o *RunChatOptions) error {
		o.PriceInPerMTokensUSD = price
		return nil
	}
}

func WithPriceOutPerMTokensUSD(price float64) RunChatOption {
	return func(o *RunChatOptions) error {
		o.PriceOutPerMTokensUSD = price
		return nil
	}
}

func WithEnvWebSearch() RunChatOption {
	return func(o *RunChatOptions) error {
		raw := strings.TrimSpace(os.Getenv("AGEN8_WEB_SEARCH"))
		if raw == "" {
			return nil
		}
		if v, err := strconv.ParseBool(raw); err == nil {
			o.WebSearchEnabled = v
		}
		return nil
	}
}

func derefBool(v *bool, def bool) bool {
	if v == nil {
		return def
	}
	return *v
}
