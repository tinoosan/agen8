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
	"github.com/tinoosan/workbench-core/internal/cost"
	"github.com/tinoosan/workbench-core/internal/debuglog"
	"github.com/tinoosan/workbench-core/internal/types"
)

// Client implements types.LLMClient and types.LLMClientStreaming using the official
// OpenAI Go SDK, pointed at OpenRouter's OpenAI-compatible endpoint.
type Client struct {
	client  *openai.Client
	baseURL string

	// DefaultMaxTokens is used when LLMRequest.MaxTokens is 0.
	DefaultMaxTokens int

	// schemaUnsupported is set when the provider rejects json_schema response_format.
	// Once set, we stop attempting schema requests to avoid repeated 400s.
	schemaUnsupported atomic.Bool
}

func NewClientFromEnv() (*Client, error) {
	key := strings.TrimSpace(os.Getenv("OPENROUTER_API_KEY"))
	if key == "" {
		return nil, fmt.Errorf("OPENROUTER_API_KEY is required")
	}

	baseURL := strings.TrimSpace(os.Getenv("OPENROUTER_BASE_URL"))
	if baseURL == "" {
		baseURL = "https://openrouter.ai/api/v1"
	}
	baseURL = strings.TrimRight(baseURL, "/")

	defaultMaxTokens := 256000
	if v := strings.TrimSpace(os.Getenv("OPENROUTER_MAX_TOKENS")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			defaultMaxTokens = n
		}
	}

	cli := openai.NewClient(
		option.WithAPIKey(key),
		option.WithBaseURL(baseURL),
	)

	return &Client{
		client:           &cli,
		baseURL:          baseURL,
		DefaultMaxTokens: defaultMaxTokens,
	}, nil
}

func isOpenRouterBaseURL(baseURL string) bool {
	u := strings.ToLower(strings.TrimSpace(baseURL))
	return strings.Contains(u, "openrouter.ai")
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

func cursorDebugLog(hypothesisId, location, message string, data map[string]any) {
	// #region agent log
	const logPath = "/Users/santinoonyeme/personal/dev/Projects/workbench/.cursor/debug.log"
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	payload := map[string]any{
		"sessionId":    "debug-session",
		"runId":        "pre-fix",
		"hypothesisId": hypothesisId,
		"location":     location,
		"message":      message,
		"data":         data,
		"timestamp":    time.Now().UnixMilli(),
	}
	if b, err := json.Marshal(payload); err == nil {
		_, _ = f.Write(append(b, '\n'))
	}
	// #endregion
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
		role := strings.ToLower(strings.TrimSpace(m.Role))
		switch role {
		case "system":
			msgs = append(msgs, openai.SystemMessage(m.Content))
		case "assistant":
			// If the assistant message included tool calls, preserve them so that
			// subsequent tool messages (role="tool") can reference tool_call_id.
			if len(m.ToolCalls) != 0 {
				tcps := make([]openai.ChatCompletionMessageToolCallUnionParam, 0, len(m.ToolCalls))
				for _, tc := range m.ToolCalls {
					if strings.TrimSpace(strings.ToLower(tc.Type)) != "function" {
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
		case "developer":
			msgs = append(msgs, openai.DeveloperMessage(m.Content))
		case "tool":
			msgs = append(msgs, openai.ToolMessage(m.Content, strings.TrimSpace(m.ToolCallID)))
		default:
			// Treat unknown roles as user.
			msgs = append(msgs, openai.UserMessage(m.Content))
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
			if strings.TrimSpace(strings.ToLower(t.Type)) != "function" {
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

		switch strings.ToLower(strings.TrimSpace(req.ToolChoice)) {
		case "", "auto":
			params.ToolChoice = openai.ChatCompletionToolChoiceOptionUnionParam{OfAuto: param.NewOpt(string(openai.ChatCompletionToolChoiceOptionAutoAuto))}
		case "none":
			params.ToolChoice = openai.ChatCompletionToolChoiceOptionUnionParam{OfAuto: param.NewOpt(string(openai.ChatCompletionToolChoiceOptionAutoNone))}
		case "required":
			params.ToolChoice = openai.ChatCompletionToolChoiceOptionUnionParam{OfAuto: param.NewOpt(string(openai.ChatCompletionToolChoiceOptionAutoRequired))}
			// Allow multiple tool calls per turn to reduce round-trips. The agent loop
			// already supports handling multiple tool calls in one response.
		default:
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

	isInternalUserRepair := func(s string) bool {
		s = strings.TrimSpace(s)
		if s == "" {
			return false
		}
		return strings.HasPrefix(s, "Your last message was not valid JSON") ||
			strings.HasPrefix(s, "Your last JSON op was invalid:") ||
			strings.HasPrefix(s, "Your last JSON op was not valid JSON") ||
			strings.HasPrefix(s, "HostOpResponse:")
	}

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

	// #region agent log
	// Log the role/content shape we are sending (safe; no raw user text).
	// This helps diagnose "model didn't receive a message" reports.
	if previousResponseID != "" {
		roles := make([]string, 0, len(msgs))
		lens := make([]int, 0, len(msgs))
		emptyN := 0
		userN := 0
		toolN := 0
		assistantN := 0
		for _, m := range msgs {
			r := strings.ToLower(strings.TrimSpace(m.Role))
			roles = append(roles, r)
			l := len(strings.TrimSpace(m.Content))
			lens = append(lens, l)
			if l == 0 && r != "tool" {
				emptyN++
			}
			switch r {
			case "user":
				userN++
			case "tool":
				toolN++
			case "assistant":
				assistantN++
			}
		}
		debuglog.Log("toolcalling", "H14", "openai_client.go:buildResponseParams", "responses_input_msgs_shape", map[string]any{
			"prevIDUsed":   true,
			"usesToolCall": usesToolCalling,
			"msgsLen":      len(msgs),
			"userN":        userN,
			"assistantN":   assistantN,
			"toolN":        toolN,
			"emptyN":       emptyN,
			"roles":        roles,
			"contentLens":  lens,
		})
	}
	// #endregion

	// Build input items. For tool calling, we encode tool results as function_call_output items.
	items := make(responses.ResponseInputParam, 0, len(msgs))
	toolMsgN := 0
	assistantToolCallN := 0
	var functionCallIDs []string
	var functionCallOutputIDs []string
	for _, m := range msgs {
		role := strings.ToLower(strings.TrimSpace(m.Role))
		if role == "tool" {
			toolMsgN++
			callID := strings.TrimSpace(m.ToolCallID)
			if callID == "" {
				return responses.ResponseNewParams{}, fmt.Errorf("tool message requires toolCallID")
			}
			functionCallOutputIDs = append(functionCallOutputIDs, callID)
			items = append(items, responses.ResponseInputItemParamOfFunctionCallOutput(callID, m.Content))
			continue
		}
		rrole := responses.EasyInputMessageRoleUser
		switch role {
		case "system":
			rrole = responses.EasyInputMessageRoleSystem
		case "developer":
			rrole = responses.EasyInputMessageRoleDeveloper
		case "assistant":
			rrole = responses.EasyInputMessageRoleAssistant
		default:
			rrole = responses.EasyInputMessageRoleUser
		}
		items = append(items, responses.ResponseInputItemParamOfMessage(m.Content, rrole))
		// If the assistant message carried tool calls, include them as function_call items.
		for _, tc := range m.ToolCalls {
			if strings.TrimSpace(strings.ToLower(tc.Type)) != "function" {
				continue
			}
			assistantToolCallN++
			callID := strings.TrimSpace(tc.ID)
			name := strings.TrimSpace(tc.Function.Name)
			args := tc.Function.Arguments
			if callID == "" || name == "" {
				continue
			}
			functionCallIDs = append(functionCallIDs, callID)
			items = append(items, responses.ResponseInputItemParamOfFunctionCall(args, callID, name))
		}
	}

	// #region agent log
	debuglog.Log("toolcalling", "H7", "openai_client.go:buildResponseParams", "responses_input_shape", map[string]any{
		"usesToolCalling":    usesToolCalling,
		"deltaOnly":          previousResponseID != "" && !usesToolCalling,
		"deltaToolOutputs":   previousResponseID != "" && usesToolCalling,
		"previousResponseID": strings.TrimSpace(req.PreviousResponseID) != "",
		"prevIDUsed":         previousResponseID != "",
		"msgsLenSent":        len(msgs),
		"toolMsgN":           toolMsgN,
		"assistantToolCallN": assistantToolCallN,
		"toolsLen":           len(req.Tools),
		"toolChoice":         strings.TrimSpace(req.ToolChoice),
	})
	debuglog.Log("toolcalling", "H7", "openai_client.go:buildResponseParams", "responses_input_call_ids", map[string]any{
		"functionCallN":       len(functionCallIDs),
		"functionCallOutputN": len(functionCallOutputIDs),
	})
	// #endregion

	params := responses.ResponseNewParams{
		Model: shared.ResponsesModel(req.Model),
		Input: responses.ResponseNewParamsInputUnion{
			OfInputItemList: items,
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
	//
	// Important: many models reject reasoning params. Keep this best-effort and only
	// send reasoning config when we expect the model to support explicit reasoning controls.
	if cost.SupportsReasoningSummary(req.Model) {
		params.Reasoning = shared.ReasoningParam{}
		if v := strings.TrimSpace(req.ReasoningEffort); v != "" {
			params.Reasoning.Effort = shared.ReasoningEffort(v)
		}
		// Summary control:
		// - empty => default to auto (current behavior)
		// - "off" => omit summary fields entirely
		// - otherwise => set to the requested level
		sv := strings.ToLower(strings.TrimSpace(req.ReasoningSummary))
		switch sv {
		case "":
			params.Reasoning.GenerateSummary = shared.ReasoningGenerateSummaryAuto // deprecated but harmless; helps compat
			params.Reasoning.Summary = shared.ReasoningSummaryAuto
		case "off":
			// omit
		default:
			params.Reasoning.GenerateSummary = shared.ReasoningGenerateSummary(sv)
			params.Reasoning.Summary = shared.ReasoningSummary(sv)
		}
	}

	// Tool/function calling (Responses API).
	if len(req.Tools) != 0 {
		tools := make([]responses.ToolUnionParam, 0, len(req.Tools))
		for _, t := range req.Tools {
			if strings.TrimSpace(strings.ToLower(t.Type)) != "function" {
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

		switch strings.ToLower(strings.TrimSpace(req.ToolChoice)) {
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

	out := types.LLMResponse{Text: text, ToolCalls: toolCalls}

	if raw := strings.TrimSpace(resp.RawJSON()); raw != "" {
		out.Raw = json.RawMessage(raw)
	}

	// If usage was not provided, these will be 0.
	if resp.Usage.TotalTokens != 0 || resp.Usage.PromptTokens != 0 || resp.Usage.CompletionTokens != 0 {
		out.Usage = &types.LLMUsage{
			InputTokens:  int(resp.Usage.PromptTokens),
			OutputTokens: int(resp.Usage.CompletionTokens),
			TotalTokens:  int(resp.Usage.TotalTokens),
		}
	}

	return out, nil
}

func (c *Client) toResponseFromResponses(resp *responses.Response) (types.LLMResponse, error) {
	if resp == nil {
		return types.LLMResponse{}, fmt.Errorf("response is nil")
	}

	text := strings.TrimSpace(resp.OutputText())
	out := types.LLMResponse{Text: text}

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
	if resp.Usage.TotalTokens != 0 || resp.Usage.InputTokens != 0 || resp.Usage.OutputTokens != 0 {
		out.Usage = &types.LLMUsage{
			InputTokens:  int(resp.Usage.InputTokens),
			OutputTokens: int(resp.Usage.OutputTokens),
			TotalTokens:  int(resp.Usage.TotalTokens),
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

func (c *Client) generateOnce(ctx context.Context, req types.LLMRequest) (types.LLMResponse, error) {
	// Prefer Responses API (enables reasoning summaries) and fall back to Chat Completions.
	if out, err := c.generateResponses(ctx, req); err == nil {
		return out, nil
	} else if shouldFallbackToChat(err) {
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
	resp, err := c.client.Responses.New(ctx, params)
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
	resp, err := c.client.Chat.Completions.New(ctx, params)
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
	if s, ok := deltaString(delta, "reasoning_summary"); ok {
		if err := cb(types.LLMStreamChunk{Text: s, IsReasoning: true}); err != nil {
			return err
		}
	}
	if s, ok := deltaString(delta, "reasoning_summary_text"); ok {
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
	if cb == nil {
		// Still track completion for final response mapping when desired.
		switch e := ev.AsAny().(type) {
		case responses.ResponseCompletedEvent:
			if completed != nil {
				r := e.Response
				*completed = &r
			}
		}
		return nil
	}

	switch e := ev.AsAny().(type) {
	case responses.ResponseTextDeltaEvent:
		if outText != nil {
			outText.WriteString(e.Delta)
		}
		return cb(types.LLMStreamChunk{Text: e.Delta})
	case responses.ResponseReasoningSummaryTextDeltaEvent:
		// Provider-supplied reasoning summary (safe to show).
		if sawReasoningSummaryText != nil {
			*sawReasoningSummaryText = true
		}
		return cb(types.LLMStreamChunk{IsReasoning: true, Text: e.Delta})
	case responses.ResponseReasoningSummaryPartAddedEvent:
		// Some providers emit summary parts instead of summary_text deltas.
		if strings.TrimSpace(e.Part.Text) == "" {
			return nil
		}
		if sawReasoningSummaryText != nil {
			*sawReasoningSummaryText = true
		}
		return cb(types.LLMStreamChunk{IsReasoning: true, Text: e.Part.Text})
	case responses.ResponseReasoningSummaryPartDoneEvent:
		// Fallback: emit the part text only if we have not seen any summary deltas.
		if sawReasoningSummaryText != nil && *sawReasoningSummaryText {
			return nil
		}
		if strings.TrimSpace(e.Part.Text) == "" {
			return nil
		}
		if sawReasoningSummaryText != nil {
			*sawReasoningSummaryText = true
		}
		return cb(types.LLMStreamChunk{IsReasoning: true, Text: e.Part.Text})
	case responses.ResponseReasoningSummaryTextDoneEvent:
		// Fallback-only: some providers emit only the final completed summary text.
		if sawReasoningSummaryText != nil && *sawReasoningSummaryText {
			return nil
		}
		if strings.TrimSpace(e.Text) == "" {
			return nil
		}
		if sawReasoningSummaryText != nil {
			*sawReasoningSummaryText = true
		}
		return cb(types.LLMStreamChunk{IsReasoning: true, Text: e.Text})
	case responses.ResponseReasoningTextDeltaEvent:
		// Raw reasoning (never show): indicator only.
		return cb(types.LLMStreamChunk{IsReasoning: true})
	case responses.ResponseCompletedEvent:
		if completed != nil {
			r := e.Response
			*completed = &r
		}
		return nil
	default:
		return nil
	}
}

func (c *Client) GenerateStream(ctx context.Context, req types.LLMRequest, cb types.LLMStreamCallback) (types.LLMResponse, error) {
	if c == nil || c.client == nil {
		return types.LLMResponse{}, fmt.Errorf("llm client is nil")
	}

	// #region agent log
	cursorDebugLog("H2", "openai_client.go:GenerateStream", "GenerateStream_request", map[string]any{
		"model":           strings.TrimSpace(req.Model),
		"enableWebSearch": req.EnableWebSearch,
		"baseURL":         strings.TrimSpace(c.baseURL),
		"hasSchema":       req.ResponseSchema != nil,
		"jsonOnly":        req.JSONOnly,
		"toolsLen":        len(req.Tools),
		"toolChoice":      strings.TrimSpace(req.ToolChoice),
	})
	// #endregion

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
	// Prefer Responses API (enables reasoning summaries) and fall back to Chat Completions.
	if out, err := c.generateStreamResponses(ctx, req, cb); err == nil {
		return out, nil
	} else if shouldFallbackToChat(err) {
		return c.generateStreamChat(ctx, req, cb)
	} else {
		return types.LLMResponse{}, err
	}
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

	stream := c.client.Responses.NewStreaming(ctx, params)
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
	return types.LLMResponse{Text: strings.TrimSpace(outText.String())}, nil
}

func (c *Client) generateStreamChat(ctx context.Context, req types.LLMRequest, cb types.LLMStreamCallback) (types.LLMResponse, error) {
	params, err := c.buildParams(req)
	if err != nil {
		return types.LLMResponse{}, err
	}

	stream := c.client.Chat.Completions.NewStreaming(ctx, params)
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
