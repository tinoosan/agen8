package app

import "testing"

func TestMergeTaskMetadataPreservesExistingValues(t *testing.T) {
	merged := mergeTaskMetadata(map[string]any{
		"missionRef": "mission-1",
		"owner":      "backend",
	}, map[string]any{
		"commit": "abc123",
		"owner":  "reviewed",
	})

	if merged["missionRef"] != "mission-1" {
		t.Fatalf("missionRef=%v", merged["missionRef"])
	}
	if merged["commit"] != "abc123" {
		t.Fatalf("commit=%v", merged["commit"])
	}
	if merged["owner"] != "reviewed" {
		t.Fatalf("owner=%v", merged["owner"])
	}
}

func TestMergeTaskMetadataCopiesExistingWhenNoUpdates(t *testing.T) {
	existing := map[string]any{"missionRef": "mission-1"}
	merged := mergeTaskMetadata(existing, nil)
	merged["missionRef"] = "changed"

	if existing["missionRef"] != "mission-1" {
		t.Fatalf("existing map was mutated: %+v", existing)
	}
}
