package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/tinoosan/workbench-core/internal/debuglog"
	"github.com/tinoosan/workbench-core/internal/types"
	"github.com/tinoosan/workbench-core/internal/validate"
)

// Agent is the minimal "agent loop v0": ask a model what to do next and execute it.
//
// This loop is deliberately tiny so you can validate the core architecture:
//   - host primitives (fs.* and tool.run) are always available (not discovered)
//   - tools are discovered via /tools manifests
//   - tool execution writes results under /results/<callId>/...
//
// v0 limitations:
//   - no streaming
//   - no function calling / tool calling features
//   - no events (telemetry can be added later as a decorator)
//
// Contract:
// The model must output exactly one JSON object per turn:
//   - either a HostOpRequest (op=fs.list/fs.read/fs.write/fs.append/tool.run)
//   - or {"op":"final","text":"..."} to stop the loop
type Agent struct {
	LLM  types.LLMClient
	Exec HostExecutor

	// Model is required. Example: "openai/gpt-4o-mini" (via OpenRouter), etc.
	Model string

	// EnableWebSearch controls whether the agent requests web-search-grounded model variants
	// when supported by the provider (e.g. OpenRouter ":online"). Host controls this.
	EnableWebSearch bool

	// ReasoningEffort is an optional hint for reasoning-capable models.
	ReasoningEffort string

	// ReasoningSummary controls whether and how providers should emit reasoning summaries.
	ReasoningSummary string

	// SystemPrompt is the base system instructions passed to the model.
	SystemPrompt string

	// Context optionally refreshes bounded context (memory/profile/log/etc) per model step.
	Context ContextSource

	// MaxSteps caps the number of model -> host op iterations.
	// If zero, a default is used.
	MaxSteps int

	// Hooks are optional observability callbacks invoked by the agent loop.
	Hooks Hooks

	// ExtraTools are additional function tools exposed by the host (derived from manifests).
	ExtraTools []types.Tool
	// ToolFunctionRoutes map function names back to tool.run routes.
	ToolFunctionRoutes map[string]ToolRoute
}

// RunConversation executes the agent loop for an existing conversation.
//
// Why this exists:
//   - In an interactive REPL, users often respond with short follow-ups like "2" or "go on".
//   - If the host starts a fresh loop per input line, those short replies lose context and
//     the model restarts discovery (wasting tokens and producing confusing behavior).
//
// Conversation model:
//   - msgs is the full chat history so far (typically ending with the latest user message).
//   - The agent appends each model-emitted HostOpRequest (as an assistant message) and the
//     corresponding HostOpResponse (as a user message) as the loop proceeds.
//   - When the model returns {"op":"final","text":"..."}, the agent appends that final JSON
//     object as the last assistant message and returns text to the host to display.
func (a *Agent) RunConversation(ctx context.Context, msgs []types.LLMMessage) (final string, updated []types.LLMMessage, steps int, err error) {
	return a.runConversation(ctx, msgs, 1, "", "", "")
}

// RunConversationWithCheckpoints executes the agent loop like RunConversation, but
// persists and resumes from a durable checkpoint at checkpointPath.
//
// If a valid checkpoint exists at checkpointPath, it takes precedence over msgs.
func (a *Agent) RunConversationWithCheckpoints(ctx context.Context, msgs []types.LLMMessage, checkpointPath string) (final string, updated []types.LLMMessage, steps int, err error) {
	cp, err := LoadAgentCheckpoint(checkpointPath)
	if err != nil {
		return "", nil, 0, err
	}
	startStep := 1
	lastResponseID := ""
	userMsg := ""
	if cp != nil {
		msgs = cp.Messages
		startStep = cp.NextStep
		lastResponseID = strings.TrimSpace(cp.LastResponseID)
		userMsg = strings.TrimSpace(cp.UserMessage)
	}
	if strings.TrimSpace(userMsg) == "" {
		userMsg = extractTurnUserMessage(msgs)
	}
	return a.runConversation(ctx, msgs, startStep, lastResponseID, checkpointPath, userMsg)
}

func (a *Agent) runConversation(ctx context.Context, msgs []types.LLMMessage, startStep int, lastResponseID string, checkpointPath string, checkpointUserMessage string) (final string, updated []types.LLMMessage, steps int, err error) {
	if a == nil || a.LLM == nil {
		return "", nil, 0, fmt.Errorf("agent LLM is required")
	}
	if a.Exec == nil {
		return "", nil, 0, fmt.Errorf("agent Exec is required")
	}
	if err := validate.NonEmpty("agent Model", a.Model); err != nil {
		return "", nil, 0, err
	}
	if len(msgs) == 0 {
		return "", nil, 0, fmt.Errorf("msgs is required")
	}

	maxSteps := a.MaxSteps
	if maxSteps == 0 {
		maxSteps = 20
	}
	if maxSteps < 1 {
		return "", nil, 0, fmt.Errorf("MaxSteps must be >= 1")
	}

	baseSystem := strings.TrimSpace(a.SystemPrompt)
	if baseSystem == "" {
		baseSystem = agentLoopV0SystemPrompt()
	}

	// Copy the slice so the caller can keep their own version if needed.
	msgs = append([]types.LLMMessage(nil), msgs...)

	if startStep < 1 {
		startStep = 1
	}
	lastResponseID = strings.TrimSpace(lastResponseID)

	turnUserMessage := strings.TrimSpace(checkpointUserMessage)
	if turnUserMessage == "" {
		turnUserMessage = extractTurnUserMessage(msgs)
	}

	turnStartIdx := indexOfTurnUserMessage(msgs)
	turnHasToolOutput := hasToolMessageAfterIdx(msgs, turnStartIdx)

	// Track last parsed model op (debug-only) for checkpointing.
	lastOpForCheckpoint := ""

	// Enable function/tool calling for host primitives.
	hostOpTools := HostOpFunctions()
	if len(a.ExtraTools) != 0 {
		hostOpTools = append(hostOpTools, a.ExtraTools...)
	}

	finalizeNudgeSent := false

	for step := startStep; step <= maxSteps; step++ {
		toolChoice, toolChoiceReason, turnFlags := toolChoiceForTurn(turnUserMessage, turnHasToolOutput)
		envWanted, _ := turnFlags["envWanted"].(bool)
		// #region agent log
		debuglog.Log("toolcalling", "H1", "loop.go:runConversation", "step_start", map[string]any{
			"step":          step,
			"maxSteps":      maxSteps,
			"msgsLen":       len(msgs),
			"toolChoice":    toolChoice,
			"toolChoiceWhy": toolChoiceReason,
			"turnUserLen":   len(turnUserMessage),
			"turnFlags":     turnFlags,
			"toolsLen":      len(hostOpTools),
			"checkpointOn":  strings.TrimSpace(checkpointPath) != "",
		})
		// #endregion

		system := baseSystem
		if a.Context != nil {
			updated, err := a.Context.SystemPrompt(ctx, baseSystem, step)
			if err != nil {
				return "", nil, 0, err
			}
			system = updated
		}
		// Always restate the current user request in the system prompt (not the message list),
		// so Responses API delta-chaining can omit repeating the user message without models
		// "forgetting" what they are trying to accomplish.
		if strings.TrimSpace(turnUserMessage) != "" {
			system = strings.TrimSpace(system) + "\n\n## Current User Request\n\n" + strings.TrimSpace(turnUserMessage) + "\n"
		}
		// #region agent log
		debuglog.Log("toolcalling", "H3", "loop.go:runConversation", "system_prompt_flags", map[string]any{
			"step":               step,
			"hasLegacyJsonRules": strings.Contains(system, "exactly ONE JSON object"),
			"hasToolCalling":     strings.Contains(system, "tool/function calling") || strings.Contains(system, "tool calling"),
			"mentionsBatchTool":  strings.Contains(system, "batch(") || strings.Contains(system, "\n  - batch"),
			"mentionsToolRun":    strings.Contains(system, "tool_run") || strings.Contains(system, "tool.run"),
			"hasHistoryBlock":    strings.Contains(system, "## Recent Conversation (from /history)"),
			"hasTurnBlock":       strings.Contains(system, "## Current User Request"),
			"turnUserLen":        len(strings.TrimSpace(turnUserMessage)),
		})
		// #endregion

		// Loop breaker: some models can get stuck repeatedly calling tools without ever
		// calling final_answer. After enough tool outputs have been produced, inject a
		// strong finalization nudge (once) to converge the turn.
		//
		// We keep toolChoice as-is (often "required") so the model can satisfy the
		// protocol by calling final_answer.
		if envWanted && turnHasToolOutput && !finalizeNudgeSent && step >= 12 {
			finalizeNudgeSent = true
			msgs = append(msgs, types.LLMMessage{
				Role: "user",
				Content: "You have already used the environment tools for this request. Now STOP calling tools.\n\n" +
					"Call final_answer({\"text\":\"...\"}) NOW with your best possible response based on the tool outputs you already have.\n\n" +
					"If something is missing, say exactly what and why, and what the user should provide next.",
			})
			// #region agent log
			debuglog.Log("toolcalling", "H17", "loop.go:runConversation", "finalize_nudge_injected", map[string]any{
				"step":       step,
				"toolChoice": toolChoice,
				"msgsLen":    len(msgs),
			})
			// #endregion
		}

		req := types.LLMRequest{
			Model:              a.Model,
			System:             system,
			Messages:           msgs,
			MaxTokens:          1024,
			Tools:              hostOpTools,
			ToolChoice:         toolChoice,
			JSONOnly:           false,
			ResponseSchema:     nil,
			EnableWebSearch:    a.EnableWebSearch,
			PreviousResponseID: lastResponseID,
			ReasoningEffort:    strings.TrimSpace(a.ReasoningEffort),
			ReasoningSummary:   strings.TrimSpace(a.ReasoningSummary),
		}

		var resp types.LLMResponse
		var err error
		if s, ok := a.LLM.(types.LLMClientStreaming); ok {
			dec := &finalTextStreamDecoder{}
			// #region agent log
			streamTextChunkN := 0
			streamTextBytes := 0
			streamDecoderEmittedBytes := 0
			streamRawEmittedBytes := 0
			streamLoggedFirst := false
			streamMode := "unknown" // "unknown" | "raw" | "json"
			streamModeLogged := false
			var streamPrefix strings.Builder
			const streamPrefixMax = 1024
			// #endregion
			// #region agent log
			// Capture provider-supplied reasoning summaries (NOT raw chain-of-thought).
			var reasoningBuf strings.Builder
			reasoningTextN := 0
			reasoningSignalN := 0
			reasoningCharsTotal := 0
			// #endregion
			resp, err = s.GenerateStream(ctx, req, func(chunk types.LLMStreamChunk) error {
				if chunk.Done {
					if a.Hooks.OnStreamChunk != nil {
						a.Hooks.OnStreamChunk(step, chunk)
					}
					// #region agent log
					debuglog.Log("toolcalling", "H16", "loop.go:runConversation", "stream_text_totals", map[string]any{
						"step":              step,
						"model":             strings.TrimSpace(a.Model),
						"toolChoice":        strings.TrimSpace(toolChoice),
						"jsonOnly":          req.JSONOnly,
						"hasSchema":         req.ResponseSchema != nil,
						"mode":              streamMode,
						"textChunks":        streamTextChunkN,
						"textBytes":         streamTextBytes,
						"decoderEmittedB":   streamDecoderEmittedBytes,
						"decoderEmittedAny": streamDecoderEmittedBytes != 0,
						"rawEmittedB":       streamRawEmittedBytes,
						"rawEmittedAny":     streamRawEmittedBytes != 0,
					})
					// #endregion
					// #region agent log
					if reasoningSignalN != 0 || reasoningTextN != 0 {
						prev := reasoningBuf.String()
						if len(prev) > 800 {
							prev = prev[:800] + "…"
						}
						debuglog.Log("toolcalling", "H13", "loop.go:runConversation", "reasoning_summary_seen", map[string]any{
							"step":               step,
							"toolChoice":         toolChoice,
							"reasoningSignalN":   reasoningSignalN,
							"reasoningTextN":     reasoningTextN,
							"reasoningChars":     reasoningCharsTotal,
							"reasoningStoredLen": reasoningBuf.Len(),
							"reasoningPreview":   prev,
						})
					}
					// #endregion
					return nil
				}
				if chunk.IsReasoning {
					if a.Hooks.OnStreamChunk != nil {
						a.Hooks.OnStreamChunk(step, chunk)
					}
					// #region agent log
					reasoningSignalN++
					if chunk.Text != "" {
						reasoningTextN++
						reasoningCharsTotal += len(chunk.Text)
						// Avoid huge logs: store at most ~8KB for preview/debug.
						if reasoningBuf.Len() < 8*1024 {
							remain := 8*1024 - reasoningBuf.Len()
							if len(chunk.Text) > remain {
								reasoningBuf.WriteString(chunk.Text[:remain])
							} else {
								reasoningBuf.WriteString(chunk.Text)
							}
						}
					}
					// #endregion
					return nil
				}
				if chunk.Text == "" {
					return nil
				}
				// #region agent log
				streamTextChunkN++
				streamTextBytes += len(chunk.Text)
				// #endregion
				emit := func(s string) {
					if s == "" {
						return
					}
					// #region agent log
					streamRawEmittedBytes += len(s)
					// #endregion
					if a.Hooks.OnToken != nil {
						a.Hooks.OnToken(step, s)
					}
				}

				// Auto-detect: some providers/models stream plain assistant text, others stream a JSON
				// envelope like {"op":"final","text":"..."} (or HostOpRequest JSON). We must not drop
				// plain text, and we also should not stream raw JSON to the UI.
				if streamMode == "unknown" {
					// Buffer a small prefix until we see the first non-whitespace character.
					if streamPrefix.Len() < streamPrefixMax {
						remain := streamPrefixMax - streamPrefix.Len()
						if len(chunk.Text) > remain {
							streamPrefix.WriteString(chunk.Text[:remain])
						} else {
							streamPrefix.WriteString(chunk.Text)
						}
					}
					// Decide mode based on the first non-whitespace rune/byte.
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
						// Still only whitespace so far; wait for more bytes.
						return nil
					}
					if first == '{' {
						streamMode = "json"
					} else {
						streamMode = "raw"
					}
					// #region agent log
					if !streamModeLogged {
						streamModeLogged = true
						debuglog.Log("toolcalling", "H16", "loop.go:runConversation", "stream_mode_selected", map[string]any{
							"step":       step,
							"model":      strings.TrimSpace(a.Model),
							"toolChoice": strings.TrimSpace(toolChoice),
							"mode":       streamMode,
							"firstByte":  string([]byte{first}),
						})
					}
					// #endregion
					// Flush buffered prefix through the chosen streaming mode.
					prefix := streamPrefix.String()
					streamPrefix.Reset()
					if streamMode == "raw" {
						emit(prefix)
						return nil
					}
					out := dec.Consume(prefix)
					// #region agent log
					streamDecoderEmittedBytes += len(out)
					// #endregion
					if !streamLoggedFirst {
						streamLoggedFirst = true
						prev := prefix
						if len(prev) > 120 {
							prev = prev[:120] + "…"
						}
						debuglog.Log("toolcalling", "H16", "loop.go:runConversation", "stream_first_text", map[string]any{
							"step":       step,
							"model":      strings.TrimSpace(a.Model),
							"toolChoice": strings.TrimSpace(toolChoice),
							"mode":       streamMode,
							"chunkLen":   len(prefix),
							"chunkPrev":  prev,
							"outLen":     len(out),
						})
					}
					if out != "" {
						emit(out)
					}
					return nil
				}

				if streamMode == "raw" {
					if !streamLoggedFirst {
						streamLoggedFirst = true
						prev := chunk.Text
						if len(prev) > 120 {
							prev = prev[:120] + "…"
						}
						// #region agent log
						debuglog.Log("toolcalling", "H16", "loop.go:runConversation", "stream_first_text", map[string]any{
							"step":       step,
							"model":      strings.TrimSpace(a.Model),
							"toolChoice": strings.TrimSpace(toolChoice),
							"mode":       streamMode,
							"chunkLen":   len(chunk.Text),
							"chunkPrev":  prev,
							"outLen":     len(chunk.Text),
						})
						// #endregion
					}
					emit(chunk.Text)
					return nil
				}

				// streamMode == "json": only emit decoded final.text (never raw JSON).
				out := dec.Consume(chunk.Text)
				// #region agent log
				streamDecoderEmittedBytes += len(out)
				// #endregion
				if !streamLoggedFirst {
					streamLoggedFirst = true
					prev := chunk.Text
					if len(prev) > 120 {
						prev = prev[:120] + "…"
					}
					debuglog.Log("toolcalling", "H16", "loop.go:runConversation", "stream_first_text", map[string]any{
						"step":       step,
						"model":      strings.TrimSpace(a.Model),
						"toolChoice": strings.TrimSpace(toolChoice),
						"mode":       streamMode,
						"chunkLen":   len(chunk.Text),
						"chunkPrev":  prev,
						"outLen":     len(out),
					})
				}
				if out != "" {
					emit(out)
				}
				return nil
			})
		} else {
			resp, err = a.LLM.Generate(ctx, req)
		}
		if err != nil {
			return "", nil, 0, err
		}

		// Update chaining state for next step. If we fell back to a provider path that
		// doesn't return a Responses ID, clear the ID to avoid stale chaining later.
		if strings.TrimSpace(resp.ResponseID) != "" {
			lastResponseID = resp.ResponseID
		} else {
			lastResponseID = ""
		}

		if a.Hooks.OnLLMUsage != nil && resp.Usage != nil {
			a.Hooks.OnLLMUsage(step, *resp.Usage)
		}
		if a.Hooks.OnWebSearch != nil && len(resp.Citations) != 0 {
			a.Hooks.OnWebSearch(step, resp.Citations)
		}

		// #region agent log
		firstTool := ""
		if len(resp.ToolCalls) != 0 {
			firstTool = strings.TrimSpace(resp.ToolCalls[0].Function.Name)
		}
		debuglog.Log("toolcalling", "H1", "loop.go:runConversation", "llm_response", map[string]any{
			"step":         step,
			"textLen":      len(resp.Text),
			"toolCallsLen": len(resp.ToolCalls),
			"firstTool":    firstTool,
		})
		if len(resp.ToolCalls) != 0 {
			counts := map[string]int{}
			for _, tc := range resp.ToolCalls {
				counts[strings.TrimSpace(tc.Function.Name)]++
			}
			debuglog.Log("toolcalling", "H2", "loop.go:runConversation", "tool_calls_breakdown", map[string]any{
				"step":   step,
				"counts": counts,
			})
		}
		// #endregion

		// Function/tool calling path.
		// If the model produced tool calls, execute them and feed outputs back as role="tool".
		if len(resp.ToolCalls) != 0 {
			// Preserve the assistant tool_calls message in the transcript so tool_call_id references
			// are valid on the next model call (mirrors OpenAI's tool-calling flow).
			msgs = append(msgs, types.LLMMessage{
				Role:      "assistant",
				Content:   strings.TrimSpace(resp.Text),
				ToolCalls: resp.ToolCalls,
			})

			appendedToolOutputAnyThisStep := false
			appendedToolOutputOkThisStep := false
			for _, tc := range resp.ToolCalls {
				which := strings.TrimSpace(tc.Function.Name)
				lastOpForCheckpoint = "tool_call:" + which

				// Control: end the turn explicitly (no heuristics on assistant prose).
				if which == "final_answer" {
					var args struct {
						Text string `json:"text"`
					}
					if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
						hostResp := types.HostOpResponse{Op: "final_answer", Ok: false, Error: "final_answer args were not valid JSON: " + err.Error()}
						hostRespJSON, _ := types.MarshalPretty(hostResp)
						msgs = append(msgs, types.LLMMessage{Role: "tool", ToolCallID: strings.TrimSpace(tc.ID), Content: string(hostRespJSON)})
						appendedToolOutputAnyThisStep = true
						continue
					}
					out := strings.TrimSpace(args.Text)
					if out == "" {
						hostResp := types.HostOpResponse{Op: "final_answer", Ok: false, Error: "final_answer.text is required"}
						hostRespJSON, _ := types.MarshalPretty(hostResp)
						msgs = append(msgs, types.LLMMessage{Role: "tool", ToolCallID: strings.TrimSpace(tc.ID), Content: string(hostRespJSON)})
						appendedToolOutputAnyThisStep = true
						continue
					}
					// Provide a tool output for transcript completeness.
					hostResp := types.HostOpResponse{Op: "final_answer", Ok: true}
					hostRespJSON, _ := types.MarshalPretty(hostResp)
					msgs = append(msgs, types.LLMMessage{Role: "tool", ToolCallID: strings.TrimSpace(tc.ID), Content: string(hostRespJSON)})
					appendedToolOutputAnyThisStep = true
					appendedToolOutputOkThisStep = true

					_ = ClearAgentCheckpoint(checkpointPath)
					msgs = append(msgs, types.LLMMessage{Role: "assistant", Content: out})
					return out, msgs, step, nil
				}

				// Special: tool_batch -> HostOpBatchResponse (tool.run ops)
				if which == "tool_batch" {
					var args struct {
						Parallel *bool `json:"parallel"`
						Calls    []struct {
							ToolID    string          `json:"toolId"`
							ActionID  string          `json:"actionId"`
							Input     json.RawMessage `json:"input"`
							TimeoutMs *int            `json:"timeoutMs"`
						} `json:"calls"`
					}
					if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
						batchResp := types.HostOpBatchResponse{Ok: false, Error: "tool_batch args were not valid JSON: " + err.Error()}
						batchRespJSON, _ := types.MarshalPretty(batchResp)
						// #region agent log
						debuglog.Log("toolcalling", "H8", "loop.go:runConversation", "tool_output_error_sent", map[string]any{
							"step": step,
							"tool": which,
						})
						// #endregion
						msgs = append(msgs, types.LLMMessage{Role: "tool", ToolCallID: strings.TrimSpace(tc.ID), Content: string(batchRespJSON)})
						appendedToolOutputAnyThisStep = true
						continue
					}
					parallel := false
					if args.Parallel != nil {
						parallel = *args.Parallel
					}
					// #region agent log
					debuglog.Log("toolcalling", "H5", "loop.go:runConversation", "tool_batch_exec", map[string]any{
						"step":     step,
						"parallel": parallel,
						"callsLen": len(args.Calls),
					})
					// #endregion
					ops := make([]types.HostOpRequest, 0, len(args.Calls))
					for _, c := range args.Calls {
						timeout := 0
						if c.TimeoutMs != nil {
							timeout = *c.TimeoutMs
						}
						ops = append(ops, types.HostOpRequest{
							Op:        types.HostOpToolRun,
							ToolID:    types.ToolID(strings.TrimSpace(c.ToolID)),
							ActionID:  strings.TrimSpace(c.ActionID),
							Input:     c.Input,
							TimeoutMs: timeout,
						})
					}
					batchReq := types.HostOpBatchRequest{Op: types.HostOpFSBatch, Operations: ops, Parallel: parallel}
					if err := batchReq.Validate(); err != nil {
						batchResp := types.HostOpBatchResponse{Ok: false, Error: "tool_batch args invalid: " + err.Error()}
						batchRespJSON, _ := types.MarshalPretty(batchResp)
						// #region agent log
						debuglog.Log("toolcalling", "H8", "loop.go:runConversation", "tool_output_error_sent", map[string]any{
							"step": step,
							"tool": which,
						})
						// #endregion
						msgs = append(msgs, types.LLMMessage{Role: "tool", ToolCallID: strings.TrimSpace(tc.ID), Content: string(batchRespJSON)})
						appendedToolOutputAnyThisStep = true
						continue
					}
					results := make([]types.HostOpResponse, len(batchReq.Operations))
					if batchReq.Parallel {
						var wg sync.WaitGroup
						wg.Add(len(batchReq.Operations))
						for i, sub := range batchReq.Operations {
							i, sub := i, sub
							go func() {
								defer wg.Done()
								results[i] = a.Exec.Exec(ctx, sub)
							}()
						}
						wg.Wait()
					} else {
						for i, sub := range batchReq.Operations {
							results[i] = a.Exec.Exec(ctx, sub)
						}
					}
					okAll := true
					for _, r := range results {
						if !r.Ok {
							okAll = false
							break
						}
					}
					batchResp := types.HostOpBatchResponse{Ok: okAll, Results: results}
					batchRespJSON, _ := types.MarshalPretty(batchResp)
					msgs = append(msgs, types.LLMMessage{
						Role:       "tool",
						ToolCallID: strings.TrimSpace(tc.ID),
						Content:    string(batchRespJSON),
					})
					appendedToolOutputAnyThisStep = true
					if okAll {
						appendedToolOutputOkThisStep = true
					}
					continue
				}

				// Special: batch (aka fs_batch) -> HostOpBatchResponse
				if which == "batch" || which == "fs_batch" {
					var args struct {
						Parallel   *bool                 `json:"parallel"`
						Operations []types.HostOpRequest `json:"operations"`
					}
					if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
						// #region agent log
						debuglog.Log("toolcalling", "H5", "loop.go:runConversation", "batch_args_unmarshal_err", map[string]any{
							"step": step,
							"tool": which,
							"err":  errString(err),
						})
						// #endregion
						batchResp := types.HostOpBatchResponse{Ok: false, Error: "batch args were not valid JSON: " + err.Error()}
						batchRespJSON, _ := types.MarshalPretty(batchResp)
						// #region agent log
						debuglog.Log("toolcalling", "H8", "loop.go:runConversation", "tool_output_error_sent", map[string]any{
							"step": step,
							"tool": which,
						})
						// #endregion
						msgs = append(msgs, types.LLMMessage{Role: "tool", ToolCallID: strings.TrimSpace(tc.ID), Content: string(batchRespJSON)})
						appendedToolOutputAnyThisStep = true
						continue
					}
					parallel := false
					if args.Parallel != nil {
						parallel = *args.Parallel
					}
					// #region agent log
					toolRunN := 0
					for _, sub := range args.Operations {
						if strings.TrimSpace(sub.Op) == types.HostOpToolRun {
							toolRunN++
						}
					}
					debuglog.Log("toolcalling", "H5", "loop.go:runConversation", "fs_batch_exec", map[string]any{
						"step":       step,
						"parallel":   parallel,
						"opsLen":     len(args.Operations),
						"toolRunOps": toolRunN,
					})
					// #endregion
					batchReq := types.HostOpBatchRequest{Op: types.HostOpFSBatch, Operations: args.Operations, Parallel: parallel}
					// Execute batch with per-operation validation so a single bad operation
					// doesn't waste the entire LLM turn (reduces retry loops).
					results := make([]types.HostOpResponse, len(batchReq.Operations))
					valid := make([]bool, len(batchReq.Operations))
					invalidN := 0
					for i, sub := range batchReq.Operations {
						if strings.TrimSpace(sub.Op) == types.HostOpFinal {
							results[i] = types.HostOpResponse{Op: strings.TrimSpace(sub.Op), Ok: false, Error: "final is not allowed in batch"}
							invalidN++
							continue
						}
						if err := sub.Validate(); err != nil {
							results[i] = types.HostOpResponse{Op: strings.TrimSpace(sub.Op), Ok: false, Error: err.Error()}
							invalidN++
							continue
						}
						valid[i] = true
					}
					if batchReq.Parallel {
						var wg sync.WaitGroup
						for i, sub := range batchReq.Operations {
							if !valid[i] {
								continue
							}
							wg.Add(1)
							i, sub := i, sub
							go func() {
								defer wg.Done()
								results[i] = a.Exec.Exec(ctx, sub)
							}()
						}
						wg.Wait()
					} else {
						for i, sub := range batchReq.Operations {
							if !valid[i] {
								continue
							}
							results[i] = a.Exec.Exec(ctx, sub)
						}
					}
					okAll := true
					for _, r := range results {
						if !r.Ok {
							okAll = false
							break
						}
					}
					batchResp := types.HostOpBatchResponse{Ok: okAll, Results: results}
					batchRespJSON, _ := types.MarshalPretty(batchResp)
					msgs = append(msgs, types.LLMMessage{
						Role:       "tool",
						ToolCallID: strings.TrimSpace(tc.ID),
						Content:    string(batchRespJSON),
					})
					appendedToolOutputAnyThisStep = true
					if okAll {
						appendedToolOutputOkThisStep = true
					}
					continue
				}

				op, err := functionCallToHostOp(tc, a.ToolFunctionRoutes)
				if err != nil {
					// #region agent log
					debuglog.Log("toolcalling", "H11", "loop.go:runConversation", "tool_call_args_invalid", map[string]any{
						"step":    step,
						"tool":    which,
						"err":     errString(err),
						"argsLen": len(tc.Function.Arguments),
					})
					// #endregion
					hostResp := types.HostOpResponse{Op: "tool_call", Ok: false, Error: "invalid tool call args: " + err.Error()}
					hostRespJSON, _ := types.MarshalPretty(hostResp)
					// #region agent log
					debuglog.Log("toolcalling", "H8", "loop.go:runConversation", "tool_output_error_sent", map[string]any{
						"step": step,
						"tool": which,
					})
					// #endregion
					msgs = append(msgs, types.LLMMessage{Role: "tool", ToolCallID: strings.TrimSpace(tc.ID), Content: string(hostRespJSON)})
					appendedToolOutputAnyThisStep = true
					continue
				}

				// #region agent log
				// Log op *shape* (not content) so we can see what the model is reading/listing.
				debuglog.Log("toolcalling", "H11", "loop.go:runConversation", "host_op_exec", map[string]any{
					"step":     step,
					"tool":     which,
					"op":       strings.TrimSpace(op.Op),
					"path":     strings.TrimSpace(op.Path),
					"maxBytes": op.MaxBytes,
					"textLen":  len(op.Text),
					"toolId":   op.ToolID.String(),
					"actionId": strings.TrimSpace(op.ActionID),
					"inputLen": len(op.Input),
				})
				// #endregion

				hostResp := a.Exec.Exec(ctx, op)
				hostRespJSON, _ := types.MarshalPretty(hostResp)
				msgs = append(msgs, types.LLMMessage{
					Role:       "tool",
					ToolCallID: strings.TrimSpace(tc.ID),
					Content:    string(hostRespJSON),
				})
				appendedToolOutputAnyThisStep = true
				if hostResp.Ok {
					appendedToolOutputOkThisStep = true
				}
			}

			if appendedToolOutputAnyThisStep {
				// We have satisfied the tool-calling protocol for this step (no dangling call_ids).
			}
			if appendedToolOutputOkThisStep {
				turnHasToolOutput = true
			}

			// Persist checkpoint after executing tool calls for this step.
			if strings.TrimSpace(checkpointPath) != "" {
				if err := SaveAgentCheckpoint(checkpointPath, AgentCheckpoint{
					UserMessage:    turnUserMessage,
					NextStep:       step + 1,
					Messages:       msgs,
					LastOp:         lastOpForCheckpoint,
					LastResponseID: lastResponseID,
				}); err != nil {
					return "", msgs, step, err
				}
			}
			continue
		}

		// Fallback path: JSON parsing for providers/models without tool calling output.
		// For environment turns we require tools; plain assistant text is treated as an invalid step.
		if envWanted, _ := turnFlags["envWanted"].(bool); envWanted {
			trim := strings.TrimSpace(resp.Text)
			if trim != "" {
				msgs = append(msgs,
					types.LLMMessage{Role: "assistant", Content: resp.Text},
					types.LLMMessage{Role: "user", Content: "Continue by calling the next tool. Do not respond in plain text. When fully done, call final_answer({\"text\":\"...\"})."},
				)
				continue
			}
		}

		opJSON, parseErr := extractSingleJSONObject(resp.Text)
		if parseErr != nil {
			trim := strings.TrimSpace(resp.Text)
			// If the model produced a normal assistant reply (not a host op JSON object),
			// treat it as final (but only if tools were not required for this turn).
			if trim != "" && !strings.HasPrefix(trim, "{") && !strings.HasPrefix(trim, "```") && !strings.Contains(trim, "\"op\"") {
				envWanted, _ := turnFlags["envWanted"].(bool)
				if envWanted && !turnHasToolOutput {
					msgs = append(msgs, types.LLMMessage{
						Role:    "user",
						Content: "You must use the environment tools for this request (do not answer from assumptions). Start by batching what you need (prefer batch(...)).",
					})
					continue
				}
				// #region agent log
				debuglog.Log("toolcalling", "H4", "loop.go:runConversation", "return_final", map[string]any{
					"step":    step,
					"reason":  "assistant_text_no_tool_calls",
					"textLen": len(trim),
				})
				// #endregion
				_ = ClearAgentCheckpoint(checkpointPath)
				msgs = append(msgs, types.LLMMessage{Role: "assistant", Content: trim})
				return trim, msgs, step, nil
			}
			msgs = append(msgs,
				types.LLMMessage{Role: "assistant", Content: resp.Text},
				types.LLMMessage{Role: "user", Content: "Your last message was not valid JSON. Error: " + parseErr.Error() + ". Return ONLY one JSON object with a required \"op\" field."},
			)
			continue
		}
		lastOpForCheckpoint = strings.TrimSpace(opJSON)
		if a.Hooks.Logf != nil {
			a.Hooks.Logf("model -> host (step %d): %s", step, strings.TrimSpace(opJSON))
		}

		// Peek at op first so we can route batch requests before HostOpRequest validation.
		var opHead struct {
			Op string `json:"op"`
		}
		if err := json.Unmarshal([]byte(opJSON), &opHead); err != nil {
			// Feed the parse error back to the model as a user message and keep going.
			msgs = append(msgs,
				types.LLMMessage{Role: "assistant", Content: resp.Text},
				types.LLMMessage{Role: "user", Content: "Your last message was not valid JSON for the required schema. Error: " + err.Error() + ". Return ONLY one JSON object."},
			)
			continue
		}

		if strings.TrimSpace(opHead.Op) == types.HostOpBatch || strings.TrimSpace(opHead.Op) == types.HostOpFSBatch {
			// Back-compat: accept both op:"fs.batch" and op:"batch", and accept both
			// field names "operations" (preferred) and "ops" (common model mistake).
			var batchCompat struct {
				Op         string                `json:"op"`
				Operations []types.HostOpRequest `json:"operations"`
				Ops        []types.HostOpRequest `json:"ops"`
				Parallel   bool                  `json:"parallel,omitempty"`
			}
			if err := json.Unmarshal([]byte(opJSON), &batchCompat); err != nil {
				msgs = append(msgs,
					types.LLMMessage{Role: "assistant", Content: resp.Text},
					types.LLMMessage{Role: "user", Content: "Your last JSON op was not valid JSON for the required schema. Error: " + err.Error() + ". Return ONLY one JSON object."},
				)
				continue
			}
			ops := batchCompat.Operations
			if len(ops) == 0 && len(batchCompat.Ops) != 0 {
				ops = batchCompat.Ops
			}
			batchReq := types.HostOpBatchRequest{Op: strings.TrimSpace(batchCompat.Op), Operations: ops, Parallel: batchCompat.Parallel}

			if a.Hooks.Logf != nil {
				a.Hooks.Logf("executing batch (step %d): ops=%d parallel=%v", step, len(batchReq.Operations), batchReq.Parallel)
			}

			// An empty batch is always invalid (it would otherwise "succeed" with zero work).
			if len(batchReq.Operations) == 0 {
				msgs = append(msgs,
					types.LLMMessage{Role: "assistant", Content: opJSON},
					types.LLMMessage{Role: "user", Content: "Your last JSON op was invalid: operations must be non-empty.\n\nReturn ONLY one corrected JSON object."},
				)
				continue
			}

			// Execute with per-operation validation so a single invalid operation doesn't waste the turn.
			results := make([]types.HostOpResponse, len(batchReq.Operations))
			valid := make([]bool, len(batchReq.Operations))
			for i, sub := range batchReq.Operations {
				if strings.TrimSpace(sub.Op) == types.HostOpFinal {
					results[i] = types.HostOpResponse{Op: strings.TrimSpace(sub.Op), Ok: false, Error: "final is not allowed in batch"}
					continue
				}
				if err := sub.Validate(); err != nil {
					results[i] = types.HostOpResponse{Op: strings.TrimSpace(sub.Op), Ok: false, Error: err.Error()}
					continue
				}
				valid[i] = true
			}
			if batchReq.Parallel {
				var wg sync.WaitGroup
				for i, sub := range batchReq.Operations {
					if !valid[i] {
						continue
					}
					wg.Add(1)
					i, sub := i, sub
					go func() {
						defer wg.Done()
						results[i] = a.Exec.Exec(ctx, sub)
					}()
				}
				wg.Wait()
			} else {
				for i, sub := range batchReq.Operations {
					if !valid[i] {
						continue
					}
					results[i] = a.Exec.Exec(ctx, sub)
				}
			}

			okAll := true
			for _, r := range results {
				if !r.Ok {
					okAll = false
					break
				}
			}
			batchResp := types.HostOpBatchResponse{Ok: okAll, Results: results}
			batchRespJSON, _ := types.MarshalPretty(batchResp)

			msgs = append(msgs,
				types.LLMMessage{Role: "assistant", Content: opJSON},
				types.LLMMessage{
					Role: "user",
					Content: "HostOpBatchResponse:\n" + string(batchRespJSON) +
						"\n\nReturn the next HostOpRequest as ONE JSON object (or {\"op\":\"final\",\"text\":\"...\"}).",
				},
			)
			if strings.TrimSpace(checkpointPath) != "" {
				if err := SaveAgentCheckpoint(checkpointPath, AgentCheckpoint{
					UserMessage:    turnUserMessage,
					NextStep:       step + 1,
					Messages:       msgs,
					LastOp:         lastOpForCheckpoint,
					LastResponseID: lastResponseID,
				}); err != nil {
					return "", msgs, step, err
				}
			}
			continue
		}

		var op types.HostOpRequest
		if err := json.Unmarshal([]byte(opJSON), &op); err != nil {
			// Feed the parse error back to the model as a user message and keep going.
			msgs = append(msgs,
				types.LLMMessage{Role: "assistant", Content: resp.Text},
				types.LLMMessage{Role: "user", Content: "Your last message was not valid JSON for the required schema. Error: " + err.Error() + ". Return ONLY one JSON object."},
			)
			continue
		}

		if err := validateModelOp(op); err != nil {
			hint := validationHint(op, err)
			msgs = append(msgs,
				types.LLMMessage{Role: "assistant", Content: resp.Text},
				types.LLMMessage{Role: "user", Content: "Your last JSON op was invalid: " + err.Error() + ".\n\nReturn ONLY one corrected JSON object. " + hint},
			)
			continue
		}

		if op.Op == "final" {
			msgs = append(msgs, types.LLMMessage{Role: "assistant", Content: strings.TrimSpace(opJSON)})
			_ = ClearAgentCheckpoint(checkpointPath)
			return strings.TrimSpace(op.Text), msgs, step, nil
		}

		hostResp := a.Exec.Exec(ctx, op)
		hostRespJSON, _ := types.MarshalPretty(hostResp)

		// Feed the host response back to the model as the next user turn.
		msgs = append(msgs,
			types.LLMMessage{Role: "assistant", Content: opJSON},
			types.LLMMessage{
				Role: "user",
				Content: "HostOpResponse:\n" + string(hostRespJSON) +
					"\n\nReturn the next HostOpRequest as ONE JSON object (or {\"op\":\"final\",\"text\":\"...\"}).",
			},
		)
		if strings.TrimSpace(checkpointPath) != "" {
			if err := SaveAgentCheckpoint(checkpointPath, AgentCheckpoint{
				UserMessage:    turnUserMessage,
				NextStep:       step + 1,
				Messages:       msgs,
				LastOp:         lastOpForCheckpoint,
				LastResponseID: lastResponseID,
			}); err != nil {
				return "", msgs, step, err
			}
		}
	}

	// MaxSteps reached: attempt one-shot graceful finalization.
	if strings.TrimSpace(checkpointPath) != "" {
		_ = SaveAgentCheckpoint(checkpointPath, AgentCheckpoint{
			UserMessage:    turnUserMessage,
			NextStep:       maxSteps + 1,
			Messages:       msgs,
			LastOp:         lastOpForCheckpoint,
			LastResponseID: lastResponseID,
		})
	}
	final, updated, usedSteps, err := a.finalizeOnMaxSteps(ctx, baseSystem, msgs, maxSteps+1, lastResponseID)
	if err != nil {
		return "", updated, maxSteps, fmt.Errorf("agent exceeded max steps (%d) without final: %w", maxSteps, err)
	}
	_ = ClearAgentCheckpoint(checkpointPath)
	return final, updated, usedSteps, nil
}

func (a *Agent) finalizeOnMaxSteps(ctx context.Context, baseSystem string, msgs []types.LLMMessage, step int, lastResponseID string) (final string, updated []types.LLMMessage, usedSteps int, err error) {
	// Ask the model to stop and provide a final summary without further host ops.
	msgs = append(msgs, types.LLMMessage{
		Role:    "user",
		Content: "You have reached the maximum step limit. Return a final response summarizing what has been completed so far, what remains, and any important context for resuming. Return ONLY one JSON object: {\"op\":\"final\",\"text\":\"...\"}.",
	})

	system := baseSystem
	if a.Context != nil {
		updatedSys, sysErr := a.Context.SystemPrompt(ctx, baseSystem, step)
		if sysErr != nil {
			return "", msgs, step, sysErr
		}
		system = updatedSys
	}

	req := types.LLMRequest{
		Model:              a.Model,
		System:             system,
		Messages:           msgs,
		MaxTokens:          1024,
		JSONOnly:           true,
		ResponseSchema:     hostOpResponseSchema(),
		EnableWebSearch:    a.EnableWebSearch,
		PreviousResponseID: strings.TrimSpace(lastResponseID),
		ReasoningEffort:    strings.TrimSpace(a.ReasoningEffort),
		ReasoningSummary:   strings.TrimSpace(a.ReasoningSummary),
	}

	var resp types.LLMResponse
	var callErr error
	if s, ok := a.LLM.(types.LLMClientStreaming); ok {
		dec := &finalTextStreamDecoder{}
		// #region agent log
		streamTextChunkN := 0
		streamTextBytes := 0
		streamDecoderEmittedBytes := 0
		streamRawEmittedBytes := 0
		streamLoggedFirst := false
		streamMode := "unknown" // "unknown" | "raw" | "json"
		streamModeLogged := false
		var streamPrefix strings.Builder
		const streamPrefixMax = 1024
		// #endregion
		// #region agent log
		// Capture provider-supplied reasoning summaries (NOT raw chain-of-thought).
		var reasoningBuf strings.Builder
		reasoningTextN := 0
		reasoningSignalN := 0
		reasoningCharsTotal := 0
		// #endregion
		resp, callErr = s.GenerateStream(ctx, req, func(chunk types.LLMStreamChunk) error {
			if chunk.Done {
				if a.Hooks.OnStreamChunk != nil {
					a.Hooks.OnStreamChunk(step, chunk)
				}
				// #region agent log
				debuglog.Log("toolcalling", "H16", "loop.go:finalizeOnMaxSteps", "stream_text_totals", map[string]any{
					"step":              step,
					"model":             strings.TrimSpace(a.Model),
					"toolChoice":        "none",
					"jsonOnly":          req.JSONOnly,
					"hasSchema":         req.ResponseSchema != nil,
					"mode":              streamMode,
					"textChunks":        streamTextChunkN,
					"textBytes":         streamTextBytes,
					"decoderEmittedB":   streamDecoderEmittedBytes,
					"decoderEmittedAny": streamDecoderEmittedBytes != 0,
					"rawEmittedB":       streamRawEmittedBytes,
					"rawEmittedAny":     streamRawEmittedBytes != 0,
				})
				// #endregion
				// #region agent log
				if reasoningSignalN != 0 || reasoningTextN != 0 {
					prev := reasoningBuf.String()
					if len(prev) > 800 {
						prev = prev[:800] + "…"
					}
					debuglog.Log("toolcalling", "H13", "loop.go:finalizeOnMaxSteps", "reasoning_summary_seen", map[string]any{
						"step":               step,
						"toolChoice":         "none",
						"reasoningSignalN":   reasoningSignalN,
						"reasoningTextN":     reasoningTextN,
						"reasoningChars":     reasoningCharsTotal,
						"reasoningStoredLen": reasoningBuf.Len(),
						"reasoningPreview":   prev,
					})
				}
				// #endregion
				return nil
			}
			if chunk.IsReasoning {
				if a.Hooks.OnStreamChunk != nil {
					a.Hooks.OnStreamChunk(step, chunk)
				}
				// #region agent log
				reasoningSignalN++
				if chunk.Text != "" {
					reasoningTextN++
					reasoningCharsTotal += len(chunk.Text)
					if reasoningBuf.Len() < 8*1024 {
						remain := 8*1024 - reasoningBuf.Len()
						if len(chunk.Text) > remain {
							reasoningBuf.WriteString(chunk.Text[:remain])
						} else {
							reasoningBuf.WriteString(chunk.Text)
						}
					}
				}
				// #endregion
				return nil
			}
			if chunk.Text == "" {
				return nil
			}
			// #region agent log
			streamTextChunkN++
			streamTextBytes += len(chunk.Text)
			// #endregion
			emit := func(s string) {
				if s == "" {
					return
				}
				// #region agent log
				streamRawEmittedBytes += len(s)
				// #endregion
				if a.Hooks.OnToken != nil {
					a.Hooks.OnToken(step, s)
				}
			}

			if streamMode == "unknown" {
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
				// #region agent log
				if !streamModeLogged {
					streamModeLogged = true
					debuglog.Log("toolcalling", "H16", "loop.go:finalizeOnMaxSteps", "stream_mode_selected", map[string]any{
						"step":      step,
						"model":     strings.TrimSpace(a.Model),
						"mode":      streamMode,
						"firstByte": string([]byte{first}),
					})
				}
				// #endregion
				prefix := streamPrefix.String()
				streamPrefix.Reset()
				if streamMode == "raw" {
					emit(prefix)
					return nil
				}
				out := dec.Consume(prefix)
				// #region agent log
				streamDecoderEmittedBytes += len(out)
				// #endregion
				if !streamLoggedFirst {
					streamLoggedFirst = true
					prev := prefix
					if len(prev) > 120 {
						prev = prev[:120] + "…"
					}
					debuglog.Log("toolcalling", "H16", "loop.go:finalizeOnMaxSteps", "stream_first_text", map[string]any{
						"step":      step,
						"model":     strings.TrimSpace(a.Model),
						"mode":      streamMode,
						"chunkLen":  len(prefix),
						"chunkPrev": prev,
						"outLen":    len(out),
					})
				}
				if out != "" {
					emit(out)
				}
				return nil
			}

			if streamMode == "raw" {
				if !streamLoggedFirst {
					streamLoggedFirst = true
					prev := chunk.Text
					if len(prev) > 120 {
						prev = prev[:120] + "…"
					}
					// #region agent log
					debuglog.Log("toolcalling", "H16", "loop.go:finalizeOnMaxSteps", "stream_first_text", map[string]any{
						"step":      step,
						"model":     strings.TrimSpace(a.Model),
						"mode":      streamMode,
						"chunkLen":  len(chunk.Text),
						"chunkPrev": prev,
						"outLen":    len(chunk.Text),
					})
					// #endregion
				}
				emit(chunk.Text)
				return nil
			}

			out := dec.Consume(chunk.Text)
			// #region agent log
			streamDecoderEmittedBytes += len(out)
			// #endregion
			if !streamLoggedFirst {
				streamLoggedFirst = true
				prev := chunk.Text
				if len(prev) > 120 {
					prev = prev[:120] + "…"
				}
				debuglog.Log("toolcalling", "H16", "loop.go:finalizeOnMaxSteps", "stream_first_text", map[string]any{
					"step":      step,
					"model":     strings.TrimSpace(a.Model),
					"mode":      streamMode,
					"chunkLen":  len(chunk.Text),
					"chunkPrev": prev,
					"outLen":    len(out),
				})
			}
			if out != "" {
				emit(out)
			}
			return nil
		})
	} else {
		resp, callErr = a.LLM.Generate(ctx, req)
	}
	if callErr != nil {
		return "", msgs, step, callErr
	}

	opJSON, parseErr := extractSingleJSONObject(resp.Text)
	if parseErr != nil {
		return "", msgs, step, parseErr
	}

	var op types.HostOpRequest
	if err := json.Unmarshal([]byte(opJSON), &op); err != nil {
		return "", msgs, step, err
	}
	if err := validateModelOp(op); err != nil {
		return "", msgs, step, err
	}
	if strings.TrimSpace(op.Op) != types.HostOpFinal {
		return "", msgs, step, fmt.Errorf("expected op=final, got %q", strings.TrimSpace(op.Op))
	}

	msgs = append(msgs, types.LLMMessage{Role: "assistant", Content: strings.TrimSpace(opJSON)})
	return strings.TrimSpace(op.Text), msgs, step, nil
}

func extractTurnUserMessage(msgs []types.LLMMessage) string {
	// Best-effort heuristic: find the most recent user message that is not a host op response wrapper.
	for i := len(msgs) - 1; i >= 0; i-- {
		m := msgs[i]
		if strings.TrimSpace(m.Role) != "user" {
			continue
		}
		c := strings.TrimSpace(m.Content)
		if c == "" {
			continue
		}
		if strings.HasPrefix(c, "HostOpResponse:") || strings.HasPrefix(c, "HostOpBatchResponse:") {
			continue
		}
		if strings.HasPrefix(c, "Your last message was not valid JSON") || strings.HasPrefix(c, "Your last JSON op was invalid:") {
			continue
		}
		return c
	}
	return ""
}

func functionCallToHostOp(tc types.ToolCall, routes map[string]ToolRoute) (types.HostOpRequest, error) {
	name := strings.TrimSpace(tc.Function.Name)
	argsJSON := []byte(tc.Function.Arguments)

	if route, ok := routes[name]; ok {
		if len(strings.TrimSpace(tc.Function.Arguments)) == 0 {
			argsJSON = []byte(`{}`)
		}
		var input json.RawMessage
		if err := json.Unmarshal(argsJSON, &input); err != nil {
			return types.HostOpRequest{}, err
		}
		if input == nil {
			input = json.RawMessage(`{}`)
		}
		timeout := route.TimeoutMs
		if timeout <= 0 {
			timeout = defaultToolFunctionTimeoutMs
		}
		return types.HostOpRequest{
			Op:        types.HostOpToolRun,
			ToolID:    route.ToolID,
			ActionID:  strings.TrimSpace(route.ActionID),
			Input:     input,
			TimeoutMs: timeout,
		}, nil
	}

	switch name {
	case "fs_list":
		var args struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal(argsJSON, &args); err != nil {
			return types.HostOpRequest{}, err
		}
		return types.HostOpRequest{Op: types.HostOpFSList, Path: strings.TrimSpace(args.Path)}, nil

	case "fs_read":
		var args struct {
			Path     string `json:"path"`
			MaxBytes *int   `json:"maxBytes"`
		}
		if err := json.Unmarshal(argsJSON, &args); err != nil {
			return types.HostOpRequest{}, err
		}
		maxBytes := 0
		if args.MaxBytes != nil {
			maxBytes = *args.MaxBytes
		}
		return types.HostOpRequest{Op: types.HostOpFSRead, Path: strings.TrimSpace(args.Path), MaxBytes: maxBytes}, nil

	case "fs_write":
		var args struct {
			Path string `json:"path"`
			Text string `json:"text"`
		}
		if err := json.Unmarshal(argsJSON, &args); err != nil {
			return types.HostOpRequest{}, err
		}
		return types.HostOpRequest{Op: types.HostOpFSWrite, Path: strings.TrimSpace(args.Path), Text: args.Text}, nil

	case "fs_append":
		var args struct {
			Path string `json:"path"`
			Text string `json:"text"`
		}
		if err := json.Unmarshal(argsJSON, &args); err != nil {
			return types.HostOpRequest{}, err
		}
		return types.HostOpRequest{Op: types.HostOpFSAppend, Path: strings.TrimSpace(args.Path), Text: args.Text}, nil

	case "fs_edit":
		var args struct {
			Path  string `json:"path"`
			Edits []struct {
				Old        string `json:"old"`
				New        string `json:"new"`
				Occurrence int    `json:"occurrence"`
			} `json:"edits"`
		}
		if err := json.Unmarshal(argsJSON, &args); err != nil {
			return types.HostOpRequest{}, err
		}
		inp, _ := json.Marshal(map[string]any{"edits": args.Edits})
		return types.HostOpRequest{Op: types.HostOpFSEdit, Path: strings.TrimSpace(args.Path), Input: inp}, nil

	case "fs_patch":
		var args struct {
			Path string `json:"path"`
			Text string `json:"text"`
		}
		if err := json.Unmarshal(argsJSON, &args); err != nil {
			return types.HostOpRequest{}, err
		}
		return types.HostOpRequest{Op: types.HostOpFSPatch, Path: strings.TrimSpace(args.Path), Text: args.Text}, nil

	case "tool_run":
		var args struct {
			ToolID    string          `json:"toolId"`
			ActionID  string          `json:"actionId"`
			Input     json.RawMessage `json:"input"`
			TimeoutMs *int            `json:"timeoutMs"`
		}
		if err := json.Unmarshal(argsJSON, &args); err != nil {
			return types.HostOpRequest{}, err
		}
		// H3 fix: some models omit input entirely; treat missing input as empty object.
		if args.Input == nil {
			args.Input = json.RawMessage(`{}`)
		}
		timeout := 0
		if args.TimeoutMs != nil {
			timeout = *args.TimeoutMs
		}
		return types.HostOpRequest{
			Op:        types.HostOpToolRun,
			ToolID:    types.ToolID(strings.TrimSpace(args.ToolID)),
			ActionID:  strings.TrimSpace(args.ActionID),
			Input:     args.Input,
			TimeoutMs: timeout,
		}, nil

	default:
		return types.HostOpRequest{}, fmt.Errorf("unknown tool function %q", name)
	}
}

// Run executes the agent loop for a single user goal and returns the final response text.
func (a *Agent) Run(ctx context.Context, goal string) (string, error) {
	if err := validate.NonEmpty("goal", goal); err != nil {
		return "", err
	}
	final, _, _, err := a.RunConversation(ctx, []types.LLMMessage{
		{Role: "user", Content: goal},
	})
	return final, err
}

func agentLoopV0SystemPrompt() string {
	return strings.TrimSpace(`
You are an agent running inside a host-controlled environment.

Workbench uses tool/function calling for environment operations.

You may either:
  - call one or more provided tools (functions) to interact with the environment, OR
  - respond normally in plain text when no environment interaction is needed.

Critical rules:
  - All VFS paths MUST be absolute (start with "/"). Never use "." or relative paths.
  - Do NOT call tools unnecessarily. If the user says "hi" or asks a question you can answer without file/tool access, answer directly without calling tools.
  - If the user explicitly requests environment interaction (files, repo, tools, builtin.trace), you MUST use the tools. Do NOT answer from assumptions.

When you need the environment, use the provided function tools:
  - final_answer (end the turn with the final response)
  - fs_list, fs_read, fs_write, fs_append, fs_edit, fs_patch
  - batch (batch of host ops)
  - tool_run (execute a discovered tool)

To end the turn:
  - Call final_answer with the final response text.

Batch notes:
  - Prefer batch for multiple independent ops.
  - If you need 2+ tool executions, DO NOT call tool_run repeatedly. Use batch with multiple operations where each operation is {"op":"tool.run", ... }.
  - batch.operations is a list of host op objects like {"op":"fs.read",...}, not tool calls.

Tool discovery rules:
  - Tools are discovered via VFS only:
      fs_list("/tools") returns tool IDs as directory entries.
      fs_read("/tools/<toolId>") returns the tool manifest JSON bytes.
  - Do NOT assume tool actions; read the manifest first.

Tool execution + results rules:
  - To run a tool, use tool_run(...) with the selected toolId/actionId and an input object.
  - After tool.run, the host persists:
      /results/<callId>/response.json
      /results/<callId>/<artifactPath>
  - You can read those files with fs_read to inspect outputs/artifacts.

Always:
  - Prefer discovering tools first when tool usage is required.
  - Use small reads (maxBytes) unless you need more.
  - If a tool or host op fails, recover and try a different approach or answer with what you know.
`)
}

func toolChoiceForTurn(userMsg string, turnHasToolOutput bool) (choice string, reason string, flags map[string]any) {
	s := strings.ToLower(strings.TrimSpace(userMsg))
	flags = map[string]any{
		"empty":       s == "",
		"isShort":     len(s) <= 8,
		"hasAtRef":    strings.Contains(s, "@"),
		"hasTrace":    strings.Contains(s, "trace") || strings.Contains(s, "builtin.trace"),
		"hasToolWord": strings.Contains(s, "tool") || strings.Contains(s, "tools"),
		"hasFileWord": strings.Contains(s, "file") || strings.Contains(s, "files"),
		"hasRepoWord": strings.Contains(s, "repo") || strings.Contains(s, "codebase") || strings.Contains(s, "project"),
	}
	// Treat "tools" as environment intent only when the user is asking to enumerate/run/discover them,
	// not when they're using the word "tool" conceptually.
	hasToolWord := flags["hasToolWord"].(bool)
	toolIntent := hasToolWord && (strings.Contains(s, "list") || strings.Contains(s, "show") || strings.Contains(s, "available") || strings.Contains(s, "discover") || strings.Contains(s, "run"))

	// NOTE: We intentionally do NOT force toolChoice=required solely on "file/files" because
	// some models get stuck repeatedly calling fs.write/fs.read. For file tasks, system prompt
	// guidance is usually sufficient with toolChoice=auto.
	envWanted := flags["hasAtRef"].(bool) || flags["hasTrace"].(bool) || flags["hasRepoWord"].(bool) || toolIntent
	flags["envWanted"] = envWanted
	flags["turnHasToolOutput"] = turnHasToolOutput

	// Avoid forcing tools for greetings / very short chit-chat.
	switch s {
	case "", "hi", "hey", "hello", "yo", "sup", "thanks", "thank you":
		return "auto", "greeting_or_empty", flags
	}

	// If the user is clearly requesting environment interaction, force tool calling
	// for the entire turn. The model must call tools repeatedly and end with final_answer.
	_ = turnHasToolOutput
	if envWanted {
		return "required", "explicit_env_request", flags
	}

	return "auto", "default_auto", flags
}

func indexOfTurnUserMessage(msgs []types.LLMMessage) int {
	for i := len(msgs) - 1; i >= 0; i-- {
		m := msgs[i]
		if strings.TrimSpace(m.Role) != "user" {
			continue
		}
		c := strings.TrimSpace(m.Content)
		if c == "" {
			continue
		}
		if strings.HasPrefix(c, "HostOpResponse:") || strings.HasPrefix(c, "HostOpBatchResponse:") {
			continue
		}
		if strings.HasPrefix(c, "Your last message was not valid JSON") || strings.HasPrefix(c, "Your last JSON op was invalid:") {
			continue
		}
		return i
	}
	return -1
}

func hasToolMessageAfterIdx(msgs []types.LLMMessage, idx int) bool {
	if idx < -1 {
		idx = -1
	}
	for i := idx + 1; i < len(msgs); i++ {
		if strings.TrimSpace(strings.ToLower(msgs[i].Role)) == "tool" {
			return true
		}
	}
	return false
}

func validateModelOp(op types.HostOpRequest) error {
	return op.Validate()
}

func summarizeModelObject(opJSON string, step int) map[string]any {
	// Do NOT log raw model content (may contain PII). Only log structural metadata.
	var m map[string]json.RawMessage
	if err := json.Unmarshal([]byte(opJSON), &m); err != nil {
		return map[string]any{"step": step, "unmarshalErr": err.Error()}
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if len(keys) > 12 {
		keys = append(keys[:12], "…")
	}
	hasOp := false
	op := ""
	if b, ok := m["op"]; ok && len(b) != 0 {
		hasOp = true
		_ = json.Unmarshal(b, &op)
		op = strings.TrimSpace(op)
	}
	_, hasText := m["text"]
	_, hasOps := m["ops"]
	_, hasOperations := m["operations"]
	return map[string]any{
		"step":          step,
		"keys":          keys,
		"hasOp":         hasOp,
		"op":            op,
		"hasText":       hasText,
		"hasOps":        hasOps,
		"hasOperations": hasOperations,
	}
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	s := strings.TrimSpace(err.Error())
	if len(s) > 240 {
		return s[:240] + "…"
	}
	return s
}

func pathCategory(p string) string {
	p = strings.TrimSpace(p)
	switch {
	case strings.HasPrefix(p, "/tools/") || p == "/tools":
		return "/tools"
	case strings.HasPrefix(p, "/project/") || p == "/project":
		return "/project"
	case strings.HasPrefix(p, "/scratch/") || p == "/scratch":
		return "/scratch"
	case strings.HasPrefix(p, "/results/") || p == "/results":
		return "/results"
	case strings.HasPrefix(p, "/profile/") || p == "/profile":
		return "/profile"
	case strings.HasPrefix(p, "/memory/") || p == "/memory":
		return "/memory"
	case strings.HasPrefix(p, "/log") || p == "/log":
		return "/log"
	case strings.HasPrefix(p, "/"):
		return "/"
	default:
		if p == "" {
			return ""
		}
		return "relative"
	}
}

func validationHint(op types.HostOpRequest, err error) string {
	_ = err
	which := strings.TrimSpace(op.Op)
	switch which {
	case "":
		return "You are missing the required \"op\" field. Return exactly one object like {\"op\":\"fs.list\",\"path\":\"/tools\"}."
	case types.HostOpFSList:
		return "For fs.list you must include a non-empty absolute \"path\" starting with \"/\" (example: {\"op\":\"fs.list\",\"path\":\"/tools\"})."
	case types.HostOpFSRead:
		return "For fs.read you must include a non-empty absolute \"path\" starting with \"/\" (example: {\"op\":\"fs.read\",\"path\":\"/tools/builtin.shell\",\"maxBytes\":2048})."
	case types.HostOpFSWrite, types.HostOpFSAppend:
		return "For " + which + " you must include an absolute \"path\" starting with \"/\" and non-empty \"text\"."
	case types.HostOpFSEdit:
		return "For fs.edit you must include an absolute \"path\" starting with \"/\" and an \"input\" object with edits (example: {\"op\":\"fs.edit\",\"path\":\"/project/x.txt\",\"input\":{\"edits\":[{\"old\":\"a\",\"new\":\"b\",\"occurrence\":1}]}})."
	case types.HostOpFSPatch:
		return "For fs.patch you must include an absolute \"path\" starting with \"/\" and non-empty \"text\" (unified diff)."
	case types.HostOpToolRun:
		return "For tool.run you must include non-empty \"toolId\", \"actionId\", and an \"input\" object."
	case types.HostOpBatch, types.HostOpFSBatch:
		return "For " + which + " you must include a non-empty \"operations\" array (alias: \"ops\"). Example: {\"op\":\"fs.batch\",\"parallel\":true,\"operations\":[{\"op\":\"fs.list\",\"path\":\"/tools\"}]}"
	case types.HostOpFinal:
		return "For final you must include non-empty \"text\" (example: {\"op\":\"final\",\"text\":\"...\"})."
	default:
		// Common confusions: "cat", "ack", "noop", "http.get".
		lc := strings.ToLower(which)
		if lc == "cat" || lc == "ack" || lc == "noop" {
			return "You used an unknown op. If you want to run a shell command, you must first discover tools (fs.list(\"/tools\")), then use tool.run with builtin.shell."
		}
		if strings.Contains(lc, "http") || strings.Contains(lc, "get") {
			return "You used an unknown op. If you want HTTP, you must discover tools (fs.list(\"/tools\")), then use tool.run with builtin.http."
		}
		return "Allowed ops (exact strings): fs.list, fs.read, fs.write, fs.append, fs.edit, fs.patch, tool.run, fs.batch, batch, final."
	}
}
