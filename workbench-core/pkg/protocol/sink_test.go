package protocol

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/tinoosan/workbench-core/pkg/emit"
	"github.com/tinoosan/workbench-core/pkg/types"
)

type capturedNotification struct {
	method string
	params json.RawMessage
}

func TestSink_TaskStartEmitsTurnAndUserItem(t *testing.T) {
	now := time.Date(2026, 2, 5, 12, 0, 0, 0, time.UTC)
	var got []capturedNotification

	s := NewEventSink(emit.SinkFunc[Notification](func(_ context.Context, msg emit.Message[Notification]) error {
		b, err := json.Marshal(msg.Payload.Params)
		if err != nil {
			t.Fatalf("marshal params: %v", err)
		}
		got = append(got, capturedNotification{method: msg.Payload.Method, params: b})
		return nil
	}), WithThreadID("sess-1"), WithNow(func() time.Time { return now }))

	ev := types.EventRecord{
		Type:      "task.start",
		Message:   "Task started",
		Timestamp: now,
		Data:      map[string]string{"taskId": "task-1", "goal": "do the thing"},
	}
	if err := s.Emit(context.Background(), emit.Message[types.EventRecord]{RunID: "run-1", Payload: ev}); err != nil {
		t.Fatalf("Emit: %v", err)
	}

	if len(got) != 3 {
		t.Fatalf("notifications = %d want %d", len(got), 3)
	}
	if got[0].method != NotifyTurnStarted {
		t.Fatalf("got[0].method = %q want %q", got[0].method, NotifyTurnStarted)
	}

	var tp TurnNotificationParams
	if err := json.Unmarshal(got[0].params, &tp); err != nil {
		t.Fatalf("unmarshal TurnNotificationParams: %v", err)
	}
	if tp.Turn.ID != "task-1" || tp.Turn.ThreadID != "sess-1" || tp.Turn.Status != TurnStatusInProgress {
		t.Fatalf("turn = %#v", tp.Turn)
	}

	var ip ItemNotificationParams
	if err := json.Unmarshal(got[1].params, &ip); err != nil {
		t.Fatalf("unmarshal ItemNotificationParams: %v", err)
	}
	if ip.Item.Type != ItemTypeUserMessage || ip.Item.Status != ItemStatusStarted {
		t.Fatalf("item started = %#v", ip.Item)
	}
	var um UserMessageContent
	if err := ip.Item.DecodeContent(&um); err != nil {
		t.Fatalf("DecodeContent: %v", err)
	}
	if um.Text != "do the thing" {
		t.Fatalf("user text = %q want %q", um.Text, "do the thing")
	}

	if got[2].method != NotifyItemCompleted {
		t.Fatalf("got[2].method = %q want %q", got[2].method, NotifyItemCompleted)
	}
}

func TestSink_OpRequestResponseEmitsToolItems(t *testing.T) {
	now := time.Date(2026, 2, 5, 12, 0, 0, 0, time.UTC)
	var got []capturedNotification
	s := NewEventSink(emit.SinkFunc[Notification](func(_ context.Context, msg emit.Message[Notification]) error {
		b, err := json.Marshal(msg.Payload.Params)
		if err != nil {
			t.Fatalf("marshal params: %v", err)
		}
		got = append(got, capturedNotification{method: msg.Payload.Method, params: b})
		return nil
	}), WithThreadID("sess-1"), WithNow(func() time.Time { return now }))

	// Start a turn.
	_ = s.Emit(context.Background(), emit.Message[types.EventRecord]{RunID: "run-1", Payload: types.EventRecord{
		Type:      "task.start",
		Message:   "Task started",
		Timestamp: now,
		Data:      map[string]string{"taskId": "task-1", "goal": "read a file"},
	}})
	got = nil

	// Tool request.
	_ = s.Emit(context.Background(), emit.Message[types.EventRecord]{RunID: "run-1", Payload: types.EventRecord{
		Type:      "agent.op.request",
		Message:   "Agent requested host op",
		Timestamp: now,
		Data:      map[string]string{"opId": "op-1", "op": "fs.read", "path": "/foo.txt"},
	}})
	if len(got) != 1 || got[0].method != NotifyItemStarted {
		t.Fatalf("got = %#v", got)
	}
	var started ItemNotificationParams
	if err := json.Unmarshal(got[0].params, &started); err != nil {
		t.Fatalf("unmarshal started: %v", err)
	}
	if started.Item.ID != "op-1" || started.Item.TurnID != "task-1" || started.Item.Type != ItemTypeToolExecution {
		t.Fatalf("started item = %#v", started.Item)
	}
	if started.Item.RunID != "run-1" {
		t.Fatalf("started item runId = %q want %q", started.Item.RunID, "run-1")
	}

	// Tool response.
	got = nil
	_ = s.Emit(context.Background(), emit.Message[types.EventRecord]{RunID: "run-1", Payload: types.EventRecord{
		Type:      "agent.op.response",
		Message:   "Host op completed",
		Timestamp: now,
		Data:      map[string]string{"opId": "op-1", "op": "fs.read", "ok": "true", "bytesLen": "10"},
	}})
	if len(got) != 1 || got[0].method != NotifyItemCompleted {
		t.Fatalf("got = %#v", got)
	}
	var completed ItemNotificationParams
	if err := json.Unmarshal(got[0].params, &completed); err != nil {
		t.Fatalf("unmarshal completed: %v", err)
	}
	if completed.Item.ID != "op-1" || completed.Item.Status != ItemStatusCompleted {
		t.Fatalf("completed item = %#v", completed.Item)
	}
	if completed.Item.RunID != "run-1" {
		t.Fatalf("completed item runId = %q want %q", completed.Item.RunID, "run-1")
	}
	var te ToolExecutionContent
	if err := completed.Item.DecodeContent(&te); err != nil {
		t.Fatalf("DecodeContent: %v", err)
	}
	if te.ToolName != "fs.read" || !te.Ok || len(te.Output) == 0 {
		t.Fatalf("tool content = %#v", te)
	}
}

func TestSink_TaskDoneEmitsAgentMessageAndTurnCompleted(t *testing.T) {
	now := time.Date(2026, 2, 5, 12, 0, 0, 0, time.UTC)
	var got []capturedNotification
	s := NewEventSink(emit.SinkFunc[Notification](func(_ context.Context, msg emit.Message[Notification]) error {
		b, err := json.Marshal(msg.Payload.Params)
		if err != nil {
			t.Fatalf("marshal params: %v", err)
		}
		got = append(got, capturedNotification{method: msg.Payload.Method, params: b})
		return nil
	}), WithThreadID("sess-1"), WithNow(func() time.Time { return now }))

	_ = s.Emit(context.Background(), emit.Message[types.EventRecord]{RunID: "run-1", Payload: types.EventRecord{
		Type:      "task.start",
		Message:   "Task started",
		Timestamp: now,
		Data:      map[string]string{"taskId": "task-1", "goal": "say hi"},
	}})
	got = nil

	_ = s.Emit(context.Background(), emit.Message[types.EventRecord]{RunID: "run-1", Payload: types.EventRecord{
		Type:      "task.done",
		Message:   "Task finished",
		Timestamp: now,
		Data:      map[string]string{"taskId": "task-1", "status": "succeeded", "summary": "hello"},
	}})

	if len(got) != 3 {
		t.Fatalf("notifications = %d want %d", len(got), 3)
	}
	if got[0].method != NotifyItemStarted || got[1].method != NotifyItemCompleted || got[2].method != NotifyTurnCompleted {
		t.Fatalf("methods = %q,%q,%q", got[0].method, got[1].method, got[2].method)
	}

	var completed ItemNotificationParams
	if err := json.Unmarshal(got[1].params, &completed); err != nil {
		t.Fatalf("unmarshal completed: %v", err)
	}
	var am AgentMessageContent
	if err := completed.Item.DecodeContent(&am); err != nil {
		t.Fatalf("DecodeContent: %v", err)
	}
	if am.Text != "hello" {
		t.Fatalf("agent text = %q want %q", am.Text, "hello")
	}
}

func TestSink_StepEmitsReasoningItem(t *testing.T) {
	now := time.Date(2026, 2, 5, 12, 0, 0, 0, time.UTC)
	var got []capturedNotification
	s := NewEventSink(emit.SinkFunc[Notification](func(_ context.Context, msg emit.Message[Notification]) error {
		b, err := json.Marshal(msg.Payload.Params)
		if err != nil {
			t.Fatalf("marshal params: %v", err)
		}
		got = append(got, capturedNotification{method: msg.Payload.Method, params: b})
		return nil
	}), WithThreadID("sess-1"), WithNow(func() time.Time { return now }))

	_ = s.Emit(context.Background(), emit.Message[types.EventRecord]{RunID: "run-1", Payload: types.EventRecord{
		Type:      "task.start",
		Message:   "Task started",
		Timestamp: now,
		Data:      map[string]string{"taskId": "task-1", "goal": "reason"},
	}})
	got = nil

	_ = s.Emit(context.Background(), emit.Message[types.EventRecord]{RunID: "run-1", Payload: types.EventRecord{
		Type:      "agent.step",
		Message:   "Step 1 completed",
		Timestamp: now,
		Data:      map[string]string{"step": "1", "reasoningSummary": "did stuff"},
	}})

	if len(got) != 2 || got[0].method != NotifyItemStarted || got[1].method != NotifyItemCompleted {
		t.Fatalf("got = %#v", got)
	}
	var started ItemNotificationParams
	if err := json.Unmarshal(got[0].params, &started); err != nil {
		t.Fatalf("unmarshal started: %v", err)
	}
	if started.Item.Type != ItemTypeReasoning {
		t.Fatalf("item type = %q", started.Item.Type)
	}
	var rc ReasoningContent
	if err := started.Item.DecodeContent(&rc); err != nil {
		t.Fatalf("DecodeContent: %v", err)
	}
	if rc.Step != 1 || rc.Summary != "did stuff" {
		t.Fatalf("reasoning = %#v", rc)
	}
}

func TestEventSink_Diagnostics_MissingActiveTurn(t *testing.T) {
	now := time.Date(2026, 2, 5, 12, 0, 0, 0, time.UTC)
	var diags []Diagnostic

	s := NewEventSink(emit.SinkFunc[Notification](func(_ context.Context, _ emit.Message[Notification]) error {
		return nil
	}), WithThreadID("sess-1"), WithNow(func() time.Time { return now }), WithDiagnostics(func(d Diagnostic) {
		diags = append(diags, d)
	}))

	_ = s.Emit(context.Background(), emit.Message[types.EventRecord]{RunID: "run-1", Payload: types.EventRecord{
		Type:      "agent.step",
		Message:   "Step 1 completed",
		Timestamp: now,
		Data:      map[string]string{"step": "1", "reasoningSummary": "did stuff"},
	}})

	if len(diags) != 1 {
		t.Fatalf("diags=%d want 1", len(diags))
	}
	if diags[0].Reason != "missing_active_turn" {
		t.Fatalf("reason=%q want %q", diags[0].Reason, "missing_active_turn")
	}
}
