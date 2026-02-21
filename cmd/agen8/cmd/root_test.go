package cmd

import (
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
