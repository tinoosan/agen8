package harness

import "testing"

func TestSelectHarnessIDPrecedence(t *testing.T) {
	meta := map[string]any{"harnessId": "custom-a"}
	id := SelectHarnessID(meta, "run-a", func(string) string { return "env-a" })
	if id != "custom-a" {
		t.Fatalf("metadata precedence failed: got %q", id)
	}

	id = SelectHarnessID(nil, "run-a", func(string) string { return "env-a" })
	if id != "run-a" {
		t.Fatalf("run precedence failed: got %q", id)
	}

	id = SelectHarnessID(nil, "", func(string) string { return "env-a" })
	if id != "env-a" {
		t.Fatalf("env precedence failed: got %q", id)
	}

	id = SelectHarnessID(nil, "", func(string) string { return "" })
	if id != NativeAdapterID {
		t.Fatalf("fallback failed: got %q", id)
	}
}

func TestSetHarnessIDMetadata(t *testing.T) {
	meta := SetHarnessIDMetadata(nil, "Codex-CLI")
	if got := HarnessIDFromMetadata(meta); got != "codex-cli" {
		t.Fatalf("HarnessIDFromMetadata = %q", got)
	}
	meta = SetHarnessIDMetadata(meta, "")
	if got := HarnessIDFromMetadata(meta); got != "" {
		t.Fatalf("expected empty harness id, got %q", got)
	}
}
