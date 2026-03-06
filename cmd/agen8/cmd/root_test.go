package cmd

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestRootCommand_UsesConfiguredEntrypoint(t *testing.T) {
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

func TestRootHelp_UsesCutoverSurface(t *testing.T) {
	buf := &bytes.Buffer{}
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"--help"})
	t.Cleanup(func() {
		rootCmd.SetOut(os.Stdout)
		rootCmd.SetErr(os.Stderr)
		rootCmd.SetArgs(nil)
	})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	helpText := buf.String()
	commandsSection := helpText
	if idx := strings.Index(helpText, "Flags:"); idx >= 0 {
		commandsSection = helpText[:idx]
	}
	for _, want := range []string{"project", "team", "task", "view", "monitor", "logs"} {
		if !strings.Contains(commandsSection, want) {
			t.Fatalf("help missing %q:\n%s", want, helpText)
		}
	}
	for _, unwanted := range []string{"coordinator", "sessions", "watch", "doctor", "whoami"} {
		if strings.Contains(commandsSection, unwanted) {
			t.Fatalf("help unexpectedly contains %q:\n%s", unwanted, helpText)
		}
	}
}
