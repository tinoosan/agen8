package protocol

import (
	"encoding/json"
	"testing"
	"time"
)

func TestIndex_ApplyAndListByTurn(t *testing.T) {
	x := NewIndex(0, 0)

	now := time.Now().UTC().Truncate(time.Millisecond)
	turn := Turn{
		ID:        "task-1",
		ThreadID:  "sess-1",
		RunID:     "run-1",
		Status:    TurnStatusInProgress,
		CreatedAt: now,
	}
	x.Apply(NotifyTurnStarted, TurnNotificationParams{Turn: turn})

	item := Item{
		ID:        "item-1",
		TurnID:    "task-1",
		RunID:     "run-1",
		Type:      ItemTypeAgentMessage,
		Status:    ItemStatusStarted,
		CreatedAt: now,
	}
	_ = item.SetContent(AgentMessageContent{Text: "", IsPartial: true})
	x.Apply(NotifyItemStarted, ItemNotificationParams{Item: item})
	x.Apply(NotifyItemDelta, ItemDeltaParams{ItemID: item.ID, Delta: ItemDelta{TextDelta: "hel"}})
	x.Apply(NotifyItemDelta, ItemDeltaParams{ItemID: item.ID, Delta: ItemDelta{TextDelta: "lo"}})

	item.Status = ItemStatusCompleted
	_ = item.SetContent(AgentMessageContent{Text: "hello", IsPartial: false})
	x.Apply(NotifyItemCompleted, ItemNotificationParams{Item: item})

	items, next := x.ListByTurn("task-1", "", 100)
	if next != "" {
		t.Fatalf("next cursor = %q want empty", next)
	}
	if len(items) != 1 {
		t.Fatalf("items len = %d want 1", len(items))
	}
	if items[0].RunID != "run-1" {
		t.Fatalf("item.runId = %q want %q", items[0].RunID, "run-1")
	}
	var c AgentMessageContent
	if err := json.Unmarshal(items[0].Content, &c); err != nil {
		t.Fatalf("unmarshal content: %v", err)
	}
	if c.Text != "hello" || c.IsPartial {
		t.Fatalf("content = %#v", c)
	}
}

func TestIndex_ListByTurn_CursorPagination(t *testing.T) {
	x := NewIndex(0, 0)
	now := time.Now().UTC()
	turnID := TurnID("task-1")

	for i := 0; i < 5; i++ {
		it := Item{
			ID:        ItemID("item-" + string(rune('a'+i))),
			TurnID:    turnID,
			RunID:     "run-1",
			Type:      ItemTypeReasoning,
			Status:    ItemStatusCompleted,
			CreatedAt: now,
		}
		_ = it.SetContent(ReasoningContent{Summary: "s", Step: i})
		x.Apply(NotifyItemStarted, ItemNotificationParams{Item: it})
	}

	page1, cur := x.ListByTurn(turnID, "", 2)
	if len(page1) != 2 || cur == "" {
		t.Fatalf("page1 len=%d cur=%q", len(page1), cur)
	}
	page2, cur2 := x.ListByTurn(turnID, cur, 2)
	if len(page2) != 2 || cur2 == "" {
		t.Fatalf("page2 len=%d cur2=%q", len(page2), cur2)
	}
	page3, cur3 := x.ListByTurn(turnID, cur2, 2)
	if len(page3) != 1 || cur3 != "" {
		t.Fatalf("page3 len=%d cur3=%q", len(page3), cur3)
	}
}

func TestIndex_ListByThread(t *testing.T) {
	x := NewIndex(0, 0)
	now := time.Now().UTC().Truncate(time.Millisecond)

	// Create two turns on the same thread, one turn on a different thread.
	turn1 := Turn{ID: "task-1", ThreadID: "sess-1", RunID: "run-1", Status: TurnStatusCompleted, CreatedAt: now}
	turn2 := Turn{ID: "task-2", ThreadID: "sess-1", RunID: "run-1", Status: TurnStatusCompleted, CreatedAt: now.Add(time.Second)}
	turnOther := Turn{ID: "task-other", ThreadID: "sess-other", RunID: "run-2", Status: TurnStatusCompleted, CreatedAt: now}

	x.Apply(NotifyTurnStarted, TurnNotificationParams{Turn: turn1})
	x.Apply(NotifyTurnStarted, TurnNotificationParams{Turn: turn2})
	x.Apply(NotifyTurnStarted, TurnNotificationParams{Turn: turnOther})

	// Add items: 2 in turn1, 1 in turn2, 1 in turnOther.
	for i, tid := range []TurnID{"task-1", "task-1", "task-2", "task-other"} {
		it := Item{
			ID:        ItemID("item-" + string(rune('a'+i))),
			TurnID:    tid,
			RunID:     "run-1",
			Type:      ItemTypeAgentMessage,
			Status:    ItemStatusCompleted,
			CreatedAt: now.Add(time.Duration(i) * time.Millisecond),
		}
		_ = it.SetContent(AgentMessageContent{Text: "msg-" + string(rune('a'+i))})
		x.Apply(NotifyItemStarted, ItemNotificationParams{Item: it})
	}

	// ListByThread for sess-1 should return 3 items (2 from turn1 + 1 from turn2), in order.
	items, next := x.ListByThread("sess-1", "", 100)
	if next != "" {
		t.Fatalf("next cursor = %q want empty", next)
	}
	if len(items) != 3 {
		t.Fatalf("items len = %d want 3", len(items))
	}
	// Verify ordering: turn1 items first (task-1), then turn2 items (task-2).
	if items[0].TurnID != "task-1" || items[1].TurnID != "task-1" || items[2].TurnID != "task-2" {
		t.Fatalf("turn ordering wrong: %v, %v, %v", items[0].TurnID, items[1].TurnID, items[2].TurnID)
	}

	// ListByThread for sess-other should return 1 item.
	items2, _ := x.ListByThread("sess-other", "", 100)
	if len(items2) != 1 {
		t.Fatalf("sess-other items len = %d want 1", len(items2))
	}

	// ListByThread with pagination.
	page1, cur := x.ListByThread("sess-1", "", 2)
	if len(page1) != 2 || cur == "" {
		t.Fatalf("page1 len=%d cur=%q", len(page1), cur)
	}
	page2, cur2 := x.ListByThread("sess-1", cur, 2)
	if len(page2) != 1 || cur2 != "" {
		t.Fatalf("page2 len=%d cur2=%q", len(page2), cur2)
	}

	// Unknown thread returns nothing.
	items3, _ := x.ListByThread("nonexistent", "", 100)
	if len(items3) != 0 {
		t.Fatalf("nonexistent thread items len = %d want 0", len(items3))
	}
}

func TestIndex_ApplyTurnCanceledUpdatesTurnState(t *testing.T) {
	x := NewIndex(0, 0)
	now := time.Now().UTC().Truncate(time.Millisecond)
	turn := Turn{
		ID:        "task-1",
		ThreadID:  "sess-1",
		RunID:     "run-1",
		Status:    TurnStatusCanceled,
		CreatedAt: now,
	}

	x.Apply(NotifyTurnCanceled, TurnNotificationParams{Turn: turn})

	// Ensure turn notification is ingested and indexed without breaking item listing.
	_ = x.turns[turn.ID]
	if got := x.turns[turn.ID]; got.Status != TurnStatusCanceled {
		t.Fatalf("turn.status = %q want %q", got.Status, TurnStatusCanceled)
	}
	items, next := x.ListByTurn(turn.ID, "", 10)
	if len(items) != 0 {
		t.Fatalf("items len = %d want 0", len(items))
	}
	if next != "" {
		t.Fatalf("next cursor = %q want empty", next)
	}
}
