package protocol

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

func TestMessage_JSONRoundTrip(t *testing.T) {
	msg, err := NewRequest("1", MethodThreadCreate, ThreadCreateParams{Title: "hello"})
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}

	b, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var decoded Message
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if decoded.JSONRPC != "2.0" {
		t.Fatalf("JSONRPC = %q, want %q", decoded.JSONRPC, "2.0")
	}
	if decoded.ID == nil || *decoded.ID != "1" {
		t.Fatalf("ID = %v, want %q", decoded.ID, "1")
	}
	if decoded.Method != MethodThreadCreate {
		t.Fatalf("Method = %q, want %q", decoded.Method, MethodThreadCreate)
	}
	if len(decoded.Params) == 0 {
		t.Fatalf("Params is empty")
	}
}

func TestItem_ContentDecoding(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Millisecond)
	item := Item{
		ID:        "item-1",
		TurnID:    "turn-1",
		Type:      ItemTypeAgentMessage,
		Status:    ItemStatusCompleted,
		CreatedAt: now,
	}

	want := AgentMessageContent{Text: "hi", IsPartial: false}
	if err := item.SetContent(want); err != nil {
		t.Fatalf("SetContent: %v", err)
	}

	var got AgentMessageContent
	if err := item.DecodeContent(&got); err != nil {
		t.Fatalf("DecodeContent: %v", err)
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("content mismatch: got %#v want %#v", got, want)
	}
}

func TestTurn_JSONRoundTrip(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Millisecond)
	turn := Turn{
		ID:        "turn-1",
		ThreadID:  "thread-1",
		Status:    TurnStatusInProgress,
		CreatedAt: now,
		StepCount: 2,
	}

	b, err := json.Marshal(turn)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var got Turn
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if got.ID != turn.ID || got.ThreadID != turn.ThreadID || got.Status != turn.Status || got.StepCount != turn.StepCount {
		t.Fatalf("turn mismatch: got %#v want %#v", got, turn)
	}
	if !got.CreatedAt.Equal(turn.CreatedAt) {
		t.Fatalf("CreatedAt mismatch: got %v want %v", got.CreatedAt, turn.CreatedAt)
	}
}

func TestStatusConstants_Unique(t *testing.T) {
	turnStatuses := []string{
		string(TurnStatusPending),
		string(TurnStatusInProgress),
		string(TurnStatusCompleted),
		string(TurnStatusFailed),
		string(TurnStatusCanceled),
	}
	itemStatuses := []string{
		string(ItemStatusStarted),
		string(ItemStatusStreaming),
		string(ItemStatusCompleted),
		string(ItemStatusFailed),
		string(ItemStatusCanceled),
	}

	assertUnique := func(name string, vals []string) {
		seen := map[string]struct{}{}
		for _, v := range vals {
			if _, ok := seen[v]; ok {
				t.Fatalf("%s duplicate: %q", name, v)
			}
			seen[v] = struct{}{}
		}
	}

	assertUnique("TurnStatus", turnStatuses)
	assertUnique("ItemStatus", itemStatuses)
}
