package runtime

import "testing"

func TestFSWriteTextPreviewForEvent_JSONPrettyPrints(t *testing.T) {
	prev, truncated, redacted, _, isJSON := fsWriteTextPreviewForEvent("/workspace/x.json", `{"b":2,"a":1}`)
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
	prev, _, redacted, _, _ := fsWriteTextPreviewForEvent("/workspace/x.txt", "Authorization: Bearer sk-SECRET")
	if !redacted {
		t.Fatalf("expected redacted=true")
	}
	if prev != "<omitted>" {
		t.Fatalf("expected <omitted>, got %q", prev)
	}
}

func TestIsSensitiveKey(t *testing.T) {
	cases := map[string]bool{
		"text":          true,
		"stdin":         true,
		"authorization": true,
		"token":         true,
		"apikey":        true,
		"api_key":       true,
		"secret":        true,
		"password":      true,
		"body":          false,
		"other":         false,
	}
	for in, want := range cases {
		if got := isSensitiveKey(in); got != want {
			t.Fatalf("isSensitiveKey(%q)=%v want %v", in, got, want)
		}
	}
}

func TestIsMessageLikeKey(t *testing.T) {
	cases := map[string]bool{
		"message": true,
		"body":    true,
		"patch":   true,
		"text":    false,
		"other":   false,
	}
	for in, want := range cases {
		if got := isMessageLikeKey(in); got != want {
			t.Fatalf("isMessageLikeKey(%q)=%v want %v", in, got, want)
		}
	}
}

func TestLooksSensitiveText(t *testing.T) {
	sensitive := []string{
		"Authorization: Bearer abc",
		`{"api_key":"value"}`,
		"apikey=123",
		"keep secret value",
		"password=abc",
		"token sk-abc",
	}
	for _, s := range sensitive {
		if !looksSensitiveText(s) {
			t.Fatalf("expected sensitive: %q", s)
		}
	}
	notSensitive := []string{
		"hello world",
		"message body",
		"authorization header missing bearer",
	}
	for _, s := range notSensitive {
		if looksSensitiveText(s) {
			t.Fatalf("expected non-sensitive: %q", s)
		}
	}
}
