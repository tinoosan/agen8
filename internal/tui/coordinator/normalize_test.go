package coordinator

import (
	"fmt"
	"testing"
	"time"
)

func TestNormalizeFeedEntry_CanonicalizesToolResultTag(t *testing.T) {
	entry := &feedEntry{
		kind:      feedAgent,
		opKind:    "tool_result",
		sourceID:  "run-1|op-2",
		timestamp: time.Now(),
		data: map[string]string{
			"tag":  "task_create",
			"opId": "op-2",
		},
	}
	got := normalizeFeedEntry(entry)
	if got.opKind != "task_create" {
		t.Fatalf("opKind=%q want task_create", got.opKind)
	}
	if got.identityKey != "op:run-1|op-2" {
		t.Fatalf("identityKey=%q want op:run-1|op-2", got.identityKey)
	}
}

func TestDedupeFeedEntriesByIdentity_ReplayStable(t *testing.T) {
	base := make([]feedEntry, 0, 1000)
	now := time.Now()
	for i := 0; i < 1000; i++ {
		base = append(base, feedEntry{
			kind:      feedAgent,
			timestamp: now.Add(time.Duration(i) * time.Millisecond),
			opKind:    "task_create",
			sourceID:  fmt.Sprintf("run-a|op-%d", i),
			data: map[string]string{
				"op":   "task_create",
				"opId": fmt.Sprintf("op-%d", i),
			},
		})
	}

	feed := []feedEntry{}
	for i := 0; i < 10; i++ {
		feed = append(feed, base...)
		feed = dedupeFeedEntriesByIdentity(feed)
	}

	if len(feed) != 1000 {
		t.Fatalf("len(feed)=%d want 1000", len(feed))
	}
}
