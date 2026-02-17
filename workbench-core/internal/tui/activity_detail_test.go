package tui

import (
	"strings"
	"testing"
)

func TestRenderActivityDetailMarkdown_FSWrite_ShowsContentPreview(t *testing.T) {
	a := Activity{
		Kind:        "fs_write",
		Title:       "Write /workspace/example.json",
		Path:        "/workspace/example.json",
		TextPreview: `{"a":1,"b":{"c":2}}`,
		TextIsJSON:  true,
	}

	md := renderActivityDetailMarkdown(a, false, false)
	if !strings.Contains(md, "Written content preview") {
		t.Fatalf("expected content preview section, got:\n%s", md)
	}
	if !strings.Contains(md, "```json") {
		t.Fatalf("expected json code fence, got:\n%s", md)
	}
}

func TestRenderActivityDetailMarkdown_BrowserArgumentsAndOutput(t *testing.T) {
	a := Activity{
		Kind:   "browser",
		Title:  "browse.navigate https://example.com",
		Path:   "",
		Ok:     "true",
		Status: ActivityOK,
		Data: map[string]string{
			"action":    "navigate",
			"url":       "https://example.com",
			"sessionId": "sess-1",
			"browserOp": "browser.navigate",
			"title":     "Example Domain",
		},
	}

	md := renderActivityDetailMarkdown(a, false, false)
	for _, want := range []string{
		"- action: `navigate`",
		"- url: `https://example.com`",
		"- sessionId: `sess-1`",
		"- browserOp: `browser.navigate`",
		"- title: `Example Domain`",
	} {
		if !strings.Contains(md, want) {
			t.Fatalf("expected %q in markdown, got:\n%s", want, md)
		}
	}
}

func TestRenderActivityDetailMarkdown_EmailAndFSSearch(t *testing.T) {
	email := Activity{
		Kind:   "email",
		Ok:     "true",
		Status: ActivityOK,
		Data: map[string]string{
			"to":      "team@example.com",
			"subject": "Daily report",
		},
	}
	emailMD := renderActivityDetailMarkdown(email, false, false)
	for _, want := range []string{
		"- to: `team@example.com`",
		"- subject: `Daily report`",
		"- status: sent",
	} {
		if !strings.Contains(emailMD, want) {
			t.Fatalf("expected %q in email markdown, got:\n%s", want, emailMD)
		}
	}

	search := Activity{
		Kind:   "fs_search",
		Path:   "/workspace",
		Status: ActivityOK,
		Ok:     "true",
		Data: map[string]string{
			"query":   "needle",
			"limit":   "25",
			"results": "7",
		},
	}
	searchMD := renderActivityDetailMarkdown(search, false, false)
	for _, want := range []string{
		"- path: `/workspace`",
		"- query: `needle`",
		"- limit: `25`",
		"- results: `7`",
	} {
		if !strings.Contains(searchMD, want) {
			t.Fatalf("expected %q in fs_search markdown, got:\n%s", want, searchMD)
		}
	}
}

func TestRenderActivityDetailMarkdown_AgentSpawnArgumentsAndOutput(t *testing.T) {
	a := Activity{
		Kind:          "agent_spawn",
		Status:        ActivityOK,
		Ok:            "true",
		OutputPreview: "Child finished: 42",
		Data: map[string]string{
			"goal":               "Compute 40 + 2",
			"model":              "gpt-5-mini",
			"requestedMaxTokens": "128",
			"maxTokens":          "128",
			"backgroundCount":    "2",
			"backgroundPreview":  `["prior=40","add two"]`,
			"currentDepth":       "0",
			"maxDepth":           "3",
		},
	}

	md := renderActivityDetailMarkdown(a, false, false)
	for _, want := range []string{
		"- goal: `Compute 40 + 2`",
		"- model: `gpt-5-mini`",
		"- requestedMaxTokens: `128`",
		"- maxTokens: `128`",
		"- backgroundCount: `2`",
		"- backgroundPreview: `[\"prior=40\",\"add two\"]`",
		"- depth: `0/3`",
		"Tool output preview",
		"Child finished: 42",
	} {
		if !strings.Contains(md, want) {
			t.Fatalf("expected %q in agent_spawn markdown, got:\n%s", want, md)
		}
	}
}

func TestRenderActivityDetailMarkdown_TaskCreate(t *testing.T) {
	a := Activity{
		Kind:   "task_create",
		Status: ActivityOK,
		Ok:     "true",
		Data: map[string]string{
			"goal":       "Implement the login flow",
			"taskId":     "task-20250101T120000Z-abc",
			"childRunId": "run-child-1",
			"output":     "Task task-20250101T120000Z-abc created and worker agent spawned",
		},
	}

	md := renderActivityDetailMarkdown(a, false, false)
	for _, want := range []string{
		"- goal: `Implement the login flow`",
		"- taskId: `task-20250101T120000Z-abc`",
		"- childRunId: `run-child-1`",
		"Task task-20250101T120000Z-abc created and worker agent spawned",
	} {
		if !strings.Contains(md, want) {
			t.Fatalf("expected %q in task_create markdown, got:\n%s", want, md)
		}
	}
}

func TestRenderActivityDetailMarkdown_UnknownKind_DefaultFallback(t *testing.T) {
	a := Activity{
		Kind:          "mcp_call",
		Status:        ActivityOK,
		Path:          "/workspace/notes.md",
		Ok:            "true",
		OutputPreview: "done",
		Data: map[string]string{
			"any": "value",
		},
	}

	md := renderActivityDetailMarkdown(a, false, false)
	for _, want := range []string{
		"**Fields**",
		"**Arguments**",
		"**Output**",
		"- path: `/workspace/notes.md`",
		"Tool output preview",
		"done",
	} {
		if !strings.Contains(md, want) {
			t.Fatalf("expected %q in unknown-kind markdown, got:\n%s", want, md)
		}
	}
}

func TestRenderActivityDetailMarkdown_Dispatch_ShellExecSpecificOutput(t *testing.T) {
	a := Activity{
		Kind:   "shell_exec",
		Status: ActivityOK,
		Ok:     "true",
		Data: map[string]string{
			"argvPreview": "echo hello",
			"cwd":         "/workspace",
			"exitCode":    "0",
			"stdout":      "hello",
		},
	}

	md := renderActivityDetailMarkdown(a, false, false)
	for _, want := range []string{
		"- command:",
		"```bash",
		"echo hello",
		"- cwd: `/workspace`",
		"- exitCode: `0`",
		"**stdout**",
		"hello",
	} {
		if !strings.Contains(md, want) {
			t.Fatalf("expected %q in shell_exec markdown, got:\n%s", want, md)
		}
	}
	if strings.Contains(md, "**Body**") {
		t.Fatalf("did not expect http_fetch body section in shell_exec markdown, got:\n%s", md)
	}
}

func TestRenderActivityDetailMarkdown_Dispatch_HTTPFetchSpecificOutput(t *testing.T) {
	a := Activity{
		Kind:   "http_fetch",
		Status: ActivityOK,
		Ok:     "true",
		Data: map[string]string{
			"url":    "https://example.com",
			"method": "GET",
			"status": "200",
			"body":   "<html>ok</html>",
		},
	}

	md := renderActivityDetailMarkdown(a, false, false)
	for _, want := range []string{
		"- url: `https://example.com`",
		"- method: `GET`",
		"- status: `200`",
		"**Body**",
		"<html>ok</html>",
	} {
		if !strings.Contains(md, want) {
			t.Fatalf("expected %q in http_fetch markdown, got:\n%s", want, md)
		}
	}
	if strings.Contains(md, "**stdout**") {
		t.Fatalf("did not expect shell stdout section in http_fetch markdown, got:\n%s", md)
	}
}

func TestRenderActivityDetailMarkdown_Dispatch_WorkdirChangedArguments(t *testing.T) {
	a := Activity{
		Kind:   "workdir.changed",
		Status: ActivityOK,
		From:   "/workspace/old",
		To:     "/workspace/new",
	}

	md := renderActivityDetailMarkdown(a, false, false)
	for _, want := range []string{
		"- from: `/workspace/old`",
		"- to: `/workspace/new`",
	} {
		if !strings.Contains(md, want) {
			t.Fatalf("expected %q in workdir.changed markdown, got:\n%s", want, md)
		}
	}
	if strings.Contains(md, "- path:") {
		t.Fatalf("did not expect path argument for workdir.changed, got:\n%s", md)
	}
}

func TestRenderActivityDetailMarkdown_TelemetryParity(t *testing.T) {
	fsRead := Activity{
		Kind:     "fs_read",
		Status:   ActivityOK,
		Ok:       "true",
		MaxBytes: "4096",
		BytesLen: "512",
	}
	withTelemetry := renderActivityDetailMarkdown(fsRead, true, false)
	if !strings.Contains(withTelemetry, "**Telemetry**") || !strings.Contains(withTelemetry, "- maxBytes: 4096") || !strings.Contains(withTelemetry, "- bytesLen: 512") {
		t.Fatalf("expected fs_read telemetry details, got:\n%s", withTelemetry)
	}
	withoutTelemetry := renderActivityDetailMarkdown(fsRead, false, false)
	if strings.Contains(withoutTelemetry, "**Telemetry**") || strings.Contains(withoutTelemetry, "maxBytes") {
		t.Fatalf("did not expect telemetry section when disabled, got:\n%s", withoutTelemetry)
	}

	fsWrite := Activity{
		Kind:      "fs_write",
		Status:    ActivityOK,
		Ok:        "true",
		TextBytes: "123",
		BytesLen:  "123",
		Truncated: true,
	}
	writeTelemetry := renderActivityDetailMarkdown(fsWrite, true, false)
	for _, want := range []string{
		"**Telemetry**",
		"- textBytes: 123",
		"- bytesLen: 123",
		"- truncated: true",
	} {
		if !strings.Contains(writeTelemetry, want) {
			t.Fatalf("expected %q in fs_write telemetry markdown, got:\n%s", want, writeTelemetry)
		}
	}
}
