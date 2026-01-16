package cmd

import "github.com/spf13/cobra"

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List workbench resources",
}

func init() {
	listCmd.AddCommand(listSessionsCmd)
	listCmd.AddCommand(listRunsCmd)
}
