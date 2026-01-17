package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tinoosan/workbench-core/internal/app"
	"github.com/tinoosan/workbench-core/internal/store"
)

var resumeCmd = &cobra.Command{
	Use:   "resume <sessionId>",
	Short: "Resume a previous session by starting a new run",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		sessionID := strings.TrimSpace(args[0])
		if sessionID == "" {
			return fmt.Errorf("sessionId is required")
		}
		sess, err := store.LoadSession(sessionID)
		if err != nil {
			return err
		}
		parent := strings.TrimSpace(sess.CurrentRunID)
		run, err := store.CreateRunInSession(sessionID, parent, "resume session", maxContextB)
		if err != nil {
			return err
		}
		opts := app.RunChatOptions{
			Model:                 modelID,
			WorkDir:               workDir,
			MaxSteps:              maxSteps,
			MaxTraceBytes:         maxTraceBytes,
			MaxMemoryBytes:        maxMemoryBytes,
			MaxProfileBytes:       maxProfileBytes,
			RecentHistoryPairs:    recentHistoryPairs,
			UserID:                userID,
			IncludeHistoryOps:     &includeHistoryOps,
			PriceInPerMTokensUSD:  priceInPerM,
			PriceOutPerMTokensUSD: priceOutPerM,
			PricingFile:           pricingFile,
		}
		switch strings.ToLower(strings.TrimSpace(uiMode)) {
		case "", "tui":
			return app.RunChatTUI(cmd.Context(), run, opts)
		case "repl":
			return app.RunChat(cmd.Context(), run, opts)
		default:
			return fmt.Errorf("unknown --ui %q (expected tui or repl)", uiMode)
		}
	},
}
