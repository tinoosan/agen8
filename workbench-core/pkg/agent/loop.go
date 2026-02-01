package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/tinoosan/workbench-core/pkg/llm"
	"github.com/tinoosan/workbench-core/pkg/types"
	"github.com/tinoosan/workbench-core/pkg/validate"
)

// DefaultAgent is the minimalist streaming loop: stream the model response, execute its tool calls, and return the final text.
type DefaultAgent struct {
	LLM  llm.LLMClient
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
	ExtraTools       []llm.Tool
	ToolRegistry     *ToolRegistry
}

// Compile-time check: DefaultAgent implements Agent.
var _ Agent = (*DefaultAgent)(nil)

// RunConversation executes the agent loop for an existing conversation.
func (a *DefaultAgent) RunConversation(ctx context.Context, msgs []llm.LLMMessage) (final RunResult, updated []llm.LLMMessage, steps int, err error) {
	return a.runConversation(ctx, msgs, 1)
}

func (a *DefaultAgent) runConversation(ctx context.Context, msgs []llm.LLMMessage, startStep int) (final RunResult, updated []llm.LLMMessage, steps int, err error) {
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
		baseSystem = DefaultSystemPrompt()
	}

	msgs = append([]llm.LLMMessage(nil), msgs...)
	if startStep < 1 {
		startStep = 1
	}

	hostOpTools := []llm.Tool{FinalAnswerTool()}
	if a.ToolRegistry != nil {
		hostOpTools = append(hostOpTools, a.ToolRegistry.Definitions()...)
	}
	if len(a.ExtraTools) != 0 {
		hostOpTools = append(hostOpTools, a.ExtraTools...)
	}

	for step := startStep; ; step++ {

		system := baseSystem
			if a.PromptSource != nil {
				updatedSystem, err := a.PromptSource.SystemPrompt(ctx, baseSystem, step)
				if err != nil {
					return RunResult{}, nil, 0, err
				}
				system = updatedSystem
			}
		req := llm.LLMRequest{
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

		resp, err := a.streamToAccumulator(ctx, step, req)
		if err != nil {
			return RunResult{}, nil, 0, err
		}

		if a.Hooks.OnLLMUsage != nil && resp.Usage != nil {
			a.Hooks.OnLLMUsage(step, *resp.Usage)
		}
		if a.Hooks.OnWebSearch != nil && len(resp.Citations) != 0 {
			a.Hooks.OnWebSearch(step, resp.Citations)
		}

		if len(resp.ToolCalls) == 0 {
			finalText := strings.TrimSpace(resp.Text)
			msgs = append(msgs, llm.LLMMessage{Role: "assistant", Content: finalText})
			return RunResult{Text: finalText}, msgs, step, nil
		}

		assistantMsg := llm.LLMMessage{
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
				var args struct {
					Text      string   `json:"text"`
					Artifacts []string `json:"artifacts"`
				}
				dec := json.NewDecoder(strings.NewReader(tc.Function.Arguments))
				if err := dec.Decode(&args); err != nil {
					hostResp := types.HostOpResponse{Op: "final_answer", Ok: false, Error: "final_answer args were not valid JSON: " + err.Error()}
					hostRespJSON, _ := types.MarshalPretty(hostResp)
					msgs = append(msgs, llm.LLMMessage{Role: "tool", ToolCallID: strings.TrimSpace(tc.ID), Content: string(hostRespJSON)})
					continue
				}
				finalText := strings.TrimSpace(args.Text)
				if finalText == "" {
					hostResp := types.HostOpResponse{Op: "final_answer", Ok: false, Error: "final_answer.text is required"}
					hostRespJSON, _ := types.MarshalPretty(hostResp)
					msgs = append(msgs, llm.LLMMessage{Role: "tool", ToolCallID: strings.TrimSpace(tc.ID), Content: string(hostRespJSON)})
					continue
				}
				hostResp := types.HostOpResponse{Op: "final_answer", Ok: true}
				hostRespJSON, _ := types.MarshalPretty(hostResp)
				msgs = append(msgs, llm.LLMMessage{Role: "tool", ToolCallID: strings.TrimSpace(tc.ID), Content: string(hostRespJSON)})
				msgs = append(msgs, llm.LLMMessage{Role: "assistant", Content: finalText})
				return RunResult{Text: finalText, Artifacts: args.Artifacts}, msgs, step, nil
			}

			if a.ToolRegistry == nil {
				hostResp := types.HostOpResponse{Op: "tool_call", Ok: false, Error: "tool registry is not configured"}
				hostRespJSON, _ := types.MarshalPretty(hostResp)
				msgs = append(msgs, llm.LLMMessage{Role: "tool", ToolCallID: strings.TrimSpace(tc.ID), Content: string(hostRespJSON)})
				continue
			}
			op, err := a.ToolRegistry.Dispatch(ctx, which, []byte(tc.Function.Arguments))
			if err != nil {
				hostResp := types.HostOpResponse{Op: "tool_call", Ok: false, Error: "invalid tool call args: " + err.Error()}
				hostRespJSON, _ := types.MarshalPretty(hostResp)
				msgs = append(msgs, llm.LLMMessage{Role: "tool", ToolCallID: strings.TrimSpace(tc.ID), Content: string(hostRespJSON)})
				continue
			}
			pending = append(pending, pendingHostOp{req: op, callID: strings.TrimSpace(tc.ID)})
		}

		for _, item := range pending {
			hostResp := a.Exec.Exec(ctx, item.req)
			hostRespJSON, _ := types.MarshalPretty(hostResp)
			msgs = append(msgs, llm.LLMMessage{Role: "tool", ToolCallID: item.callID, Content: string(hostRespJSON)})
		}

	}
}

func (a *DefaultAgent) streamToAccumulator(ctx context.Context, step int, req llm.LLMRequest) (llm.LLMResponse, error) {
	s, ok := a.LLM.(llm.LLMClientStreaming)
	if !ok {
		return llm.LLMResponse{}, fmt.Errorf("LLM client does not support streaming")
	}
	dec := &finalTextStreamDecoder{}
	var streamMode string
	var streamPrefix strings.Builder
	const streamPrefixMax = 1024
	emit := func(token string) {
		if token == "" {
			return
		}
		if a.Hooks.OnToken != nil {
			a.Hooks.OnToken(step, token)
		}
	}
	resp, err := s.GenerateStream(ctx, req, func(chunk llm.LLMStreamChunk) error {
		if a.Hooks.OnStreamChunk != nil {
			a.Hooks.OnStreamChunk(step, chunk)
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
	return resp, err
}

func isDangerousHostOp(req types.HostOpRequest) bool {
	// Approvals are disabled in autonomous mode; no-op safeguard.
	_ = req
	return false
}

// Run executes the agent loop for a single user goal and returns the final result.
func (a *DefaultAgent) Run(ctx context.Context, goal string) (RunResult, error) {
	if err := validate.NonEmpty("goal", goal); err != nil {
		return RunResult{}, err
	}
	final, _, _, err := a.RunConversation(ctx, []llm.LLMMessage{
		{Role: "user", Content: goal},
	})
	return final, err
}

// DefaultSystemPrompt returns the built-in system instructions for the agent.
func DefaultSystemPrompt() string {
	raw := `<system>
  <identity>You are a capable AI assistant running in Workbench. You have access to a virtual filesystem and powerful tools to help users accomplish a wide range of tasks—from software engineering to analysis and automation.</identity>
  <general_assistance>
    <rule id="helpfulness">When asked general questions, answer directly and helpfully. Do not feel constrained to only perform file operations unless the task requires it.</rule>
    <rule id="skills_usage">You have access to specialized skills under the /skills directory. Always check /skills or read /skills/<skill_name>.md to see what capabilities you have (research, planning, etc.) before inventing your own approach.</rule>
    <rule id="curiosity">If a user request is open-ended, use your tools to explore and research before answering.</rule>
  </general_assistance>
  <critical_rules>
    <planning>
      <rule id="planning">
        COMPLEX TASKS REQUIRE A PLAN.
        1. INITIALIZATION: If the user request implies multiple steps, write high-level details to "/plan/HEAD.md" and a Markdown checklist to "/plan/CHECKLIST.md" before any fs_write/fs_shell/fun calls.
        2. FORMAT: The checklist must use "- [ ]" / "- [x]" tokens for each actionable step (e.g., "- [ ] Analyze requirements", "- [ ] Implement feature", "- [ ] Verify results").
        3. NARRATIVE: Keep narrative planning out of the checklist file. Use "/plan/HEAD.md" for reasoning and context; the checklist remains the single source of truth for tasks.
        4. GATE: Without a plan at "/plan/HEAD.md" AND a checklist at "/plan/CHECKLIST.md", do not execute side-effect tools (fs_write, shell_exec, etc.).
        5. EXECUTION: After each step completes, overwrite "/plan/CHECKLIST.md" with the updated checklist, marking done items with "- [x]".
        6. CONTINUOUS: Before starting a new step, re-read the checklist, ensure the next item is accurate, and update it if needed.
        7. ADAPTATION: If the plan evolves, immediately rewrite "/plan/HEAD.md" and "/plan/CHECKLIST.md" so details and tasks stay current.
        8. SKIP: Do NOT create a plan for greetings/smalltalk, single factual questions, or single small edits. Respond directly instead.
      </rule>
      <rule id="planning.externalize">Plans must live in "/plan/HEAD.md" (details) and "/plan/CHECKLIST.md" (checklist); don’t keep plan reasoning solely in your head.</rule>
      <rule id="planning.visibility">Whenever asked about planning, point to "/plan/HEAD.md" for details and "/plan/CHECKLIST.md" for the checklist—the mount is always available via fs_list.</rule>
    </planning>
    <rule id="tool_results">Tool results are YOUR output, not user input.</rule>
    <rule id="skills_vs_tools">Skills live under /skills (see /skills/<skill_name>.md) and are not tools; plugins belong to /tools.</rule>
    <rule id="skills_first">If the user mentions skill(s), or if you need to perform a general task (research, audit, etc.), ALWAYS check /skills before /tools.</rule>
    <rule id="skills_paths">Skills are only accessible via the /skills mount; do not assume /project contains a skills folder.</rule>
  </critical_rules>
  <capabilities>
    <direct_ops>
      <op name="fs_list">List VFS paths.</op>
      <op name="fs_read">Read file contents.</op>
      <op name="fs_search">Search a VFS mount using a semantic/indexed search (e.g. /memory).</op>
      <op name="fs_write">Write new files.</op>
      <op name="fs_append">Append to files.</op>
      <op name="fs_edit">Make precise edits via JSON diffs.</op>
      <op name="fs_patch">Apply unified-diff patches.</op>
      <op name="final_answer">Emit the final response once the user's goal is complete.</op>
      <op name="shell_exec">Run shell commands (pipes, redirects, etc.).</op>
      <op name="http_fetch">Make HTTP requests.</op>
      <op name="trace_events_latest">Read the latest trace events.</op>
      <op name="trace_events_since">Stream trace events since a cursor.</op>
      <op name="trace_events_summary">Summarize trace events.</op>
    </direct_ops>
    <skills>Refer to the <available_skills> block below and fs_read /skills/<skill>.md to follow documented workflows. THESE ARE YOUR PRIMARY GENERAL CAPABILITIES.</skills>
    <planning>For multi-step work, write details to /plan/HEAD.md and the checklist to /plan/CHECKLIST.md using fs_write. Keep the checklist current: re-read before each step, mark completed items with "- [x]", and add/adjust items as work changes. Skip planning for greetings/smalltalk, single factual questions, or single small edits.</planning>
    <external_tools>Use tool_run only after inspecting /tools/<toolId> manifests; prefer direct ops, skills, and /plan first.</external_tools>
  </capabilities>
  <vfs>
    <mount path="/project">User's actual project files.</mount>
    <mount path="/workspace">Run-scoped, writable workspace for artifacts and notes.</mount>
    <mount path="/inbox">Incoming task envelopes.</mount>
    <mount path="/outbox">Task results written by the agent.</mount>
    <mount path="/skills">These are YOUR skills. ALWAYS check /skills before /tools.</mount>
    <mount path="/plan">Planning workspace for complex tasks. /plan/HEAD.md is details; /plan/CHECKLIST.md is the checklist.</mount>
  </vfs>
  <skill_creation>
    You can create reusable skills when you notice repeatable patterns. Write a <skill_name>.md file using YAML front matter (name & description) followed by markdown instructions.
    1. Start a skill by writing the file:
       fs.write("/skills/my-skill.md", "---\nname: My Skill\ndescription: Brief summary\n---\n# Instructions\nDescribe when and how to run this skill.\n")
    2. Update or extend a skill with fs.append when needed.
    Skills appear in <available_skills> after creation and you can inspect /skills/skill-template.md for a starter layout.
  </skill_creation>
  <operating_rules>
    <rule id="stop">Call final_answer only once the overarching goal is complete; plain assistant text without tool calls is treated as final output when finished.</rule>
    <rule id="path_resolution">Shell commands should use relative paths (e.g. ./src) with the project root as cwd; fs_* tools still expect absolute VFS paths.</rule>
    <rule id="tool_usage">Use fs_* for file operations, shell_exec for shell commands, http_fetch for HTTP, and trace event helpers for diagnostics; do not invent other tools.</rule>
    <rule id="fs_edit">fs_edit expects JSON like {"path": "/project/file", "edits": [{"old": "...", "new": "...", "occurrence": 1}]}; if it fails, re-read the file and try a more specific snippet.</rule>
    <rule id="fs_patch">fs_patch needs a unified diff with hunk headers (e.g., @@ -1,3 +1,3 @@) or adjust until the patch applies cleanly.</rule>
    <memory_management>
      <structure>
        The /memory directory contains daily memory files in YYYY-MM-DD-memory.md format.
        - /memory/MEMORY.MD: Master instructions (read-only)
        - /memory/TODAY-memory.md: Today's file (writable)
        - /memory/PRIOR-DATE-memory.md: Previous days (read-only)
      </structure>

      <read_strategy>
        Prefer fs_search on /memory for recall instead of reading entire memory files.
        Use fs_read only when you need full context from a specific file.
      </read_strategy>

      <write_access>
        You can ONLY write to today's memory file (/memory/TODAY-memory.md).
        Use fs_write, fs_edit, or fs_append to update it when you learn something worth remembering.
      </write_access>

      <when_to_save>
        Save memories immediately when:
        1. User shares preferences, workflow patterns, or context
        2. User corrects you or provides domain knowledge
        3. You complete a significant task worth documenting
        4. You discover patterns in their codebase
        5. Conversation ending (user says bye/thanks/done)
      </when_to_save>

      <format>
        Write factual, concise entries with:
        - Timestamp (HH:MM)
        - Category: [preference|correction|decision|pattern|context]
        - Entry: Brief description with WHY, not just WHAT

        Do NOT write as if talking to a user. Write objective notes.
      </format>

      <startup>
        At conversation start:
        1. Read /memory/MEMORY.MD for guidelines
        2. Read today's memory file
        3. Acknowledge what you remember if relevant to the task
      </startup>
    </memory_management>
    <rule id="principles">Action-first, recover gracefully, assume defaults, prefer direct ops, and never hallucinate; always deliver a final response when work is complete.</rule>
    <rule id="skills_block">When asked about "<available_skills>", inspect the <available_skills> section injected below, and respond by describing each skill's name, description, and location instead of repeating the built-in host-capabilities list.</rule>
  </operating_rules>`
	return strings.TrimSpace(raw)
}
