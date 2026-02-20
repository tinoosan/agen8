package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

func runSmartEntrypoint(cmd *cobra.Command) error {
	cfg, err := effectiveConfig(cmd)
	if err != nil {
		return err
	}

	projectCtx, projectErr := loadProjectContext()
	if projectErr != nil {
		return projectErr
	}

	daemonReachable := rpcPing(cmd.Context()) == nil
	if daemonReachable && projectCtx.Exists && strings.TrimSpace(projectCtx.State.ActiveSessionID) != "" {
		return runCoordinatorFn(cmd, strings.TrimSpace(projectCtx.State.ActiveSessionID))
	}

	if !daemonReachable {
		// Preserve existing workflow: monitor still starts even if daemon is down.
		return runDetachedMonitorFn(cmd.Context(), cfg)
	}

	if !isInteractiveTerminal() {
		return runDashboardFlow(cmd)
	}

	choice, err := promptRootChoice()
	if err != nil {
		return err
	}
	switch choice {
	case "1", "new":
		return runNewSessionFlow(cmd, true)
	case "2", "attach":
		reader := bufio.NewReader(os.Stdin)
		fmt.Fprint(cmd.OutOrStdout(), "Session ID: ")
		raw, _ := reader.ReadString('\n')
		sessionID := strings.TrimSpace(raw)
		if sessionID == "" {
			return fmt.Errorf("session id is required")
		}
		return runCoordinatorFn(cmd, sessionID)
	case "3", "dashboard":
		return runDashboardFlow(cmd)
	default:
		return runDashboardFlow(cmd)
	}
}

func promptRootChoice() (string, error) {
	reader := bufio.NewReader(os.Stdin)
	fmt.Println("Agen8")
	fmt.Println("1) New session")
	fmt.Println("2) Attach existing session")
	fmt.Println("3) Dashboard")
	fmt.Print("Choose [1/2/3]: ")
	raw, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.ToLower(strings.TrimSpace(raw)), nil
}

func isInteractiveTerminal() bool {
	info, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}
