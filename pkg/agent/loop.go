package agent

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/tinoosan/agen8/pkg/cost"
	llmtypes "github.com/tinoosan/agen8/pkg/llm/types"
	"github.com/tinoosan/agen8/pkg/prompts"
	"github.com/tinoosan/agen8/pkg/types"
	"github.com/tinoosan/agen8/pkg/validate"
)

var (
	// repeatedInvalidToolCallThreshold bounds repeated invalid tool-call arg loops.
	repeatedInvalidToolCallThreshold = 6
)

var ErrRepeatedInvalidToolCall = errors.New("agent repeated invalid tool call")

const (
	compactionNoticeServer = "Context was compacted automatically (server-side) to stay within a safe budget for long-running tasks. " +
		"Older tool outputs and earlier conversation turns may be truncated/omitted. " +
		"Re-open required details via tools (e.g., fs_read) rather than relying on long scrollback."
	compactionNoticeClient = "Context was compacted automatically to stay within a safe budget for long-running tasks. " +
		"Older tool outputs and earlier conversation turns may be truncated/omitted. " +
		"Re-open required details via tools (e.g., fs_read) rather than relying on long scrollback."
	hostResponseMarshalFallback = `{"ok":false,"error":"internal: failed to serialise response"}`
)

type RepeatedInvalidToolCallError struct {
	ToolName    string
	Count       int
	LastError   string
	Elapsed     time.Duration
	Coordinator bool
}

func (e *RepeatedInvalidToolCallError) Error() string {
	msg := fmt.Sprintf("agent repeated invalid tool call: tool=%s count=%d reason=%s", fallbackLabel(e.ToolName), e.Count, strings.TrimSpace(e.LastError))
	if e.Coordinator {
		msg += " hint=coordinator task_create calls must include assignedRole"
	}
	return msg
}

func (e *RepeatedInvalidToolCallError) Unwrap() error { return ErrRepeatedInvalidToolCall }

// DefaultAgent is the minimalist streaming loop: stream the model response, execute its tool calls, and return the final text.
type DefaultAgent struct {
	LLM  llmtypes.LLMClient
	Exec HostExecutor

	Model            string
	EnableWebSearch  bool
	ApprovalsMode    string
	ReasoningEffort  string
	ReasoningSummary string
	SystemPrompt     string
	PromptSource     PromptSource
	Hooks            Hooks
	MaxTokens        int
	ExtraTools       []llmtypes.Tool
	ToolRegistry     ToolRegistryProvider
}

// Compile-time check: DefaultAgent implements Agent.
var _ Agent = (*DefaultAgent)(nil)

// RunConversation executes the agent loop for an existing conversation.
func (a *DefaultAgent) RunConversation(ctx context.Context, msgs []llmtypes.LLMMessage) (final RunResult, updated []llmtypes.LLMMessage, steps int, err error) {
	return a.runConversation(ctx, msgs, 1)
}

func (a *DefaultAgent) runConversation(ctx context.Context, msgs []llmtypes.LLMMessage, startStep int) (final RunResult, updated []llmtypes.LLMMessage, steps int, err error) {
	if a == nil || a.LLM == nil {
		return RunResult{}, nil, 0, fmt.Errorf("agent LLM is required")
	}
	if a.Exec == nil {
		return RunResult{}, nil, 0, fmt.Errorf("agent Exec is required")
	}
	if err := validate.NonEmpty("agent Model", a.Model); err != nil {
		return RunResult{}, nil, 0, err
	}
	if len(msgs) == 0 {
		return RunResult{}, nil, 0, fmt.Errorf("msgs is required")
	}

	baseSystem := strings.TrimSpace(a.SystemPrompt)
	if baseSystem == "" {
		baseSystem = prompts.DefaultSystemPromptWithTools(PromptToolSpecFromSources(a.ToolRegistry, a.ExtraTools))
	}

	msgs = append([]llmtypes.LLMMessage(nil), msgs...)
	if startStep < 1 {
		startStep = 1
	}

	hostOpTools := []llmtypes.Tool{FinalAnswerTool()}
	if a.ToolRegistry != nil {
		hostOpTools = append(hostOpTools, a.ToolRegistry.Definitions()...)
	}
	if len(a.ExtraTools) != 0 {
		hostOpTools = append(hostOpTools, a.ExtraTools...)
	}

	loopStart := time.Now()
	lastFailedTool := ""
	lastFailureReason := ""
	consecutiveInvalid := 0

	for step := startStep; ; step++ {
		system := baseSystem
		if a.PromptSource != nil {
			updatedSystem, err := a.PromptSource.SystemPrompt(ctx, baseSystem, step)
			if err != nil {
				return RunResult{}, nil, 0, err
			}
			system = updatedSystem
		}

		// Keep context bounded for long-running tool loops.
		// Prefer provider-side compaction when available, then fall back to local compaction.
		msgs = a.compactConversationForBudget(ctx, step, msgs, system, compactBudgetBytes(ctx, a.Model))

		req := llmtypes.LLMRequest{
			Model:            a.Model,
			System:           system,
			Messages:         msgs,
			MaxTokens:        a.MaxTokens,
			Tools:            hostOpTools,
			ToolChoice:       "auto",
			JSONOnly:         false,
			EnableWebSearch:  a.EnableWebSearch,
			ReasoningEffort:  strings.TrimSpace(a.ReasoningEffort),
			ReasoningSummary: strings.TrimSpace(a.ReasoningSummary),
		}

		resp, reasoningSummary, err := a.streamToAccumulator(ctx, step, req)
		if err != nil {
			return RunResult{}, nil, 0, err
		}

		if a.Hooks.OnLLMUsage != nil && resp.Usage != nil {
			a.Hooks.OnLLMUsage(step, *resp.Usage)
		}
		if a.Hooks.OnStep != nil {
			a.Hooks.OnStep(step, a.Model, strings.TrimSpace(resp.EffectiveModel), strings.TrimSpace(reasoningSummary))
		}
		if a.Hooks.OnWebSearch != nil && len(resp.Citations) != 0 {
			a.Hooks.OnWebSearch(step, resp.Citations)
		}
		if a.Hooks.OnContextSize != nil {
			budgetTokens := cost.ContextBudgetTokens(ctx, a.Model)
			currentTokens := estimateTokens(estimateConversationBytes(system, msgs))
			if resp.Usage != nil {
				currentTokens = resp.Usage.InputTokens
			}
			a.Hooks.OnContextSize(step, currentTokens, budgetTokens)
		}

		if len(resp.ToolCalls) == 0 {
			finalText := strings.TrimSpace(resp.Text)
			msgs = append(msgs, llmtypes.LLMMessage{Role: "assistant", Content: finalText})
			return RunResult{Text: finalText, Status: types.TaskStatusSucceeded}, msgs, step, nil
		}

		assistantMsg := llmtypes.LLMMessage{
			Role:      "assistant",
			Content:   strings.TrimSpace(resp.Text),
			ToolCalls: resp.ToolCalls,
		}
		msgs = append(msgs, assistantMsg)

		type pendingHostOp struct {
			req    types.HostOpRequest
			callID string
		}
		pending := make([]pendingHostOp, 0, len(resp.ToolCalls))

		for _, tc := range resp.ToolCalls {
			which := strings.TrimSpace(tc.Function.Name)

			if which == "final_answer" {
				args, err := parseFinalAnswerArgs(tc.Function.Arguments)
				if err != nil {
					lastFailedTool = "final_answer"
					lastFailureReason = err.Error()
					hostResp := types.HostOpResponse{Op: "final_answer", Ok: false, Error: err.Error()}
					hostRespJSON := marshalHostResponseJSON(hostResp)
					msgs = append(msgs, llmtypes.LLMMessage{Role: "tool", ToolCallID: strings.TrimSpace(tc.ID), Content: hostRespJSON})
					continue
				}
				finalText := strings.TrimSpace(args.Text)
				hostResp := types.HostOpResponse{Op: "final_answer", Ok: true}
				hostRespJSON := marshalHostResponseJSON(hostResp)
				msgs = append(msgs, llmtypes.LLMMessage{Role: "tool", ToolCallID: strings.TrimSpace(tc.ID), Content: hostRespJSON})
				msgs = append(msgs, llmtypes.LLMMessage{Role: "assistant", Content: finalText})
				return RunResult{
					Text:      finalText,
					Artifacts: args.Artifacts,
					Status:    args.Status,
					Error:     strings.TrimSpace(args.Error),
				}, msgs, step, nil
			}

			if a.ToolRegistry == nil {
					lastFailedTool = which
					lastFailureReason = "tool registry is not configured"
					hostResp := types.HostOpResponse{Op: "tool_call", Ok: false, Error: "tool registry is not configured"}
					hostRespJSON := marshalHostResponseJSON(hostResp)
					msgs = append(msgs, llmtypes.LLMMessage{Role: "tool", ToolCallID: strings.TrimSpace(tc.ID), Content: hostRespJSON})
					continue
				}
				op, err := a.ToolRegistry.Dispatch(ctx, which, []byte(tc.Function.Arguments))
				if err != nil {
					dispatchErr := "invalid tool call args: " + err.Error()
					hostResp := types.HostOpResponse{Op: "tool_call", Ok: false, Error: dispatchErr}
					hostRespJSON := marshalHostResponseJSON(hostResp)
					msgs = append(msgs, llmtypes.LLMMessage{Role: "tool", ToolCallID: strings.TrimSpace(tc.ID), Content: hostRespJSON})
					if strings.EqualFold(strings.TrimSpace(lastFailedTool), which) {
						consecutiveInvalid++
				} else {
					consecutiveInvalid = 1
				}
				lastFailedTool = which
				lastFailureReason = strings.TrimSpace(err.Error())
				if repeatedInvalidToolCallThreshold > 0 && consecutiveInvalid >= repeatedInvalidToolCallThreshold {
					return RunResult{}, nil, 0, &RepeatedInvalidToolCallError{
						ToolName:    which,
						Count:       consecutiveInvalid,
						LastError:   fallbackReason(lastFailureReason, dispatchErr),
						Elapsed:     time.Since(loopStart),
						Coordinator: shouldEmitCoordinatorHint(which, lastFailureReason),
					}
				}
				continue
			}
			consecutiveInvalid = 0
			pending = append(pending, pendingHostOp{req: op, callID: strings.TrimSpace(tc.ID)})
		}

			for _, item := range pending {
				hostResp := a.Exec.Exec(ctx, item.req)
				hostRespJSON := marshalHostResponseJSON(hostResp)
				msgs = append(msgs, llmtypes.LLMMessage{Role: "tool", ToolCallID: item.callID, Content: hostRespJSON})
				if !hostResp.Ok {
					lastFailureReason = strings.TrimSpace(hostResp.Error)
				}
		}
	}
}

func shouldEmitCoordinatorHint(toolName, detail string) bool {
	if !strings.EqualFold(strings.TrimSpace(toolName), "task_create") {
		return false
	}
	detail = strings.ToLower(strings.TrimSpace(detail))
	return strings.Contains(detail, "assignedrole") && strings.Contains(detail, "coordinator")
}

func fallbackReason(current, fallback string) string {
	current = strings.TrimSpace(current)
	if current != "" {
		return current
	}
	return strings.TrimSpace(fallback)
}

func fallbackLabel(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return "unknown"
	}
	return v
}

func (a *DefaultAgent) compactConversationForBudget(ctx context.Context, step int, msgs []llmtypes.LLMMessage, system string, budgetBytes int) []llmtypes.LLMMessage {
	if budgetBytes <= 0 || len(msgs) == 0 {
		return msgs
	}
	beforeBytes := estimateConversationBytes(system, msgs)
	if beforeBytes <= budgetBytes {
		return msgs
	}

	if compactor, ok := a.LLM.(llmtypes.LLMClientCompaction); ok && compactor.SupportsServerCompaction() {
		compacted, err := compactor.CompactConversation(ctx, llmtypes.LLMCompactionRequest{
			Model:    a.Model,
			System:   system,
			Messages: msgs,
		})
			if err == nil && len(compacted.Messages) != 0 {
				// Prepend developer notice so the agent knows context was compacted server-side.
				notice := llmtypes.LLMMessage{
					Role:    "developer",
					Content: compactionNoticeServer,
				}
				result := append([]llmtypes.LLMMessage{notice}, compacted.Messages...)
				if a.Hooks.OnCompaction != nil {
				afterBytes := estimateConversationBytes(system, result)
				a.Hooks.OnCompaction(step, estimateTokens(beforeBytes), estimateTokens(afterBytes), true)
			}
			return result
		}
	}

	result := compactConversationForBudget(msgs, system, budgetBytes)
	if a.Hooks.OnCompaction != nil {
		afterBytes := estimateConversationBytes(system, result)
		a.Hooks.OnCompaction(step, estimateTokens(beforeBytes), estimateTokens(afterBytes), false)
	}
	return result
}

func (a *DefaultAgent) streamToAccumulator(ctx context.Context, step int, req llmtypes.LLMRequest) (llmtypes.LLMResponse, string, error) {
	s, ok := a.LLM.(llmtypes.LLMClientStreaming)
	if !ok {
		return llmtypes.LLMResponse{}, "", fmt.Errorf("LLM client does not support streaming")
	}
	dec := &finalTextStreamDecoder{}
	var streamMode string
	var streamPrefix strings.Builder
	const streamPrefixMax = 1024
	var reasoningBuf strings.Builder
	// Large safety cap to avoid unbounded memory growth if a provider emits extensive
	// reasoning text. For typical "reasoning summary" streams this should behave like
	// "no truncation".
	const reasoningMax = 1024 * 1024
	emit := func(token string) {
		if token == "" {
			return
		}
		if a.Hooks.OnToken != nil {
			a.Hooks.OnToken(step, token)
		}
	}
	resp, err := s.GenerateStream(ctx, req, func(chunk llmtypes.LLMStreamChunk) error {
		if a.Hooks.OnStreamChunk != nil {
			a.Hooks.OnStreamChunk(step, chunk)
		}
		if chunk.IsReasoning && chunk.Text != "" && reasoningBuf.Len() < reasoningMax {
			remain := reasoningMax - reasoningBuf.Len()
			if len(chunk.Text) > remain {
				reasoningBuf.WriteString(chunk.Text[:remain])
			} else {
				reasoningBuf.WriteString(chunk.Text)
			}
		}
		if chunk.Done || chunk.IsReasoning || chunk.Text == "" {
			return nil
		}
		if streamMode == "" {
			if streamPrefix.Len() < streamPrefixMax {
				remain := streamPrefixMax - streamPrefix.Len()
				if len(chunk.Text) > remain {
					streamPrefix.WriteString(chunk.Text[:remain])
				} else {
					streamPrefix.WriteString(chunk.Text)
				}
			}
			buf := streamPrefix.String()
			first := byte(0)
			for i := 0; i < len(buf); i++ {
				switch buf[i] {
				case ' ', '\t', '\r', '\n':
					continue
				default:
					first = buf[i]
				}
				if first != 0 {
					break
				}
			}
			if first == 0 && streamPrefix.Len() < streamPrefixMax {
				return nil
			}
			if first == '{' {
				streamMode = "json"
			} else {
				streamMode = "raw"
			}
			prefix := streamPrefix.String()
			streamPrefix.Reset()
			if streamMode == "raw" {
				emit(prefix)
				return nil
			}
			out := dec.Consume(prefix)
			if out != "" {
				emit(out)
			}
			return nil
		}
		if streamMode == "raw" {
			emit(chunk.Text)
			return nil
		}
		out := dec.Consume(chunk.Text)
		if out != "" {
			emit(out)
		}
		return nil
	})
	return resp, reasoningBuf.String(), err
}

func compactBudgetBytes(ctx context.Context, modelID string) int {
	if v := strings.TrimSpace(os.Getenv("AGEN8_CONTEXT_BUDGET_BYTES")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return cost.ContextBudgetTokens(ctx, modelID) * 4
}

func compactConversationForBudget(msgs []llmtypes.LLMMessage, system string, budgetBytes int) []llmtypes.LLMMessage {
	if budgetBytes <= 0 || len(msgs) == 0 {
		return msgs
	}
	if estimateConversationBytes(system, msgs) <= budgetBytes {
		return msgs
	}

	// Copy so we don't mutate caller-owned slices.
	out := append([]llmtypes.LLMMessage(nil), msgs...)

	// Phase 1: truncate older tool outputs (largest contributor) while keeping recent turns intact.
	const keepRecentToolMsgs = 8
	const toolMsgMaxChars = 1600
	toolSeen := 0
	for i := len(out) - 1; i >= 0; i-- {
		if out[i].Role != "tool" {
			continue
		}
		toolSeen++
		if toolSeen <= keepRecentToolMsgs {
			continue
		}
		out[i].Content = truncateWithMarker(out[i].Content, toolMsgMaxChars, " [truncated: older tool output omitted for context budget]")
	}
	if estimateConversationBytes(system, out) <= budgetBytes {
		return out
	}

	// Phase 2: keep the first user goal message (if present) + a recent suffix.
	const keepTailMsgs = 40
	cut := max(0, len(out)-keepTailMsgs)
	cut = adjustCutForToolMessages(out, cut)

	prefix := []llmtypes.LLMMessage(nil)
	if len(out) > 0 {
		// Preserve the first non-system message (typically the user's goal).
		prefix = append(prefix, out[0])
	}
	tail := out
	if cut > 0 && cut < len(out) {
		tail = out[cut:]
	}
	// Avoid duplicating the first message if it's already in tail.
	if len(prefix) != 0 && len(tail) != 0 && messagesEqual(prefix[0], tail[0]) {
		prefix = nil
	}

	notice := llmtypes.LLMMessage{
		Role:    "developer",
		Content: compactionNoticeClient,
	}

	compacted := append([]llmtypes.LLMMessage(nil), prefix...)
	// Always prepend notice; any preserved prefix message is already in compacted.
	compacted = append(compacted, notice)
	compacted = append(compacted, tail...)

	// Final guard: if still over budget, drop more from the head of the tail.
	for estimateConversationBytes(system, compacted) > budgetBytes && len(compacted) > 10 {
		// Keep the notice and trim the oldest message after it.
		dropIdx := 0
		if len(prefix) != 0 {
			dropIdx = 2 // prefix[0] + notice
		} else {
			dropIdx = 1 // notice
		}
		if dropIdx >= 0 && dropIdx < len(compacted) {
			compacted = append(compacted[:dropIdx], compacted[dropIdx+1:]...)
		} else {
			break
		}
	}

	return compacted
}

func estimateTokens(bytes int) int { return bytes / 4 }

func estimateConversationBytes(system string, msgs []llmtypes.LLMMessage) int {
	total := len(system)
	for _, m := range msgs {
		total += len(m.Role) + len(m.Content) + len(m.ToolCallID)
		for _, tc := range m.ToolCalls {
			total += len(tc.ID) + len(tc.Type) + len(tc.Function.Name) + len(tc.Function.Arguments)
		}
	}
	return total
}

func truncateWithMarker(s string, maxChars int, marker string) string {
	s = strings.TrimSpace(s)
	if maxChars <= 0 || s == "" || len(s) <= maxChars {
		return s
	}
	if marker == "" {
		marker = "…"
	}
	if maxChars <= len(marker) {
		return s[:maxChars]
	}
	return strings.TrimSpace(s[:maxChars-len(marker)]) + marker
}

func adjustCutForToolMessages(msgs []llmtypes.LLMMessage, cut int) int {
	// Ensure we don't start the kept tail with a tool message (tool messages should follow an assistant tool call).
	for cut < len(msgs) && cut > 0 && msgs[cut].Role == "tool" {
		cut++
	}
	if cut > len(msgs) {
		cut = len(msgs)
	}
	return cut
}

func messagesEqual(a, b llmtypes.LLMMessage) bool {
	return a.Role == b.Role && a.Content == b.Content && a.ToolCallID == b.ToolCallID
}

func marshalHostResponseJSON(hostResp types.HostOpResponse) string {
	hostRespJSON, err := types.MarshalPretty(hostResp)
	if err != nil {
		log.Printf("agent: marshal host response: %v", err)
		return hostResponseMarshalFallback
	}
	return string(hostRespJSON)
}

// Run executes the agent loop for a single user goal and returns the final result.
func (a *DefaultAgent) Run(ctx context.Context, goal string) (RunResult, error) {
	if err := validate.NonEmpty("goal", goal); err != nil {
		return RunResult{}, err
	}
	final, _, _, err := a.RunConversation(ctx, []llmtypes.LLMMessage{
		{Role: "user", Content: goal},
	})
	return final, err
}
