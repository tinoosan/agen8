package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/tinoosan/workbench-core/internal/jsonutil"
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

	// ReasoningEffort is an optional hint for reasoning-capable models.
	ReasoningEffort string

	// ReasoningSummary controls whether and how providers should emit reasoning summaries.
	ReasoningSummary string

	// SystemPrompt is the base system instructions passed to the model.
	SystemPrompt string

	// Context optionally refreshes bounded context (memory/profile/trace/etc) per model step.
	Context ContextSource

	// MaxSteps caps the number of model -> host op iterations.
	// If zero, a default is used.
	MaxSteps int

	// Hooks are optional observability callbacks invoked by the agent loop.
	Hooks Hooks
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

	// Track the last Responses API response ID so we can preserve reasoning context
	// across steps via previous_response_id.
	var lastResponseID string

	for step := 1; step <= maxSteps; step++ {
		system := baseSystem
		if a.Context != nil {
			updated, err := a.Context.SystemPrompt(ctx, baseSystem, step)
			if err != nil {
				return "", nil, 0, err
			}
			system = updated
		}

		req := types.LLMRequest{
			Model:              a.Model,
			System:             system,
			Messages:           msgs,
			MaxTokens:          1024,
			JSONOnly:           true,
			PreviousResponseID: lastResponseID,
			ReasoningEffort:    strings.TrimSpace(a.ReasoningEffort),
			ReasoningSummary:   strings.TrimSpace(a.ReasoningSummary),
		}

		var resp types.LLMResponse
		var err error
		if s, ok := a.LLM.(types.LLMClientStreaming); ok {
			dec := &finalTextStreamDecoder{}
			resp, err = s.GenerateStream(ctx, req, func(chunk types.LLMStreamChunk) error {
				if chunk.Done {
					if a.Hooks.OnStreamChunk != nil {
						a.Hooks.OnStreamChunk(step, chunk)
					}
					return nil
				}
				if chunk.IsReasoning {
					if a.Hooks.OnStreamChunk != nil {
						a.Hooks.OnStreamChunk(step, chunk)
					}
					return nil
				}
				if chunk.Text == "" {
					return nil
				}
				// Stream only decoded final.text to the host via OnToken.
				if a.Hooks.OnToken != nil {
					if out := dec.Consume(chunk.Text); out != "" {
						a.Hooks.OnToken(step, out)
					}
				} else {
					_ = dec.Consume(chunk.Text)
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

		opJSON := extractJSONObject(resp.Text)
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

		if strings.TrimSpace(opHead.Op) == types.HostOpBatch {
			var batchReq types.HostOpBatchRequest
			if err := json.Unmarshal([]byte(opJSON), &batchReq); err != nil {
				msgs = append(msgs,
					types.LLMMessage{Role: "assistant", Content: resp.Text},
					types.LLMMessage{Role: "user", Content: "Your last JSON op was not valid JSON for the required schema. Error: " + err.Error() + ". Return ONLY one JSON object."},
				)
				continue
			}
			if err := batchReq.Validate(); err != nil {
				msgs = append(msgs,
					types.LLMMessage{Role: "assistant", Content: resp.Text},
					types.LLMMessage{Role: "user", Content: "Your last JSON op was invalid: " + err.Error() + ". Return ONLY one corrected JSON object."},
				)
				continue
			}

			if a.Hooks.Logf != nil {
				a.Hooks.Logf("executing batch (step %d): ops=%d parallel=%v", step, len(batchReq.Operations), batchReq.Parallel)
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
			batchRespJSON, _ := jsonutil.MarshalPretty(batchResp)

			msgs = append(msgs,
				types.LLMMessage{Role: "assistant", Content: opJSON},
				types.LLMMessage{
					Role: "user",
					Content: "HostOpBatchResponse:\n" + string(batchRespJSON) +
						"\n\nReturn the next HostOpRequest as ONE JSON object (or {\"op\":\"final\",\"text\":\"...\"}).",
				},
			)
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
			msgs = append(msgs,
				types.LLMMessage{Role: "assistant", Content: resp.Text},
				types.LLMMessage{Role: "user", Content: "Your last JSON op was invalid: " + err.Error() + ". Return ONLY one corrected JSON object."},
			)
			continue
		}

		if op.Op == "final" {
			msgs = append(msgs, types.LLMMessage{Role: "assistant", Content: strings.TrimSpace(opJSON)})
			return strings.TrimSpace(op.Text), msgs, step, nil
		}

		hostResp := a.Exec.Exec(ctx, op)
		hostRespJSON, _ := jsonutil.MarshalPretty(hostResp)

		// Feed the host response back to the model as the next user turn.
		msgs = append(msgs,
			types.LLMMessage{Role: "assistant", Content: opJSON},
			types.LLMMessage{
				Role: "user",
				Content: "HostOpResponse:\n" + string(hostRespJSON) +
					"\n\nReturn the next HostOpRequest as ONE JSON object (or {\"op\":\"final\",\"text\":\"...\"}).",
			},
		)
	}

	return "", msgs, maxSteps, fmt.Errorf("agent exceeded max steps (%d) without final", maxSteps)
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

You can ONLY interact with the environment by returning exactly ONE JSON object per turn.
Do not include any other text, markdown, or code fences.

Allowed operations (host primitives; always available):
  - fs.list:   {"op":"fs.list","path":"/tools"}
  - fs.read:   {"op":"fs.read","path":"/tools/<toolId>","maxBytes":2048}
  - fs.write:  {"op":"fs.write","path":"/workspace/file.txt","text":"..."}
  - fs.append: {"op":"fs.append","path":"/workspace/log.txt","text":"..."}
  - tool.run:  {"op":"tool.run","toolId":"<toolId>","actionId":"<actionId>","input":{...},"timeoutMs":5000}
  - final:     {"op":"final","text":"..."}   (stop)

Tool discovery rules:
  - Tools are discovered via VFS only:
      fs.list("/tools") returns tool IDs as directory entries.
      fs.read("/tools/<toolId>") returns the tool manifest JSON bytes.
  - Do NOT assume tool actions; read the manifest first.

Tool execution + results rules:
  - To run a tool, use tool.run(...) with the selected toolId/actionId and an input object.
  - After tool.run, the host persists:
      /results/<callId>/response.json
      /results/<callId>/<artifactPath>
  - You can read those files with fs.read to inspect outputs/artifacts.

Always:
  - Prefer discovering tools first.
  - Use small reads (maxBytes) unless you need more.
  - If a host op fails (ok=false), recover and try a different op or return final with an explanation.
`)
}

func extractJSONObject(s string) string {
	trim := strings.TrimSpace(s)
	trim = strings.TrimPrefix(trim, "```json")
	trim = strings.TrimPrefix(trim, "```")
	trim = strings.TrimSuffix(trim, "```")
	trim = strings.TrimSpace(trim)

	start := strings.Index(trim, "{")
	end := strings.LastIndex(trim, "}")
	if start >= 0 && end >= 0 && end > start {
		return trim[start : end+1]
	}
	return trim
}

func validateModelOp(op types.HostOpRequest) error {
	return op.Validate()
}
