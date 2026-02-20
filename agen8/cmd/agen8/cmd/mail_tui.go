package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tinoosan/agen8/internal/tui/mail"
)

func runMailTUI(cmd *cobra.Command) error {
	sessionID := strings.TrimSpace(mailWatchSessionID)
	if sessionID == "" {
		projectCtx, err := loadProjectContext()
		if err == nil && projectCtx.Exists {
			sessionID = strings.TrimSpace(projectCtx.State.ActiveSessionID)
		}
	}
	if sessionID == "" {
		return fmt.Errorf("session id is required (use --session-id or initialize project and attach a session)")
	}
	return mail.Run(resolvedRPCEndpoint(), sessionID)
}
