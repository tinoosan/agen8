package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
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
			ResponseSchema:     hostOpResponseSchema(),
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

		opJSON, parseErr := extractSingleJSONObject(resp.Text)
		if parseErr != nil {
			msgs = append(msgs,
				types.LLMMessage{Role: "assistant", Content: resp.Text},
				types.LLMMessage{Role: "user", Content: "Your last message was not valid JSON. Error: " + parseErr.Error() + ". Return ONLY one JSON object with a required \"op\" field."},
			)
			continue
		}
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
				Op         string              `json:"op"`
				Operations []types.HostOpRequest `json:"operations"`
				Ops        []types.HostOpRequest `json:"ops"`
				Parallel   bool                `json:"parallel,omitempty"`
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

			if err := batchReq.Validate(); err != nil {
				msgs = append(msgs,
					types.LLMMessage{Role: "assistant", Content: resp.Text},
					types.LLMMessage{Role: "user", Content: "Your last JSON op was invalid: " + err.Error() + ".\n\nBatch rules:\n- Use op:\"fs.batch\" (alias: op:\"batch\")\n- Use field \"operations\" (alias: \"ops\")\n\nReturn ONLY one corrected JSON object."},
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
						r := a.Exec.Exec(ctx, sub)
						results[i] = r
					}()
				}
				wg.Wait()
			} else {
				for i, sub := range batchReq.Operations {
					r := a.Exec.Exec(ctx, sub)
					results[i] = r
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
			hint := validationHint(op, err)
			msgs = append(msgs,
				types.LLMMessage{Role: "assistant", Content: resp.Text},
				types.LLMMessage{Role: "user", Content: "Your last JSON op was invalid: " + err.Error() + ".\n\nReturn ONLY one corrected JSON object. " + hint},
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

Critical rules:
  - The JSON object MUST include an "op" string field.
  - The value of "op" MUST be EXACTLY one of the allowed strings below. Do NOT add extra words.
    Bad: {"op":"fs.batch DONT USE THIS PLEASE", ...}
    Good: {"op":"fs.batch", ...}
  - All VFS paths MUST be absolute (start with "/"). Never use "." or relative paths.
  - Do NOT emit helper/status objects like {"valid":true,...} or {"error":...} as your main response.

Allowed operations (host primitives; always available):
  - fs.list:   {"op":"fs.list","path":"/tools"}
  - fs.read:   {"op":"fs.read","path":"/tools/<toolId>","maxBytes":2048}
  - fs.write:  {"op":"fs.write","path":"/workspace/file.txt","text":"..."}
  - fs.append: {"op":"fs.append","path":"/workspace/log.txt","text":"..."}
  - fs.edit:   {"op":"fs.edit","path":"/workspace/file.txt","input":{"edits":[{"old":"...","new":"...","occurrence":1}]}}
  - fs.patch:  {"op":"fs.patch","path":"/workspace/file.txt","text":"--- a/file.txt\n+++ b/file.txt\n@@ -1,1 +1,1 @@\n-old\n+new\n"}
  - tool.run:  {"op":"tool.run","toolId":"<toolId>","actionId":"<actionId>","input":{...},"timeoutMs":5000}
  - fs.batch:  {"op":"fs.batch","parallel":true,"operations":[{"op":"fs.read","path":"/tools/<toolId>","maxBytes":2048}]}
  - final:     {"op":"final","text":"..."}   (stop)

Batch notes:
  - Alias: {"op":"batch", ...} is also accepted (op:"fs.batch" preferred).
  - Field alias: "ops" is accepted, but "operations" is preferred.

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
	case strings.HasPrefix(p, "/workdir/") || p == "/workdir":
		return "/workdir"
	case strings.HasPrefix(p, "/workspace/") || p == "/workspace":
		return "/workspace"
	case strings.HasPrefix(p, "/results/") || p == "/results":
		return "/results"
	case strings.HasPrefix(p, "/profile/") || p == "/profile":
		return "/profile"
	case strings.HasPrefix(p, "/memory/") || p == "/memory":
		return "/memory"
	case strings.HasPrefix(p, "/trace") || p == "/trace":
		return "/trace"
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
		return "For fs.read you must include a non-empty absolute \"path\" starting with \"/\" (example: {\"op\":\"fs.read\",\"path\":\"/tools/builtin.bash\",\"maxBytes\":2048})."
	case types.HostOpFSWrite, types.HostOpFSAppend:
		return "For " + which + " you must include an absolute \"path\" starting with \"/\" and non-empty \"text\"."
	case types.HostOpFSEdit:
		return "For fs.edit you must include an absolute \"path\" starting with \"/\" and an \"input\" object with edits (example: {\"op\":\"fs.edit\",\"path\":\"/workdir/x.txt\",\"input\":{\"edits\":[{\"old\":\"a\",\"new\":\"b\",\"occurrence\":1}]}})."
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
			return "You used an unknown op. If you want to run a shell command, you must first discover tools (fs.list(\"/tools\")), then use tool.run with builtin.bash."
		}
		if strings.Contains(lc, "http") || strings.Contains(lc, "get") {
			return "You used an unknown op. If you want HTTP, you must discover tools (fs.list(\"/tools\")), then use tool.run with builtin.http."
		}
		return "Allowed ops (exact strings): fs.list, fs.read, fs.write, fs.append, fs.edit, fs.patch, tool.run, fs.batch, batch, final."
	}
}
