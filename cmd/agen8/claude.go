package main

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/tinoosan/agen8/internal/claudehook"
)

// newClaudeCmd builds the `claude` group. It is Hidden: `claude hook` is an
// internal entrypoint that Claude Code invokes on every mcp__agen8__* tool call
// (it stamps the conversation's session_id so each conversation resolves to its
// own member — see internal/claudehook and internal/mcp/server.go). It is wired
// into the harness by `hooks install`, never typed by a human, so it does not
// belong in the help synopsis.
func newClaudeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:    "claude",
		Short:  "Claude Code integration entrypoints (internal)",
		Hidden: true,
		Args:   cobra.NoArgs,
	}
	cmd.AddCommand(&cobra.Command{
		Use:    "hook",
		Short:  "Claude Code PreToolUse hook entrypoint (reads a hook payload on stdin)",
		Hidden: true,
		Args:   cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return claudehook.Run(os.Stdin, os.Stdout, os.Stderr)
		},
	})
	return cmd
}
