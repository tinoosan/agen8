package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tinoosan/agen8/internal/tui/mail"
)

func runMailTUI(cmd *cobra.Command) error {
	followProjectState := strings.TrimSpace(mailWatchSessionID) == ""
	projectRoot := projectSearchDir()
	sessionID := strings.TrimSpace(mailWatchSessionID)
	if sessionID == "" {
		projectCtx, err := loadProjectContext()
		if err == nil && projectCtx.Exists {
			projectRoot = strings.TrimSpace(projectCtx.RootDir)
			sessionID = strings.TrimSpace(projectCtx.State.ActiveSessionID)
		}
	}
	if sessionID == "" {
		return fmt.Errorf("active team session is required (start a team with `agen8 team start <profile-ref>` or pass --session-id)")
	}
	return mail.Run(resolvedRPCEndpoint(), sessionID, mail.Options{
		ProjectRoot:        projectRoot,
		FollowProjectState: followProjectState,
	})
}
