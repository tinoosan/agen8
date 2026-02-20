package cmd

import (
	"context"
	"testing"

	"github.com/tinoosan/agen8/pkg/config"
)

func TestRootCommand_StartsDetachedMonitorByDefault(t *testing.T) {
	orig := runDetachedMonitorFn
	t.Cleanup(func() { runDetachedMonitorFn = orig })

	called := false
	runDetachedMonitorFn = func(_ context.Context, _ config.Config) error {
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
