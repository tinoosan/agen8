package coordinator

import (
	"strings"
	"testing"
	"time"

	"github.com/tinoosan/agen8/pkg/types"
)

func TestMergeThinkingEntries(t *testing.T) {
	m := &Model{}
	ts := time.Now()

	entries1 := []feedEntry{
		{kind: feedThinking, sourceID: "evt_1", timestamp: ts},
		{kind: feedAgent, isTaskResponse: true, sourceID: "task_1", timestamp: ts.Add(time.Second)},
	}

	m.mergeThinkingEntries(entries1)
	if len(m.feed) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(m.feed))
	}

	// Add same again plus one new
	entries2 := []feedEntry{
		{kind: feedThinking, sourceID: "evt_1", timestamp: ts},
		{kind: feedAgent, isTaskResponse: true, sourceID: "task_1", timestamp: ts.Add(time.Second)},
		{kind: feedThinking, sourceID: "evt_2", timestamp: ts.Add(time.Second * 2)},
	}

	m.mergeThinkingEntries(entries2)
	if len(m.feed) != 3 {
		t.Fatalf("expected 3 entries after deduplication, got %d", len(m.feed))
	}
}

func TestMergeActivityEntries(t *testing.T) {
	m := &Model{}
	ts := time.Now()

	// Initial feed with user, thinking, agent text, agent op
	m.feed = []feedEntry{
		{kind: feedUser, text: "do it", timestamp: ts},
		{kind: feedThinking, sourceID: "evt_1", timestamp: ts.Add(time.Second)},
		{kind: feedAgent, isText: false, sourceID: "op_1", timestamp: ts.Add(2 * time.Second)},
		{kind: feedAgent, isTaskResponse: true, sourceID: "task_1", timestamp: ts.Add(3 * time.Second)}, // This is response block
	}

	activityEntries := []feedEntry{
		{kind: feedUser, text: "do it", timestamp: ts},
		{kind: feedAgent, isText: false, sourceID: "op_1", timestamp: ts.Add(2 * time.Second)},
		{kind: feedAgent, isTaskResponse: true, sourceID: "task_1", timestamp: ts.Add(3 * time.Second)},
		{kind: feedAgent, isText: false, sourceID: "op_2", timestamp: ts.Add(4 * time.Second)},
	}

	m.mergeActivityEntries(activityEntries)

	// We expect: thinking (retained), user, text response, and both agent ops from new list
	if len(m.feed) != 5 {
		t.Fatalf("expected 5 entries, got %d", len(m.feed))
	}

	// Verify the text entry is retained
	foundText := false
	for _, e := range m.feed {
		if e.kind == feedAgent && e.isTaskResponse && e.sourceID == "task_1" {
			foundText = true
		}
	}
	if !foundText {
		t.Fatalf("agent text entry was missing")
	}
}

func TestActivityToFeedEntry_TaskDoneUsesTaskIDAndIsTaskResponse(t *testing.T) {
	act := types.Activity{
		ID:        "event-123",
		Kind:      "task.done",
		Title:     "Final summary",
		StartedAt: time.Now(),
		Data: map[string]string{
			"taskId": "task-abc",
		},
	}
	entry := activityToFeedEntry(act)
	if entry == nil {
		t.Fatalf("expected entry, got nil")
	}
	if !entry.isTaskResponse {
		t.Fatalf("expected task.done to be marked isTaskResponse")
	}
	if entry.sourceID != "task-abc" {
		t.Fatalf("expected sourceID task-abc, got %q", entry.sourceID)
	}
}

func TestSingleWriter_TaskDoneAndPollSameTaskIDRendersOnce(t *testing.T) {
	m := &Model{}
	ts := time.Now()

	pushAct := types.Activity{
		ID:        "event-1",
		Kind:      "task.done",
		Title:     "Ship it",
		StartedAt: ts,
		Data: map[string]string{
			"taskId": "task-1",
		},
	}
	pushEntry := activityToFeedEntry(pushAct)
	if pushEntry == nil {
		t.Fatalf("expected push entry")
	}

	m.mergeActivityEntries([]feedEntry{*pushEntry})
	m.mergeThinkingEntries([]feedEntry{
		{
			kind:           feedAgent,
			isText:         true,
			isTaskResponse: true,
			sourceID:       "task-1",
			text:           "Ship it",
			timestamp:      ts.Add(time.Second),
		},
	})

	taskResponses := 0
	for _, e := range m.feed {
		if e.isTaskResponse && e.sourceID == "task-1" {
			taskResponses++
		}
	}
	if taskResponses != 1 {
		t.Fatalf("expected exactly one task response for task-1, got %d", taskResponses)
	}
}

func TestAgentSpeakIsNotTerminalTaskResponse(t *testing.T) {
	act := types.Activity{
		ID:        "event-2",
		Kind:      "agent_speak",
		Title:     "Some assistant text",
		StartedAt: time.Now(),
		Data: map[string]string{
			"taskId": "task-2",
		},
	}
	entry := activityToFeedEntry(act)
	if entry == nil {
		t.Fatalf("expected entry, got nil")
	}
	if entry.isTaskResponse {
		t.Fatalf("agent_speak should not be marked as terminal task response")
	}
}

func TestIsActivityText_ToolResultIsOperation(t *testing.T) {
	act := types.Activity{
		ID:    "run-1|op-1",
		Kind:  "tool_result",
		Title: "tool_result",
		Data: map[string]string{
			"opId": "op-1",
		},
	}
	if isActivityText(act) {
		t.Fatalf("tool_result must be treated as operation, not text")
	}
}

func TestMergeActivityEntries_DedupesByIdentity(t *testing.T) {
	m := &Model{}
	ts := time.Now()

	entry := feedEntry{
		kind:      feedAgent,
		timestamp: ts,
		role:      "reviewer",
		text:      "Create task",
		status:    "ok",
		opKind:    "task_create",
		sourceID:  "run-a|op-1",
		data: map[string]string{
			"opId": "op-1",
			"op":   "task_create",
			"goal": "one",
		},
	}
	entry = *normalizeFeedEntry(&entry)

	entries := []feedEntry{entry}
	for i := 0; i < 10; i++ {
		m.mergeActivityEntries(entries)
	}

	count := 0
	for _, e := range m.feed {
		if strings.TrimSpace(e.identityKey) == "op:run-a|op-1" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected one normalized op entry after replay, got %d", count)
	}
}

func TestUpdate_ModelSetSchedulesImmediateRefresh(t *testing.T) {
	m := &Model{endpoint: "tcp://127.0.0.1:1", sessionID: "sess-1"}
	_, cmd := m.Update(modelSetMsg{model: "openai/gpt-5"})
	if cmd == nil {
		t.Fatalf("expected refresh command batch after successful model set")
	}
}
