package auth

import "testing"

func TestParseProvider(t *testing.T) {
	tests := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{in: "", want: ProviderAPIKey},
		{in: "api_key", want: ProviderAPIKey},
		{in: "chatgpt_account", want: ProviderChatGPTAccount},
		{in: "bad", wantErr: true},
	}
	for _, tt := range tests {
		got, err := ParseProvider(tt.in)
		if tt.wantErr {
			if err == nil {
				t.Fatalf("ParseProvider(%q): expected error", tt.in)
			}
			continue
		}
		if err != nil {
			t.Fatalf("ParseProvider(%q): %v", tt.in, err)
		}
		if got != tt.want {
			t.Fatalf("ParseProvider(%q)=%q, want %q", tt.in, got, tt.want)
		}
	}
}
