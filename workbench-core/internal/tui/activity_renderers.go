package tui

import "strings"

type ActivityRenderer interface {
	RenderDetail(a Activity, expanded bool, telemetry bool, b *strings.Builder)
	RenderArguments(a Activity, telemetry bool, b *strings.Builder)
}

var defaultActivityRenderer ActivityRenderer = baseRenderer{}

var activityRenderers = map[string]ActivityRenderer{
	"agent_spawn":     agentSpawnRenderer{},
	"browser":         browserRenderer{},
	"code_exec":       codeExecRenderer{},
	"email":           emailRenderer{},
	"fs_append":       fsWriteAppendRenderer{},
	"fs_search":       fsSearchRenderer{},
	"fs_write":        fsWriteAppendRenderer{},
	"http_fetch":      httpFetchRenderer{},
	"llm.web.search":  llmWebSearchRenderer{},
	"shell_exec":      shellExecRenderer{},
	"task_create":     taskCreateRenderer{},
	"trace_run":       traceRunRenderer{},
	"workdir.changed": workdirChangedRenderer{},
}

func activityRendererFor(kind string) ActivityRenderer {
	kind = strings.TrimSpace(kind)
	if kind == "" {
		return defaultActivityRenderer
	}
	if r, ok := activityRenderers[kind]; ok {
		return r
	}
	return defaultActivityRenderer
}

type baseRenderer struct{}

func (baseRenderer) RenderDetail(a Activity, expanded bool, telemetry bool, b *strings.Builder) {
	renderCommonOutputPreview(a, expanded, b)
	renderTelemetryBlock(a, telemetry, false, false, b)
}

func (baseRenderer) RenderArguments(a Activity, telemetry bool, b *strings.Builder) {
	renderDefaultArgumentsPrefix(a, telemetry, b)
}

type shellExecRenderer struct{ baseRenderer }

func (shellExecRenderer) RenderDetail(a Activity, expanded bool, telemetry bool, b *strings.Builder) {
	if !renderCommonOutputPreview(a, expanded, b) {
		exitCode := strings.TrimSpace(a.Data["exitCode"])
		stdout := strings.TrimSpace(a.Data["stdout"])
		stderr := strings.TrimSpace(a.Data["stderr"])
		if exitCode != "" {
			b.WriteString("- exitCode: `")
			b.WriteString(exitCode)
			b.WriteString("`\n")
		}
		if stdout != "" {
			b.WriteString("\n**stdout**\n\n")
			b.WriteString(FormatCode("text", stdout))
			b.WriteString("\n")
		}
		if stderr != "" {
			b.WriteString("\n**stderr**\n\n")
			b.WriteString(FormatCode("text", stderr))
			b.WriteString("\n")
		}
	}
	renderTelemetryBlock(a, telemetry, false, false, b)
}

func (shellExecRenderer) RenderArguments(a Activity, telemetry bool, b *strings.Builder) {
	renderDefaultArgumentsPrefix(a, telemetry, b)
	if v := strings.TrimSpace(a.Data["argvPreview"]); v != "" {
		b.WriteString("- command:\n\n")
		b.WriteString(FormatCode("bash", v))
		b.WriteString("\n")
	}
	if v := strings.TrimSpace(a.Data["cwd"]); v != "" {
		b.WriteString("- cwd: `")
		b.WriteString(v)
		b.WriteString("`\n")
	}
}

type codeExecRenderer struct{ baseRenderer }

func (codeExecRenderer) RenderDetail(a Activity, expanded bool, telemetry bool, b *strings.Builder) {
	if v := strings.TrimSpace(a.Data["result"]); v != "" {
		b.WriteString("\n**result**\n\n")
		b.WriteString(FormatCode("json", v))
		b.WriteString("\n")
		if strings.TrimSpace(a.Data["resultPreviewTruncated"]) == "true" {
			b.WriteString("_result preview truncated_\n")
		}
	}
	if v := strings.TrimSpace(a.Data["stdout"]); v != "" {
		b.WriteString("\n**stdout**\n\n")
		b.WriteString(FormatCode("text", v))
		b.WriteString("\n")
	}
	if v := strings.TrimSpace(a.Data["stderr"]); v != "" {
		b.WriteString("\n**stderr**\n\n")
		b.WriteString(FormatCode("text", v))
		b.WriteString("\n")
	}
	if v := strings.TrimSpace(a.Data["toolCallCount"]); v != "" {
		b.WriteString("- toolCallCount: `")
		b.WriteString(v)
		b.WriteString("`\n")
	}
	if v := strings.TrimSpace(a.Data["runtimeMs"]); v != "" {
		b.WriteString("- runtimeMs: `")
		b.WriteString(v)
		b.WriteString("`\n")
	}
	renderCommonOutputPreview(a, expanded, b)
	renderTelemetryBlock(a, telemetry, false, false, b)
}

func (codeExecRenderer) RenderArguments(a Activity, telemetry bool, b *strings.Builder) {
	renderDefaultArgumentsPrefix(a, telemetry, b)
	if v := strings.TrimSpace(a.Data["language"]); v != "" {
		b.WriteString("- language: `")
		b.WriteString(v)
		b.WriteString("`\n")
	}
	if v := strings.TrimSpace(a.Data["cwd"]); v != "" {
		b.WriteString("- cwd: `")
		b.WriteString(v)
		b.WriteString("`\n")
	}
	if v := strings.TrimSpace(a.Data["timeoutMs"]); v != "" {
		b.WriteString("- timeoutMs: `")
		b.WriteString(v)
		b.WriteString("`\n")
	}
	if v := strings.TrimSpace(a.Data["maxBytes"]); v != "" {
		b.WriteString("- maxOutputBytes: `")
		b.WriteString(v)
		b.WriteString("`\n")
	}
	if v := strings.TrimSpace(a.Data["maxToolCalls"]); v != "" {
		b.WriteString("- maxToolCalls: `")
		b.WriteString(v)
		b.WriteString("`\n")
	}
	if v := strings.TrimSpace(a.Data["code"]); v != "" {
		b.WriteString("- code:\n\n")
		b.WriteString(FormatCode("python", v))
		b.WriteString("\n")
	}
}

type httpFetchRenderer struct{ baseRenderer }

func (httpFetchRenderer) RenderDetail(a Activity, expanded bool, telemetry bool, b *strings.Builder) {
	if !renderCommonOutputPreview(a, expanded, b) {
		status := strings.TrimSpace(a.Data["status"])
		body := strings.TrimSpace(a.Data["body"])
		if status != "" {
			b.WriteString("- status: `")
			b.WriteString(status)
			b.WriteString("`\n")
		}
		if body != "" {
			b.WriteString("\n**Body**\n\n")
			b.WriteString(FormatCode("html", body))
			b.WriteString("\n")
		}
	}
	renderTelemetryBlock(a, telemetry, false, false, b)
}

func (httpFetchRenderer) RenderArguments(a Activity, telemetry bool, b *strings.Builder) {
	renderDefaultArgumentsPrefix(a, telemetry, b)
	if v := strings.TrimSpace(a.Data["url"]); v != "" {
		b.WriteString("- url: `")
		b.WriteString(v)
		b.WriteString("`\n")
	}
	if v := strings.TrimSpace(a.Data["method"]); v != "" {
		b.WriteString("- method: `")
		b.WriteString(v)
		b.WriteString("`\n")
	}
	if v := strings.TrimSpace(a.Data["body"]); v != "" {
		b.WriteString("- body: `")
		b.WriteString(v)
		b.WriteString("`")
		if strings.TrimSpace(a.Data["bodyTruncated"]) == "true" {
			b.WriteString(" _(truncated)_")
		}
		b.WriteString("\n")
	}
}

type traceRunRenderer struct{ baseRenderer }

func (traceRunRenderer) RenderDetail(a Activity, expanded bool, telemetry bool, b *strings.Builder) {
	if !renderCommonOutputPreview(a, expanded, b) {
		output := strings.TrimSpace(a.Data["output"])
		if output != "" {
			b.WriteString("\n**Output**\n\n")
			b.WriteString(FormatCode("text", output))
			b.WriteString("\n")
		}
	}
	renderTelemetryBlock(a, telemetry, false, false, b)
}

func (traceRunRenderer) RenderArguments(a Activity, telemetry bool, b *strings.Builder) {
	renderDefaultArgumentsPrefix(a, telemetry, b)
	if v := strings.TrimSpace(a.Data["traceAction"]); v != "" {
		b.WriteString("- action: `")
		b.WriteString(v)
		b.WriteString("`\n")
	}
	if v := strings.TrimSpace(a.Data["traceKey"]); v != "" {
		b.WriteString("- key: `")
		b.WriteString(v)
		b.WriteString("`\n")
	}
	if v := strings.TrimSpace(a.Data["traceInput"]); v != "" {
		b.WriteString("- input: `")
		b.WriteString(v)
		b.WriteString("`\n")
	}
}

type browserRenderer struct{ baseRenderer }

func (browserRenderer) RenderDetail(a Activity, expanded bool, telemetry bool, b *strings.Builder) {
	if !renderCommonOutputPreview(a, expanded, b) {
		if v := strings.TrimSpace(a.Data["browserOp"]); v != "" {
			b.WriteString("- browserOp: `")
			b.WriteString(v)
			b.WriteString("`\n")
		}
		if v := strings.TrimSpace(a.Data["title"]); v != "" {
			b.WriteString("- title: `")
			b.WriteString(v)
			b.WriteString("`\n")
		}
		if v := strings.TrimSpace(a.Data["url"]); v != "" {
			b.WriteString("- url: `")
			b.WriteString(v)
			b.WriteString("`\n")
		}
		if v := strings.TrimSpace(a.Data["count"]); v != "" {
			b.WriteString("- count: `")
			b.WriteString(v)
			b.WriteString("`\n")
		}
		if v := strings.TrimSpace(a.Data["items"]); v != "" {
			b.WriteString("- items: `")
			b.WriteString(v)
			b.WriteString("`\n")
		}
	}
	renderTelemetryBlock(a, telemetry, false, false, b)
}

func (browserRenderer) RenderArguments(a Activity, telemetry bool, b *strings.Builder) {
	renderDefaultArgumentsPrefix(a, telemetry, b)
	if v := strings.TrimSpace(a.Data["action"]); v != "" {
		b.WriteString("- action: `")
		b.WriteString(v)
		b.WriteString("`\n")
	}
	if v := strings.TrimSpace(a.Data["url"]); v != "" {
		b.WriteString("- url: `")
		b.WriteString(v)
		b.WriteString("`\n")
	}
	if v := strings.TrimSpace(a.Data["selector"]); v != "" {
		b.WriteString("- selector: `")
		b.WriteString(v)
		b.WriteString("`\n")
	}
	if v := strings.TrimSpace(a.Data["sessionId"]); v != "" {
		b.WriteString("- sessionId: `")
		b.WriteString(v)
		b.WriteString("`\n")
	}
	if v := strings.TrimSpace(a.Data["key"]); v != "" {
		b.WriteString("- key: `")
		b.WriteString(v)
		b.WriteString("`\n")
	}
	if v := strings.TrimSpace(a.Data["value"]); v != "" {
		b.WriteString("- value: `")
		b.WriteString(v)
		b.WriteString("`\n")
	}
	if v := strings.TrimSpace(a.Data["filename"]); v != "" {
		b.WriteString("- filename: `")
		b.WriteString(v)
		b.WriteString("`\n")
	}
}

type emailRenderer struct{ baseRenderer }

func (emailRenderer) RenderDetail(a Activity, expanded bool, telemetry bool, b *strings.Builder) {
	if !renderCommonOutputPreview(a, expanded, b) {
		if strings.TrimSpace(a.Ok) == "true" {
			b.WriteString("- status: sent\n")
		}
	}
	renderTelemetryBlock(a, telemetry, false, false, b)
}

func (emailRenderer) RenderArguments(a Activity, telemetry bool, b *strings.Builder) {
	renderDefaultArgumentsPrefix(a, telemetry, b)
	if v := strings.TrimSpace(a.Data["to"]); v != "" {
		b.WriteString("- to: `")
		b.WriteString(v)
		b.WriteString("`\n")
	}
	if v := strings.TrimSpace(a.Data["subject"]); v != "" {
		b.WriteString("- subject: `")
		b.WriteString(v)
		b.WriteString("`\n")
	}
}

type fsSearchRenderer struct{ baseRenderer }

func (fsSearchRenderer) RenderDetail(a Activity, expanded bool, telemetry bool, b *strings.Builder) {
	if !renderCommonOutputPreview(a, expanded, b) {
		if v := strings.TrimSpace(a.Data["results"]); v != "" {
			b.WriteString("- results: `")
			b.WriteString(v)
			b.WriteString("`\n")
		}
	}
	renderTelemetryBlock(a, telemetry, false, false, b)
}

func (fsSearchRenderer) RenderArguments(a Activity, telemetry bool, b *strings.Builder) {
	renderDefaultArgumentsPrefix(a, telemetry, b)
	if v := strings.TrimSpace(a.Data["query"]); v != "" {
		b.WriteString("- query: `")
		b.WriteString(v)
		b.WriteString("`\n")
	}
	if v := strings.TrimSpace(a.Data["limit"]); v != "" {
		b.WriteString("- limit: `")
		b.WriteString(v)
		b.WriteString("`\n")
	}
}

type agentSpawnRenderer struct{ baseRenderer }

func (agentSpawnRenderer) RenderArguments(a Activity, telemetry bool, b *strings.Builder) {
	renderDefaultArgumentsPrefix(a, telemetry, b)
	if v := strings.TrimSpace(a.Data["goal"]); v != "" {
		b.WriteString("- goal: `")
		b.WriteString(v)
		b.WriteString("`\n")
	}
	if v := strings.TrimSpace(a.Data["model"]); v != "" {
		b.WriteString("- model: `")
		b.WriteString(v)
		b.WriteString("`\n")
	}
	if v := strings.TrimSpace(a.Data["requestedMaxTokens"]); v != "" {
		b.WriteString("- requestedMaxTokens: `")
		b.WriteString(v)
		b.WriteString("`\n")
	}
	if v := strings.TrimSpace(a.Data["maxTokens"]); v != "" {
		b.WriteString("- maxTokens: `")
		b.WriteString(v)
		b.WriteString("`\n")
	}
	if v := strings.TrimSpace(a.Data["backgroundCount"]); v != "" {
		b.WriteString("- backgroundCount: `")
		b.WriteString(v)
		b.WriteString("`\n")
	}
	if v := strings.TrimSpace(a.Data["backgroundPreview"]); v != "" {
		b.WriteString("- backgroundPreview: `")
		b.WriteString(v)
		b.WriteString("`\n")
	}
	if v := strings.TrimSpace(a.Data["currentDepth"]); v != "" {
		maxDepth := strings.TrimSpace(a.Data["maxDepth"])
		if maxDepth != "" {
			b.WriteString("- depth: `")
			b.WriteString(v)
			b.WriteString("/")
			b.WriteString(maxDepth)
			b.WriteString("`\n")
		} else {
			b.WriteString("- currentDepth: `")
			b.WriteString(v)
			b.WriteString("`\n")
		}
	}
}

type taskCreateRenderer struct{ baseRenderer }

func (taskCreateRenderer) RenderArguments(a Activity, telemetry bool, b *strings.Builder) {
	renderDefaultArgumentsPrefix(a, telemetry, b)
	if v := strings.TrimSpace(a.Data["goal"]); v != "" {
		b.WriteString("- goal: `")
		b.WriteString(v)
		b.WriteString("`\n")
	}
	if v := strings.TrimSpace(a.Data["taskId"]); v != "" {
		b.WriteString("- taskId: `")
		b.WriteString(v)
		b.WriteString("`\n")
	}
	if v := strings.TrimSpace(a.Data["childRunId"]); v != "" {
		b.WriteString("- childRunId: `")
		b.WriteString(v)
		b.WriteString("`\n")
	}
	if v := strings.TrimSpace(a.Data["batchMode"]); strings.EqualFold(v, "true") {
		b.WriteString("- batchMode: `true`\n")
	}
	if v := strings.TrimSpace(a.Data["batchParentTaskId"]); v != "" {
		b.WriteString("- batchParentTaskId: `")
		b.WriteString(v)
		b.WriteString("`\n")
	}
	if v := strings.TrimSpace(a.Data["output"]); v != "" {
		b.WriteString("- output: ")
		b.WriteString(v)
		b.WriteString("\n")
	} else if v := strings.TrimSpace(a.Data["outputPreview"]); v != "" {
		b.WriteString("- output: ")
		b.WriteString(v)
		b.WriteString("\n")
	}
}

type workdirChangedRenderer struct{ baseRenderer }

func (workdirChangedRenderer) RenderArguments(a Activity, telemetry bool, b *strings.Builder) {
	if strings.TrimSpace(a.From) != "" || strings.TrimSpace(a.To) != "" {
		b.WriteString("- from: `")
		b.WriteString(strings.TrimSpace(a.From))
		b.WriteString("`\n")
		b.WriteString("- to: `")
		b.WriteString(strings.TrimSpace(a.To))
		b.WriteString("`\n")
	}
}

type fsWriteAppendRenderer struct{ baseRenderer }

func (fsWriteAppendRenderer) RenderDetail(a Activity, expanded bool, telemetry bool, b *strings.Builder) {
	if !a.TextRedacted && strings.TrimSpace(a.TextPreview) != "" {
		lang := guessCodeFenceLang(a.Path, a.TextIsJSON)
		b.WriteString("\n**Written content preview**")
		if a.TextTruncated {
			b.WriteString(" _(truncated)_")
		}
		b.WriteString("\n\n")
		if strings.EqualFold(lang, "json") {
			b.WriteString(FormatJSON(a.TextPreview))
		} else {
			b.WriteString(FormatCode(lang, a.TextPreview))
		}
		b.WriteString("\n")
	} else if a.TextRedacted {
		b.WriteString("\n**Written content preview**\n\n_(redacted)_\n")
	}
	renderCommonOutputPreview(a, expanded, b)
	renderTelemetryBlock(a, telemetry, false, true, b)
}

type llmWebSearchRenderer struct{ baseRenderer }

func (llmWebSearchRenderer) RenderDetail(a Activity, expanded bool, telemetry bool, b *strings.Builder) {
	if strings.TrimSpace(a.OutputPreview) != "" {
		b.WriteString("\n**Sources**\n\n")
		b.WriteString(a.OutputPreview)
		if !strings.HasSuffix(a.OutputPreview, "\n") {
			b.WriteString("\n")
		}
	}
	renderTelemetryBlock(a, telemetry, false, false, b)
}

func renderDefaultArgumentsPrefix(a Activity, telemetry bool, b *strings.Builder) {
	if strings.TrimSpace(a.Path) != "" {
		b.WriteString("- path: `")
		b.WriteString(a.Path)
		b.WriteString("`\n")
	}
	if telemetry && strings.TrimSpace(a.MaxBytes) != "" && a.Kind == "fs_read" {
		b.WriteString("- maxBytes: ")
		b.WriteString(a.MaxBytes)
		b.WriteString("\n")
	}
}

func renderCommonOutputPreview(a Activity, expanded bool, b *strings.Builder) bool {
	if strings.TrimSpace(a.OutputPreview) == "" || strings.TrimSpace(a.Kind) == "llm.web.search" {
		return false
	}
	txt := a.OutputPreview
	if !expanded && len(txt) > 600 {
		txt = txt[:599] + "…"
	}
	b.WriteString("\n**Tool output preview** _(press `e` to expand)_\n\n")
	b.WriteString(FormatCode("text", txt))
	b.WriteString("\n")
	return true
}

func renderTelemetryBlock(a Activity, telemetry bool, includeMaxBytes bool, includeTextBytes bool, b *strings.Builder) {
	if !telemetry {
		return
	}
	b.WriteString("\n**Telemetry**\n\n")
	if includeMaxBytes && strings.TrimSpace(a.MaxBytes) != "" {
		b.WriteString("- maxBytes: ")
		b.WriteString(a.MaxBytes)
		b.WriteString("\n")
	}
	if includeTextBytes && strings.TrimSpace(a.TextBytes) != "" {
		b.WriteString("- textBytes: ")
		b.WriteString(a.TextBytes)
		b.WriteString("\n")
	}
	if strings.TrimSpace(a.BytesLen) != "" {
		b.WriteString("- bytesLen: ")
		b.WriteString(a.BytesLen)
		b.WriteString("\n")
	}
	if a.Truncated {
		b.WriteString("- truncated: true\n")
	}
}
