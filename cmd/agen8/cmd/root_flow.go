package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

func runSmartEntrypoint(cmd *cobra.Command) error {
	return cmd.Help()
}

func isInteractiveTerminal() bool {
	info, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}
