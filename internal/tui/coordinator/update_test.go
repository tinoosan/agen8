package coordinator

import (
	"testing"
	"time"
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
		{kind: feedAgent, isText: false, sourceID: "op_1", timestamp: ts.Add(2 * time.Second)},
		{kind: feedAgent, isText: false, sourceID: "op_2", timestamp: ts.Add(4 * time.Second)},
	}

	m.mergeActivityEntries(activityEntries)

	// We expect: user, thinking, agent text (retained), and both agent ops from new list
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
