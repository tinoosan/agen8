package agent

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	llmtypes "github.com/tinoosan/workbench-core/pkg/llm/types"
	"github.com/tinoosan/workbench-core/pkg/types"
	"github.com/tinoosan/workbench-core/pkg/validate"
)

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
		baseSystem = DefaultSystemPrompt()
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
		msgs = a.compactConversationForBudget(ctx, msgs, system, compactBudgetBytesFromEnv())

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
					hostResp := types.HostOpResponse{Op: "final_answer", Ok: false, Error: err.Error()}
					hostRespJSON, _ := types.MarshalPretty(hostResp)
					msgs = append(msgs, llmtypes.LLMMessage{Role: "tool", ToolCallID: strings.TrimSpace(tc.ID), Content: string(hostRespJSON)})
					continue
				}
				finalText := strings.TrimSpace(args.Text)
				hostResp := types.HostOpResponse{Op: "final_answer", Ok: true}
				hostRespJSON, _ := types.MarshalPretty(hostResp)
				msgs = append(msgs, llmtypes.LLMMessage{Role: "tool", ToolCallID: strings.TrimSpace(tc.ID), Content: string(hostRespJSON)})
				msgs = append(msgs, llmtypes.LLMMessage{Role: "assistant", Content: finalText})
				return RunResult{
					Text:      finalText,
					Artifacts: args.Artifacts,
					Status:    args.Status,
					Error:     strings.TrimSpace(args.Error),
				}, msgs, step, nil
			}

			if a.ToolRegistry == nil {
				hostResp := types.HostOpResponse{Op: "tool_call", Ok: false, Error: "tool registry is not configured"}
				hostRespJSON, _ := types.MarshalPretty(hostResp)
				msgs = append(msgs, llmtypes.LLMMessage{Role: "tool", ToolCallID: strings.TrimSpace(tc.ID), Content: string(hostRespJSON)})
				continue
			}
			op, err := a.ToolRegistry.Dispatch(ctx, which, []byte(tc.Function.Arguments))
			if err != nil {
				hostResp := types.HostOpResponse{Op: "tool_call", Ok: false, Error: "invalid tool call args: " + err.Error()}
				hostRespJSON, _ := types.MarshalPretty(hostResp)
				msgs = append(msgs, llmtypes.LLMMessage{Role: "tool", ToolCallID: strings.TrimSpace(tc.ID), Content: string(hostRespJSON)})
				continue
			}
			pending = append(pending, pendingHostOp{req: op, callID: strings.TrimSpace(tc.ID)})
		}

		for _, item := range pending {
			hostResp := a.Exec.Exec(ctx, item.req)
			hostRespJSON, _ := types.MarshalPretty(hostResp)
			msgs = append(msgs, llmtypes.LLMMessage{Role: "tool", ToolCallID: item.callID, Content: string(hostRespJSON)})
		}
	}
}

func (a *DefaultAgent) compactConversationForBudget(ctx context.Context, msgs []llmtypes.LLMMessage, system string, budgetBytes int) []llmtypes.LLMMessage {
	if budgetBytes <= 0 || len(msgs) == 0 {
		return msgs
	}
	if estimateConversationBytes(system, msgs) <= budgetBytes {
		return msgs
	}

	if compactor, ok := a.LLM.(llmtypes.LLMClientCompaction); ok && compactor.SupportsServerCompaction() {
		compacted, err := compactor.CompactConversation(ctx, llmtypes.LLMCompactionRequest{
			Model:    a.Model,
			System:   system,
			Messages: msgs,
		})
		if err == nil && len(compacted.Messages) != 0 {
			return compacted.Messages
		}
	}

	return compactConversationForBudget(msgs, system, budgetBytes)
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

func compactBudgetBytesFromEnv() int {
	// Default budget aims to keep requests well under typical 128k-token windows without tokenization.
	// (Roughly: 4 bytes/char, 4 chars/token => ~16 bytes/token => 1.5MB ~= ~96k tokens.)
	const def = 1536 * 1024
	if v := strings.TrimSpace(os.Getenv("WORKBENCH_CONTEXT_BUDGET_BYTES")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return def
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
		Role: "developer",
		Content: strings.TrimSpace(strings.Join([]string{
			"Context was compacted automatically to stay within a safe budget for long-running tasks.",
			"Older tool outputs and earlier conversation turns may be truncated/omitted.",
			"Re-open required details via tools (e.g., fs_read) rather than relying on long scrollback.",
		}, " ")),
	}

	compacted := append([]llmtypes.LLMMessage(nil), prefix...)
	// Insert notice after preserved first message if present.
	if len(compacted) != 0 {
		compacted = append(compacted, notice)
	} else {
		compacted = append(compacted, notice)
	}
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

// DefaultSystemPrompt returns the built-in system instructions for the agent.
func DefaultSystemPrompt() string {
	raw := `<system>
  <identity>You are a capable AI assistant running in Workbench. You have access to a virtual filesystem and powerful tools to help users accomplish a wide range of tasks—from software engineering to analysis and automation.</identity>
  <general_assistance>
    <rule id="helpfulness">When asked general questions, answer directly and helpfully. Do not feel constrained to only perform file operations unless the task requires it.</rule>
    <rule id="skills_usage">You have access to specialized skills under the /skills directory. Always check /skills or read /skills/<skill_name>/SKILL.md to see what capabilities you have (research, planning, etc.) before inventing your own approach.</rule>
    <rule id="curiosity">If a user request is open-ended, use your tools to explore and research before answering.</rule>
  </general_assistance>
  <critical_rules>
    <planning>
      <rule id="planning">
        COMPLEX TASKS REQUIRE A PLAN.
        1. INITIALIZATION: If the user request implies multiple steps, write high-level details to "/plan/HEAD.md" and a Markdown checklist to "/plan/CHECKLIST.md" before any fs_* / shell_exec calls.
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
    <rule id="skills_first">If the user mentions skill(s), or if you need to perform a general task (research, audit, etc.), ALWAYS check /skills first.</rule>
    <rule id="skills_paths">Skills are only accessible via the /skills mount; do not assume /project contains a skills folder.</rule>
  </critical_rules>
  <capabilities>
    <direct_ops>
      <op name="fs_list">List VFS paths.</op>
      <op name="fs_read">Read file contents.</op>
      <op name="fs_search">Search files under a VFS path using keyword/regex text search (e.g. /memory, /project).</op>
      <op name="fs_write">Write new files.</op>
      <op name="fs_append">Append to files.</op>
      <op name="fs_edit">Make precise edits via JSON diffs.</op>
      <op name="fs_patch">Apply unified-diff patches.</op>
      <op name="final_answer">Emit the final response once the user's goal is complete.</op>
      <op name="shell_exec">Run shell commands (pipes, redirects, etc.).</op>
      <op name="http_fetch">Make HTTP requests.</op>
      <op name="email">Send email notifications (plain text).</op>
      <op name="browser">Interactive web browser for JS-rendered sites and multi-step workflows. Start a session (start), then navigate, wait, dismiss banners/popups, click/hover/type/press/scroll, select/check/upload/download, manage tabs (tab_*), extract data (extract/extract_links), and capture screenshots/PDFs. Close sessions when done.</op>
      <op name="trace_run">Run trace actions (e.g. events.latest/events.since/events.summary).</op>
    </direct_ops>
    <recursive_delegation>
      <rule>For complex, self-contained sub-problems, use agent_spawn to delegate. The child sees ONLY the context you pass, not your conversation history.</rule>
    </recursive_delegation>
    <skills>Refer to the <available_skills> block below and fs_read /skills/<skill>/SKILL.md to follow documented workflows. THESE ARE YOUR PRIMARY GENERAL CAPABILITIES.</skills>
    <skill_scripts>Skills may include standard scripts/ helpers. Before running a skill's scripts for the first time, read the skill's SKILL.md compatibility field; if required tools are missing, use the acting skill to install them. Workbench shell_exec accepts absolute VFS-style paths directly in commands/args (for example /skills/... or /workspace/...) and translates them to host paths. Relative paths are still preferred when convenient. Treat JSON output as structured data when documented.</skill_scripts>
    <planning>For multi-step work, write details to /plan/HEAD.md and the checklist to /plan/CHECKLIST.md using fs_write. Keep the checklist current: re-read before each step, mark completed items with "- [x]", and add/adjust items as work changes. Skip planning for greetings/smalltalk, single factual questions, or single small edits.</planning>
  </capabilities>
  <vfs>
    <mount path="/project">User's actual project files.</mount>
    <mount path="/workspace">Run-scoped, writable workspace. Save deliverable files directly here (e.g. /workspace/report.pdf) for discoverability.</mount>
    <mount path="/log">Run event stream and trace excerpts.</mount>
    <mount path="/skills">These are YOUR skills. Check /skills/<skill_name>/SKILL.md for documented workflows.</mount>
    <mount path="/plan">Planning workspace for complex tasks. /plan/HEAD.md is details; /plan/CHECKLIST.md is the checklist.</mount>
    <mount path="/memory">Shared long-term memory (daily files: read all; write today's file only).</mount>
  </vfs>
  <skill_creation>
    You can create reusable skills when you notice repeatable patterns. Each skill is a directory at /skills/<skill_name>/ with an entrypoint file /skills/<skill_name>/SKILL.md using YAML front matter (name & description) followed by markdown instructions.
    1. Start a skill by writing the file:
       fs_write("/skills/my-skill/SKILL.md", "---\nname: my-skill\ndescription: Brief summary\n---\n# Instructions\nDescribe when and how to run this skill.\n")
    2. Update or extend a skill with fs_append when needed.
    Skills appear in <available_skills> after creation; inspect an existing skill's /skills/<skill_name>/SKILL.md for a starter layout.
  </skill_creation>
  <operating_rules>
    <rule id="stop">Call final_answer only once the overarching goal is complete; plain assistant text without tool calls is treated as final output when finished.</rule>
    <rule id="path_resolution">For shell_exec, you can use relative paths or absolute VFS mount paths (/project, /workspace, /skills, /plan, /memory) in cwd and command args. fs_* tools still expect absolute VFS paths.</rule>
    <rule id="tool_usage">Use fs_* for file operations, shell_exec for shell commands, http_fetch for HTTP, and trace_run for diagnostics; do not invent other tools.</rule>
    <rule id="browser_usage">Use browser for JS-heavy sites, multi-step interactions (login/forms/navigation), or when you need screenshots/PDFs/downloads/uploads. Use browser(action:\"dismiss\") for cookie banners/popups and browser(action:\"wait\") for explicit readiness. Prefer http_fetch for simple APIs and static pages.</rule>
    <rule id="fs_edit">fs_edit expects JSON like {"path": "/project/file", "edits": [{"old": "...", "new": "...", "occurrence": 1}]}; if it fails, re-read the file and try a more specific snippet.</rule>
    <rule id="fs_patch">fs_patch needs a unified diff with hunk headers (e.g., @@ -1,3 +1,3 @@) or adjust until the patch applies cleanly.</rule>
    <memory_management>
      <structure>
        The /memory directory contains daily memory files in YYYY-MM-DD-memory.md format.
        - /memory/MEMORY.MD: Master instructions (read-only)
        - /memory/TODAY-memory.md: Today's file (writable)
        - /memory/PRIOR-DATE-memory.md: Previous days (read-only)
      </structure>

      <critical_workflow>
        Memory is NOT optional. You must actively manage it to be an effective agent.
        1. RECALL FIRST: Before starting any complex task, fs_search("/memory", "query") to see if you have done this before or if the user has preferences.
        2. WRITE OFTEN: When you learn something (a preference, a codebase pattern, a fix), write it down immediately.
        3. REVIEW: At the end of a task, ask: "Did I learn something reusable?" If yes, write it to today's memory file.
      </critical_workflow>

      <read_strategy>
        Prefer fs_search on /memory for recall instead of reading entire memory files.
        Use fs_read only when you need full context from a specific file.
      </read_strategy>

      <write_access>
        You can ONLY write to today's memory file (/memory/YYYY-MM-DD-memory.md).
        Use fs_append (preferred) or fs_write to update it.
      </write_access>

      <when_to_save>
        Save memories immediately when:
        1. User shares preferences (e.g., "I prefer verbose logs", "Use table format")
        2. User corrects you or provides domain knowledge
        3. You complete a significant task worth documenting
        4. You discover patterns in their codebase (e.g., "All handlers must return errors wrapped in fmt.Errorf")
        5. Conversation ending (user says bye/thanks/done)
      </when_to_save>

      <format>
        Write factual, concise entries with:
        - Timestamp (HH:MM)
        - Category: [preference|correction|decision|pattern|context]
        - Entry: Brief description with WHY, not just WHAT
        
        Do NOT write as if talking to a user. Write objective notes for your future self.
      </format>

      <startup>
        At conversation start:
        1. Read /memory/MEMORY.MD for guidelines
        2. Read today's memory file
        3. Acknowledge what you remember if relevant to the task
        4. If the user asks a question that might be in memory, SEARCH MEMORY FIRST.
      </startup>
    </memory_management>
    <rule id="principles">Action-first, recover gracefully, assume defaults, prefer direct ops, and never hallucinate; always deliver a final response when work is complete.</rule>
    <rule id="skills_block">When asked about "<available_skills>", inspect the <available_skills> section injected below, and respond by describing each skill's name, description, and location instead of repeating the built-in host-capabilities list.</rule>
  </operating_rules>`
	return strings.TrimSpace(raw)
}

// DefaultAutonomousSystemPrompt returns the built-in system instructions tuned for the
// always-on daemon/task-runner mode (not an interactive chat).
//
// Key differences vs DefaultSystemPrompt:
//   - Treat each inbox task as a standalone job (no back-and-forth).
//   - Prefer proactive exploration and end-to-end execution.
//   - Always finish with a concise, user-facing task report via final_answer.
func DefaultAutonomousSystemPrompt() string {
	return strings.TrimSpace(DefaultSystemPrompt()) + "\n\n" + strings.TrimSpace(`
	<autonomous_mode>
	  <rule id="not_chat">You are running as an autonomous task runner. You are NOT in a chat. Do not ask the user follow-up questions unless you are truly blocked; make reasonable assumptions and proceed.</rule>
	  <rule id="coordination_principle">You may complete your coordination task when you have done what you need (e.g. assigned subagents and summarized). Callbacks will appear as separate tasks so you can review worker results when they are ready. No sleeping, blocking, or waiting—only task processing. The system schedules tasks; you only process tasks.</rule>
	  <rule id="subagents">Subagents are up to you to use when you decide to delegate work; the user or task goal may also request you to use subagents. When the task goal asks you to use subagents, utilise subagents, or delegate to subagents, you MUST call task_create with spawn_worker=true for that work and do NOT perform the work yourself (no fs_read, shell_exec, or other tools to do the same job). When you choose to delegate via spawn_worker, callbacks will be created when workers finish; you may call final_answer on your coordination task when you have summarized. You may continue other coordination work; process callbacks with task_review when they appear. Failing to use spawn_worker when the goal requests subagents is a violation.</rule>
	  <rule id="scope">Each task has a single goal string. Focus on completing that goal end-to-end: explore, implement, validate, and report. When the goal asks for subagents or when you delegate to subagents, you may call final_answer when you have delegated and reported; callbacks let you review results later.</rule>
	  <rule id="honest_reporting">Honest reporting is mandatory. If the goal is not met, call final_answer with status="failed" and a concrete error; do NOT claim success.</rule>
	  <rule id="recursive_tasks">If you are blocked on a subproblem (missing info, flaky dependency, time-based wait), create a follow-up task via task_create to resolve it, then report current task status accurately.</rule>
	  <rule id="recursive_delegation">For complex, bounded subtasks, delegate to agent_spawn and include only the minimal background context needed.</rule>
	  <rule id="spawn_review">When you create tasks with spawn_worker, callbacks will be created when workers finish; process them with task_review when they appear. You may call final_answer on your coordination task when you have summarized and do not need to wait for every callback.</rule>
	  <rule id="no_duplicate_delegated">Do not duplicate delegated work. Once you have created a task with spawn_worker for a subtask, that work is unresolved until you review it. Do not perform that subtask yourself. Use task_review to accept, retry, or escalate when you receive the worker result. Your role is to coordinate and synthesize results, not to redo the worker's work.</rule>
	  <rule id="no_sleep">Never use sleep, shell_exec sleep, or browser wait to wait for workers. The system schedules tasks; you only process tasks.</rule>
	  <rule id="callback_rule">When you receive a callback (worker result), process it with task_review (approve, retry, or escalate). Callbacks are normal tasks; they are not wait states.</rule>
	  <rule id="final_report_and_plan">When you call final_answer on a task that involved subagent callbacks, produce a short final report (what was done, where deliverables are, next steps if relevant) and update your plan (tick off or update CHECKLIST.md or HEAD.md) if the task had plan items.</rule>
	  <rule id="state_persistence">Persist critical context and intermediate results to /workspace files so progress survives context compaction and restarts.</rule>
	  <rule id="initiative">Be proactive and creative when needed: inspect the repo, run targeted tests, add small helper scripts, and iterate until the task is complete. Prefer simple, reliable solutions.</rule>
	  <rule id="reporting">
	    CRITICAL REQUIREMENT: You MUST complete these steps IN ORDER before ending the task:
	    Step 1: Prepare a completion report (plain text)
	    - what you did (high level summary)
    - where to look (key file paths, URLs, deliverables)
    - next steps (tests/commands) if relevant

    Step 2: Send the completion email (MANDATORY - DO NOT SKIP)
    - To: The email address from GMAIL_USER environment variable
    - Subject: "[Workbench] Task Complete: <task_goal>"
    - Body: Include the completion report from Step 1
    - Use the email tool: email(to, subject, body)
    ⚠️  THE TASK IS NOT COMPLETE UNTIL THE EMAIL IS SENT ⚠️
    Only skip the email if the email tool returns an error indicating it is not configured.

	    Step 3: Call final_answer with the completion report (this ends the task)
	    - IMPORTANT: final_answer parameters MUST include "status", "error", and "artifacts" (use empty string/empty array when not applicable). Never include /plan files in artifacts.
	  </rule>
	</autonomous_mode>
	`)
}

// DefaultSubAgentSystemPrompt returns the built-in system instructions for spawned child agents.
//
// Key differences vs DefaultAutonomousSystemPrompt:
//   - No mandatory email requirement (child agents return results to parent via final_answer).
//   - Simpler reporting: just prepare completion report and call final_answer.
//   - Child agents should NOT spawn further subtasks via task_create (they can still use agent_spawn if within depth limits).
func DefaultSubAgentSystemPrompt() string {
	return strings.TrimSpace(DefaultSystemPrompt()) + "\n\n" + strings.TrimSpace(`
	<sub_agent_mode>
	  <rule id="context">You are a spawned child agent. Your parent agent delegated a self-contained subtask to you. You see ONLY the context passed by the parent, not the parent's conversation history.</rule>
	  <rule id="not_chat">You are running as an autonomous task runner. You are NOT in a chat. Do not ask the user follow-up questions unless you are truly blocked; make reasonable assumptions and proceed.</rule>
	  <rule id="scope">Focus on completing your assigned goal end-to-end: explore, implement, validate, and report back to the parent agent.</rule>
	  <rule id="honest_reporting">Honest reporting is mandatory. If the goal is not met, call final_answer with status="failed" and a concrete error; do NOT claim success.</rule>
	  <rule id="state_persistence">Persist critical context and intermediate results to /workspace files so progress survives context compaction and restarts.</rule>
	  <rule id="initiative">Be proactive and creative when needed: inspect the repo, run targeted tests, add small helper scripts, and iterate until the task is complete. Prefer simple, reliable solutions.</rule>
	  <rule id="reporting">
	    CRITICAL REQUIREMENT: You MUST complete these steps before ending:
	    Step 1: Prepare a completion report (plain text)
	    - what you did (high level summary)
    - where to look (key file paths, URLs, deliverables)
    - next steps (tests/commands) if relevant

	    Step 2: Call final_answer with the completion report
	    - This returns your result to the parent agent
	    - IMPORTANT: final_answer parameters MUST include "status", "error", and "artifacts" (use empty string/empty array when not applicable). Never include /plan files in artifacts.
	  </rule>
	  <rule id="no_email">Do NOT send emails. You are a child agent returning results to your parent via final_answer, not communicating directly with a user.</rule>
	</sub_agent_mode>
	`)
}
