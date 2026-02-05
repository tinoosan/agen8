package protocol

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
)

// Index maintains an in-memory view of turns and items derived from protocol notifications.
//
// It is used by the app server to serve item.list for clients that connect after a run has started.
type Index struct {
	mu sync.Mutex

	turns map[TurnID]Turn

	itemsByTurn map[TurnID][]Item
	itemsByID   map[ItemID]Item

	maxTotal   int
	maxPerTurn int
	totalItems int
}

// NewIndex creates a new Index.
//
// maxTotal caps the total number of items retained across all turns (0 disables the cap).
// maxPerTurn caps items retained per turn (0 disables the cap).
func NewIndex(maxTotal int, maxPerTurn int) *Index {
	return &Index{
		turns:       make(map[TurnID]Turn),
		itemsByTurn: make(map[TurnID][]Item),
		itemsByID:   make(map[ItemID]Item),
		maxTotal:    maxTotal,
		maxPerTurn:  maxPerTurn,
	}
}

// Apply consumes a protocol notification (method+params) and updates the index.
func (x *Index) Apply(method string, params any) {
	if x == nil {
		return
	}
	method = strings.TrimSpace(method)
	if method == "" {
		return
	}

	x.mu.Lock()
	defer x.mu.Unlock()

	switch method {
	case NotifyTurnStarted, NotifyTurnCompleted, NotifyTurnFailed:
		var p TurnNotificationParams
		if !coerceParams(params, &p) {
			return
		}
		if strings.TrimSpace(string(p.Turn.ID)) == "" {
			return
		}
		x.turns[p.Turn.ID] = p.Turn
		return

	case NotifyItemStarted, NotifyItemCompleted:
		var p ItemNotificationParams
		if !coerceParams(params, &p) {
			return
		}
		x.applyItem(p.Item)
		return

	case NotifyItemDelta:
		var p ItemDeltaParams
		if !coerceParams(params, &p) {
			return
		}
		x.applyDelta(p.ItemID, p.Delta)
		return
	}
}

// ListByTurn returns items for a given turn, paginated by an opaque cursor.
//
// Cursor format: "i:<n>" where n is the starting index in the per-turn timeline.
func (x *Index) ListByTurn(turnID TurnID, cursor string, limit int) ([]Item, string) {
	if x == nil {
		return nil, ""
	}
	turnID = TurnID(strings.TrimSpace(string(turnID)))
	if turnID == "" {
		return nil, ""
	}
	if limit <= 0 {
		limit = 100
	}

	start := 0
	if cursor != "" {
		if i, ok := parseIndexCursor(cursor); ok && i >= 0 {
			start = i
		}
	}

	x.mu.Lock()
	defer x.mu.Unlock()

	all := x.itemsByTurn[turnID]
	if start >= len(all) {
		return nil, ""
	}
	end := start + limit
	if end > len(all) {
		end = len(all)
	}
	out := make([]Item, 0, end-start)
	out = append(out, all[start:end]...)

	nextCursor := ""
	if end < len(all) {
		nextCursor = fmt.Sprintf("i:%d", end)
	}
	return out, nextCursor
}

func (x *Index) applyItem(item Item) {
	if x == nil {
		return
	}
	if strings.TrimSpace(string(item.ID)) == "" {
		return
	}
	turnID := TurnID(strings.TrimSpace(string(item.TurnID)))
	if turnID == "" {
		return
	}

	if existing, ok := x.itemsByID[item.ID]; ok {
		// Preserve content when a caller doesn't send it in later snapshots.
		if len(item.Content) == 0 && len(existing.Content) != 0 {
			item.Content = existing.Content
		}
		if item.CreatedAt.IsZero() && !existing.CreatedAt.IsZero() {
			item.CreatedAt = existing.CreatedAt
		}
	}

	// Append to timeline if this is the first time we see the item.
	if _, ok := x.itemsByID[item.ID]; !ok {
		x.itemsByTurn[turnID] = append(x.itemsByTurn[turnID], item)
		x.totalItems++
	} else {
		x.replaceInTimeline(turnID, item)
	}

	x.itemsByID[item.ID] = item
	x.enforceCaps(turnID)
}

func (x *Index) applyDelta(itemID ItemID, delta ItemDelta) {
	if x == nil {
		return
	}
	itemID = ItemID(strings.TrimSpace(string(itemID)))
	if itemID == "" {
		return
	}
	item, ok := x.itemsByID[itemID]
	if !ok {
		return
	}
	if item.Type != ItemTypeAgentMessage && item.Type != ItemTypeReasoning {
		return
	}

	switch item.Type {
	case ItemTypeAgentMessage:
		var c AgentMessageContent
		_ = item.DecodeContent(&c)
		if delta.TextDelta != "" {
			c.Text += delta.TextDelta
			c.IsPartial = true
			_ = item.SetContent(c)
		}
	case ItemTypeReasoning:
		var c ReasoningContent
		_ = item.DecodeContent(&c)
		if delta.ReasoningDelta != "" {
			c.Summary += delta.ReasoningDelta
			_ = item.SetContent(c)
		}
	}

	if item.Status == ItemStatusStarted {
		item.Status = ItemStatusStreaming
	}

	x.itemsByID[itemID] = item
	turnID := TurnID(strings.TrimSpace(string(item.TurnID)))
	if turnID != "" {
		x.replaceInTimeline(turnID, item)
	}
}

func (x *Index) replaceInTimeline(turnID TurnID, item Item) {
	if x == nil {
		return
	}
	arr := x.itemsByTurn[turnID]
	for i := len(arr) - 1; i >= 0; i-- {
		if arr[i].ID == item.ID {
			arr[i] = item
			x.itemsByTurn[turnID] = arr
			return
		}
	}
	// If missing, append as a last resort.
	x.itemsByTurn[turnID] = append(arr, item)
	x.totalItems++
}

func (x *Index) enforceCaps(turnID TurnID) {
	if x == nil {
		return
	}

	if x.maxPerTurn > 0 {
		arr := x.itemsByTurn[turnID]
		if len(arr) > x.maxPerTurn {
			drop := len(arr) - x.maxPerTurn
			for i := 0; i < drop; i++ {
				delete(x.itemsByID, arr[i].ID)
				x.totalItems--
			}
			x.itemsByTurn[turnID] = append([]Item(nil), arr[drop:]...)
		}
	}

	if x.maxTotal > 0 && x.totalItems > x.maxTotal {
		// Best-effort global cap: drop from the oldest turns we have, without trying to be perfect.
		for x.totalItems > x.maxTotal {
			var oldestTurn TurnID
			for tid := range x.itemsByTurn {
				oldestTurn = tid
				break
			}
			if oldestTurn == "" {
				break
			}
			arr := x.itemsByTurn[oldestTurn]
			if len(arr) == 0 {
				delete(x.itemsByTurn, oldestTurn)
				continue
			}
			delete(x.itemsByID, arr[0].ID)
			x.itemsByTurn[oldestTurn] = arr[1:]
			x.totalItems--
			if len(x.itemsByTurn[oldestTurn]) == 0 {
				delete(x.itemsByTurn, oldestTurn)
			}
		}
	}
}

func parseIndexCursor(s string) (int, bool) {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "i:") {
		return 0, false
	}
	raw := strings.TrimSpace(strings.TrimPrefix(s, "i:"))
	if raw == "" {
		return 0, false
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, false
	}
	return n, true
}

func coerceParams(in any, out any) bool {
	if in == nil || out == nil {
		return false
	}
	switch v := in.(type) {
	case []byte:
		return json.Unmarshal(v, out) == nil
	case json.RawMessage:
		return json.Unmarshal(v, out) == nil
	default:
		// Marshal->Unmarshal is a pragmatic bridge for tests and internal handler paths.
		b, err := json.Marshal(in)
		if err != nil {
			return false
		}
		return json.Unmarshal(b, out) == nil
	}
}
