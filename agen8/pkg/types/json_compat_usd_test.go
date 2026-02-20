package types

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestUSDKeys_Marshal_UsesUSDNotUsd(t *testing.T) {
	now := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	sess := Session{
		SessionID:   "sess-1",
		CreatedAt:   &now,
		InputTokens: 1,
		CostUSD:     1.25,
	}
	b, err := json.Marshal(sess)
	if err != nil {
		t.Fatalf("marshal session: %v", err)
	}
	out := string(b)
	if !strings.Contains(out, `"costUSD"`) {
		t.Fatalf("expected session JSON to contain costUSD, got: %s", out)
	}
	if strings.Contains(out, `"costUsd"`) {
		t.Fatalf("expected session JSON to not contain costUsd, got: %s", out)
	}

	run := Run{
		RunID:     "run-1",
		SessionID: "sess-1",
		Goal:      "g",
		Status:    RunStatusRunning,
		StartedAt: &now,
		CostUSD:   2.50,
		Runtime: &RunRuntimeConfig{
			PriceInPerMTokensUSD:  1.1,
			PriceOutPerMTokensUSD: 2.2,
		},
	}
	b, err = json.Marshal(run)
	if err != nil {
		t.Fatalf("marshal run: %v", err)
	}
	out = string(b)
	for _, want := range []string{`"costUSD"`, `"priceInPerMTokensUSD"`, `"priceOutPerMTokensUSD"`} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected run JSON to contain %s, got: %s", want, out)
		}
	}
	for _, old := range []string{`"costUsd"`, `"priceInPerMTokensUsd"`, `"priceOutPerMTokensUsd"`} {
		if strings.Contains(out, old) {
			t.Fatalf("expected run JSON to not contain %s, got: %s", old, out)
		}
	}

	task := Task{TaskID: "task-1", Goal: "g", CostUSD: 3.75}
	b, err = json.Marshal(task)
	if err != nil {
		t.Fatalf("marshal task: %v", err)
	}
	out = string(b)
	if !strings.Contains(out, `"costUSD"`) || strings.Contains(out, `"costUsd"`) {
		t.Fatalf("expected task JSON to use costUSD only, got: %s", out)
	}

	tr := TaskResult{TaskID: "task-1", Status: TaskStatusSucceeded, CostUSD: 4.00}
	b, err = json.Marshal(tr)
	if err != nil {
		t.Fatalf("marshal task result: %v", err)
	}
	out = string(b)
	if !strings.Contains(out, `"costUSD"`) || strings.Contains(out, `"costUsd"`) {
		t.Fatalf("expected task result JSON to use costUSD only, got: %s", out)
	}
}

func TestUSDKeys_Unmarshal_AcceptsUsdAndUSD(t *testing.T) {
	t.Run("session", func(t *testing.T) {
		var s Session
		if err := json.Unmarshal([]byte(`{"sessionId":"sess-1","createdAt":"2020-01-01T00:00:00Z","costUsd":0}`), &s); err != nil {
			t.Fatalf("unmarshal old: %v", err)
		}
		if s.CostUSD != 0 {
			t.Fatalf("expected cost 0 from old key, got %v", s.CostUSD)
		}

		if err := json.Unmarshal([]byte(`{"sessionId":"sess-1","createdAt":"2020-01-01T00:00:00Z","costUSD":2}`), &s); err != nil {
			t.Fatalf("unmarshal new: %v", err)
		}
		if s.CostUSD != 2 {
			t.Fatalf("expected cost 2 from new key, got %v", s.CostUSD)
		}

		if err := json.Unmarshal([]byte(`{"sessionId":"sess-1","createdAt":"2020-01-01T00:00:00Z","costUsd":1,"costUSD":3}`), &s); err != nil {
			t.Fatalf("unmarshal both: %v", err)
		}
		if s.CostUSD != 3 {
			t.Fatalf("expected new key to win, got %v", s.CostUSD)
		}
	})

	t.Run("run_and_runtime_pricing", func(t *testing.T) {
		var r Run
		if err := json.Unmarshal([]byte(`{
			"runId":"run-1",
			"sessionId":"sess-1",
			"goal":"g",
			"status":"running",
			"startedAt":"2020-01-01T00:00:00Z",
			"costUsd":0,
			"runtime":{
				"priceInPerMTokensUsd":0,
				"priceOutPerMTokensUsd":2.2
			}
		}`), &r); err != nil {
			t.Fatalf("unmarshal old: %v", err)
		}
		if r.CostUSD != 0 {
			t.Fatalf("expected cost 0 from old key, got %v", r.CostUSD)
		}
		if r.Runtime == nil {
			t.Fatalf("expected runtime to be set")
		}
		if r.Runtime.PriceInPerMTokensUSD != 0 || r.Runtime.PriceOutPerMTokensUSD != 2.2 {
			t.Fatalf("unexpected pricing: in=%v out=%v", r.Runtime.PriceInPerMTokensUSD, r.Runtime.PriceOutPerMTokensUSD)
		}

		if err := json.Unmarshal([]byte(`{
			"runId":"run-1",
			"sessionId":"sess-1",
			"goal":"g",
			"status":"running",
			"startedAt":"2020-01-01T00:00:00Z",
			"costUSD":5,
			"runtime":{
				"priceInPerMTokensUSD":1.1,
				"priceOutPerMTokensUSD":2.2
			}
		}`), &r); err != nil {
			t.Fatalf("unmarshal new: %v", err)
		}
		if r.CostUSD != 5 {
			t.Fatalf("expected cost 5 from new key, got %v", r.CostUSD)
		}
		if r.Runtime.PriceInPerMTokensUSD != 1.1 || r.Runtime.PriceOutPerMTokensUSD != 2.2 {
			t.Fatalf("unexpected pricing: in=%v out=%v", r.Runtime.PriceInPerMTokensUSD, r.Runtime.PriceOutPerMTokensUSD)
		}
	})

	t.Run("task", func(t *testing.T) {
		var task Task
		if err := json.Unmarshal([]byte(`{"taskId":"task-1","goal":"g","costUsd":0}`), &task); err != nil {
			t.Fatalf("unmarshal old: %v", err)
		}
		if task.CostUSD != 0 {
			t.Fatalf("expected cost 0 from old key, got %v", task.CostUSD)
		}

		if err := json.Unmarshal([]byte(`{"taskId":"task-1","goal":"g","costUSD":7}`), &task); err != nil {
			t.Fatalf("unmarshal new: %v", err)
		}
		if task.CostUSD != 7 {
			t.Fatalf("expected cost 7 from new key, got %v", task.CostUSD)
		}
	})

	t.Run("task_result", func(t *testing.T) {
		var tr TaskResult
		if err := json.Unmarshal([]byte(`{"taskId":"task-1","status":"succeeded","costUsd":0}`), &tr); err != nil {
			t.Fatalf("unmarshal old: %v", err)
		}
		if tr.CostUSD != 0 {
			t.Fatalf("expected cost 0 from old key, got %v", tr.CostUSD)
		}

		if err := json.Unmarshal([]byte(`{"taskId":"task-1","status":"succeeded","costUSD":9}`), &tr); err != nil {
			t.Fatalf("unmarshal new: %v", err)
		}
		if tr.CostUSD != 9 {
			t.Fatalf("expected cost 9 from new key, got %v", tr.CostUSD)
		}
	})
}
