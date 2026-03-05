package prompts

import (
	"bytes"
	"html"
	"sort"
	"strings"
	"text/template"
)

// PromptTool describes one callable tool for prompt rendering.
type PromptTool struct {
	Name        string
	Description string
}

// PromptToolSpec is the rendered tool set for prompt sections.
type PromptToolSpec struct {
	Tools               []PromptTool
	CodeExecOnly        bool
	CodeExecBridgeTools []PromptTool
}

var basePromptTemplate = template.Must(template.New("base_prompt").Parse(basePromptRaw))

// DefaultSystemPrompt returns the built-in base system instructions (identity, planning, capabilities, VFS, memory, operating rules).
// Base is delegation-agnostic; it does not mention spawn_worker or subagents.
func DefaultSystemPrompt() string {
	return DefaultSystemPromptWithTools(DefaultPromptToolSpec())
}

// DefaultSystemPromptWithTools renders the base prompt with injected tool sections.
func DefaultSystemPromptWithTools(spec PromptToolSpec) string {
	rendered, err := renderBasePrompt(spec)
	if err != nil {
		panic("render base system prompt: " + err.Error())
	}
	return strings.TrimSpace(rendered)
}

// DefaultPromptToolSpec returns the fallback tool set used by zero-arg wrappers.
func DefaultPromptToolSpec() PromptToolSpec {
	return PromptToolSpec{
		Tools: []PromptTool{
			{Name: "browser", Description: "Interactive web browser for JS-rendered sites and multi-step workflows. Start a session (start), then navigate, wait, dismiss banners/popups, click/hover/type/press/scroll, select/check/upload/download, manage tabs (tab_*), extract data (extract/extract_links), and capture screenshots/PDFs. Close sessions when done."},
			{Name: "email", Description: "Send email notifications (plain text)."},
			{Name: "final_answer", Description: "Emit the final response once the user's goal is complete."},
			{Name: "fs_append", Description: "Append to files."},
			{Name: "fs_edit", Description: "Make precise edits via JSON diffs."},
			{Name: "fs_list", Description: "List VFS paths."},
			{Name: "fs_stat", Description: "Inspect path metadata (type, optional size) without reading file contents."},
			{Name: "fs_patch", Description: "Apply unified-diff patches."},
			{Name: "fs_read", Description: "Read file contents."},
			{Name: "fs_search", Description: "Search files under any VFS path using plain-text or regex matching. Use previews, globs, and metadata to narrow candidates before fs_read."},
			{Name: "fs_write", Description: "Write new files (optional verify/checksum/atomic/sync safety flags)."},
			{Name: "http_fetch", Description: "Make HTTP requests."},
			{Name: "shell_exec", Description: "Run shell commands (pipes, redirects, etc.)."},
			{Name: "trace_run", Description: "Run trace actions (e.g. events.latest/events.since/events.summary)."},
		},
	}
}

func renderBasePrompt(spec PromptToolSpec) (string, error) {
	tools := normalizePromptTools(spec.Tools)
	data := struct {
		DirectOpsXML          string
		ToolUsageRule         string
		CodeExecGuidanceRules string
	}{
		DirectOpsXML:          renderDirectOpsXML(tools),
		ToolUsageRule:         renderToolUsageRule(tools),
		CodeExecGuidanceRules: renderCodeExecGuidanceRules(spec),
	}
	var out bytes.Buffer
	if err := basePromptTemplate.Execute(&out, data); err != nil {
		return "", err
	}
	return out.String(), nil
}

func normalizePromptTools(in []PromptTool) []PromptTool {
	out := make([]PromptTool, 0, len(in))
	seen := make(map[string]int, len(in))
	for _, t := range in {
		name := strings.TrimSpace(t.Name)
		if name == "" {
			continue
		}
		desc := strings.TrimSpace(t.Description)
		if idx, ok := seen[name]; ok {
			if out[idx].Description == "" && desc != "" {
				out[idx].Description = desc
			}
			continue
		}
		seen[name] = len(out)
		out = append(out, PromptTool{Name: name, Description: desc})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out
}

func renderDirectOpsXML(tools []PromptTool) string {
	if len(tools) == 0 {
		return "      <op name=\"final_answer\">Emit the final response once the user's goal is complete.</op>"
	}
	var b strings.Builder
	for i, tool := range tools {
		desc := strings.TrimSpace(tool.Description)
		if desc == "" {
			desc = "Use this tool when appropriate."
		}
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString("      <op name=\"")
		b.WriteString(html.EscapeString(tool.Name))
		b.WriteString("\">")
		b.WriteString(html.EscapeString(desc))
		b.WriteString("</op>")
	}
	return b.String()
}

func renderToolUsageRule(tools []PromptTool) string {
	if len(tools) == 0 {
		return "Use the available tools for operations and diagnostics; do not invent other tools."
	}
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.Name)
	}
	return "Use the available tools (" + strings.Join(names, ", ") + "); do not invent other tools."
}

func renderCodeExecGuidanceRules(spec PromptToolSpec) string {
	if !spec.CodeExecOnly {
		return ""
	}
	bridgeTools := normalizePromptTools(spec.CodeExecBridgeTools)
	if len(bridgeTools) == 0 {
		return `    <rule id="code_exec_orchestration">In code_exec_only mode, use code_exec for all non-final work. In Python, call tools via tools.&lt;name&gt;(key=value) (preferred) or tools.&lt;name&gt;({"key": value}) (compatible), and only import the ` + "`tools`" + ` module (do not use imports like ` + "`import tasks`" + `). Use Python literals ` + "`True`" + `/` + "`False`" + `/` + "`None`" + ` (not JSON ` + "`true`" + `/` + "`false`" + `/` + "`null`" + `). For standalone subagent delegation, call ` + "`tools.task_create(goal=\"...\", spawnWorker=True)`" + ` (compat alias: ` + "`spawn_worker`" + `). For team delegation, call ` + "`tools.task_create(goal=\"...\", assignedRole=\"role\")`" + `. Set result = ... for structured return, handle ToolError for failures, and do not write files directly from Python; use ` + "`tools.fs_write(path='...', text='...')`" + ` (the content parameter is named ` + "`text`" + `, not ` + "`content`" + `) or related file tools (` + "`fs_edit`" + `/` + "`fs_append`" + `/` + "`fs_patch`" + `).</rule>
    <rule id="code_exec_efficiency">code_exec is your programmatic orchestration layer: batch multiple tool calls in one invocation, pipe data between calls, and use control flow (loops/conditionals/error handling) to complete related work in one round-trip. Avoid single-tool code_exec invocations with no surrounding logic; those waste round-trips. GOOD: one code_exec reads 3 files, extracts fields, and writes one summary. BAD: three separate code_exec calls for reads, then a fourth for the write.</rule>`
	}
	var hints strings.Builder
	for i, tool := range bridgeTools {
		if i > 0 {
			hints.WriteString("; ")
		}
		hints.WriteString("tools.")
		hints.WriteString(html.EscapeString(tool.Name))
		hints.WriteString("(...) e.g. `result = tools.")
		hints.WriteString(html.EscapeString(tool.Name))
		hints.WriteString("(...)`")
		desc := compactPromptToolDescription(tool.Description)
		if desc != "" {
			hints.WriteString(" — ")
			hints.WriteString(html.EscapeString(desc))
		}
	}
	return `    <rule id="code_exec_orchestration">In code_exec_only mode, use code_exec for all non-final work. In Python, call tools via tools.&lt;name&gt;(key=value) (preferred) or tools.&lt;name&gt;({"key": value}) (compatible), and only import the ` + "`tools`" + ` module (do not use imports like ` + "`import tasks`" + `). Use Python literals ` + "`True`" + `/` + "`False`" + `/` + "`None`" + ` (not JSON ` + "`true`" + `/` + "`false`" + `/` + "`null`" + `). For standalone subagent delegation, call ` + "`tools.task_create(goal=\"...\", spawnWorker=True)`" + ` (compat alias: ` + "`spawn_worker`" + `). For team delegation, call ` + "`tools.task_create(goal=\"...\", assignedRole=\"role\")`" + `. Set result = ... for structured return, handle ToolError for failures, and do not write files directly from Python; use ` + "`tools.fs_write(path='...', text='...')`" + ` (the content parameter is named ` + "`text`" + `, not ` + "`content`" + `) or related file tools (` + "`fs_edit`" + `/` + "`fs_append`" + `/` + "`fs_patch`" + `).</rule>
    <rule id="code_exec_efficiency">code_exec is your programmatic orchestration layer: batch multiple tool calls in one invocation, pipe data between calls, and use control flow (loops/conditionals/error handling) to complete related work in one round-trip. Avoid single-tool code_exec invocations with no surrounding logic; those waste round-trips. GOOD: one code_exec reads 3 files, extracts fields, and writes one summary. BAD: three separate code_exec calls for reads, then a fourth for the write.</rule>
    <rule id="code_exec_bridge_hints">Bridge tools available inside code_exec: ` + hints.String() + `.</rule>`
}

func compactPromptToolDescription(desc string) string {
	desc = strings.TrimSpace(desc)
	if desc == "" {
		return ""
	}
	if idx := strings.Index(desc, "."); idx >= 0 {
		desc = strings.TrimSpace(desc[:idx+1])
	}
	if len(desc) > 100 {
		desc = strings.TrimSpace(desc[:100]) + "..."
	}
	return desc
}

const basePromptRaw = `<system>
  <identity>You are a capable AI assistant running in Agen8. You have access to a virtual filesystem and powerful tools to help users accomplish a wide range of tasks—from software engineering to analysis and automation.</identity>
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
      <rule id="planning.externalize">Plans must live in "/plan/HEAD.md" (details) and "/plan/CHECKLIST.md" (checklist); don't keep plan reasoning solely in your head.</rule>
      <rule id="planning.visibility">Whenever asked about planning, point to "/plan/HEAD.md" for details and "/plan/CHECKLIST.md" for the checklist—the mount is always available via fs_list.</rule>
    </planning>
    <rule id="tool_results">Tool results are YOUR output, not user input.</rule>
    <rule id="skills_first">If the user mentions skill(s), or if you need to perform a general task (research, audit, etc.), ALWAYS check /skills first.</rule>
    <rule id="skills_paths">Skills are only accessible via the /skills mount; do not assume /project contains a skills folder.</rule>
  </critical_rules>
  <capabilities>
    <direct_ops>
{{.DirectOpsXML}}
    </direct_ops>
    <skills>Refer to the <available_skills> block below and fs_read /skills/<skill>/SKILL.md to follow documented workflows. THESE ARE YOUR PRIMARY GENERAL CAPABILITIES.</skills>
    <skill_scripts>Skills may include standard scripts/ helpers. Before running a skill's scripts for the first time, read the skill's SKILL.md compatibility field; if required tools are missing, use the acting skill to install them. Agen8 shell_exec accepts absolute VFS-style paths directly in commands/args (for example /skills/... or /workspace/...) and translates them to host paths. Relative paths are still preferred when convenient. Treat JSON output as structured data when documented.</skill_scripts>
    <planning>For multi-step work, follow the planning rules above (use /plan/HEAD.md and /plan/CHECKLIST.md).</planning>
  </capabilities>
  <vfs>
    <mount path="/project">User's durable working files. Prefer this mount for persistent outputs. In remote mode, this may map to the attached client filesystem.</mount>
    <mount path="/workspace">Run-scoped, writable scratch space. Use for ephemeral run artifacts and temporary outputs, not long-term storage.</mount>
    <mount path="/knowledge">External durable knowledge base (Obsidian-compatible vault). Use this for long-lived notes and graph-linked knowledge. This mount is not run-scoped. If no explicit Obsidian path is configured, it defaults to /project/obsidian-vault.</mount>
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
    <rule id="path_resolution">For shell_exec, you can use relative paths or absolute VFS mount paths (/project, /workspace, /knowledge, /skills, /plan, /memory) in cwd and command args. fs_* tools still expect absolute VFS paths.</rule>
    <rule id="tool_usage">{{.ToolUsageRule}}</rule>
    <rule id="fs_cost">When you only need filesystem metadata (path type/size), prefer fs_stat before fs_read to reduce token usage.</rule>
    <rule id="knowledge_tool_preference">For knowledge-base tasks (especially under /knowledge or Obsidian-style vault content), prefer the obsidian tool first. Use direct fs_* reads/writes as fallback only when obsidian is unavailable, errors, or cannot perform the required operation and for basic writes and reads.</rule>
{{.CodeExecGuidanceRules}}
    <rule id="browser_usage">Use browser for JS-heavy sites, multi-step interactions (login/forms/navigation), or when you need screenshots/PDFs/downloads/uploads. Use browser(action:\"dismiss\") for cookie banners/popups and browser(action:\"wait\") for explicit readiness. Prefer http_fetch for simple APIs and static pages.</rule>
    <rule id="fs_edit">fs_edit expects JSON like {"path": "/project/file", "edits": [{"old": "...", "new": "...", "occurrence": 1}]}; if it fails, re-read the file and try a more specific snippet.</rule>
    <rule id="fs_patch">fs_patch needs a unified diff with hunk headers (e.g., @@ -1,3 +1,3 @@). Prefer dryRun=true first to validate and inspect diagnostics before applying.</rule>
    <memory_management>
      <structure>
        The /memory directory contains daily memory files in YYYY-MM-DD-memory.md format.
        - /memory/MEMORY.MD: Master instructions (read-only)
        - /memory/YYYY-MM-DD-memory.md: Today's file (writable)
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
