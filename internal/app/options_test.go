package app

import "testing"

func TestResolveRunChatOptions_AuthProviderFromEnv(t *testing.T) {
	t.Setenv("AGEN8_AUTH_PROVIDER", "chatgpt_account")
	opts, err := resolveRunChatOptions()
	if err != nil {
		t.Fatalf("resolveRunChatOptions: %v", err)
	}
	if opts.AuthProvider != "chatgpt_account" {
		t.Fatalf("AuthProvider=%q", opts.AuthProvider)
	}
}

func TestResolveRunChatOptions_AuthProviderOverride(t *testing.T) {
	t.Setenv("AGEN8_AUTH_PROVIDER", "api_key")
	opts, err := resolveRunChatOptions(WithAuthProvider("chatgpt_account"))
	if err != nil {
		t.Fatalf("resolveRunChatOptions: %v", err)
	}
	if opts.AuthProvider != "chatgpt_account" {
		t.Fatalf("AuthProvider=%q", opts.AuthProvider)
	}
}
