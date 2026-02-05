package protocol

import (
	"testing"
	"time"
)

func TestEmitter_TurnStarted(t *testing.T) {
	var gotMethod string
	var gotParams any
	e := NewEmitter(func(method string, params any) error {
		gotMethod = method
		gotParams = params
		return nil
	})

	now := time.Now().UTC().Truncate(time.Millisecond)
	turn := Turn{ID: "turn-1", ThreadID: "thread-1", Status: TurnStatusInProgress, CreatedAt: now}
	if err := e.EmitTurnStarted(turn); err != nil {
		t.Fatalf("EmitTurnStarted: %v", err)
	}
	if gotMethod != NotifyTurnStarted {
		t.Fatalf("method = %q want %q", gotMethod, NotifyTurnStarted)
	}
	p, ok := gotParams.(TurnNotificationParams)
	if !ok {
		t.Fatalf("params type = %T want TurnNotificationParams", gotParams)
	}
	if p.Turn.ID != turn.ID || p.Turn.ThreadID != turn.ThreadID || p.Turn.Status != turn.Status {
		t.Fatalf("turn mismatch: got %#v want %#v", p.Turn, turn)
	}
}
