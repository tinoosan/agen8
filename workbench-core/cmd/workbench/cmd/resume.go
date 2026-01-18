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
		cfg, err := effectiveConfig()
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
		parent := strings.TrimSpace(sess.CurrentRunID)
		run, err := store.CreateRunInSession(cfg, sessionID, parent, "resume session", maxContextB)
		if err != nil {
			return err
		}

		modelOverride := ""
		if cmd.Root().PersistentFlags().Changed("model") {
			modelOverride = strings.TrimSpace(modelID)
		}
		opts := app.RunChatOptions{
			Model:                 modelOverride,
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
			return app.RunChatTUI(cmd.Context(), cfg, run, opts)
		case "repl":
			return app.RunChat(cmd.Context(), cfg, run, opts)
		default:
			return fmt.Errorf("unknown --ui %q (expected tui or repl)", uiMode)
		}
	},
}
