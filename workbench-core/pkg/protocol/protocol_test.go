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

func TestArtifactRequest_JSONRoundTrip(t *testing.T) {
	msg, err := NewRequest("a1", MethodArtifactSearch, ArtifactSearchParams{
		ThreadID: "thread-1",
		Query:    "summary",
		ScopeKey: "role:2026-02-08:ceo",
		Limit:    25,
	})
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
	if decoded.Method != MethodArtifactSearch {
		t.Fatalf("Method = %q, want %q", decoded.Method, MethodArtifactSearch)
	}
	var params ArtifactSearchParams
	if err := json.Unmarshal(decoded.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.Query != "summary" || params.ScopeKey == "" {
		t.Fatalf("unexpected params: %+v", params)
	}
}

func TestSessionTotalsRequest_JSONRoundTrip(t *testing.T) {
	msg, err := NewRequest("s1", MethodSessionGetTotals, SessionGetTotalsParams{
		ThreadID: "thread-1",
		TeamID:   "team-a",
		RunID:    "run-a",
	})
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
	if decoded.Method != MethodSessionGetTotals {
		t.Fatalf("Method = %q, want %q", decoded.Method, MethodSessionGetTotals)
	}
	var params SessionGetTotalsParams
	if err := json.Unmarshal(decoded.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.ThreadID != "thread-1" || params.TeamID != "team-a" || params.RunID != "run-a" {
		t.Fatalf("unexpected params: %+v", params)
	}
}

func TestSessionStartRequest_JSONRoundTrip(t *testing.T) {
	msg, err := NewRequest("ss1", MethodSessionStart, SessionStartParams{
		ThreadID: "thread-1",
		Mode:     "standalone",
		Profile:  "general",
		Goal:     "new goal",
		Model:    "openai/gpt-5-mini",
	})
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
	if decoded.Method != MethodSessionStart {
		t.Fatalf("Method = %q, want %q", decoded.Method, MethodSessionStart)
	}
	var params SessionStartParams
	if err := json.Unmarshal(decoded.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.ThreadID != "thread-1" || params.Mode != "standalone" || params.Profile != "general" || params.Model != "openai/gpt-5-mini" {
		t.Fatalf("unexpected params: %+v", params)
	}
}

func TestSessionListRequest_JSONRoundTrip(t *testing.T) {
	msg, err := NewRequest("sl1", MethodSessionList, SessionListParams{
		ThreadID:      "thread-1",
		TitleContains: "proj",
		Limit:         25,
		Offset:        50,
	})
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
	if decoded.Method != MethodSessionList {
		t.Fatalf("Method = %q, want %q", decoded.Method, MethodSessionList)
	}
	var params SessionListParams
	if err := json.Unmarshal(decoded.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.ThreadID != "thread-1" || params.Limit != 25 || params.Offset != 50 {
		t.Fatalf("unexpected params: %+v", params)
	}
}

func TestSessionRenameRequest_JSONRoundTrip(t *testing.T) {
	msg, err := NewRequest("sr1", MethodSessionRename, SessionRenameParams{
		ThreadID:  "thread-1",
		SessionID: "sess-1",
		Title:     "Renamed Session",
	})
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
	if decoded.Method != MethodSessionRename {
		t.Fatalf("Method = %q, want %q", decoded.Method, MethodSessionRename)
	}
	var params SessionRenameParams
	if err := json.Unmarshal(decoded.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.ThreadID != "thread-1" || params.SessionID != "sess-1" || params.Title != "Renamed Session" {
		t.Fatalf("unexpected params: %+v", params)
	}
}

func TestAgentListRequest_JSONRoundTrip(t *testing.T) {
	msg, err := NewRequest("al1", MethodAgentList, AgentListParams{
		ThreadID:  "thread-1",
		SessionID: "sess-1",
	})
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
	if decoded.Method != MethodAgentList {
		t.Fatalf("Method = %q, want %q", decoded.Method, MethodAgentList)
	}
	var params AgentListParams
	if err := json.Unmarshal(decoded.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.ThreadID != "thread-1" || params.SessionID != "sess-1" {
		t.Fatalf("unexpected params: %+v", params)
	}
}

func TestAgentStartRequest_JSONRoundTrip(t *testing.T) {
	msg, err := NewRequest("as1", MethodAgentStart, AgentStartParams{
		ThreadID:  "thread-1",
		SessionID: "sess-1",
		Profile:   "general",
		Goal:      "continue",
		Model:     "openai/gpt-5-mini",
	})
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
	if decoded.Method != MethodAgentStart {
		t.Fatalf("Method = %q, want %q", decoded.Method, MethodAgentStart)
	}
	var params AgentStartParams
	if err := json.Unmarshal(decoded.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.ThreadID != "thread-1" || params.SessionID != "sess-1" || params.Profile != "general" {
		t.Fatalf("unexpected params: %+v", params)
	}
}
