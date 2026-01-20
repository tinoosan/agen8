package agent

import "testing"

func TestExtractSingleJSONObject(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		in      string
		wantErr bool
	}{
		{name: "raw_object", in: `{"op":"final","text":"hi"}`, wantErr: false},
		{name: "fenced_json", in: "```json\n{\"op\":\"final\",\"text\":\"hi\"}\n```", wantErr: false},
		{name: "leading_text", in: "here you go:\n{\"op\":\"final\",\"text\":\"hi\"}", wantErr: false},
		{name: "trailing_text", in: "{\"op\":\"final\",\"text\":\"hi\"}\nthanks", wantErr: true},
		{name: "multiple_objects", in: "{\"op\":\"final\",\"text\":\"hi\"}\n{\"op\":\"final\",\"text\":\"bye\"}", wantErr: true},
		{name: "array_not_object", in: `[]`, wantErr: true},
		{name: "empty", in: "   ", wantErr: true},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := extractSingleJSONObject(tc.in)
			if tc.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
