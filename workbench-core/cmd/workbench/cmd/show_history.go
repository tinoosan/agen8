package cmd

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tinoosan/workbench-core/internal/store"
)

var showHistoryTail int

var showHistoryCmd = &cobra.Command{
	Use:   "history <sessionId>",
	Short: "Show recent session history (JSONL)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := effectiveConfig()
		if err != nil {
			return err
		}
		sessionID := strings.TrimSpace(args[0])
		if sessionID == "" {
			return fmt.Errorf("sessionId is required")
		}
		hs, err := store.NewDiskHistoryStore(cfg, sessionID)
		if err != nil {
			return err
		}
		limit := showHistoryTail
		if limit <= 0 {
			limit = 50
		}
		batch, err := hs.LinesLatest(cmd.Context(), store.HistoryLatestOptions{MaxBytes: 64 * 1024, Limit: limit})
		if err != nil {
			return err
		}
		out := cmd.OutOrStdout()
		for _, line := range batch.Lines {
			_, _ = fmt.Fprintln(out, string(bytes.TrimSpace(line)))
		}
		return nil
	},
}

func init() {
	showHistoryCmd.Flags().IntVar(&showHistoryTail, "tail", 50, "number of history lines to print")
}
