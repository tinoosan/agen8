package app

import "testing"

func TestFSWriteTextPreviewForEvent_JSONPrettyPrints(t *testing.T) {
	prev, truncated, redacted, _, isJSON := fsWriteTextPreviewForEvent("/scratch/x.json", `{"b":2,"a":1}`)
	if redacted {
		t.Fatalf("expected not redacted")
	}
	if truncated {
		t.Fatalf("expected not truncated")
	}
	if !isJSON {
		t.Fatalf("expected isJSON true")
	}
	if prev == `{"b":2,"a":1}` {
		t.Fatalf("expected pretty-printed json, got raw: %q", prev)
	}
}

func TestFSWriteTextPreviewForEvent_RedactsSecrets(t *testing.T) {
	prev, _, redacted, _, _ := fsWriteTextPreviewForEvent("/scratch/x.txt", "Authorization: Bearer sk-SECRET")
	if !redacted {
		t.Fatalf("expected redacted=true")
	}
	if prev != "<omitted>" {
		t.Fatalf("expected <omitted>, got %q", prev)
	}
}
