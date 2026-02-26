package cmd

import "testing"

func TestObserveAliasCommandsRegistered(t *testing.T) {
	for _, name := range []string{"status", "feed", "trace", "costs"} {
		if _, _, err := rootCmd.Find([]string{name}); err != nil {
			t.Fatalf("expected command %q to be registered: %v", name, err)
		}
	}
}
