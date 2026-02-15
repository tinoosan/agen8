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
