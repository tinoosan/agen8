package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/tinoosan/agen8/pkg/buildinfo"
)

// versionText renders version, commit, and build date. Shared by the `version`
// subcommand and the root `--version` template so both print identically.
func versionText() string {
	info := buildinfo.Current()
	out := fmt.Sprintf("agen8 %s\ncommit: %s\n", info.Version, info.Commit)
	if info.BuildDate != "" {
		out += fmt.Sprintf("built: %s\n", info.BuildDate)
	}
	return out
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version, commit, and build date",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			fmt.Fprint(cmd.OutOrStdout(), versionText())
			return nil
		},
	}
}
