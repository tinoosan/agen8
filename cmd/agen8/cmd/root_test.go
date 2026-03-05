package cmd

import (
	"os"
	"testing"

	"github.com/spf13/cobra"
)

func TestRootCommand_StartsDetachedMonitorByDefault(t *testing.T) {
	orig := runRootEntrypointFn
	t.Cleanup(func() { runRootEntrypointFn = orig })

	called := false
	runRootEntrypointFn = func(_ *cobra.Command) error {
		called = true
		return nil
	}

	if err := rootCmd.RunE(rootCmd, nil); err != nil {
		t.Fatalf("root command returned error: %v", err)
	}
	if !called {
		t.Fatalf("expected detached monitor start")
	}
}

func TestPersistentPreRun_DoesNotForceAuthProviderWhenFlagNotSet(t *testing.T) {
	t.Setenv("AGEN8_AUTH_PROVIDER", "")
	authProvider = "api_key"
	if f := rootCmd.PersistentFlags().Lookup("auth-provider"); f != nil {
		f.Changed = false
	}
	if err := rootCmd.PersistentPreRunE(rootCmd, nil); err != nil {
		t.Fatalf("PersistentPreRunE: %v", err)
	}
	if got, ok := os.LookupEnv("AGEN8_AUTH_PROVIDER"); ok && got != "" {
		t.Fatalf("expected auth provider env to remain unset when flag not set, got %q", got)
	}
}
