package cmd

import "github.com/spf13/cobra"

var showCmd = &cobra.Command{
	Use:   "show",
	Short: "Show detailed metadata",
}

func init() {
	showCmd.AddCommand(showSessionCmd)
	showCmd.AddCommand(showRunCmd)
	showCmd.AddCommand(showHistoryCmd)
}
