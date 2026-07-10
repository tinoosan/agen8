package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/tinoosan/agen8/pkg/buildinfo"
)

func main() {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "agen8: %v\n", err)
		os.Exit(1)
	}
}

// newRootCmd assembles the command tree. Bare `agen8` and `agen8 help` print the
// synopsis; the daemon is always launched explicitly via `daemon start`
// (.air.toml and the Dockerfile CMD both pass it), so there is no daemon default.
func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "agen8",
		Short:         "Durable work-context daemon and MCP server",
		SilenceUsage:  true, // a failing RunE shouldn't dump full usage…
		SilenceErrors: true, // …and main() prints the error with the agen8: prefix
		// Bare invocation prints help instead of running anything.
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
		// Version powers `agen8 --version`; the version subcommand prints the same.
		Version: buildinfo.Current().Version,
	}
	root.SetVersionTemplate(versionText())
	root.AddCommand(
		newDaemonCmd(),
		newClientCmd(),
		newSkillCmd(),
		newHooksCmd(),
		newHealthcheckCmd(),
		newClaudeCmd(),
		newVersionCmd(),
	)
	return root
}
