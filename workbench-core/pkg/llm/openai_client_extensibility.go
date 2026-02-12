package llm

import (
	"fmt"
	"strings"
	"sync"

	"github.com/openai/openai-go/v3/responses"
	"github.com/tinoosan/workbench-core/pkg/llm/types"
)

// CanonicalRole is the normalized role used by OpenAI client role mapping.
type CanonicalRole string

const (
	RoleUser       CanonicalRole = "user"
	RoleSystem     CanonicalRole = "system"
	RoleDeveloper  CanonicalRole = "developer"
	RoleAssistant  CanonicalRole = "assistant"
	RoleTool       CanonicalRole = "tool"
	RoleCompaction CanonicalRole = "compaction"
)

// RoleMapper maps provider/input role strings to canonical roles.
type RoleMapper interface {
	Canonicalize(raw string) (CanonicalRole, error)
}

// StreamEventHandler handles a single Responses stream event type.
type StreamEventHandler interface {
	EventType() string
	Handle(ev responses.ResponseStreamEventUnion, ctx *ResponsesStreamEventContext) (handled bool, err error)
}

// ResponsesStreamEventContext contains mutable state used during stream event handling.
type ResponsesStreamEventContext struct {
	Callback                types.LLMStreamCallback
	OutText                 *strings.Builder
	Completed               **responses.Response
	SawReasoningSummaryText *bool
}

// OpenAIClientConfig configures optional OpenAI client extension points.
type OpenAIClientConfig struct {
	RoleMapper          RoleMapper
	StreamEventHandlers []StreamEventHandler
}

type defaultRoleMapper struct{}

func (defaultRoleMapper) Canonicalize(raw string) (CanonicalRole, error) {
	role := strings.ToLower(strings.TrimSpace(raw))
	switch role {
	case string(RoleUser):
		return RoleUser, nil
	case string(RoleSystem):
		return RoleSystem, nil
	case string(RoleDeveloper):
		return RoleDeveloper, nil
	case string(RoleAssistant):
		return RoleAssistant, nil
	case string(RoleTool):
		return RoleTool, nil
	case string(RoleCompaction):
		return RoleCompaction, nil
	default:
		if role == "" {
			return "", fmt.Errorf("message role is empty")
		}
		return "", fmt.Errorf("unsupported message role %q", raw)
	}
}

func (c *Client) canonicalRole(raw string) (CanonicalRole, error) {
	var mapper RoleMapper = defaultRoleMapper{}
	if c != nil && c.roleMapper != nil {
		mapper = c.roleMapper
	}
	return mapper.Canonicalize(raw)
}

type streamEventHandlerFunc struct {
	eventType string
	fn        func(ev responses.ResponseStreamEventUnion, ctx *ResponsesStreamEventContext) (bool, error)
}

func (h streamEventHandlerFunc) EventType() string {
	return h.eventType
}

func (h streamEventHandlerFunc) Handle(ev responses.ResponseStreamEventUnion, ctx *ResponsesStreamEventContext) (bool, error) {
	if h.fn == nil {
		return false, nil
	}
	return h.fn(ev, ctx)
}

func (c *Client) dispatchResponsesStreamEvent(ev responses.ResponseStreamEventUnion, ctx *ResponsesStreamEventContext) (bool, error) {
	eventType := normalizeStreamEventType(ev.Type)
	if eventType == "" {
		eventType = inferStreamEventType(ev)
		if eventType == "" {
			return false, nil
		}
	}
	handlers := c.responsesStreamEventHandlerIndex()[eventType]
	for _, h := range handlers {
		if h == nil {
			continue
		}
		handled, err := h.Handle(ev, ctx)
		if err != nil {
			return true, err
		}
		if handled {
			return true, nil
		}
	}
	return false, nil
}

func inferStreamEventType(ev responses.ResponseStreamEventUnion) string {
	switch ev.AsAny().(type) {
	case responses.ResponseTextDeltaEvent:
		return "response.output_text.delta"
	case responses.ResponseReasoningSummaryTextDeltaEvent:
		return "response.reasoning_summary_text.delta"
	case responses.ResponseReasoningSummaryPartAddedEvent:
		return "response.reasoning_summary_part.added"
	case responses.ResponseReasoningSummaryPartDoneEvent:
		return "response.reasoning_summary_part.done"
	case responses.ResponseReasoningSummaryTextDoneEvent:
		return "response.reasoning_summary_text.done"
	case responses.ResponseReasoningTextDeltaEvent:
		return "response.reasoning_text.delta"
	case responses.ResponseOutputItemDoneEvent:
		return "response.output_item.done"
	case responses.ResponseCompletedEvent:
		return "response.completed"
	default:
		return ""
	}
}

func (c *Client) responsesStreamEventHandlerIndex() map[string][]StreamEventHandler {
	if c != nil && len(c.streamEventHandlers) != 0 {
		return c.streamEventHandlers
	}
	return defaultResponsesStreamEventHandlerIndex()
}

func buildResponsesStreamEventHandlerIndex(custom []StreamEventHandler) map[string][]StreamEventHandler {
	if len(custom) == 0 {
		return defaultResponsesStreamEventHandlerIndex()
	}
	all := make([]StreamEventHandler, 0, len(custom)+len(defaultResponsesStreamEventHandlers()))
	for _, h := range custom {
		if h != nil {
			all = append(all, h)
		}
	}
	all = append(all, defaultResponsesStreamEventHandlers()...)
	return indexResponsesStreamEventHandlers(all)
}

func normalizeStreamEventType(v string) string {
	return strings.ToLower(strings.TrimSpace(v))
}

func indexResponsesStreamEventHandlers(handlers []StreamEventHandler) map[string][]StreamEventHandler {
	out := make(map[string][]StreamEventHandler)
	for _, h := range handlers {
		if h == nil {
			continue
		}
		t := normalizeStreamEventType(h.EventType())
		if t == "" {
			continue
		}
		out[t] = append(out[t], h)
	}
	return out
}

var (
	defaultStreamHandlersOnce sync.Once
	defaultStreamHandlers     map[string][]StreamEventHandler
)

func defaultResponsesStreamEventHandlerIndex() map[string][]StreamEventHandler {
	defaultStreamHandlersOnce.Do(func() {
		defaultStreamHandlers = indexResponsesStreamEventHandlers(defaultResponsesStreamEventHandlers())
	})
	return defaultStreamHandlers
}

func defaultResponsesStreamEventHandlers() []StreamEventHandler {
	handlers := make([]StreamEventHandler, 0, 24)
	register := func(eventTypes []string, fn func(ev responses.ResponseStreamEventUnion, ctx *ResponsesStreamEventContext) (bool, error)) {
		for _, t := range eventTypes {
			handlers = append(handlers, streamEventHandlerFunc{eventType: t, fn: fn})
		}
	}

	register([]string{"response.output_text.delta", "response.text.delta"}, handleResponseTextDeltaEvent)
	register([]string{"response.reasoning_summary_text.delta", "response.reasoning_summary.delta"}, handleResponseReasoningSummaryTextDeltaEvent)
	register([]string{"response.reasoning_summary_part.added", "response.reasoning_summary.part.added"}, handleResponseReasoningSummaryPartAddedEvent)
	register([]string{"response.reasoning_summary_part.done", "response.reasoning_summary.part.done"}, handleResponseReasoningSummaryPartDoneEvent)
	register([]string{"response.reasoning_summary_text.done", "response.reasoning_summary.done"}, handleResponseReasoningSummaryTextDoneEvent)
	register([]string{"response.reasoning_text.delta", "response.reasoning.delta"}, handleResponseReasoningTextDeltaEvent)
	register([]string{"response.output_item.done"}, handleResponseOutputItemDoneEvent)
	register([]string{"response.completed"}, handleResponseCompletedEvent)

	return handlers
}

func emitStreamChunk(ctx *ResponsesStreamEventContext, chunk types.LLMStreamChunk) error {
	if ctx == nil || ctx.Callback == nil {
		return nil
	}
	return ctx.Callback(chunk)
}

func markSawReasoningSummaryText(ctx *ResponsesStreamEventContext) {
	if ctx == nil || ctx.SawReasoningSummaryText == nil {
		return
	}
	*ctx.SawReasoningSummaryText = true
}

func sawReasoningSummaryText(ctx *ResponsesStreamEventContext) bool {
	return ctx != nil && ctx.SawReasoningSummaryText != nil && *ctx.SawReasoningSummaryText
}

func handleResponseTextDeltaEvent(ev responses.ResponseStreamEventUnion, ctx *ResponsesStreamEventContext) (bool, error) {
	e, ok := ev.AsAny().(responses.ResponseTextDeltaEvent)
	if !ok {
		return false, nil
	}
	if ctx == nil || ctx.Callback == nil {
		return true, nil
	}
	if ctx.OutText != nil {
		ctx.OutText.WriteString(e.Delta)
	}
	return true, emitStreamChunk(ctx, types.LLMStreamChunk{Text: e.Delta})
}

func handleResponseReasoningSummaryTextDeltaEvent(ev responses.ResponseStreamEventUnion, ctx *ResponsesStreamEventContext) (bool, error) {
	e, ok := ev.AsAny().(responses.ResponseReasoningSummaryTextDeltaEvent)
	if !ok {
		return false, nil
	}
	if ctx == nil || ctx.Callback == nil {
		return true, nil
	}
	markSawReasoningSummaryText(ctx)
	return true, emitStreamChunk(ctx, types.LLMStreamChunk{IsReasoning: true, Text: e.Delta})
}

func handleResponseReasoningSummaryPartAddedEvent(ev responses.ResponseStreamEventUnion, ctx *ResponsesStreamEventContext) (bool, error) {
	e, ok := ev.AsAny().(responses.ResponseReasoningSummaryPartAddedEvent)
	if !ok {
		return false, nil
	}
	if ctx == nil || ctx.Callback == nil {
		return true, nil
	}
	if strings.TrimSpace(e.Part.Text) == "" {
		return true, nil
	}
	markSawReasoningSummaryText(ctx)
	return true, emitStreamChunk(ctx, types.LLMStreamChunk{IsReasoning: true, Text: e.Part.Text})
}

func handleResponseReasoningSummaryPartDoneEvent(ev responses.ResponseStreamEventUnion, ctx *ResponsesStreamEventContext) (bool, error) {
	e, ok := ev.AsAny().(responses.ResponseReasoningSummaryPartDoneEvent)
	if !ok {
		return false, nil
	}
	if ctx == nil || ctx.Callback == nil {
		return true, nil
	}
	if sawReasoningSummaryText(ctx) || strings.TrimSpace(e.Part.Text) == "" {
		return true, nil
	}
	markSawReasoningSummaryText(ctx)
	return true, emitStreamChunk(ctx, types.LLMStreamChunk{IsReasoning: true, Text: e.Part.Text})
}

func handleResponseReasoningSummaryTextDoneEvent(ev responses.ResponseStreamEventUnion, ctx *ResponsesStreamEventContext) (bool, error) {
	e, ok := ev.AsAny().(responses.ResponseReasoningSummaryTextDoneEvent)
	if !ok {
		return false, nil
	}
	if ctx == nil || ctx.Callback == nil {
		return true, nil
	}
	if sawReasoningSummaryText(ctx) || strings.TrimSpace(e.Text) == "" {
		return true, nil
	}
	markSawReasoningSummaryText(ctx)
	return true, emitStreamChunk(ctx, types.LLMStreamChunk{IsReasoning: true, Text: e.Text})
}

func handleResponseReasoningTextDeltaEvent(ev responses.ResponseStreamEventUnion, ctx *ResponsesStreamEventContext) (bool, error) {
	_, ok := ev.AsAny().(responses.ResponseReasoningTextDeltaEvent)
	if !ok {
		return false, nil
	}
	if ctx == nil || ctx.Callback == nil {
		return true, nil
	}
	return true, emitStreamChunk(ctx, types.LLMStreamChunk{IsReasoning: true})
}

func handleResponseOutputItemDoneEvent(ev responses.ResponseStreamEventUnion, ctx *ResponsesStreamEventContext) (bool, error) {
	e, ok := ev.AsAny().(responses.ResponseOutputItemDoneEvent)
	if !ok {
		return false, nil
	}
	if ctx == nil || ctx.Callback == nil {
		return true, nil
	}
	if sawReasoningSummaryText(ctx) {
		return true, nil
	}
	texts := responseReasoningSummaryTexts(e.Item)
	if len(texts) == 0 {
		return true, nil
	}
	markSawReasoningSummaryText(ctx)
	for i, s := range texts {
		if i > 0 {
			s = "\n\n" + s
		}
		if err := emitStreamChunk(ctx, types.LLMStreamChunk{IsReasoning: true, Text: s}); err != nil {
			return true, err
		}
	}
	return true, nil
}

func handleResponseCompletedEvent(ev responses.ResponseStreamEventUnion, ctx *ResponsesStreamEventContext) (bool, error) {
	e, ok := ev.AsAny().(responses.ResponseCompletedEvent)
	if !ok {
		return false, nil
	}
	if ctx != nil && ctx.Completed != nil {
		r := e.Response
		*ctx.Completed = &r
	}
	return true, nil
}
