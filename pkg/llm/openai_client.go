package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/packages/respjson"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"
	openaiConstant "github.com/openai/openai-go/v3/shared/constant"
	"github.com/tinoosan/agen8/pkg/config"
	"github.com/tinoosan/agen8/pkg/cost"
	"github.com/tinoosan/agen8/pkg/debuglog"
	"github.com/tinoosan/agen8/pkg/llm/types"
)

// Client implements types.LLMClient and types.LLMClientStreaming using the official
// OpenAI Go SDK, pointed at OpenRouter's OpenAI-compatible endpoint.
type Client struct {
	client  *openai.Client
	baseURL string

	// DefaultMaxTokens is used when LLMRequest.MaxTokens is 0.
	DefaultMaxTokens int
	// roleMapper canonicalizes input message roles.
	roleMapper RoleMapper
	// streamEventHandlers indexes stream handlers by normalized event type.
	streamEventHandlers map[string][]StreamEventHandler

	// schemaUnsupported is set when the provider rejects json_schema response_format.
	// Once set, we stop attempting schema requests to avoid repeated 400s.
	schemaUnsupported atomic.Bool
	// compactionUnsupported is set when /responses/compact is not supported by the provider.
	compactionUnsupported atomic.Bool
}

const (
	internalUserRepairPrefixInvalidMessageJSON = "Your last message was not valid JSON"
	internalUserRepairPrefixInvalidJSONOp      = "Your last JSON op was invalid:"
	internalUserRepairPrefixInvalidJSONOpJSON  = "Your last JSON op was not valid JSON"
	internalUserRepairPrefixHostOpResponse     = "HostOpResponse:"
)

var internalUserRepairPrefixes = [...]string{
	internalUserRepairPrefixInvalidMessageJSON,
	internalUserRepairPrefixInvalidJSONOp,
	internalUserRepairPrefixInvalidJSONOpJSON,
	internalUserRepairPrefixHostOpResponse,
}

func NewClientFromEnv() (*Client, error) {
	return NewClientFromEnvWithConfig(OpenAIClientConfig{})
}

func NewClientFromEnvWithConfig(cfg OpenAIClientConfig) (*Client, error) {
	key := strings.TrimSpace(os.Getenv("OPENROUTER_API_KEY"))
	if key == "" {
		return nil, fmt.Errorf("OPENROUTER_API_KEY is required")
	}

	baseURL := strings.TrimSpace(os.Getenv("OPENROUTER_BASE_URL"))
	if baseURL == "" {
		baseURL = "https://openrouter.ai/api/v1"
	}
	baseURL = strings.TrimRight(baseURL, "/")

	// DefaultMaxTokens is the default *output* token budget for a request when the caller does not specify one.
	// Keep this conservative to avoid requesting impossible generations that exceed the model context window.
	defaultMaxTokens := 8192
	if v := strings.TrimSpace(os.Getenv("OPENROUTER_MAX_TOKENS")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			defaultMaxTokens = n
		}
	}

	cli := openai.NewClient(
		option.WithAPIKey(key),
		option.WithBaseURL(baseURL),
	)
	roleMapper := cfg.RoleMapper
	if roleMapper == nil {
		roleMapper = defaultRoleMapper{}
	}

	return &Client{
		client:              &cli,
		baseURL:             baseURL,
		DefaultMaxTokens:    defaultMaxTokens,
		roleMapper:          roleMapper,
		streamEventHandlers: buildResponsesStreamEventHandlerIndex(cfg.StreamEventHandlers),
	}, nil
}

func isOpenRouterBaseURL(baseURL string) bool {
	u := strings.ToLower(strings.TrimSpace(baseURL))
	return strings.Contains(u, "openrouter.ai")
}

func isOpenAIBaseURL(baseURL string) bool {
	u := strings.ToLower(strings.TrimSpace(baseURL))
	return strings.Contains(u, "api.openai.com")
}

func shouldAllowOpenRouterFreeModelDataCollection(model string) bool {
	id := strings.ToLower(strings.TrimSpace(model))
	if !strings.Contains(id, ":free") {
		return false
	}
	// Default on for :free models so OpenRouter can route models gated by free-model data policy.
	return config.ParseBoolEnvDefault("OPENROUTER_FREE_MODEL_DATA_COLLECTION", true)
}

func (c *Client) openRouterRequestOptions(model string) []option.RequestOption {
	if c == nil || !isOpenRouterBaseURL(c.baseURL) {
		return nil
	}
	opts := make([]option.RequestOption, 0, 3)
	if ref := strings.TrimSpace(os.Getenv("OPENROUTER_HTTP_REFERER")); ref != "" {
		opts = append(opts, option.WithHeader("HTTP-Referer", ref))
	}
	if title := strings.TrimSpace(os.Getenv("OPENROUTER_X_TITLE")); title != "" {
		opts = append(opts, option.WithHeader("X-Title", title))
	}
	if shouldAllowOpenRouterFreeModelDataCollection(model) {
		opts = append(opts, option.WithJSONSet("provider", map[string]any{
			"data_collection": "allow",
		}))
	}
	return opts
}

func (c *Client) openRouterChatReasoningOptions(req types.LLMRequest) []option.RequestOption {
	if c == nil || !isOpenRouterBaseURL(c.baseURL) {
		return nil
	}
	effort := strings.TrimSpace(req.ReasoningEffort)
	if effort == "" {
		// OpenRouter Chat can return reasoning summaries across providers even when
		// the model is not explicitly registered as reasoning-capable.
		effort = "medium"
	}
	return []option.RequestOption{
		option.WithJSONSet("reasoning", map[string]any{
			"effort": effort,
		}),
	}
}

func shouldRetryOpenRouterFreeModelPolicy(baseURL, model string, err error) bool {
	if !isOpenRouterBaseURL(baseURL) || !isOpenRouterDataPolicyError(err) {
		return false
	}
	id := strings.ToLower(strings.TrimSpace(model))
	return strings.Contains(id, ":free")
}

func withOpenRouterForcedDataCollection(opts []option.RequestOption) []option.RequestOption {
	out := make([]option.RequestOption, 0, len(opts)+1)
	out = append(out, opts...)
	out = append(out, option.WithJSONSet("provider", map[string]any{
		"data_collection": "allow",
	}))
	return out
}

func isOpenRouterDataPolicyError(err error) bool {
	if err == nil {
		return false
	}
	s := ""
	var apiErr *openai.Error
	if errors.As(err, &apiErr) {
		s = strings.ToLower(strings.TrimSpace(apiErr.Message + " " + apiErr.RawJSON()))
	} else {
		s = strings.ToLower(strings.TrimSpace(err.Error()))
	}
	return strings.Contains(s, "data policy") ||
		strings.Contains(s, "free model publication") ||
		strings.Contains(s, "settings/privacy")
}

func maybeEnableWebSearchModel(baseURL string, model string, enable bool) string {
	model = strings.TrimSpace(model)
	if !enable || model == "" {
		return model
	}
	// OpenRouter supports web search via model variants like ":online".
	if !isOpenRouterBaseURL(baseURL) {
		return model
	}
	if strings.Contains(model, ":online") {
		return model
	}
	// If a model variant is already set (e.g. ":free"), override to ":online" when web search
	// is requested (best-effort; OpenRouter supports exactly one variant suffix).
	if strings.Contains(model, ":") {
		if i := strings.LastIndex(model, ":"); i > 0 {
			return model[:i] + ":online"
		}
	}
	return model + ":online"
}

type responsesInputBuildStats struct {
	toolMsgN           int
	assistantToolCallN int
	functionCallIDs    []string
	functionOutputIDs  []string
}

func buildResponsesInputItems(
	messages []types.LLMMessage,
	canonicalRoleFn func(string) (CanonicalRole, error),
) ([]responses.ResponseInputItemUnionParam, responsesInputBuildStats, error) {
	items := make([]responses.ResponseInputItemUnionParam, 0, len(messages))
	stats := responsesInputBuildStats{}
	for _, m := range messages {
		role, err := canonicalRoleFn(m.Role)
		if err != nil {
			return nil, responsesInputBuildStats{}, err
		}
		if role == RoleTool {
			stats.toolMsgN++
			callID := strings.TrimSpace(m.ToolCallID)
			if callID == "" {
				return nil, responsesInputBuildStats{}, fmt.Errorf("tool message requires toolCallID")
			}
			stats.functionOutputIDs = append(stats.functionOutputIDs, callID)
			items = append(items, responses.ResponseInputItemParamOfFunctionCallOutput(callID, m.Content))
			continue
		}

		switch role {
		case RoleCompaction:
			enc := strings.TrimSpace(m.Content)
			if enc == "" {
				return nil, responsesInputBuildStats{}, fmt.Errorf("compaction message requires encrypted content")
			}
			items = append(items, responses.ResponseInputItemParamOfCompaction(enc))
			continue
		case RoleSystem, RoleDeveloper, RoleAssistant, RoleUser:
			rrole := responses.EasyInputMessageRoleUser
			if role == RoleSystem {
				rrole = responses.EasyInputMessageRoleSystem
			} else if role == RoleDeveloper {
				rrole = responses.EasyInputMessageRoleDeveloper
			} else if role == RoleAssistant {
				rrole = responses.EasyInputMessageRoleAssistant
			}
			items = append(items, responses.ResponseInputItemParamOfMessage(m.Content, rrole))
		default:
			return nil, responsesInputBuildStats{}, fmt.Errorf("unsupported message role %q", m.Role)
		}

		// If the assistant message carried tool calls, include them as function_call items.
			for _, tc := range m.ToolCalls {
				if !strings.EqualFold(tc.Type, "function") {
					continue
				}
			stats.assistantToolCallN++
			callID := strings.TrimSpace(tc.ID)
			name := strings.TrimSpace(tc.Function.Name)
			args := tc.Function.Arguments
			if callID == "" || name == "" {
				continue
			}
			stats.functionCallIDs = append(stats.functionCallIDs, callID)
			items = append(items, responses.ResponseInputItemParamOfFunctionCall(args, callID, name))
		}
	}

	return items, stats, nil
}

func (c *Client) buildParams(req types.LLMRequest) (openai.ChatCompletionNewParams, error) {
	if c == nil || c.client == nil {
		return openai.ChatCompletionNewParams{}, fmt.Errorf("llm client is nil")
	}
	if strings.TrimSpace(req.Model) == "" {
		return openai.ChatCompletionNewParams{}, fmt.Errorf("model is required")
	}
	req.Model = maybeEnableWebSearchModel(c.baseURL, req.Model, req.EnableWebSearch)

	// Message mapping: prepend explicit system prompt as a system message.
	msgs := make([]openai.ChatCompletionMessageParamUnion, 0, len(req.Messages)+1)
	if strings.TrimSpace(req.System) != "" {
		msgs = append(msgs, openai.SystemMessage(req.System))
	}
	for _, m := range req.Messages {
		role, err := c.canonicalRole(m.Role)
		if err != nil {
			return openai.ChatCompletionNewParams{}, err
		}
		switch role {
		case RoleSystem:
			msgs = append(msgs, openai.SystemMessage(m.Content))
		case RoleAssistant:
			// If the assistant message included tool calls, preserve them so that
			// subsequent tool messages (role="tool") can reference tool_call_id.
			if len(m.ToolCalls) != 0 {
				tcps := make([]openai.ChatCompletionMessageToolCallUnionParam, 0, len(m.ToolCalls))
					for _, tc := range m.ToolCalls {
						if !strings.EqualFold(tc.Type, "function") {
							return openai.ChatCompletionNewParams{}, fmt.Errorf("unsupported toolCall type %q", tc.Type)
						}
					id := strings.TrimSpace(tc.ID)
					name := strings.TrimSpace(tc.Function.Name)
					args := tc.Function.Arguments
					if id == "" || name == "" {
						return openai.ChatCompletionNewParams{}, fmt.Errorf("toolCall id and function.name are required")
					}
					tcps = append(tcps, openai.ChatCompletionMessageToolCallUnionParam{
						OfFunction: &openai.ChatCompletionMessageFunctionToolCallParam{
							ID: id,
							Function: openai.ChatCompletionMessageFunctionToolCallFunctionParam{
								Name:      name,
								Arguments: args,
							},
						},
					})
				}
				ap := openai.ChatCompletionAssistantMessageParam{
					ToolCalls: tcps,
				}
				if strings.TrimSpace(m.Content) != "" {
					ap.Content = openai.ChatCompletionAssistantMessageParamContentUnion{
						OfString: param.NewOpt(m.Content),
					}
				}
				msgs = append(msgs, openai.ChatCompletionMessageParamUnion{OfAssistant: &ap})
			} else {
				msgs = append(msgs, openai.AssistantMessage(m.Content))
			}
		case RoleDeveloper:
			msgs = append(msgs, openai.DeveloperMessage(m.Content))
		case RoleTool:
			msgs = append(msgs, openai.ToolMessage(m.Content, strings.TrimSpace(m.ToolCallID)))
		case RoleUser, RoleCompaction:
			msgs = append(msgs, openai.UserMessage(m.Content))
		default:
			return openai.ChatCompletionNewParams{}, fmt.Errorf("unsupported message role %q", m.Role)
		}
	}

	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = c.DefaultMaxTokens
	}
	if maxTokens < 0 {
		return openai.ChatCompletionNewParams{}, fmt.Errorf("maxTokens must be >= 0")
	}

	params := openai.ChatCompletionNewParams{
		Model:    openai.ChatModel(req.Model),
		Messages: msgs,
	}
	if v := strings.TrimSpace(req.ReasoningEffort); v != "" && cost.SupportsReasoningEffort(req.Model) {
		params.ReasoningEffort = shared.ReasoningEffort(v)
	}
	if maxTokens > 0 {
		params.MaxTokens = openai.Int(int64(maxTokens))
	}
	if req.Temperature != 0 {
		params.Temperature = openai.Float(req.Temperature)
	}
	if c.schemaUnsupported.Load() {
		req.ResponseSchema = nil
	}
	if sch := req.ResponseSchema; sch != nil && strings.TrimSpace(sch.Name) != "" && sch.Schema != nil {
		schemaParam := openai.ResponseFormatJSONSchemaJSONSchemaParam{
			Name:   strings.TrimSpace(sch.Name),
			Schema: sch.Schema,
		}
		if strings.TrimSpace(sch.Description) != "" {
			schemaParam.Description = openai.String(strings.TrimSpace(sch.Description))
		}
		if sch.Strict {
			schemaParam.Strict = openai.Bool(true)
		}
		params.ResponseFormat = openai.ChatCompletionNewParamsResponseFormatUnion{
			OfJSONSchema: &openai.ResponseFormatJSONSchemaParam{JSONSchema: schemaParam},
		}
	} else if req.JSONOnly {
		rf := shared.NewResponseFormatJSONObjectParam()
		params.ResponseFormat = openai.ChatCompletionNewParamsResponseFormatUnion{
			OfJSONObject: &rf,
		}
	}

	// Tool/function calling (Chat Completions).
	if len(req.Tools) != 0 {
		tools := make([]openai.ChatCompletionToolUnionParam, 0, len(req.Tools))
			for _, t := range req.Tools {
				if !strings.EqualFold(t.Type, "function") {
					return openai.ChatCompletionNewParams{}, fmt.Errorf("unsupported tool type %q", t.Type)
				}
			name := strings.TrimSpace(t.Function.Name)
			if name == "" {
				return openai.ChatCompletionNewParams{}, fmt.Errorf("tool.function.name is required")
			}
			schema, ok := t.Function.Parameters.(map[string]any)
			if !ok && t.Function.Parameters != nil {
				return openai.ChatCompletionNewParams{}, fmt.Errorf("tool.function.parameters must be a JSON schema object")
			}
			fn := shared.FunctionDefinitionParam{
				Name: name,
			}
			if strings.TrimSpace(t.Function.Description) != "" {
				fn.Description = param.NewOpt(strings.TrimSpace(t.Function.Description))
			}
			if t.Function.Strict {
				fn.Strict = param.NewOpt(true)
			}
			if schema != nil {
				fn.Parameters = shared.FunctionParameters(schema)
			}
			tools = append(tools, openai.ChatCompletionFunctionTool(fn))
		}
		params.Tools = tools

		choice := strings.TrimSpace(req.ToolChoice)
		switch strings.ToLower(choice) {
		case "", "auto":
			params.ToolChoice = openai.ChatCompletionToolChoiceOptionUnionParam{OfAuto: param.NewOpt(string(openai.ChatCompletionToolChoiceOptionAutoAuto))}
		case "none":
			params.ToolChoice = openai.ChatCompletionToolChoiceOptionUnionParam{OfAuto: param.NewOpt(string(openai.ChatCompletionToolChoiceOptionAutoNone))}
		case "required":
			params.ToolChoice = openai.ChatCompletionToolChoiceOptionUnionParam{OfAuto: param.NewOpt(string(openai.ChatCompletionToolChoiceOptionAutoRequired))}
			// Allow multiple tool calls per turn to reduce round-trips. The agent loop
			// already supports handling multiple tool calls in one response.
		default:
			if strings.HasPrefix(strings.ToLower(choice), "function:") {
				funcName := strings.TrimSpace(choice[len("function:"):])
				if funcName == "" {
					return openai.ChatCompletionNewParams{}, fmt.Errorf("toolChoice function name is required")
				}
				params.ToolChoice = openai.ChatCompletionToolChoiceOptionUnionParam{
					OfFunctionToolChoice: &openai.ChatCompletionNamedToolChoiceParam{
						Function: openai.ChatCompletionNamedToolChoiceFunctionParam{Name: funcName},
						Type:     openaiConstant.Function("function"),
					},
				}
				break
			}
			return openai.ChatCompletionNewParams{}, fmt.Errorf("unsupported toolChoice %q", req.ToolChoice)
		}
	}

	return params, nil
}

func requestUsesToolCalling(req types.LLMRequest) bool {
	if len(req.Tools) != 0 {
		return true
	}
	for _, m := range req.Messages {
		if strings.EqualFold(strings.TrimSpace(m.Role), "tool") {
			return true
		}
		if strings.EqualFold(strings.TrimSpace(m.Role), "assistant") && len(m.ToolCalls) != 0 {
			return true
		}
	}
	return false
}

func isInternalUserRepair(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	for _, prefix := range internalUserRepairPrefixes {
		if strings.HasPrefix(s, prefix) {
			return true
		}
	}
	return false
}

func (c *Client) buildResponseParams(req types.LLMRequest) (responses.ResponseNewParams, error) {
	if c == nil || c.client == nil {
		return responses.ResponseNewParams{}, fmt.Errorf("llm client is nil")
	}
	if strings.TrimSpace(req.Model) == "" {
		return responses.ResponseNewParams{}, fmt.Errorf("model is required")
	}
	req.Model = maybeEnableWebSearchModel(c.baseURL, req.Model, req.EnableWebSearch)

	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = c.DefaultMaxTokens
	}
	if maxTokens < 0 {
		return responses.ResponseNewParams{}, fmt.Errorf("maxTokens must be >= 0")
	}

	previousResponseID := strings.TrimSpace(req.PreviousResponseID)
	usesToolCalling := requestUsesToolCalling(req)

	// Message mapping: Responses API uses an input item list with explicit roles.
	// System/developer instructions are provided via `Instructions`.
	allMsgs := req.Messages
	msgs := allMsgs

	lastRealUserIdx := -1
	for i := len(allMsgs) - 1; i >= 0; i-- {
		m := allMsgs[i]
		if !strings.EqualFold(strings.TrimSpace(m.Role), "user") {
			continue
		}
		c := strings.TrimSpace(m.Content)
		if c == "" {
			continue
		}
		if isInternalUserRepair(c) {
			continue
		}
		lastRealUserIdx = i
		break
	}
	// When chaining with previous_response_id:
	// - For non-tool-calling turns, we can send only the newest message (delta-only).
	// - For tool-calling turns, we must send *all* trailing tool outputs so the model
	//   can see every tool result from the previous response's tool calls.
	if previousResponseID != "" && len(msgs) > 0 {
		if usesToolCalling {
			// Collect the contiguous trailing tool messages (role="tool"). For some providers
			// (e.g. Azure), tool outputs alone are rejected unless the corresponding tool call
			// is also present, so we also include the immediately preceding assistant message
			// that contains tool call metadata.
			end := len(msgs)
			i := end - 1
			for i >= 0 && strings.EqualFold(strings.TrimSpace(msgs[i].Role), "tool") {
				i--
			}
			// If we found any trailing tool messages, keep them (and maybe the preceding assistant tool-call msg).
			if i < end-1 {
				start := i + 1
				if i >= 0 && strings.EqualFold(strings.TrimSpace(msgs[i].Role), "assistant") && len(msgs[i].ToolCalls) != 0 {
					start = i
				}
				msgs = msgs[start:]
			} else {
				msgs = msgs[end-1:]
			}
		} else {
			msgs = msgs[len(msgs)-1:]
		}
	}

	// If tools are REQUIRED for this request, ensure the model also sees the user's
	// actual instruction during chained steps (some models ignore/miss it otherwise).
	if previousResponseID != "" && usesToolCalling && strings.EqualFold(strings.TrimSpace(req.ToolChoice), "required") && lastRealUserIdx >= 0 {
		hasUser := false
		for _, m := range msgs {
			if !strings.EqualFold(strings.TrimSpace(m.Role), "user") {
				continue
			}
			c := strings.TrimSpace(m.Content)
			if c == "" || isInternalUserRepair(c) {
				continue
			}
			hasUser = true
			break
		}
		if !hasUser {
			msgs = append([]types.LLMMessage{allMsgs[lastRealUserIdx]}, msgs...)
		}
	}

	// Build input items. For tool calling, we encode tool results as function_call_output items.
	items, _, err := buildResponsesInputItems(msgs, c.canonicalRole)
	if err != nil {
		return responses.ResponseNewParams{}, err
	}

	params := responses.ResponseNewParams{
		Model: shared.ResponsesModel(req.Model),
		Input: responses.ResponseNewParamsInputUnion{
			OfInputItemList: responses.ResponseInputParam(items),
		},
	}
	if previousResponseID != "" {
		params.PreviousResponseID = openai.String(previousResponseID)
	}
	if strings.TrimSpace(req.System) != "" {
		params.Instructions = openai.String(req.System)
	}
	if maxTokens > 0 {
		params.MaxOutputTokens = openai.Int(int64(maxTokens))
	}
	if req.Temperature != 0 {
		params.Temperature = openai.Float(req.Temperature)
	}
	if c.schemaUnsupported.Load() {
		req.ResponseSchema = nil
	}
	if sch := req.ResponseSchema; sch != nil && strings.TrimSpace(sch.Name) != "" && sch.Schema != nil {
		js := responses.ResponseFormatTextJSONSchemaConfigParam{
			Name:   strings.TrimSpace(sch.Name),
			Schema: sch.Schema,
		}
		if sch.Strict {
			js.Strict = openai.Bool(true)
		}
		if strings.TrimSpace(sch.Description) != "" {
			js.Description = openai.String(strings.TrimSpace(sch.Description))
		}
		params.Text = responses.ResponseTextConfigParam{
			Format: responses.ResponseFormatTextConfigUnionParam{
				OfJSONSchema: &js,
			},
		}
	} else if req.JSONOnly {
		rf := shared.NewResponseFormatJSONObjectParam()
		params.Text = responses.ResponseTextConfigParam{
			Format: responses.ResponseFormatTextConfigUnionParam{
				OfJSONObject: &rf,
			},
		}
	}

	// Request reasoning summaries when supported.
	requestReasoning := cost.SupportsReasoningSummary(req.Model)
	if isOpenRouterBaseURL(c.baseURL) {
		// Prefer OpenRouter's live model metadata to avoid stale local flags.
		if supports, known := cost.SupportsReasoningSummaryFromOpenRouter(context.Background(), req.Model); known {
			requestReasoning = supports
		} else {
			// If metadata is unavailable, keep prior OpenRouter behavior and request
			// reasoning by default.
			requestReasoning = true
		}
	}
	if requestReasoning {
		params.Reasoning = shared.ReasoningParam{}
		effort := strings.TrimSpace(req.ReasoningEffort)
		if effort == "" && isOpenRouterBaseURL(c.baseURL) {
			effort = "medium"
		}
		if effort != "" {
			params.Reasoning.Effort = shared.ReasoningEffort(effort)
		}
		// Summary control:
		// - empty => default to auto (current behavior)
		// - "off"/"none" => omit summary fields entirely
		// - auto|concise|detailed => set to requested level
		// - invalid values => default to auto
		sv := strings.ToLower(strings.TrimSpace(req.ReasoningSummary))
		switch sv {
		case "":
			params.Reasoning.Summary = shared.ReasoningSummaryAuto
		case "off", "none":
			// omit
		case "auto", "concise", "detailed":
			params.Reasoning.Summary = shared.ReasoningSummary(sv)
		default:
			params.Reasoning.Summary = shared.ReasoningSummaryAuto
		}
	}

	// Tool/function calling (Responses API).
	if len(req.Tools) != 0 {
		tools := make([]responses.ToolUnionParam, 0, len(req.Tools))
			for _, t := range req.Tools {
				if !strings.EqualFold(t.Type, "function") {
					return responses.ResponseNewParams{}, fmt.Errorf("unsupported tool type %q", t.Type)
				}
			name := strings.TrimSpace(t.Function.Name)
			if name == "" {
				return responses.ResponseNewParams{}, fmt.Errorf("tool.function.name is required")
			}
			schema, ok := t.Function.Parameters.(map[string]any)
			if !ok && t.Function.Parameters != nil {
				return responses.ResponseNewParams{}, fmt.Errorf("tool.function.parameters must be a JSON schema object")
			}
			if schema == nil {
				schema = map[string]any{}
			}
			fn := responses.FunctionToolParam{
				Name:       name,
				Parameters: schema,
				Strict:     param.NewOpt(t.Function.Strict),
			}
			if strings.TrimSpace(t.Function.Description) != "" {
				fn.Description = param.NewOpt(strings.TrimSpace(t.Function.Description))
			}
			tools = append(tools, responses.ToolUnionParam{OfFunction: &fn})
		}
		params.Tools = tools

		choice := strings.TrimSpace(req.ToolChoice)
		switch strings.ToLower(choice) {
		case "", "auto":
			params.ToolChoice = responses.ResponseNewParamsToolChoiceUnion{
				OfToolChoiceMode: param.NewOpt(responses.ToolChoiceOptionsAuto),
			}
		case "none":
			params.ToolChoice = responses.ResponseNewParamsToolChoiceUnion{
				OfToolChoiceMode: param.NewOpt(responses.ToolChoiceOptionsNone),
			}
		case "required":
			params.ToolChoice = responses.ResponseNewParamsToolChoiceUnion{
				OfToolChoiceMode: param.NewOpt(responses.ToolChoiceOptionsRequired),
			}
			// Allow multiple tool calls per turn to reduce round-trips. The agent loop
			// already supports handling multiple tool calls in one response.
		default:
			if strings.HasPrefix(strings.ToLower(choice), "function:") {
				funcName := strings.TrimSpace(choice[len("function:"):])
				if funcName == "" {
					return responses.ResponseNewParams{}, fmt.Errorf("toolChoice function name is required")
				}
				params.ToolChoice = responses.ResponseNewParamsToolChoiceUnion{
					OfFunctionTool: &responses.ToolChoiceFunctionParam{
						Name: funcName,
						Type: openaiConstant.Function("function"),
					},
				}
				break
			}
			return responses.ResponseNewParams{}, fmt.Errorf("unsupported toolChoice %q", req.ToolChoice)
		}
	}

	return params, nil
}

func (c *Client) toResponse(resp *openai.ChatCompletion) (types.LLMResponse, error) {
	if resp == nil {
		return types.LLMResponse{}, fmt.Errorf("response is nil")
	}

	text := ""
	var toolCalls []types.ToolCall
	if len(resp.Choices) != 0 {
		text = strings.TrimSpace(resp.Choices[0].Message.Content)
		for _, tc := range resp.Choices[0].Message.ToolCalls {
			// Function tool calls.
			if strings.TrimSpace(tc.Type) == "function" && strings.TrimSpace(tc.Function.Name) != "" {
				toolCalls = append(toolCalls, types.ToolCall{
					ID:   strings.TrimSpace(tc.ID),
					Type: strings.TrimSpace(tc.Type),
					Function: types.ToolCallFunction{
						Name:      tc.Function.Name,
						Arguments: tc.Function.Arguments,
					},
				})
			}
		}
	}

	out := types.LLMResponse{
		Text:           text,
		ToolCalls:      toolCalls,
		EffectiveModel: strings.TrimSpace(resp.Model),
	}

	if raw := strings.TrimSpace(resp.RawJSON()); raw != "" {
		out.Raw = json.RawMessage(raw)
	}

	// If usage was not provided, these will be 0.
	reasoningTokens := int(resp.Usage.CompletionTokensDetails.ReasoningTokens)
	if resp.Usage.TotalTokens != 0 || resp.Usage.PromptTokens != 0 || resp.Usage.CompletionTokens != 0 || reasoningTokens > 0 {
		out.Usage = &types.LLMUsage{
			InputTokens:     int(resp.Usage.PromptTokens),
			OutputTokens:    int(resp.Usage.CompletionTokens),
			TotalTokens:     int(resp.Usage.TotalTokens),
			ReasoningTokens: reasoningTokens,
		}
	}

	return out, nil
}

func (c *Client) toResponseFromResponses(resp *responses.Response) (types.LLMResponse, error) {
	if resp == nil {
		return types.LLMResponse{}, fmt.Errorf("response is nil")
	}

	text := strings.TrimSpace(resp.OutputText())
	out := types.LLMResponse{
		Text:           text,
		EffectiveModel: strings.TrimSpace(string(resp.Model)),
	}

	// Extract function tool calls from Responses output items.
	for _, it := range resp.Output {
		if strings.TrimSpace(it.Type) != "function_call" {
			continue
		}
		callID := strings.TrimSpace(it.CallID)
		name := strings.TrimSpace(it.Name)
		args := it.Arguments
		if callID == "" || name == "" {
			continue
		}
		out.ToolCalls = append(out.ToolCalls, types.ToolCall{
			ID:   callID, // Responses uses call_id; we store it in ToolCall.ID for the agent loop
			Type: "function",
			Function: types.ToolCallFunction{
				Name:      name,
				Arguments: args,
			},
		})
	}

	// Preserve response ID for follow-up calls via previous_response_id.
	if strings.TrimSpace(resp.ID) != "" {
		out.ResponseID = resp.ID
	}

	if raw := strings.TrimSpace(resp.RawJSON()); raw != "" {
		out.Raw = json.RawMessage(raw)
	}

	// Best-effort: extract URL citations from raw JSON (provider-specific).
	// We keep this tolerant to schema differences across OpenAI-compatible providers.
	if len(out.Raw) != 0 {
		cits, _ := extractURLCitationsFromResponsesRaw(out.Raw)
		if len(cits) != 0 {
			out.Citations = cits
			// Append a Sources section to the text for user-visible citations.
			// Avoid duplicating if the model already included a Sources block.
			if !strings.Contains(strings.ToLower(out.Text), "\nsources:") {
				var b strings.Builder
				if strings.TrimSpace(out.Text) != "" {
					b.WriteString(strings.TrimSpace(out.Text))
					b.WriteString("\n\n")
				}
				b.WriteString("Sources:\n")
				for _, c := range cits {
					if strings.TrimSpace(c.URL) == "" {
						continue
					}
					title := strings.TrimSpace(c.Title)
					if title == "" {
						title = c.URL
					}
					b.WriteString("- [")
					b.WriteString(title)
					b.WriteString("](")
					b.WriteString(c.URL)
					b.WriteString(")\n")
				}
				out.Text = strings.TrimSpace(b.String())
			}
		}
	}

	// Usage is required by the Responses API, but some providers may still return zeros.
	reasoningTokens := int(resp.Usage.OutputTokensDetails.ReasoningTokens)
	if resp.Usage.TotalTokens != 0 || resp.Usage.InputTokens != 0 || resp.Usage.OutputTokens != 0 || reasoningTokens > 0 {
		out.Usage = &types.LLMUsage{
			InputTokens:     int(resp.Usage.InputTokens),
			OutputTokens:    int(resp.Usage.OutputTokens),
			TotalTokens:     int(resp.Usage.TotalTokens),
			ReasoningTokens: reasoningTokens,
		}
	}

	return out, nil
}

func extractURLCitationsFromResponsesRaw(raw json.RawMessage) ([]types.LLMCitation, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var top map[string]any
	if err := json.Unmarshal(raw, &top); err != nil {
		return nil, err
	}
	outAny, ok := top["output"]
	if !ok {
		return nil, nil
	}
	outArr, ok := outAny.([]any)
	if !ok {
		return nil, nil
	}

	seen := map[string]types.LLMCitation{}
	for _, item := range outArr {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		typ, _ := m["type"].(string)
		if strings.TrimSpace(typ) != "message" {
			continue
		}
		contentAny, ok := m["content"]
		if !ok {
			continue
		}
		contentArr, ok := contentAny.([]any)
		if !ok {
			continue
		}
		for _, citem := range contentArr {
			cm, ok := citem.(map[string]any)
			if !ok {
				continue
			}
			ctype, _ := cm["type"].(string)
			if strings.TrimSpace(ctype) != "output_text" {
				continue
			}
			annAny, ok := cm["annotations"]
			if !ok {
				continue
			}
			annArr, ok := annAny.([]any)
			if !ok {
				continue
			}
			for _, ann := range annArr {
				am, ok := ann.(map[string]any)
				if !ok {
					continue
				}
				at, _ := am["type"].(string)
				if strings.TrimSpace(at) != "url_citation" {
					continue
				}
				url, _ := am["url"].(string)
				url = strings.TrimSpace(url)
				if url == "" {
					continue
				}
				title, _ := am["title"].(string)
				title = strings.TrimSpace(title)
				if _, exists := seen[url]; !exists {
					seen[url] = types.LLMCitation{URL: url, Title: title}
				}
			}
		}
	}

	if len(seen) == 0 {
		return nil, nil
	}
	// Stable-ish order: by URL.
	urls := make([]string, 0, len(seen))
	for u := range seen {
		urls = append(urls, u)
	}
	sort.Strings(urls)
	out := make([]types.LLMCitation, 0, len(urls))
	for _, u := range urls {
		out = append(out, seen[u])
	}
	return out, nil
}

func (c *Client) Generate(ctx context.Context, req types.LLMRequest) (types.LLMResponse, error) {
	if c == nil || c.client == nil {
		return types.LLMResponse{}, fmt.Errorf("llm client is nil")
	}

	out, err := c.generateOnce(ctx, req)
	if err != nil && req.ResponseSchema != nil && shouldFallbackFromJSONSchema(err) {
		c.schemaUnsupported.Store(true)
		// Provider/model rejected json_schema (unsupported/invalid). Fall back to JSON object mode.
		req2 := req
		req2.ResponseSchema = nil
		req2.JSONOnly = true
		return c.generateOnce(ctx, req2)
	}
	return out, err
}

func (c *Client) SupportsStreaming() bool {
	return c != nil && c.client != nil
}

func (c *Client) SupportsServerCompaction() bool {
	return c != nil && c.client != nil && !c.compactionUnsupported.Load()
}

func (c *Client) CompactConversation(ctx context.Context, req types.LLMCompactionRequest) (types.LLMCompactionResponse, error) {
	if c == nil || c.client == nil {
		return types.LLMCompactionResponse{}, fmt.Errorf("llm client is nil")
	}
	if c.compactionUnsupported.Load() {
		return types.LLMCompactionResponse{}, fmt.Errorf("server compaction is unsupported")
	}
	if strings.TrimSpace(req.Model) == "" {
		return types.LLMCompactionResponse{}, fmt.Errorf("model is required")
	}
	input, _, err := buildResponsesInputItems(req.Messages, c.canonicalRole)
	if err != nil {
		return types.LLMCompactionResponse{}, err
	}

	params := responses.ResponseCompactParams{
		Model: responses.ResponseCompactParamsModel(strings.TrimSpace(req.Model)),
		Input: responses.ResponseCompactParamsInputUnion{
			OfResponseInputItemArray: input,
		},
	}
	if strings.TrimSpace(req.System) != "" {
		params.Instructions = openai.String(strings.TrimSpace(req.System))
	}

	resp, err := c.client.Responses.Compact(ctx, params)
	if err != nil {
		if shouldFallbackToChat(err) {
			c.compactionUnsupported.Store(true)
		}
		return types.LLMCompactionResponse{}, err
	}

	// Compact API returns all user messages + one compaction item.
	// Preserve user messages from our local state and append the compacted payload.
	out := make([]types.LLMMessage, 0, len(req.Messages)+1)
	for _, m := range req.Messages {
		if strings.EqualFold(strings.TrimSpace(m.Role), "user") {
			out = append(out, types.LLMMessage{
				Role:    "user",
				Content: m.Content,
			})
		}
	}
	for _, item := range resp.Output {
		if strings.TrimSpace(item.Type) != "compaction" {
			continue
		}
		if strings.TrimSpace(item.EncryptedContent) == "" {
			continue
		}
		out = append(out, types.LLMMessage{
			Role:    "compaction",
			Content: item.EncryptedContent,
		})
		break
	}
	if len(out) == 0 {
		return types.LLMCompactionResponse{}, fmt.Errorf("compaction response did not include reusable items")
	}
	return types.LLMCompactionResponse{Messages: out}, nil
}

func (c *Client) generateOnce(ctx context.Context, req types.LLMRequest) (types.LLMResponse, error) {
	if req.ForceChat {
		return c.generateChat(ctx, req)
	}
	// Prefer Responses API (enables reasoning summaries) and fall back to Chat Completions.
	if out, err := c.generateResponses(ctx, req); err == nil {
		return out, nil
	} else if shouldFallbackToChatForRequest(c.baseURL, req, err) {
		return c.generateChat(ctx, req)
	} else {
		return types.LLMResponse{}, err
	}
}

func (c *Client) generateResponses(ctx context.Context, req types.LLMRequest) (types.LLMResponse, error) {
	params, err := c.buildResponseParams(req)
	if err != nil {
		return types.LLMResponse{}, err
	}
	opts := c.openRouterRequestOptions(req.Model)
	resp, err := c.client.Responses.New(ctx, params, opts...)
	if err != nil && shouldRetryOpenRouterFreeModelPolicy(c.baseURL, req.Model, err) {
		resp, err = c.client.Responses.New(ctx, params, withOpenRouterForcedDataCollection(opts)...)
	}
	if err != nil {
		return types.LLMResponse{}, err
	}
	return c.toResponseFromResponses(resp)
}

func (c *Client) generateChat(ctx context.Context, req types.LLMRequest) (types.LLMResponse, error) {
	params, err := c.buildParams(req)
	if err != nil {
		return types.LLMResponse{}, err
	}
	opts := c.openRouterRequestOptions(req.Model)
	opts = append(opts, c.openRouterChatReasoningOptions(req)...)
	resp, err := c.client.Chat.Completions.New(ctx, params, opts...)
	if err != nil && shouldRetryOpenRouterFreeModelPolicy(c.baseURL, req.Model, err) {
		resp, err = c.client.Chat.Completions.New(ctx, params, withOpenRouterForcedDataCollection(opts)...)
	}
	if err != nil {
		return types.LLMResponse{}, err
	}
	return c.toResponse(resp)
}

func (c *Client) onStreamChunk(acc *openai.ChatCompletionAccumulator, chunk openai.ChatCompletionChunk, cb types.LLMStreamCallback) error {
	if acc != nil {
		_ = acc.AddChunk(chunk)
	}
	if cb == nil || len(chunk.Choices) == 0 {
		return nil
	}

	delta := chunk.Choices[0].Delta

	// Some OpenAI-compatible providers (including OpenRouter-backed reasoning models)
	// return reasoning fields that aren't part of the standard SDK struct.
	//
	// IMPORTANT: we do not forward raw reasoning text. We only:
	// - emit a reasoning signal (IsReasoning=true, Text="")
	// - forward an explicit reasoning summary if provided separately
	// Summary (safe to show if the provider returns it explicitly).
	// Some providers send this as a plain string, while others send an object/array payload.
	for _, s := range deltaSummaryStrings(delta, "reasoning_summary") {
		if err := cb(types.LLMStreamChunk{Text: s, IsReasoning: true}); err != nil {
			return err
		}
	}
	for _, s := range deltaSummaryStrings(delta, "reasoning_summary_text") {
		if err := cb(types.LLMStreamChunk{Text: s, IsReasoning: true}); err != nil {
			return err
		}
	}
	for _, s := range deltaReasoningDetailsSummaryStrings(delta) {
		if err := cb(types.LLMStreamChunk{Text: s, IsReasoning: true}); err != nil {
			return err
		}
	}

	// Raw reasoning (do not display).
	if s, ok := deltaString(delta, "reasoning_content"); ok && strings.TrimSpace(s) != "" {
		if err := cb(types.LLMStreamChunk{IsReasoning: true}); err != nil {
			return err
		}
	}
	if s, ok := deltaString(delta, "reasoning"); ok && strings.TrimSpace(s) != "" {
		if err := cb(types.LLMStreamChunk{IsReasoning: true}); err != nil {
			return err
		}
	}

	// Standard streamed content.
	// Important: do NOT TrimSpace here. Providers can stream single-space deltas,
	// and the agent's JSON-string decoder relies on receiving them.
	if delta.Content != "" {
		return cb(types.LLMStreamChunk{Text: delta.Content, IsReasoning: false})
	}
	return nil
}

func (c *Client) onResponsesStreamEvent(ev responses.ResponseStreamEventUnion, cb types.LLMStreamCallback, outText *strings.Builder, completed **responses.Response, sawReasoningSummaryText *bool) error {
	streamCtx := &ResponsesStreamEventContext{
		Callback:                cb,
		OutText:                 outText,
		Completed:               completed,
		SawReasoningSummaryText: sawReasoningSummaryText,
		AllowRawReasoningFallback: isOpenRouterBaseURL(c.baseURL),
	}
	handled, err := c.dispatchResponsesStreamEvent(ev, streamCtx)
	if err != nil {
		return err
	}
	if handled || cb == nil {
		return nil
	}
	return emitReasoningSummaryFromRawEvent(ev, cb, sawReasoningSummaryText)
}

func emitReasoningSummaryFromRawEvent(ev responses.ResponseStreamEventUnion, cb types.LLMStreamCallback, sawReasoningSummaryText *bool) error {
	if cb == nil {
		return nil
	}
	t := strings.ToLower(strings.TrimSpace(ev.Type))
	if t == "" || !strings.Contains(t, "reasoning_summary") {
		return nil
	}
	out := make([]string, 0, 2)
	if s := strings.TrimSpace(ev.Delta); s != "" {
		out = append(out, s)
	}
	if s := strings.TrimSpace(ev.Text); s != "" {
		out = append(out, s)
	}
	if s := strings.TrimSpace(ev.Part.Text); s != "" {
		out = append(out, s)
	}
	items := uniqueNonEmptyStrings(out)
	for i, s := range items {
		if sawReasoningSummaryText != nil {
			*sawReasoningSummaryText = true
		}
		if i > 0 {
			s = "\n\n" + s
		}
		if err := cb(types.LLMStreamChunk{IsReasoning: true, Text: s}); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) GenerateStream(ctx context.Context, req types.LLMRequest, cb types.LLMStreamCallback) (types.LLMResponse, error) {
	if c == nil || c.client == nil {
		return types.LLMResponse{}, fmt.Errorf("llm client is nil")
	}

	// #region agent log
	debuglog.Log("toolcalling", "H2", "openai_client.go:GenerateStream", "route_selected", map[string]any{
		"route":      "responses",
		"model":      strings.TrimSpace(req.Model),
		"hasSchema":  req.ResponseSchema != nil,
		"jsonOnly":   req.JSONOnly,
		"toolsLen":   len(req.Tools),
		"toolChoice": strings.TrimSpace(req.ToolChoice),
	})
	// #endregion

	// When ResponseSchema is set, prefer enforcing it if possible. Some OpenAI-compatible
	// providers reject `json_schema` on the Responses API but accept it on Chat Completions.
	if req.ResponseSchema != nil {
		// Try Responses API first (keeps reasoning summaries when supported).
		out, err := c.generateStreamResponses(ctx, req, cb)
		if err == nil {
			return out, nil
		}
		if shouldFallbackFromJSONSchema(err) {
			c.schemaUnsupported.Store(true)
			// Fallback 1: try Chat Completions streaming with the schema.
			out2, err2 := c.generateStreamChat(ctx, req, cb)
			if err2 == nil {
				return out2, nil
			}
			// Fallback 2: drop schema and try normal streaming selection.
			req2 := req
			req2.ResponseSchema = nil
			req2.JSONOnly = true
			return c.generateStreamOnce(ctx, req2, cb)
		}
		// Non-schema error: fall back to normal selection.
		return c.generateStreamOnce(ctx, req, cb)
	}

	// No schema: current behavior.
	return c.generateStreamOnce(ctx, req, cb)
}

func (c *Client) generateStreamOnce(ctx context.Context, req types.LLMRequest, cb types.LLMStreamCallback) (types.LLMResponse, error) {
	if req.ForceChat {
		return c.generateStreamChat(ctx, req, cb)
	}
	// Prefer Responses API (enables reasoning summaries) and fall back to Chat Completions.
	if out, err := c.generateStreamResponses(ctx, req, cb); err == nil {
		return out, nil
	} else if shouldFallbackToChatForRequest(c.baseURL, req, err) {
		return c.generateStreamChat(ctx, req, cb)
	} else {
		return types.LLMResponse{}, err
	}
}

func shouldFallbackToChatForRequest(baseURL string, req types.LLMRequest, err error) bool {
	// For native OpenAI, keep reasoning paths strictly on Responses API.
	if isOpenAIBaseURL(baseURL) {
		return false
	}
	// OpenRouter policy failures are not route incompatibilities; avoid wasteful route fallback.
	if isOpenRouterDataPolicyError(err) {
		return false
	}
	if isOpenRouterBaseURL(baseURL) && shouldFallbackToChat(err) {
		return true
	}
	// Reasoning-capable models should remain on Responses to preserve summary events.
	if cost.SupportsReasoningSummary(req.Model) {
		return false
	}
	return shouldFallbackToChat(err)
}

func (c *Client) generateStreamResponses(ctx context.Context, req types.LLMRequest, cb types.LLMStreamCallback) (types.LLMResponse, error) {
	params, err := c.buildResponseParams(req)
	if err != nil {
		return types.LLMResponse{}, err
	}

	start := time.Now()
	evN := 0
	// #region agent log
	debuglog.Log("toolcalling", "H6", "openai_client.go:generateStreamResponses", "responses_stream_start", map[string]any{
		"model": strings.TrimSpace(req.Model),
	})
	// #endregion

	stream := c.client.Responses.NewStreaming(ctx, params, c.openRouterRequestOptions(req.Model)...)
	if stream == nil {
		return types.LLMResponse{}, fmt.Errorf("stream is nil")
	}

	var outText strings.Builder
	var completed *responses.Response
	sawReasoningSummaryText := false

	for stream.Next() {
		evN++
		ev := stream.Current()
		if err := c.onResponsesStreamEvent(ev, cb, &outText, &completed, &sawReasoningSummaryText); err != nil {
			// #region agent log
			debuglog.Log("toolcalling", "H6", "openai_client.go:generateStreamResponses", "responses_stream_event_err", map[string]any{
				"durMs": time.Since(start).Milliseconds(),
				"evN":   evN,
				"err":   err.Error(),
			})
			// #endregion
			return types.LLMResponse{}, err
		}
	}
	if err := stream.Err(); err != nil {
		// #region agent log
		debuglog.Log("toolcalling", "H6", "openai_client.go:generateStreamResponses", "responses_stream_err", map[string]any{
			"durMs": time.Since(start).Milliseconds(),
			"evN":   evN,
			"err":   err.Error(),
		})
		// #endregion
		return types.LLMResponse{}, err
	}

	if cb != nil && completed != nil && !sawReasoningSummaryText {
		for i, s := range responseReasoningSummaryTextsFromResponse(completed) {
			if i > 0 {
				s = "\n\n" + s
			}
			if err := cb(types.LLMStreamChunk{IsReasoning: true, Text: s}); err != nil {
				return types.LLMResponse{}, err
			}
			sawReasoningSummaryText = true
		}
	}

	if cb != nil {
		if err := cb(types.LLMStreamChunk{Done: true}); err != nil {
			return types.LLMResponse{}, err
		}
	}

	// #region agent log
	debuglog.Log("toolcalling", "H6", "openai_client.go:generateStreamResponses", "responses_stream_end", map[string]any{
		"durMs":        time.Since(start).Milliseconds(),
		"evN":          evN,
		"hasCompleted": completed != nil,
		"outTextLen":   outText.Len(),
	})
	// #endregion

	if completed != nil {
		return c.toResponseFromResponses(completed)
	}
	// Fallback: return whatever we observed as output text.
	return types.LLMResponse{
		Text:           strings.TrimSpace(outText.String()),
		EffectiveModel: strings.TrimSpace(req.Model),
	}, nil
}

func (c *Client) generateStreamChat(ctx context.Context, req types.LLMRequest, cb types.LLMStreamCallback) (types.LLMResponse, error) {
	params, err := c.buildParams(req)
	if err != nil {
		return types.LLMResponse{}, err
	}

	opts := c.openRouterRequestOptions(req.Model)
	opts = append(opts, c.openRouterChatReasoningOptions(req)...)
	stream := c.client.Chat.Completions.NewStreaming(ctx, params, opts...)
	if stream == nil {
		return types.LLMResponse{}, fmt.Errorf("stream is nil")
	}

	var acc openai.ChatCompletionAccumulator

	for stream.Next() {
		chunk := stream.Current()
		if err := c.onStreamChunk(&acc, chunk, cb); err != nil {
			return types.LLMResponse{}, err
		}
	}
	if err := stream.Err(); err != nil {
		return types.LLMResponse{}, err
	}

	if cb != nil {
		if err := cb(types.LLMStreamChunk{Done: true}); err != nil {
			return types.LLMResponse{}, err
		}
	}

	return c.toResponse(&acc.ChatCompletion)
}

func shouldFallbackToChat(err error) bool {
	if err == nil {
		return false
	}
	var apierr *openai.Error
	if errors.As(err, &apierr) {
		// Common: OpenRouter/provider doesn't support /responses.
		if apierr.StatusCode == 404 {
			return true
		}
	}
	// Conservative string matching for unknown routes/unsupported params.
	s := strings.ToLower(err.Error())
	switch {
	case strings.Contains(s, " responses"),
		strings.Contains(s, "/responses"),
		strings.Contains(s, "not found"),
		strings.Contains(s, "unknown route"),
		strings.Contains(s, "unknown endpoint"),
		strings.Contains(s, "unsupported"):
		return true
	default:
		return false
	}
}

func shouldFallbackFromJSONSchema(err error) bool {
	if err == nil {
		return false
	}

	// Prefer structured inspection of OpenAI-compatible API errors.
	var apierr *openai.Error
	if errors.As(err, &apierr) {
		switch apierr.StatusCode {
		case 400, 404, 422:
			// Likely: provider doesn't support json_schema, or rejected strict/schema subset.
		default:
			return false
		}
		// Avoid calling apierr.Error() here: it depends on Request/Response being non-nil.
		msg := strings.ToLower(strings.TrimSpace(apierr.Message))
		param := strings.ToLower(strings.TrimSpace(apierr.Param))
		typ := strings.ToLower(strings.TrimSpace(apierr.Type))
		raw := strings.ToLower(strings.TrimSpace(apierr.RawJSON()))
		s := msg + " " + param + " " + typ + " " + raw
		ok := strings.Contains(s, "json_schema") ||
			strings.Contains(s, "response_format") ||
			strings.Contains(s, "structured") ||
			strings.Contains(s, "schema") ||
			strings.Contains(s, "strict")
		return ok
	}

	// Fallback: conservative string matching.
	s := strings.ToLower(strings.TrimSpace(err.Error()))
	ok := strings.Contains(s, "json_schema") ||
		strings.Contains(s, "response_format") ||
		strings.Contains(s, "structured") ||
		strings.Contains(s, "schema") ||
		strings.Contains(s, "strict")
	return ok
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return truncateErr(err.Error())
}

func truncateErr(s string) string {
	const max = 240
	s = strings.TrimSpace(s)
	if len(s) > max {
		return s[:max] + "…"
	}
	return s
}

func responseFormatKindResponses(p responses.ResponseNewParams) string {
	// p.Text is zero unless JSONOnly/ResponseSchema are set.
	if p.Text.Format.OfJSONSchema != nil {
		return "json_schema"
	}
	if p.Text.Format.OfJSONObject != nil {
		return "json_object"
	}
	if p.Text.Format.OfText != nil {
		return "text"
	}
	return "none"
}

func responseFormatKindChat(p openai.ChatCompletionNewParams) string {
	if p.ResponseFormat.OfJSONSchema != nil {
		return "json_schema"
	}
	if p.ResponseFormat.OfJSONObject != nil {
		return "json_object"
	}
	return "none"
}

func extraString(fields map[string]respjson.Field, key string) (string, bool) {
	if fields == nil || strings.TrimSpace(key) == "" {
		return "", false
	}
	f, ok := fields[key]
	if !ok || !f.Valid() {
		return "", false
	}
	raw := strings.TrimSpace(f.Raw())
	if raw == "" || raw == "null" {
		return "", false
	}
	var s string
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		return "", false
	}
	if strings.TrimSpace(s) == "" {
		return "", false
	}
	return s, true
}

func deltaString(delta openai.ChatCompletionChunkChoiceDelta, key string) (string, bool) {
	if strings.TrimSpace(key) == "" {
		return "", false
	}

	// Preferred: parsed extra fields (when available).
	if fields := delta.JSON.ExtraFields; fields != nil {
		if s, ok := extraString(fields, key); ok {
			return s, true
		}
	}

	// Fallback: parse raw delta JSON (some providers may not populate ExtraFields).
	raw := strings.TrimSpace(delta.RawJSON())
	if raw == "" {
		return "", false
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return "", false
	}
	b, ok := m[key]
	if !ok || len(b) == 0 || strings.TrimSpace(string(b)) == "null" {
		return "", false
	}
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return "", false
	}
	if strings.TrimSpace(s) == "" {
		return "", false
	}
	return s, true
}

func deltaSummaryStrings(delta openai.ChatCompletionChunkChoiceDelta, key string) []string {
	if strings.TrimSpace(key) == "" {
		return nil
	}

	// Preferred: parsed extra fields (when available).
	if fields := delta.JSON.ExtraFields; fields != nil {
		if out := extraSummaryStrings(fields, key); len(out) > 0 {
			return out
		}
	}

	// Fallback: parse raw delta JSON (some providers may not populate ExtraFields).
	raw := strings.TrimSpace(delta.RawJSON())
	if raw == "" {
		return nil
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return nil
	}
	b, ok := m[key]
	if !ok || len(b) == 0 || strings.TrimSpace(string(b)) == "null" {
		return nil
	}
	return extractSummaryStringsRaw(b)
}

func deltaReasoningDetailsSummaryStrings(delta openai.ChatCompletionChunkChoiceDelta) []string {
	const key = "reasoning_details"
	raw := []byte{}
	if fields := delta.JSON.ExtraFields; fields != nil {
		if f, ok := fields[key]; ok && f.Valid() {
			if s := strings.TrimSpace(f.Raw()); s != "" && s != "null" {
				raw = []byte(s)
			}
		}
	}
	if len(raw) == 0 {
		deltaRaw := strings.TrimSpace(delta.RawJSON())
		if deltaRaw == "" {
			return nil
		}
		var m map[string]json.RawMessage
		if err := json.Unmarshal([]byte(deltaRaw), &m); err != nil {
			return nil
		}
		if b, ok := m[key]; ok && len(b) > 0 && strings.TrimSpace(string(b)) != "null" {
			raw = b
		}
	}
	if len(raw) == 0 {
		return nil
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil
	}
	out := make([]string, 0, 2)
	appendReasoningDetailSummaries(v, &out)
	return uniqueNonEmptyStrings(out)
}

func appendReasoningDetailSummaries(v any, out *[]string) {
	switch x := v.(type) {
	case []any:
		for _, item := range x {
			appendReasoningDetailSummaries(item, out)
		}
	case map[string]any:
		typ := strings.ToLower(strings.TrimSpace(anyToString(x["type"])))
		switch typ {
		case "summary_text", "reasoning_summary", "summary":
			if s := strings.TrimSpace(anyToString(x["text"])); s != "" {
				*out = append(*out, s)
			}
			if s := strings.TrimSpace(anyToString(x["summary_text"])); s != "" {
				*out = append(*out, s)
			}
			if vv, ok := x["summary"]; ok {
				appendSummaryStrings(vv, out)
			}
		default:
			// Conservative recursion: only follow explicit summary keys.
			if vv, ok := x["summary"]; ok {
				appendReasoningDetailSummaries(vv, out)
			}
			if vv, ok := x["summary_text"]; ok {
				appendReasoningDetailSummaries(vv, out)
			}
		}
	}
}

func anyToString(v any) string {
	if v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	default:
		return fmt.Sprint(v)
	}
}

func extraSummaryStrings(fields map[string]respjson.Field, key string) []string {
	if fields == nil || strings.TrimSpace(key) == "" {
		return nil
	}
	f, ok := fields[key]
	if !ok || !f.Valid() {
		return nil
	}
	raw := strings.TrimSpace(f.Raw())
	if raw == "" || raw == "null" {
		return nil
	}
	return extractSummaryStringsRaw([]byte(raw))
}

func extractSummaryStringsRaw(raw []byte) []string {
	if len(raw) == 0 || strings.TrimSpace(string(raw)) == "" || strings.TrimSpace(string(raw)) == "null" {
		return nil
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil
	}
	out := make([]string, 0, 2)
	appendSummaryStrings(v, &out)
	return uniqueNonEmptyStrings(out)
}

func appendSummaryStrings(v any, out *[]string) {
	switch x := v.(type) {
	case string:
		if s := strings.TrimSpace(x); s != "" {
			*out = append(*out, s)
		}
	case []any:
		for _, item := range x {
			appendSummaryStrings(item, out)
		}
	case map[string]any:
		// Common summary-carrying keys from OpenAI-compatible providers.
		keys := []string{"text", "summary_text", "summary", "parts", "content", "delta"}
		found := false
		for _, k := range keys {
			if vv, ok := x[k]; ok {
				found = true
				appendSummaryStrings(vv, out)
			}
		}
		if found {
			return
		}
		// Best effort fallback for unknown object shapes: recurse into all values.
		for _, vv := range x {
			appendSummaryStrings(vv, out)
		}
	}
}

func uniqueNonEmptyStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		ss := strings.TrimSpace(s)
		if ss == "" {
			continue
		}
		if _, ok := seen[ss]; ok {
			continue
		}
		seen[ss] = struct{}{}
		out = append(out, ss)
	}
	return out
}

func responseReasoningSummaryTexts(item responses.ResponseOutputItemUnion) []string {
	if strings.EqualFold(strings.TrimSpace(item.Type), "reasoning") && len(item.Summary) > 0 {
		out := make([]string, 0, len(item.Summary))
		for _, s := range item.Summary {
			if text := strings.TrimSpace(s.Text); text != "" {
				out = append(out, text)
			}
		}
		if len(out) > 0 {
			return uniqueNonEmptyStrings(out)
		}
	}
	switch v := item.AsAny().(type) {
	case responses.ResponseReasoningItem:
		out := make([]string, 0, len(v.Summary))
		for _, s := range v.Summary {
			if text := strings.TrimSpace(s.Text); text != "" {
				out = append(out, text)
			}
		}
		return uniqueNonEmptyStrings(out)
	default:
		return nil
	}
}

func responseReasoningSummaryTextsFromResponse(resp *responses.Response) []string {
	if resp == nil {
		return nil
	}
	out := make([]string, 0, 2)
	for _, item := range resp.Output {
		out = append(out, responseReasoningSummaryTexts(item)...)
	}
	if len(out) == 0 {
		out = append(out, responseReasoningSummaryTextsFromRaw([]byte(resp.RawJSON()))...)
	}
	return uniqueNonEmptyStrings(out)
}

func responseReasoningSummaryTextsFromRaw(raw []byte) []string {
	if len(raw) == 0 || strings.TrimSpace(string(raw)) == "" || strings.TrimSpace(string(raw)) == "null" {
		return nil
	}
	var top map[string]any
	if err := json.Unmarshal(raw, &top); err != nil {
		return nil
	}
	outputAny, ok := top["output"]
	if !ok {
		return nil
	}
	outputArr, ok := outputAny.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, 2)
	for _, itemAny := range outputArr {
		item, ok := itemAny.(map[string]any)
		if !ok {
			continue
		}
		typ := strings.ToLower(strings.TrimSpace(anyToString(item["type"])))
		if typ != "reasoning" {
			continue
		}
		if summaryAny, ok := item["summary"]; ok {
			appendSummaryStrings(summaryAny, &out)
		}
		if summaryTextAny, ok := item["summary_text"]; ok {
			appendSummaryStrings(summaryTextAny, &out)
		}
	}
	return uniqueNonEmptyStrings(out)
}
