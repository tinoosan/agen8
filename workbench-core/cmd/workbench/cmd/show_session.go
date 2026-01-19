package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tinoosan/workbench-core/internal/jsonutil"
	"github.com/tinoosan/workbench-core/internal/store"
)

var showSessionCmd = &cobra.Command{
	Use:   "session <sessionId>",
	Short: "Show session.json for a session",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := effectiveConfig(cmd)
		if err != nil {
			return err
		}
		sessionID := strings.TrimSpace(args[0])
		if sessionID == "" {
			return fmt.Errorf("sessionId is required")
		}
		sess, err := store.LoadSession(cfg, sessionID)
		if err != nil {
			return err
		}
		b, err := jsonutil.MarshalPretty(sess)
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(b))
		return nil
	},
}
