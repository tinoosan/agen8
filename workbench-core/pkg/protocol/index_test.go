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
