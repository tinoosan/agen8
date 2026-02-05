package protocol

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/tinoosan/workbench-core/pkg/events"
	"github.com/tinoosan/workbench-core/pkg/types"
)

// Sink converts EventRecord events into protocol notifications.
//
// It is intended to be installed alongside existing sinks (store, console, etc.)
// and should not change existing behavior.
type Sink struct {
	emitter *Emitter

	mu sync.Mutex

	threadID ThreadID
	now      func() time.Time

	activeTurn  map[string]*Turn // runID -> active turn
	activeItems map[string]*Item // opId -> active item
}

type SinkOption func(*Sink)

// WithThreadID sets the thread ID used for all emitted turns (typically SessionID).
func WithThreadID(id ThreadID) SinkOption {
	return func(s *Sink) {
		if s == nil {
			return
		}
		s.threadID = id
	}
}

// WithNow overrides the clock used when events have no timestamp (for tests).
func WithNow(now func() time.Time) SinkOption {
	return func(s *Sink) {
		if s == nil {
			return
		}
		s.now = now
	}
}

// NewSink creates a new protocol sink.
func NewSink(handler NotificationHandler, opts ...SinkOption) *Sink {
	s := &Sink{
		emitter:     NewEmitter(handler),
		now:         time.Now,
		activeTurn:  make(map[string]*Turn),
		activeItems: make(map[string]*Item),
	}
	for _, opt := range opts {
		if opt != nil {
			opt(s)
		}
	}
	return s
}

// Emit implements events.Sink.
func (s *Sink) Emit(_ context.Context, runID string, event types.EventRecord) error {
	if s == nil || s.emitter == nil {
		return nil
	}

	s.mu.Lock()
	calls := s.mapEventLocked(runID, event)
	s.mu.Unlock()

	var errs error
	for _, c := range calls {
		switch c.method {
		case NotifyTurnStarted:
			if p, ok := c.params.(TurnNotificationParams); ok {
				errs = errors.Join(errs, s.emitter.EmitTurnStarted(p.Turn))
				continue
			}
		case NotifyTurnCompleted:
			if p, ok := c.params.(TurnNotificationParams); ok {
				errs = errors.Join(errs, s.emitter.EmitTurnCompleted(p.Turn))
				continue
			}
		case NotifyTurnFailed:
			if p, ok := c.params.(TurnNotificationParams); ok {
				errs = errors.Join(errs, s.emitter.EmitTurnFailed(p.Turn))
				continue
			}
		case NotifyItemStarted:
			if p, ok := c.params.(ItemNotificationParams); ok {
				errs = errors.Join(errs, s.emitter.EmitItemStarted(p.Item))
				continue
			}
		case NotifyItemCompleted:
			if p, ok := c.params.(ItemNotificationParams); ok {
				errs = errors.Join(errs, s.emitter.EmitItemCompleted(p.Item))
				continue
			}
		case NotifyItemDelta:
			if p, ok := c.params.(ItemDeltaParams); ok {
				errs = errors.Join(errs, s.emitter.EmitItemDelta(p.ItemID, p.Delta))
				continue
			}
		}
		// Fallback (should not happen in normal flow).
		errs = errors.Join(errs, s.emitter.emit(c.method, c.params))
	}
	return errs
}

func (s *Sink) mapEventLocked(runID string, ev types.EventRecord) []notificationCall {
	runID = strings.TrimSpace(runID)
	kind := trimType(ev)
	if kind == "" {
		return nil
	}
	ts := eventTime(s.now, ev)
	threadID := ensureThreadID(s.threadID, runID)
	protoRunID := RunID(strings.TrimSpace(runID))

	switch kind {
	case "task.start", "task.started":
		taskID := mapGet(ev.Data, "taskId")
		turnID := TurnID(taskID)
		if strings.TrimSpace(string(turnID)) == "" {
			turnID = TurnID(newID("turn-"))
		}
		turn := Turn{
			ID:        turnID,
			ThreadID:  threadID,
			RunID:     protoRunID,
			Status:    TurnStatusInProgress,
			CreatedAt: ts,
		}
		s.activeTurn[runID] = &turn

		out := []notificationCall{
			{method: NotifyTurnStarted, params: TurnNotificationParams{Turn: turn}},
		}

		goal := mapGet(ev.Data, "goal")
		if goal == "" {
			goal = strings.TrimSpace(ev.Message)
		}
		if goal != "" {
			item := Item{
				ID:        itemIDForTurn(turn.ID, "user"),
				TurnID:    turn.ID,
				RunID:     protoRunID,
				Type:      ItemTypeUserMessage,
				Status:    ItemStatusStarted,
				CreatedAt: ts,
			}
			_ = item.SetContent(UserMessageContent{Text: goal})
			out = append(out,
				notificationCall{method: NotifyItemStarted, params: ItemNotificationParams{Item: item}},
			)
			item.Status = ItemStatusCompleted
			out = append(out,
				notificationCall{method: NotifyItemCompleted, params: ItemNotificationParams{Item: item}},
			)
		}
		return out

	case "task.done", "task.completed", "task.failed", "task.quarantined", "task.canceled", "task.cancelled":
		taskID := mapGet(ev.Data, "taskId")
		turn := s.activeTurn[runID]
		if turn == nil || (taskID != "" && string(turn.ID) != taskID) {
			// If we missed task.start, synthesize a minimal turn to anchor completion.
			turnID := TurnID(taskID)
			if strings.TrimSpace(string(turnID)) == "" {
				turnID = TurnID(newID("turn-"))
			}
			turn = &Turn{
				ID:        turnID,
				ThreadID:  threadID,
				RunID:     protoRunID,
				Status:    TurnStatusInProgress,
				CreatedAt: ts,
			}
		}

		statusStr := mapGet(ev.Data, "status")
		if statusStr == "" && kind == "task.failed" || kind == "task.quarantined" {
			statusStr = "failed"
		}
		if statusStr == "" && (kind == "task.canceled" || kind == "task.cancelled") {
			statusStr = "canceled"
		}

		if st, ok := turnStatusFromTaskStatus(statusStr); ok {
			turn.Status = st
		} else {
			turn.Status = TurnStatusCompleted
		}
		if turn.Status == TurnStatusFailed {
			if msg := mapGet(ev.Data, "error"); msg != "" {
				turn.Error = &Error{Message: msg}
			} else {
				turn.Error = &Error{Message: "task failed"}
			}
		}

		out := []notificationCall(nil)

		// Emit final agent message if present (best-effort).
		if sum := mapGet(ev.Data, "summary"); sum != "" {
			item := Item{
				ID:        itemIDForTurn(turn.ID, "assistant"),
				TurnID:    turn.ID,
				RunID:     protoRunID,
				Type:      ItemTypeAgentMessage,
				Status:    ItemStatusStarted,
				CreatedAt: ts,
			}
			_ = item.SetContent(AgentMessageContent{Text: sum, IsPartial: false})
			out = append(out, notificationCall{method: NotifyItemStarted, params: ItemNotificationParams{Item: item}})

			item.Status = ItemStatusCompleted
			out = append(out, notificationCall{method: NotifyItemCompleted, params: ItemNotificationParams{Item: item}})
		}

		// Turn notification.
		switch turn.Status {
		case TurnStatusFailed:
			out = append(out, notificationCall{method: NotifyTurnFailed, params: TurnNotificationParams{Turn: *turn}})
		default:
			out = append(out, notificationCall{method: NotifyTurnCompleted, params: TurnNotificationParams{Turn: *turn}})
		}

		delete(s.activeTurn, runID)
		// Prevent leaks if a turn terminates while ops are in-flight.
		s.activeItems = make(map[string]*Item)
		return out

	case "agent.op.request":
		opID := mapGet(ev.Data, "opId")
		if opID == "" {
			opID = newID("item-")
		}
		op := mapGet(ev.Data, "op")
		path := mapGet(ev.Data, "path")
		if shouldSuppressOp(op, path) {
			return nil
		}

		turn := s.activeTurn[runID]
		if turn == nil {
			// No active turn to attach to; avoid inventing one implicitly.
			return nil
		}

		item := Item{
			ID:        ItemID(opID),
			TurnID:    turn.ID,
			RunID:     protoRunID,
			Type:      ItemTypeToolExecution,
			Status:    ItemStatusStarted,
			CreatedAt: ts,
		}
		reqPayload := struct {
			Message string            `json:"message,omitempty"`
			Data    map[string]string `json:"data,omitempty"`
		}{
			Message: strings.TrimSpace(ev.Message),
			Data:    ev.Data,
		}
		content := ToolExecutionContent{
			ToolName: op,
			Input:    rawJSON(reqPayload),
		}
		_ = item.SetContent(content)

		s.activeItems[opID] = &item
		return []notificationCall{
			{method: NotifyItemStarted, params: ItemNotificationParams{Item: item}},
		}

	case "agent.op.response":
		opID := mapGet(ev.Data, "opId")
		if opID == "" {
			return nil
		}
		item := s.activeItems[opID]
		turn := s.activeTurn[runID]
		if item == nil {
			if turn == nil {
				return nil
			}
			item = &Item{
				ID:        ItemID(opID),
				TurnID:    turn.ID,
				RunID:     protoRunID,
				Type:      ItemTypeToolExecution,
				Status:    ItemStatusStarted,
				CreatedAt: ts,
			}
		}

		// Update with response payload.
		var prev ToolExecutionContent
		_ = item.DecodeContent(&prev)

		okVal, okParsed := parseBoolString(mapGet(ev.Data, "ok"))
		respPayload := struct {
			Message string            `json:"message,omitempty"`
			Data    map[string]string `json:"data,omitempty"`
		}{
			Message: strings.TrimSpace(ev.Message),
			Data:    ev.Data,
		}
		prev.Output = rawJSON(respPayload)
		if okParsed {
			prev.Ok = okVal
		}
		if strings.TrimSpace(prev.ToolName) == "" {
			prev.ToolName = mapGet(ev.Data, "op")
		}
		_ = item.SetContent(prev)

		item.Status = ItemStatusCompleted
		if okParsed && !okVal {
			item.Status = ItemStatusFailed
			if msg := mapGet(ev.Data, "err"); msg != "" {
				item.Error = &Error{Message: msg}
			}
		}

		delete(s.activeItems, opID)
		return []notificationCall{
			{method: NotifyItemCompleted, params: ItemNotificationParams{Item: *item}},
		}

	case "agent.step":
		turn := s.activeTurn[runID]
		if turn == nil {
			return nil
		}
		stepN, _ := parseIntString(mapGet(ev.Data, "step"))
		summary := mapGet(ev.Data, "reasoningSummary")

		item := Item{
			ID:        itemIDForTurn(turn.ID, "reasoning-"+mapGet(ev.Data, "step")),
			TurnID:    turn.ID,
			RunID:     protoRunID,
			Type:      ItemTypeReasoning,
			Status:    ItemStatusStarted,
			CreatedAt: ts,
		}
		_ = item.SetContent(ReasoningContent{Summary: summary, Step: stepN})

		started := notificationCall{method: NotifyItemStarted, params: ItemNotificationParams{Item: item}}
		item.Status = ItemStatusCompleted
		completed := notificationCall{method: NotifyItemCompleted, params: ItemNotificationParams{Item: item}}
		return []notificationCall{started, completed}

	case "user.message":
		turn := s.activeTurn[runID]
		if turn == nil {
			return nil
		}
		text := mapGet(ev.Data, "text")
		if text == "" {
			text = strings.TrimSpace(ev.Message)
		}
		if text == "" {
			return nil
		}
		item := Item{
			ID:        ItemID(newID("item-")),
			TurnID:    turn.ID,
			RunID:     protoRunID,
			Type:      ItemTypeUserMessage,
			Status:    ItemStatusStarted,
			CreatedAt: ts,
		}
		_ = item.SetContent(UserMessageContent{Text: text})
		out := []notificationCall{{method: NotifyItemStarted, params: ItemNotificationParams{Item: item}}}
		item.Status = ItemStatusCompleted
		out = append(out, notificationCall{method: NotifyItemCompleted, params: ItemNotificationParams{Item: item}})
		return out

	case "agent.final":
		turn := s.activeTurn[runID]
		if turn == nil {
			return nil
		}
		text := mapGet(ev.Data, "text")
		if text == "" {
			text = strings.TrimSpace(ev.Message)
		}
		if text == "" {
			return nil
		}
		item := Item{
			ID:        ItemID(newID("item-")),
			TurnID:    turn.ID,
			RunID:     protoRunID,
			Type:      ItemTypeAgentMessage,
			Status:    ItemStatusStreaming,
			CreatedAt: ts,
		}
		_ = item.SetContent(AgentMessageContent{Text: "", IsPartial: true})
		out := []notificationCall{{method: NotifyItemStarted, params: ItemNotificationParams{Item: item}}}
		out = append(out, notificationCall{method: NotifyItemDelta, params: ItemDeltaParams{ItemID: item.ID, Delta: ItemDelta{TextDelta: text}}})
		item.Status = ItemStatusCompleted
		_ = item.SetContent(AgentMessageContent{Text: text, IsPartial: false})
		out = append(out, notificationCall{method: NotifyItemCompleted, params: ItemNotificationParams{Item: item}})
		return out

	default:
		// Ignore all other events.
		return nil
	}
}

var _ events.Sink = (*Sink)(nil)
