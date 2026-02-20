package protocol

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestThreadUSDKeys_Marshal_UsesUSDNotUsd(t *testing.T) {
	th := Thread{
		ID:        ThreadID("sess-1"),
		CreatedAt: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
		CostUSD:   1.0,
	}
	b, err := json.Marshal(th)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	out := string(b)
	if !strings.Contains(out, `"costUSD"`) {
		t.Fatalf("expected thread JSON to contain costUSD, got: %s", out)
	}
	if strings.Contains(out, `"costUsd"`) {
		t.Fatalf("expected thread JSON to not contain costUsd, got: %s", out)
	}
}

func TestThreadUSDKeys_Unmarshal_AcceptsUsdAndUSD(t *testing.T) {
	var th Thread
	if err := json.Unmarshal([]byte(`{"id":"sess-1","createdAt":"2020-01-01T00:00:00Z","costUsd":0}`), &th); err != nil {
		t.Fatalf("unmarshal old: %v", err)
	}
	if th.CostUSD != 0 {
		t.Fatalf("expected old key cost 0, got %v", th.CostUSD)
	}
	if err := json.Unmarshal([]byte(`{"id":"sess-1","createdAt":"2020-01-01T00:00:00Z","costUSD":2}`), &th); err != nil {
		t.Fatalf("unmarshal new: %v", err)
	}
	if th.CostUSD != 2 {
		t.Fatalf("expected new key cost 2, got %v", th.CostUSD)
	}
	if err := json.Unmarshal([]byte(`{"id":"sess-1","createdAt":"2020-01-01T00:00:00Z","costUsd":1,"costUSD":3}`), &th); err != nil {
		t.Fatalf("unmarshal both: %v", err)
	}
	if th.CostUSD != 3 {
		t.Fatalf("expected new key to win, got %v", th.CostUSD)
	}
}
