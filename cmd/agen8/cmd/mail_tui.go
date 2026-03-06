package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tinoosan/agen8/internal/tui/mail"
)

func runMailTUI(cmd *cobra.Command) error {
	explicitSession := strings.TrimSpace(mailWatchSessionID) != ""
	projectRoot := projectSearchDir()
	sessionID := strings.TrimSpace(mailWatchSessionID)
	if sessionID == "" {
		resolvedRoot, _, resolvedSessionID, _, err := resolveActiveProjectScope(cmd.Context())
		if err == nil {
			projectRoot = resolvedRoot
			sessionID = resolvedSessionID
		}
	}

	// Interactive mode: project-first when we have a project root and no
	// explicit --session-id; session-first otherwise.
	if !explicitSession && strings.TrimSpace(projectRoot) != "" {
		return mail.Run(resolvedRPCEndpoint(), mail.Options{
			ProjectRoot:        projectRoot,
			FollowProjectState: true,
			SessionID:          sessionID,
			SessionExplicit:    false,
		})
	}

	if sessionID == "" {
		return fmt.Errorf("active team is required (start a team with `agen8 team start <profile-ref>`)")
	}
	return mail.Run(resolvedRPCEndpoint(), mail.Options{
		ProjectRoot:        projectRoot,
		FollowProjectState: !explicitSession,
		SessionID:          sessionID,
		SessionExplicit:    explicitSession,
	})
}
