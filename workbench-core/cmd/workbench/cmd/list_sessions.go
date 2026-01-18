package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/tinoosan/workbench-core/internal/store"
)

var listSessionsCmd = &cobra.Command{
	Use:   "sessions",
	Short: "List sessions",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := effectiveConfig()
		if err != nil {
			return err
		}
		ids, err := store.ListSessionIDs(cfg)
		if err != nil {
			return err
		}
		out := cmd.OutOrStdout()
		for _, id := range ids {
			fmt.Fprintln(out, id)
		}
		return nil
	},
}
