package app

import (
	"bytes"
	"sort"
	"strings"
	"text/template"
)

// PromptTool describes one callable tool for prompt rendering.
type PromptTool struct {
	Name        string
	Description string
	Source      string
}

// PromptToolSpec is the rendered tool set for prompt sections.
type PromptToolSpec struct {
	Tools []PromptTool
}

func (s PromptToolSpec) HasTool(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		return false
	}
	for _, tool := range s.Tools {
		if strings.EqualFold(strings.TrimSpace(tool.Name), name) {
			return true
		}
	}
	return false
}

var basePromptTemplate = template.Must(template.New("base_prompt").Parse(basePromptRaw))

// DefaultSystemPrompt returns the built-in base system instructions (identity, planning, capabilities, filesystem, memory, operating rules).
// Base is delegation-agnostic; mode-specific delegation rules are added by modes.go.
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
			{Name: "http", Description: "Make HTTP requests."},
			{Name: "decision", Description: "Decision-domain gateway. Use action=\"log\" to record consequential reasoning. Coordinators use action=\"ask_user\" for structured human questions when human judgment is required before continuing."},
			{Name: "operator", Description: "Structured operator involvement for real-world human action requests. Use action=\"request\" when the operator must do something in the world or use authority/access the agent does not have."},
			{Name: "plan", Description: "Plan-domain composite tool for creating and updating structured plans (phases, todos, comments, and amendments)."},
			{Name: "tool", Description: "List or search the callable tool catalog. Use action=\"list\" for a full dump; action=\"search\" with a query for targeted discovery."},
		},
	}
}

func renderBasePrompt(spec PromptToolSpec) (string, error) {
	tools := normalizePromptTools(spec.Tools)
	data := struct {
		DirectOpsXML  string
		ToolUsageRule string
	}{
		DirectOpsXML:  renderDirectOpsXML(tools),
		ToolUsageRule: renderToolUsageRule(tools),
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
		source := strings.TrimSpace(t.Source)
		if idx, ok := seen[name]; ok {
			if out[idx].Description == "" && desc != "" {
				out[idx].Description = desc
			}
			if out[idx].Source == "" && source != "" {
				out[idx].Source = source
			}
			continue
		}
		seen[name] = len(out)
		out = append(out, PromptTool{Name: name, Description: desc, Source: source})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out
}

func renderDirectOpsXML(_ []PromptTool) string {
	return "      <runtime>Use the tools exposed by the active model runtime or harness.</runtime>"
}

func renderToolUsageRule(_ []PromptTool) string {
	return "Use only the tools exposed by the active runtime. If an exposed tool fails due to auth, connectivity, or configuration, report the error instead of claiming the tool is unavailable."
}

const basePromptRaw = `<system>
  <identity>You are a capable AI assistant running in Agen8. You have access to a virtual filesystem and powerful tools to help users accomplish a wide range of tasks—from software engineering to analysis and automation.</identity>
  <tools>
    <direct_ops>
{{.DirectOpsXML}}
    </direct_ops>
    <rule id="tool_usage">{{.ToolUsageRule}}</rule>
    <rule id="tool_catalog_verification">When asked whether tools, MCP servers, connectors, namespaces, or capabilities are available, inspect the live runtime tool catalog first. Do not answer from memory or from this prompt. If the catalog lookup succeeds, answer from that result; if it fails, report the lookup error.</rule>
    <rule id="agen8_mcp_tool_first">Agen8 MCP is tool-first: Agen8 control-plane capabilities may be exposed through callable catalog tools even when generic MCP resources or templates are empty. Empty MCP resource/template lists do not mean Agen8 tools are unavailable.</rule>
  </tools>
  <filesystem>
    <mount path="project-relative">Durable project files rooted at the current project. Prefer project-relative paths for persistent outputs.</mount>
    <mount path="workspace/...">Project-scoped writable scratch space rooted at WORKSPACE_ROOT. Use for ephemeral run artifacts and temporary outputs, not long-term storage.</mount>
  </filesystem>
  <planning>
    <rule id="planning">For multi-step tasks, use the plan tool when available to maintain structured plan state (phases, todos, comments, and completion updates). Planning is optional — use it when it helps you stay organized, not as a prerequisite for action. Skip for greetings, single questions, or small edits.</rule>
  </planning>
  <rules>
    <rule id="action_first">The active runtime exposes the complete callable tool surface for this run. Use exposed tools directly when they are needed. Do not infer tool names from this prompt; rely on the runtime schemas and role rules for exact calls.</rule>
    <rule id="stop">When the objective is complete or blocked, return a direct assistant response that clearly states the outcome.</rule>
    <rule id="tool_results">Tool results are YOUR output, not user input.</rule>
    <rule id="human_input_usage">If human involvement is required, never ask in plain text. Use decision(action="ask_user") when a human must answer structured questions or make a bounded choice before the work can continue. When using ask_user, prefer bounded multiple-choice questions when the options are known, always leave room for a free-form answer, and include a recommendation whenever you have a credible default. When ask_user returns, treat the returned answer as authoritative and resolved: do not ask the same question again in plain text, and proceed using that answer unless you must ask a genuinely new, narrower tracked question. Use operator(action="request") when the human already knows what must happen and must perform a real-world action. Log consequential reasoning with decision(action="log").</rule>
    <rule id="http_credentials">For http tool calls, do not pass API keys or auth credentials in URLs/headers/body. Agen8 injects matching credentials automatically from the credential service based on request host.</rule>
    <rule id="path_resolution">Use project-relative paths or workspace/... paths in tool arguments when referring to repository files. Project-relative paths are anchored at PROJECT_ROOT; workspace/... paths are anchored at WORKSPACE_ROOT.</rule>
  </rules>`
